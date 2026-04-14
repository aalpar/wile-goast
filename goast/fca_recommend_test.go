package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestSplitCandidates_Lattice(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("ProcessRequest" "A.x" "A.y" "B.z" "B.w")
		    ("ConfigOnly" "A.x")
		    ("MetricsOnly" "B.z"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("ProcessRequest is a split candidate", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((splits (split-candidates lat #f)))
			  (and (pair? splits)
			       (let ((first (car splits)))
			         (string=? (cdr (assoc 'function first))
			                   "ProcessRequest"))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("ConfigOnly is not a split candidate", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((splits (split-candidates lat #f)))
			  (let loop ((ss splits))
			    (cond ((null? ss) #t)
			          ((string=? (cdr (assoc 'function (car ss)))
			                     "ConfigOnly") #f)
			          (else (loop (cdr ss))))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("split has intent-disjointness factor", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((splits (split-candidates lat #f))
			       (first (car splits))
			       (factors (cdr (assoc 'factors first))))
			  (assoc 'intent-disjointness factors))`)
		qt.New(t).Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})
}

func TestPareto_Dominates(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast fca-recommend))`)

	t.Run("strictly better on all factors", func(t *testing.T) {
		result := eval(t, engine, `
			(dominates?
			  '((a . 3) (b . 2) (c . #t))
			  '((a . 1) (b . 1) (c . #f)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("equal on all factors is not domination", func(t *testing.T) {
		result := eval(t, engine, `
			(dominates?
			  '((a . 3) (b . 2))
			  '((a . 3) (b . 2)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#f")
	})

	t.Run("better on one worse on another is not domination", func(t *testing.T) {
		result := eval(t, engine, `
			(dominates?
			  '((a . 3) (b . 1))
			  '((a . 1) (b . 3)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#f")
	})

	t.Run("boolean factor ordering", func(t *testing.T) {
		result := eval(t, engine, `
			(dominates?
			  '((a . 3) (b . #t))
			  '((a . 3) (b . #f)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestConceptSignature(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("F1" "A.x" "B.y")
		    ("F2" "A.x")
		    ("F3" "B.y"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("F1 in multiple concepts", func(t *testing.T) {
		result := eval(t, engine, `(length (concept-signature lat "F1"))`)
		qt.New(t).Assert(result.SchemeString(), qt.Not(qt.Equals), "0")
	})

	t.Run("F2 in at least one concept", func(t *testing.T) {
		result := eval(t, engine, `(>= (length (concept-signature lat "F2")) 1)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestIncomparablePairs(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("F1" "A.x" "B.y")
		    ("F2" "A.x")
		    ("F3" "B.y"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("F1 has incomparable pairs", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sig (concept-signature lat "F1")))
			  (length (incomparable-pairs sig)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("F2 has no incomparable pairs", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sig (concept-signature lat "F2")))
			  (length (incomparable-pairs sig)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "0")
	})
}

func TestPareto_Frontier(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast fca-recommend))`)

	t.Run("single item frontier", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((result (pareto-frontier
			                (list (list 'r1 '((a . 3) (b . 2))))
			                '(a b))))
			  (length (cdr (assoc 'frontier result))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("incomparable items both on frontier", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((result (pareto-frontier
			                (list (list 'r1 '((a . 3) (b . 1)))
			                      (list 'r2 '((a . 1) (b . 3))))
			                '(a b))))
			  (length (cdr (assoc 'frontier result))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "2")
	})

	t.Run("dominated item not on frontier", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((result (pareto-frontier
			                (list (list 'r1 '((a . 3) (b . 3)))
			                      (list 'r2 '((a . 1) (b . 1))))
			                '(a b))))
			  (list (length (cdr (assoc 'frontier result)))
			        (length (cdr (assoc 'dominated result)))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(1 1)")
	})

	t.Run("dominated grouped under dominator", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((result (pareto-frontier
			                (list (list 'r1 '((a . 3) (b . 3)))
			                      (list 'r2 '((a . 1) (b . 1))))
			                '(a b))))
			  (let ((dom-groups (cdr (assoc 'dominated result))))
			    (car (car dom-groups))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "r1")
	})
}
