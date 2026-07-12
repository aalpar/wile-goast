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

// Package dispatch is the golden fixture for (wile goast dispatch).
//
// It pins every case the library must classify. Keep it small enough to verify
// by hand; every assertion in goast/dispatch_test.go is anchored to a site here.
package dispatch

// --- One interface, one implementor => class `must` -------------------------

type Single interface{ S() }

type OnlyImpl struct{}

func (OnlyImpl) S() {}

// MustSite: exactly one concrete type flows here, so VTA's sound set is a
// singleton => if this call executes, it calls (OnlyImpl).S. class = must, n = 1.
func MustSite() {
	var x Single = OnlyImpl{}
	x.S()
}

// --- One interface, three implementors, all flowing => class `may`, n = 3 ---

type Multi interface{ M() }

type A struct{}
type B struct{}
type C struct{}

func (A) M() {}
func (B) M() {}
func (C) M() {}

// Decoy implements Multi and IS allocated below — but is never converted to
// Multi, so no Multi value ever holds it. CHA folds it in (it implements the
// interface); VTA must prune it. If `Decoy` appears among MaySite's candidates,
// the library is reporting a CHA bound, not a value trace.
type Decoy struct{}

func (Decoy) M() {}

func MaySite(which int) {
	var ms []Multi
	ms = append(ms, A{}) // implicit: call arg
	var b Multi = B{}    // implicit: var decl
	ms = append(ms, b)
	var c Multi
	c = C{} // implicit: assignment
	ms = append(ms, c)

	_ = Decoy{} // allocated, never converted to Multi — the decoy

	ms[which].M() // n = 3 (A, B, C) — NOT 4
}

// ExplicitConversion is the ONLY form for which ssa.MakeInterface carries a
// valid Pos(). The other three forms above are implicit and yield NoPos, which
// is why the witness needs a fallback chain (see the design doc).
func ExplicitConversion() {
	x := Multi(A{})
	x.M()
}

// --- Generics: the unresolved risk. A witness may be MISSING here; it must
// never be WRONG. ------------------------------------------------------------

type Box[T any] struct{ v T }

func (Box[T]) M() {}

func GenericSite() {
	var x Multi = Box[int]{}
	x.M()
}
