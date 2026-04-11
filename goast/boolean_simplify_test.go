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

// Tests for (wile goast boolean-simplify): boolean normalization for
// belief selectors and Go AST condition expressions.
// The test helper "eval" is a Scheme expression evaluator defined in
// prim_goast_test.go — it runs Scheme code through the Wile engine.

package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// -- Core boolean normalization --

func TestBooleanSimplify_Absorption(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast boolean-simplify))`)

	t.Run("and-or absorption", func(t *testing.T) {
		result := eval(t, engine, `
			(let-values (((norm trace) (boolean-normalize '(and x (or x y)))))
			  norm)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "x")
	})

	t.Run("double negation", func(t *testing.T) {
		result := eval(t, engine, `
			(let-values (((norm trace) (boolean-normalize '(not (not x)))))
			  norm)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "x")
	})

	t.Run("idempotence", func(t *testing.T) {
		result := eval(t, engine, `
			(let-values (((norm trace) (boolean-normalize '(and x x))))
			  norm)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "x")
	})

	t.Run("irreducible stays unchanged", func(t *testing.T) {
		result := eval(t, engine, `
			(let-values (((norm trace) (boolean-normalize '(and x y))))
			  (and (pair? norm) (eq? (car norm) 'and)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("trace records rule name", func(t *testing.T) {
		result := eval(t, engine, `
			(import (wile algebra symbolic))
			(let-values (((norm trace) (boolean-normalize '(not (not x)))))
			  (and (pair? trace)
			       (string? (step-rule-name (car trace)))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestBooleanSimplify_Equivalent(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast boolean-simplify))`)

	t.Run("commuted and is equivalent", func(t *testing.T) {
		result := eval(t, engine, `
			(boolean-equivalent? '(and a b) '(and b a))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("commuted or is equivalent", func(t *testing.T) {
		result := eval(t, engine, `
			(boolean-equivalent? '(or a b) '(or b a))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("different terms are not equivalent", func(t *testing.T) {
		result := eval(t, engine, `
			(boolean-equivalent? '(and a b) '(or a b))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#f")
	})

	t.Run("absorption equivalence", func(t *testing.T) {
		result := eval(t, engine, `
			(boolean-equivalent? '(and x (or x y)) 'x)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

// -- Belief selector projection (Task 2) --

func TestBooleanSimplify_SelectorProjection(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast boolean-simplify))`)

	t.Run("contains-call becomes calls atom", func(t *testing.T) {
		result := eval(t, engine, `
			(selector->symbolic '(contains-call "Lock"))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `(calls "Lock")`)
	})

	t.Run("all-of becomes and", func(t *testing.T) {
		result := eval(t, engine, `
			(selector->symbolic '(all-of (contains-call "Lock") (contains-call "Unlock")))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `(and (calls "Lock") (calls "Unlock"))`)
	})

	t.Run("any-of becomes or", func(t *testing.T) {
		result := eval(t, engine, `
			(selector->symbolic '(any-of (contains-call "Lock") (contains-call "Unlock")))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `(or (calls "Lock") (calls "Unlock"))`)
	})

	t.Run("none-of becomes not-or", func(t *testing.T) {
		result := eval(t, engine, `
			(selector->symbolic '(none-of (contains-call "Lock")))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `(not (calls "Lock"))`)
	})
}

func TestBooleanSimplify_SelectorEquivalence(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast boolean-simplify))`)

	t.Run("commuted all-of selectors are equivalent", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((s1 (selector->symbolic
			            '(all-of (contains-call "Lock") (contains-call "Unlock"))))
			      (s2 (selector->symbolic
			            '(all-of (contains-call "Unlock") (contains-call "Lock")))))
			  (boolean-equivalent? s1 s2))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("different selectors are not equivalent", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((s1 (selector->symbolic '(contains-call "Lock")))
			      (s2 (selector->symbolic '(contains-call "Unlock"))))
			  (boolean-equivalent? s1 s2))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#f")
	})
}

// -- Go AST condition projection (Task 1) --

func TestBooleanSimplify_ASTProjection(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast boolean-simplify))`)

	t.Run("simple comparison", func(t *testing.T) {
		result := eval(t, engine, `
			(ast-condition->symbolic (go-parse-expr "x != nil"))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(neq x nil)")
	})

	t.Run("and operator", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sym (ast-condition->symbolic (go-parse-expr "x != nil && y > 0"))))
			  (car sym))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "and")
	})

	t.Run("or operator", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sym (ast-condition->symbolic (go-parse-expr "x != nil || y > 0"))))
			  (car sym))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "or")
	})

	t.Run("not operator", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sym (ast-condition->symbolic (go-parse-expr "!x"))))
			  (car sym))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "not")
	})
}

func TestBooleanSimplify_ASTNormalization(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast boolean-simplify))`)

	t.Run("redundant condition via absorption", func(t *testing.T) {
		// End-to-end: parse Go expression, project, normalize
		result := eval(t, engine, `
			(let* ((ast (go-parse-expr "x != nil && (x != nil || y > 0)"))
			       (sym (ast-condition->symbolic ast)))
			  (let-values (((norm trace) (boolean-normalize sym)))
			    norm))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(neq x nil)")
	})

	t.Run("double negation elimination", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((ast (go-parse-expr "!!x"))
			       (sym (ast-condition->symbolic ast)))
			  (let-values (((norm trace) (boolean-normalize sym)))
			    norm))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "x")
	})

	t.Run("idempotent and", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((ast (go-parse-expr "x != nil && x != nil"))
			       (sym (ast-condition->symbolic ast)))
			  (let-values (((norm trace) (boolean-normalize sym)))
			    norm))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(neq x nil)")
	})
}
