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

	"github.com/aalpar/wile/values"

	qt "github.com/frankban/quicktest"
)

func TestSSANormalize_Idempotence(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast utils) (wile goast ssa-normalize))`)

	t.Run("x & x becomes x", func(t *testing.T) {
		result := eval(t, engine, `
			(ssa-normalize
				'(ssa-binop (name . "t0") (op . &) (x . "r1") (y . "r1")
				  (type . "int") (operands "r1" "r1")))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r1")
	})

	t.Run("x | x becomes x", func(t *testing.T) {
		result := eval(t, engine, `
			(ssa-normalize
				'(ssa-binop (name . "t0") (op . |\||) (x . "r2") (y . "r2")
				  (type . "int") (operands "r2" "r2")))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r2")
	})

	t.Run("x & y does not collapse", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((node '(ssa-binop (name . "t0") (op . &) (x . "r1") (y . "r2")
			              (type . "int") (operands "r1" "r2"))))
				(tag? (ssa-normalize node) 'ssa-binop))`)
		c.Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("float passthrough (not integer-scoped)", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((node '(ssa-binop (name . "t0") (op . &) (x . "r1") (y . "r1")
			              (type . "float64") (operands "r1" "r1"))))
				(tag? (ssa-normalize node) 'ssa-binop))`)
		c.Assert(result.Internal(), qt.Equals, values.TrueValue)
	})
}

func TestSSANormalize_Absorption(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast utils) (wile goast ssa-normalize))`)

	// Absorption requires NESTED binop terms. Flat SSA (operand strings as
	// names) won't trigger it because the matcher needs the inner operand
	// to be a compound term matching the op2 signature.

	t.Run("x | (x & y) becomes x", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((inner '(ssa-binop (name . "t0") (op . &) (x . "r1") (y . "r2")
			                (type . "int") (operands "r1" "r2")))
			       (outer (list 'ssa-binop
			                    '(name . "t1") '(op . |\||) (cons 'x "r1") (cons 'y inner)
			                    '(type . "int")
			                    (cons 'operands (list "r1" inner)))))
				(ssa-normalize outer))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r1")
	})

	t.Run("x & (x | y) becomes x", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((inner '(ssa-binop (name . "t0") (op . |\||) (x . "r3") (y . "r4")
			                (type . "int") (operands "r3" "r4")))
			       (outer (list 'ssa-binop
			                    '(name . "t1") '(op . &) (cons 'x "r3") (cons 'y inner)
			                    '(type . "int")
			                    (cons 'operands (list "r3" inner)))))
				(ssa-normalize outer))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r3")
	})
}

func TestSSANormalize_Associativity(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast utils) (wile goast ssa-normalize))`)

	// Associativity rewrites (a+b)+c -> a+(b+c). Requires nested binop terms
	// (left operand must itself be a compound term). Flat SSA doesn't exercise
	// this rule; it's exercised by discover-equivalences over theories that
	// synthesize nested candidates. These tests assert the rule is available
	// and integer-scoped — flat and float inputs pass through unchanged.

	t.Run("flat binop passes through unchanged", func(t *testing.T) {
		c := qt.New(t)
		result := eval(t, engine, `
			(let ((node '(ssa-binop (name . "t0") (op . +) (x . "r1") (y . "r2")
			              (type . "int") (operands "r1" "r2"))))
				(equal? (ssa-normalize node (ssa-rule-associativity)) node))`)
		c.Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("non-integer type skipped (IEEE 754)", func(t *testing.T) {
		c := qt.New(t)
		result := eval(t, engine, `
			(let ((node '(ssa-binop (name . "t0") (op . +) (x . "r1") (y . "r2")
			              (type . "float64") (operands "r1" "r2"))))
				(equal? (ssa-normalize node (ssa-rule-associativity)) node))`)
		c.Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	// Positive rewrite: (a+b)+c rotates to right-nested a+(b+c).
	// Structural check: after normalization, x is the leaf "a" and y is
	// a nested ssa-binop whose operands are b and c.
	t.Run("left-nested (a+b)+c rotates to right-nested a+(b+c)", func(t *testing.T) {
		c := qt.New(t)
		result := eval(t, engine, `
			(let* ((inner (list 'ssa-binop (cons 'name "t0") (cons 'op '+)
			                    (cons 'x "a") (cons 'y "b")
			                    (cons 'type "int") (cons 'operands (list "a" "b"))))
			       (outer (list 'ssa-binop (cons 'name "t1") (cons 'op '+)
			                    (cons 'x inner) (cons 'y "c")
			                    (cons 'type "int") (cons 'operands (list inner "c"))))
			       (norm (ssa-normalize outer (ssa-rule-associativity))))
			  (and (equal? (nf norm 'x) "a")
			       (tag? (nf norm 'y) 'ssa-binop)
			       (equal? (nf (nf norm 'y) 'x) "b")
			       (equal? (nf (nf norm 'y) 'y) "c")))`)
		c.Assert(result.Internal(), qt.Equals, values.TrueValue,
			qt.Commentf("expected (a+b)+c to rewrite to a+(b+c) structure"))
	})

	// Equivalence: (a+b)+c and a+(b+c) share a canonical form under the
	// associativity rule. Compares structural shape while ignoring the
	// per-input 'name' bookkeeping.
	t.Run("both bracketings normalize to the same structural layout", func(t *testing.T) {
		c := qt.New(t)
		result := eval(t, engine, `
			(define (same-shape? a b)
			  (cond ((and (pair? a) (pair? b)
			              (eq? (car a) 'ssa-binop) (eq? (car b) 'ssa-binop))
			         (and (equal? (nf a 'op) (nf b 'op))
			              (same-shape? (nf a 'x) (nf b 'x))
			              (same-shape? (nf a 'y) (nf b 'y))))
			        (else (equal? a b))))
			(let* ((lhs-inner (list 'ssa-binop (cons 'name "t0") (cons 'op '+)
			                        (cons 'x "a") (cons 'y "b")
			                        (cons 'type "int") (cons 'operands (list "a" "b"))))
			       (lhs (list 'ssa-binop (cons 'name "t1") (cons 'op '+)
			                  (cons 'x lhs-inner) (cons 'y "c")
			                  (cons 'type "int") (cons 'operands (list lhs-inner "c"))))
			       (rhs-inner (list 'ssa-binop (cons 'name "t2") (cons 'op '+)
			                        (cons 'x "b") (cons 'y "c")
			                        (cons 'type "int") (cons 'operands (list "b" "c"))))
			       (rhs (list 'ssa-binop (cons 'name "t3") (cons 'op '+)
			                  (cons 'x "a") (cons 'y rhs-inner)
			                  (cons 'type "int") (cons 'operands (list "a" rhs-inner))))
			       (lhs-norm (ssa-normalize lhs (ssa-rule-associativity)))
			       (rhs-norm (ssa-normalize rhs (ssa-rule-associativity))))
			  (same-shape? lhs-norm rhs-norm))`)
		c.Assert(result.Internal(), qt.Equals, values.TrueValue,
			qt.Commentf("(a+b)+c and a+(b+c) should share a canonical form"))
	})
}
