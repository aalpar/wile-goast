package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

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
