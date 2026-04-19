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
		{Name: "go-ssa-build", ParamCount: 1, IsVariadic: true, Impl: PrimGoSSABuild,
			Doc: "Builds SSA form for a Go package and returns a list of ssa-func nodes.\n" +
				"Pattern or GoSession may be the first arg; if absent, (current-go-target) is used.\n" +
				"Options: 'debug.\n" +
				"Each ssa-func has: name, signature, params, free-vars, blocks, pkg.\n" +
				"Each ssa-block has: index, preds, succs, instrs, and optional idom, comment.\n" +
				"Instructions are tagged nodes: ssa-call, ssa-if, ssa-return, ssa-store, etc.\n" +
				"ssa-call has: args, mode (call or invoke), func (or method+recv), name, type.\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define funcs (go-ssa-build \"./...\"))\n" +
				"  (nf (car funcs) 'name)       ; => \"init\"\n" +
				"  (nf (car funcs) 'pkg)         ; => \"my/pkg\"\n" +
				"  (define blocks (nf (car funcs) 'blocks))\n" +
				"  (nf (car blocks) 'index)      ; => 0\n" +
				"  (define instrs (nf (car blocks) 'instrs))\n" +
				"  (tag? (car instrs) 'ssa-call)  ; => #t or #f\n" +
				"  (nf (car instrs) 'func)        ; => \"fmt.Println\"\n\n" +
				"See also: `go-load', `go-ssa-canonicalize', `go-ssa-field-index'.",
			ParamNames: []string{"options"}, Category: "goast-ssa",
			ReturnType: values.TypeList},
		{Name: "go-ssa-field-index", ParamCount: 1, IsVariadic: true, Impl: PrimGoSSAFieldIndex,
			Doc: "Returns per-function field access summaries for a Go package.\n" +
				"Each entry is an ssa-field-summary with: func, pkg, fields.\n" +
				"Each field access is an ssa-field-access with: struct, struct-pkg,\n" +
				"field, recv, and mode (symbol: read or write).\n" +
				"Optional arg is a package pattern or GoSession; if absent, uses\n" +
				"(current-go-target).\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define idx (go-ssa-field-index \"./...\"))\n" +
				"  (nf (car idx) 'func)    ; => \"NewServer\"\n" +
				"  (nf (car idx) 'pkg)     ; => \"my/pkg\"\n" +
				"  (define accesses (nf (car idx) 'fields))\n" +
				"  (nf (car accesses) 'struct)  ; => \"Config\"\n" +
				"  (nf (car accesses) 'field)   ; => \"Host\"\n" +
				"  (nf (car accesses) 'mode)    ; => write\n\n" +
				"See also: `go-ssa-build', `stores-to-fields'.",
			ParamNames: []string{"pattern"}, Category: "goast-ssa",
			ReturnType: values.TypeList},
		{Name: "go-ssa-canonicalize", ParamCount: 1, Impl: PrimGoSSACanonicalize,
			Doc: "Canonicalizes an SSA function: dominator-order blocks, alpha-renamed registers.\n" +
				"Produces deterministic output for structural comparison.\n" +
				"Returns an ssa-func with the same shape as go-ssa-build output.\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define canonical (map go-ssa-canonicalize (go-ssa-build \"./...\")))\n" +
				"  ;; Registers are now t0, t1, ... in dominator order.\n" +
				"  ;; Two structurally equivalent functions produce identical output.\n\n" +
				"See also: `go-ssa-build'.",
			ParamNames: []string{"ssa-func"}, Category: "goast-ssa",
			ParamTypes: []values.TypeConstraint{values.TypeList},
			ReturnType: values.TypeList},
	}, registry.PhaseRuntime)
	return nil
}
