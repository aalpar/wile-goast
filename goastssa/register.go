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

import (
	"github.com/aalpar/wile/registry"
	"github.com/aalpar/wile/values"
)

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
			Doc: "Builds SSA form for a Go package and returns a list of ssa-func nodes.\n" +
				"First arg is a package pattern or GoSession. Options: 'debug.\n\n" +
				"Examples:\n" +
				"  (go-ssa-build \"./...\")\n" +
				"  (go-ssa-build (go-load \"./...\") 'debug)\n\n" +
				"See also: `go-load', `go-ssa-canonicalize', `go-ssa-field-index'.",
			ParamNames: []string{"pattern", "options"}, Category: "goast-ssa",
			ReturnType: values.TypeList},
		{Name: "go-ssa-field-index", ParamCount: 1, Impl: PrimGoSSAFieldIndex,
			Doc: "Returns per-function field access summaries for a Go package.\n" +
				"Each entry maps a function to its struct field store/load sites.\n" +
				"Arg is a package pattern or GoSession.\n\n" +
				"Examples:\n" +
				"  (go-ssa-field-index \"./...\")\n\n" +
				"See also: `go-ssa-build', `stores-to-fields'.",
			ParamNames: []string{"pattern"}, Category: "goast-ssa",
			ReturnType: values.TypeList},
		{Name: "go-ssa-canonicalize", ParamCount: 1, Impl: PrimGoSSACanonicalize,
			Doc: "Canonicalizes an SSA function: dominator-order blocks, alpha-renamed registers.\n" +
				"Produces deterministic output for structural comparison.\n\n" +
				"Examples:\n" +
				"  (map go-ssa-canonicalize (go-ssa-build \"./...\"))\n\n" +
				"See also: `go-ssa-build', `ssa-normalize'.",
			ParamNames: []string{"ssa-func"}, Category: "goast-ssa",
			ParamTypes: []values.ValueType{values.TypeList},
			ReturnType: values.TypeList},
	}, registry.PhaseRuntime)
	return nil
}
