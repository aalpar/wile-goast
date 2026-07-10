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

package goastcg

import (
	"go/token"
	"maps"
	"slices"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/callgraph/static"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/aalpar/wile/pkg/machine"
	"github.com/aalpar/wile/pkg/registry/helpers"
	"github.com/aalpar/wile/pkg/values"
	"github.com/aalpar/wile/pkg/werr"

	"github.com/aalpar/wile-goast/goast"
)

var (
	errCGBuild            = werr.NewStaticError("callgraph build error")
	errCGInvalidAlgorithm = werr.NewStaticError("invalid callgraph algorithm")
	errCGNoMainFunction   = werr.NewStaticError("no main function for rta")
)

// callgraphBuilders maps each accepted algorithm symbol to its call graph
// constructor. The map does double duty: its key set is the validation set
// (membership == "is this a known algorithm?") and its values are the dispatch
// table. This collapses what would otherwise be a Set of names kept in sync by
// hand with a switch enumerating the same names -- a hand-unrolled dispatch
// table. See PrimGoCallgraph for validation and dispatchCallgraph for lookup.
// The user-facing "valid algorithms" message derives from these keys too
// (validAlgorithmNames), so no copy of the name list survives to drift.
var callgraphBuilders = map[string]func(*ssa.Program) (*callgraph.Graph, error){
	"static": func(prog *ssa.Program) (*callgraph.Graph, error) {
		return static.CallGraph(prog), nil
	},

	"cha": func(prog *ssa.Program) (*callgraph.Graph, error) {
		return cha.CallGraph(prog), nil
	},

	"rta": func(prog *ssa.Program) (*callgraph.Graph, error) {
		mains := ssautil.MainPackages(prog.AllPackages())
		var roots []*ssa.Function
		for _, m := range mains {
			f := m.Func("main")
			if f != nil {
				roots = append(roots, f)
			}
		}
		if len(roots) == 0 {
			return nil, werr.WrapForeignErrorf(errCGNoMainFunction,
				"go-callgraph: rta requires a main function; use cha or vta for libraries")
		}
		result := rta.Analyze(roots, true)
		return result.CallGraph, nil
	},

	"vta": func(prog *ssa.Program) (*callgraph.Graph, error) {
		initial := cha.CallGraph(prog)
		allFuncs := ssautil.AllFunctions(prog)
		return vta.CallGraph(allFuncs, initial), nil
	},

	"precise": func(prog *ssa.Program) (*callgraph.Graph, error) {
		return preciseCallGraph(prog), nil
	},
}

// validAlgorithmNames is the sorted key set of callgraphBuilders, used to build
// the "valid algorithms" error message deterministically (map order isn't).
var validAlgorithmNames = slices.Sorted(maps.Keys(callgraphBuilders))

// PrimGoCallgraph implements (go-callgraph target algorithm).
// target is a package pattern string or a GoSession from go-load.
// algorithm is a string (canonical); a symbol is also accepted for back-compat,
// keeping the query surface's string convention shared with go-callgraph-callers,
// go-callgraph-callees, and go-cfg.
func PrimGoCallgraph(mc machine.CallContext) error {
	var algorithm string
	switch v := mc.Arg(1).(type) {
	case *values.String:
		algorithm = v.Value
	case *values.Symbol:
		algorithm = v.Key
	default:
		return werr.WrapForeignErrorf(werr.ErrNotAString,
			"go-callgraph: argument 2: expected an algorithm string but got %T", mc.Arg(1))
	}

	if _, ok := callgraphBuilders[algorithm]; !ok {
		return werr.WrapForeignErrorf(errCGInvalidAlgorithm,
			"go-callgraph: algorithm must be one of %s; got %s",
			strings.Join(validAlgorithmNames, ", "), algorithm)
	}

	return goast.DispatchSessionOrPattern(mc.Arg(0), "go-callgraph",
		func(s *goast.GoSession) error { return callgraphFromSession(mc, s, algorithm) },
		func(p *values.String) error { return callgraphFromPattern(mc, p, algorithm) })
}

// callgraphFromSession builds the call graph from an already-loaded GoSession,
// reusing its SSA program and file set, then sets the mapped graph as the result.
func callgraphFromSession(mc machine.CallContext, session *goast.GoSession, algorithm string) error {
	prog := session.SSAAllPackages()

	cg, cgErr := dispatchCallgraph(prog, algorithm)
	if cgErr != nil {
		return cgErr
	}

	mapper := &cgMapper{fset: session.FileSet()}
	mc.SetValue(mapper.mapGraph(cg))
	return nil
}

// callgraphFromPattern loads and type-checks the packages matching pattern,
// builds their SSA program from scratch, then dispatches to the algorithm and
// sets the mapped graph as the result. Used when no GoSession is supplied.
func callgraphFromPattern(mc machine.CallContext, pattern *values.String, algorithm string) error {
	fset := token.NewFileSet()

	pkgs, err := goast.LoadPackagesChecked(mc,
		packages.NeedName|packages.NeedFiles|packages.NeedSyntax|
			packages.NeedTypes|packages.NeedTypesInfo|
			packages.NeedImports|packages.NeedDeps,
		fset, errCGBuild, "go-callgraph",
		pattern.Value)
	if err != nil {
		return err
	}

	prog, _ := ssautil.Packages(pkgs, ssa.SanityCheckFunctions|ssa.InstantiateGenerics)
	for _, pkg := range prog.AllPackages() {
		pkg.Build()
	}

	cg, cgErr := dispatchCallgraph(prog, algorithm)
	if cgErr != nil {
		return cgErr
	}

	mapper := &cgMapper{fset: fset}
	mc.SetValue(mapper.mapGraph(cg))
	return nil
}

// dispatchCallgraph dispatches to the selected call graph algorithm by looking
// its builder up in callgraphBuilders.
func dispatchCallgraph(prog *ssa.Program, algorithm string) (*callgraph.Graph, error) {
	build, ok := callgraphBuilders[algorithm]
	if !ok {
		return nil, werr.WrapForeignErrorf(errCGInvalidAlgorithm,
			"go-callgraph: unknown algorithm %s", algorithm)
	}
	return build(prog)
}

// findCGNode walks a list of cg-node s-expressions and returns the
// node whose "name" field matches the given function name.
// Accepts Form 3 qualified names (exact match) or Form 1 short names
// (suffix match: "." + name or ")." + name at end of node name).
// Returns nil if not found.
func findCGNode(graph values.Value, name string) values.Value {
	isQualified := strings.Contains(name, ".") || strings.Contains(name, "(")

	tuple, ok := graph.(values.Tuple)
	if !ok {
		return nil
	}
	for !values.IsEmptyList(tuple) {
		pair, ok := tuple.(*values.Pair)
		if !ok {
			break
		}
		node := pair.Car()
		np, ok := node.(*values.Pair)
		if !ok {
			goto next
		}
		{
			nameVal, found := goast.GetField(np.Cdr(), "name")
			if found {
				s, ok := nameVal.(*values.String)
				if ok && cgNameMatches(s.Value, name, isQualified) {
					return node
				}
			}
		}
	next:
		tuple, ok = pair.Cdr().(values.Tuple)
		if !ok {
			break
		}
	}
	return nil
}

// cgNameMatches tests whether a CG node name matches the search name.
// Qualified names (Form 3) require exact match. Short names (Form 1)
// match if the node name ends with ".name" or ").name".
func cgNameMatches(nodeName, searchName string, isQualified bool) bool {
	if isQualified {
		return nodeName == searchName
	}
	if nodeName == searchName {
		return true
	}
	nLen := len(nodeName)
	sLen := len(searchName)
	if nLen <= sLen {
		return false
	}
	if nodeName[nLen-sLen:] != searchName {
		return false
	}
	prev := nodeName[nLen-sLen-1]
	return prev == '.' || prev == ')'
}

// PrimGoCallgraphCallers implements (go-callgraph-callers graph func-name).
func PrimGoCallgraphCallers(mc machine.CallContext) error {
	graph := mc.Arg(0)
	funcName, err := helpers.RequireArg[*values.String](mc, 1, werr.ErrNotAString, "go-callgraph-callers")
	if err != nil {
		return err
	}

	node := findCGNode(graph, funcName.Value)
	if node == nil {
		mc.SetValue(values.FalseValue)
		return nil
	}

	np, ok := node.(*values.Pair)
	if !ok {
		mc.SetValue(values.FalseValue)
		return nil
	}
	edgesIn, ok := goast.GetField(np.Cdr(), "edges-in")
	if !ok {
		mc.SetValue(values.FalseValue)
		return nil
	}
	mc.SetValue(edgesIn)
	return nil
}

// PrimGoCallgraphCallees implements (go-callgraph-callees graph func-name).
func PrimGoCallgraphCallees(mc machine.CallContext) error {
	graph := mc.Arg(0)
	funcName, err := helpers.RequireArg[*values.String](mc, 1, werr.ErrNotAString, "go-callgraph-callees")
	if err != nil {
		return err
	}

	node := findCGNode(graph, funcName.Value)
	if node == nil {
		mc.SetValue(values.FalseValue)
		return nil
	}

	np, ok := node.(*values.Pair)
	if !ok {
		mc.SetValue(values.FalseValue)
		return nil
	}
	edgesOut, ok := goast.GetField(np.Cdr(), "edges-out")
	if !ok {
		mc.SetValue(values.FalseValue)
		return nil
	}
	mc.SetValue(edgesOut)
	return nil
}
