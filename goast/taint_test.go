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

	"github.com/aalpar/wile/values"
)

// TestTaint_InterproceduralCanary asserts both halves of the taint-flows
// precision story:
//
//   - Negative (precision): shared-helper graph — A and B each call p through
//     different call sites. taint-flows from A to B returns '() because the
//     only A⇝B path is open₀ close₁ (return to the wrong caller), which the
//     valid-path grammar rejects. Context-insensitive interprocedural
//     reachability would connect A⇝B through p — that false positive is what
//     valid-path taint eliminates.
//
//   - Positive (meaningful flow): matched return-then-call graph — A calls s
//     (site 0) and A calls t (site 1). The path s→A→t is labeled close₀ open₁,
//     which IS a valid path (unmatched close allowed at start, open at end).
//     taint-flows reports ("s" . "t").
func TestTaint_InterproceduralCanary(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast taint))`)

	// Helper: predicate selecting the cg-node whose 'name field equals nm.
	eval(t, engine, `
		(define (is-name nm)
			(lambda (n)
				(equal? (cdr (assq 'name (cdr n))) nm)))`)

	// --- Negative half: shared-helper graph ---
	// A calls p at call-site 0; B calls p at call-site 1.
	// A's edges-out: [(A→p)]; B's edges-out: [(B→p)]; p has no outgoing edges.
	// taint-flows from A to B: the only path A⇝B is open₀ close₁ (wrong-caller
	// return), which the valid-path grammar rejects.  Expect '().
	eval(t, engine, `
		(define cgAB (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list
					(list 'cg-edge (cons 'caller "A") (cons 'callee "p") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in '())
				(cons 'edges-out (list
					(list 'cg-edge (cons 'caller "B") (cons 'callee "p") (cons 'description "static")))))
			(list 'cg-node (cons 'name "p") (cons 'id 2)
				(cons 'edges-in '())
				(cons 'edges-out '()))))`)

	result := eval(t, engine, `(null? (taint-flows cgAB (is-name "A") (is-name "B")))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// --- Positive half: matched return-then-call graph ---
	// A calls s at call-site 0 (open₀: A→s, close₀: s→A).
	// A calls t at call-site 1 (open₁: A→t, close₁: t→A).
	// Path from s to t: s -close₀→ A -open₁→ t = close₀ open₁, a valid path.
	// taint-flows from s to t must report ("s" . "t").
	eval(t, engine, `
		(define cgst (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list
					(list 'cg-edge (cons 'caller "A") (cons 'callee "s") (cons 'description "static"))
					(list 'cg-edge (cons 'caller "A") (cons 'callee "t") (cons 'description "static")))))
			(list 'cg-node (cons 'name "s") (cons 'id 1)
				(cons 'edges-in '())
				(cons 'edges-out '()))
			(list 'cg-node (cons 'name "t") (cons 'id 2)
				(cons 'edges-in '())
				(cons 'edges-out '()))))`)

	result = eval(t, engine, `(taint-flows cgst (is-name "s") (is-name "t"))`)
	flows, ok := result.Internal().(*values.Pair)
	if !ok {
		t.Fatalf("taint-flows positive: expected a non-empty list, got %T: %v", result.Internal(), result.Internal())
	}
	flow, ok := flows.Car().(*values.Pair)
	if !ok {
		t.Fatalf("taint-flows positive: expected a pair (source . sink), got %T", flows.Car())
	}
	srcName, ok := flow.Car().(*values.String)
	if !ok {
		t.Fatalf("taint-flows positive: car of flow is not a string, got %T", flow.Car())
	}
	sinkName, ok := flow.Cdr().(*values.String)
	if !ok {
		t.Fatalf("taint-flows positive: cdr of flow is not a string, got %T", flow.Cdr())
	}
	c.Assert(srcName.Value, qt.Equals, "s")
	c.Assert(sinkName.Value, qt.Equals, "t")
}
