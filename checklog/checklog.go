// Copyright 2021 - 2022 Matrix Origin
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package checklog

import (
	"fmt"
	"go/ast"
	"go/types"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/ast/inspector"
	"regexp"
	"strconv"
	"strings"
)

const doc = "Tool to check the usage of log.*() or fmt.Print*() to ensure it is only used by pre-approved methods and functions."

// Analyzer check usage of log.* and fmt.Print*() with following rules
// - exclude *_test.go file
// - exclude functions in whiteList4fmt
var Analyzer = &analysis.Analyzer{
	Name:     "checklog",
	Doc:      doc,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	// get the inspector. This will not panic because inspect.Analyzer is part
	// of `Requires`. go/analysis will populate the `pass.ResultOf` map with
	// the prerequisite analyzers.
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	// the inspector has a `filter` feature that enables type-based filtering
	// The anonymous function will be only called for the ast nodes whose type
	// matches an element in the filter
	nodeFilter := []ast.Node{
		(*ast.ImportSpec)(nil),
		(*ast.CallExpr)(nil),
	}
	packagePath2Name := make(map[string]string)
	// this is basically the same as ast.Inspect(), only we don't return a
	// boolean anymore as it'll visit all the nodes based on the filter.
	inspect.Preorder(nodeFilter, func(n ast.Node) {

		switch node := n.(type) {
		case *ast.ImportSpec:
			path, err := strconv.Unquote(node.Path.Value)
			if err != nil {
				fmt.Errorf("[warn] failed to unquote: %s\n", node.Path.Value)
				path = strings.Trim(node.Path.Value, `"`)
			}
			if node.Name != nil {
				packagePath2Name[path] = node.Name.String()
			}
			for _, spec := range blockPackageList {
				if path == spec.MatchPath {
					if spec.PassTestFile && isTestFile(pass, n) {
						continue
					}
					message := fmt.Sprintf("do not use package %s", spec.MatchPath)
					if spec.UseInstead != "" {
						message += fmt.Sprintf(", use %s instead", spec.UseInstead)
					}
					pass.Reportf(n.Pos(), message)
				}
			}
		case *ast.CallExpr:
			for _, spec := range blockFuncList {
				function := getCallingFunction(node.Fun)
				if function == spec.getFunction(packagePath2Name) && !isTestFile(pass, n) {
					typeName, funcName, ok := getMethodDetails(pass, n)
					if !ok {
						typeName, funcName, ok = getFunctionDetails(pass, n)
						if !ok {
							panic("failed to identify method/function details")
						}
					}
					if isWhiteListed(whiteList4fmt, typeName, funcName) {
						continue
					}
					pass.Reportf(n.Pos(), "unsafe log %s: in %s.%s()", function, typeName, funcName)
				}
			}
		default:
			return
		}
	})
	return nil, nil
}

type blockSpec struct {
	MatchModule  string
	MatchPath    string
	UseInstead   string
	PassTestFile bool
}

var blockPackageList = []blockSpec{
	{
		MatchPath:    "log",
		UseInstead:   "github.com/matrixorigin/matrixone/pkg/logutil",
		PassTestFile: true,
	},
}

type functionSpec struct {
	Module   string // like: fmt
	Function string // like: Printf
}

func (spec functionSpec) getFunction(pkgAlias map[string]string) string {
	if len(spec.Module) == 0 {
		return spec.Function
	} else {
		var moduleSelector = spec.Module
		if val, ok := pkgAlias[spec.Module]; ok {
			moduleSelector = val
		}
		return moduleSelector + "." + spec.Function
	}
}

var blockFuncList = []functionSpec{
	{Module: "fmt", Function: "Print"},
	{Module: "fmt", Function: "Printf"},
	{Module: "fmt", Function: "Println"},
}

// approved like
// - struct{ package_path,            global_function }
// - struct{ package_path.class_name, member_function }
type approved struct {
	receiverTypeName string
	functionName     string
}

// whiteList4fmt array of approved
var whiteList4fmt = []approved{
	// bin tool
	{`github.com/matrixorigin/matrixone/pkg/sql/parsers/goyacc`, `*`},
	// version info
	{`github.com/matrixorigin/matrixone/cmd/mo-service`, `maybePrintVersion`},
	// example
	{`.*/example`, `*`},
	{`.*/example\..*`, `*`},
	// generated
	{`github.com/matrixorigin/matrixone/pkg/sql/parsers/dialect/postgresql`, `*`},
	{`github.com/matrixorigin/matrixone/pkg/sql/parsers/dialect/mysql`, `*`},
}

func isWhiteListed(whiteList []approved, typeName string, functionName string) (match bool) {
	for _, rec := range whiteList {
		if rec.receiverTypeName == typeName && (rec.functionName == functionName || rec.functionName == "*") {
			return true
		}
		valuesRe := regexp.MustCompile(rec.receiverTypeName)
		if len(valuesRe.FindAllString(typeName, -1)) > 0 && (rec.functionName == functionName || rec.functionName == "*") {
			return true
		}
	}
	return false
}

// getSelector parse X ast.Expr in ast.SelectorExpr.
// like:         a.Func1()
// - or:       a.b.Func2()
// - or:  a.arr[0].Func3()
// -------------- | -------
// -            X | Sel
func getSelector(xExpr ast.Expr) (selector string) {
	if id, ok := xExpr.(*ast.Ident); ok {
		selector = id.Name
	} else if se, ok := xExpr.(*ast.SelectorExpr); ok {
		selector = getSelector(se.X) + "." + se.Sel.Name
	} else if _, ok := xExpr.(*ast.IndexExpr); ok {
		selector = "" // not implement
	}
	return selector
}

func getCallingFunction(expr ast.Expr) (function string) {
	if s, ok := expr.(*ast.SelectorExpr); ok {
		var selector = getSelector(s.X)
		var currentFunc = s.Sel.Name
		if len(selector) > 0 {
			currentFunc = selector + "." + currentFunc
		}
		function = currentFunc
	} else if id, ok := expr.(*ast.Ident); ok {
		function = id.Name
	}
	return function
}

func isTestFile(pass *analysis.Pass, node ast.Node) bool {
	var file *ast.File
	for _, f := range pass.Files {
		if f.Pos() <= node.Pos() && node.Pos() < f.End() {
			file = f
			break
		}
	}
	if file == nil {
		panic("failed to locate the file")
	}
	tokenFile := pass.Fset.File(file.Pos())
	if tokenFile == nil {
		panic("token file is nil")
	}

	return strings.HasSuffix(tokenFile.Name(), "_test.go")
}

func getMethodDetails(pass *analysis.Pass, node ast.Node) (string, string, bool) {
	for _, file := range pass.Files {
		path, _ := astutil.PathEnclosingInterval(file, node.Pos(), node.Pos())
		if len(path) == 0 {
			continue
		}
		for _, cp := range path {
			if fd, ok := cp.(*ast.FuncDecl); ok && fd.Recv != nil {
				recvExp := fd.Recv.List[0].Type
				if p, ok := pass.TypesInfo.Types[recvExp].Type.(*types.Pointer); ok {
					return p.Elem().String(), fd.Name.Name, true
				}
			}
		}
	}

	return "", "", false
}

func getFunctionDetails(pass *analysis.Pass, node ast.Node) (string, string, bool) {
	for _, file := range pass.Files {
		path, _ := astutil.PathEnclosingInterval(file, node.Pos(), node.Pos())
		if len(path) == 0 {
			continue
		}
		for _, cp := range path {
			if fd, ok := cp.(*ast.FuncDecl); ok && fd.Recv == nil {
				p := pass.TypesInfo.Defs[fd.Name].Pkg().Path()
				return p, fd.Name.Name, true
			}
		}
	}

	return "", "", false
}
