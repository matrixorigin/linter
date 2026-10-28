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
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/matrixorigin/linter/checkunsafe"
	"github.com/matrixorigin/linter/pkgblocklist"
)

type diagnostic struct {
	Posn    string `json:"posn"`
	Message string `json:"message"`
}

type resultKey struct {
	pkg, analyzer string
	diagnostic
}

type results struct {
	seen        map[resultKey]struct{}
	Diagnostics map[string]map[string][]diagnostic `json:"diagnostics"`
	Error       string                             `json:"error,omitempty"`
}

func (r *results) add(pkg, analyzer string, d diagnostic) {
	if r.Diagnostics[pkg] == nil {
		r.Diagnostics[pkg] = make(map[string][]diagnostic)
	}
	// Keep duplicate test-variant imports linear even in packages with many
	// findings. This index has only the lifetime of the output being assembled.
	if r.seen == nil {
		r.seen = make(map[resultKey]struct{})
	}
	key := resultKey{pkg, analyzer, d}
	if _, ok := r.seen[key]; ok {
		return
	}
	r.seen[key] = struct{}{}
	r.Diagnostics[pkg][analyzer] = append(r.Diagnostics[pkg][analyzer], d)
}

func check(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(ctx, args, stderr)
	r := results{Diagnostics: make(map[string]map[string][]diagnostic)}
	if err == nil {
		err = runChecks(ctx, opts, &r, stderr)
	}
	if err != nil {
		r.Error = err.Error()
	}
	if opts.json {
		err = json.NewEncoder(stdout).Encode(r)
	} else {
		var lines []string
		for _, analyzers := range r.Diagnostics {
			for analyzer, ds := range analyzers {
				for _, d := range ds {
					lines = append(lines, fmt.Sprintf("%s: %s: %s\n", d.Posn, analyzer, d.Message))
				}
			}
		}
		sort.Strings(lines)
		_, err = io.WriteString(stdout, strings.Join(lines, ""))
		if r.Error != "" {
			fmt.Fprintln(stderr, "molint:", r.Error)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "molint: write results:", err)
		return 2
	}
	if r.Error != "" {
		return 2
	}
	if len(r.Diagnostics) != 0 {
		return 1
	}
	return 0
}

// Go owns package selection and module identity. Keep only the metadata needed
// for import policy; never load dependency syntax or types in this process.
type listedPackage struct {
	Dir, ImportPath, ForTest string
	Match, GoFiles, CgoFiles []string
	SwigFiles, SwigCXXFiles  []string
	ImportMap                map[string]string
	Standard                 bool
	Module                   *struct{ Path string }
	Error                    *struct{ Err string }
	DepsErrors               []struct{ Err string }
}

func goCommand(ctx context.Context, opts options, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = opts.dir
	// Environ supplies PWD for Dir, including a logical symlink used by -C.
	// The source overlay and Go must resolve relative paths from that same dir.
	cmd.Env = append(cmd.Environ(), "MOLINT_INTERNAL_VET=1")
	cmd.WaitDelay = 5 * time.Second
	ownCommand(cmd)
	return cmd
}

func loadGraph(ctx context.Context, opts options) (map[string]*listedPackage, error) {
	args := append([]string{"list", "-deps", "-json", "-test"}, opts.buildFlags...)
	cmd := goCommand(ctx, opts, append(args, opts.packages...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	graph := make(map[string]*listedPackage)
	decoder := json.NewDecoder(out)
	for {
		var pkg listedPackage
		err = decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			err = nil
			break
		}
		if err != nil {
			_ = cmd.Cancel()
			break
		}
		if pkg.ImportPath == "" || graph[pkg.ImportPath] != nil {
			err = fmt.Errorf("missing or duplicate package identity %q", pkg.ImportPath)
			_ = cmd.Cancel()
			break
		}
		graph[pkg.ImportPath] = &pkg
	}
	waitErr := cmd.Wait()
	if err != nil || waitErr != nil {
		return nil, fmt.Errorf("go list: %w\n%s", errors.Join(err, waitErr), &stderr)
	}
	for _, pkg := range graph {
		if pkg.Error != nil {
			return nil, fmt.Errorf("go list %s: %s", pkg.ImportPath, pkg.Error.Err)
		}
		if len(pkg.DepsErrors) != 0 {
			return nil, fmt.Errorf("go list %s: %s", pkg.ImportPath, pkg.DepsErrors[0].Err)
		}
	}
	return graph, nil
}

func runChecks(ctx context.Context, opts options, r *results, stderr io.Writer) error {
	graph, err := loadGraph(ctx, opts)
	if err != nil {
		return err
	}
	roots := make(map[string]bool)
	for id, pkg := range graph {
		if len(pkg.Match) != 0 && pkg.ForTest == "" {
			roots[id] = true
		}
	}
	if len(roots) == 0 {
		return errors.New("no packages selected")
	}
	overlay, err := opts.readOverlay()
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	parsed := make(map[string]*ast.File)
	for id, pkg := range graph {
		pkgPath, _, _ := strings.Cut(id, " [")
		// ForTest also marks dependencies recompiled for a test. Only check
		// the selected root itself and its external test package.
		selectedTest := roots[pkg.ForTest] && (pkgPath == pkg.ForTest || pkgPath == pkg.ForTest+"_test")
		if (!roots[id] && !selectedTest) || (!opts.analyzers["checkunsafe"] && !opts.analyzers["pkgblocklist"]) {
			continue
		}
		if len(pkg.SwigFiles)+len(pkg.SwigCXXFiles) != 0 {
			return fmt.Errorf("%s: SWIG-generated source is not supported by molint check", id)
		}
		for _, group := range []struct {
			files []string
			cgo   bool
		}{{pkg.GoFiles, false}, {pkg.CgoFiles, true}} {
			for _, name := range group.files {
				if err := ctx.Err(); err != nil {
					return err
				}
				logical := name
				if !filepath.IsAbs(logical) {
					logical = filepath.Join(pkg.Dir, logical)
				}
				physical := logical
				if replacement, ok := overlay[logical]; ok {
					if replacement == "" {
						return fmt.Errorf("selected source %s is deleted by the overlay", logical)
					}
					physical = replacement
				}
				file := parsed[logical]
				if file == nil {
					data, err := os.ReadFile(physical)
					if err != nil {
						return err
					}
					file, err = parser.ParseFile(fset, logical, data, parser.ImportsOnly|parser.ParseComments)
					if err != nil {
						return err
					}
					parsed[logical] = file
				}
				for _, imp := range file.Imports {
					importPath, err := strconv.Unquote(imp.Path.Value)
					if err != nil {
						return err
					}
					if importPath == "C" {
						continue
					}
					pos := fset.Position(imp.Pos()).String()
					if opts.analyzers["checkunsafe"] && importPath == "unsafe" {
						if msg := checkunsafe.Message(pkgPath, filepath.Base(logical), group.cgo); msg != "" {
							r.add(pkgPath, "checkunsafe", diagnostic{pos, msg})
						}
					}
					if !opts.analyzers["pkgblocklist"] {
						continue
					}
					resolved := importPath
					if mapped, ok := pkg.ImportMap[importPath]; ok {
						resolved = mapped
					}
					dep := graph[resolved]
					if dep == nil {
						return fmt.Errorf("%s: no Go package metadata for import %q (%s)", id, importPath, resolved)
					}
					module := ""
					if dep.Module != nil {
						module = dep.Module.Path
						if module == "" {
							return fmt.Errorf("%s: empty module owner for %s", id, resolved)
						}
					} else if !dep.Standard && opts.moduleMode {
						return fmt.Errorf("%s: missing module owner for %s", id, resolved)
					}
					resolvedPath, _, _ := strings.Cut(resolved, " [")
					if msg := pkgblocklist.Message(resolvedPath, module); msg != "" {
						r.add(pkgPath, "pkgblocklist", diagnostic{pos, msg})
					}
				}
			}
		}
	}

	tool, err := os.Executable()
	if err != nil {
		return err
	}
	args := append([]string{"vet", "-json", "-vettool=" + tool}, opts.buildFlags...)
	for _, name := range []string{"checkrecover", "checklog"} {
		args = append(args, "-"+name+"="+strconv.FormatBool(opts.analyzers[name]))
	}
	cmd := goCommand(ctx, opts, append(args, opts.packages...)...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go vet: %w\n%s%s", err, &out, &errOut)
	}
	// Go 1.24 writes JSON to stderr with package headers; Go 1.26+ writes
	// its cached JSON to stdout. Consume both, never rely on JSON-mode status.
	seen := false
	for _, stream := range [][]byte{out.Bytes(), errOut.Bytes()} {
		found, err := r.readVetOutput(stream, stderr)
		if err != nil {
			return err
		}
		seen = seen || found
	}
	if !seen {
		return errors.New("go vet returned no JSON results")
	}
	return ctx.Err()
}

func (r *results) readVetOutput(data []byte, stderr io.Writer) (bool, error) {
	seen := false
	for len(bytes.TrimSpace(data)) != 0 {
		data = bytes.TrimSpace(data)
		if bytes.HasPrefix(data, []byte("# ")) || bytes.HasPrefix(data, []byte("go: ")) {
			line, rest, _ := bytes.Cut(data, []byte("\n"))
			if bytes.HasPrefix(line, []byte("go: ")) {
				fmt.Fprintln(stderr, string(line))
			}
			data = rest
			continue
		}
		var pkgs map[string]map[string]json.RawMessage
		d := json.NewDecoder(bytes.NewReader(data))
		if err := d.Decode(&pkgs); err != nil || pkgs == nil {
			return seen, fmt.Errorf("invalid go vet JSON: %v", err)
		}
		seen = true
		data = data[d.InputOffset():]
		for pkg, analyzers := range pkgs {
			if pkg == "" || analyzers == nil {
				return seen, errors.New("invalid package in go vet JSON")
			}
			for name, raw := range analyzers {
				var ds []diagnostic
				if err := json.Unmarshal(raw, &ds); err != nil || ds == nil {
					return seen, fmt.Errorf("go vet %s/%s: %s", pkg, name, raw)
				}
				for _, diag := range ds {
					if diag.Message == "" {
						return seen, fmt.Errorf("go vet %s/%s: missing diagnostic message", pkg, name)
					}
					r.add(pkg, name, diag)
				}
			}
		}
	}
	return seen, nil
}
