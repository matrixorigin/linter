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

// Exercise the actual go vet protocol: unit tests of analyzer diagnostics alone
// cannot detect a mismatch between Go's vet driver and unitchecker output.
func TestVetToolReportsFindings(t *testing.T) {
	goCommand := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goCommand += ".exe"
	}
	t.Setenv("PATH", filepath.Join(runtime.GOROOT(), "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOPACKAGESDRIVER", "off")

	tool := filepath.Join(t.TempDir(), "molint")
	if runtime.GOOS == "windows" {
		tool += ".exe"
	}
	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildCtx, goCommand, "build", "-o", tool, ".")
	build.WaitDelay = 5 * time.Second
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build molint: %v\n%s", err, out)
	}

	tests := []struct {
		name, source, want string
		failDriver         bool
	}{
		{name: "clean", source: "package fixture\n"},
		{name: "blocked import", source: "package fixture\nimport _ \"io/ioutil\"\n", want: "do not use package io/ioutil"},
		{name: "unsafe import", source: "package fixture\nimport _ \"unsafe\"\n", want: "importing the unsafe package is not allowed"},
		{name: "recover", source: "package fixture\nfunc f() { recover() }\n", want: "unsafe recover() found"},
		{name: "logging", source: "package fixture\nimport \"fmt\"\nfunc f() { fmt.Println(1) }\n", want: "uncatch log function"},
		{name: "loader failure", source: "package fixture\nimport _ \"fmt\"\n", want: "injected loader failure", failDriver: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.failDriver && runtime.GOOS == "windows" {
				t.Skip("fixture driver is a shell script")
			}
			dir := t.TempDir()
			for name, content := range map[string]string{
				"go.mod":     "module example.org/molint-fixture\n\ngo 1.24\n",
				"fixture.go": tc.source,
			} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.failDriver {
				driver := filepath.Join(dir, "driver")
				if err := os.WriteFile(driver, []byte("#!/bin/sh\necho injected loader failure >&2\nexit 1\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				t.Setenv("GOPACKAGESDRIVER", driver)
			}
			for attempt := 0; attempt < 2; attempt++ {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				cmd := exec.CommandContext(ctx, goCommand, "vet", "-vettool="+tool, "./...")
				cmd.WaitDelay = 5 * time.Second
				cmd.Dir = dir
				out, err := cmd.CombinedOutput()
				contextErr := ctx.Err()
				cancel()
				if contextErr != nil {
					t.Fatalf("vet timed out: %v\n%s", contextErr, out)
				}
				if tc.want == "" && !tc.failDriver {
					if err != nil {
						t.Fatalf("clean fixture failed: %v\n%s", err, out)
					}
					continue
				}
				if err == nil {
					t.Fatalf("expected vet failure, got success:\n%s", out)
				}
				if !strings.Contains(string(out), tc.want) {
					t.Fatalf("missing diagnostic %q:\n%s", tc.want, out)
				}
			}
			if tc.name == "blocked import" {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				cmd := exec.CommandContext(ctx, goCommand, "vet", "-json", "-vettool="+tool, "./...")
				cmd.WaitDelay = 5 * time.Second
				cmd.Dir = dir
				out, err := cmd.CombinedOutput()
				contextErr := ctx.Err()
				cancel()
				if contextErr != nil || err != nil {
					t.Fatalf("JSON diagnostics: context=%v err=%v\n%s", contextErr, err, out)
				}
				start := bytes.IndexByte(out, '{') // Older Go versions prefix the package name.
				var diagnostics map[string]map[string][]struct {
					Message string `json:"message"`
				}
				if start < 0 || json.Unmarshal(out[start:], &diagnostics) != nil {
					t.Fatalf("invalid JSON diagnostics:\n%s", out)
				}
				entries := diagnostics["example.org/molint-fixture"]["pkgblocklist"]
				if len(entries) != 1 || !strings.Contains(entries[0].Message, tc.want) {
					t.Fatalf("unexpected pkgblocklist diagnostics: %+v", entries)
				}
			}
		})
	}
}
