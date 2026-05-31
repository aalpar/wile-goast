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
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	qt "github.com/frankban/quicktest"
)

// buildTestProgram loads synthetic Go source from dir and builds its SSA program.
func buildTestProgram(t *testing.T, dir, source string) *ssa.Program {
	t.Helper()
	c := qt.New(t)
	writeTestPackage(t, dir, source)

	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedImports | packages.NeedDeps,
		Fset: fset,
		Dir:  dir,
	}
	pkgs, err := packages.Load(cfg, ".")
	c.Assert(err, qt.IsNil)
	c.Assert(len(pkgs), qt.Not(qt.Equals), 0)

	prog, _ := ssautil.Packages(pkgs, ssa.SanityCheckFunctions|ssa.InstantiateGenerics)
	prog.Build()
	return prog
}

// reachableShortNames does BFS over a *callgraph.Graph from the function whose
// fully-qualified name ends with "."+rootShort, returning the sorted set of
// reachable short function names (the part after the last ".").
func reachableShortNames(g *callgraph.Graph, rootShort string) []string {
	var root *callgraph.Node
	for fn, node := range g.Nodes {
		if fn != nil && shortName(fn.String()) == rootShort {
			root = node
			break
		}
	}
	if root == nil {
		return nil
	}
	seen := map[string]bool{}
	var visit func(n *callgraph.Node)
	visit = func(n *callgraph.Node) {
		if n.Func == nil {
			return
		}
		s := shortName(n.Func.String())
		if seen[s] {
			return
		}
		seen[s] = true
		for _, e := range n.Out {
			visit(e.Callee)
		}
	}
	visit(root)
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func shortName(qualified string) string {
	i := strings.LastIndex(qualified, ".")
	if i < 0 {
		return qualified
	}
	return qualified[i+1:]
}

// TestPreciseCallgraph_ConstantIndexSlice is the core-win test: a constant
// index into a literal []func() resolves to exactly one callee. cha folds in
// every address-taken func() (f2, f3); the precise graph must not.
func TestPreciseCallgraph_ConstantIndexSlice(t *testing.T) {
	c := qt.New(t)
	src := `package p

func f0() {
	t := []func(){f1, f2, f3}
	t[0]()
}
func f1() {}
func f2() {}
func f3() {}
`
	prog := buildTestProgram(t, t.TempDir(), src)

	// Baseline: cha over-approximates (this documents the bug being fixed).
	chaReach := reachableShortNames(cha.CallGraph(prog), "f0")
	c.Assert(chaReach, qt.DeepEquals, []string{"f0", "f1", "f2", "f3"})

	// Precise: the constant index t[0] selects only f1.
	precise := preciseCallGraph(prog)
	c.Assert(reachableShortNames(precise, "f0"), qt.DeepEquals, []string{"f0", "f1"})
}

// TestPreciseCallgraph_DynamicIndex_FallsBack: a non-constant index is not
// statically decidable, so the resolver must NOT prune — it falls back to
// CHA's sound over-approximation (all candidates remain reachable).
func TestPreciseCallgraph_DynamicIndex_FallsBack(t *testing.T) {
	c := qt.New(t)
	src := `package p

func f0(i int) {
	t := []func(){f1, f2, f3}
	t[i]()
}
func f1() {}
func f2() {}
func f3() {}
`
	prog := buildTestProgram(t, t.TempDir(), src)
	precise := preciseCallGraph(prog)
	// i is dynamic → every element stays reachable (== CHA).
	c.Assert(reachableShortNames(precise, "f0"), qt.DeepEquals,
		[]string{"f0", "f1", "f2", "f3"})
}

// TestPreciseCallgraph_EscapeMutation_Sound is the CORE SOUNDNESS PROPERTY.
//
// The backing array escapes f0 into mut(), which reassigns index 0 to f3.
// After the call, t[0]() may invoke EITHER f1 (the literal) or f3 (the
// mutation). Pinning to the literal f1 would DROP the real f3 edge — an
// unsound refinement, strictly worse than CHA's over-approximation. The
// resolver must therefore detect the escape and fall back.
//
// This is the invariant that makes the whole pass trustworthy:
//
//	precise-reachable(root) ⊇ true-reachable(root)   for every program.
//
// The load-bearing branch is the *ssa.Slice / escapesVia case in
// storedFuncAt: disabling it pins t[0] to the literal f1 and drops the f3
// edge, turning this test red (verified).
func TestPreciseCallgraph_EscapeMutation_Sound(t *testing.T) {
	src := `package p

func mut(s []func()) { s[0] = f3 }
func f0() {
	t := []func(){f1, f2, f3}
	mut(t)
	t[0]()
}
func f1() {}
func f2() {}
func f3() {}
`
	c := qt.New(t)
	prog := buildTestProgram(t, t.TempDir(), src)
	preciseReach := reachableShortNames(preciseCallGraph(prog), "f0")

	// The array escapes into mut(), which may rewrite index 0, so t[0]() is
	// NOT statically pinnable. The escape guard (escapesVia, reached from
	// storedFuncAt) must fire and fall back to CHA — keeping f3 reachable.
	c.Assert(preciseReach, qt.DeepEquals, []string{"f0", "f1", "f2", "f3", "mut"})

	// Soundness as a property, not a hand-listed set: the refinement must
	// never drop an edge CHA had. precise-reachable(f0) ⊇ cha-reachable(f0);
	// since precise also ⊆ cha by construction, equality holds when (as here)
	// nothing is prunable.
	c.Assert(preciseReach, qt.DeepEquals, reachableShortNames(cha.CallGraph(prog), "f0"))
}
