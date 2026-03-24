# Transformation Primitives Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `ast-transform`, `ast-splice`, `take`, `drop` as Scheme utilities and `go-cfg-to-structured` as a Go primitive that restructures early-return blocks into single-exit if/else trees.

**Architecture:** Scheme utilities go in `cmd/wile-goast/lib/wile/goast/utils.scm` (exported from `utils.sld`). The Go primitive lives in `goast/` — unmaps a block s-expression to `*ast.BlockStmt`, right-folds guard-if-return statements into nested if/else, maps back to s-expression. No SSA or type info needed.

**Tech Stack:** Scheme (R7RS), Go (`go/ast`, existing bidirectional mapper in `goast/`).

**Design decisions (from Q&A):**
- Guard matching: any if-body containing a `return-stmt` (not just single-statement bodies)
- Non-if statements between guards: absorb into else branches (safe — restructured block is terminal)
- Nested blocks: top-level only, caller recurses if needed
- No early returns: return block unchanged (identity)
- goto/labeled branches: return `#f`
- Implementation: Go primitive via unmap→transform→map round-trip

---

### Task 1: `take` and `drop` in utils.scm

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/utils.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/utils.sld`
- Test: `goast/belief_integration_test.go`

**Step 1: Write the test**

Add to `goast/belief_integration_test.go`:

```go
func TestUtilsTakeDrop(t *testing.T) {
	engine := newBeliefEngine(t)

	t.Run("take", func(t *testing.T) {
		result := evalMultiple(t, engine, `
			(import (wile goast utils))
			(take '(a b c d e) 3)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(a b c)")
	})

	t.Run("take zero", func(t *testing.T) {
		result := evalMultiple(t, engine, `
			(import (wile goast utils))
			(take '(a b c) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "()")
	})

	t.Run("drop", func(t *testing.T) {
		result := evalMultiple(t, engine, `
			(import (wile goast utils))
			(drop '(a b c d e) 2)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(c d e)")
	})

	t.Run("drop all", func(t *testing.T) {
		result := evalMultiple(t, engine, `
			(import (wile goast utils))
			(drop '(a b c) 3)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "()")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run TestUtilsTakeDrop -v`
Expected: FAIL — `take` not defined.

**Step 3: Add `take` and `drop` to utils.scm**

Append to `cmd/wile-goast/lib/wile/goast/utils.scm`:

```scheme
;; First n elements of a list
(define (take lst n)
  (if (or (= n 0) (null? lst)) '()
    (cons (car lst) (take (cdr lst) (- n 1)))))

;; Drop first n elements of a list
(define (drop lst n)
  (if (or (= n 0) (null? lst)) lst
    (drop (cdr lst) (- n 1))))
```

**Step 4: Export from utils.sld**

Add `take` and `drop` to the `(export ...)` form in `utils.sld`:

```scheme
(define-library (wile goast utils)
  (export
    nf tag? walk
    filter-map flat-map
    member? unique has-char?
    ordered-pairs
    take drop)
  (include "utils.scm"))
```

**Step 5: Run test to verify it passes**

Run: `go test ./goast/ -run TestUtilsTakeDrop -v`
Expected: PASS

**Step 6: Commit**

```
git add cmd/wile-goast/lib/wile/goast/utils.scm cmd/wile-goast/lib/wile/goast/utils.sld goast/belief_integration_test.go
git commit -m "feat: add take and drop to (wile goast utils)"
```

---

### Task 2: `ast-transform` in utils.scm

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/utils.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/utils.sld`
- Test: `goast/belief_integration_test.go`

**Step 1: Write the test**

Add to `goast/belief_integration_test.go`:

```go
func TestAstTransform(t *testing.T) {
	engine := newBeliefEngine(t)

	t.Run("replace matching node", func(t *testing.T) {
		// Parse a Go expression, transform all idents named "x" to "y"
		result := evalMultiple(t, engine, `
			(import (wile goast utils))
			(let* ((node (go-parse-expr "x + 1"))
			       (transformed (ast-transform node
			         (lambda (n)
			           (and (tag? n 'ident)
			                (equal? (nf n 'name) "x")
			                (list 'ident (cons 'name "y")))))))
			  (go-format transformed))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `"y + 1"`)
	})

	t.Run("no match returns unchanged", func(t *testing.T) {
		result := evalMultiple(t, engine, `
			(import (wile goast utils))
			(let* ((node (go-parse-expr "x + 1"))
			       (transformed (ast-transform node
			         (lambda (n) #f))))
			  (go-format transformed))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `"x + 1"`)
	})

	t.Run("no recursion into replacement", func(t *testing.T) {
		// Replace ident "x" with binary-expr containing "x".
		// If it recursed into the replacement, it would loop forever.
		result := evalMultiple(t, engine, `
			(import (wile goast utils))
			(let* ((node (go-parse-expr "x"))
			       (transformed (ast-transform node
			         (lambda (n)
			           (and (tag? n 'ident)
			                (equal? (nf n 'name) "x")
			                (go-parse-expr "x + 1"))))))
			  (go-format transformed))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `"x + 1"`)
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run TestAstTransform -v`
Expected: FAIL — `ast-transform` not defined.

**Step 3: Add `ast-transform` to utils.scm**

Append to `cmd/wile-goast/lib/wile/goast/utils.scm`:

```scheme
;; Depth-first pre-order tree rewriter over goast s-expressions.
;; f returns a replacement node (no recursion into it) or #f (keep, recurse).
(define (ast-transform node f)
  (let ((replacement (f node)))
    (if replacement replacement
      (cond
        ;; Tagged alist: recurse into field values
        ((and (pair? node) (symbol? (car node)))
         (cons (car node)
               (map (lambda (field)
                      (if (pair? field)
                        (cons (car field)
                              (ast-transform (cdr field) f))
                        field))
                    (cdr node))))
        ;; List of child nodes
        ((and (pair? node) (pair? (car node)))
         (map (lambda (n) (ast-transform n f)) node))
        ;; Atom
        (else node)))))
```

**Step 4: Export from utils.sld**

Add `ast-transform` to the `(export ...)` form.

**Step 5: Run test to verify it passes**

Run: `go test ./goast/ -run TestAstTransform -v`
Expected: PASS

**Step 6: Commit**

```
git add cmd/wile-goast/lib/wile/goast/utils.scm cmd/wile-goast/lib/wile/goast/utils.sld goast/belief_integration_test.go
git commit -m "feat: add ast-transform tree rewriter to (wile goast utils)"
```

---

### Task 3: `ast-splice` in utils.scm

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/utils.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/utils.sld`
- Test: `goast/belief_integration_test.go`

**Step 1: Write the test**

Add to `goast/belief_integration_test.go`:

```go
func TestAstSplice(t *testing.T) {
	engine := newBeliefEngine(t)

	t.Run("splice replaces element with multiple", func(t *testing.T) {
		result := evalMultiple(t, engine, `
			(import (wile goast utils))
			(ast-splice '(a b c)
			  (lambda (x) (and (eq? x 'b) '(x y z))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(a x y z c)")
	})

	t.Run("splice no match keeps original", func(t *testing.T) {
		result := evalMultiple(t, engine, `
			(import (wile goast utils))
			(ast-splice '(a b c) (lambda (x) #f))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(a b c)")
	})

	t.Run("splice with empty list deletes", func(t *testing.T) {
		result := evalMultiple(t, engine, `
			(import (wile goast utils))
			(ast-splice '(a b c)
			  (lambda (x) (and (eq? x 'b) '())))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(a c)")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run TestAstSplice -v`
Expected: FAIL — `ast-splice` not defined.

**Step 3: Add `ast-splice` to utils.scm**

Append to `cmd/wile-goast/lib/wile/goast/utils.scm`:

```scheme
;; Flat-mapping rewriter for lists (e.g. statement lists).
;; f returns a list (splice in place) or #f (keep original element).
(define (ast-splice lst f)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (let ((result (f (car xs))))
        (if result
          (loop (cdr xs) (append (reverse result) acc))
          (loop (cdr xs) (cons (car xs) acc)))))))
```

**Step 4: Export from utils.sld**

Add `ast-splice` to the `(export ...)` form.

**Step 5: Run test to verify it passes**

Run: `go test ./goast/ -run TestAstSplice -v`
Expected: PASS

**Step 6: Commit**

```
git add cmd/wile-goast/lib/wile/goast/utils.scm cmd/wile-goast/lib/wile/goast/utils.sld goast/belief_integration_test.go
git commit -m "feat: add ast-splice list rewriter to (wile goast utils)"
```

---

### Task 4: `go-cfg-to-structured` — core algorithm

**Files:**
- Create: `goast/restructure.go`
- Create: `goast/restructure_test.go`

**Step 1: Write the test**

The test round-trips through the full pipeline: parse Go source → extract body →
restructure → format back to Go source.

```go
// goast/restructure_test.go
package goast_test

import (
	"strings"
	"testing"

	"github.com/aalpar/wile-goast/testutil"
	"github.com/aalpar/wile/values"

	qt "github.com/frankban/quicktest"
)

func TestGoCFGToStructured_GuardClauses(t *testing.T) {
	engine := newEngine(t)

	// clamp pattern: if guard, if guard, return
	testutil.RunScheme(t, engine, `
		(define source "
			package p
			func clamp(x, lo, hi int) int {
				if x < lo { return lo }
				if x > hi { return hi }
				return x
			}")
		(define file (go-parse-string source))
		(define body (nf (car (nf file 'decls)) 'body))
		(define result (go-cfg-to-structured body))`)

	t.Run("returns a block", func(t *testing.T) {
		result := testutil.RunScheme(t, engine, `(tag? result 'block)`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("formats to nested if-else", func(t *testing.T) {
		result := testutil.RunScheme(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "} else if"), qt.IsTrue,
			qt.Commentf("expected else-if chain, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "} else {"), qt.IsTrue,
			qt.Commentf("expected final else, got:\n%s", s))
	})
}

func TestGoCFGToStructured_MultiStmtGuard(t *testing.T) {
	engine := newEngine(t)

	// Guard body has multiple statements before return
	testutil.RunScheme(t, engine, `
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
		(define body (nf (car (nf file 'decls)) 'body))
		(define result (go-cfg-to-structured body))`)

	t.Run("preserves multi-stmt body", func(t *testing.T) {
		result := testutil.RunScheme(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "println"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "} else {"), qt.IsTrue)
	})
}

func TestGoCFGToStructured_InterleavedStatements(t *testing.T) {
	engine := newEngine(t)

	// Non-if statements between guards get absorbed into else branches
	testutil.RunScheme(t, engine, `
		(define source "
			package p
			func f(x int) int {
				if x < 0 { return -1 }
				y := x * 2
				if y > 100 { return 100 }
				return y
			}")
		(define file (go-parse-string source))
		(define body (nf (car (nf file 'decls)) 'body))
		(define result (go-cfg-to-structured body))`)

	t.Run("absorbs non-if into else", func(t *testing.T) {
		result := testutil.RunScheme(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		// y := x * 2 should appear inside the else branch
		qt.New(t).Assert(strings.Contains(s, "} else {"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "y := x * 2"), qt.IsTrue)
	})
}

func TestGoCFGToStructured_NoEarlyReturns(t *testing.T) {
	engine := newEngine(t)

	// Already single-exit: return unchanged
	testutil.RunScheme(t, engine, `
		(define source "
			package p
			func f(x int) int {
				y := x + 1
				return y
			}")
		(define file (go-parse-string source))
		(define body (nf (car (nf file 'decls)) 'body))
		(define result (go-cfg-to-structured body))`)

	t.Run("returns block unchanged", func(t *testing.T) {
		result := testutil.RunScheme(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "y := x + 1"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "return y"), qt.IsTrue)
	})
}

func TestGoCFGToStructured_GotoReturnsFalse(t *testing.T) {
	engine := newEngine(t)

	// Block with goto → returns #f
	testutil.RunScheme(t, engine, `
		(define source "
			package p
			func f() {
				goto end
				end:
			}")
		(define file (go-parse-string source))
		(define body (nf (car (nf file 'decls)) 'body))
		(define result (go-cfg-to-structured body))`)

	t.Run("returns false", func(t *testing.T) {
		result := testutil.RunScheme(t, engine, `result`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.FalseValue)
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run TestGoCFGToStructured -v`
Expected: FAIL — `go-cfg-to-structured` not defined.

**Step 3: Write the restructurer**

```go
// goast/restructure.go
package goast

import (
	"go/ast"
	"go/token"

	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

var errRestructureError = werr.NewStaticError("restructure error")

// PrimGoCFGToStructured implements (go-cfg-to-structured block-sexpr).
// Takes a block s-expression. Returns a restructured block where early
// returns are nested into if/else chains (single exit point), or the
// block unchanged if no early returns, or #f if the block contains
// control flow it cannot restructure (goto, labeled branches).
func PrimGoCFGToStructured(mc *machine.MachineContext) error {
	blockVal := mc.Arg(0)

	node, err := unmapNode(blockVal)
	if err != nil {
		return werr.WrapForeignErrorf(errRestructureError,
			"go-cfg-to-structured: %s", err)
	}

	block, ok := node.(*ast.BlockStmt)
	if !ok {
		return werr.WrapForeignErrorf(errRestructureError,
			"go-cfg-to-structured: expected block, got %T", node)
	}

	// Reject blocks containing goto or labeled branches.
	if containsGoto(block) {
		mc.SetValue(values.FalseValue)
		return nil
	}

	// If no early returns, return unchanged.
	if !hasEarlyReturns(block.List) {
		mc.SetValue(blockVal)
		return nil
	}

	// Right-fold into nested if/else.
	result := restructureStmts(block.List)

	opts := &mapperOpts{}
	mc.SetValue(mapNode(&ast.BlockStmt{List: result}, opts))
	return nil
}

// hasEarlyReturns checks whether the statement list contains return
// statements before the final position (inside if-bodies counts).
func hasEarlyReturns(stmts []ast.Stmt) bool {
	for i, stmt := range stmts {
		if i == len(stmts)-1 {
			break // last statement — not "early"
		}
		if isGuardIf(stmt) {
			return true
		}
	}
	return false
}

// isGuardIf returns true if the statement is an if-stmt with no else
// branch whose body contains a return-stmt.
func isGuardIf(stmt ast.Stmt) bool {
	ifStmt, ok := stmt.(*ast.IfStmt)
	if !ok || ifStmt.Else != nil {
		return false
	}
	return bodyContainsReturn(ifStmt.Body)
}

// bodyContainsReturn walks a block looking for any return-stmt.
// Does NOT recurse into nested function literals (their returns
// belong to the literal, not the enclosing function).
func bodyContainsReturn(block *ast.BlockStmt) bool {
	found := false
	ast.Inspect(block, func(n ast.Node) bool {
		if found {
			return false
		}
		switch n.(type) {
		case *ast.ReturnStmt:
			found = true
			return false
		case *ast.FuncLit:
			return false // don't recurse into nested functions
		}
		return true
	})
	return found
}

// containsGoto walks the block for goto statements or labeled
// statements. Returns true if any are found.
func containsGoto(block *ast.BlockStmt) bool {
	found := false
	ast.Inspect(block, func(n ast.Node) bool {
		if found {
			return false
		}
		switch v := n.(type) {
		case *ast.BranchStmt:
			if v.Tok == token.GOTO {
				found = true
				return false
			}
		case *ast.LabeledStmt:
			found = true
			return false
		case *ast.FuncLit:
			return false
		}
		return true
	})
	return found
}

// restructureStmts right-folds the statement list. Guard-if statements
// get the accumulated "rest" as their else branch. Non-if statements
// are prepended to "rest."
func restructureStmts(stmts []ast.Stmt) []ast.Stmt {
	if len(stmts) == 0 {
		return stmts
	}

	// Start from the last statement.
	rest := []ast.Stmt{stmts[len(stmts)-1]}

	// Process from second-to-last backward.
	for i := len(stmts) - 2; i >= 0; i-- {
		stmt := stmts[i]
		if isGuardIf(stmt) {
			ifStmt := stmt.(*ast.IfStmt)
			// Clone the if-stmt with rest as else branch.
			ifStmt = &ast.IfStmt{
				Init: ifStmt.Init,
				Cond: ifStmt.Cond,
				Body: ifStmt.Body,
				Else: wrapBlock(rest),
			}
			rest = []ast.Stmt{ifStmt}
		} else {
			rest = append([]ast.Stmt{stmt}, rest...)
		}
	}

	return rest
}

// wrapBlock wraps a statement list in a BlockStmt. If the list is a
// single BlockStmt, returns it directly (avoids double-wrapping).
// If the list is a single IfStmt, returns it directly (Go allows
// if-stmt as else branch without wrapping).
func wrapBlock(stmts []ast.Stmt) ast.Stmt {
	if len(stmts) == 1 {
		switch stmts[0].(type) {
		case *ast.BlockStmt, *ast.IfStmt:
			return stmts[0]
		}
	}
	return &ast.BlockStmt{List: stmts}
}
```

**Step 4: Register the primitive**

Add to `goast/register.go` inside `addPrimitives`, before the closing `})`:

```go
{Name: "go-cfg-to-structured", ParamCount: 1, Impl: PrimGoCFGToStructured,
	Doc:        "Restructures a block with early returns into a single-exit if/else tree.",
	ParamNames: []string{"block"}, Category: "goast"},
```

**Step 5: Run test to verify it passes**

Run: `go test ./goast/ -run TestGoCFGToStructured -v`
Expected: PASS

**Step 6: Run all goast tests**

Run: `go test ./goast/ -count=1`
Expected: PASS — no regressions.

**Step 7: Commit**

```
git add goast/restructure.go goast/restructure_test.go goast/register.go
git commit -m "feat: add go-cfg-to-structured primitive for early-return restructuring"
```

---

### Task 5: Integration test — inline-expand with correct output

**Files:**
- Create: `goast/restructure_integration_test.go`

This test verifies the full inlining pipeline: parse → expression-inline →
restructure → return-rewrite → splice → format. Uses the same `clamp`/`compute`
example from `examples/goast-query/inline-expand.scm`.

**Step 1: Write the test**

```go
// goast/restructure_integration_test.go
package goast_test

import (
	"strings"
	"testing"

	"github.com/aalpar/wile-goast/testutil"

	qt "github.com/frankban/quicktest"
)

func TestInlinePipeline(t *testing.T) {
	engine := newBeliefEngine(t)

	// Load the source, import utils, define helpers, run full pipeline
	result := evalMultiple(t, engine, `
		(import (wile goast utils))

		(define source "
			package example

			func double(x int) int { return x * 2 }
			func addOne(x int) int { return x + 1 }

			func clamp(x, lo, hi int) int {
				if x < lo { return lo }
				if x > hi { return hi }
				return x
			}

			func compute(a, b int) int {
				x := double(a)
				y := addOne(b)
				z := clamp(x+y, 0, 100)
				return z
			}")

		(define file (go-parse-string source))

		;; Helpers
		(define (find-func name)
		  (let loop ((ds (nf file 'decls)))
		    (cond ((null? ds) #f)
		          ((and (tag? (car ds) 'func-decl)
		                (equal? (nf (car ds) 'name) name))
		           (car ds))
		          (else (loop (cdr ds))))))

		(define (formal-names fn)
		  (flat-map
		    (lambda (f) (let ((ns (nf f 'names))) (if (pair? ns) ns '())))
		    (let ((ps (nf (nf fn 'type) 'params)))
		      (if (pair? ps) ps '()))))

		(define (subst-idents node mapping)
		  (ast-transform node
		    (lambda (n)
		      (and (tag? n 'ident)
		           (let ((e (assoc (nf n 'name) mapping)))
		             (and e (cdr e)))))))

		(define (build-map fs as)
		  (if (or (null? fs) (null? as)) '()
		    (cons (cons (car fs) (car as))
		          (build-map (cdr fs) (cdr as)))))

		;; Single-expr return body → expression
		(define (single-ret-expr fn)
		  (let* ((stmts (nf (nf fn 'body) 'list)))
		    (and (pair? stmts) (= (length stmts) 1)
		         (tag? (car stmts) 'return-stmt)
		         (let ((rs (nf (car stmts) 'results)))
		           (and (pair? rs) (= (length rs) 1) (car rs))))))

		;; Phase 1: inline single-expression callees
		(define (inline-expr node)
		  (ast-transform node
		    (lambda (n)
		      (and (tag? n 'call-expr)
		           (let* ((fun (nf n 'fun))
		                  (name (and (tag? fun 'ident) (nf fun 'name)))
		                  (callee (and name (find-func name)))
		                  (ret (and callee (single-ret-expr callee))))
		             (and ret
		                  (let ((m (build-map (formal-names callee)
		                                     (or (nf n 'args) '()))))
		                    (inline-expr (subst-idents ret m)))))))))

		;; Phase 2: inline multi-statement callees via restructuring
		(define target (find-func "compute"))
		(define phase1 (inline-expr target))

		(define phase2-body
		  (ast-splice (nf (nf phase1 'body) 'list)
		    (lambda (stmt)
		      (and (tag? stmt 'assign-stmt)
		           (let* ((rhs (nf stmt 'rhs))
		                  (call (and (pair? rhs) (car rhs)))
		                  (lhs (nf stmt 'lhs))
		                  (target-name (and (pair? lhs)
		                                   (tag? (car lhs) 'ident)
		                                   (nf (car lhs) 'name)))
		                  (callee-name (and (tag? call 'call-expr)
		                                   (let ((f (nf call 'fun)))
		                                     (and (tag? f 'ident) (nf f 'name)))))
		                  (callee (and callee-name (find-func callee-name))))
		             (and callee (not (single-ret-expr callee))
		                  (let* ((structured (go-cfg-to-structured (nf callee 'body)))
		                         (m (build-map (formal-names callee)
		                                      (or (nf call 'args) '())))
		                         (substituted (subst-idents structured m))
		                         (rewritten (ast-transform substituted
		                                      (lambda (n)
		                                        (and (tag? n 'return-stmt)
		                                             (list 'assign-stmt
		                                                   (cons 'lhs (list (list 'ident (cons 'name target-name))))
		                                                   (cons 'tok '=)
		                                                   (cons 'rhs (nf n 'results))))))))
		                    (nf rewritten 'list))))))))

		;; Rebuild function with new body
		(define phase2
		  (ast-transform phase1
		    (lambda (n)
		      (and (tag? n 'block)
		           (equal? (nf n 'list) (nf (nf phase1 'body) 'list))
		           (list 'block (cons 'list phase2-body))))))

		(go-format phase2)
	`)

	c := qt.New(t)
	s := result.Internal().(*values.String).Value

	// Phase 1: double and addOne should be inlined
	c.Assert(strings.Contains(s, "double"), qt.IsFalse,
		qt.Commentf("double should be inlined"))
	c.Assert(strings.Contains(s, "addOne"), qt.IsFalse,
		qt.Commentf("addOne should be inlined"))

	// Phase 2: clamp should be inlined with correct control flow
	c.Assert(strings.Contains(s, "clamp"), qt.IsFalse,
		qt.Commentf("clamp should be inlined"))
	c.Assert(strings.Contains(s, "else"), qt.IsTrue,
		qt.Commentf("should have if/else structure"))

	// Should NOT have the broken pattern (three separate ifs)
	c.Assert(strings.Count(s, "z :="), qt.Equals, 0,
		qt.Commentf("should use z = not z :="))
}
```

**Step 2: Run test**

Run: `go test ./goast/ -run TestInlinePipeline -v`
Expected: PASS

Note: This test may need adjustment based on exact `go-format` output. Run it,
inspect the output, and adjust assertions if the formatting differs from
expectations (e.g., extra newlines, different else-if formatting). The key
properties to check: no `double`/`addOne`/`clamp` calls remain, `else` exists,
no `z :=` (should be `z =`).

**Step 3: Commit**

```
git add goast/restructure_integration_test.go
git commit -m "test: add inline pipeline integration test"
```

---

### Task 6: Update documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs/PRIMITIVES.md`

**Step 1: Add primitives to CLAUDE.md**

In the primitives table under `goast — (wile goast)`, add:

```
| `go-cfg-to-structured` | Restructure early-return block into single-exit if/else tree |
```

In the Key Files table, add:

```
| `goast/restructure.go` | Early-return restructuring (go-cfg-to-structured) |
```

**Step 2: Add to docs/PRIMITIVES.md**

Add a new subsection under the AST Layer section:

```markdown
### Transformation

| Primitive | Returns | Description |
|-----------|---------|-------------|
| `(go-cfg-to-structured block)` | block or `#f` | Restructure early returns into single-exit if/else |

`go-cfg-to-structured` takes a block s-expression and returns a restructured
block where guard-if-return patterns are folded into nested if/else chains.
Every return in the output is at a leaf of the if/else tree.

Returns the block unchanged if there are no early returns. Returns `#f` if
the block contains `goto` or labeled statements.

**Limitations (documented):**
- Top-level only — does not recurse into nested blocks
- Case 1 only (linear early returns) — loop-internal early returns not handled
- No multiple return value support
```

Also document the new utils:

```markdown
### Scheme Utilities — `(wile goast utils)`

| Function | Description |
|----------|-------------|
| `(ast-transform node f)` | Depth-first pre-order tree rewriter. `f` returns replacement or `#f` |
| `(ast-splice lst f)` | Flat-map rewriter for lists. `f` returns list (splice) or `#f` (keep) |
| `(take lst n)` | First n elements |
| `(drop lst n)` | Drop first n elements |
```

**Step 3: Update TODO.md**

Mark B1 and B2 (Case 1) as done.

**Step 4: Commit**

```
git add CLAUDE.md docs/PRIMITIVES.md TODO.md
git commit -m "docs: add transformation primitives to reference"
```

---

### Summary

| Task | Scope | Estimated complexity |
|------|-------|---------------------|
| 1. `take`/`drop` | 10 lines Scheme | Trivial |
| 2. `ast-transform` | 15 lines Scheme | Straightforward |
| 3. `ast-splice` | 10 lines Scheme | Straightforward |
| 4. `go-cfg-to-structured` | ~120 lines Go | Core algorithm |
| 5. Integration test | ~80 lines Scheme in Go test | Validates full pipeline |
| 6. Documentation | CLAUDE.md, PRIMITIVES.md, TODO.md | Mechanical |
