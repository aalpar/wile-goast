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

func TestUnify_ASTDiffIdentical(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast unify))`)
	result := eval(t, engine, `
		(let* ((node '(func-decl (name . "Foo") (type . (func-type (params) (results)))))
		       (r (ast-diff node node)))
			(diff-result-similarity r))`)
	// Identical nodes should have similarity 1.0
	f, ok := result.Internal().(*values.Float)
	c.Assert(ok, qt.IsTrue)
	c.Assert(f.Value, qt.Equals, 1.0)
}

func TestUnify_ASTDiffTypeDiff(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast unify))`)
	result := eval(t, engine, `
		(let* ((a '(func-decl (name . "Foo") (type . "intA")))
		       (b '(func-decl (name . "Foo") (type . "intB")))
		       (r (ast-diff a b))
		       (diffs (diff-result-diffs r)))
			(caar diffs))`)
	// Type field diff should be classified as 'type-name
	sym, ok := result.Internal().(*values.Symbol)
	c.Assert(ok, qt.IsTrue)
	c.Assert(sym.Key, qt.Equals, "type-name")
}

func TestUnify_SSADiffIdentical(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast unify))`)
	result := eval(t, engine, `
		(let* ((node '(ssa-binop (name . "t0") (op . +) (x . "r1") (y . "r2")
		               (type . "int")))
		       (r (ssa-diff node node)))
			(diff-result-similarity r))`)
	f, ok := result.Internal().(*values.Float)
	c.Assert(ok, qt.IsTrue)
	c.Assert(f.Value, qt.Equals, 1.0)
}

func TestUnify_SSADiffTypeName(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast unify))`)
	result := eval(t, engine, `
		(let* ((a '(ssa-binop (name . "t0") (op . +) (x . "r1") (y . "r2")
		            (type . "int")))
		       (b '(ssa-binop (name . "t0") (op . +) (x . "r1") (y . "r2")
		            (type . "int64")))
		       (r (ssa-diff a b))
		       (diffs (diff-result-diffs r)))
			(caar diffs))`)
	sym, ok := result.Internal().(*values.Symbol)
	c.Assert(ok, qt.IsTrue)
	c.Assert(sym.Key, qt.Equals, "type-name")
}

func TestUnify_SSADiffRegister(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast unify))`)
	result := eval(t, engine, `
		(let* ((a '(ssa-binop (name . "t0") (op . +) (x . "r1") (y . "r2")
		            (type . "int")))
		       (b '(ssa-binop (name . "t1") (op . +) (x . "r1") (y . "r2")
		            (type . "int")))
		       (r (ssa-diff a b))
		       (diffs (diff-result-diffs r)))
			(caar diffs))`)
	// 'name' field in ssa-binop (instruction) should be classified as 'register
	sym, ok := result.Internal().(*values.Symbol)
	c.Assert(ok, qt.IsTrue)
	c.Assert(sym.Key, qt.Equals, "register")
}

func TestUnify_Unifiable(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast unify))`)

	t.Run("identical is unifiable", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((node '(ssa-func (name . "Foo") (type . "int")))
			       (r (ssa-diff node node)))
				(unifiable? r 0.8))`)
		c.Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("structural diff not unifiable", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((a '(ssa-binop (name . "t0") (op . +) (x . "r1") (y . "r2") (type . "int")))
			       (b '(ssa-call (name . "t0") (type . "int")))
			       (r (ssa-diff a b)))
				(unifiable? r 0.8))`)
		c.Assert(result.Internal(), qt.Equals, values.FalseValue)
	})
}
