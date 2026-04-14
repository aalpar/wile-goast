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

func TestCrossFlowFilter(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))
		(import (wile goast dataflow))

		(define s (go-load
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/funcboundary"))
		(define ssa-funcs (go-ssa-build s))
		(define idx (go-ssa-field-index s))
		(define ctx (field-index->context idx 'write-only))
		(define lat (concept-lattice ctx))
		(define splits (split-candidates lat ssa-funcs))
	`)

	t.Run("ProcessRequest has no cross-flow", func(t *testing.T) {
		result := eval(t, engine, `
			(let loop ((ss splits))
			  (cond ((null? ss) 'not-found)
			        ((string-suffix? ".ProcessRequest"
			           (cdr (assoc 'function (car ss))))
			         (cdr (assoc 'no-cross-flow
			                (cdr (assoc 'factors (car ss))))))
			        (else (loop (cdr ss)))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("ProcessAndRecord has cross-flow", func(t *testing.T) {
		result := eval(t, engine, `
			(let loop ((ss splits))
			  (cond ((null? ss) 'not-found)
			        ((string-suffix? ".ProcessAndRecord"
			           (cdr (assoc 'function (car ss))))
			         (cdr (assoc 'no-cross-flow
			                (cdr (assoc 'factors (car ss))))))
			        (else (loop (cdr ss)))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#f")
	})
}

func TestMergeCandidates(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("ResetSession" "Session.Token" "Session.Expiry")
		    ("ExpireSession" "Session.Token" "Session.Expiry")
		    ("Other" "X.a"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("merge candidate found", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((merges (merge-candidates lat)))
			  (pair? merges))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("merge has intent-overlap factor", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((merges (merge-candidates lat))
			       (first (car merges))
			       (factors (cdr (assoc 'factors first))))
			  (cdr (assoc 'intent-overlap factors)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1.0")
	})

	t.Run("merge functions are ResetSession and ExpireSession", func(t *testing.T) {
		eval(t, engine, `(import (wile goast utils))`)
		result := eval(t, engine, `
			(let* ((merges (merge-candidates lat))
			       (first (car merges))
			       (fns (cdr (assoc 'functions first))))
			  (and (member? "ResetSession" fns)
			       (member? "ExpireSession" fns)))`)
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
