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

package goastlint

import (
	"github.com/aalpar/wile/registry"
	"github.com/aalpar/wile/values"
)

// lintExtension wraps Extension to implement LibraryNamer.
type lintExtension struct {
	registry.Extension
}

// LibraryName returns (wile goast lint) for R7RS import.
func (p *lintExtension) LibraryName() []string {
	return []string{"wile", "goast", "lint"}
}

// Extension is the lint extension entry point.
var Extension registry.Extension = &lintExtension{
	Extension: registry.NewExtension("goast-lint", AddToRegistry),
}

// Builder aggregates all lint registration functions.
var Builder = registry.NewRegistryBuilder(addPrimitives)

// AddToRegistry registers all lint primitives.
var AddToRegistry = Builder.AddToRegistry

func addPrimitives(r *registry.Registry) error {
	r.AddPrimitives([]registry.PrimitiveSpec{
		{
			Name: "go-analyze", ParamCount: 2, IsVariadic: true,
			Impl: PrimGoAnalyze,
			Doc: "Runs named go/analysis passes on a Go package and returns diagnostics.\n" +
				"First arg is a package pattern or GoSession. Remaining args are\n" +
				"analyzer names (strings). Use go-analyze-list for available names.\n" +
				"Returns a list of diagnostic alists, each with: analyzer, pos,\n" +
				"message, and category.\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define diags (go-analyze \"./...\" \"nilness\" \"shadow\"))\n" +
				"  (for-each\n" +
				"    (lambda (d)\n" +
				"      (display (nf d 'pos))       ; => \"file.go:10:5\"\n" +
				"      (display (nf d 'message))   ; => \"nil dereference\"\n" +
				"      (display (nf d 'analyzer))) ; => \"nilness\"\n" +
				"    diags)\n\n" +
				"See also: `go-analyze-list'.",
			ParamNames: []string{"pattern", "analyzer-names"},
			Category:   "goast-lint",
			ReturnType: values.TypeList,
		},
		{
			Name: "go-analyze-list", ParamCount: 0,
			Impl: PrimGoAnalyzeList,
			Doc: "Returns a sorted list of available analyzer names as strings.\n" +
				"These names are valid arguments to go-analyze.\n\n" +
				"Examples:\n" +
				"  (go-analyze-list)  ; => (\"appends\" \"asmdecl\" \"assign\" ...)\n" +
				"  (length (go-analyze-list))  ; => ~40 analyzers\n\n" +
				"See also: `go-analyze'.",
			ParamNames: []string{},
			Category:   "goast-lint",
			ReturnType: values.TypeList,
		},
	}, registry.PhaseRuntime)
	return nil
}
