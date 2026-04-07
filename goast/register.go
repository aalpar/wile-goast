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

package goast

import (
	"github.com/aalpar/wile/registry"
	"github.com/aalpar/wile/values"
)

// Extension is the Go AST extension.
var Extension = registry.NewExtension("goast", AddToRegistry)

// Builder aggregates all Go AST registration functions.
var Builder = registry.NewRegistryBuilder(addPrimitives)

// AddToRegistry registers all Go AST primitives.
var AddToRegistry = Builder.AddToRegistry

func addPrimitives(r *registry.Registry) error {
	r.AddPrimitives([]registry.PrimitiveSpec{
		{Name: "go-parse-file", ParamCount: 2, IsVariadic: true, Impl: PrimGoParseFile,
			Doc: "Parses a Go source file and returns an s-expression AST.\n" +
				"Options: 'comments includes comment nodes in the AST.\n\n" +
				"Examples:\n" +
				"  (go-parse-file \"main.go\")\n" +
				"  (go-parse-file \"main.go\" 'comments)\n\n" +
				"See also: `go-parse-string', `go-parse-expr', `go-format'.",
			ParamNames: []string{"filename", "options"}, Category: "goast",
			ReturnType: values.TypeList},
		{Name: "go-parse-string", ParamCount: 2, IsVariadic: true, Impl: PrimGoParseString,
			Doc: "Parses a Go source string as a complete file and returns an s-expression AST.\n" +
				"Options: 'comments includes comment nodes.\n\n" +
				"Examples:\n" +
				"  (go-parse-string \"package main\\nfunc f() {}\")\n" +
				"  (go-parse-string \"package main\" 'comments)\n\n" +
				"See also: `go-parse-file', `go-parse-expr', `go-format'.",
			ParamNames: []string{"source", "options"}, Category: "goast",
			ReturnType: values.TypeList},
		{Name: "go-parse-expr", ParamCount: 1, Impl: PrimGoParseExpr,
			Doc: "Parses a single Go expression and returns an s-expression AST.\n\n" +
				"Examples:\n" +
				"  (go-parse-expr \"x + y\")\n" +
				"  (go-parse-expr \"func() { return 1 }\")\n\n" +
				"See also: `go-parse-file', `go-node-type'.",
			ParamNames: []string{"source"}, Category: "goast",
			ParamTypes: []values.ValueType{values.TypeString},
			ReturnType: values.TypeList},
		{Name: "go-format", ParamCount: 1, Impl: PrimGoFormat,
			Doc: "Converts an s-expression AST back to Go source code.\n" +
				"Falls back to unformatted output for partial or invalid ASTs.\n\n" +
				"Examples:\n" +
				"  (go-format (go-parse-expr \"x + 1\"))\n" +
				"  (go-format (go-parse-file \"main.go\"))\n\n" +
				"See also: `go-parse-file', `go-parse-string'.",
			ParamNames: []string{"ast"}, Category: "goast",
			ParamTypes: []values.ValueType{values.TypeList},
			ReturnType: values.TypeString},
		{Name: "go-node-type", ParamCount: 1, Impl: PrimGoNodeType,
			Doc: "Returns the tag symbol of an AST node (e.g., 'func-decl, 'binary-expr).\n\n" +
				"Examples:\n" +
				"  (go-node-type (go-parse-expr \"x + 1\"))  ; => binary-expr\n\n" +
				"See also: `go-parse-file', `nf'.",
			ParamNames: []string{"ast"}, Category: "goast",
			ParamTypes: []values.ValueType{values.TypeList}},
		{Name: "go-typecheck-package", ParamCount: 2, IsVariadic: true, Impl: PrimGoTypecheckPackage,
			Doc: "Loads a Go package with full type information and returns annotated ASTs.\n" +
				"First arg is a package pattern or GoSession. Options: 'debug.\n\n" +
				"Examples:\n" +
				"  (go-typecheck-package \"./...\")\n" +
				"  (go-typecheck-package (go-load \"./...\"))\n\n" +
				"See also: `go-load', `go-ssa-build', `go-interface-implementors'.",
			ParamNames: []string{"pattern", "options"}, Category: "goast",
			ReturnType: values.TypeList},
		{Name: "go-interface-implementors", ParamCount: 2, Impl: PrimInterfaceImplementors,
			Doc: "Finds all concrete types implementing a named interface.\n" +
				"Second arg is a package pattern or GoSession.\n\n" +
				"Examples:\n" +
				"  (go-interface-implementors \"Reader\" \"io/...\")\n" +
				"  (go-interface-implementors \"Handler\" (go-load \"net/http\"))\n\n" +
				"See also: `go-typecheck-package', `implementors-of'.",
			ParamNames: []string{"interface-name", "package-pattern"}, Category: "goast",
			ReturnType: values.TypeList},
		{Name: "go-load", ParamCount: 2, IsVariadic: true, Impl: PrimGoLoad,
			Doc: "Loads Go packages and returns a GoSession for reuse across analysis primitives.\n" +
				"Avoids redundant packages.Load calls when multiple primitives share a target.\n\n" +
				"Examples:\n" +
				"  (define s (go-load \"./...\"))\n" +
				"  (go-typecheck-package s)\n" +
				"  (go-ssa-build s)\n\n" +
				"See also: `go-session?', `go-typecheck-package', `go-ssa-build'.",
			ParamNames: []string{"pattern", "rest"}, Category: "goast"},
		{Name: "go-session?", ParamCount: 1, Impl: PrimGoSessionP,
			Doc: "Returns #t if the argument is a GoSession value.\n\n" +
				"Examples:\n" +
				"  (go-session? (go-load \"./...\"))  ; => #t\n" +
				"  (go-session? \"./...\")            ; => #f\n\n" +
				"See also: `go-load'.",
			ParamNames: []string{"v"}, Category: "goast",
			ReturnType: values.TypeBoolean},
		{Name: "go-list-deps", ParamCount: 2, IsVariadic: true, Impl: PrimGoListDeps,
			Doc: "Returns the transitive closure of import paths for the given package patterns.\n" +
				"Lightweight alternative to go-load for dependency discovery.\n\n" +
				"Examples:\n" +
				"  (go-list-deps \"net/http\")\n\n" +
				"See also: `go-load'.",
			ParamNames: []string{"pattern", "rest"}, Category: "goast",
			ReturnType: values.TypeList},
		{Name: "go-cfg-to-structured", ParamCount: 2, IsVariadic: true, Impl: PrimGoCFGToStructured,
			Doc: "Restructures a block containing early returns into a single-exit if/else tree.\n" +
				"Optional second arg: func-type for result variable synthesis.\n" +
				"Phase 1 rewrites returns inside for/range to break+guard. Phase 2 folds\n" +
				"guard-if-return sequences into nested if/else via right-fold.\n\n" +
				"Examples:\n" +
				"  (go-cfg-to-structured block)\n" +
				"  (go-cfg-to-structured block func-type)\n\n" +
				"See also: `go-cfg', `go-format'.",
			ParamNames: []string{"block", "rest"}, Category: "goast",
			ReturnType: values.TypeList},
	}, registry.PhaseRuntime)
	return nil
}
