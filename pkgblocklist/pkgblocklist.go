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

// Message checks the identity reported by the Go package graph. Module
// ownership is deliberately checked by the command, outside go vet's cache.
// A replacement changes the code location, not the logical module identity.
func Message(importPath, modulePath string) string {
	switch {
	case importPath == "io/ioutil":
		return "do not use package io/ioutil, use os instead"
	case modulePath == "go.etcd.io/etcd":
		return "do not use packages in module go.etcd.io/etcd, use go.etcd.io/etcd/v3 instead"
	default:
		return ""
	}
}
