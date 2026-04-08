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

// ── Task 6: field-index->context bridge ─────────────────

func TestFCA_FieldIndexToContext(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast fca))`)

	eval(t, engine, `
		(define s (go-load
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/falseboundary"))
		(define idx (go-ssa-field-index s))
		(define ctx (field-index->context idx 'write-only))`)

	t.Run("5 functions", func(t *testing.T) {
		result := eval(t, engine, `(length (context-objects ctx))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "5")
	})

	t.Run("qualified attributes", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((attrs (context-attributes ctx)))
			  (and (member "Cache.Entries" attrs)
			       (member "Index.Keys" attrs)
			       #t))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

// ── Task 7: Cross-boundary detection ────────────────────

func TestFCA_CrossBoundaryConcepts(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast fca))`)

	eval(t, engine, `
		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))
		(define lat (concept-lattice ctx))`)

	t.Run("one cross-boundary concept", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((xb (cross-boundary-concepts lat)))
			  (length xb))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("extent is F1 and F2", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((xb (cross-boundary-concepts lat)))
			  (concept-extent (car xb)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `("F1" "F2")`)
	})
}

// ── Task 8: Full pipeline integration ──────────────────

func TestFCA_FullPipeline(t *testing.T) {
	engine := newBeliefEngine(t)

	// Load the falseboundary testdata and run the full pipeline:
	// go-load → go-ssa-field-index → field-index→context → concept-lattice
	// → cross-boundary-concepts → boundary-report
	eval(t, engine, `
		(import (wile goast fca))

		(define s (go-load
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/falseboundary"))
		(define idx (go-ssa-field-index s))
		(define ctx (field-index->context idx 'write-only))
		(define lat (concept-lattice ctx))
		(define xb  (cross-boundary-concepts lat))
		(define rpt (boundary-report xb))
	`)

	t.Run("at least one cross-boundary concept", func(t *testing.T) {
		result := eval(t, engine, `(>= (length xb) 1)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("cross-boundary concept spans Cache and Index", func(t *testing.T) {
		// The boundary report extracts types. Check that at least one
		// report entry spans both Cache and Index.
		result := eval(t, engine, `
			(let loop ((rs rpt))
			  (if (null? rs) #f
			    (let* ((r (car rs))
			           (types (cdr (assoc 'types r))))
			      (if (and (member "Cache" types)
			               (member "Index" types))
			        #t
			        (loop (cdr rs))))))
		`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("cross-boundary concept backed by exactly 3 functions", func(t *testing.T) {
		result := eval(t, engine, `
			(let loop ((rs rpt))
			  (if (null? rs) #f
			    (let* ((r (car rs))
			           (types (cdr (assoc 'types r))))
			      (if (and (member "Cache" types)
			               (member "Index" types))
			        (cdr (assoc 'extent-size r))
			        (loop (cdr rs))))))
		`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "3")
	})

	t.Run("cross-boundary extent contains the 3 expected functions", func(t *testing.T) {
		pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/falseboundary"
		result := eval(t, engine, `
			(let loop ((rs rpt))
			  (if (null? rs) #f
			    (let* ((r (car rs))
			           (types (cdr (assoc 'types r))))
			      (if (and (member "Cache" types)
			               (member "Index" types))
			        (let ((fns (cdr (assoc 'functions r))))
			          (and (member "`+pkg+`.UpdateBoth" fns)
			               (member "`+pkg+`.Invalidate" fns)
			               (member "`+pkg+`.Rebuild" fns)
			               #t))
			        (loop (cdr rs))))))
		`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("boundary report has correct structure", func(t *testing.T) {
		// Each report alist must have types, fields, functions, extent-size keys.
		result := eval(t, engine, `
			(let loop ((rs rpt))
			  (if (null? rs) #f
			    (let* ((r (car rs))
			           (types (cdr (assoc 'types r))))
			      (if (and (member "Cache" types)
			               (member "Index" types))
			        (and (assoc 'types r)
			             (assoc 'fields r)
			             (assoc 'functions r)
			             (assoc 'extent-size r)
			             #t)
			        (loop (cdr rs))))))
		`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("boundary report functions are fully qualified", func(t *testing.T) {
		// Verify functions are import-path qualified, not short names.
		result := eval(t, engine, `
			(let loop ((rs rpt))
			  (if (null? rs) #f
			    (let* ((r (car rs))
			           (types (cdr (assoc 'types r))))
			      (if (and (member "Cache" types)
			               (member "Index" types))
			        (let ((fns (cdr (assoc 'functions r))))
			          ;; Short names must NOT appear (they're import-path qualified).
			          (and (not (member "UpdateBoth" fns))
			               (not (member "Invalidate" fns))
			               (not (member "Rebuild" fns))
			               #t))
			        (loop (cdr rs))))))
		`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestFCA_CrossBoundaryMinExtent(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast fca))`)

	eval(t, engine, `
		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))
		(define lat (concept-lattice ctx))`)

	t.Run("min-extent 3 filters it out", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((xb (cross-boundary-concepts lat 'min-extent 3)))
			  (length xb))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "0")
	})
}
