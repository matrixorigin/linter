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
	"crypto/sha256"
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
	cmd.Env = cmd.Environ()
	if opts.childGOFLAGS != nil {
		cmd.Env = append(cmd.Env, "GOFLAGS="+*opts.childGOFLAGS)
	}
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

// Go vets the internal test variant instead of the base package, and also
// vets the external test variant. The built-in documentation packages have
// no vet action. Use Go's listed identities for the action cache boundary.
func selectedVetActions(graph map[string]*listedPackage, roots map[string]bool) []string {
	actions := make(map[string]string)
	for id, pkg := range graph {
		if roots[id] && !(pkg.Standard && (id == "unsafe" || id == "builtin")) && len(pkg.GoFiles)+len(pkg.CgoFiles) > 0 {
			actions[id] = id
		}
	}
	for id, pkg := range graph {
		if !roots[pkg.ForTest] || len(pkg.GoFiles)+len(pkg.CgoFiles) == 0 {
			continue
		}
		pkgPath, _, _ := strings.Cut(id, " [")
		switch pkgPath {
		case pkg.ForTest:
			actions[pkg.ForTest] = id
		case pkg.ForTest + "_test":
			actions[id] = id
		}
	}
	selected := make([]string, 0, len(actions))
	for _, id := range actions {
		selected = append(selected, id)
	}
	sort.Strings(selected)
	return selected
}

func runChecks(ctx context.Context, opts options, r *results, stderr io.Writer) error {
	graph, err := loadGraph(ctx, opts)
	if err != nil {
		return err
	}
	roots := make(map[string]bool)
	for id, pkg := range graph {
		// PGO dependency copies inherit Match from their original package but
		// are listed under a qualified identity, not as selected roots.
		if len(pkg.Match) != 0 && pkg.ForTest == "" && !strings.Contains(id, " [") {
			roots[id] = true
		}
	}
	if len(roots) == 0 {
		return errors.New("no packages selected")
	}
	vetActions := selectedVetActions(graph, roots)
	overlay, err := opts.readOverlay()
	if err != nil {
		return err
	}
	// go vet sees overlay backing paths, whereas test status belongs to the
	// logical source. Reject ambiguous physical identities rather than guess.
	var sourceTests map[string]bool
	if len(overlay) != 0 && (opts.analyzers["checklog"] || opts.analyzers["checkrecover"]) {
		sourceTests = make(map[string]bool)
		for _, id := range vetActions {
			pkg := graph[id]
			for _, name := range pkg.GoFiles {
				logical := name
				if !filepath.IsAbs(logical) {
					logical = filepath.Join(pkg.Dir, logical)
				}
				logical = filepath.Clean(logical)
				physical := logical
				if mapped, ok := overlay[logical]; ok && mapped != "" {
					physical = mapped
				}
				test := strings.HasSuffix(logical, "_test.go")
				if previous, ok := sourceTests[physical]; ok && previous != test {
					return fmt.Errorf("overlay backing file %s has ambiguous test status", physical)
				}
				sourceTests[physical] = test
			}
		}
		// Most paths already have the right suffix. Pass only exceptions to
		// vet processes, avoiding a map proportional to the whole package set.
		for physical, test := range sourceTests {
			if strings.HasSuffix(physical, "_test.go") == test {
				delete(sourceTests, physical)
			}
		}
	}
	fset := token.NewFileSet()
	parsed := make(map[string]*ast.File)
	for id, pkg := range graph {
		pkgPath, _, _ := strings.Cut(id, " [")
		// ForTest also marks dependencies recompiled for a test. Only check
		// the selected root itself and its external test package.
		selectedTest := roots[pkg.ForTest] && (pkgPath == pkg.ForTest || pkgPath == pkg.ForTest+"_test")
		if !roots[id] && !selectedTest {
			continue
		}
		if len(pkg.SwigFiles)+len(pkg.SwigCXXFiles) != 0 {
			return fmt.Errorf("%s: SWIG-generated source is not supported by molint check", id)
		}
		if !opts.analyzers["checkunsafe"] && !opts.analyzers["pkgblocklist"] {
			continue
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
	tempRoot, err := filepath.Abs(os.TempDir())
	if err != nil {
		return err
	}
	legacyDir, err := os.MkdirTemp(tempRoot, "molint-vet-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(legacyDir)
	scopeInputs := append([]string{"v2"}, vetActions...)
	if len(overlay) != 0 {
		identity, err := json.Marshal(overlay)
		if err != nil {
			return err
		}
		scopeInputs = append(scopeInputs, string(identity))
	}
	if roots["command-line-arguments"] {
		// The synthetic import path alone does not identify named-file inputs.
		scopeInputs = append(scopeInputs, opts.dir)
		scopeInputs = append(scopeInputs, opts.packages...)
	}
	scope := sha256.Sum256([]byte(strings.Join(scopeInputs, "\x00")))
	args := append([]string{"vet", "-json", "-vettool=" + tool, fmt.Sprintf("-molintscope=%x", scope)}, opts.buildFlags...)
	for _, name := range []string{"checkrecover", "checklog"} {
		args = append(args, "-"+name+"="+strconv.FormatBool(opts.analyzers[name]))
	}
	cmd := goCommand(ctx, opts, append(args, opts.packages...)...)
	cmd.Env = append(cmd.Env, "MOLINT_INTERNAL_VET="+legacyDir)
	if sourceTests != nil {
		data, err := json.Marshal(sourceTests)
		if err != nil {
			return err
		}
		path := filepath.Join(legacyDir, "sources.json")
		if err := os.WriteFile(path, data, 0600); err != nil {
			return err
		}
		cmd.Env = append(cmd.Env, "MOLINT_INTERNAL_SOURCES="+path)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go vet: %w\n%s%s", err, &out, &errOut)
	}
	// Build warnings belong to stderr, which is not a JSON protocol. Go
	// 1.26+ returns cached analyzer JSON on stdout; older Go writes each
	// selected action's JSON into its private file above.
	if _, err := stderr.Write(errOut.Bytes()); err != nil {
		return err
	}
	count, err := r.readVetOutput(out.Bytes(), stderr)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(legacyDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == "sources.json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(legacyDir, entry.Name()))
		if err != nil {
			return err
		}
		if !bytes.HasSuffix(data, []byte("\n")) || !json.Valid(data) {
			return fmt.Errorf("invalid legacy vet output %s", entry.Name())
		}
		found, err := r.readVetOutput(data, stderr)
		if err != nil {
			return err
		}
		if found != 1 {
			return fmt.Errorf("empty legacy vet output %s", entry.Name())
		}
		count += found
	}
	if count != len(vetActions) {
		return fmt.Errorf("go vet returned %d JSON results for %d selected actions", count, len(vetActions))
	}
	return ctx.Err()
}

func (r *results) readVetOutput(data []byte, stderr io.Writer) (int, error) {
	count := 0
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
			return count, fmt.Errorf("invalid go vet JSON: %v", err)
		}
		count++
		data = data[d.InputOffset():]
		for pkg, analyzers := range pkgs {
			if pkg == "" || analyzers == nil {
				return count, errors.New("invalid package in go vet JSON")
			}
			for name, raw := range analyzers {
				var ds []diagnostic
				if err := json.Unmarshal(raw, &ds); err != nil || ds == nil {
					return count, fmt.Errorf("go vet %s/%s: %s", pkg, name, raw)
				}
				for _, diag := range ds {
					if diag.Message == "" {
						return count, fmt.Errorf("go vet %s/%s: missing diagnostic message", pkg, name)
					}
					r.add(pkg, name, diag)
				}
			}
		}
	}
	return count, nil
}
