// Copyright 2026 Aaron Alpar
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

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
}
