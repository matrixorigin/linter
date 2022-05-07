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

package pkgblocklist

import (
	"fmt"
	"sync"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

const doc = "Block unwanted packages"

var blockList = []BlockSpec{
	{
		MatchModule: "go.etcd.io/etcd",
		UseInstead:  "go.etcd.io/etcd/v3",
	},
}

type BlockSpec struct {
	MatchModule string
	MatchPath   string
	UseInstead  string
}

var Analyzer = &analysis.Analyzer{
	Name: "pkgblocklist",
	Doc:  doc,
	Run:  run,
}

var packagesPackages sync.Map // import path -> *packages.Package

func getPackagesPackage(importPath string) *packages.Package {
	v, ok := packagesPackages.Load(importPath)
	if ok {
		return v.(*packages.Package)
	}
	cfg := &packages.Config{
		Mode: packages.NeedModule,
	}
	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		panic(err)
	}
	pkg := pkgs[0]
	packagesPackages.Store(importPath, pkg)
	return pkg
}

func run(pass *analysis.Pass) (any, error) {

	for _, dep := range pass.Pkg.Imports() {

		path := dep.Path()
		packagesPkg := getPackagesPackage(path)

		for _, spec := range blockList {

			// match import path
			if path == spec.MatchPath {
				message := fmt.Sprintf("do not use package %s", spec.MatchPath)
				if spec.UseInstead != "" {
					message += fmt.Sprintf(", use %s instead", spec.UseInstead)
				}
				pass.Reportf(0, message)
			}

			// match module path
			if packagesPkg.Module != nil {
				modulePath := packagesPkg.Module.Path
				if modulePath == spec.MatchModule {
					message := fmt.Sprintf("do not use packages in module %s", spec.MatchModule)
					if spec.UseInstead != "" {
						message += fmt.Sprintf(", use %s instead", spec.UseInstead)
					}
					pass.Reportf(0, message)
				}
			}

		}
	}

	return nil, nil
}
