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
	"github.com/aalpar/wile/pkg/registry"
	"github.com/aalpar/wile/pkg/values"
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

func addPrimitives(r *registry.PrimitiveRegistry) error {
	r.AddPrimitives([]registry.PrimitiveSpec{
		{Name: "go-cfg", ParamCount: 3, IsVariadic: true, Impl: PrimGoCFG,
			Doc: "Builds the control flow graph for a named function in a Go package.\n" +
				"First arg is a package pattern or GoSession.\n" +
				"Returns a list of cfg-block alists. Each block has: index, preds,\n" +
				"succs, and idom (#f for entry block). Optional fields: comment,\n" +
				"recover (#t for recover block), closures (func literals the block\n" +
				"builds). Option 'positions adds pos fields.\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define blocks (go-cfg \"./...\" \"MyFunc\"))\n" +
				"  (nf (car blocks) 'index)  ; => 0\n" +
				"  (nf (car blocks) 'succs)  ; => (1 2)\n" +
				"  (nf (car blocks) 'idom)   ; => #f  (entry block)\n" +
				"  ;; With positions:\n" +
				"  (go-cfg \"./...\" \"MyFunc\" 'positions)\n\n" +
				"Func literals are reachable, and visible. go/ssa hoists each `func'\n" +
				"literal into its enclosing function under the name parent$N, 1-based\n" +
				"in go/ssa's build order -- source order in an ordinary body, but not\n" +
				"in a `for' post statement, a `select', or a package `init' that\n" +
				"aggregates var initializers across files. So pass \"MyFunc$1\" to get\n" +
				"that literal's own CFG; nesting composes (\"MyFunc$1$2\"), and a\n" +
				"literal in a method takes either form, \"M$1\" or \"(*pkg.T).M$1\".\n" +
				"A block that builds one carries a `closures' field naming it, so a\n" +
				"walk over the blocks finds every literal without a second lookup.\n" +
				"Those names are the qualified form (\"pkg.MyFunc$1\",\n" +
				"\"(*pkg.T).M$1\") and feed straight back in; the short form would not,\n" +
				"because it is not unique -- two same-named methods on different\n" +
				"types both yield \"Same$1\".\n" +
				"go/ssa emits no instruction at all for a literal that captures\n" +
				"nothing, so that one is attributed to the block that first uses it\n" +
				"instead. A bound-method wrapper (`return t.M') is not a literal and\n" +
				"is not listed; a Go 1.23 range-over-func body is one, and is.\n\n" +
				"LIMITATION -- one CFG per function. A literal's blocks are NOT merged\n" +
				"into the enclosing function's, because block indices are per-function\n" +
				"and preds/succs index into one block list. So a derived metric\n" +
				"(cyclomatic = E - N + 2) still measures one function at a time; sum\n" +
				"over a function and its literals, less 1 per extra function, to cover\n" +
				"the source function as written.\n\n" +
				"See also: `go-cfg-dominators', `go-cfg-paths', `go-cfg-to-structured'.",
			ParamNames: []string{"pattern", "func-name", "options"}, Category: "goast-cfg",
			ReturnType: values.TypeList},
		{Name: "go-cfg-dominators", ParamCount: 1, Impl: PrimGoCFGDominators,
			Doc: "Builds a dominator tree from a cfg-block list returned by go-cfg.\n" +
				"Returns a list of dom-node alists. Each node has: block (index),\n" +
				"idom (#f for root), and children (list of dominated block indices).\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define cfg (go-cfg \"./...\" \"MyFunc\"))\n" +
				"  (define dom (go-cfg-dominators cfg))\n" +
				"  (nf (car dom) 'block)     ; => 0\n" +
				"  (nf (car dom) 'children)  ; => (1 2)\n" +
				"  (nf (car dom) 'idom)      ; => #f  (root)\n\n" +
				"See also: `go-cfg', `go-cfg-dominates?'.",
			ParamNames: []string{"cfg"}, Category: "goast-cfg",
			ParamTypes: []values.TypeConstraint{values.TypeList},
			ReturnType: values.TypeList},
		{Name: "go-cfg-dominates?", ParamCount: 3, Impl: PrimGoCFGDominates,
			Doc: "Returns #t if block A dominates block B in the dominator tree.\n" +
				"A dominates B if A is on every path from the entry to B.\n\n" +
				"Examples:\n" +
				"  (define dom (go-cfg-dominators (go-cfg \"./...\" \"MyFunc\")))\n" +
				"  (go-cfg-dominates? dom 0 3)  ; => #t  (entry dominates all)\n" +
				"  (go-cfg-dominates? dom 3 0)  ; => #f\n\n" +
				"See also: `go-cfg-dominators', `go-cfg'.",
			ParamNames: []string{"dom-tree", "a", "b"}, Category: "goast-cfg",
			ReturnType: values.TypeBoolean},
		{Name: "go-cfg-paths", ParamCount: 3, Impl: PrimGoCFGPaths,
			Doc: "Enumerates simple paths between two blocks in the CFG.\n" +
				"Returns a list of paths; each path is a list of block indices.\n" +
				"Capped at 1024 paths to bound computation.\n\n" +
				"Examples:\n" +
				"  (define cfg (go-cfg \"./...\" \"MyFunc\"))\n" +
				"  (define paths (go-cfg-paths cfg 0 5))\n" +
				"  (car paths)    ; => (0 1 3 5)  — a path through blocks\n" +
				"  (length paths) ; => number of distinct paths\n\n" +
				"See also: `go-cfg', `go-cfg-dominators'.",
			ParamNames: []string{"cfg", "from", "to"}, Category: "goast-cfg",
			ReturnType: values.TypeList},
	}, registry.PhaseSetRuntime)
	return nil
}
