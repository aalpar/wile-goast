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
	"strings"
	"testing"

	"github.com/aalpar/wile/values"

	qt "github.com/frankban/quicktest"
)

func TestGoCFGToStructured_GuardClauses(t *testing.T) {
	engine := newEngine(t)

	// clamp pattern: if guard, if guard, return
	eval(t, engine, `
		(define source "
			package p
			func clamp(x, lo, hi int) int {
				if x < lo { return lo }
				if x > hi { return hi }
				return x
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("returns a block", func(t *testing.T) {
		result := eval(t, engine, `(eq? (car result) 'block)`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("formats to nested if-else", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "} else if"), qt.IsTrue,
			qt.Commentf("expected else-if chain, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "} else {"), qt.IsTrue,
			qt.Commentf("expected final else, got:\n%s", s))
	})
}

func TestGoCFGToStructured_MultiStmtGuard(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(x int) int {
				if x < 0 {
					println(x)
					return -1
				}
				return x
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("preserves multi-stmt body", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "println"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "} else {"), qt.IsTrue)
	})
}

func TestGoCFGToStructured_InterleavedStatements(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(x int) int {
				if x < 0 { return -1 }
				y := x * 2
				if y > 100 { return 100 }
				return y
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("absorbs non-if into else", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "} else {"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "y := x * 2"), qt.IsTrue)
	})
}

func TestGoCFGToStructured_NoEarlyReturns(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(x int) int {
				y := x + 1
				return y
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("returns block unchanged", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "y := x + 1"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "return y"), qt.IsTrue)
	})
}

func TestGoCFGToStructured_GotoReturnsFalse(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(x int) {
				if x > 0 { goto end }
				println(x)
				end:
				println(0)
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("returns false", func(t *testing.T) {
		result := eval(t, engine, `result`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.FalseValue)
	})
}
