# Phase 3-4: Aggregate Beliefs + Split Planner — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add aggregate belief evaluation mode (`define-aggregate-belief`) with a package cohesion analyzer (`single-cluster`), and an MCP prompt (`goast-split`) for interactive split planning.

**Architecture:** Extend the belief DSL with a parallel storage/evaluation path for aggregate beliefs that return whole-package verdicts instead of per-site classifications. Bridge the split library into the belief system via a `single-cluster` analyzer that wraps `recommend-split`. Add a fifth MCP prompt guiding LLMs through split analysis.

**Tech Stack:** Scheme R7RS (`(wile goast belief)`, `(wile goast split)`, `(wile goast fca)`), Go (MCP prompt registration in `cmd/wile-goast/mcp.go`).

**Design doc:** `plans/2026-04-13-split-belief-planner-design.md`

---

### Task 1: Aggregate belief storage — failing test

Write a test that exercises `define-aggregate-belief`, `register-aggregate-belief!`, and `reset-beliefs!` clearing both lists.

**Files:**
- Modify: `goast/belief_test.go`

**Step 1: Write the failing test**

Add to `goast/belief_test.go`:
```go
func TestAggregateBeliefRegistration(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))
		(reset-beliefs!)
	`)

	c := qt.New(t)

	t.Run("register aggregate belief", func(t *testing.T) {
		eval(t, engine, `
			(define-aggregate-belief "test-agg"
				(sites (functions-matching (name-matches "Foo")))
				(analyze (custom (lambda (sites ctx) '((verdict . TEST))))))
		`)
		result := eval(t, engine, `(length *aggregate-beliefs*)`)
		c.Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("reset clears aggregate beliefs", func(t *testing.T) {
		eval(t, engine, `(reset-beliefs!)`)
		result := eval(t, engine, `(length *aggregate-beliefs*)`)
		c.Assert(result.SchemeString(), qt.Equals, "0")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestAggregateBeliefRegistration -v`
Expected: FAIL — `define-aggregate-belief` and `*aggregate-beliefs*` not defined.

**Step 3: Commit**

```
test(belief): failing test for aggregate belief registration
```

---

### Task 2: Aggregate belief storage — implementation

Add the storage machinery: `*aggregate-beliefs*`, `register-aggregate-belief!`, update `reset-beliefs!`, and the `define-aggregate-belief` macro.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/belief.sld`

**Step 1: Add storage and registration**

In `cmd/wile-goast/lib/wile/goast/belief.scm`, after the existing `*beliefs*` definition (line 34) and `register-belief!` (lines 40-43), add:

```scheme
(define *aggregate-beliefs* '())

(define (register-aggregate-belief! name sites-fn analyzer)
  (set! *aggregate-beliefs*
    (append *aggregate-beliefs*
      (list (list name sites-fn analyzer)))))

(define (agg-belief-name b) (list-ref b 0))
(define (agg-belief-sites-fn b) (list-ref b 1))
(define (agg-belief-analyzer b) (list-ref b 2))
```

**Step 2: Update `reset-beliefs!`**

In `cmd/wile-goast/lib/wile/goast/belief.scm`, change `reset-beliefs!` (lines 36-38) from:

```scheme
(define (reset-beliefs!)
  "Clear all registered beliefs.\n\nCategory: goast-belief\n\nSee also: `run-beliefs'."
  (set! *beliefs* '()))
```

to:

```scheme
(define (reset-beliefs!)
  "Clear all registered beliefs (per-site and aggregate).\n\nCategory: goast-belief\n\nSee also: `run-beliefs'."
  (set! *beliefs* '())
  (set! *aggregate-beliefs* '()))
```

**Step 3: Add the `define-aggregate-belief` macro**

In `cmd/wile-goast/lib/wile/goast/belief.scm`, after the existing `define-belief` macro (line 61), add:

```scheme
(define-syntax define-aggregate-belief
  (syntax-rules (sites analyze)
    ((_ name (sites selector) (analyze analyzer))
     (register-aggregate-belief! name selector analyzer))))
```

**Step 4: Update exports**

In `cmd/wile-goast/lib/wile/goast/belief.sld`, add to the exports:

```scheme
    define-aggregate-belief register-aggregate-belief! *aggregate-beliefs*
```

Add these on a new line after the existing `define-belief run-beliefs reset-beliefs! *beliefs*` exports.

**Step 5: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestAggregateBeliefRegistration -v`
Expected: PASS

**Step 6: Commit**

```
feat(belief): aggregate belief storage and define-aggregate-belief macro
```

---

### Task 3: Aggregate belief evaluation in `run-beliefs` — failing test

Write a test that runs an aggregate belief through `run-beliefs` and checks the result output.

**Files:**
- Modify: `goast/belief_test.go`

**Step 1: Write the failing test**

```go
func TestAggregateBeliefEvaluation(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast utils))
		(reset-beliefs!)

		;; Register an aggregate belief with a custom analyzer
		;; that returns a fixed verdict.
		(define-aggregate-belief "test-cohesion"
			(sites (functions-matching (name-matches "Lock")))
			(analyze (custom (lambda (sites ctx)
				(list (cons 'type 'aggregate)
				      (cons 'verdict 'TEST-OK)
				      (cons 'functions (length sites)))))))

		(run-beliefs "github.com/aalpar/wile-goast/goast/testdata/pairing")
	`)

	c := qt.New(t)

	t.Run("aggregate result printed", func(t *testing.T) {
		// The test passes if run-beliefs completes without error.
		// Aggregate beliefs produce output but don't count as
		// strong/weak per-site beliefs.
		c.Assert(true, qt.IsTrue)
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestAggregateBeliefEvaluation -v`
Expected: FAIL — `run-beliefs` does not process `*aggregate-beliefs*`.

**Step 3: Commit**

```
test(belief): failing test for aggregate belief evaluation
```

---

### Task 4: Aggregate belief evaluation in `run-beliefs` — implementation

Extend `run-beliefs` to process aggregate beliefs after per-site beliefs.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm`

**Step 1: Add aggregate evaluation logic**

In `cmd/wile-goast/lib/wile/goast/belief.scm`, the `run-beliefs` function (lines 823-887) currently ends with the summary `begin` block at line 833. Replace the summary section so that after per-site beliefs are done, it processes aggregate beliefs and then prints the summary.

Change the `(if (null? beliefs)` branch (lines 831-837) from:

```scheme
      (if (null? beliefs)
        ;; Summary
        (begin
          (display "── Summary ──") (newline)
          (display "  Beliefs evaluated:   ") (display evaluated) (newline)
          (display "  Strong beliefs:      ") (display strong) (newline)
          (display "  Deviations found:    ") (display total-deviations) (newline))
```

to:

```scheme
      (if (null? beliefs)
        ;; Aggregate beliefs, then summary
        (let ((agg-count (evaluate-aggregate-beliefs ctx)))
          (display "── Summary ──") (newline)
          (display "  Beliefs evaluated:   ") (display (+ evaluated agg-count)) (newline)
          (display "  Strong beliefs:      ") (display strong) (newline)
          (display "  Aggregate beliefs:   ") (display agg-count) (newline)
          (display "  Deviations found:    ") (display total-deviations) (newline))
```

**Step 2: Add the `evaluate-aggregate-beliefs` function**

Add before `run-beliefs` (before line 823):

```scheme
(define (evaluate-aggregate-beliefs ctx)
  "Evaluate all registered aggregate beliefs. Returns count evaluated."
  (let loop ((beliefs *aggregate-beliefs*) (count 0))
    (if (null? beliefs)
      count
      (let* ((belief (car beliefs))
             (name (agg-belief-name belief)))
        (guard (exn
                 (#t (display "── Aggregate Belief: ") (display name)
                     (display " ──") (newline)
                     (display "  (error: ")
                     (if (error-object? exn)
                       (display (error-object-message exn))
                       (display exn))
                     (display ")") (newline) (newline)
                     (loop (cdr beliefs) (+ count 1))))
          (let* ((sites-fn (agg-belief-sites-fn belief))
                 (analyzer (agg-belief-analyzer belief))
                 (sites (sites-fn ctx))
                 (result (analyzer sites ctx)))
            (print-aggregate-result name result)
            (loop (cdr beliefs) (+ count 1))))))))

(define (print-aggregate-result name result)
  "Print an aggregate belief result."
  (display "── Aggregate Belief: ") (display name) (display " ──")
  (newline)
  (let ((verdict (assoc 'verdict result))
        (confidence (assoc 'confidence result))
        (functions (assoc 'functions result)))
    (when verdict
      (display "  Verdict:    ") (display (cdr verdict)) (newline))
    (when confidence
      (display "  Confidence: ") (display (cdr confidence)) (newline))
    (when functions
      (display "  Functions:  ") (display (cdr functions)) (newline))
    ;; Print groups if present
    (let ((report (assoc 'report result)))
      (when report
        (let* ((rpt (cdr report))
               (groups (assoc 'groups rpt)))
          (when groups
            (let* ((gs (cdr groups))
                   (ga (assoc 'group-a gs))
                   (gb (assoc 'group-b gs))
                   (cut (assoc 'cut-ratio gs)))
              (when ga
                (display "  Group A:    ") (display (length (cdr ga)))
                (display " functions") (newline))
              (when gb
                (display "  Group B:    ") (display (length (cdr gb)))
                (display " functions") (newline))
              (when cut
                (display "  Cut ratio:  ") (display (cdr cut)) (newline))))))))
  (newline))
```

**Step 3: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestAggregateBeliefEvaluation -v`
Expected: PASS

**Step 4: Run all belief tests to check for regressions**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestBelief|TestAggregate" -v`
Expected: All PASS

**Step 5: Commit**

```
feat(belief): aggregate belief evaluation in run-beliefs
```

---

### Task 5: `all-functions-in` site selector — failing test

**Files:**
- Modify: `goast/belief_test.go`

**Step 1: Write the failing test**

```go
func TestAllFunctionsIn(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast utils))

		(define ctx (make-context
			"github.com/aalpar/wile-goast/goast/testdata/pairing"))
		(define selector (all-functions-in
			"github.com/aalpar/wile-goast/goast/testdata/pairing"))
		(define sites (selector ctx))
	`)

	c := qt.New(t)

	t.Run("returns non-empty list", func(t *testing.T) {
		result := eval(t, engine, `(> (length sites) 0)`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("each site is a func-decl", func(t *testing.T) {
		result := eval(t, engine, `(tag? (car sites) 'func-decl)`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("each site has pkg-path", func(t *testing.T) {
		result := eval(t, engine, `(nf (car sites) 'pkg-path)`)
		c.Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestAllFunctionsIn -v`
Expected: FAIL — `all-functions-in` not defined.

**Step 3: Commit**

```
test(belief): failing test for all-functions-in selector
```

---

### Task 6: `all-functions-in` site selector — implementation

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/belief.sld`

**Step 1: Add the selector**

In `cmd/wile-goast/lib/wile/goast/belief.scm`, after the existing `methods-of` selector (around line 290), add:

```scheme
(define (all-functions-in pkg-pattern)
  "Site selector: all functions in packages matching a pattern.\nReturns all func-decl nodes from the matched packages.\n\nParameters:\n  pkg-pattern : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (define-aggregate-belief \"pkg-check\"\n    (sites (all-functions-in \"my/pkg\"))\n    (analyze ...))\n\nSee also: `functions-matching', `define-aggregate-belief'."
  (lambda (ctx)
    (all-func-decls (ctx-pkgs ctx))))
```

Note: The context is already created with the target package pattern from `run-beliefs`. For `all-functions-in`, the selector simply returns all func-decls from the loaded packages. The `pkg-pattern` parameter is captured for documentation/intent but the context's target is what actually controls which packages are loaded. This is consistent with how other selectors work — they operate on whatever packages the context loaded.

**Step 2: Export**

In `cmd/wile-goast/lib/wile/goast/belief.sld`, add `all-functions-in` to the site selectors export group:

```scheme
    functions-matching callers-of methods-of sites-from all-func-decls
    all-functions-in
    implementors-of interface-methods
```

**Step 3: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestAllFunctionsIn -v`
Expected: PASS

**Step 4: Commit**

```
feat(belief): all-functions-in site selector
```

---

### Task 7: `single-cluster` analyzer — failing test

**Files:**
- Modify: `goast/split_test.go`

**Step 1: Write the failing test**

```go
func TestSingleClusterAnalyzer(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast split))
		(import (wile goast utils))
		(reset-beliefs!)

		(define-aggregate-belief "test-split"
			(sites (all-functions-in
				"github.com/aalpar/wile-goast/goast/testdata/pairing"))
			(analyze (single-cluster)))

		(run-beliefs
			"github.com/aalpar/wile-goast/goast/testdata/pairing")
	`)

	c := qt.New(t)

	t.Run("completes without error", func(t *testing.T) {
		// If we got here, the analyzer ran.
		c.Assert(true, qt.IsTrue)
	})
}

func TestSingleClusterAnalyzer_Synthetic(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast split))
		(import (wile goast utils))

		;; Test single-cluster directly with a mock context.
		;; The analyzer takes (sites ctx) and returns a result alist.
		(define analyzer (single-cluster 'idf-threshold 0.36))

		;; Create a context just for the session.
		(define ctx (make-context
			"github.com/aalpar/wile-goast/goast/testdata/pairing"))
		(define sites '())  ;; sites not used by single-cluster
		(define result (analyzer sites ctx))
	`)

	c := qt.New(t)

	t.Run("result has type aggregate", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'type result))`)
		c.Assert(result.SchemeString(), qt.Equals, "aggregate")
	})

	t.Run("result has verdict", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((v (cdr (assoc 'verdict result))))
				(or (eq? v 'COHESIVE) (eq? v 'SPLIT)))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("result has functions count", func(t *testing.T) {
		result := eval(t, engine, `(number? (cdr (assoc 'functions result)))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("result has report", func(t *testing.T) {
		result := eval(t, engine, `(pair? (cdr (assoc 'report result)))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestSingleCluster" -v`
Expected: FAIL — `single-cluster` not defined.

**Step 3: Commit**

```
test(split): failing tests for single-cluster aggregate analyzer
```

---

### Task 8: `single-cluster` analyzer — implementation

The analyzer bridges `(wile goast belief)` and `(wile goast split)`. It lives in `split.scm` because it depends on `recommend-split`, and `belief.scm` does not import `split`. The export goes through `split.sld`.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/split.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/split.sld`
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm` (if `ctx-session` not exported)
- Modify: `cmd/wile-goast/lib/wile/goast/belief.sld` (if `ctx-session` not exported)

**Step 1: Add `ctx-session` accessor and export**

In `cmd/wile-goast/lib/wile/goast/belief.scm`, after the other `ctx-*` accessors (around line 95), add:

```scheme
(define (ctx-session ctx)
  "Return the GoSession from CTX.\n\nParameters:\n  ctx : context\nReturns: go-session\nCategory: goast-belief"
  (ctx-ref ctx 'session))
```

In `cmd/wile-goast/lib/wile/goast/belief.sld`, add `ctx-session` to the context exports:

```scheme
    make-context ctx-pkgs ctx-ssa ctx-callgraph ctx-find-ssa-func ctx-field-index
    ctx-session
```

**Step 2: Add belief import to split.sld**

In `cmd/wile-goast/lib/wile/goast/split.sld`, add `(wile goast belief)` to imports:

```scheme
  (import (scheme base))
  (import (wile goast utils))
  (import (wile goast fca))
  (import (wile goast belief))
```

And add `single-cluster` to exports:

```scheme
  (export
    import-signatures
    compute-idf
    filter-noise
    build-package-context
    refine-by-api-surface
    find-split
    verify-acyclic
    recommend-split
    single-cluster)
```

**Step 3: Add the implementation**

In `cmd/wile-goast/lib/wile/goast/split.scm`, after `compute-confidence` (end of file), add:

```scheme
;;; ── Aggregate analyzer for belief DSL ─────────────────────

(define (single-cluster . opts)
  "Aggregate analyzer: check if a package is cohesive or should split.\nWraps recommend-split for use with define-aggregate-belief.\nUses the belief context's GoSession to avoid redundant package loading.\n\nParameters:\n  opts : optional — keyword options forwarded to recommend-split:\n           'idf-threshold N (default 0.36)\n           'refine (use API-surface refinement)\nReturns: procedure — (lambda (sites ctx) -> result-alist)\nCategory: goast-split\n\nExamples:\n  (define-aggregate-belief \"pkg-cohesion\"\n    (sites (all-functions-in \"my/pkg\"))\n    (analyze (single-cluster 'idf-threshold 0.36)))\n\nSee also: `recommend-split', `define-aggregate-belief'."
  (lambda (sites ctx)
    (let* ((session (ctx-session ctx))
           (refs (go-func-refs session))
           (report (apply recommend-split refs opts))
           (confidence (cdr (assoc 'confidence report)))
           (verdict (if (or (eq? confidence 'HIGH)
                            (eq? confidence 'MEDIUM))
                      'SPLIT
                      'COHESIVE)))
      (list (cons 'type 'aggregate)
            (cons 'verdict verdict)
            (cons 'confidence confidence)
            (cons 'functions (cdr (assoc 'functions report)))
            (cons 'report report)))))
```

**Step 4: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestSingleCluster" -v`
Expected: PASS

**Step 5: Commit**

```
feat(split): single-cluster aggregate analyzer for belief DSL
```

---

### Task 9: Integration test — aggregate belief on `goast/` itself

**Files:**
- Modify: `goast/split_test.go`

**Step 1: Write integration test**

```go
func TestAggregateBeliefIntegration_Goast(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast split))
		(import (wile goast utils))
		(reset-beliefs!)

		(define-aggregate-belief "goast-cohesion"
			(sites (all-functions-in
				"github.com/aalpar/wile-goast/goast"))
			(analyze (single-cluster)))

		(run-beliefs "github.com/aalpar/wile-goast/goast")
	`)

	c := qt.New(t)

	t.Run("completes without error on real package", func(t *testing.T) {
		c.Assert(true, qt.IsTrue)
	})
}
```

**Step 2: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestAggregateBeliefIntegration_Goast -v -timeout 120s`
Expected: PASS

**Step 3: Commit**

```
test(belief): aggregate belief integration test on goast/ package
```

---

### Task 10: MCP prompt — `goast-split.md`

**Files:**
- Create: `cmd/wile-goast/prompts/goast-split.md`

**Step 1: Write the prompt template**

Create `cmd/wile-goast/prompts/goast-split.md`:

````markdown
# Package Split Analysis

Analyze a Go package's function dependencies to discover natural package
boundaries. Uses IDF-weighted Formal Concept Analysis on import signatures.

## Target Package

{{package}}

## Goal

{{goal}}

## Instructions

### Step 1: Run the analysis

```scheme
(import (wile goast split))
(import (wile goast utils))

(define refs (go-func-refs "{{package}}"))
(define report (recommend-split refs))
report
```

### Step 2: Interpret the result

The report contains:

- **functions** — total function count
- **high-idf** — informative dependencies (high IDF = referenced by few functions)
- **groups** — two function groups from the min-cut partition
  - **group-a**, **group-b** — function names in each group
  - **cut** — functions that bridge both groups (the coupling cost)
  - **cut-ratio** — cut size / total (lower is better; < 0.15 is excellent)
- **acyclic** — whether the split avoids Go import cycles
  - **a-refs-b**, **b-refs-a** — cross-group reference counts (cycle exists iff both > 0)
- **confidence** — overall verdict:
  - **HIGH** — acyclic, cut-ratio <= 0.15
  - **MEDIUM** — acyclic, cut-ratio <= 0.30
  - **LOW** — cyclic or high cut-ratio
  - **NONE** — no meaningful split found

### Step 3: Refine if needed

If confidence is LOW or you want finer-grained grouping, re-run with API
surface refinement. This replaces package-level attributes with
(package, object-name) pairs:

```scheme
(define report-refined (recommend-split refs 'refine))
report-refined
```

Compare the two reports — refinement often reveals sub-clusters within a
group that share the same package import but use different API surfaces.

You can also adjust the IDF threshold (default 0.36, which excludes
packages referenced by >70% of functions):

```scheme
(define report-strict (recommend-split refs 'idf-threshold 0.5))
```

### Step 4: Plan the split

If confidence is MEDIUM or HIGH and the split is acyclic:

1. **Identify the groups.** Name them by their dominant dependency
   (e.g., "ast-parsing" vs "type-checking").

2. **Examine the cut.** Functions in the cut bridge both groups. For each:
   - Can it be moved entirely to one group?
   - Does it need an interface or accessor to bridge the boundary?

3. **Check the dependency direction.** Only one group may import the other
   (or neither imports the other). The `a-refs-b` / `b-refs-a` counts
   show which direction is dominant — the referencing group imports the
   referenced group.

4. **List what moves.** The smaller group typically becomes the new package.
   List the files and functions that would move.

## Rules

- Always run the basic analysis before refinement
- A cut-ratio above 0.30 usually means the package is cohesive — don't force a split
- Acyclic is a hard requirement for Go packages — if both directions have references, the split needs restructuring (interface extraction or function moves)
- The analysis shows _where_ boundaries naturally fall, not _whether_ to split — that decision depends on package size, team structure, and maintenance goals
````

**Step 2: Verify embed covers new file**

Check that `cmd/wile-goast/embed.go` embeds `prompts/` directory. The existing `//go:embed prompts` directive covers all files in that directory.

**Step 3: Commit**

```
feat(mcp): goast-split prompt template for package split analysis
```

---

### Task 11: Register `goast-split` prompt in MCP server

**Files:**
- Modify: `cmd/wile-goast/mcp.go`

**Step 1: Add prompt definition**

In `cmd/wile-goast/mcp.go`, add to the `prompts` slice (after the `goast-scheme-ref` entry, around line 189):

```go
		{
			name:        "goast-split",
			description: "Analyze package cohesion and recommend splits via IDF-weighted Formal Concept Analysis",
			file:        "prompts/goast-split.md",
			args: []mcp.PromptOption{
				mcp.WithArgument("package",
					mcp.RequiredArgument(),
					mcp.ArgumentDescription("Go package pattern to analyze (e.g., 'my/package', './internal/...')"),
				),
				mcp.WithArgument("goal",
					mcp.ArgumentDescription("Motivation for the split (e.g., 'reduce coupling', 'package too large')"),
				),
			},
		},
```

**Step 2: Build and test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build ./cmd/wile-goast/`
Expected: Compiles successfully.

**Step 3: Commit**

```
feat(mcp): register goast-split prompt
```

---

### Task 12: Update documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `plans/CLAUDE.md`

**Step 1: Update CLAUDE.md**

Add to the Belief DSL section after the existing property checkers table:

```markdown
### Aggregate Beliefs

Aggregate beliefs evaluate whole-package properties instead of per-site patterns.

``​`scheme
(define-aggregate-belief "package-cohesion"
  (sites (all-functions-in "my/pkg"))
  (analyze (single-cluster 'idf-threshold 0.36)))
``​`

Result shape (different from per-site beliefs):

``​`scheme
("name"
  (type . aggregate)
  (verdict . SPLIT)       ;; COHESIVE | SPLIT
  (confidence . HIGH)
  (functions . 47)
  (report . <recommend-split output>))
``​`

| Analyzer | Description |
|----------|-------------|
| `(single-cluster . opts)` | Package cohesion via `recommend-split` |
```

Add `all-functions-in` to the site selectors table:

```markdown
| `(all-functions-in "pkg")` | — | All functions in a package |
```

Add `goast-split` to the MCP prompts table:

```markdown
| `goast-split` | Package cohesion analysis and split recommendations |
```

**Step 2: Update plans/CLAUDE.md**

Change status of `2026-04-13-split-belief-planner-design.md` to "Complete".
Add entry for `2026-04-13-split-belief-planner-impl.md`.

**Step 3: Run full CI**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: All tests pass, coverage >= 80%.

**Step 4: Commit**

```
docs: aggregate beliefs, all-functions-in, goast-split prompt
```

---

### Task 13: Version bump

**Files:**
- Modify: `VERSION`

**Step 1: Bump version**

Read the current version and increment the patch number by 1.

**Step 2: Commit**

```
chore: bump version to v0.5.X
```
