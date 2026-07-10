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

// Tests for goast.Set: the shared membership-only collection.

package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/aalpar/wile-goast/goast"
)

func TestSet_NewSetSeedsMembers(t *testing.T) {
	c := qt.New(t)
	s := goast.NewSet("static", "cha", "rta")

	c.Assert(s.Contains("static"), qt.IsTrue)
	c.Assert(s.Contains("cha"), qt.IsTrue)
	c.Assert(s.Contains("rta"), qt.IsTrue)
	c.Assert(s.Contains("vta"), qt.IsFalse)
	c.Assert(len(s), qt.Equals, 3)
}

func TestSet_EmptyContainsNothing(t *testing.T) {
	c := qt.New(t)

	// Both construction paths yield an empty set that reports no membership
	// and never panics on lookup.
	c.Assert(goast.NewSet[string]().Contains("x"), qt.IsFalse)
	c.Assert(make(goast.Set[string]).Contains("x"), qt.IsFalse)
}

func TestSet_AddThenContains(t *testing.T) {
	c := qt.New(t)
	s := make(goast.Set[string])

	c.Assert(s.Contains("a"), qt.IsFalse)
	s.Add("a")
	c.Assert(s.Contains("a"), qt.IsTrue)
}

func TestSet_Generic(t *testing.T) {
	c := qt.New(t)
	s := goast.NewSet(1, 2, 3)

	c.Assert(s.Contains(2), qt.IsTrue)
	c.Assert(s.Contains(9), qt.IsFalse)
}

// TestSet_AddIsIdempotent encodes the type's central invariant, the one the
// doc comment leans on: Add is a no-op when the element is already present, so
// membership and cardinality are stable under repeated Add. This is also what
// makes NewSet(items...) equivalent to a fresh set with each item Added once
// (the "static literal and runtime-built set share one code path" claim): seed
// duplicates must collapse exactly as repeated Add calls do.
func TestSet_AddIsIdempotent(t *testing.T) {
	c := qt.New(t)

	s := make(goast.Set[string])
	s.Add("dup")
	s.Add("dup")
	c.Assert(s.Contains("dup"), qt.IsTrue)
	c.Assert(len(s), qt.Equals, 1)

	// Seed duplicates collapse through the same Add path NewSet routes through.
	c.Assert(len(goast.NewSet("a", "a", "b")), qt.Equals, 2)
}
