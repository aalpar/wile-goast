// Copyright 2026 Aaron Alpar
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

package goastssa

import "github.com/aalpar/wile/registry"

// ssaExtension wraps ExtensionFunc to implement LibraryNamer.
type ssaExtension struct {
	registry.Extension
}

// LibraryName returns (wile goast ssa) for R7RS import.
func (p *ssaExtension) LibraryName() []string {
	return []string{"wile", "goast", "ssa"}
}

// Extension is the SSA extension entry point.
var Extension registry.Extension = &ssaExtension{
	Extension: registry.NewExtension("goast-ssa", AddToRegistry),
}

// Builder aggregates all SSA registration functions.
var Builder = registry.NewRegistryBuilder(addPrimitives)

// AddToRegistry registers all SSA primitives.
var AddToRegistry = Builder.AddToRegistry

func addPrimitives(r *registry.Registry) error {
	r.AddPrimitives([]registry.PrimitiveSpec{
		{Name: "go-ssa-build", ParamCount: 2, IsVariadic: true, Impl: PrimGoSSABuild,
			Doc:        "Builds SSA for a Go package and returns a list of ssa-func nodes.",
			ParamNames: []string{"pattern", "options"}, Category: "goast-ssa"},
		{Name: "go-ssa-field-index", ParamCount: 1, Impl: PrimGoSSAFieldIndex,
			Doc:        "Returns per-function field access summaries for a Go package.",
			ParamNames: []string{"pattern"}, Category: "goast-ssa"},
		{Name: "go-ssa-canonicalize", ParamCount: 1, Impl: PrimGoSSACanonicalize,
			Doc:        "Canonicalizes an SSA function s-expression: dominator-order blocks, alpha-renamed registers.",
			ParamNames: []string{"ssa-func"}, Category: "goast-ssa"},
	}, registry.PhaseRuntime)
	return nil
}
