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

func TestFCAAlgebra_ConceptRelationship(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: eval is the test helper from prim_goast_test.go that runs
	// Scheme code via the Wile engine. It is not the JavaScript eval().
	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-algebra))

		;; Context: F1={A.x, A.y, B.z}, F2={A.x, A.y, B.z}, F3={A.x, A.y}
		;; Concepts:
		;;   C_top = ({F1,F2,F3}, {A.x, A.y})       — broader extent, smaller intent
		;;   C_bot = ({F1,F2},    {A.x, A.y, B.z})   — smaller extent, larger intent
		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("subconcept relationship", func(t *testing.T) {
		// C_bot <= C_top (smaller extent, larger intent)
		result := eval(t, engine, `
			(let ((c-top (car lat))
			      (c-bot (cadr lat)))
			  (concept-relationship c-bot c-top))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "subconcept")
	})

	t.Run("superconcept relationship", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((c-top (car lat))
			      (c-bot (cadr lat)))
			  (concept-relationship c-top c-bot))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "superconcept")
	})
}

func TestFCAAlgebra_IncomparableRelationship(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-algebra))
		(import (wile goast utils))

		;; Disjoint clusters: no concept subsumes the other
		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y")
		    ("F2" "A.x" "A.y")
		    ("F3" "B.z" "B.w"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("disjoint clusters are incomparable", func(t *testing.T) {
		// filter-map with identity-returning predicate serves as filter
		result := eval(t, engine, `
			(let ((non-trivial
			        (filter-map (lambda (c)
			                      (and (pair? (concept-intent c))
			                           (< (length (concept-extent c)) 3)
			                           c))
			                    lat)))
			  (if (>= (length non-trivial) 2)
			    (concept-relationship (car non-trivial) (cadr non-trivial))
			    'not-enough-concepts))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "incomparable")
	})
}

func TestFCAAlgebra_AlgebraLattice(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-algebra))
		(import (wile algebra lattice))

		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))
		(define concepts (concept-lattice ctx))
		(define alg-lat (concept-lattice->algebra-lattice ctx concepts))
	`)

	t.Run("is a lattice", func(t *testing.T) {
		result := eval(t, engine, `(lattice? alg-lat)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("top has largest extent", func(t *testing.T) {
		result := eval(t, engine, `
			(length (concept-extent (lattice-top alg-lat)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "3")
	})

	t.Run("bottom has smallest extent", func(t *testing.T) {
		result := eval(t, engine, `
			(length (concept-extent (lattice-bottom alg-lat)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "2")
	})

	t.Run("leq works", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((c-top (car concepts))
			      (c-bot (cadr concepts)))
			  (lattice-leq? alg-lat c-bot c-top))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("join of bot with itself is bot", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((c-bot (cadr concepts))
			       (joined (lattice-join alg-lat c-bot c-bot)))
			  (equal? (concept-intent joined) (concept-intent c-bot)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestFCAAlgebra_AnnotatedReport(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-algebra))

		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))
		(define lat (concept-lattice ctx))
		(define xb (cross-boundary-concepts lat))
		(define report (annotated-boundary-report xb lat))
	`)

	t.Run("report has one entry", func(t *testing.T) {
		result := eval(t, engine, `(length report)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("entry has summary", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((entry (car report)))
			  (string? (cdr (assoc 'summary entry))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("entry has subconcept-of", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((entry (car report)))
			  (list? (cdr (assoc 'subconcept-of entry))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("cross-boundary concept is subconcept of the A-only concept", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((entry (car report))
			       (subs (cdr (assoc 'subconcept-of entry))))
			  (pair? subs))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestFCAAlgebra_FullPipeline(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-algebra))
		(import (wile algebra lattice))

		(define s (go-load
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/falseboundary"))
		(define idx (go-ssa-field-index s))
		(define ctx (field-index->context idx 'write-only))
		(define concepts (concept-lattice ctx))
		(define xb (cross-boundary-concepts concepts))
		(define report (annotated-boundary-report xb concepts))
		(define alg-lat (concept-lattice->algebra-lattice ctx concepts))
	`)

	t.Run("algebra lattice from real data", func(t *testing.T) {
		result := eval(t, engine, `(lattice? alg-lat)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("annotated report has entries", func(t *testing.T) {
		result := eval(t, engine, `(>= (length report) 1)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("report entries have relationship annotations", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((entry (car report)))
			  (and (assoc 'subconcept-of entry)
			       (assoc 'superconcept-of entry)
			       (assoc 'incomparable-with entry)
			       #t))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
