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
				"Algorithm is a symbol: 'static, 'cha, 'rta, 'vta, or 'precise.\n" +
				"  'cha/'vta/'rta soundly OVER-approximate indirect (func-value)\n" +
				"  calls; 'static omits them (UNDER-approximates). 'precise refines\n" +
				"  'cha by statically resolving the decidable subset of indirect\n" +
				"  calls (a constant index into a literal []func(), e.g. t[0]() of\n" +
				"  []func(){f,g}), keeping 'cha's sound edges where it cannot — so\n" +
				"  it is never less sound than 'cha, only sharper where SSA permits.\n" +
				"First arg is a package pattern or GoSession.\n" +
				"Returns a list of cg-node alists. Each node has: name, id,\n" +
				"edges-in, edges-out, and pkg. Edges are cg-edge alists with:\n" +
				"caller, callee, pos, and description.\n" +
				"Node names are fully qualified (e.g., \"pkg.Func\", \"(*pkg.Type).Method\").\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define cg (go-callgraph \"./...\" 'cha))\n" +
				"  (nf (car cg) 'name)       ; => \"pkg.init\"\n" +
				"  (nf (car cg) 'edges-out)  ; => list of cg-edge nodes\n" +
				"  (let ((edge (car (nf (car cg) 'edges-out))))\n" +
				"    (nf edge 'callee))       ; => \"pkg.helper\"\n\n" +
				"See also: `go-callgraph-callers', `go-callgraph-callees', `go-callgraph-reachable'.",
			ParamNames: []string{"pattern", "algorithm"}, Category: "goast-callgraph",
			ReturnType: values.TypeList},
		{Name: "go-callgraph-callers", ParamCount: 2, Impl: PrimGoCallgraphCallers,
			Doc: "Returns the incoming edges (callers) of a function in the call graph.\n" +
				"Returns #f if the function is not found.\n" +
				"IMPORTANT: func-name must be fully qualified as it appears in the\n" +
				"call graph (e.g., \"pkg.Func\", \"(*pkg.Type).Method\").\n" +
				"Each edge is a cg-edge alist with: caller, callee, pos, description.\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define edges (go-callgraph-callers cg \"pkg.handleRequest\"))\n" +
				"  (if edges  ; #f when not found\n" +
				"    (for-each\n" +
				"      (lambda (e) (display (nf e 'caller)))\n" +
				"      edges))\n\n" +
				"See also: `go-callgraph', `go-callgraph-callees', `callers-of'.",
			ParamNames: []string{"graph", "func-name"}, Category: "goast-callgraph"},
		{Name: "go-callgraph-callees", ParamCount: 2, Impl: PrimGoCallgraphCallees,
			Doc: "Returns the outgoing edges (callees) of a function in the call graph.\n" +
				"Returns #f if the function is not found.\n" +
				"func-name must be fully qualified (e.g., \"pkg.main\").\n" +
				"Each edge is a cg-edge alist with: caller, callee, pos, description.\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define edges (go-callgraph-callees cg \"pkg.main\"))\n" +
				"  (if edges\n" +
				"    (map (lambda (e) (nf e 'callee)) edges))\n" +
				"  ; => (\"pkg.init\" \"pkg.run\" ...)\n\n" +
				"See also: `go-callgraph', `go-callgraph-callers'.",
			ParamNames: []string{"graph", "func-name"}, Category: "goast-callgraph"},
		// go-callgraph-reachable now lives in the (wile goast path-algebra)
		// Scheme layer, delegating reachability to wile's boolean-semiring
		// single-source path query (graph-query-all) — see path-algebra.scm.
		// Build the graph here ('precise/'cha/...), query it there.
	}, registry.PhaseSetRuntime)
	return nil
}
