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

func TestGoCFGToStructured_LoopReturn(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(items []int) error {
				for _, v := range items {
					if v < 0 { return errNeg }
				}
				return nil
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("returns a block", func(t *testing.T) {
		result := eval(t, engine, `(eq? (car result) 'block)`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("has break instead of return in loop", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "break"), qt.IsTrue,
			qt.Commentf("expected break in loop, got:\n%s", s))
	})

	t.Run("has guard after loop", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "errNeg"), qt.IsTrue,
			qt.Commentf("expected errNeg in guard, got:\n%s", s))
	})

	t.Run("has single-exit structure", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "} else {"), qt.IsTrue,
			qt.Commentf("expected else from Case 1 folding, got:\n%s", s))
	})
}

func TestGoCFGToStructured_LoopMultipleReturns(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(items []int) int {
				for _, v := range items {
					if v < 0 { return -1 }
					if v > 100 { return 100 }
				}
				return 0
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("both returns become guards", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "return -1"), qt.IsTrue,
			qt.Commentf("expected return -1 in guard, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "return 100"), qt.IsTrue,
			qt.Commentf("expected return 100 in guard, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "break"), qt.IsTrue,
			qt.Commentf("expected break in loop, got:\n%s", s))
	})
}

func TestGoCFGToStructured_NestedLoopReturn(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(matrix [][]int) int {
				for _, row := range matrix {
					for _, v := range row {
						if v < 0 { return v }
					}
				}
				return 0
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("nested loops produce two ctl vars", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "_ctl0"), qt.IsTrue,
			qt.Commentf("expected _ctl0, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "_ctl1"), qt.IsTrue,
			qt.Commentf("expected _ctl1, got:\n%s", s))
		qt.New(t).Assert(strings.Count(s, "break"), qt.Equals, 2,
			qt.Commentf("expected 2 breaks, got:\n%s", s))
	})
}

func TestGoCFGToStructured_LoopNoReturn(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(items []int) int {
				sum := 0
				for _, v := range items {
					sum += v
				}
				return sum
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("returns block unchanged", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "sum += v"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "return sum"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "_ctl"), qt.IsFalse,
			qt.Commentf("should not have ctl var, got:\n%s", s))
	})
}

func TestGoCFGToStructured_LoopReturnInSwitch(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(items []int) int {
				for _, v := range items {
					switch {
					case v < 0:
						return -1
					}
				}
				return 0
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("returns a block", func(t *testing.T) {
		result := eval(t, engine, `(eq? (car result) 'block)`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("has labeled break", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "_loop0"), qt.IsTrue,
			qt.Commentf("expected loop label, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "break _loop0"), qt.IsTrue,
			qt.Commentf("expected labeled break, got:\n%s", s))
	})

	t.Run("has guard after loop", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "return -1"), qt.IsTrue,
			qt.Commentf("expected return in guard, got:\n%s", s))
	})
}

func TestGoCFGToStructured_LoopReturnInTypeSwitch(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(items []interface{}) int {
				for _, v := range items {
					switch v.(type) {
					case int:
						return 1
					case string:
						return 2
					}
				}
				return 0
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("has labeled break and multiple guards", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "break _loop0"), qt.IsTrue,
			qt.Commentf("expected labeled break, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "return 1"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "return 2"), qt.IsTrue)
	})
}

func TestGoCFGToStructured_LoopReturnInSelect(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(ch chan int, done chan bool) int {
				for {
					select {
					case v := <-ch:
						if v < 0 { return v }
					case <-done:
						return 0
					}
				}
				return -1
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("has labeled break", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "break _loop0"), qt.IsTrue,
			qt.Commentf("expected labeled break, got:\n%s", s))
	})
}

func TestGoCFGToStructured_GuardPlusLoop(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(items []int) int {
				if items == nil { return -1 }
				for _, v := range items {
					if v < 0 { return v }
				}
				return 0
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("both guard and loop are restructured", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		// Guard clause becomes if/else
		qt.New(t).Assert(strings.Contains(s, "} else"), qt.IsTrue,
			qt.Commentf("expected else from guard folding, got:\n%s", s))
		// Loop return becomes break
		qt.New(t).Assert(strings.Contains(s, "break"), qt.IsTrue,
			qt.Commentf("expected break in loop, got:\n%s", s))
		// Control variable present
		qt.New(t).Assert(strings.Contains(s, "_ctl"), qt.IsTrue,
			qt.Commentf("expected ctl var, got:\n%s", s))
	})
}

func TestGoCFGToStructured_LoopReturnLocalVar(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(items []int) int {
				for _, v := range items {
					if v < 0 { return v }
				}
				return 0
			}")
		(define file (go-parse-string source))
		(define decl (car (cdr (assoc 'decls (cdr file)))))
		(define body (cdr (assoc 'body (cdr decl))))
		(define ftype (cdr (assoc 'type (cdr decl))))
		(define result (go-cfg-to-structured body ftype))`)

	t.Run("has result variable", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "var _r0 int"), qt.IsTrue,
			qt.Commentf("expected _r0 declaration, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "_r0 = v"), qt.IsTrue,
			qt.Commentf("expected _r0 assignment, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "return _r0"), qt.IsTrue,
			qt.Commentf("expected return _r0 in guard, got:\n%s", s))
	})
}

func TestGoCFGToStructured_LoopMultiReturn(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(items []int) (int, error) {
				for _, v := range items {
					if v < 0 { return v, errNeg }
				}
				return 0, nil
			}")
		(define file (go-parse-string source))
		(define decl (car (cdr (assoc 'decls (cdr file)))))
		(define body (cdr (assoc 'body (cdr decl))))
		(define ftype (cdr (assoc 'type (cdr decl))))
		(define result (go-cfg-to-structured body ftype))`)

	t.Run("has two result variables", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "var _r0 int"), qt.IsTrue,
			qt.Commentf("expected _r0 int, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "var _r1 error"), qt.IsTrue,
			qt.Commentf("expected _r1 error, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "return _r0, _r1"), qt.IsTrue,
			qt.Commentf("expected return _r0, _r1 in guard, got:\n%s", s))
	})
}

func TestGoCFGToStructured_LoopReturnNoFuncType(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(items []int) error {
				for _, v := range items {
					if v < 0 { return errNeg }
				}
				return nil
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("still works without func-type", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "errNeg"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "_r0"), qt.IsFalse,
			qt.Commentf("should not synthesize _r vars without func-type, got:\n%s", s))
	})
}

func TestGoCFGToStructured_ForwardGoto(t *testing.T) {
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

	t.Run("eliminates goto", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "goto"), qt.IsFalse,
			qt.Commentf("should eliminate goto, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "println(x)"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "println(0)"), qt.IsTrue)
	})
}

func TestGoCFGToStructured_MultipleForwardGotos(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(x, y int) {
				if x > 0 { goto cleanup }
				if y > 0 { goto cleanup }
				work()
				cleanup:
				close()
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("nests conditions", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "goto"), qt.IsFalse,
			qt.Commentf("should eliminate goto, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "work()"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "close()"), qt.IsTrue)
	})
}

func TestGoCFGToStructured_CrossBranchGoto(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(x int) {
				goto inner
				if x > 0 {
					inner:
					println(x)
				}
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

func TestGoCFGToStructured_BackwardGoto(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f() {
				start:
				work()
				if shouldContinue() { goto start }
				cleanup()
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("becomes for loop", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "goto"), qt.IsFalse,
			qt.Commentf("should eliminate goto, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "for {"), qt.IsTrue,
			qt.Commentf("expected for loop, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "work()"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "break"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "cleanup()"), qt.IsTrue)
	})
}

func TestGoCFGToStructured_BareBackwardGoto(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f() {
				start:
				work()
				goto start
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("becomes infinite for loop", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "goto"), qt.IsFalse,
			qt.Commentf("should eliminate goto, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "for {"), qt.IsTrue,
			qt.Commentf("expected for loop, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "work()"), qt.IsTrue)
		// No break — this is an intentional infinite loop.
		qt.New(t).Assert(strings.Contains(s, "break"), qt.IsFalse,
			qt.Commentf("should not have break in infinite loop, got:\n%s", s))
	})
}

func TestGoCFGToStructured_MixedGoto(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f() int {
				retry:
				result := try()
				if result == nil { goto retry }
				if result.err != nil { goto cleanup }
				process(result)
				cleanup:
				return close()
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("eliminates both gotos", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "goto"), qt.IsFalse,
			qt.Commentf("should eliminate both gotos, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "for {"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "close()"), qt.IsTrue)
	})
}
