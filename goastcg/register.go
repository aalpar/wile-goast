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

package goastcg

import (
	"github.com/aalpar/wile/registry"
	"github.com/aalpar/wile/values"
)

// cgExtension wraps Extension to implement LibraryNamer.
type cgExtension struct {
	registry.Extension
}

// LibraryName returns (wile goast callgraph) for R7RS import.
func (p *cgExtension) LibraryName() []string {
	return []string{"wile", "goast", "callgraph"}
}

// Extension is the callgraph extension entry point.
var Extension registry.Extension = &cgExtension{
	Extension: registry.NewExtension("goast-callgraph", AddToRegistry),
}

// Builder aggregates all callgraph registration functions.
var Builder = registry.NewRegistryBuilder(addPrimitives)

// AddToRegistry registers all callgraph primitives.
var AddToRegistry = Builder.AddToRegistry

func addPrimitives(r *registry.Registry) error {
	r.AddPrimitives([]registry.PrimitiveSpec{
		{Name: "go-callgraph", ParamCount: 2, Impl: PrimGoCallgraph,
			Doc: "Builds a call graph for a Go package using the specified algorithm.\n" +
				"Algorithm is a symbol: 'static, 'cha, 'rta, or 'vta.\n" +
				"First arg is a package pattern or GoSession.\n\n" +
				"Examples:\n" +
				"  (go-callgraph \"./...\" 'rta)\n" +
				"  (go-callgraph (go-load \"./...\") 'cha)\n\n" +
				"See also: `go-callgraph-callers', `go-callgraph-callees', `go-callgraph-reachable'.",
			ParamNames: []string{"pattern", "algorithm"}, Category: "goast-callgraph",
			ReturnType: values.TypeList},
		{Name: "go-callgraph-callers", ParamCount: 2, Impl: PrimGoCallgraphCallers,
			Doc: "Returns the incoming edges (callers) of a function in the call graph.\n" +
				"Returns #f if the function is not found. Use qualified names for methods\n" +
				"(e.g., \"(*Type).Method\").\n\n" +
				"Examples:\n" +
				"  (go-callgraph-callers cg \"handleRequest\")\n\n" +
				"See also: `go-callgraph', `go-callgraph-callees', `callers-of'.",
			ParamNames: []string{"graph", "func-name"}, Category: "goast-callgraph"},
		{Name: "go-callgraph-callees", ParamCount: 2, Impl: PrimGoCallgraphCallees,
			Doc: "Returns the outgoing edges (callees) of a function in the call graph.\n" +
				"Returns #f if the function is not found.\n\n" +
				"Examples:\n" +
				"  (go-callgraph-callees cg \"main\")\n\n" +
				"See also: `go-callgraph', `go-callgraph-callers'.",
			ParamNames: []string{"graph", "func-name"}, Category: "goast-callgraph"},
		{Name: "go-callgraph-reachable", ParamCount: 2, Impl: PrimGoCallgraphReachable,
			Doc: "Returns function names transitively reachable from the root in the call graph.\n\n" +
				"Examples:\n" +
				"  (go-callgraph-reachable cg \"main\")\n\n" +
				"See also: `go-callgraph'.",
			ParamNames: []string{"graph", "root-name"}, Category: "goast-callgraph",
			ReturnType: values.TypeList},
	}, registry.PhaseRuntime)
	return nil
}
