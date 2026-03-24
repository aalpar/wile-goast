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
			Doc:        "Parses a Go source file and returns an s-expression AST.",
			ParamNames: []string{"filename", "options"}, Category: "goast"},
		{Name: "go-parse-string", ParamCount: 2, IsVariadic: true, Impl: PrimGoParseString,
			Doc:        "Parses a Go source string as a file and returns an s-expression AST.",
			ParamNames: []string{"source", "options"}, Category: "goast"},
		{Name: "go-parse-expr", ParamCount: 1, Impl: PrimGoParseExpr,
			Doc:        "Parses a single Go expression and returns an s-expression AST.",
			ParamNames: []string{"source"}, Category: "goast"},
		{Name: "go-format", ParamCount: 1, Impl: PrimGoFormat,
			Doc:        "Converts an s-expression AST to Go source. Falls back to unformatted output for partial ASTs.",
			ParamNames: []string{"ast"}, Category: "goast"},
		{Name: "go-node-type", ParamCount: 1, Impl: PrimGoNodeType,
			Doc:        "Returns the tag symbol of an AST node.",
			ParamNames: []string{"ast"}, Category: "goast"},
		{Name: "go-typecheck-package", ParamCount: 2, IsVariadic: true, Impl: PrimGoTypecheckPackage,
			Doc:        "Loads a Go package with type information and returns annotated s-expression ASTs.",
			ParamNames: []string{"pattern", "options"}, Category: "goast"},
		{Name: "go-interface-implementors", ParamCount: 2, Impl: PrimInterfaceImplementors,
			Doc:        "Finds all types implementing a named interface within the specified packages.",
			ParamNames: []string{"interface-name", "package-pattern"}, Category: "goast"},
		{Name: "go-load", ParamCount: 2, IsVariadic: true, Impl: PrimGoLoad,
			Doc:        "Loads Go packages and returns a GoSession for reuse across analysis primitives.",
			ParamNames: []string{"pattern", "rest"}, Category: "goast"},
		{Name: "go-session?", ParamCount: 1, Impl: PrimGoSessionP,
			Doc:        "Returns #t if the argument is a GoSession.",
			ParamNames: []string{"v"}, Category: "goast"},
		{Name: "go-list-deps", ParamCount: 2, IsVariadic: true, Impl: PrimGoListDeps,
			Doc:        "Returns the transitive closure of import paths for the given package patterns.",
			ParamNames: []string{"pattern", "rest"}, Category: "goast"},
		{Name: "go-cfg-to-structured", ParamCount: 1, Impl: PrimGoCFGToStructured,
			Doc:        "Restructures a block with early returns into a single-exit if/else tree.",
			ParamNames: []string{"block"}, Category: "goast"},
	}, registry.PhaseRuntime)
	return nil
}
