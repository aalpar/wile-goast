# Belief DSL Emit Mode — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `emit-beliefs` to `(wile goast belief)` — given `run-beliefs` output, produce Scheme source code reproducing the discovered beliefs as `define-belief` and `define-aggregate-belief` forms.

**Architecture:** Extend the `define-belief` and `define-aggregate-belief` macros to quote their selector/checker/analyzer expressions alongside the compiled lambdas. Carry these expressions through to `run-beliefs` result alists. A new `emit-beliefs` procedure formats strong/ok results into loadable Scheme source.

**Tech Stack:** Scheme (R7RS via Wile), Go (test harness)

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `cmd/wile-goast/lib/wile/goast/belief.scm` | Modify | Extend tuples, macros, accessors, result alists, add `emit-beliefs` |
| `cmd/wile-goast/lib/wile/goast/belief.sld` | Modify | Export `emit-beliefs` |
| `goast/belief_integration_test.go` | Modify | Add tests for expression metadata and `emit-beliefs` |

---

### Task 1: Extend per-site belief tuple with expression metadata

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm:47-56` (register-belief!, accessors)
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm:74-77` (define-belief macro)

- [ ] **Step 1: Write the failing test**

In `goast/belief_integration_test.go`, add a test that defines a belief and checks that the result alist from `run-beliefs` contains `sites-expr` and `expect-expr` keys.

```go
func TestBeliefExpressionMetadata(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile goast belief))
		(reset-beliefs!)

		(define-belief "test-expr"
		  (sites (functions-matching (name-matches "Prim")))
		  (expect (custom (lambda (site ctx) 'ok)))
		  (threshold 0.50 1))

		(define results
		  (run-beliefs "github.com/aalpar/wile-goast/goast"))
	`)

	c := qt.New(t)

	t.Run("sites-expr present", func(t *testing.T) {
		result := eval(t, engine, `(assoc 'sites-expr (car results))`)
		c.Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})

	t.Run("sites-expr matches source", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'sites-expr (car results)))`)
		c.Assert(result.SchemeString(), qt.Matches, `.*functions-matching.*name-matches.*Prim.*`)
	})

	t.Run("expect-expr present", func(t *testing.T) {
		result := eval(t, engine, `(assoc 'expect-expr (car results))`)
		c.Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})

	t.Run("expect-expr matches source", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'expect-expr (car results)))`)
		c.Assert(result.SchemeString(), qt.Matches, `.*custom.*lambda.*`)
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefExpressionMetadata -v -count=1`
Expected: FAIL -- `sites-expr` key not found in result alist (returns `#f`)

- [ ] **Step 3: Extend register-belief! to accept 7 args**

In `cmd/wile-goast/lib/wile/goast/belief.scm`, change `register-belief!` (line 47) and add accessors:

```scheme
(define (register-belief! name sites-fn expect-fn min-adherence min-sites
                          sites-expr expect-expr)
  (set! *beliefs*
    (append *beliefs*
      (list (list name sites-fn expect-fn min-adherence min-sites
                  sites-expr expect-expr)))))
```

Add accessors after line 56:

```scheme
(define (belief-sites-expr b) (list-ref b 5))
(define (belief-expect-expr b) (list-ref b 6))
```

- [ ] **Step 4: Update define-belief macro to quote expressions**

In `cmd/wile-goast/lib/wile/goast/belief.scm`, change the macro (line 74):

```scheme
(define-syntax define-belief
  (syntax-rules (sites expect threshold)
    ((_ name (sites selector) (expect checker) (threshold min-adh min-n))
     (register-belief! name selector checker min-adh min-n
                       '(sites selector)
                       '(expect checker)))))
```

- [ ] **Step 5: Attach expression metadata to run-beliefs result alists**

In `cmd/wile-goast/lib/wile/goast/belief.scm`, in the `run-beliefs` function (around line 903), add `sites-expr` and `expect-expr` to the result alist for the `strong` and `weak` branches. Find the alist construction that starts with `(cons (list (cons 'name name)` and add two more entries after `deviations`:

```scheme
(cons 'sites-expr (belief-sites-expr belief))
(cons 'expect-expr (belief-expect-expr belief))
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefExpressionMetadata -v -count=1`
Expected: PASS

- [ ] **Step 7: Verify existing tests still pass**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBelief -v -count=1`
Expected: All existing belief tests PASS. The 7-arg `register-belief!` is only called by the macro, so no call sites break.

- [ ] **Step 8: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/belief.scm goast/belief_integration_test.go
git commit -m "belief: add expression metadata to per-site belief tuples

Extend define-belief macro to quote sites/expect expressions alongside
compiled lambdas. Carry expressions through to run-beliefs result alists
as sites-expr and expect-expr keys."
```

---

### Task 2: Extend aggregate belief tuple with expression metadata

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm:58-65` (register-aggregate-belief!, accessors)
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm:79-82` (define-aggregate-belief macro)
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm:824-850` (evaluate-aggregate-beliefs)

- [ ] **Step 1: Write the failing test**

In `goast/belief_integration_test.go`, add:

```go
func TestAggregateBeliefExpressionMetadata(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast utils))
		(reset-beliefs!)

		(define-aggregate-belief "test-agg-expr"
			(sites (functions-matching (name-matches "Lock")))
			(analyze (aggregate-custom (lambda (sites ctx)
				(list (cons 'verdict 'TEST-OK))))))

		(define results
		  (run-beliefs "github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing"))
	`)

	c := qt.New(t)

	t.Run("sites-expr present", func(t *testing.T) {
		result := eval(t, engine, `(assoc 'sites-expr (car results))`)
		c.Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})

	t.Run("sites-expr matches source", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'sites-expr (car results)))`)
		c.Assert(result.SchemeString(), qt.Matches, `.*functions-matching.*name-matches.*Lock.*`)
	})

	t.Run("analyze-expr present", func(t *testing.T) {
		result := eval(t, engine, `(assoc 'analyze-expr (car results))`)
		c.Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})

	t.Run("analyze-expr matches source", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'analyze-expr (car results)))`)
		c.Assert(result.SchemeString(), qt.Matches, `.*aggregate-custom.*lambda.*`)
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestAggregateBeliefExpressionMetadata -v -count=1`
Expected: FAIL -- `sites-expr` key not found

- [ ] **Step 3: Extend register-aggregate-belief! to accept 5 args**

In `cmd/wile-goast/lib/wile/goast/belief.scm`, change `register-aggregate-belief!` (line 58) and add accessors:

```scheme
(define (register-aggregate-belief! name sites-fn analyzer
                                    sites-expr analyze-expr)
  (set! *aggregate-beliefs*
    (append *aggregate-beliefs*
      (list (list name sites-fn analyzer sites-expr analyze-expr)))))
```

Add accessors after `aggregate-belief-analyzer`:

```scheme
(define (aggregate-belief-sites-expr b) (list-ref b 3))
(define (aggregate-belief-analyze-expr b) (list-ref b 4))
```

- [ ] **Step 4: Update define-aggregate-belief macro to quote expressions**

```scheme
(define-syntax define-aggregate-belief
  (syntax-rules (sites analyze)
    ((_ name (sites selector) (analyze analyzer))
     (register-aggregate-belief! name selector analyzer
                                 '(sites selector)
                                 '(analyze analyzer)))))
```

- [ ] **Step 5: Attach expression metadata to aggregate result alists**

In `evaluate-aggregate-beliefs` (line 824), add `sites-expr` and `analyze-expr` to both the success and error result alists. In the success branch (around line 845), change the `append` to include:

```scheme
(cons 'sites-expr (aggregate-belief-sites-expr belief))
(cons 'analyze-expr (aggregate-belief-analyze-expr belief))
```

Add to both the success alist (after `(cons 'status 'ok)`) and the error alist (after `(cons 'status 'error)`). The error branch needs them too so that suppression can match even failed beliefs.

- [ ] **Step 6: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestAggregateBeliefExpressionMetadata -v -count=1`
Expected: PASS

- [ ] **Step 7: Verify existing aggregate tests still pass**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestAggregate -v -count=1`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/belief.scm goast/belief_integration_test.go
git commit -m "belief: add expression metadata to aggregate belief tuples

Extend define-aggregate-belief macro to quote sites/analyze expressions.
Carry expressions through to run-beliefs result alists as sites-expr and
analyze-expr keys."
```

---

### Task 3: Implement emit-beliefs

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm` (add `emit-beliefs` procedure)
- Modify: `cmd/wile-goast/lib/wile/goast/belief.sld` (export `emit-beliefs`)

- [ ] **Step 1: Write the failing test for per-site emission**

In `goast/belief_integration_test.go`, add:

```go
func TestEmitBeliefsPerSite(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile goast belief))
		(reset-beliefs!)

		(define-belief "prim-have-body"
		  (sites (functions-matching (name-matches "Prim")))
		  (expect (custom (lambda (site ctx)
		    (if (nf site 'body) 'has-body 'no-body))))
		  (threshold 0.90 3))

		(define results (run-beliefs "github.com/aalpar/wile-goast/goast"))
		(define emitted (emit-beliefs results))
	`)

	c := qt.New(t)

	t.Run("returns a string", func(t *testing.T) {
		result := eval(t, engine, `(string? emitted)`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("contains define-belief", func(t *testing.T) {
		result := eval(t, engine, `(string-contains emitted "define-belief")`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("contains belief name", func(t *testing.T) {
		result := eval(t, engine, `(string-contains emitted "prim-have-body")`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("contains sites expression", func(t *testing.T) {
		result := eval(t, engine, `(string-contains emitted "functions-matching")`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("contains expect expression", func(t *testing.T) {
		result := eval(t, engine, `(string-contains emitted "custom")`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("contains threshold", func(t *testing.T) {
		result := eval(t, engine, `(string-contains emitted "threshold")`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestEmitBeliefsPerSite -v -count=1`
Expected: FAIL -- `emit-beliefs` is not defined

- [ ] **Step 3: Write the failing test for aggregate emission**

In `goast/belief_integration_test.go`, add:

```go
func TestEmitBeliefsAggregate(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast utils))
		(reset-beliefs!)

		(define-aggregate-belief "test-cohesion"
			(sites (all-functions-in))
			(analyze (aggregate-custom (lambda (sites ctx)
				(list (cons 'verdict 'TEST-OK))))))

		(define results
		  (run-beliefs "github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing"))
		(define emitted (emit-beliefs results))
	`)

	c := qt.New(t)

	t.Run("contains define-aggregate-belief", func(t *testing.T) {
		result := eval(t, engine, `(string-contains emitted "define-aggregate-belief")`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("contains belief name", func(t *testing.T) {
		result := eval(t, engine, `(string-contains emitted "test-cohesion")`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("contains sites expression", func(t *testing.T) {
		result := eval(t, engine, `(string-contains emitted "all-functions-in")`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("contains analyze expression", func(t *testing.T) {
		result := eval(t, engine, `(string-contains emitted "aggregate-custom")`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

- [ ] **Step 4: Write the failing test for filtering**

In `goast/belief_integration_test.go`, add:

```go
func TestEmitBeliefsFiltering(t *testing.T) {
	engine := newBeliefEngine(t)

	// Register three beliefs:
	// - strong (>= threshold): should be emitted
	// - weak (< threshold): should NOT be emitted
	// - no-sites: should NOT be emitted
	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile goast belief))
		(reset-beliefs!)

		(define-belief "strong-one"
		  (sites (functions-matching
		           (all-of (contains-call "Validate") (contains-call "Process"))))
		  (expect (ordered "Validate" "Process"))
		  (threshold 0.60 3))

		(define-belief "weak-one"
		  (sites (functions-matching
		           (all-of (contains-call "Validate") (contains-call "Process"))))
		  (expect (ordered "Validate" "Process"))
		  (threshold 0.99 3))

		(define-belief "empty-one"
		  (sites (functions-matching (name-matches "ZZZZZ_NO_MATCH")))
		  (expect (ordered "Validate" "Process"))
		  (threshold 0.50 1))

		(define results
		  (run-beliefs
		    "github.com/aalpar/wile-goast/examples/goast-query/testdata/ordering"))
		(define emitted (emit-beliefs results))
	`)

	c := qt.New(t)

	t.Run("strong belief emitted", func(t *testing.T) {
		result := eval(t, engine, `(string-contains emitted "strong-one")`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("weak belief not emitted", func(t *testing.T) {
		result := eval(t, engine, `(string-contains emitted "weak-one")`)
		c.Assert(result.SchemeString(), qt.Equals, "#f")
	})

	t.Run("empty belief not emitted", func(t *testing.T) {
		result := eval(t, engine, `(string-contains emitted "empty-one")`)
		c.Assert(result.SchemeString(), qt.Equals, "#f")
	})
}
```

- [ ] **Step 5: Implement emit-beliefs**

In `cmd/wile-goast/lib/wile/goast/belief.scm`, add after the `run-beliefs` procedure:

```scheme
;; -- Emit mode -----------------------------------------

;; Format a Scheme value as a string suitable for source output.
;; Uses write (not display) so strings get quoted.
(define (write-to-string val)
  (let ((port (open-output-string)))
    (write val port)
    (get-output-string port)))

;; Emit define-belief and define-aggregate-belief forms for
;; strong per-site beliefs and ok aggregate beliefs.
;; Returns a string of Scheme source code.
(define (emit-beliefs results)
  (let ((port (open-output-string)))
    (let loop ((rs results))
      (cond
        ((null? rs)
         (get-output-string port))
        (else
         (let* ((r (car rs))
                (type (cdr (assoc 'type r)))
                (status (cdr (assoc 'status r))))
           (cond
             ;; Per-site: emit only strong beliefs
             ((and (eq? type 'per-site) (eq? status 'strong))
              (emit-per-site-belief r port)
              (loop (cdr rs)))
             ;; Aggregate: emit only ok beliefs
             ((and (eq? type 'aggregate) (eq? status 'ok))
              (emit-aggregate-belief r port)
              (loop (cdr rs)))
             ;; Skip weak, no-sites, error
             (else (loop (cdr rs))))))))))

(define (emit-per-site-belief r port)
  (let ((name (cdr (assoc 'name r)))
        (pattern (cdr (assoc 'pattern r)))
        (ratio (cdr (assoc 'ratio r)))
        (total (cdr (assoc 'total r)))
        (deviations (cdr (assoc 'deviations r)))
        (sites-expr (cdr (assoc 'sites-expr r)))
        (expect-expr (cdr (assoc 'expect-expr r))))
    ;; Comment header
    (display ";; " port) (display name port) (newline port)
    (display ";; Adherence: " port)
    (display (exact->inexact ratio) port)
    (display " (" port) (display (- total (length deviations)) port)
    (display "/" port) (display total port) (display ")" port)
    (display ", Pattern: " port) (display pattern port) (newline port)
    (when (pair? deviations)
      (display ";; Deviations: " port)
      (display (string-join (map (lambda (d) (car d)) deviations) ", ") port)
      (newline port))
    (display ";;" port) (newline port)
    ;; Form
    (display "(define-belief " port) (write name port) (newline port)
    (display "  " port) (write sites-expr port) (newline port)
    (display "  " port) (write expect-expr port) (newline port)
    (display "  (threshold " port)
    (display (exact->inexact ratio) port)
    (display " " port) (display total port)
    (display "))" port) (newline port)
    (newline port)))

(define (emit-aggregate-belief r port)
  (let ((name (cdr (assoc 'name r)))
        (sites-expr (cdr (assoc 'sites-expr r)))
        (analyze-expr (cdr (assoc 'analyze-expr r))))
    ;; Comment header
    (display ";; " port) (display name port) (newline port)
    (display ";; Status: ok" port) (newline port)
    (display ";;" port) (newline port)
    ;; Form
    (display "(define-aggregate-belief " port) (write name port) (newline port)
    (display "  " port) (write sites-expr port) (newline port)
    (display "  " port) (write analyze-expr port) (display ")" port) (newline port)
    (newline port)))

;; Join a list of strings with a separator.
(define (string-join strs sep)
  (if (null? strs) ""
    (let loop ((rest (cdr strs)) (acc (car strs)))
      (if (null? rest) acc
        (loop (cdr rest)
              (string-append acc sep (car rest)))))))
```

- [ ] **Step 6: Export emit-beliefs from belief.sld**

In `cmd/wile-goast/lib/wile/goast/belief.sld`, add `emit-beliefs` to the export list. Add it after `run-beliefs` on the `;; Core` line:

```scheme
  (export
    ;; Core
    define-belief run-beliefs reset-beliefs! *beliefs* emit-beliefs
    ...
```

- [ ] **Step 7: Run all three new tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestEmitBeliefs|TestBeliefExpressionMetadata|TestAggregateBeliefExpressionMetadata" -v -count=1`
Expected: All PASS

- [ ] **Step 8: Run full belief test suite**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestBelief|TestAggregate|TestEmit" -v -count=1`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/belief.scm cmd/wile-goast/lib/wile/goast/belief.sld goast/belief_integration_test.go
git commit -m "belief: add emit-beliefs for discovery lifecycle

emit-beliefs takes run-beliefs output and produces Scheme source code:
define-belief forms for strong per-site beliefs, define-aggregate-belief
forms for ok aggregates. Skips weak, no-sites, and error results."
```

---

### Task 4: Verify full test suite and update docs

**Files:**
- Modify: `CLAUDE.md` (update exports table)
- Modify: `plans/BELIEF-DSL.md` (mark emit as done)

- [ ] **Step 1: Run full CI**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: All passes (lint + build + test + covercheck + verify-mod)

- [ ] **Step 2: Update CLAUDE.md**

In `CLAUDE.md`, find the Belief DSL section's export table and add `emit-beliefs`:

```markdown
| `emit-beliefs` | Format strong/ok belief results as Scheme source code |
```

- [ ] **Step 3: Update plans/BELIEF-DSL.md**

Mark the emit mode section as done. Change the header to:

```markdown
> **Incomplete items:**
> 1. ~~**Discovery `--emit` mode**~~ -- DONE (v0.5.x)
> 2. **Suppression** -- diff discovery output against committed belief files (below)
```

- [ ] **Step 4: Update plans/CLAUDE.md**

Update the active plan files table entry for BELIEF-DSL.md:

```markdown
| `BELIEF-DSL.md` | Belief DSL: suppression | Suppression open (emit done) |
```

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md plans/BELIEF-DSL.md plans/CLAUDE.md
git commit -m "docs: update for emit-beliefs completion"
```

---

## Notes

**`string-contains` dependency:** The `string-contains` helper already exists in `belief.scm` (line 484). The new tests and `emit-beliefs` both use it. No new dependency needed.

**`exact->inexact` for ratio display:** `run-beliefs` stores ratio as an exact rational (e.g., `4/5`). The emitted threshold uses `exact->inexact` to produce `0.8` -- human-readable and valid Scheme. The emitted `min-sites` uses the actual total from the discovery run.

**`string-join` helper:** Added locally in `belief.scm` rather than importing from `utils.scm`. It is 5 lines and avoids coupling utils to a display concern. If `string-join` already exists in `(wile goast utils)`, the local version can be removed.

**`when` usage:** The `(when (pair? deviations) ...)` form requires `(scheme base)` which is built-in in Wile. No import needed.

**Macro quoting in syntax-rules:** Pattern variables in `syntax-rules` templates are substituted at the syntax level, before `quote` is processed. So `'(sites selector)` correctly produces the quoted source form of the selector expression, not the literal symbols `sites` and `selector`.
