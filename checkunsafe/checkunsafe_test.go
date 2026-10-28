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

package checkunsafe

import "testing"

func TestMessageUsesSourceProvenance(t *testing.T) {
	for _, tt := range []struct {
		name, pkg, file string
		cgo, allowed    bool
	}{
		{"historic cgo", "github.com/matrixorigin/matrixone/pkg/common/docfilter", "cbitmap.go", true, true},
		{"ordinary cooked-looking file", "github.com/matrixorigin/matrixone/pkg/common/docfilter", "cbitmap.cgo1.go", false, false},
		{"linux cgo source", "github.com/matrixorigin/matrixone/pkg/common/malloc", "c_allocator_linux.go", true, true},
		{"ordinary approved source", "github.com/matrixorigin/matrixone/pkg/common/docfilter", "filter.go", false, true},
		{"unrelated cgo source", "github.com/matrixorigin/matrixone/pkg/common/docfilter", "other.go", true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Message(tt.pkg, tt.file, tt.cgo); (got == "") != tt.allowed {
				t.Fatalf("message %q; allowed=%v", got, tt.allowed)
			}
		})
	}
}
