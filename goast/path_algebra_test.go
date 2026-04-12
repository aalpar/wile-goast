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
