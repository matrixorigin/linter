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
	"regexp"
	"strconv"
	"strings"

	"github.com/matrixorigin/linter/internal/sourcefile"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/ast/inspector"
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
	var runErr error
	// this is basically the same as ast.Inspect(), only we don't return a
	// boolean anymore as it'll visit all the nodes based on the filter.
	inspect.Preorder(nodeFilter, func(n ast.Node) {
		if runErr != nil {
			return
		}

		switch node := n.(type) {
		case *ast.ImportSpec:
			path, err := strconv.Unquote(node.Path.Value)
			if err != nil {
				path = strings.Trim(node.Path.Value, `"`)
			}
			for _, spec := range blockPackageList {
				if path == spec.MatchPath {
					if spec.PassTestFile && sourcefile.IsTest(pass, n) {
						continue
					}
					message := fmt.Sprintf("do not use package %s", spec.MatchPath)
					if spec.UseInstead != "" {
						message += fmt.Sprintf(", use %s instead", spec.UseInstead)
					}
					pass.Reportf(n.Pos(), "%s", message)
				}
			}
		case *ast.CallExpr:
			function, err := blockedFunction(pass, node.Fun)
			if err != nil {
				runErr = err
				return
			}
			if function != "" && !sourcefile.IsTest(pass, n) {
				// $pass is a specific analyzer to a single Go package.
				// $n is the function calling, including: filename, pos
				typeName, funcName, ok := getMethodDetails(pass, n)
				if !ok {
					typeName, funcName, ok = getFunctionDetails(pass, n)
					if !ok {
						// example:
						// pkg/vm/engine/tae/db/merge/simulation/simulation.go:46:3: unsafe log fmt.Println
						pass.Reportf(n.Pos(), "uncatch log fuction: %s", function)
						return
					}
				}
				if isWhiteListed(whiteList4fmt, typeName, funcName) {
					return
				}
				pass.Reportf(n.Pos(), "uncatch log function: %s: in %s.%s()", function, typeName, funcName)
			}
		default:
			return
		}
	})
	return nil, runErr
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

func blockedFunction(pass *analysis.Pass, expr ast.Expr) (string, error) {
	var id *ast.Ident
	switch expr := ast.Unparen(expr).(type) {
	case *ast.Ident:
		id = expr
	case *ast.SelectorExpr:
		id = expr.Sel
	default:
		return "", nil
	}
	if id.Name != "Print" && id.Name != "Printf" && id.Name != "Println" {
		return "", nil
	}
	if pass.TypesInfo == nil || pass.TypesInfo.Uses[id] == nil {
		return "", fmt.Errorf("missing type information for %s at %s", id.Name, pass.Fset.Position(id.Pos()))
	}
	fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
	if ok && fn.Pkg() != nil && fn.Pkg().Path() == "fmt" && fn.Parent() == fn.Pkg().Scope() {
		return "fmt." + id.Name, nil
	}
	return "", nil
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
	// mo-debug
	{`github.com/matrixorigin/matrixone/cmd/mo-debug`, `*`},
	// Opt-in regexp2 engine tracing preserves the upstream Debug output.
	{`^github[.]com/matrixorigin/matrixone/pkg/sql/plan/function/internal/regexp2[.]runner$`, `scan`},
	{`^github[.]com/matrixorigin/matrixone/pkg/sql/plan/function/internal/regexp2[.]runner$`, `backtrack`},
	{`^github[.]com/matrixorigin/matrixone/pkg/sql/plan/function/internal/regexp2[.]runner$`, `dumpState`},
	// This standalone E2E command prints its report location for its caller.
	{`^github[.]com/matrixorigin/matrixone/test/kafkaconnect$`, `runMain`},
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

func getMethodDetails(pass *analysis.Pass, node ast.Node) (string, string, bool) {
	for _, file := range pass.Files {
		path, _ := astutil.PathEnclosingInterval(file, node.Pos(), node.End())
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
		path, _ := astutil.PathEnclosingInterval(file, node.Pos(), node.End())
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
