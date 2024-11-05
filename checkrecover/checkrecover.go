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

package checkrecover

import (
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/ast/inspector"
)

const doc = "Tool to check the usage of recover() to ensure it is only used by pre-approved methods and functions. See https://github.com/matrixorigin/matrixone/blob/main/pkg/common/moerr/error_handling.md for more details."

var errUnsafeRecover = errors.New(
	"unsafe recover() found",
)

var Analyzer = &analysis.Analyzer{
	Name:     "checkrecover",
	Doc:      doc,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

type approved struct {
	receiverTypeName string
	functionName     string
}

var whiteList = []approved{
	// recover for pipeline building and pipeline running.
	{"github.com/matrixorigin/matrixone/pkg/sql/compile.Compile", "Compile"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile.Compile", "runOnce"},
	{"github.com/matrixorigin/matrixone/pkg/vm", "Run"},
	{"github.com/matrixorigin/matrixone/pkg/vm/pipeline", "catchPanic"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile.Scope", "Run"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile.Scope", "MergeRun"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile.Scope", "ParallelRun"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile.Scope", "remoteRun"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile.Scope", "RemoteRun"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile", "CnServerMessageHandler"},

	// https://github.com/matrixorigin/matrixone/issues/2764
	{"github.com/matrixorigin/matrixone/pkg/frontend.MysqlCmdExecutor", "ExecRequest"},
	{"github.com/matrixorigin/matrixone/pkg/frontend", "ExecRequest"},
	{"github.com/matrixorigin/matrixone/pkg/frontend", "commitTxnFunc"},
	{"github.com/matrixorigin/matrixone/pkg/frontend", "finishTxnFunc"},
	{"github.com/matrixorigin/matrixone/pkg/frontend.Conn", "ReadLoadLocalPacket"},
	{"github.com/matrixorigin/matrixone/pkg/frontend.Conn", "Read"},
	{"github.com/matrixorigin/matrixone/pkg/frontend.Conn", "ReadOnePayload"},
	{"github.com/matrixorigin/matrixone/pkg/frontend.Conn", "AllocNewBlock"},
	{"github.com/matrixorigin/matrixone/pkg/frontend.Conn", "ExecuteFuncWithRecover"},
	{"github.com/matrixorigin/matrixone/pkg/frontend", "ExecuteFuncWithRecover"},

	// transaction state management
	{"github.com/matrixorigin/matrixone/pkg/frontend.MysqlCmdExecutor", "executeStmt"},

	{"github.com/matrixorigin/matrixone/pkg/common/morpc.remoteBackend", "writeLoop"},
	{"github.com/matrixorigin/matrixone/pkg/common/morpc.remoteBackend", "readLoop"},

	// recover for inpsect dn
	{"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/rpc.Handle", "HandleInspectTN"},

	// recover for catching error.
	// we cannot change the Aliyun SDK interface, so throw and catch errors with panic and recover
	{"github.com/matrixorigin/matrixone/pkg/fileservice", "catch"},

	// temporary recover to location issue #16007
	{"github.com/matrixorigin/matrixone/pkg/pb/status.Session", "MarshalToSizedBuffer"},
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, errors.New("analyzer is not type *inspector.Inspector")
	}

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	inspect.Preorder(nodeFilter, func(node ast.Node) {
		stmt, ok := node.(*ast.CallExpr)
		if !ok {
			return
		}
		if isCallingBuiltInRecover(stmt.Fun) && !isTestFile(pass, node) {
			typeName, funcName, ok := getMethodDetails(pass, node)
			if !ok {
				typeName, funcName, ok = getFunctionDetails(pass, node)
				if !ok {
					panic("failed to identify method/function details")
				}
			}
			if !isWhiteListed(whiteList, typeName, funcName) {
				msg := fmt.Sprintf("%v: in %s.%s()",
					errUnsafeRecover,
					typeName,
					funcName,
				)
				pass.Reportf(node.Pos(), msg)
			}
		}
	})

	return nil, nil
}

func isWhiteListed(whiteList []approved, typeName string, functionName string) bool {
	for _, rec := range whiteList {
		if rec.receiverTypeName == typeName && rec.functionName == functionName {
			return true
		}
	}

	return false
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

func isCallingBuiltInRecover(expr ast.Expr) bool {
	if _, ok := expr.(*ast.SelectorExpr); ok {
		return false
	}
	id, ok := expr.(*ast.Ident)

	return ok && id.Name == "recover"
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
