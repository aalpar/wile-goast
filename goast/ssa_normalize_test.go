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

func TestSSANormalize_CommutativeSort(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast utils) (wile goast ssa-normalize))`)

	t.Run("commutative op swaps when x > y", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((node '(ssa-binop (name . "t0") (op . +) (x . "r5") (y . "r2")
			              (type . "int") (operands "r5" "r2"))))
				(nf (ssa-normalize node) 'x))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r2")
	})

	t.Run("commutative op preserves when x < y", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((node '(ssa-binop (name . "t0") (op . +) (x . "r2") (y . "r5")
			              (type . "int") (operands "r2" "r5"))))
				(nf (ssa-normalize node) 'x))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r2")
	})

	t.Run("non-commutative op preserves order", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((node '(ssa-binop (name . "t0") (op . -) (x . "r5") (y . "r2")
			              (type . "int") (operands "r5" "r2"))))
				(nf (ssa-normalize node) 'x))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r5")
	})
}

func TestSSANormalize_Identity(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast utils) (wile goast ssa-normalize))`)

	t.Run("x + 0 becomes x", func(t *testing.T) {
		result := eval(t, engine, `
			(ssa-normalize
				'(ssa-binop (name . "t0") (op . +) (x . "r1") (y . "0:int")
				  (type . "int") (operands "r1" "0:int")))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r1")
	})

	t.Run("0 + x becomes x", func(t *testing.T) {
		result := eval(t, engine, `
			(ssa-normalize
				'(ssa-binop (name . "t0") (op . +) (x . "0:int") (y . "r1")
				  (type . "int") (operands "0:int" "r1")))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r1")
	})

	t.Run("x * 1 becomes x", func(t *testing.T) {
		result := eval(t, engine, `
			(ssa-normalize
				'(ssa-binop (name . "t0") (op . *) (x . "r3") (y . "1:int")
				  (type . "int") (operands "r3" "1:int")))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r3")
	})

	t.Run("x ^ 0 becomes x", func(t *testing.T) {
		result := eval(t, engine, `
			(ssa-normalize
				'(ssa-binop (name . "t0") (op . ^) (x . "r2") (y . "0:int")
				  (type . "int") (operands "r2" "0:int")))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r2")
	})

	t.Run("float identity skipped (IEEE 754)", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((node '(ssa-binop (name . "t0") (op . +) (x . "r1") (y . "0:float64")
			              (type . "float64") (operands "r1" "0:float64"))))
				(tag? (ssa-normalize node) 'ssa-binop))`)
		c.Assert(result.Internal(), qt.Equals, values.TrueValue)
	})
}

func TestSSANormalize_IdentityBitwiseOr(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast utils) (wile goast ssa-normalize))`)

	t.Run("x | 0 becomes x", func(t *testing.T) {
		result := eval(t, engine, `
			(ssa-normalize
				'(ssa-binop (name . "t0") (op . |\||) (x . "r4") (y . "0:int")
				  (type . "int") (operands "r4" "0:int")))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r4")
	})
}

func TestSSANormalize_Annihilation(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast utils) (wile goast ssa-normalize))`)

	t.Run("x * 0 becomes 0", func(t *testing.T) {
		result := eval(t, engine, `
			(ssa-normalize
				'(ssa-binop (name . "t0") (op . *) (x . "r1") (y . "0:int")
				  (type . "int") (operands "r1" "0:int")))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "0:int")
	})

	t.Run("x & 0 becomes 0", func(t *testing.T) {
		result := eval(t, engine, `
			(ssa-normalize
				'(ssa-binop (name . "t0") (op . &) (x . "r1") (y . "0:int")
				  (type . "int") (operands "r1" "0:int")))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "0:int")
	})

	t.Run("0 * x becomes 0", func(t *testing.T) {
		result := eval(t, engine, `
			(ssa-normalize
				'(ssa-binop (name . "t0") (op . *) (x . "0:int") (y . "r1")
				  (type . "int") (operands "0:int" "r1")))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "0:int")
	})
}

func TestSSANormalize_NonBinopPassthrough(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast utils) (wile goast ssa-normalize))`)

	result := eval(t, engine, `
		(tag? (ssa-normalize '(ssa-alloc (name . "t0") (type . "*int") (heap . #t)))
		      'ssa-alloc)`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestSSANormalize_CustomRuleSet(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast utils) (wile goast ssa-normalize))`)

	t.Run("custom rules via ssa-rule-set", func(t *testing.T) {
		// Use only commutative rule, identity should not fire
		result := eval(t, engine, `
			(let ((rules (ssa-rule-set (ssa-rule-commutative)))
			      (node '(ssa-binop (name . "t0") (op . +) (x . "r1") (y . "0:int")
			               (type . "int") (operands "r1" "0:int"))))
				(tag? (ssa-normalize node rules) 'ssa-binop))`)
		c.Assert(result.Internal(), qt.Equals, values.TrueValue)
	})
}

func TestSSANormalize_ConstantFormats(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast utils) (wile goast ssa-normalize))`)

	t.Run("bare zero constant", func(t *testing.T) {
		result := eval(t, engine, `
			(ssa-normalize
				'(ssa-binop (name . "t0") (op . +) (x . "r1") (y . "0")
				  (type . "int") (operands "r1" "0")))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r1")
	})

	t.Run("typed zero constant uint64", func(t *testing.T) {
		result := eval(t, engine, `
			(ssa-normalize
				'(ssa-binop (name . "t0") (op . +) (x . "r1") (y . "0:uint64")
				  (type . "uint64") (operands "r1" "0:uint64")))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r1")
	})

	t.Run("bare one constant", func(t *testing.T) {
		result := eval(t, engine, `
			(ssa-normalize
				'(ssa-binop (name . "t0") (op . *) (x . "r1") (y . "1")
				  (type . "int") (operands "r1" "1")))`)
		c.Assert(result.Internal().(*values.String).Value, qt.Equals, "r1")
	})
}
