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
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type options struct {
	dir, overlay         string
	buildFlags, packages []string
	analyzers            map[string]bool
	json                 bool
	moduleMode           bool
	childGOFLAGS         *string
}

// Build flags are forwarded, not interpreted. Go remains responsible for their
// validation and effect. Preserve ordered CLI occurrences and leave GOFLAGS
// in the environment: Go distinguishes those inputs (for example in GOPATH).
type buildFlag struct {
	value   string
	name    string
	flags   *[]string
	boolean bool
}

func (f *buildFlag) String() string { return f.value }
func (f *buildFlag) Set(s string) error {
	f.value = s
	*f.flags = append(*f.flags, "-"+f.name+"="+s)
	return nil
}
func (f *buildFlag) IsBoolFlag() bool { return f.boolean }

func parseOptions(ctx context.Context, args []string, stderr io.Writer) (options, error) {
	opts := options{analyzers: make(map[string]bool)}
	// Like Go, -C is an entry option, before all other flags and packages.
	if len(args) > 0 && (args[0] == "-C" || strings.HasPrefix(args[0], "-C=")) {
		value := strings.TrimPrefix(args[0], "-C=")
		if args[0] == "-C" {
			if len(args) < 2 {
				return opts, errors.New("-C requires a directory")
			}
			value, args = args[1], args[1:]
		}
		opts.dir, args = value, args[1:]
	}
	fs := flag.NewFlagSet("molint check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.BoolVar(&opts.json, "json", false, "write JSON results (findings still exit 1)")
	selected := make(map[string]*bool)
	for _, name := range []string{"checkunsafe", "pkgblocklist", "checkrecover", "checklog"} {
		selected[name] = fs.Bool(name, true, "enable "+name)
	}
	build := make(map[string]*buildFlag)
	for _, name := range strings.Fields("a race msan asan trimpath modcacherw cover buildvcs") {
		build[name] = &buildFlag{boolean: true}
	}
	for _, name := range strings.Fields("p tags mod modfile overlay compiler buildmode asmflags gcflags ldflags gccgoflags pkgdir toolexec installsuffix covermode coverpkg pgo") {
		build[name] = &buildFlag{}
	}
	for name, value := range build {
		value.name, value.flags = name, &opts.buildFlags
		fs.Var(value, name, "Go build flag")
	}
	// Establish the caller's output mode before filesystem or Go preflight.
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	opts.packages = fs.Args()
	if len(opts.packages) == 0 {
		opts.packages = []string{"."}
	}
	for _, pkg := range opts.packages {
		if strings.HasPrefix(pkg, "-") {
			return opts, errors.New("flags must precede package arguments")
		}
	}
	var err error
	if opts.dir == "" {
		opts.dir, err = os.Getwd()
	} else {
		opts.dir, err = filepath.Abs(opts.dir)
	}
	if err != nil {
		return opts, err
	}
	cmd := goCommand(ctx, opts, "env", "-json", "GOFLAGS", "GOMOD")
	envJSON, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return opts, fmt.Errorf("go env: %w\n%s", err, exit.Stderr)
		}
		return opts, fmt.Errorf("go env: %w", err)
	}
	var goEnv struct{ GOFLAGS, GOMOD string }
	if err := json.Unmarshal(envJSON, &goEnv); err != nil {
		return opts, fmt.Errorf("go env: %w", err)
	}
	opts.moduleMode = goEnv.GOMOD != ""
	envFlags, rawFlags, err := splitGOFLAGS(goEnv.GOFLAGS)
	if err != nil {
		return opts, err
	}
	// Parse each input once. Environment values are defaults: restore explicit
	// CLI values afterward, and forward the original ordered CLI build flags.
	cliValues := make(map[string]string)
	fs.Visit(func(f *flag.Flag) { cliValues[f.Name] = f.Value.String() })
	cliBuildFlags := opts.buildFlags
	opts.buildFlags = nil
	err = fs.Parse(envFlags)
	for name, value := range cliValues {
		_ = fs.Set(name, value) // This value already passed the same parser.
	}
	opts.buildFlags = cliBuildFlags
	if err != nil {
		return opts, err
	}
	if len(fs.Args()) != 0 {
		return opts, errors.New("GOFLAGS must contain flags, not package arguments")
	}
	// Analyzer selection belongs to molint, not to go list or its internal
	// typed vet. Preserve the original spelling of every Go-owned flag.
	var goFlags []string
	removed := false
	for i, value := range envFlags {
		name, _, _ := strings.Cut(strings.TrimLeft(value, "-"), "=")
		if _, ok := selected[name]; ok {
			removed = true
			continue
		}
		goFlags = append(goFlags, rawFlags[i])
	}
	if removed {
		value := strings.Join(goFlags, " ")
		if value == "" {
			// Go treats an empty environment value as unset and can fall back
			// to a persisted GOFLAGS in its user environment file.
			value = " "
		}
		opts.childGOFLAGS = &value
	}
	anyEnabled := false
	fs.Visit(func(f *flag.Flag) {
		if value, ok := selected[f.Name]; ok && *value {
			anyEnabled = true
		}
	})
	for name, value := range selected {
		opts.analyzers[name] = *value && !anyEnabled
	}
	fs.Visit(func(f *flag.Flag) {
		if value, ok := selected[f.Name]; ok {
			opts.analyzers[f.Name] = *value
		}
	})
	opts.overlay = build["overlay"].value
	return opts, nil
}

// GOFLAGS uses Go's quoted-word convention, not shell expansion: only a quote
// at the beginning of a word is special, with no escapes inside that word.
func splitGOFLAGS(value string) ([]string, []string, error) {
	var fields []string
	var raw []string
	for value = strings.TrimLeft(value, " \t\r\n"); value != ""; value = strings.TrimLeft(value, " \t\r\n") {
		if value[0] == '\'' || value[0] == '"' {
			end := strings.IndexByte(value[1:], value[0])
			if end < 0 {
				return nil, nil, errors.New("unterminated quote in GOFLAGS")
			}
			fields = append(fields, value[1:1+end])
			raw = append(raw, value[:end+2])
			value = value[end+2:]
		} else {
			end := strings.IndexAny(value, " \t\r\n")
			if end < 0 {
				end = len(value)
			}
			fields = append(fields, value[:end])
			raw = append(raw, value[:end])
			value = value[end:]
		}
	}
	return fields, raw, nil
}

func (o options) readOverlay() (map[string]string, error) {
	if o.overlay == "" {
		return nil, nil
	}
	abs := func(p string) string {
		if filepath.IsAbs(p) {
			return filepath.Clean(p)
		}
		return filepath.Join(o.dir, p)
	}
	data, err := os.ReadFile(abs(o.overlay))
	if err != nil {
		return nil, err
	}
	var overlay struct{ Replace map[string]string }
	if err := json.Unmarshal(data, &overlay); err != nil {
		return nil, fmt.Errorf("overlay: %w", err)
	}
	files := make(map[string]string, len(overlay.Replace))
	for source, target := range overlay.Replace {
		if target != "" {
			target = abs(target)
		}
		files[abs(source)] = target
	}
	return files, nil
}
