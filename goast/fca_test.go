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
