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
	{"github.com/matrixorigin/matrixone/pkg/queryservice", "RequestMultipleCn"},

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
	{"github.com/matrixorigin/matrixone/pkg/frontend", "executeStmtWithWorkspace"},
	{"github.com/matrixorigin/matrixone/pkg/frontend", "isConnected"},

	// transaction state management
	{"github.com/matrixorigin/matrixone/pkg/frontend.MysqlCmdExecutor", "executeStmt"},

	// cdc related
	{"github.com/matrixorigin/matrixone/pkg/cdc.TableDetector", "processCallback"},

	{"github.com/matrixorigin/matrixone/pkg/common/morpc.remoteBackend", "writeLoop"},
	{"github.com/matrixorigin/matrixone/pkg/common/morpc.remoteBackend", "readLoop"},

	// recover for inpsect dn
	{"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/rpc.Handle", "HandleInspectTN"},
	{"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/tasks.CancelableJob", "Start"},

	// recover for engine
	{"github.com/matrixorigin/matrixone/pkg/vm/engine/disttae.GlobalStats", "consumeLogtail"},

	// recover for catching error.
	// we cannot change the Aliyun SDK interface, so throw and catch errors with panic and recover
	{"github.com/matrixorigin/matrixone/pkg/fileservice", "catch"},

	// temporary recover to location issue #16007
	{"github.com/matrixorigin/matrixone/pkg/pb/status.Session", "MarshalToSizedBuffer"},

	// temporary recover to location issue #19755
	{"github.com/matrixorigin/matrixone/pkg/util/export.bufferHolder", "Add"},
	{"github.com/matrixorigin/matrixone/pkg/util/export.bufferHolder", "getGenerateReq"},
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, errors.New("analyzer is not type *inspector.Inspector")
	}

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	var analysisErr error
	inspect.Preorder(nodeFilter, func(node ast.Node) {
		if analysisErr != nil {
			return
		}
		stmt, ok := node.(*ast.CallExpr)
		if !ok {
			return
		}
		isRecover, err := isCallingBuiltInRecover(pass, stmt.Fun)
		if err != nil {
			analysisErr = err
			return
		}
		if isRecover && !isTestFile(pass, node) {
			typeName, funcName, ok := getMethodDetails(pass, node)
			if !ok {
				typeName, funcName, ok = getFunctionDetails(pass, node)
				if !ok {
					typeName, funcName = pass.Pkg.Path(), "<package initializer>"
				}
			}
			if !isWhiteListed(whiteList, typeName, funcName) {
				msg := fmt.Sprintf("%v: in %s.%s()",
					errUnsafeRecover,
					typeName,
					funcName,
				)
				pass.Reportf(node.Pos(), "%s", msg)
			}
		}
	})

	return nil, analysisErr
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
				recvType := types.Unalias(pass.TypesInfo.TypeOf(fd.Recv.List[0].Type))
				if p, ok := recvType.(*types.Pointer); ok {
					recvType = types.Unalias(p.Elem())
				}
				if named, ok := recvType.(*types.Named); ok && named.Obj().Pkg() != nil {
					return named.Obj().Pkg().Path() + "." + named.Obj().Name(), fd.Name.Name, true
				}
			}
		}
	}

	return "", "", false
}

func isCallingBuiltInRecover(pass *analysis.Pass, expr ast.Expr) (bool, error) {
	id, ok := ast.Unparen(expr).(*ast.Ident)
	if !ok || id.Name != "recover" {
		return false, nil
	}
	if pass.TypesInfo == nil || pass.TypesInfo.Uses[id] == nil {
		return false, fmt.Errorf("missing type information for recover at %s", pass.Fset.Position(id.Pos()))
	}
	return pass.TypesInfo.Uses[id] == types.Universe.Lookup("recover"), nil
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
