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

	"github.com/aalpar/wile/pkg/values"
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

func TestPathAlgebra_FastPathOnBigintCounting(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// bigint-counting-semiring + #f weight-fn attaches the fast path.
	eval(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in '())
				(cons 'edges-out '()))))
		(define pa-fast (make-path-analysis (bigint-counting-semiring) cg #f))
		(define pa-slow (make-path-analysis (boolean-semiring) cg #f))`)

	result := eval(t, engine, `(path-analysis-fast-path? pa-fast)`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	result = eval(t, engine, `(path-analysis-fast-path-kind pa-fast)`)
	c.Assert(result.Internal().(*values.Symbol).Key, qt.Equals, "bigint-counting")

	// Boolean semiring does not get the fast path.
	result = eval(t, engine, `(path-analysis-fast-path? pa-slow)`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)

	result = eval(t, engine, `(path-analysis-fast-path-kind pa-slow)`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)
}

func TestPathAlgebra_CyclicNodesOnAcyclic(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B -> C (acyclic): no nodes in cycles.
	eval(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in '())
				(cons 'edges-out '()))))
		(define pa (make-path-analysis (bigint-counting-semiring) cg #f))`)

	result := eval(t, engine, `(null? (path-cyclic-nodes pa))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	result = eval(t, engine, `(path-node-in-cycle? pa "A")`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)
}

func TestPathAlgebra_CyclicNodesOnMutualRecursion(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B -> A (mutually recursive 2-cycle); C is non-recursive but reachable from B.
	eval(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in '())
				(cons 'edges-out (list
					(list 'cg-edge (cons 'caller "B") (cons 'callee "A") (cons 'description "static"))
					(list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in '())
				(cons 'edges-out '()))))
		(define pa (make-path-analysis (bigint-counting-semiring) cg #f))`)

	// A and B are mutually recursive.
	result := eval(t, engine, `(path-node-in-cycle? pa "A")`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	result = eval(t, engine, `(path-node-in-cycle? pa "B")`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// C is not in a cycle.
	result = eval(t, engine, `(path-node-in-cycle? pa "C")`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)

	// path-cyclic-nodes should report both A and B (and only them).
	result = eval(t, engine, `
		(let ((cyc (path-cyclic-nodes pa)))
			(and (= (length cyc) 2)
			     (and (member "A" cyc) #t)
			     (and (member "B" cyc) #t)))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestPathAlgebra_SccsOnRealCallgraph(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// Real call graph from the wile-goast goast package. The unmapper
	// dispatch table contains mutual recursion, so cyclic-nodes must be
	// non-empty — corroborates the SCC computation traversed the real
	// adjacency rather than short-circuiting on an empty graph.
	eval(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra graph))
		(import (wile algebra semiring))
		(define cg (go-callgraph "github.com/aalpar/wile-goast/goast" 'static))
		(define pa (make-path-analysis (bigint-counting-semiring) cg #f))`)

	result := eval(t, engine, `(graph-scc? (path-analysis-sccs pa))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	result = eval(t, engine, `(list? (path-cyclic-nodes pa))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	result = eval(t, engine, `(> (length (path-cyclic-nodes pa)) 0)`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoCallgraphReachable_Scheme(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B -> C chain, built as a synthetic cg-node list (no go-callgraph needed).
	eval(t, engine, `
		(import (wile goast path-algebra))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0) (cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1) (cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2) (cons 'edges-in '()) (cons 'edges-out '()))))`)

	// Reachable from A is the sorted set {A,B,C}, root included.
	result := eval(t, engine, `(equal? (go-callgraph-reachable cg "A") (list "A" "B" "C"))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// A leaf reaches only itself.
	result = eval(t, engine, `(equal? (go-callgraph-reachable cg "C") (list "C"))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// A root absent from the graph returns the empty set (not the seeded root).
	result = eval(t, engine, `(null? (go-callgraph-reachable cg "Z"))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoCallgraphReaching_Scheme(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B -> C chain. go-callgraph-reaching reads edges-in/caller (the
	// transpose), so unlike the reachable test the synthetic nodes populate
	// edges-in here.
	eval(t, engine, `
		(import (wile goast path-algebra))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0) (cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B"))))
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C"))))
				(cons 'edges-out '()))))`)

	// Everything reaches C: sorted set {A,B,C}, target included.
	result := eval(t, engine, `(equal? (go-callgraph-reaching cg "C") (list "A" "B" "C"))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// The chain root is reached only by itself: {A}.
	result = eval(t, engine, `(equal? (go-callgraph-reaching cg "A") (list "A"))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// B is reached by A and itself: {A,B}.
	result = eval(t, engine, `(equal? (go-callgraph-reaching cg "B") (list "A" "B"))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// A target absent from the graph returns the empty set (not the seeded target).
	result = eval(t, engine, `(null? (go-callgraph-reaching cg "Z"))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoCallgraphReachable_RealGraph(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// Build a real static call graph, then query reachability through the
	// boolean-semiring path (delegates to wile's graph-query-all).
	eval(t, engine, `
		(import (wile goast path-algebra))
		(define cg (go-callgraph "github.com/aalpar/wile-goast/goast" 'static))`)

	// Reachable from a known function: non-empty, strings, includes the root.
	result := eval(t, engine, `
		(let ((r (go-callgraph-reachable cg "github.com/aalpar/wile-goast/goast.PrimGoParseExpr")))
			(and (pair? r) (string? (car r)) (member "github.com/aalpar/wile-goast/goast.PrimGoParseExpr" r) #t))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// A root absent from the graph returns the empty set.
	result = eval(t, engine, `(null? (go-callgraph-reachable cg "does.not.Exist"))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}
