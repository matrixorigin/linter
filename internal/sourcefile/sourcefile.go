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

package sourcefile

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// TestFiles contains logical test-file status for overlay backing files.
// It is initialized once in the vettool process before analysis starts.
var TestFiles map[string]bool

func IsTest(pass *analysis.Pass, node ast.Node) bool {
	f := pass.Fset.File(node.Pos())
	if f == nil {
		panic("failed to locate the source file")
	}
	if test, ok := TestFiles[f.Name()]; ok {
		return test
	}
	return strings.HasSuffix(f.Name(), "_test.go")
}
