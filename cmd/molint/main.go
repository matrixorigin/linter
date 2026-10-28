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

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/matrixorigin/linter/checklog"
	"github.com/matrixorigin/linter/checkrecover"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/unitchecker"
)

// An analyzer error in unitchecker's JSON mode exits successfully. Go vet can
// cache the facts before processing that error, making a retry falsely pass.
// Fail the vettool process at the command boundary so failed runs are retried.
func failOnError(analyzers ...*analysis.Analyzer) {
	seen := make(map[*analysis.Analyzer]bool)
	var wrap func(*analysis.Analyzer)
	wrap = func(a *analysis.Analyzer) {
		if seen[a] {
			return
		}
		seen[a] = true
		for _, required := range a.Requires {
			wrap(required)
		}
		run := a.Run
		a.Run = func(pass *analysis.Pass) (any, error) {
			result, err := run(pass)
			if err != nil {
				fmt.Fprintf(os.Stderr, "molint: %s: %v\n", a.Name, err)
				os.Exit(1)
			}
			return result, nil
		}
	}
	for _, a := range analyzers {
		wrap(a)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "check" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		code := check(ctx, os.Args[2:], os.Stdout, os.Stderr)
		stop()
		os.Exit(code)
	}
	// Go asks for the tool identity before consulting its vet cache. Reject
	// the old entry there as well as at execution: it cannot run import checks.
	if os.Getenv("MOLINT_INTERNAL_VET") != "1" && !(len(os.Args) == 2 && os.Args[1] == "-flags") {
		fmt.Fprintln(os.Stderr, "molint: use 'molint check [build flags] [packages]' instead of go vet -vettool=molint")
		os.Exit(2)
	}
	failOnError(checkrecover.Analyzer, checklog.Analyzer)
	unitchecker.Main(
		checkrecover.Analyzer,
		checklog.Analyzer,
	)
}
