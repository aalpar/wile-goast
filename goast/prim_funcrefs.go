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
	"go/ast"
	"go/token"
	"sort"

	"golang.org/x/tools/go/packages"

	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

// PrimGoFuncRefs implements (go-func-refs target).
// target is a package pattern string or a GoSession from go-load.
// For each function/method in the loaded packages, returns the set of
// external (cross-package) objects it references via types.Info.Uses.
func PrimGoFuncRefs(mc machine.CallContext) error {
	arg := mc.Arg(0)
	session, ok := UnwrapSession(arg)
	if ok {
		return funcRefsFromSession(mc, session)
	}
	pat, ok := arg.(*values.String)
	if !ok {
		return werr.WrapForeignErrorf(werr.ErrNotAString,
			"go-func-refs: expected string or go-session, got %T", arg)
	}
	return funcRefsFromPattern(mc, pat)
}

func funcRefsFromSession(mc machine.CallContext, session *GoSession) error {
	result := buildFuncRefs(session.Packages())
	mc.SetValue(result)
	return nil
}

func funcRefsFromPattern(mc machine.CallContext, pattern *values.String) error {
	fset := token.NewFileSet()
	pkgs, err := LoadPackagesChecked(mc,
		packages.NeedName|packages.NeedFiles|packages.NeedSyntax|
			packages.NeedTypes|packages.NeedTypesInfo,
		fset, errGoPackageLoad, "go-func-refs",
		pattern.Value)
	if err != nil {
		return err
	}
	result := buildFuncRefs(pkgs)
	mc.SetValue(result)
	return nil
}

// funcRefEntry holds the per-function external reference data during collection.
type funcRefEntry struct {
	name    string
	pkg     string
	extRefs map[string]map[string]bool // ext-pkg-path -> set of object names
}

// buildFuncRefs walks all FuncDecls in the given packages, collecting
// external references via types.Info.Uses.
func buildFuncRefs(pkgs []*packages.Package) values.Value {
	var entries []funcRefEntry

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				if fn.Body == nil {
					continue
				}

				name := funcDeclName(fn)
				entry := funcRefEntry{
					name:    name,
					pkg:     pkg.PkgPath,
					extRefs: make(map[string]map[string]bool),
				}

				ast.Inspect(fn.Body, func(n ast.Node) bool {
					ident, ok := n.(*ast.Ident)
					if !ok {
						return true
					}
					obj, exists := pkg.TypesInfo.Uses[ident]
					if !exists {
						return true
					}
					objPkg := obj.Pkg()
					if objPkg == nil || objPkg.Path() == pkg.PkgPath {
						return true
					}
					extPath := objPkg.Path()
					if entry.extRefs[extPath] == nil {
						entry.extRefs[extPath] = make(map[string]bool)
					}
					entry.extRefs[extPath][obj.Name()] = true
					return true
				})

				entries = append(entries, entry)
			}
		}
	}

	// Sort entries deterministically: by package path, then by name.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].pkg != entries[j].pkg {
			return entries[i].pkg < entries[j].pkg
		}
		return entries[i].name < entries[j].name
	})

	result := make([]values.Value, len(entries))
	for i, e := range entries {
		result[i] = mapFuncRefEntry(e)
	}
	return ValueList(result)
}

// mapFuncRefEntry converts a funcRefEntry to a tagged alist:
// (func-ref (name . N) (pkg . P) (refs . ((ref (pkg . X) (objects . (...))))))
func mapFuncRefEntry(e funcRefEntry) values.Value {
	// Sort external package paths for deterministic output.
	paths := make([]string, 0, len(e.extRefs))
	for p := range e.extRefs {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	refs := make([]values.Value, len(paths))
	for i, p := range paths {
		// Sort object names within each package.
		names := make([]string, 0, len(e.extRefs[p]))
		for n := range e.extRefs[p] {
			names = append(names, n)
		}
		sort.Strings(names)

		objs := make([]values.Value, len(names))
		for j, n := range names {
			objs[j] = Str(n)
		}

		refs[i] = Node("ref",
			Field("pkg", Str(p)),
			Field("objects", ValueList(objs)),
		)
	}

	return Node("func-ref",
		Field("name", Str(e.name)),
		Field("pkg", Str(e.pkg)),
		Field("refs", ValueList(refs)),
	)
}

// funcDeclName returns the name of a FuncDecl, formatted as "RecvType.Method"
// for methods with a receiver.
func funcDeclName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0]
	return recvTypeName(recv.Type) + "." + fn.Name.Name
}

// recvTypeName extracts the type name from a receiver expression,
// stripping pointer indirection (*T -> T).
func recvTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return recvTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		id, ok := t.X.(*ast.Ident)
		if ok {
			return id.Name
		}
	case *ast.IndexListExpr:
		id, ok := t.X.(*ast.Ident)
		if ok {
			return id.Name
		}
	}
	return "_"
}
