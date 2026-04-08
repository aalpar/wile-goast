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

package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestFCA_ContextFromAlist(t *testing.T) {
	engine := newBeliefEngine(t)

	// eval is the test helper defined in prim_goast_test.go —
	// it runs Scheme code via the Wile engine and returns the result.
	eval(t, engine, `
		(import (wile goast fca))

		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))
	`)

	t.Run("3 objects", func(t *testing.T) {
		result := eval(t, engine, `(length (context-objects ctx))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "3")
	})

	t.Run("3 attributes", func(t *testing.T) {
		result := eval(t, engine, `(length (context-attributes ctx))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "3")
	})
}

func TestFCA_ContextAttributesSorted(t *testing.T) {
	engine := newBeliefEngine(t)

	// Provide attributes in non-sorted order to verify sorting.
	result := eval(t, engine, `
		(import (wile goast fca))

		(define ctx (context-from-alist
		  '(("F1" "B.z" "A.x" "A.y")
		    ("F2" "A.y" "B.z" "A.x"))))

		(context-attributes ctx)
	`)
	qt.New(t).Assert(result.SchemeString(), qt.Equals, `("A.x" "A.y" "B.z")`)
}

// ── Task 3: Derivation operators ──────────────────────────

func TestFCA_Intent(t *testing.T) {
	engine := newBeliefEngine(t)

	// Context: F1={A.x,A.y,B.z}, F2={A.x,A.y,B.z}, F3={A.x,A.y}
	eval(t, engine, `
		(import (wile goast fca))

		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))
	`)

	t.Run("intent of all 3", func(t *testing.T) {
		// All 3 share A.x and A.y only.
		result := eval(t, engine, `(intent ctx '("F1" "F2" "F3"))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `("A.x" "A.y")`)
	})

	t.Run("intent of F1 and F2", func(t *testing.T) {
		// F1 and F2 both have all 3 attributes.
		result := eval(t, engine, `(intent ctx '("F1" "F2"))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `("A.x" "A.y" "B.z")`)
	})

	t.Run("intent of empty set", func(t *testing.T) {
		// Vacuous: empty object-set → all attributes.
		result := eval(t, engine, `(intent ctx '())`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `("A.x" "A.y" "B.z")`)
	})
}

func TestFCA_Extent(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))

		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))
	`)

	t.Run("extent of A.x and A.y", func(t *testing.T) {
		// All 3 functions have A.x and A.y.
		result := eval(t, engine, `(extent ctx '("A.x" "A.y"))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `("F1" "F2" "F3")`)
	})

	t.Run("extent of all 3 attrs", func(t *testing.T) {
		// Only F1 and F2 have all 3 attributes.
		result := eval(t, engine, `(extent ctx '("A.x" "A.y" "B.z"))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `("F1" "F2")`)
	})

	t.Run("extent of empty set", func(t *testing.T) {
		// Vacuous: empty attribute-set → all objects.
		result := eval(t, engine, `(extent ctx '())`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `("F1" "F2" "F3")`)
	})
}

// ── Task 4: Concept lattice ──────────────────────────────

func TestFCA_ConceptLattice(t *testing.T) {
	engine := newBeliefEngine(t)

	// Same 3-function context.
	// Expected concepts:
	//   ({F1,F2,F3}, {A.x, A.y})   — all 3 share A.x, A.y
	//   ({F1,F2},    {A.x, A.y, B.z}) — F1+F2 share all three
	eval(t, engine, `
		(import (wile goast fca))

		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))

		(define lattice (concept-lattice ctx))
	`)

	t.Run("2 concepts", func(t *testing.T) {
		result := eval(t, engine, `(length lattice)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "2")
	})

	t.Run("3-attribute intent exists", func(t *testing.T) {
		// One concept should have intent {A.x, A.y, B.z}.
		result := eval(t, engine, `
			(let loop ((cs lattice))
			  (cond ((null? cs) #f)
			        ((= (length (concept-intent (car cs))) 3) #t)
			        (else (loop (cdr cs)))))
		`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestFCA_ConceptLattice_NoCrossing(t *testing.T) {
	engine := newBeliefEngine(t)

	// Disjoint access patterns:
	//   F1, F2: {A.x, A.y}
	//   F3:     {B.z, B.w}
	// Expected concepts:
	//   top:    ({F1,F2,F3}, {})         — closure of {} gives all objects, empty intent
	//   group1: ({F1,F2},   {A.x, A.y})
	//   group2: ({F3},      {B.w, B.z})
	eval(t, engine, `
		(import (wile goast fca))

		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y")
		    ("F2" "A.x" "A.y")
		    ("F3" "B.z" "B.w"))))

		(define lattice (concept-lattice ctx))
	`)

	t.Run("4 concepts", func(t *testing.T) {
		// top({F1,F2,F3}, {}), group1({F1,F2}, {A.x,A.y}),
		// group2({F3}, {B.w,B.z}), bottom({}, {A.x,A.y,B.w,B.z})
		result := eval(t, engine, `(length lattice)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "4")
	})
}
