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

package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func testSourceSelection(t *testing.T, tool string) {
	for _, cgo := range []bool{false, true} {
		name := "go"
		preamble := "package fixture\n"
		if cgo {
			name = "cgo"
			preamble += "import \"C\"\n"
		}
		t.Run(name+" overlay", func(t *testing.T) {
			dir := t.TempDir()
			overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{"fixture.go": ".overlay/source.go"}})
			if err != nil {
				t.Fatal(err)
			}
			writeFixture(t, dir, map[string]string{
				"go.mod":       "module example.org/overlay-fixture\ngo 1.24\n",
				"overlay.json": string(overlay),
				"empty.json":   "{}",
			})
			for _, overlayUnsafe := range []bool{false, true, false} {
				disk, replacement := preamble+"import _ \"unsafe\"\n", preamble
				if overlayUnsafe {
					disk, replacement = replacement, disk
				}
				writeFixture(t, dir, map[string]string{"fixture.go": disk, ".overlay/source.go": replacement})
				// A CLI overlay must override GOFLAGS for BOTH phases.
				t.Setenv("GOFLAGS", "-p=2 -overlay=empty.json")
				code, out := runTool(t, dir, tool, "check", "-overlay=overlay.json", "./...")
				want := 0
				if overlayUnsafe {
					want = 1
				}
				if code != want || (want == 1 && strings.Count(out, "importing the unsafe package") != 1) {
					t.Fatalf("overlay unsafe=%v: %d %s", overlayUnsafe, code, out)
				}
			}
			// GOFLAGS-only overlay, relative to -C, is the same input source.
			t.Setenv("GOFLAGS", "-p=2 -overlay=overlay.json")
			if code, out := runTool(t, filepath.Dir(dir), tool, "check", "-C", dir, "./..."); code != 0 {
				t.Fatalf("GOFLAGS/-C overlay: %d %s", code, out)
			}
		})
	}
	t.Run("test variants", func(t *testing.T) {
		dir := t.TempDir()
		writeFixture(t, dir, map[string]string{
			"go.mod":           "module example.org/tests-fixture\ngo 1.24\n",
			"fixture.go":       "package fixture\n",
			"internal_test.go": "package fixture\nimport _ \"io/ioutil\"\n",
			"external_test.go": "package fixture_test\nimport _ \"unsafe\"\n",
		})
		code, out := runTool(t, dir, tool, "check", "./...")
		if code != 1 || !strings.Contains(out, "internal_test.go") || !strings.Contains(out, "external_test.go") {
			t.Fatalf("test variants: %d %s", code, out)
		}
	})
	t.Run("dependency-only and build tags", func(t *testing.T) {
		dir := t.TempDir()
		writeFixture(t, dir, map[string]string{
			"go.mod":     "module example.org/selection-fixture\ngo 1.24\n",
			"fixture.go": "package fixture\nimport _ \"example.org/selection-fixture/dep\"\n",
			"dep/dep.go": "package dep\nimport _ \"unsafe\"\n",
			"tagged.go":  "//go:build challenge\n\npackage fixture\nimport _ \"io/ioutil\"\n",
		})
		if code, out := runTool(t, dir, tool, "check", "."); code != 0 {
			t.Fatalf("dependency-only package checked: %d %s", code, out)
		}
		if code, out := runTool(t, dir, tool, "check", "-tags=challenge", "."); code != 1 || !strings.Contains(out, "tagged.go") {
			t.Fatalf("build tags: %d %s", code, out)
		}
		if code, out := runTool(t, dir, tool, "check", "./..."); code != 1 || !strings.Contains(out, "dep.go") {
			t.Fatalf("explicit dependency selection: %d %s", code, out)
		}
		if code, out := runTool(t, dir, tool, "check", "-checkrecover", "./..."); code != 0 {
			t.Fatalf("analyzer selection: %d %s", code, out)
		}
		if code, out := runTool(t, dir, tool, "check", "-pkgblocklist", "-tags=challenge", "."); code != 1 || !strings.Contains(out, "io/ioutil") {
			t.Fatalf("import-only selection: %d %s", code, out)
		}
	})
}

func testModuleSelection(t *testing.T, tool string) {
	dir := t.TempDir()
	// These are valid Go replacements even though the replacement's go.mod
	// declares a different identity. Match logical ownership, not its directory.
	writeFixture(t, dir, map[string]string{
		"replacement/go.mod": "module example.org/code-location\ngo 1.24\n",
		"replacement/p.go":   "package replacement\n",
	})
	t.Setenv("GOPACKAGESDRIVER", filepath.Join(dir, "nonexistent-driver"))
	for _, module := range []string{"example.org/good", "go.etcd.io/etcd"} {
		writeFixture(t, dir, map[string]string{
			"go.mod":     "module example.org/replacement-fixture\ngo 1.24\nrequire " + module + " v1.0.0\nreplace " + module + " => ./replacement\n",
			"fixture.go": "package fixture\nimport _ \"" + module + "\"\n",
			// Deliberately not the module graph selected by -mod=readonly.
			"vendor/modules.txt": "# unrelated.invalid/module v1.0.0\n## explicit; go 1.24\n" + module + "\n",
		})
		t.Setenv("GOFLAGS", "-p=2 -mod=vendor")
		code, out := runTool(t, dir, tool, "check", "-mod=readonly", ".")
		want := 0
		if module == "go.etcd.io/etcd" {
			want = 1
		}
		if code != want || (want == 1 && !strings.Contains(out, "module go.etcd.io/etcd")) {
			t.Fatalf("readonly replacement %s: %d %s", module, code, out)
		}
	}
}
