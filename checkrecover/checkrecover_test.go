// Copyright 2026 Matrix Origin
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package checkrecover

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestRecoverRequiresTypeInformation(t *testing.T) {
	fset := token.NewFileSet()
	file := fset.AddFile("input.go", -1, 8)
	id := &ast.Ident{NamePos: file.Pos(0), Name: "recover"}
	for _, info := range []*types.Info{nil, {Uses: make(map[*ast.Ident]types.Object)}} {
		pass := &analysis.Pass{Fset: fset, TypesInfo: info}
		if _, err := isCallingBuiltInRecover(pass, id); err == nil {
			t.Fatal("missing type information must fail analysis")
		}
	}
}
