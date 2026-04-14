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
				"Options: 'comments includes comment nodes in the AST.\n" +
				"Returns a file node with: package, decls, imports.\n" +
				"Declarations are tagged nodes: func-decl, gen-decl, etc.\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define file (go-parse-file \"main.go\"))\n" +
				"  (nf file 'package)    ; => \"main\"\n" +
				"  (define decls (nf file 'decls))\n" +
				"  (tag? (car decls) 'func-decl)  ; => #t\n" +
				"  (nf (car decls) 'name)          ; => \"main\"\n\n" +
				"See also: `go-parse-string', `go-parse-expr', `go-format'.",
			ParamNames: []string{"filename", "options"}, Category: "goast",
			ReturnType: values.TypeList},
		{Name: "go-parse-string", ParamCount: 2, IsVariadic: true, Impl: PrimGoParseString,
			Doc: "Parses a Go source string as a complete file and returns an s-expression AST.\n" +
				"Options: 'comments includes comment nodes.\n" +
				"Returns the same file node shape as go-parse-file.\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define file (go-parse-string \"package main\\nfunc f() {}\"))\n" +
				"  (nf file 'package)  ; => \"main\"\n" +
				"  (nf (car (nf file 'decls)) 'name)  ; => \"f\"\n\n" +
				"See also: `go-parse-file', `go-parse-expr', `go-format'.",
			ParamNames: []string{"source", "options"}, Category: "goast",
			ReturnType: values.TypeList},
		{Name: "go-parse-expr", ParamCount: 1, Impl: PrimGoParseExpr,
			Doc: "Parses a single Go expression and returns an s-expression AST.\n" +
				"The node tag identifies the expression type.\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define expr (go-parse-expr \"x + y\"))\n" +
				"  (go-node-type expr)  ; => binary-expr\n" +
				"  (nf expr 'op)        ; => +\n" +
				"  (nf expr 'x)         ; => (ident (name . \"x\"))\n\n" +
				"See also: `go-parse-file', `go-node-type'.",
			ParamNames: []string{"source"}, Category: "goast",
			ParamTypes: []values.TypeConstraint{values.TypeString},
			ReturnType: values.TypeList},
		{Name: "go-format", ParamCount: 1, Impl: PrimGoFormat,
			Doc: "Converts an s-expression AST back to Go source code.\n" +
				"Falls back to unformatted output for partial or invalid ASTs.\n" +
				"Returns a string.\n\n" +
				"Examples:\n" +
				"  (go-format (go-parse-expr \"x + 1\"))  ; => \"x + 1\"\n" +
				"  ;; Round-trip: parse, transform, format\n" +
				"  (go-format (go-parse-file \"main.go\"))\n\n" +
				"See also: `go-parse-file', `go-parse-string'.",
			ParamNames: []string{"ast"}, Category: "goast",
			ParamTypes: []values.TypeConstraint{values.TypeList},
			ReturnType: values.TypeString},
		{Name: "go-node-type", ParamCount: 1, Impl: PrimGoNodeType,
			Doc: "Returns the tag symbol of an AST node.\n" +
				"Common tags: func-decl, gen-decl, binary-expr, call-expr, ident,\n" +
				"selector-expr, if-stmt, for-stmt, return-stmt, assign-stmt.\n\n" +
				"Examples:\n" +
				"  (go-node-type (go-parse-expr \"x + 1\"))   ; => binary-expr\n" +
				"  (go-node-type (go-parse-expr \"f(x)\"))     ; => call-expr\n" +
				"  ;; Equivalent to (car node) but safer:\n" +
				"  (tag? node 'func-decl)  ; preferred for dispatch\n\n" +
				"See also: `go-parse-file', `nf', `tag?'.",
			ParamNames: []string{"ast"}, Category: "goast",
			ParamTypes: []values.TypeConstraint{values.TypeList}},
		{Name: "go-typecheck-package", ParamCount: 2, IsVariadic: true, Impl: PrimGoTypecheckPackage,
			Doc: "Loads a Go package with full type information and returns annotated ASTs.\n" +
				"First arg is a package pattern or GoSession. Options: 'debug.\n" +
				"Returns a list of package alists. Each has: name, path, files.\n" +
				"Type annotations appear as inferred-type fields on AST nodes.\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define pkgs (go-typecheck-package \"./...\"))\n" +
				"  (nf (car pkgs) 'name)   ; => \"main\"\n" +
				"  (nf (car pkgs) 'path)   ; => \"my/module\"\n" +
				"  (define files (nf (car pkgs) 'files))\n" +
				"  (nf (car (nf (car files) 'decls)) 'name)  ; => \"main\"\n\n" +
				"See also: `go-load', `go-ssa-build', `go-interface-implementors'.",
			ParamNames: []string{"pattern", "options"}, Category: "goast",
			ReturnType: values.TypeList},
		{Name: "go-interface-implementors", ParamCount: 2, Impl: PrimInterfaceImplementors,
			Doc: "Finds all concrete types implementing a named interface.\n" +
				"Second arg is a package pattern or GoSession.\n" +
				"Returns an alist with: methods (list of method name strings)\n" +
				"and implementors (list of alists, each with a type field).\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define info (go-interface-implementors \"Reader\" \"io/...\"))\n" +
				"  (nf info 'methods)       ; => (\"Read\")\n" +
				"  (define impls (nf info 'implementors))\n" +
				"  (nf (car impls) 'type)   ; => \"*bytes.Buffer\"\n\n" +
				"See also: `go-typecheck-package', `implementors-of'.",
			ParamNames: []string{"interface-name", "package-pattern"}, Category: "goast",
			ReturnType: values.TypeList},
		{Name: "go-load", ParamCount: 2, IsVariadic: true, Impl: PrimGoLoad,
			Doc: "Loads Go packages and returns a GoSession for reuse across analysis primitives.\n" +
				"Avoids redundant packages.Load calls when multiple primitives share a target.\n" +
				"GoSession is an opaque value accepted by go-typecheck-package, go-ssa-build,\n" +
				"go-callgraph, go-cfg, go-analyze, and go-ssa-field-index.\n\n" +
				"Examples:\n" +
				"  (define s (go-load \"./...\"))\n" +
				"  (go-session? s)           ; => #t\n" +
				"  (go-typecheck-package s)   ; reuses loaded packages\n" +
				"  (go-ssa-build s)           ; reuses loaded packages\n" +
				"  (go-callgraph s 'cha)      ; reuses loaded packages\n\n" +
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
				"Lightweight alternative to go-load for dependency discovery.\n" +
				"Returns a list of import path strings.\n\n" +
				"Examples:\n" +
				"  (define deps (go-list-deps \"net/http\"))\n" +
				"  (length deps)          ; => number of transitive deps\n" +
				"  (member \"net\" deps)    ; => (\"net\" ...) or #f\n\n" +
				"See also: `go-load'.",
			ParamNames: []string{"pattern", "rest"}, Category: "goast",
			ReturnType: values.TypeList},
		{Name: "go-func-refs", ParamCount: 1, Impl: PrimGoFuncRefs,
			Doc: "Returns per-function external reference profiles for a Go package.\n" +
				"For each function/method, lists the external (package, object-names)\n" +
				"pairs it references via types.Info.Uses.\n" +
				"Input: package pattern string or GoSession.\n" +
				"Output: list of (func-ref (name . N) (pkg . P) (refs . ((ref ...)))).\n\n" +
				"Examples:\n" +
				"  (define refs (go-func-refs \"my/pkg\"))\n" +
				"  (cdr (assoc 'name (cdr (car refs))))  ; => \"MyFunc\"\n" +
				"  (cdr (assoc 'refs (cdr (car refs))))  ; => ((ref (pkg . \"io\") (objects . (\"Reader\"))))\n\n" +
				"See also: `go-typecheck-package', `go-load', `go-list-deps'.",
			ParamNames: []string{"target"}, Category: "goast",
			ReturnType: values.TypeList},
		{Name: "go-cfg-to-structured", ParamCount: 2, IsVariadic: true, Impl: PrimGoCFGToStructured,
			Doc: "Restructures a block containing early returns into a single-exit if/else tree.\n" +
				"Optional second arg: func-type for result variable synthesis.\n" +
				"Phase 1 rewrites returns inside for/range to break+guard. Phase 2 folds\n" +
				"guard-if-return sequences into nested if/else via right-fold.\n" +
				"Returns a block AST node suitable for go-format.\n\n" +
				"Examples:\n" +
				"  (import (wile goast utils))\n" +
				"  (define block (nf func-decl 'body))\n" +
				"  (define structured (go-cfg-to-structured block))\n" +
				"  (go-format structured)  ; => Go source with single exit\n\n" +
				"See also: `go-cfg', `go-format'.",
			ParamNames: []string{"block", "rest"}, Category: "goast",
			ReturnType: values.TypeList},
	}, registry.PhaseRuntime)
	return nil
}
