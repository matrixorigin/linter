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
	// Cleanup retains the original panic; worker handoff rethrows on its owner.
	{"github.com/matrixorigin/matrixone/pkg/common/mpool", "releaseAllocationCapacity"},
	{"github.com/matrixorigin/matrixone/pkg/frontend", "getDataBranchComponentExecutor"},
	{"github.com/matrixorigin/matrixone/pkg/frontend", "finishDataBranchComponent"},
	{"github.com/matrixorigin/matrixone/pkg/frontend", "parseStmtExecute"},
	{"github.com/matrixorigin/matrixone/pkg/frontend.Routine", "handleRequest"},
	{"github.com/matrixorigin/matrixone/pkg/frontend.TxnHandler", "createUnsafe"},
	{"github.com/matrixorigin/matrixone/pkg/iscp", "runInitSQLTransaction"},
	{"github.com/matrixorigin/matrixone/pkg/lockservice", "runLockServicesForTest"},
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec/external/arrowio", "messageFromRangeLease"},
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec/external/arrowio.admissionAllocator", "Allocate"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile.Compile", "Run"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile.Compile", "buildRetryCompile"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile.Compile", "runSiriusRead"},
	{"github.com/matrixorigin/matrixone/pkg/fulltext2.BuildPipeline", "processChunk"},
	{"github.com/matrixorigin/matrixone/pkg/fulltext2.BuildPipeline", "seal"},
	{"github.com/matrixorigin/matrixone/pkg/fulltext2", "fanOut"},

	// Foreign Arrow/FlatBuffers and encoded JSON/object inputs have panic-based accessors.
	{"github.com/matrixorigin/matrixone/pkg/cdc", "TableHasForeignKeyConstraint"},
	{"github.com/matrixorigin/matrixone/pkg/container/arrowbridge", "varlenValueLength"},
	{"github.com/matrixorigin/matrixone/pkg/container/arrowbridge", "validateDictionaryIndices"},
	{"github.com/matrixorigin/matrixone/pkg/container/arrowbridge", "inspectVarlen"},
	{"github.com/matrixorigin/matrixone/pkg/container/arrowipc", "InspectMessage"},
	{"github.com/matrixorigin/matrixone/pkg/container/arrowipc", "ValidateSchemaMetadata"},
	{"github.com/matrixorigin/matrixone/pkg/container/bytejson", "storageCompatibleJSONValue"},
	{"github.com/matrixorigin/matrixone/pkg/container/bytejson.PathIterator", "NextContext"},
	{"github.com/matrixorigin/matrixone/pkg/frontend", "decodeUserVarByteJSONValue"},
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec/external/arrowio", "readFooterBlocks"},
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec/external/arrowio.rangeMessageReader", "Message"},
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec/external/arrowio", "InspectFile"},
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec/external/arrowio.ipcRecordReader", "Next"},
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec/external/arrowio", "Open"},
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec/external/arrowio.streamMessageReader", "Message"},
	{"github.com/matrixorigin/matrixone/pkg/sql/jsonvalue", "ConvertScalarWithContext"},
	{"github.com/matrixorigin/matrixone/pkg/sql/jsonvalue", "safeMarshal"},
	{"github.com/matrixorigin/matrixone/pkg/sql/jsonvalue", "safeScalarText"},
	{"github.com/matrixorigin/matrixone/pkg/tools/objecttool", "decodeObjectColumn"},

	// Named request, worker, callback, and native-driver boundaries report terminal failures.
	{"github.com/matrixorigin/matrixone/pkg/cdc.TableDetector", "processAdmittedCallback"},
	{"github.com/matrixorigin/matrixone/pkg/fileservice.DiskCache", "runAsyncUpdateCallback"},
	{"github.com/matrixorigin/matrixone/pkg/frontend.Conn", "ReadLoadLocalPacketWithTimeout"},
	{"github.com/matrixorigin/matrixone/pkg/frontend", "parseStmtSendLongData"},
	{"github.com/matrixorigin/matrixone/pkg/iscp.Fulltext2SqlWriter", "datalinkText"},
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec/shuffle.Shuffle", "runLocalProducer"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile", "allocationLifecycleCall"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile", "handlePipelineMessage"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile/siriusbridge", "callCleanup"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile/siriusbridge", "callPrepare"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile/siriusbridge.Query", "Run"},
	{"github.com/matrixorigin/matrixone/pkg/sql/compile.Scope", "createSessionTemporaryTable"},
	{"github.com/matrixorigin/matrixone/pkg/vectorindex/cache", "runLifecycleHooks"},

	// Error/EXPLAIN formatting must return a diagnostic error for malformed values.
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec", "FormatDedupKey"},
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec", "FormatDedupEntry"},
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec", "FormatDedupKeyFromRow"},
	{"github.com/matrixorigin/matrixone/pkg/sql/plan/explain", "literalVecText"},
	{"github.com/matrixorigin/matrixone/pkg/sql/plan/explain", "sqlJSONExpr"},
	{"github.com/matrixorigin/matrixone/pkg/sql/plan/substrait", "literalVectorOptions"},

	// Speculative evaluation abandons the optimization; normal execution remains authoritative.
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec", "EvaluateFilterByZoneMap"},
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec", "GetExprZoneMap"},
	{"github.com/matrixorigin/matrixone/pkg/sql/colexec", "evaluateZoneMapFunction"},
	{"github.com/matrixorigin/matrixone/pkg/sql/plan", "blockFilterConstantVectorSet"},
	{"github.com/matrixorigin/matrixone/pkg/sql/plan", "materializeInRHSValues"},
	{"github.com/matrixorigin/matrixone/pkg/sql/plan", "returningQualifierFeature"},
	{"github.com/matrixorigin/matrixone/pkg/sql/plan/rule", "EvaluateConstantExpression"},

	// Parser cancellation/limits catch only their own parseAbort and rethrow all other values.
	{"github.com/matrixorigin/matrixone/pkg/sql/parsers/dialect/mysql", "ParseWithSQLMode"},

	// Closed transaction callback registration reports the existing retry error.
	{"github.com/matrixorigin/matrixone/pkg/incrservice.service", "appendPrivateResetCallback"},
	{"github.com/matrixorigin/matrixone/pkg/incrservice.service", "appendTxnEpochCacheCallback"},

	// Optional metadata probes convert executor panics to an error/unknown, never a known generation.
	{"github.com/matrixorigin/matrixone/pkg/fulltext2", "LoadGeneration"},
	{"github.com/matrixorigin/matrixone/pkg/fulltext2", "QueryGeneration"},
	{"github.com/matrixorigin/matrixone/pkg/vectorindex/cachegen", "LoadCdcGeneration"},
	{"github.com/matrixorigin/matrixone/pkg/vectorindex/cachegen", "QueryCdcGeneration"},
	{"github.com/matrixorigin/matrixone/pkg/vectorindex/hnsw", "loadHnswGeneration"},
	{"github.com/matrixorigin/matrixone/pkg/vectorindex/hnsw", "queryHnswGeneration"},
	{"github.com/matrixorigin/matrixone/pkg/vectorindex/sqlexec", "coverageRead"},
	{"github.com/matrixorigin/matrixone/pkg/vectorindex/sqlexec", "scalarInt64"},
	{"github.com/matrixorigin/matrixone/pkg/vectorindex/sqlexec", "HasProvenanceColumns"},
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
