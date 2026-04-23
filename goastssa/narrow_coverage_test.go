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

// Targeted tests for narrow.go paths that are hard to hit via typical
// fixtures: narrowFromAllocStores (Alloc+Store+Load pattern), the
// non-Alloc arms of narrowPointerLoad, and the rarer narrowWalk branches
// (FreeVar, Builtin, Function reference). These exist because the Go
// SSA optimizer lowers many "textbook" patterns to Phi nodes, bypassing
// the pointer-load path entirely — TestNarrowAllocStoreLoad in
// narrow_test.go is one such case (it passes, but via narrowFromPhi).

package goastssa

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// TestNarrowAllocViaAddressEscape forces Alloc+Store+Load by taking
// the address of a local and passing it to a function. Escape analysis
// ensures the local isn't register-promoted, so SSA emits the
// pointer-load pattern that narrowPointerLoad → narrowFromAllocStores
// actually targets.
func TestNarrowAllocViaAddressEscape(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Bar struct{ N int }

func assign(p *interface{}, v interface{}) { *p = v }

func Foo() interface{} {
	var v interface{}
	assign(&v, &Bar{N: 1})
	return v
}
`, "Foo")

	r := narrowReturn(t, fn)
	// Outer passes &v to assign() — the Store of the concrete value
	// happens inside the callee, not at alloc.Parent(). narrowPointerLoad
	// → narrowFromAllocStores scans the current function only, so no
	// matching store exists; we widen with "alloc-no-stores". This is
	// the documented conservative-correctness trade-off — an escape
	// through a callee is out of scope for alloc-store search.
	c.Assert(r.Confidence, qt.Equals, confWidened,
		qt.Commentf("expected widened from alloc-no-stores; got %s reasons=%v types=%v",
			r.Confidence, r.Reasons, r.Types))
	c.Assert(r.Reasons, qt.DeepEquals, []string{reasonAllocNoStores})
}

// TestNarrowPointerLoadIndexAddr exercises the slice-element pointer
// dereference arm of narrowPointerLoad. Returning *&slice[i] for an
// interface-typed slice forces UnOp(*, IndexAddr) in SSA.
func TestNarrowPointerLoadIndexAddr(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Load(xs []interface{}, i int) interface{} {
	return xs[i]
}
`, "Load")

	r := narrowReturn(t, fn)
	// This path should widen with slice-deref-load or a similar
	// pointer-load reason; the important coverage point is that
	// narrowPointerLoad executed.
	c.Assert(r.Confidence, qt.Equals, confWidened)
	c.Assert(len(r.Reasons), qt.Not(qt.Equals), 0)
}

// TestNarrowPointerLoadFieldAddr ensures the FieldAddr arm of
// narrowPointerLoad runs. An interface-typed struct field accessed
// via *&p.Field produces UnOp(*, FieldAddr) in SSA.
func TestNarrowPointerLoadFieldAddr(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

type Holder struct {
	V interface{}
}

func Load(h *Holder) interface{} {
	return h.V
}
`, "Load")

	r := narrowReturn(t, fn)
	// Without any Store sites for Holder.V in the program, the
	// field-store search returns field-no-stores. Any widened reason
	// is acceptable; the coverage goal is to execute the FieldAddr arm.
	c.Assert(r.Confidence, qt.Equals, confWidened)
}

// TestNarrowWalkBuiltinCall covers the narrowFromCall → Builtin arm
// by invoking a builtin whose interface-typed result flows through.
// len() returns int (concrete) so it goes through narrowFromConcreteType,
// but `make(chan T)` exercises MakeChan which has its own narrowWalk arm.
func TestNarrowWalkMakeChan(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo() chan int {
	return make(chan int, 10)
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
	c.Assert(len(r.Types), qt.Equals, 1)
}

// TestNarrowWalkMakeMap covers the *ssa.MakeMap arm of narrowWalk.
func TestNarrowWalkMakeMap(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo() map[string]int {
	return make(map[string]int)
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
}

// TestNarrowWalkSlice covers the *ssa.Slice arm of narrowWalk by
// returning a reslicing expression.
func TestNarrowWalkSlice(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo(xs []int) []int {
	return xs[1:]
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
}

// TestNarrowWalkFunctionReference covers the *ssa.Function arm of
// narrowWalk — returning a function value produces an SSA Function
// reference, not a closure.
func TestNarrowWalkFunctionReference(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func target(x int) int { return x + 1 }

func Foo() func(int) int {
	return target
}
`, "Foo")

	r := narrowReturn(t, fn)
	// func(int) int is concrete → narrow expected.
	c.Assert(r.Confidence, qt.Equals, confNarrow)
}

// TestNarrowWalkClosureFreeVar covers FreeVar. A closure capturing
// an interface-typed variable accesses it via *ssa.FreeVar, which
// narrowWalk handles with widened("free-var"). We locate the anon
// function via Outer.AnonFuncs instead of by-name to avoid depending
// on SSA's anon-function naming convention.
func TestNarrowWalkClosureFreeVar(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	outer := buildSSAFromSource(t, dir, `
package testpkg

type Bar struct{}

func Outer() func() interface{} {
	var v interface{} = &Bar{}
	return func() interface{} {
		return v
	}
}
`, "Outer")

	c.Assert(len(outer.AnonFuncs), qt.Not(qt.Equals), 0,
		qt.Commentf("Outer should have at least one anonymous function"))
	closure := outer.AnonFuncs[0]

	r := narrow(closure, firstReturnValue(t, closure))
	// Closure body is `return v` where v is a FreeVar captured from
	// Outer. SSA lowers this to UnOp(*, FreeVar). narrowPointerLoad has
	// no FreeVar arm, so it falls through to the residual "pointer-load"
	// bucket. A regression that re-adds FreeVar handling should flip
	// this assertion — intentionally so.
	c.Assert(r.Confidence, qt.Equals, confWidened,
		qt.Commentf("expected widened from pointer-load; got %s reasons=%v", r.Confidence, r.Reasons))
	c.Assert(r.Reasons, qt.DeepEquals, []string{reasonPointerLoad})
}

// TestNarrowWalkRangeNext exercises the *ssa.Next / *ssa.Range arms
// of narrowWalk. Iterating over a map produces Range+Next in SSA.
func TestNarrowWalkRangeNext(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo(m map[string]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
}

// TestNarrowWalkIndexLookup covers the *ssa.Index (array/slice) and
// *ssa.Lookup (map) arms with non-interface results.
func TestNarrowWalkIndexArray(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo(arr [3]int) int {
	return arr[1]
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
}

func TestNarrowWalkMapLookupConcrete(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo(m map[string]int) int {
	return m["key"]
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
}

// TestNarrowWalkMapLookupInterface exercises the interface-Lookup
// arm (widens with "map-lookup").
func TestNarrowWalkMapLookupInterface(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo(m map[string]interface{}) interface{} {
	return m["key"]
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confWidened)
	c.Assert(r.Reasons, qt.Contains, "map-lookup")
}

// TestNarrowWalkUnOpNegation covers the non-interface UnOp arm
// (bitwise NOT, arithmetic negation). These lower to concrete-typed
// UnOp instructions.
func TestNarrowWalkUnOpNegation(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo(x int) int {
	return -x
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
}

// TestNarrowWalkUnOpChannelRecv covers UnOp(ARROW) for channel receive
// where the received value is interface-typed — widens with
// "channel-receive".
func TestNarrowWalkUnOpChannelRecv(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo(ch chan interface{}) interface{} {
	return <-ch
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confWidened)
	c.Assert(r.Reasons, qt.Contains, "channel-receive")
}

// TestNarrowWalkConstNumeric covers the *ssa.Const (non-nil) arm.
func TestNarrowWalkConstNumeric(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo() int {
	return 42
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, confNarrow)
}
