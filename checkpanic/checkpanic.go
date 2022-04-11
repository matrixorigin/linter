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

package checkpanic

import (
	"errors"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const doc = `Check usages of panic() to ensure argument is *moerr.Error`

var Analyzer = &analysis.Analyzer{
	Name:     "checkpanic",
	Doc:      doc,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

const expectedType = "*github.com/matrixorigin/matrixone/pkg/common/moerr.Error"

func run(pass *analysis.Pass) (any, error) {
	inspect, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, errors.New("analyzer is not type *inspector.Inspector")
	}

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	inspect.Preorder(nodeFilter, func(node ast.Node) {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok {
			return
		}
		if ident.Name != "panic" {
			return
		}

		var file *ast.File
		for _, f := range pass.Files {
			if f.Pos() <= node.Pos() && node.Pos() < f.End() {
				file = f
				break
			}
		}
		tokenFile := pass.Fset.File(file.Pos())
		if strings.HasSuffix(tokenFile.Name(), "_test.go") {
			return
		}

		typ := pass.TypesInfo.Types[call.Args[0]].Type
		if typ.String() != expectedType {
			pass.Reportf(call.Pos(), "argument to panic should be %s", expectedType)
		}

	})

	return nil, nil
}
