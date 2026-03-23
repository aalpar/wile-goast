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
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/registry/helpers"
	"github.com/aalpar/wile/security"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

var (
	errSSABuildError      = werr.NewStaticError("ssa build error")
	errSSAFieldIndexError = werr.NewStaticError("ssa field index error")
)

// parseSSAOpts extracts mapper options from a variadic rest-arg list.
// Returns an error for non-symbol values or unrecognized option names.
func parseSSAOpts(rest values.Value, fset *token.FileSet) (*ssaMapper, error) {
	opts := &ssaMapper{fset: fset}
	tuple, ok := rest.(values.Tuple)
	if !ok {
		return opts, nil
	}
	for !values.IsEmptyList(tuple) {
		pair, ok := tuple.(*values.Pair)
		if !ok {
			break
		}
		s, ok := pair.Car().(*values.Symbol)
		if !ok {
			return nil, werr.WrapForeignErrorf(errSSABuildError,
				"go-ssa-build: options must be symbols, got %T", pair.Car())
		}
		switch s.Key {
		case "positions":
			opts.positions = true
		default:
			return nil, werr.WrapForeignErrorf(errSSABuildError,
				"go-ssa-build: unknown option '%s'; valid options: positions", s.Key)
		}
		cdr, ok := pair.Cdr().(values.Tuple)
		if !ok {
			break
		}
		tuple = cdr
	}
	return opts, nil
}

// PrimGoSSABuild implements (go-ssa-build pattern . options).
func PrimGoSSABuild(mc *machine.MachineContext) error {
	pattern, err := helpers.RequireArg[*values.String](mc, 0, werr.ErrNotAString, "go-ssa-build")
	if err != nil {
		return err
	}

	err = security.CheckWithAuthorizer(mc.Authorizer(), security.AccessRequest{
		Resource: security.ResourceProcess,
		Action:   security.ActionLoad,
		Target:   "go",
	})
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	mapper, err := parseSSAOpts(mc.Arg(1), fset)
	if err != nil {
		return err
	}

	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps,
		Context: mc.Context(),
		Fset:    fset,
	}

	pkgs, loadErr := packages.Load(cfg, pattern.Value)
	if loadErr != nil {
		return werr.WrapForeignErrorf(errSSABuildError,
			"go-ssa-build: %s: %s", pattern.Value, loadErr)
	}

	// Check for package load errors.
	var errs []string
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			errs = append(errs, e.Error())
		}
	}
	if len(errs) > 0 {
		return werr.WrapForeignErrorf(errSSABuildError,
			"go-ssa-build: %s: %s", pattern.Value,
			strings.Join(errs, "; "))
	}

	// Build SSA.
	prog, ssaPkgs := ssautil.Packages(pkgs, ssa.SanityCheckFunctions)
	for _, ssaPkg := range ssaPkgs {
		if ssaPkg != nil {
			ssaPkg.Build()
		}
	}

	// Collect source-level functions from the requested packages.
	var funcs []values.Value
	for _, ssaPkg := range ssaPkgs {
		if ssaPkg == nil {
			continue
		}
		for _, mem := range ssaPkg.Members {
			fn, ok := mem.(*ssa.Function)
			if !ok {
				continue
			}
			if fn.Synthetic != "" {
				continue // skip compiler-generated functions
			}
			funcs = append(funcs, mapper.mapFunction(fn))
		}
		// Collect methods on named types.
		// MethodSet(*T) is a superset of MethodSet(T): it includes both pointer- and
		// value-receiver methods. Iterating only the pointer-receiver set avoids
		// collecting value-receiver methods twice.
		for _, mem := range ssaPkg.Members {
			typ, ok := mem.(*ssa.Type)
			if !ok {
				continue
			}
			mset := prog.MethodSets.MethodSet(types.NewPointer(typ.Type()))
			for sel := range mset.Methods() {
				fn := prog.MethodValue(sel)
				if fn == nil || fn.Synthetic != "" {
					continue
				}
				if fn.Pkg == ssaPkg {
					funcs = append(funcs, mapper.mapFunction(fn))
				}
			}
		}
	}

	mc.SetValue(goast.ValueList(funcs))
	return nil
}

// PrimGoSSAFieldIndex implements (go-ssa-field-index pattern).
// Returns a list of ssa-field-summary nodes with per-function field
// access data (struct type, field name, receiver, read/write mode).
func PrimGoSSAFieldIndex(mc *machine.MachineContext) error {
	pattern, err := helpers.RequireArg[*values.String](mc, 0, werr.ErrNotAString, "go-ssa-field-index")
	if err != nil {
		return err
	}

	err = security.CheckWithAuthorizer(mc.Authorizer(), security.AccessRequest{
		Resource: security.ResourceProcess,
		Action:   security.ActionLoad,
		Target:   "go",
	})
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps,
		Context: mc.Context(),
		Fset:    fset,
	}

	pkgs, loadErr := packages.Load(cfg, pattern.Value)
	if loadErr != nil {
		return werr.WrapForeignErrorf(errSSAFieldIndexError,
			"go-ssa-field-index: %s: %s", pattern.Value, loadErr)
	}

	var errs []string
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			errs = append(errs, e.Error())
		}
	}
	if len(errs) > 0 {
		return werr.WrapForeignErrorf(errSSAFieldIndexError,
			"go-ssa-field-index: %s: %s", pattern.Value,
			strings.Join(errs, "; "))
	}

	_, ssaPkgs := ssautil.Packages(pkgs, ssa.SanityCheckFunctions)
	for _, ssaPkg := range ssaPkgs {
		if ssaPkg != nil {
			ssaPkg.Build()
		}
	}

	var summaries []values.Value
	for _, ssaPkg := range ssaPkgs {
		if ssaPkg == nil {
			continue
		}
		collectFieldSummaries(ssaPkg, &summaries)
	}

	mc.SetValue(goast.ValueList(summaries))
	return nil
}

// collectFieldSummaries iterates all source-level functions in an SSA package
// and appends field-access summaries for functions that access struct fields.
func collectFieldSummaries(ssaPkg *ssa.Package, out *[]values.Value) {
	pkgPath := ssaPkg.Pkg.Path()

	// Package-level functions.
	for _, mem := range ssaPkg.Members {
		fn, ok := mem.(*ssa.Function)
		if !ok || fn.Synthetic != "" {
			continue
		}
		if s := buildFuncSummary(fn, pkgPath); s != nil {
			*out = append(*out, s)
		}
	}

	// Methods on named types.
	prog := ssaPkg.Prog
	for _, mem := range ssaPkg.Members {
		typ, ok := mem.(*ssa.Type)
		if !ok {
			continue
		}
		mset := prog.MethodSets.MethodSet(types.NewPointer(typ.Type()))
		for sel := range mset.Methods() {
			fn := prog.MethodValue(sel)
			if fn == nil || fn.Synthetic != "" || fn.Pkg != ssaPkg {
				continue
			}
			if s := buildFuncSummary(fn, pkgPath); s != nil {
				*out = append(*out, s)
			}
		}
	}
}

// fieldInfo holds extracted data for a single FieldAddr instruction.
type fieldInfo struct {
	structName string
	structPkg  string
	fieldName  string
	recv       string
}

// buildFuncSummary does one pass over a function's instructions to
// collect field accesses. Returns nil if the function accesses no fields.
func buildFuncSummary(fn *ssa.Function, pkgPath string) values.Value {
	fieldAddrs := map[string]fieldInfo{} // register name -> info
	storeTargets := map[string]bool{}    // addr register names that are stored to
	var directReads []fieldInfo          // Field (not FieldAddr) instructions

	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			switch v := instr.(type) {
			case *ssa.FieldAddr:
				structType := typesDeref(v.X.Type())
				sName, sPkg := structTypeName(structType)
				fieldAddrs[v.Name()] = fieldInfo{
					structName: sName,
					structPkg:  sPkg,
					fieldName:  fieldNameAt(structType, v.Field),
					recv:       v.X.Name(),
				}
			case *ssa.Store:
				storeTargets[v.Addr.Name()] = true
			case *ssa.Field:
				structType := typesDeref(v.X.Type())
				sName, sPkg := structTypeName(structType)
				directReads = append(directReads, fieldInfo{
					structName: sName,
					structPkg:  sPkg,
					fieldName:  fieldNameAt(structType, v.Field),
					recv:       v.X.Name(),
				})
			}
		}
	}

	if len(fieldAddrs) == 0 && len(directReads) == 0 {
		return nil
	}

	var accesses []values.Value
	for reg, info := range fieldAddrs {
		mode := "read"
		if storeTargets[reg] {
			mode = "write"
		}
		accesses = append(accesses, fieldAccessNode(info, mode))
	}
	for _, info := range directReads {
		accesses = append(accesses, fieldAccessNode(info, "read"))
	}

	funcName := fn.Name()
	return goast.Node("ssa-field-summary",
		goast.Field("func", goast.Str(funcName)),
		goast.Field("pkg", goast.Str(pkgPath)),
		goast.Field("fields", goast.ValueList(accesses)),
	)
}

// fieldAccessNode builds a tagged alist for a single field access entry.
func fieldAccessNode(info fieldInfo, mode string) values.Value {
	return goast.Node("ssa-field-access",
		goast.Field("struct", goast.Str(info.structName)),
		goast.Field("struct-pkg", goast.Str(info.structPkg)),
		goast.Field("field", goast.Str(info.fieldName)),
		goast.Field("recv", goast.Str(info.recv)),
		goast.Field("mode", goast.Sym(mode)),
	)
}

// structTypeName extracts the short type name and package path from a
// types.Type. Returns ("<anonymous>", "") for unnamed struct types.
func structTypeName(t types.Type) (name, pkg string) {
	named, ok := t.(*types.Named)
	if !ok {
		return "<anonymous>", ""
	}
	obj := named.Obj()
	name = obj.Name()
	if obj.Pkg() != nil {
		pkg = obj.Pkg().Path()
	}
	return name, pkg
}
