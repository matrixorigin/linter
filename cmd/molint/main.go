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
	"github.com/matrixorigin/linter/checklog"
	"github.com/matrixorigin/linter/checkrecover"
	"github.com/matrixorigin/linter/checkunsafe"
	"github.com/matrixorigin/linter/pkgblocklist"
	"golang.org/x/tools/go/analysis/unitchecker"
)

func main() {
	unitchecker.Main(
		checkrecover.Analyzer,
		pkgblocklist.Analyzer,
		checklog.Analyzer,
		checkunsafe.Analyzer,
	)
}
