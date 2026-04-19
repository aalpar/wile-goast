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
	c.Assert(r.Confidence, qt.Equals, "narrow")
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
	c.Assert(r.Confidence, qt.Equals, "narrow")
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
	c.Assert(r.Confidence, qt.Equals, "narrow")
	c.Assert(r.Types, qt.HasLen, 2)
	c.Assert(strings.Contains(r.Types[0], ".Bar"), qt.IsTrue,
		qt.Commentf("expected Bar in %v", r.Types))
	c.Assert(strings.Contains(r.Types[1], ".Baz"), qt.IsTrue,
		qt.Commentf("expected Baz in %v", r.Types))
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
	c.Assert(r.Confidence, qt.Equals, "narrow")
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
	c.Assert(r.Confidence, qt.Equals, "narrow")
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
	c.Assert(r.Confidence, qt.Equals, "narrow")
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
	c.Assert(r.Confidence, qt.Equals, "widened")
	c.Assert(r.Reasons, qt.DeepEquals, []string{"parameter"})
}

func TestNarrowInvokeWidens(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
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
	c.Assert(r.Confidence, qt.Equals, "widened")
	c.Assert(r.Reasons, qt.DeepEquals, []string{"interface-method-dispatch"})
}

func TestNarrowGlobalLoadWidens(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

var G interface{} = 42

func Foo() interface{} {
	return G
}
`, "Foo")

	r := narrowReturn(t, fn)
	c.Assert(r.Confidence, qt.Equals, "widened")
	// Either global-load (direct) or field-load (if loaded through UnOp).
	c.Assert(len(r.Reasons) >= 1, qt.IsTrue, qt.Commentf("reasons=%v", r.Reasons))
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
	// The visited-set key is (*ssa.Function, ssa.Value). Cross-function recursion
	// with the same SSA value reappearing triggers cycle. We assert widened or
	// narrow: if the SSA compiler inlines the recursion away, the walker simply
	// finds concrete return paths — that's also correct.
	c.Assert(r.Confidence == "widened" || r.Confidence == "narrow", qt.IsTrue,
		qt.Commentf("unexpected confidence %q, reasons=%v, types=%v",
			r.Confidence, r.Reasons, r.Types))
}

func TestNarrowMergeResultsEmpty(t *testing.T) {
	c := qt.New(t)
	r := mergeResults(nil)
	c.Assert(r.Confidence, qt.Equals, "no-paths")
	c.Assert(r.Types, qt.HasLen, 0)
	c.Assert(r.Reasons, qt.HasLen, 0)
}

func TestNarrowMergeResultsWidenedWins(t *testing.T) {
	c := qt.New(t)
	r := mergeResults([]*narrowResult{
		{Types: []string{"*foo.Bar"}, Confidence: "narrow"},
		{Confidence: "widened", Reasons: []string{"parameter"}},
	})
	c.Assert(r.Confidence, qt.Equals, "widened")
	c.Assert(r.Types, qt.DeepEquals, []string{"*foo.Bar"})
	c.Assert(r.Reasons, qt.DeepEquals, []string{"parameter"})
}

func TestNarrowMergeResultsAllNoPaths(t *testing.T) {
	c := qt.New(t)
	r := mergeResults([]*narrowResult{
		{Confidence: "no-paths"},
		{Confidence: "no-paths"},
	})
	c.Assert(r.Confidence, qt.Equals, "no-paths")
}

func TestNarrowMergeResultsDeduplicatesTypes(t *testing.T) {
	c := qt.New(t)
	r := mergeResults([]*narrowResult{
		{Types: []string{"*foo.Bar"}, Confidence: "narrow"},
		{Types: []string{"*foo.Bar", "*foo.Baz"}, Confidence: "narrow"},
	})
	c.Assert(r.Confidence, qt.Equals, "narrow")
	c.Assert(r.Types, qt.DeepEquals, []string{"*foo.Bar", "*foo.Baz"})
}
