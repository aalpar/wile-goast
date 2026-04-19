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
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/registry/helpers"
	"github.com/aalpar/wile/security"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

// parseOpts extracts mapper options from a variadic rest-arg list of symbols.
func parseOpts(rest values.Value, fset *token.FileSet) (*mapperOpts, parser.Mode, error) {
	opts := &mapperOpts{fset: fset}
	var mode parser.Mode

	tuple, ok := rest.(values.Tuple)
	if !ok {
		return opts, mode, nil
	}
	for !values.IsEmptyList(tuple) {
		pair, ok := tuple.(*values.Pair)
		if !ok {
			break
		}
		s, ok := pair.Car().(*values.Symbol)
		if !ok {
			return nil, 0, werr.WrapForeignErrorf(errGoParseError,
				"go-parse: options must be symbols, got %T", pair.Car())
		}
		switch s.Key {
		case "positions":
			opts.positions = true
		case "comments":
			opts.comments = true
			mode |= parser.ParseComments
		default:
			return nil, 0, werr.WrapForeignErrorf(errGoParseError,
				"go-parse: unknown option '%s'; valid options: positions, comments", s.Key)
		}
		cdr, ok := pair.Cdr().(values.Tuple)
		if !ok {
			break
		}
		tuple = cdr
	}
	return opts, mode, nil
}

// PrimGoParseFile implements (go-parse-file filename . options).
// Parses a Go source file from disk and returns an s-expression AST.
func PrimGoParseFile(mc machine.CallContext) error {
	filename, err := helpers.RequireArg[*values.String](mc, 0, werr.ErrNotAString, "go-parse-file")
	if err != nil {
		return err
	}

	err = security.CheckWithAuthorizer(mc.Authorizer(), security.AccessRequest{
		Resource: security.ResourceFile,
		Action:   security.ActionRead,
		Target:   filename.Value,
	})
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	opts, mode, optErr := parseOpts(mc.Arg(1), fset)
	if optErr != nil {
		return optErr
	}

	f, parseErr := parser.ParseFile(fset, filename.Value, nil, mode)
	if parseErr != nil {
		return werr.WrapForeignErrorf(errGoParseError,
			"go-parse-file: %s: %s", filename.Value, parseErr)
	}

	mc.SetValue(mapNode(f, opts))
	return nil
}

// PrimGoParseString implements (go-parse-string source . options).
// Parses a Go source string as a file and returns an s-expression AST.
func PrimGoParseString(mc machine.CallContext) error {
	source, err := helpers.RequireArg[*values.String](mc, 0, werr.ErrNotAString, "go-parse-string")
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	opts, mode, optErr := parseOpts(mc.Arg(1), fset)
	if optErr != nil {
		return optErr
	}

	f, parseErr := parser.ParseFile(fset, "source.go", source.Value, mode)
	if parseErr != nil {
		return werr.WrapForeignErrorf(errGoParseError,
			"go-parse-string: %s", parseErr)
	}

	mc.SetValue(mapNode(f, opts))
	return nil
}

// PrimGoParseExpr implements (go-parse-expr source).
// Parses a single Go expression and returns an s-expression AST.
func PrimGoParseExpr(mc machine.CallContext) error {
	source, err := helpers.RequireArg[*values.String](mc, 0, werr.ErrNotAString, "go-parse-expr")
	if err != nil {
		return err
	}

	expr, parseErr := parser.ParseExpr(source.Value)
	if parseErr != nil {
		return werr.WrapForeignErrorf(errGoParseError,
			"go-parse-expr: %s", parseErr)
	}

	opts := &mapperOpts{}
	mc.SetValue(mapNode(expr, opts))
	return nil
}

// PrimGoFormat implements (go-format ast).
// Converts an s-expression AST back to formatted Go source.
func PrimGoFormat(mc machine.CallContext) error {
	astVal := mc.Arg(0)

	n, err := unmapNode(astVal)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()

	// When the s-expression was produced with 'comments, attach comment
	// groups with synthetic positions so go/printer places them correctly.
	file, isFile := n.(*ast.File)
	if isFile {
		fields := sexpFields(astVal)
		_, hasComments := GetField(fields, "comments")
		if hasComments {
			attachErr := attachComments(file, fields, fset)
			if attachErr != nil {
				return werr.WrapForeignErrorf(errMalformedGoAST,
					"go-format: %s", attachErr)
			}
		}
	}

	var buf strings.Builder
	err = printer.Fprint(&buf, fset, n)
	if err != nil {
		return werr.WrapForeignErrorf(errMalformedGoAST,
			"go-format: printer error: %s", err)
	}

	formatted, fmtErr := format.Source([]byte(buf.String()))
	if fmtErr != nil {
		// format.Source can fail on partial ASTs. Return unformatted.
		mc.SetValue(values.NewString(buf.String()))
		return nil //nolint:nilerr // intentional: fall back to unformatted output
	}

	mc.SetValue(values.NewString(string(formatted)))
	return nil
}

// PrimGoNodeType implements (go-node-type ast).
// Returns the tag symbol of an AST node.
func PrimGoNodeType(mc machine.CallContext) error {
	astVal := mc.Arg(0)

	pair, ok := astVal.(*values.Pair)
	if !ok {
		return werr.WrapForeignErrorf(errMalformedGoAST,
			"go-node-type: expected tagged alist, got %T", astVal)
	}
	tagSym, ok := pair.Car().(*values.Symbol)
	if !ok {
		return werr.WrapForeignErrorf(errMalformedGoAST,
			"go-node-type: expected symbol tag, got %T", pair.Car())
	}

	mc.SetValue(values.NewSymbol(tagSym.Key))
	return nil
}

// mapPackage maps a loaded, type-checked package to a (package ...) s-expression node.
// Each file in pkg.Syntax is mapped with type annotations drawn from pkg.TypesInfo.
func mapPackage(pkg *packages.Package, baseOpts *mapperOpts) values.Value {
	opts := &mapperOpts{
		fset:      pkg.Fset,
		positions: baseOpts.positions,
		comments:  baseOpts.comments,
		typeInfo:  pkg.TypesInfo,
		pkgPath:   pkg.PkgPath,
	}
	files := make([]values.Value, len(pkg.Syntax))
	for i, f := range pkg.Syntax {
		files[i] = mapFile(f, opts)
	}
	return Node("package",
		Field("name", Str(pkg.Name)),
		Field("path", Str(pkg.PkgPath)),
		Field("files", ValueList(files)),
	)
}

// PrimGoTypecheckPackage implements (go-typecheck-package [target] . options).
// target is a package pattern string or a GoSession from go-load.
// If not provided, uses (current-go-target).
// Loads a Go package using go/packages (module-aware via go list), type-checks it,
// and returns a list of annotated (package ...) s-expression nodes.
func PrimGoTypecheckPackage(mc machine.CallContext) error {
	mctx, ok := mc.(*machine.MachineContext)
	if !ok {
		return werr.WrapForeignErrorf(errGoPackageLoadError,
			"go-typecheck-package: CallContext is not *MachineContext")
	}
	arg, rest, err := ExtractTargetAndRest(mctx, mc.Arg(0))
	if err != nil {
		return err
	}
	session, ok := UnwrapSession(arg)
	if ok {
		return typecheckFromSessionWithRest(mc, session, rest)
	}
	pat, ok := arg.(*values.String)
	if !ok {
		return werr.WrapForeignErrorf(werr.ErrNotAString,
			"go-typecheck-package: expected string or go-session, got %T", arg)
	}
	return typecheckFromPatternWithRest(mc, pat, rest)
}

func typecheckFromSessionWithRest(mc machine.CallContext, session *GoSession, rest values.Value) error {
	baseOpts, _, optErr := parseOpts(rest, session.FileSet())
	if optErr != nil {
		return optErr
	}
	result := make([]values.Value, len(session.Packages()))
	for i, pkg := range session.Packages() {
		result[i] = mapPackage(pkg, baseOpts)
	}
	mc.SetValue(ValueList(result))
	return nil
}

func typecheckFromPatternWithRest(mc machine.CallContext, pattern *values.String, rest values.Value) error {
	fset := token.NewFileSet()
	baseOpts, _, optErr := parseOpts(rest, fset)
	if optErr != nil {
		return optErr
	}

	pkgs, err := LoadPackagesChecked(mc,
		packages.NeedName|packages.NeedFiles|packages.NeedSyntax|
			packages.NeedTypes|packages.NeedTypesInfo,
		fset, errGoPackageLoadError, "go-typecheck-package",
		pattern.Value)
	if err != nil {
		return err
	}

	result := make([]values.Value, len(pkgs))
	for i, pkg := range pkgs {
		result[i] = mapPackage(pkg, baseOpts)
	}
	mc.SetValue(ValueList(result))
	return nil
}

// PrimInterfaceImplementors implements (go-interface-implementors interface-name target).
// target is a package pattern string or a GoSession from go-load.
// Finds all concrete types implementing the named interface within the loaded packages.
// Returns a tagged alist: (interface-info (name . X) (pkg . Y) (methods . (...)) (implementors . (...))).
func PrimInterfaceImplementors(mc machine.CallContext) error {
	ifaceName, err := helpers.RequireArg[*values.String](mc, 0, werr.ErrNotAString, "go-interface-implementors")
	if err != nil {
		return err
	}

	arg1 := mc.Arg(1)
	session, ok := UnwrapSession(arg1)
	if ok {
		return implementorsFromSession(mc, ifaceName.Value, session)
	}
	pat, ok := arg1.(*values.String)
	if !ok {
		return werr.WrapForeignErrorf(werr.ErrNotAString,
			"go-interface-implementors: expected string or go-session, got %T", arg1)
	}
	return implementorsFromPattern(mc, ifaceName.Value, pat)
}

func implementorsFromSession(mc machine.CallContext, ifaceName string, session *GoSession) error {
	return findImplementors(mc, ifaceName, session.Packages())
}

func implementorsFromPattern(mc machine.CallContext, ifaceName string, pattern *values.String) error {
	pkgs, err := LoadPackagesChecked(mc,
		packages.NeedName|packages.NeedTypes,
		nil, errGoPackageLoadError, "go-interface-implementors",
		pattern.Value)
	if err != nil {
		return err
	}
	return findImplementors(mc, ifaceName, pkgs)
}

func findImplementors(mc machine.CallContext, name string, pkgs []*packages.Package) error {
	qualified := strings.Contains(name, ".")

	type ifaceMatch struct {
		iface   *types.Interface
		pkgPath string
		short   string
	}
	var candidates []ifaceMatch

	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		scope := pkg.Types.Scope()
		for _, n := range scope.Names() {
			obj := scope.Lookup(n)
			tn, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}
			it, ok := tn.Type().Underlying().(*types.Interface)
			if !ok {
				continue
			}
			fullName := pkg.PkgPath + "." + n
			if (qualified && fullName == name) || (!qualified && n == name) {
				candidates = append(candidates, ifaceMatch{
					iface:   it,
					pkgPath: pkg.PkgPath,
					short:   n,
				})
			}
		}
	}

	if len(candidates) == 0 {
		return werr.WrapForeignErrorf(errGoInterfaceNotFound,
			"go-interface-implementors: interface %q not found", name)
	}
	if len(candidates) > 1 {
		var names []string
		for _, c := range candidates {
			names = append(names, c.pkgPath+"."+c.short)
		}
		return werr.WrapForeignErrorf(errGoInterfaceNotFound,
			"go-interface-implementors: ambiguous interface %q: %s", name, strings.Join(names, ", "))
	}

	found := candidates[0]
	if found.iface.NumMethods() == 0 {
		return werr.WrapForeignErrorf(errGoInterfaceNotFound,
			"go-interface-implementors: interface %q has no methods", name)
	}

	// Collect interface method names.
	methods := make([]values.Value, found.iface.NumMethods())
	for i := range found.iface.NumMethods() {
		methods[i] = Str(found.iface.Method(i).Name())
	}

	// Find all concrete types satisfying the interface (T or *T).
	var implementors []values.Value
	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		scope := pkg.Types.Scope()
		for _, n := range scope.Names() {
			obj := scope.Lookup(n)
			tn, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}
			t := tn.Type()
			_, isIface := t.Underlying().(*types.Interface)
			if isIface {
				continue
			}
			ptr := types.NewPointer(t)
			if types.Implements(t, found.iface) || types.Implements(ptr, found.iface) {
				implementors = append(implementors, ValueList([]values.Value{
					Field("type", Str(n)),
					Field("pkg", Str(pkg.PkgPath)),
				}))
			}
		}
	}

	mc.SetValue(Node("interface-info",
		Field("name", Str(found.short)),
		Field("pkg", Str(found.pkgPath)),
		Field("methods", ValueList(methods)),
		Field("implementors", ValueList(implementors)),
	))
	return nil
}
