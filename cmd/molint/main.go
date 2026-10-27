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
	"fmt"
	"os"

	"github.com/matrixorigin/linter/checklog"
	"github.com/matrixorigin/linter/checkrecover"
	"github.com/matrixorigin/linter/checkunsafe"
	"github.com/matrixorigin/linter/pkgblocklist"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/unitchecker"
)

// An analyzer error in unitchecker's JSON mode exits successfully. Go vet can
// cache the facts before processing that error, making a retry falsely pass.
// Fail the vettool process at the command boundary so failed runs are retried.
func failOnError(a *analysis.Analyzer) *analysis.Analyzer {
	wrapped := *a
	wrapped.Run = func(pass *analysis.Pass) (any, error) {
		result, err := a.Run(pass)
		if err != nil {
			fmt.Fprintf(os.Stderr, "molint: %s: %v\n", a.Name, err)
			os.Exit(1)
		}
		return result, nil
	}
	return &wrapped
}

func main() {
	unitchecker.Main(
		failOnError(checkrecover.Analyzer),
		failOnError(pkgblocklist.Analyzer),
		failOnError(checklog.Analyzer),
		failOnError(checkunsafe.Analyzer),
	)
}
