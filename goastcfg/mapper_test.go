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

package goastcfg

import (
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/aalpar/wile/pkg/values"

	"github.com/aalpar/wile-goast/goast"

	qt "github.com/frankban/quicktest"
)

func writeTestPackage(t *testing.T, dir, source string) {
	t.Helper()
	c := qt.New(t)
	err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module testpkg\n\ngo 1.23\n"), 0o644)
	c.Assert(err, qt.IsNil)
	err = os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644)
	c.Assert(err, qt.IsNil)
}

func buildSSAFunc(t *testing.T, dir, source, funcName string) (*token.FileSet, *ssa.Function) {
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

	_, ssaPkgs := ssautil.Packages(pkgs, ssa.SanityCheckFunctions)
	for _, p := range ssaPkgs {
		if p != nil {
			p.Build()
		}
	}
	fn := ssaPkgs[0].Func(funcName)
	c.Assert(fn, qt.IsNotNil, qt.Commentf("function %s not found", funcName))
	return fset, fn
}

// buildSSAPackage writes source into dir, loads and builds it, and returns the
// SSA program and first package so a test can call findFunction directly.
// buildSSAFunc cannot serve here: it resolves through ssa.Package.Func, which
// is exactly the member-only lookup under test.
//
// The builder mode is the one both production entry points use
// (cfgFromPattern and goast.GoSession.SSA), InstantiateGenerics included, so a
// generic instance's literals are in scope for the round-trip invariant rather
// than merely absent from the test.
func buildSSAPackage(t *testing.T, dir, source string) (*token.FileSet, *ssa.Program, *ssa.Package) {
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

	prog, ssaPkgs := ssautil.Packages(pkgs, ssa.SanityCheckFunctions|ssa.InstantiateGenerics)
	for _, p := range ssaPkgs {
		if p != nil {
			p.Build()
		}
	}
	c.Assert(len(ssaPkgs) > 0, qt.IsTrue, qt.Commentf("no SSA packages built"))
	c.Assert(ssaPkgs[0], qt.IsNotNil)
	return fset, prog, ssaPkgs[0]
}

// funcLitSource covers every func-literal shape go/ssa constructs differently:
// an iterator body, a call argument, nesting, two siblings in one function, a
// literal inside a method, and the go/defer path (builder.go:2524, a second
// construction site from the one at builder.go:632). RangeFunc adds the Go 1.23
// range-over-func lowering, whose body is an ordinary AnonFuncs entry carrying
// Synthetic == "range-over-func yield". LateUse and Discarded are the two
// degenerate attribution cases: a non-capturing literal first used in a block
// other than the one that writes it, and one that is never referred to at all.
// Generic and UseGeneric put a literal on both sides of an instantiation, which
// only matters because buildSSAPackage builds with InstantiateGenerics as
// production does. BoundMethod yields a synthetic M$bound wrapper, which
// TestMapCFG_SyntheticClosureOmitted asserts is NOT listed.
const funcLitSource = `
package testpkg

import "iter"

type T struct{ n int }

func Spine(xs []int) iter.Seq2[int, int] {
	return func(yield func(int, int) bool) {
		for i, x := range xs {
			if x < 0 {
				continue
			}
			if !yield(i, x) {
				return
			}
		}
	}
}

func Apply(xs []int, f func(int)) {
	for _, x := range xs {
		f(x)
	}
}

func FlipScope(xs []int) int {
	total := 0
	Apply(xs, func(x int) {
		if x > 0 {
			total += x
		} else {
			total -= x
		}
	})
	return total
}

func Nested() func() func() int {
	return func() func() int {
		return func() int {
			return 42
		}
	}
}

func Two() (func() int, func() int) {
	a := func() int {
		return 1
	}
	b := func() int {
		return 2
	}
	return a, b
}

func (p *T) M() func() int {
	return func() int {
		if p.n > 0 {
			return p.n
		}
		return -p.n
	}
}

func GoDefer(ch chan int) {
	defer func() {
		close(ch)
	}()
	go func() {
		ch <- 1
	}()
}

func BoundMethod(t *T) func() func() int {
	return t.M
}

func RangeFunc(seq func(func(int) bool)) int {
	total := 0
	for x := range seq {
		total += x
		if total > 100 {
			break
		}
	}
	return total
}

func LateUse(n int) int {
	f := func() int {
		return 1
	}
	total := 0
	for i := 0; i < n; i++ {
		total += f()
	}
	return total
}

func Discarded() {
	_ = func() int {
		return 7
	}
}

func Generic[E any](xs []E, keep func(E) bool) []E {
	q := make([]E, 0, len(xs))
	Apply(nil, func(int) {
		q = q[:0]
	})
	for _, x := range xs {
		if keep(x) {
			q = append(q, x)
		}
	}
	return q
}

func UseGeneric(xs []int) []int {
	return Generic(xs, func(x int) bool {
		return x > 0
	})
}
`

func listLength(v values.Value) int {
	n := 0
	tuple, ok := v.(values.Tuple)
	if !ok {
		return 0
	}
	for !values.IsEmptyList(tuple) {
		n++
		pair, ok := tuple.(*values.Pair)
		if !ok {
			break
		}
		tuple, ok = pair.Cdr().(values.Tuple)
		if !ok {
			break
		}
	}
	return n
}

func TestMapCFG_LinearFunction(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fset, fn := buildSSAFunc(t, dir, `
package testpkg

func Add(a, b int) int {
	return a + b
}
`, "Add")

	mapper := &cfgMapper{fset: fset}
	result := mapper.mapFunction(fn)

	// Add has one block (entry == exit, no branches).
	c.Assert(listLength(result) >= 1, qt.IsTrue,
		qt.Commentf("expected at least one cfg-block"))

	// First block must be tagged cfg-block.
	first := result.(*values.Pair).Car()
	tag := first.(*values.Pair).Car().(*values.Symbol).Key
	c.Assert(tag, qt.Equals, "cfg-block")

	// Entry block has no predecessors.
	preds, ok := goast.GetField(first.(*values.Pair).Cdr(), "preds")
	c.Assert(ok, qt.IsTrue)
	c.Assert(values.IsEmptyList(preds), qt.IsTrue)

	// Entry block idom is #f.
	idom, ok := goast.GetField(first.(*values.Pair).Cdr(), "idom")
	c.Assert(ok, qt.IsTrue)
	c.Assert(idom, qt.Equals, values.FalseValue)
}

func TestMapCFG_RecoverBlock(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fset, fn := buildSSAFunc(t, dir, `
package testpkg

func SafeDiv(a, b int) (result int) {
	defer func() {
		if r := recover(); r != nil {
			result = -1
		}
	}()
	return a / b
}
`, "SafeDiv")

	c.Assert(fn.Recover, qt.IsNotNil, qt.Commentf("SafeDiv should have a recover block"))

	mapper := &cfgMapper{fset: fset}
	result := mapper.mapFunction(fn)

	// Find the block tagged (recover . #t).
	var recoverBlock values.Value
	tuple := result.(values.Tuple)
	for !values.IsEmptyList(tuple) {
		pair := tuple.(*values.Pair)
		block := pair.Car()
		_, hasRecover := goast.GetField(block.(*values.Pair).Cdr(), "recover")
		if hasRecover {
			recoverBlock = block
			break
		}
		tuple = pair.Cdr().(values.Tuple)
	}
	c.Assert(recoverBlock, qt.IsNotNil, qt.Commentf("expected a block with (recover . #t)"))

	// Recover block has idom=#f (no dominator in the normal CFG).
	idom, ok := goast.GetField(recoverBlock.(*values.Pair).Cdr(), "idom")
	c.Assert(ok, qt.IsTrue)
	c.Assert(idom, qt.Equals, values.FalseValue)
}

func TestMapCFG_BranchingFunction(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fset, fn := buildSSAFunc(t, dir, `
package testpkg

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
`, "Max")

	mapper := &cfgMapper{fset: fset}
	result := mapper.mapFunction(fn)

	// Max has at least 3 blocks: entry, then, merge/return.
	c.Assert(listLength(result) >= 3, qt.IsTrue,
		qt.Commentf("expected at least 3 cfg-blocks for Max"))

	// Every non-entry block should have idom set (not #f).
	tuple := result.(values.Tuple)
	first := true
	for !values.IsEmptyList(tuple) {
		pair := tuple.(*values.Pair)
		block := pair.Car()
		if !first {
			idom, ok := goast.GetField(block.(*values.Pair).Cdr(), "idom")
			c.Assert(ok, qt.IsTrue)
			c.Assert(idom, qt.Not(qt.Equals), values.FalseValue,
				qt.Commentf("non-entry block should have an idom"))
		}
		first = false
		tuple = pair.Cdr().(values.Tuple)
	}
}

func TestFindFunction_FuncLiterals(t *testing.T) {
	dir := t.TempDir()
	_, prog, pkg := buildSSAPackage(t, dir, funcLitSource)

	tcs := []struct {
		name       string
		lookup     string
		wantBlocks int
	}{
		{name: "a/iter-seq2 body literal", lookup: "Spine$1", wantBlocks: 6},
		{name: "b/literal as call argument", lookup: "FlipScope$1", wantBlocks: 4},
		{name: "c/nested literal", lookup: "Nested$1$1", wantBlocks: 1},
		{name: "d/first literal, $N order", lookup: "Two$1", wantBlocks: 1},
		{name: "d/second literal, $N order", lookup: "Two$2", wantBlocks: 1},
		{name: "e/literal in a method", lookup: "M$1", wantBlocks: 3},
		{name: "f/defer literal", lookup: "GoDefer$1", wantBlocks: 1},
		{name: "f/go literal", lookup: "GoDefer$2", wantBlocks: 1},
		{name: "g/range-over-func lowered body", lookup: "RangeFunc$1", wantBlocks: 5},
		{name: "a/qualified form", lookup: "testpkg.Spine$1", wantBlocks: 6},
		{name: "e/qualified method form", lookup: "(*testpkg.T).M$1", wantBlocks: 3},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			c := qt.New(t)
			fn := findFunction(prog, pkg, tc.lookup)
			c.Assert(fn, qt.IsNotNil,
				qt.Commentf("findFunction(%q) returned nil: the literal is unreachable", tc.lookup))
			c.Assert(len(fn.Blocks), qt.Equals, tc.wantBlocks,
				qt.Commentf("block count for %q", tc.lookup))
		})
	}
}

// TestFindFunction_AbsentNames is the regression guard for the obvious wrong
// way to pass TestFindFunction_FuncLiterals: making the walk return something
// for any name. errCFGFuncNotFound must still fire for a genuinely absent one.
func TestFindFunction_AbsentNames(t *testing.T) {
	dir := t.TempDir()
	_, prog, pkg := buildSSAPackage(t, dir, funcLitSource)

	for _, name := range []string{
		"NoSuchFunc",
		"NoSuchFunc$1",
		"Spine$2",
		"Spine$1$1",
		"testpkg.NoSuchFunc$1",
		"(*testpkg.T).NoSuchMethod$1",
		"M$bound",
	} {
		t.Run(name, func(t *testing.T) {
			c := qt.New(t)
			c.Assert(findFunction(prog, pkg, name), qt.IsNil)
		})
	}
}
