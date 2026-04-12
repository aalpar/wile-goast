package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/aalpar/wile/values"
)

func TestPathAlgebra_TypePredicate(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast path-algebra))`)

	// Construct a trivial CG: single node, no edges.
	eval(t, engine, `
		(define cg (list (list 'cg-node
			(cons 'name "A") (cons 'id 0)
			(cons 'edges-in '()) (cons 'edges-out '()))))`)

	eval(t, engine, `
		(import (wile algebra semiring))
		(define pa (make-path-analysis (boolean-semiring) cg #f))`)

	result := eval(t, engine, `(path-analysis? pa)`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	result = eval(t, engine, `(path-analysis? 42)`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)
}

func TestPathAlgebra_BooleanLinearChain(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B -> C (linear chain)
	eval(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))))
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static"))))
				(cons 'edges-out '()))))
		(define pa (make-path-analysis (boolean-semiring) cg #f))`)

	// A reaches C
	result := eval(t, engine, `(path-query pa "A" "C")`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// A reaches B
	result = eval(t, engine, `(path-query pa "A" "B")`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// A reaches A (self)
	result = eval(t, engine, `(path-query pa "A" "A")`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// C does not reach A
	result = eval(t, engine, `(path-query pa "C" "A")`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)
}
