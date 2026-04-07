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

package goastcfg

import (
	"github.com/aalpar/wile/registry"
	"github.com/aalpar/wile/values"
)

// cfgExtension wraps Extension to implement LibraryNamer.
type cfgExtension struct {
	registry.Extension
}

// LibraryName returns (wile goast cfg) for R7RS import.
func (p *cfgExtension) LibraryName() []string {
	return []string{"wile", "goast", "cfg"}
}

// Extension is the CFG extension entry point.
var Extension registry.Extension = &cfgExtension{
	Extension: registry.NewExtension("goast-cfg", AddToRegistry),
}

// Builder aggregates all CFG registration functions.
var Builder = registry.NewRegistryBuilder(addPrimitives)

// AddToRegistry registers all CFG primitives.
var AddToRegistry = Builder.AddToRegistry

func addPrimitives(r *registry.Registry) error {
	r.AddPrimitives([]registry.PrimitiveSpec{
		{Name: "go-cfg", ParamCount: 3, IsVariadic: true, Impl: PrimGoCFG,
			Doc: "Builds the control flow graph for a named function in a Go package.\n" +
				"First arg is a package pattern or GoSession.\n\n" +
				"Examples:\n" +
				"  (go-cfg \"./...\" \"MyFunc\")\n" +
				"  (go-cfg (go-load \"./...\") \"MyFunc\")\n\n" +
				"See also: `go-cfg-dominators', `go-cfg-paths', `go-cfg-to-structured'.",
			ParamNames: []string{"pattern", "func-name", "options"}, Category: "goast-cfg",
			ReturnType: values.TypeList},
		{Name: "go-cfg-dominators", ParamCount: 1, Impl: PrimGoCFGDominators,
			Doc: "Builds a dominator tree from a cfg-block list returned by go-cfg.\n" +
				"Uses the Lengauer-Tarjan algorithm via golang.org/x/tools.\n\n" +
				"Examples:\n" +
				"  (go-cfg-dominators (go-cfg \"./...\" \"MyFunc\"))\n\n" +
				"See also: `go-cfg', `go-cfg-dominates?'.",
			ParamNames: []string{"cfg"}, Category: "goast-cfg",
			ParamTypes: []values.ValueType{values.TypeList},
			ReturnType: values.TypeList},
		{Name: "go-cfg-dominates?", ParamCount: 3, Impl: PrimGoCFGDominates,
			Doc: "Returns #t if block A dominates block B in the dominator tree.\n\n" +
				"Examples:\n" +
				"  (go-cfg-dominates? dom-tree 0 3)\n\n" +
				"See also: `go-cfg-dominators', `go-cfg'.",
			ParamNames: []string{"dom-tree", "a", "b"}, Category: "goast-cfg",
			ReturnType: values.TypeBoolean},
		{Name: "go-cfg-paths", ParamCount: 3, Impl: PrimGoCFGPaths,
			Doc: "Enumerates simple paths between two blocks in the CFG.\n" +
				"Capped at 1024 paths to bound computation.\n\n" +
				"Examples:\n" +
				"  (go-cfg-paths cfg 0 5)\n\n" +
				"See also: `go-cfg', `go-cfg-dominators'.",
			ParamNames: []string{"cfg", "from", "to"}, Category: "goast-cfg",
			ReturnType: values.TypeList},
	}, registry.PhaseRuntime)
	return nil
}
