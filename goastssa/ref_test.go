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
	"testing"

	qt "github.com/frankban/quicktest"
	"golang.org/x/tools/go/ssa"

	"github.com/aalpar/wile/values"
)

func TestFindValueByName(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo(a, b int) int {
	return a + b
}
`, "Foo")

	var sampleName string
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			v, ok := instr.(ssa.Value)
			if ok && v.Name() != "" {
				sampleName = v.Name()
				break
			}
		}
		if sampleName != "" {
			break
		}
	}
	c.Assert(sampleName, qt.Not(qt.Equals), "")

	v, ok := findValueByName(fn, sampleName)
	c.Assert(ok, qt.IsTrue)
	c.Assert(v.Name(), qt.Equals, sampleName)
}

func TestFindValueByNameMissing(t *testing.T) {
	c := qt.New(t)
	fn := &ssa.Function{}
	_, ok := findValueByName(fn, "nonexistent")
	c.Assert(ok, qt.IsFalse)
}

func TestFindValueByNameParameter(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Bar(alpha int, beta string) int {
	return alpha
}
`, "Bar")

	v, ok := findValueByName(fn, "alpha")
	c.Assert(ok, qt.IsTrue)
	c.Assert(v.Name(), qt.Equals, "alpha")
}

func TestWrapUnwrapSSAFunctionRef(t *testing.T) {
	c := qt.New(t)
	fn := &ssa.Function{}
	wrapped := WrapSSAFunctionRef(fn)
	c.Assert(wrapped, qt.IsNotNil)

	got, ok := UnwrapSSAFunctionRef(wrapped)
	c.Assert(ok, qt.IsTrue)
	c.Assert(got, qt.Equals, fn)
}

func TestUnwrapSSAFunctionRef_WrongType(t *testing.T) {
	c := qt.New(t)
	_, ok := UnwrapSSAFunctionRef(nil)
	c.Assert(ok, qt.IsFalse)
	_, ok = UnwrapSSAFunctionRef(values.NewString("not opaque"))
	c.Assert(ok, qt.IsFalse)
}

func TestUnwrapSSAFunctionRef_WrongTag(t *testing.T) {
	c := qt.New(t)
	// Opaque value with the right payload type but a different tag —
	// simulates the sibling GoSession wrapper's failure mode.
	other := values.NewOpaqueValue("some-other-tag", &ssa.Function{})
	_, ok := UnwrapSSAFunctionRef(other)
	c.Assert(ok, qt.IsFalse)
}

func TestUnwrapSSAFunctionRef_CorruptedPayload(t *testing.T) {
	c := qt.New(t)
	// Right tag, wrong payload type. Internal invariant violation; the
	// public API still returns (nil, false) — callers treat all three
	// failure modes the same.
	corrupted := values.NewOpaqueValue(ssaFunctionRefTag, "not a function")
	_, ok := UnwrapSSAFunctionRef(corrupted)
	c.Assert(ok, qt.IsFalse)
}
