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
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeFixture(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		name = filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func runTool(t *testing.T, dir, executable string, args ...string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = dir
	cmd.WaitDelay = 5 * time.Second
	ownCommand(cmd)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("command timed out: %v\n%s", ctx.Err(), out)
	}
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return e.ExitCode(), string(out)
		}
		t.Fatalf("start command: %v\n%s", err, out)
	}
	return 0, string(out)
}

// Build once and exercise the public command. Analyzer-only tests cannot prove
// the Go driver protocol, source selection, or module-cache transitions.
func TestCheck(t *testing.T) {
	goCommand := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goCommand += ".exe"
	}
	t.Setenv("PATH", filepath.Join(runtime.GOROOT(), "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-p=2")
	t.Setenv("CGO_CFLAGS", "")
	t.Setenv("CGO_LDFLAGS", "")
	t.Setenv("MOLINT_INTERNAL_VET", "")
	tool := filepath.Join(t.TempDir(), "molint")
	if runtime.GOOS == "windows" {
		tool += ".exe"
	}
	if code, out := runTool(t, ".", goCommand, "build", "-o", tool, "."); code != 0 {
		t.Fatalf("build: %s", out)
	}
	tests := []struct {
		name, source, want string
		module, file       string
		wantCount          int
	}{
		{name: "clean", source: "package fixture\n"},
		{name: "blocked import", source: "package fixture\nimport _ \"io/ioutil\"\n", want: "do not use package io/ioutil"},
		{name: "unsafe import", source: "package fixture\nimport _ \"unsafe\"\n", want: "importing the unsafe package is not allowed"},
		{name: "raw unsafe import", source: "package fixture\nimport _ `unsafe`\n", want: "importing the unsafe package is not allowed"},
		{name: "escaped unsafe import", source: "package fixture\nimport _ \"\\x75nsafe\"\n", want: "importing the unsafe package is not allowed"},
		{name: "allowed unsafe import", module: "github.com/matrixorigin/matrixone/pkg/common/util", file: "unsafe.go", source: "package fixture\nimport _ \"unsafe\"\n"},
		{name: "allowed raw unsafe import", module: "github.com/matrixorigin/matrixone/pkg/common/util", file: "unsafe.go", source: "package fixture\nimport _ `unsafe`\n"},
		{name: "allowed escaped unsafe import", module: "github.com/matrixorigin/matrixone/pkg/common/util", file: "unsafe.go", source: "package fixture\nimport _ \"\\x75nsafe\"\n"},
		{name: "recover", source: "package fixture\nfunc f() { recover() }\n", want: "unsafe recover() found"},
		{name: "parenthesized recover", source: "package fixture\nfunc f() { ((recover))() }\n", want: "unsafe recover() found"},
		{name: "shadowed package recover", source: "package fixture\nfunc recover() {}\nfunc f() { recover() }\n"},
		{name: "shadowed local recover", source: "package fixture\nfunc f() { recover := func() {}; (recover)() }\n"},
		{name: "shadowed parameter recover", source: "package fixture\nfunc f(recover func()) { recover() }\n"},
		{name: "builtin and shadowed recover", source: "package fixture\nfunc f() { recover(); { recover := func() {}; recover() } }\n", want: "unsafe recover() found", wantCount: 1},
		{name: "generic recover", source: "package fixture\nfunc recover[T any]() {}\nfunc f() { recover[int]() }\n"},
		{name: "selector recover", source: "package fixture\ntype T struct{}\nfunc (T) recover() {}\nfunc f(t T) { t.recover() }\n"},
		{name: "recover in initializer", source: "package fixture\nvar x = (recover)()\n", want: "unsafe recover() found"},
		{name: "recover in initializer closure", source: "package fixture\nvar x = func() any { return recover() }()\n", want: "unsafe recover() found"},
		{name: "recover in value method", source: "package fixture\ntype T struct{}\nfunc (T) f() { recover() }\n", want: "unsafe recover() found"},
		{name: "recover in pointer method", source: "package fixture\ntype T struct{}\nfunc (*T) f() { recover() }\n", want: "unsafe recover() found"},
		{name: "approved recover", module: "github.com/matrixorigin/matrixone/pkg/fileservice", source: "package fixture\nfunc catch() { recover() }\n"},
		{name: "approved value alias receiver", module: "github.com/matrixorigin/matrixone/pkg/common/morpc", source: "package fixture\ntype remoteBackend struct{}\ntype Alias = remoteBackend\nfunc (Alias) writeLoop() { recover() }\n"},
		{name: "approved pointer alias receiver", module: "github.com/matrixorigin/matrixone/pkg/common/morpc", source: "package fixture\ntype remoteBackend struct{}\ntype Alias = remoteBackend\nfunc (*Alias) writeLoop() { recover() }\n"},
		{name: "approved alias of pointer receiver", module: "github.com/matrixorigin/matrixone/pkg/common/morpc", source: "package fixture\ntype remoteBackend struct{}\ntype Alias = *remoteBackend\nfunc (Alias) writeLoop() { recover() }\n"},
		{name: "approved generic receiver", module: "github.com/matrixorigin/matrixone/pkg/common/morpc", source: "package fixture\ntype remoteBackend[T any] struct{}\nfunc (remoteBackend[T]) writeLoop() { recover() }\n"},
		{name: "unapproved alias receiver", module: "github.com/matrixorigin/matrixone/pkg/common/morpc", source: "package fixture\ntype remoteBackend struct{}\ntype Alias = remoteBackend\nfunc (Alias) other() { recover() }\n", want: "unsafe recover() found"},
		{name: "alias named like approved receiver", module: "github.com/matrixorigin/matrixone/pkg/common/morpc", source: "package fixture\ntype unrelated struct{}\ntype remoteBackend = unrelated\nfunc (*remoteBackend) writeLoop() { recover() }\n", want: "unsafe recover() found"},
		{name: "recover in test file", file: "fixture_test.go", source: "package fixture\nfunc f() { recover() }\n"},
		{name: "logging", source: "package fixture\nimport \"fmt\"\nfunc f() { fmt.Println(1) }\n", want: "uncatch log function"},
		{name: "clean cgo", source: "package fixture\nimport \"C\"\n"},
		{name: "authored unsafe in cgo", source: "package fixture\nimport \"C\"\nimport _ `unsafe`\n", want: "importing the unsafe package", wantCount: 1},
		{name: "allowed cgo import", module: "github.com/matrixorigin/matrixone/pkg/common/docfilter", file: "cbitmap.go", source: "package fixture\nimport \"C\"\nimport _ \"unsafe\"\n"},
		{name: "generated source still checked", source: "// Code generated by fixture. DO NOT EDIT.\npackage fixture\nimport _ \"unsafe\"\n", want: "importing the unsafe package"},
		{name: "line directive cannot grant exemption", module: "github.com/matrixorigin/matrixone/pkg/common/util", source: "package fixture\n//line unsafe.go:3\nimport _ \"unsafe\"\n", want: "importing the unsafe package"},
		{name: "approved parser control boundary", module: "github.com/matrixorigin/matrixone/pkg/sql/parsers/dialect/mysql", source: "package fixture\nfunc ParseWithSQLMode() { defer func(){ if p:=recover(); p!=nil {panic(p)} }() }\n"},
		{name: "adjacent parser recover rejected", module: "github.com/matrixorigin/matrixone/pkg/sql/parsers/dialect/mysql", source: "package fixture\nfunc ParseOther() { recover() }\n", want: "unsafe recover() found"},
		{name: "approved layout file", module: "github.com/matrixorigin/matrixone/pkg/vm", file: "continuation.go", source: "package fixture\nimport \"unsafe\"\nconst size=unsafe.Sizeof(1)\n"},
		{name: "adjacent layout file rejected", module: "github.com/matrixorigin/matrixone/pkg/vm", file: "unapproved.go", source: "package fixture\nimport \"unsafe\"\nconst size=unsafe.Sizeof(1)\n", want: "importing the unsafe package"},
		{name: "approved regexp debug", module: "github.com/matrixorigin/matrixone/pkg/sql/plan/function/internal/regexp2", source: "package fixture\nimport \"fmt\"\ntype runner struct{}\nfunc (*runner) scan(){fmt.Printf(\"debug\")}\n"},
		{name: "adjacent regexp method rejected", module: "github.com/matrixorigin/matrixone/pkg/sql/plan/function/internal/regexp2", source: "package fixture\nimport \"fmt\"\ntype runner struct{}\nfunc (*runner) other(){fmt.Printf(\"debug\")}\n", want: "uncatch log function"},
		{name: "regexp lookalike rejected", module: "github.com/matrixorigin/matrixone/pkg/sql/plan/function/internal/regexp2_other", source: "package fixture\nimport \"fmt\"\ntype runner struct{}\nfunc (*runner) scan(){fmt.Printf(\"debug\")}\n", want: "uncatch log function"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			module, file := tc.module, tc.file
			if module == "" {
				module = "example.org/molint-fixture"
			}
			if file == "" {
				file = "fixture.go"
			}
			writeFixture(t, dir, map[string]string{"go.mod": "module " + module + "\ngo 1.24\n", file: tc.source})
			for attempt := 0; attempt < 2; attempt++ {
				code, out := runTool(t, dir, tool, "check", "./...")
				wantCode := 0
				if tc.want != "" {
					wantCode = 1
				}
				if code != wantCode || !strings.Contains(out, tc.want) || strings.Contains(out, "panic:") {
					t.Fatalf("attempt %d: status=%d, want=%d %q\n%s", attempt, code, wantCode, tc.want, out)
				}
				if tc.wantCount > 0 && strings.Count(out, tc.want) != tc.wantCount {
					t.Fatalf("expected %d occurrences of %q:\n%s", tc.wantCount, tc.want, out)
				}
			}
			if tc.name == "blocked import" || tc.name == "recover" {
				code, out := runTool(t, dir, tool, "check", "-json", "./...")
				var r results
				if code != 1 || json.Unmarshal([]byte(out), &r) != nil || r.Error != "" || len(r.Diagnostics) == 0 || !strings.Contains(out, tc.want) {
					t.Fatalf("JSON must preserve findings and failure status: %d %s", code, out)
				}
			}
		})
	}
	t.Run("module ownership changes with warm cache", func(t *testing.T) {
		dir := t.TempDir()
		writeFixture(t, dir, map[string]string{"fixture.go": "package fixture\nimport _ \"example.net/other\"\n", "vendor/example.net/other/p.go": "package other\n"})
		for _, module := range []string{"example.org/good", "go.etcd.io/etcd", "example.org/good"} {
			writeFixture(t, dir, map[string]string{
				"go.mod":             "module example.org/cache-fixture\ngo 1.24\nrequire " + module + " v1.0.0\n",
				"vendor/modules.txt": "# " + module + " v1.0.0\n## explicit; go 1.24\nexample.net/other\n",
			})
			code, out := runTool(t, dir, tool, "check", "-mod=vendor", "./...")
			want := 0
			if module == "go.etcd.io/etcd" {
				want = 1
			}
			if code != want || (want == 1 && !strings.Contains(out, "do not use packages in module go.etcd.io/etcd")) {
				t.Fatalf("module %s status=%d want=%d: %s", module, code, want, out)
			}
		}
		for attempt := 0; attempt < 2; attempt++ {
			code, out := runTool(t, dir, goCommand, "vet", "-mod=vendor", "-vettool="+tool, "./...")
			if code == 0 || !strings.Contains(out, "molint check") {
				t.Fatalf("old entry passed: %d %s", code, out)
			}
		}
	})
	t.Run("literal ellipsis import", func(t *testing.T) {
		dir := t.TempDir()
		writeFixture(t, dir, map[string]string{
			"go.mod":                        "module example.org/ellipsis-fixture\ngo 1.24\nrequire (\nexample.org/good v1.0.0\ngo.etcd.io/etcd v1.0.0\n)\n",
			"fixture.go":                    "package fixture\nimport _ \"example.org/a...b\"\n",
			"vendor/modules.txt":            "# example.org/good v1.0.0\n## explicit; go 1.24\nexample.org/a---b\n# go.etcd.io/etcd v1.0.0\n## explicit; go 1.24\nexample.org/a...b\n",
			"vendor/example.org/a---b/p.go": "package p\n",
			"vendor/example.org/a...b/p.go": "package p\n",
		})
		if code, out := runTool(t, dir, tool, "check", "-mod=vendor", "./..."); code != 1 || !strings.Contains(out, "module go.etcd.io/etcd") {
			t.Fatalf("literal import: %d %s", code, out)
		}
	})
	t.Run("source selection and overlays", func(t *testing.T) { testSourceSelection(t, tool) })
	t.Run("readonly replacements and actual Go driver", func(t *testing.T) { testModuleSelection(t, tool) })
	t.Run("named files and empty selection", func(t *testing.T) {
		dir := t.TempDir()
		writeFixture(t, dir, map[string]string{"go.mod": "module example.org/files-fixture\ngo 1.24\n", "fixture.go": "package fixture\nimport _ \"io/ioutil\"\n", "missing/.keep": ""})
		if code, out := runTool(t, dir, tool, "check", "fixture.go"); code != 1 || !strings.Contains(out, "io/ioutil") {
			t.Fatalf("named file: %d %s", code, out)
		}
		if code, out := runTool(t, dir, tool, "check", "./missing/..."); code != 2 || !strings.Contains(out, "no packages selected") {
			t.Fatalf("empty selection: %d %s", code, out)
		}
	})
	t.Run("GOPATH package ownership", func(t *testing.T) {
		gopath := t.TempDir()
		dir := filepath.Join(gopath, "src", "example.org", "fixture")
		writeFixture(t, dir, map[string]string{"fixture.go": "package fixture\nimport _ \"fmt\"\n"})
		t.Setenv("GOPATH", gopath)
		t.Setenv("GO111MODULE", "off")
		if code, out := runTool(t, dir, tool, "check", "."); code != 0 {
			t.Fatalf("GOPATH: %d %s", code, out)
		}
	})
	t.Run("unsupported source generation", func(t *testing.T) {
		dir := t.TempDir()
		writeFixture(t, dir, map[string]string{"go.mod": "module example.org/swig-fixture\ngo 1.24\n", "fixture.go": "package fixture\n", "fixture.swig": "%module fixture\n"})
		if code, out := runTool(t, dir, tool, "check", "."); code != 2 || !strings.Contains(out, "SWIG") {
			t.Fatalf("SWIG: %d %s", code, out)
		}
	})
	t.Run("infrastructure failure and recovery", func(t *testing.T) {
		dir := t.TempDir()
		writeFixture(t, dir, map[string]string{"go.mod": "module example.org/error-fixture\ngo 1.24\n", "fixture.go": "package fixture\nfunc f() int { return \"wrong type\" }\n"})
		for attempt := 0; attempt < 2; attempt++ {
			code, out := runTool(t, dir, tool, "check", "-json", "./...")
			var r results
			if code != 2 || json.Unmarshal([]byte(out), &r) != nil || !strings.Contains(r.Error, "wrong type") {
				t.Fatalf("type failure: %d %s", code, out)
			}
		}
		writeFixture(t, dir, map[string]string{"fixture.go": "package fixture\nfunc f() int { return 1 }\n"})
		if code, out := runTool(t, dir, tool, "check", "./..."); code != 0 {
			t.Fatalf("recovery: %d %s", code, out)
		}
	})
}

func TestReadVetOutput(t *testing.T) {
	for _, data := range []string{`null`, `{`, `{"p":null}`, `{"p":{"checkrecover":null}}`, `{"p":{"checkrecover":{"error":"failed"}}}`, `{"p":{"checkrecover":[{}]}}`, "unexpected output"} {
		r := results{Diagnostics: make(map[string]map[string][]diagnostic)}
		if _, err := r.readVetOutput([]byte(data), &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
}
