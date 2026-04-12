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

func TestPathAlgebra_TropicalLinearChain(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B -> C (linear chain) with tropical semiring, unit weight
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
		(define pa (make-path-analysis (tropical-semiring) cg (lambda (_) 1)))`)

	// A to A = 0 (source identity, semiring-one of tropical is 0)
	result := eval(t, engine, `(path-query pa "A" "A")`)
	c.Assert(result.Internal().(*values.Integer).Value, qt.Equals, int64(0))

	// A to B = 1
	result = eval(t, engine, `(path-query pa "A" "B")`)
	c.Assert(result.Internal().(*values.Integer).Value, qt.Equals, int64(1))

	// A to C = 2
	result = eval(t, engine, `(path-query pa "A" "C")`)
	c.Assert(result.Internal().(*values.Integer).Value, qt.Equals, int64(2))

	// C to A = tropical-inf (unreachable)
	result = eval(t, engine, `(eq? (path-query pa "C" "A") tropical-inf)`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestPathAlgebra_TropicalDiamond(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B -> C and A -> C (diamond), unit weight
	eval(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list
					(list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))
					(list 'cg-edge (cons 'caller "A") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))))
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in (list
					(list 'cg-edge (cons 'caller "A") (cons 'callee "C") (cons 'description "static"))
					(list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static"))))
				(cons 'edges-out '()))))
		(define pa (make-path-analysis (tropical-semiring) cg (lambda (_) 1)))`)

	// A to C = 1 (direct path shorter than via B which is 2)
	result := eval(t, engine, `(path-query pa "A" "C")`)
	c.Assert(result.Internal().(*values.Integer).Value, qt.Equals, int64(1))
}

func TestPathAlgebra_CountingDiamond(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// Same diamond: A -> B -> C and A -> C, counting semiring, unit weight
	eval(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list
					(list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))
					(list 'cg-edge (cons 'caller "A") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))))
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in (list
					(list 'cg-edge (cons 'caller "A") (cons 'callee "C") (cons 'description "static"))
					(list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static"))))
				(cons 'edges-out '()))))
		(define pa (make-path-analysis (counting-semiring) cg (lambda (_) 1)))`)

	// A to C = 2 (two distinct paths: direct + via B)
	result := eval(t, engine, `(path-query pa "A" "C")`)
	c.Assert(result.Internal().(*values.Integer).Value, qt.Equals, int64(2))

	// A to B = 1
	result = eval(t, engine, `(path-query pa "A" "B")`)
	c.Assert(result.Internal().(*values.Integer).Value, qt.Equals, int64(1))
}

func TestPathAlgebra_QueryAll(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B -> C (linear chain) with boolean semiring
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

	// From A: all 3 nodes reachable (A, B, C)
	result := eval(t, engine, `(length (path-query-all pa "A"))`)
	c.Assert(result.Internal().(*values.Integer).Value, qt.Equals, int64(3))

	// From C: only C itself reachable
	result = eval(t, engine, `(length (path-query-all pa "C"))`)
	c.Assert(result.Internal().(*values.Integer).Value, qt.Equals, int64(1))
}

func TestPathAlgebra_BooleanVsReachable(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(import (wile goast utils))
		(define cg (go-callgraph "github.com/aalpar/wile-goast/goast" 'static))
		(define pa (make-path-analysis (boolean-semiring) cg #f))`)

	// Use a known function as root — PrimGoParseExpr has outgoing edges in the static call graph.
	eval(t, engine, `
		(define root "github.com/aalpar/wile-goast/goast.PrimGoParseExpr")
		(define go-reach (go-callgraph-reachable cg root))
		(define pa-reach (map car (path-query-all pa root)))`)

	// Every node in go-callgraph-reachable should be reachable via path-algebra.
	result := eval(t, engine, `
		(let loop ((names go-reach))
			(cond
				((null? names) #t)
				((not (path-query pa root (car names)))
				 (car names))
				(else (loop (cdr names)))))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// Both should have the same count.
	result = eval(t, engine, `(= (length go-reach) (length pa-reach))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestPathAlgebra_CustomWeight(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B (weight 3) -> C (weight 5)
	eval(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(import (wile goast utils))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "w3")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "w3"))))
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "w5")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "w5"))))
				(cons 'edges-out '()))))

		;; Weight function uses description to assign edge weights.
		(define (edge-weight e)
			(let ((desc (nf e 'description)))
				(cond ((equal? desc "w3") 3)
				      ((equal? desc "w5") 5)
				      (else 1))))

		(define pa (make-path-analysis (tropical-semiring) cg edge-weight))`)

	// A to B = 3, A to C = 3+5 = 8
	result := eval(t, engine, `(path-query pa "A" "B")`)
	c.Assert(result.Internal().(*values.Integer).Value, qt.Equals, int64(3))

	result = eval(t, engine, `(path-query pa "A" "C")`)
	c.Assert(result.Internal().(*values.Integer).Value, qt.Equals, int64(8))
}

func TestPathAlgebra_Unreachable(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B with isolated C, boolean semiring
	eval(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))))
				(cons 'edges-out '()))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in '())
				(cons 'edges-out '()))))
		(define pa (make-path-analysis (boolean-semiring) cg #f))`)

	// A cannot reach isolated C
	result := eval(t, engine, `(path-query pa "A" "C")`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)

	// Nonexistent node Z
	result = eval(t, engine, `(path-query pa "A" "Z")`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)
}
