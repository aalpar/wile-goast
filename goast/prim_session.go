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
	"go/token"
	"sort"

	"golang.org/x/tools/go/packages"

	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/registry/helpers"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

// PrimGoLoad implements (go-load pattern ... . options).
// Loads Go packages and returns a GoSession for reuse across primitives.
func PrimGoLoad(mc machine.CallContext) error {
	mctx, ok := mc.(*machine.MachineContext)
	if !ok {
		return werr.WrapForeignErrorf(errGoLoadError,
			"go-load: CallContext is not *MachineContext")
	}
	arg, rest, err := ExtractTargetAndRest(mctx, mc.Arg(0))
	if err != nil {
		return err
	}
	first, ok := arg.(*values.String)
	if !ok {
		return werr.WrapForeignErrorf(werr.ErrNotAString,
			"go-load: first arg must be a string, got %T", arg)
	}

	patterns := []string{first.Value}
	lintMode := false

	// Walk the remaining rest list for additional patterns and options.
	tuple, ok := rest.(values.Tuple)
	if ok {
		for !values.IsEmptyList(tuple) {
			pair, pok := tuple.(*values.Pair)
			if !pok {
				break
			}
			switch v := pair.Car().(type) {
			case *values.String:
				patterns = append(patterns, v.Value)
			case *values.Symbol:
				if v.Key == "lint" {
					lintMode = true
				} else {
					return werr.WrapForeignErrorf(errGoLoadError,
						"go-load: unknown option '%s'; valid options: lint", v.Key)
				}
			default:
				return werr.WrapForeignErrorf(errGoLoadError,
					"go-load: expected string or symbol, got %T", pair.Car())
			}
			tuple, ok = pair.Cdr().(values.Tuple)
			if !ok {
				break
			}
		}
	}

	fset := token.NewFileSet()
	mode := packages.NeedName |
		packages.NeedFiles |
		packages.NeedSyntax |
		packages.NeedTypes |
		packages.NeedTypesInfo |
		packages.NeedImports |
		packages.NeedDeps
	if lintMode {
		mode = packages.LoadAllSyntax
	}

	pkgs, err := LoadPackagesChecked(mc, mode, fset,
		errGoLoadError, "go-load", patterns...)
	if err != nil {
		return err
	}

	mc.SetValue(WrapSession(NewGoSession(patterns, pkgs, fset, lintMode)))
	return nil
}

// PrimGoSessionP implements (go-session? v).
func PrimGoSessionP(mc machine.CallContext) error {
	_, ok := UnwrapSession(mc.Arg(0))
	if ok {
		mc.SetValue(values.TrueValue)
	} else {
		mc.SetValue(values.FalseValue)
	}
	return nil
}

// PrimGoListDeps implements (go-list-deps pattern ...).
// Lightweight dependency discovery — returns the transitive closure of
// import paths without type checking or syntax loading.
func PrimGoListDeps(mc machine.CallContext) error {
	first, err := helpers.RequireArg[*values.String](mc, 0, werr.ErrNotAString, "go-list-deps")
	if err != nil {
		return err
	}

	patterns := []string{first.Value}

	// Collect additional patterns from variadic rest.
	tuple, ok := mc.Arg(1).(values.Tuple)
	if ok {
		for !values.IsEmptyList(tuple) {
			pair, pok := tuple.(*values.Pair)
			if !pok {
				break
			}
			sv, sok := pair.Car().(*values.String)
			if !sok {
				return werr.WrapForeignErrorf(errGoLoadError,
					"go-list-deps: expected string, got %T", pair.Car())
			}
			patterns = append(patterns, sv.Value)
			tuple, ok = pair.Cdr().(values.Tuple)
			if !ok {
				break
			}
		}
	}

	pkgs, err := LoadPackagesChecked(mc,
		packages.NeedName|packages.NeedImports,
		nil, errGoLoadError, "go-list-deps",
		patterns...)
	if err != nil {
		return err
	}

	// BFS to collect transitive import paths.
	seen := make(map[string]bool)
	queue := append([]*packages.Package{}, pkgs...)
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		if seen[pkg.PkgPath] {
			continue
		}
		seen[pkg.PkgPath] = true
		for _, imp := range pkg.Imports {
			if !seen[imp.PkgPath] {
				queue = append(queue, imp)
			}
		}
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	result := make([]values.Value, len(paths))
	for i, p := range paths {
		result[i] = Str(p)
	}
	mc.SetValue(ValueList(result))
	return nil
}
