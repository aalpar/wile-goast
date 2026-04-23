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
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"golang.org/x/tools/go/ssa"
)

// firstReturnValue returns the first result operand of the first Return
// instruction in fn.Blocks. Tests use this to locate the value they want
// to narrow without threading names through every fixture.
func firstReturnValue(t *testing.T, fn *ssa.Function) ssa.Value {
	t.Helper()
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			ret, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}
			if len(ret.Results) == 0 {
				continue
			}
			return ret.Results[0]
		}
	}
	t.Fatalf("no return with results found in %s", fn.Name())
	return nil
}

// narrowReturn runs narrow(fn, firstReturnValue(fn)) as a test convenience.
func narrowReturn(t *testing.T, fn *ssa.Function) *narrowResult {
	t.Helper()
	return narrow(fn, firstReturnValue(t, fn))
}

// assertTypeSuffix is stricter than qt.Matches but easier to read than qt.Satisfies.
// Types in test output are `*testpkg.Bar` etc.; we assert the last segment matches.
func assertTypeSuffix(t *testing.T, c *qt.C, got []string, want string) {
	t.Helper()
	c.Assert(got, qt.HasLen, 1)
	c.Assert(strings.HasSuffix(got[0], want), qt.IsTrue,
		qt.Commentf("got type %q, expected suffix %q", got[0], want))
}

func TestNarrowDirectAlloc(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Bar struct{ N int }

func Foo() *Bar {
	return &Bar{N: 42}
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	assertTypeSuffix(t, c, r.Types, ".Bar")
	c.Assert(r.Reasons, qt.HasLen, 0)
}

func TestNarrowBinOpReturn(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Add(a, b int) int {
	return a + b
}
`, "Add")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	c.Assert(r.Types, qt.DeepEquals, []string{"int"})
}

func TestNarrowPhi(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	// Assign to a shared variable before returning to force the SSA
	// compiler to emit a Phi at the join rather than two separate Returns.
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Bar struct{ N int }
type Baz struct{ M int }

func Foo(x int) interface{} {
	var v interface{}
	if x > 0 {
		v = &Bar{}
	} else {
		v = &Baz{}
	}
	return v
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	c.Assert(r.Types, qt.HasLen, 2)
	c.Assert(strings.Contains(r.Types[0], ".Bar"), qt.IsTrue,
		qt.Commentf("expected Bar in %v", r.Types))
	c.Assert(strings.Contains(r.Types[1], ".Baz"), qt.IsTrue,
		qt.Commentf("expected Baz in %v", r.Types))
}

// TestNarrowPhiDAGReconvergence verifies that when two phi edges reach
// the same shared producer, the second edge is not misclassified as a
// cycle. Before the fix, visited was a persistent set; edge 0 marked
// the shared producer, edge 1 saw it as visited and returned 'cycle.
// After the fix, visited is per-path (defer delete on return).
func TestNarrowPhiDAGReconvergence(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	// Both phi edges wrap the SAME parameter into an interface. SSA
	// produces a Phi whose edges both resolve to the same Parameter —
	// a diamond where both paths share the original producer.
	// The shared producer is a concrete-typed parameter (*int), which
	// narrows from type. This test primarily guards against misclassification
	// as "cycle" under DAG reconvergence.
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo(x *int, cond bool) interface{} {
	var v interface{}
	if cond {
		v = x
	} else {
		v = x
	}
	return v
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	c.Assert(r.Types, qt.DeepEquals, []string{"*int"})
	for _, reason := range r.Reasons {
		c.Assert(reason, qt.Not(qt.Equals), "cycle",
			qt.Commentf("DAG reconvergence misclassified as cycle: %v", r.Reasons))
	}
}

func TestNarrowTypeAssert(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Bar struct{ N int }

func Foo(x interface{}) *Bar {
	return x.(*Bar)
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	assertTypeSuffix(t, c, r.Types, ".Bar")
}

func TestNarrowExtractTuple(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo(m map[string]int) int {
	v, _ := m["key"]
	return v
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	c.Assert(r.Types, qt.DeepEquals, []string{"int"})
}

func TestNarrowInterProceduralStaticCall(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Bar struct{ N int }

func helper() interface{} {
	return &Bar{}
}

func Foo() interface{} {
	return helper()
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	assertTypeSuffix(t, c, r.Types, ".Bar")
}

func TestNarrowParameterWidens(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo(x interface{}) interface{} {
	return x
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confWidened)
	c.Assert(r.Reasons, qt.DeepEquals, []string{reasonParameter})
}

// TestNarrowConcreteParameterNarrows locks the concrete-parameter rule
// at narrow.go:172-175. A parameter whose declared type is already
// concrete carries no call-site ambiguity, so narrowing succeeds from
// the declared type alone — no "parameter" widening. Regressions that
// revert to widening all parameters (as pre-concrete-param behavior did)
// would flip this assertion.
func TestNarrowConcreteParameterNarrows(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Bar struct{}

func Foo(x *Bar) *Bar {
	return x
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	c.Assert(r.Types, qt.DeepEquals, []string{"*testpkg.Bar"})
	c.Assert(r.Reasons, qt.HasLen, 0)
}

func TestNarrowInvokeConcreteReturn(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	// An invoke call whose interface-method signature returns a concrete type
	// narrows via the signature alone — the callee's internals can't refine
	// a type that's already concrete. Applies uniformly to static and invoke.
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Stringer interface {
	String() string
}

func Foo(s Stringer) string {
	return s.String()
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	c.Assert(r.Types, qt.DeepEquals, []string{"string"})
}

func TestNarrowInvokeWidensNoImplementors(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	// Invoke call with interface-typed return and NO concrete implementors
	// in the program → widen with "dispatch-no-implementors". Proves the
	// invoke-dispatch path distinguishes "empty implementor set" from the
	// ill-formed-invoke defensive paths.
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Producer interface {
	Produce() interface{}
}

func Foo(p Producer) interface{} {
	return p.Produce()
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confWidened)
	c.Assert(r.Reasons, qt.DeepEquals, []string{reasonDispatchNoImplementors})
}

func TestNarrowInvokeDispatchResolvesImplementor(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	// Invoke on an interface with a concrete in-program implementor whose
	// method returns a concrete type. narrowFromInvokeDispatch enumerates
	// the implementor, resolves the method, and unions over the impl's
	// returns → narrow to the concrete type.
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Box struct{ v *Cat }

type Animal interface {
	Get() interface{}
}

type Cat struct{}

func (c *Cat) Get() interface{} { return c }

func Foo(a Animal) interface{} {
	return a.Get()
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	c.Assert(r.Types, qt.DeepEquals, []string{"*testpkg.Cat"})
}

func TestNarrowInvokeDispatchUnionsMultipleImplementors(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	// Two concrete implementors of the same interface, each returning a
	// different concrete type. Dispatch enumeration unions the types.
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Animal interface {
	Get() interface{}
}

type Cat struct{}
type Dog struct{}

func (c *Cat) Get() interface{} { return c }
func (d *Dog) Get() interface{} { return d }

func Foo(a Animal) interface{} {
	return a.Get()
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	// Union of *Cat and *Dog. Order is set-driven; assert set equality.
	c.Assert(len(r.Types), qt.Equals, 2)
	seen := map[string]bool{}
	for _, tt := range r.Types {
		seen[tt] = true
	}
	c.Assert(seen["*testpkg.Cat"], qt.IsTrue)
	c.Assert(seen["*testpkg.Dog"], qt.IsTrue)
}

// TestNarrowInvokeDispatchSkipsSyntheticWrappers exercises the synthetic
// skip at narrow.go:537. Value-receiver methods generate method set
// entries for both the value type (real method) AND the pointer type
// (auto-generated wrapper marked as Synthetic). Without the skip,
// enumeration walks both; with the skip, only the real method
// contributes — avoiding redundant work and mis-attributed reasons.
//
// The observable contract: dispatch on a value-receiver fixture yields
// exactly one concrete type with no widening reasons. mergeResults'
// type dedup would mask a doubling regression in the Types slice, so
// the stricter check here is that Reasons is empty — a regression that
// walked synthetic wrappers would surface any widenings those walks
// produce.
func TestNarrowInvokeDispatchSkipsSyntheticWrappers(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Result struct{}

type Getter interface {
	Get() interface{}
}

type Holder struct{}

// Value receiver: method set of Holder has Get directly; method set of
// *Holder has a synthetic pointer-to-value wrapper that forwards here.
func (h Holder) Get() interface{} {
	return &Result{}
}

func Use(g Getter) interface{} {
	return g.Get()
}
`, "Use")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	c.Assert(r.Types, qt.DeepEquals, []string{"*testpkg.Result"})
	c.Assert(r.Reasons, qt.HasLen, 0,
		qt.Commentf("reasons should be empty; non-empty signals synthetic wrapper contributed: %v", r.Reasons))
}

func TestNarrowGlobalInitNarrows(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	// Package-level var with declaration RHS gets lowered to a Store in the
	// synthetic init function. narrowFromGlobalInit walks those stores and
	// recovers the concrete stored type.
	fn := buildSSAFromSource(t, dir, `
package testpkg

var G interface{} = 42

func Foo() interface{} {
	return G
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	c.Assert(r.Types, qt.DeepEquals, []string{"int"})
}

func TestNarrowGlobalInitNoStores(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	// Declared-but-uninitialized global: no Store emitted in init, so the
	// walker widens with "global-no-stores".
	fn := buildSSAFromSource(t, dir, `
package testpkg

var G interface{}

func Foo() interface{} {
	return G
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confWidened)
	c.Assert(r.Reasons, qt.Contains, "global-no-stores")
}

func TestNarrowCycleDetected(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	// Two mutually-recursive functions returning interfaces. The first call
	// path widens with 'cycle' when narrowWalk revisits a value already on
	// the stack. buildSSAFromSource builds both functions into the same
	// package, and narrowFromCalleeReturns recurses across them.
	fn := buildSSAFromSource(t, dir, `
package testpkg

func A(n int) interface{} {
	if n > 0 {
		return B(n - 1)
	}
	return 0
}

func B(n int) interface{} {
	if n > 0 {
		return A(n - 1)
	}
	return 1
}
`, "A")

	r := narrowReturn(t, fn)
	// Cross-function recursion: A's return path A→B→A hits the same
	// *ssa.Call (to B) already on the descent stack, producing
	// widened(cycle) on that arm. A's other arm returns MakeInterface
	// over an int constant, narrowing to "int". mergeResults unions:
	// overall widened, types=[int], reasons includes "cycle".
	c.Assert(r.Confidence, qt.Equals, confWidened,
		qt.Commentf("expected widened from cycle detection, got %s", r.Confidence))
	c.Assert(r.Reasons, qt.Contains, reasonCycle,
		qt.Commentf("cycle detection should contribute reason 'cycle', got %v", r.Reasons))
	c.Assert(r.Types, qt.DeepEquals, []string{"int"},
		qt.Commentf("non-cycle arm should narrow to int, got %v", r.Types))
}

// TestNarrowAllocStoreLoad verifies the Alloc-backed local-var pattern:
//
//	var v interface{}
//	if cond { v = &Bar{} } else { v = &Baz{} }
//	return *(&v)  // SSA: UnOp(MUL) reading the Alloc'd pointer
//
// Before PR-2', this pattern widened with reason 'pointer-load' because
// the narrower saw UnOp(MUL) on an interface result and gave up. After
// PR-2', the narrower finds every Store to the Alloc and unions the
// stored value types.
func TestNarrowAllocStoreLoad(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Bar struct{ N int }
type Baz struct{ M int }

func Foo(cond bool) interface{} {
	var v interface{}
	if cond {
		v = &Bar{}
	} else {
		v = &Baz{}
	}
	return v
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow,
		qt.Commentf("expected narrow, got %s (reasons=%v)", r.Confidence, r.Reasons))
	c.Assert(r.Types, qt.HasLen, 2,
		qt.Commentf("expected 2 types, got %v", r.Types))
	c.Assert(strings.Contains(r.Types[0], ".Bar"), qt.IsTrue,
		qt.Commentf("expected Bar in %v", r.Types))
	c.Assert(strings.Contains(r.Types[1], ".Baz"), qt.IsTrue,
		qt.Commentf("expected Baz in %v", r.Types))
}

// TestNarrowAllocNoStores verifies the edge case where a declared-but-
// unassigned local-var is read. SSA still produces an Alloc; we widen
// with a distinct reason so debuggers can tell "no stores found" apart
// from "couldn't find the alloc".
func TestNarrowAllocNoStores(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	// This fixture is contrived — Go's zero-value initialization usually
	// doesn't produce a load of an unassigned local — but it exercises
	// the no-stores branch defensively.
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo() interface{} {
	var v interface{}
	_ = v
	return v
}
`, "Foo")

	r := narrowReturn(t, fn)
	// The Go SSA builder optimizes `var v interface{}; _ = v; return v`
	// to `return nil:interface{}` — the Alloc is elided entirely. So the
	// return walks to a nil Const, not an UnOp(*, Alloc). This is the
	// realistic behavior; a future toolchain that preserves the Alloc
	// would instead hit alloc-no-stores via narrowFromAllocStores.
	c.Assert(r.Confidence, qt.Equals, confWidened)
	c.Assert(r.Reasons, qt.DeepEquals, []string{reasonNilConstant})
}

func TestNarrowMergeResultsEmpty(t *testing.T) {
	c := qt.New(t)
	r := mergeResults(nil)
	c.Assert(r.Confidence, qt.Equals, confNoPaths)
	c.Assert(r.Types, qt.HasLen, 0)
	c.Assert(r.Reasons, qt.HasLen, 0)
}

func TestNarrowMergeResultsWidenedWins(t *testing.T) {
	c := qt.New(t)
	r := mergeResults([]*narrowResult{
		{Types: []string{"*foo.Bar"}, Confidence: confNarrow},
		{Confidence: confWidened, Reasons: []string{"parameter"}},
	})
	c.Assert(r.Confidence, qt.Equals, confWidened)
	c.Assert(r.Types, qt.DeepEquals, []string{"*foo.Bar"})
	c.Assert(r.Reasons, qt.DeepEquals, []string{"parameter"})
}

func TestNarrowMergeResultsAllNoPaths(t *testing.T) {
	c := qt.New(t)
	r := mergeResults([]*narrowResult{
		{Confidence: confNoPaths},
		{Confidence: confNoPaths},
	})
	c.Assert(r.Confidence, qt.Equals, confNoPaths)
}

func TestNarrowMergeResultsDeduplicatesTypes(t *testing.T) {
	c := qt.New(t)
	r := mergeResults([]*narrowResult{
		{Types: []string{"*foo.Bar"}, Confidence: confNarrow},
		{Types: []string{"*foo.Bar", "*foo.Baz"}, Confidence: confNarrow},
	})
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	c.Assert(r.Types, qt.DeepEquals, []string{"*foo.Bar", "*foo.Baz"})
}

func TestNarrowFieldStoreSingleSite(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	// A struct field holds interface. One store site in the program assigns
	// a concrete type to it. Load should narrow to that concrete type.
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Pair struct {
	Car interface{}
}

type Cat struct{}

func NewPair() *Pair {
	p := &Pair{}
	p.Car = &Cat{}
	return p
}

func LoadCar(p *Pair) interface{} {
	return p.Car
}
`, "LoadCar")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	c.Assert(r.Types, qt.DeepEquals, []string{"*testpkg.Cat"})
}

func TestNarrowFieldStoreMultipleSites(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	// Two store sites in different functions write different concrete types
	// into the same struct field. Load should union both.
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Pair struct {
	Car interface{}
}

type Cat struct{}
type Dog struct{}

func NewCatPair() *Pair {
	p := &Pair{}
	p.Car = &Cat{}
	return p
}

func NewDogPair() *Pair {
	p := &Pair{}
	p.Car = &Dog{}
	return p
}

func LoadCar(p *Pair) interface{} {
	return p.Car
}
`, "LoadCar")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	c.Assert(len(r.Types), qt.Equals, 2)
	seen := map[string]bool{}
	for _, tt := range r.Types {
		seen[tt] = true
	}
	c.Assert(seen["*testpkg.Cat"], qt.IsTrue)
	c.Assert(seen["*testpkg.Dog"], qt.IsTrue)
}

func TestNarrowFieldStoreNoStores(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	// Declared-but-never-written field. Load widens with "field-no-stores".
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Pair struct {
	Car interface{}
}

func LoadCar(p *Pair) interface{} {
	return p.Car
}
`, "LoadCar")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confWidened)
	c.Assert(r.Reasons, qt.Contains, "field-no-stores")
}
