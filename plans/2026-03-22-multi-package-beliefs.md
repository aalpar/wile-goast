# Multi-Package Belief Context Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable the belief DSL to evaluate SSA-based beliefs across multiple packages loaded via `...` glob patterns.

**Architecture:** All changes are in `cmd/wile-goast/lib/wile/goast/belief.scm` (the Scheme-side belief DSL). Sites get annotated with their package path during extraction. The SSA index becomes a two-level structure keyed by `(pkg-path, short-name)`. No Go code changes. Backward-compatible with single-package usage.

**Tech Stack:** Scheme (R7RS), Wile engine, existing goast primitives

**Design doc:** `plans/MULTI-PACKAGE-BELIEFS.md`

---

### Task 1: Add `package-short-name` helper

Extracts the last path segment from a full import path. Used by display name and potentially by custom lambdas.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm` (after `string-contains`, ~line 289)

**Step 1: Add the helper function**

Insert after `string-contains` (line 289), before the `contains-call` definition:

```scheme
;; Extract short package name from full import path.
;; "k8s.io/kubernetes/pkg/kubelet/kuberuntime" -> "kuberuntime"
(define (package-short-name path)
  (let ((len (string-length path)))
    (let loop ((i (- len 1)))
      (cond ((<= i 0) path)
            ((char=? (string-ref path i) #\/)
             (substring path (+ i 1) len))
            (else (loop (- i 1)))))))
```

**Step 2: Run existing tests to verify no breakage**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBelief -v`
Expected: PASS — existing tests unchanged.

**Step 3: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/belief.scm
git commit -m "add: package-short-name helper for belief display"
```

---

### Task 2: Annotate func-decl sites with `pkg-path`

`all-func-decls` injects the parent package's `path` field into each func-decl node so SSA/CFG lookup can disambiguate across packages.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm:138-150` (`all-func-decls`)

**Step 1: Write a test that verifies site annotation**

Add to `goast/belief_integration_test.go`:

```go
func TestBeliefSiteAnnotation(t *testing.T) {
	engine := newBeliefEngine(t)

	// After importing the belief library, load a package and extract
	// func-decls via all-func-decls. Each site should have a pkg-path field.
	result := evalMultiple(t, engine, `
		(import (wile goast belief))

		(let* ((pkgs (go-typecheck-package "github.com/aalpar/wile-goast/goast"))
		       (funcs (all-func-decls pkgs)))
		  ;; Check that the first func-decl has a pkg-path field
		  (and (pair? funcs)
		       (nf (car funcs) 'pkg-path)))
	`)
	c := qt.New(t)
	c.Assert(result, qt.Not(qt.Equals), nil)
	// Result should be the package path string
	c.Assert(result.String(), qt.Matches, `.*wile-goast/goast.*`)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefSiteAnnotation -v`
Expected: FAIL — `nf` returns `#f` because `pkg-path` field doesn't exist yet.

**Step 3: Implement the annotation**

Replace `all-func-decls` (lines 138-150) with:

```scheme
;; Extract all func-decl nodes from a package list.
;; Each func-decl is annotated with (pkg-path . <import-path>) from its
;; parent package, enabling cross-package SSA/CFG disambiguation.
(define (all-func-decls pkgs)
  (flat-map
    (lambda (pkg)
      (let ((pkg-path (nf pkg 'path)))
        (flat-map
          (lambda (file)
            (filter-map
              (lambda (decl)
                (and (tag? decl 'func-decl)
                     (cons (car decl)
                           (cons (cons 'pkg-path pkg-path)
                                 (cdr decl)))))
              (let ((decls (nf file 'decls)))
                (if (pair? decls) decls '()))))
          (let ((files (nf pkg 'files)))
            (if (pair? files) files '())))))
    pkgs))
```

The annotation injects `(pkg-path . "full/import/path")` as the first field in the alist. Existing `nf` calls for `name`, `body`, `type`, etc. still work — `assoc` finds them past the new field.

**Step 4: Export `all-func-decls` from the library**

Modify `cmd/wile-goast/lib/wile/goast/belief.sld` — add `all-func-decls` to the export list after `sites-from`:

```scheme
    ;; Site selectors
    functions-matching callers-of methods-of sites-from all-func-decls
```

This enables the integration test to call `all-func-decls` directly.

**Step 5: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBelief -v`
Expected: ALL PASS — new test passes, existing tests unchanged.

**Step 6: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/belief.scm cmd/wile-goast/lib/wile/goast/belief.sld goast/belief_integration_test.go
git commit -m "add: annotate func-decl sites with pkg-path for multi-package beliefs"
```

---

### Task 3: Two-level SSA index and package-qualified lookup

Replace the flat short-name SSA index with a two-level structure keyed by `(pkg-path, short-name)`. Update `ctx-find-ssa-func` to accept both arguments.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm:101-117` (`ctx-ssa-index`, `ctx-find-ssa-func`)

**Step 1: Write a test for package-qualified SSA lookup**

Add to `goast/belief_integration_test.go`:

```go
func TestBeliefSSALookup(t *testing.T) {
	engine := newBeliefEngine(t)

	// Build SSA for the goast package. Look up a known function
	// by package path + short name. Should return the SSA function.
	result := evalMultiple(t, engine, `
		(import (wile goast belief))

		;; Force SSA and index construction via a belief that uses stores-to-fields.
		;; Instead, directly test ctx-find-ssa-func.
		(let ((ctx (make-context "github.com/aalpar/wile-goast/goast")))
		  ;; Trigger SSA build
		  (ctx-ssa ctx)
		  ;; Look up PrimGoParseFile by package path + short name
		  (let ((fn (ctx-find-ssa-func ctx
		              "github.com/aalpar/wile-goast/goast"
		              "PrimGoParseFile")))
		    (and fn (nf fn 'name))))
	`)
	c := qt.New(t)
	c.Assert(result, qt.Not(qt.Equals), nil)
	c.Assert(result.String(), qt.Matches, `.*PrimGoParseFile.*`)
}
```

**Step 2: Export `ctx-find-ssa-func` and `ctx-ssa`**

Modify `cmd/wile-goast/lib/wile/goast/belief.sld` — add to exports:

```scheme
    ;; Context (needed by custom lambdas)
    make-context ctx-pkgs ctx-ssa ctx-callgraph ctx-find-ssa-func
```

**Step 3: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefSSALookup -v`
Expected: FAIL — `ctx-find-ssa-func` currently takes 2 args (ctx, name), not 3.

**Step 4: Replace the SSA index implementation**

Replace lines 101-117 (`ctx-ssa-index`, `ctx-find-ssa-func`) with:

```scheme
;; SSA name lookup index — two-level, keyed by (pkg-path, short-name).
;; First level: package path -> alist of (short-name . ssa-func).
;; Methods like (*Debugger).Continue are indexed as "Continue" within
;; their package's sub-index.
(define (build-ssa-index ssa-funcs)
  (let loop ((fns (if (pair? ssa-funcs) ssa-funcs '()))
             (index '()))
    (if (null? fns) index
      (let* ((fn (car fns))
             (name (nf fn 'name))
             (pkg (nf fn 'pkg))
             (short (and name (ssa-short-name name))))
        (if (and short pkg)
          (let ((pkg-entry (assoc pkg index)))
            (if pkg-entry
              (begin
                (set-cdr! pkg-entry
                  (cons (cons short fn) (cdr pkg-entry)))
                (loop (cdr fns) index))
              (loop (cdr fns)
                    (cons (list pkg (cons short fn)) index))))
          (loop (cdr fns) index))))))

(define (ctx-ssa-index ctx)
  (or (ctx-ref ctx 'ssa-index)
      (let ((index (build-ssa-index (ctx-ssa ctx))))
        (ctx-set! ctx 'ssa-index index)
        index)))

;; Package-qualified SSA function lookup.
;; Returns the SSA function for the given package path and short name,
;; or #f if not found.
(define (ctx-find-ssa-func ctx pkg-path name)
  (let ((pkg-entry (assoc pkg-path (ctx-ssa-index ctx))))
    (and pkg-entry
         (let ((entry (assoc name (cdr pkg-entry))))
           (and entry (cdr entry))))))
```

**Step 5: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBelief -v`
Expected: `TestBeliefSSALookup` PASSES. `TestBeliefDefineAndRun` PASSES (it uses `custom` checker, not SSA). `TestBeliefSiteAnnotation` PASSES.

**Step 6: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/belief.scm cmd/wile-goast/lib/wile/goast/belief.sld goast/belief_integration_test.go
git commit -m "add: two-level SSA index keyed by (pkg-path, short-name)"
```

---

### Task 4: Update SSA-dependent checkers and predicates

Three functions call `ctx-find-ssa-func` with the old 2-arg signature: `stores-to-fields`, `co-mutated`, and `checked-before-use`. Update each to extract `pkg-path` from the annotated site.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm` — three functions

**Step 1: Update `stores-to-fields`**

Replace the `stores-to-fields` definition (lines 312-320):

```scheme
;; (stores-to-fields struct-name field ...) — SSA: function stores to
;; these fields. Requires SSA layer.
(define (stores-to-fields struct-name . field-names)
  (lambda (fn ctx)
    (let* ((fname (nf fn 'name))
           (pkg-path (nf fn 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if ssa-fn
        (let* ((all-fields (struct-field-names (ctx-pkgs ctx) struct-name))
               (stored (stored-fields-in-func ssa-fn field-names all-fields)))
          (pair? stored))
        #f))))
```

**Step 2: Update `co-mutated`**

Replace the `co-mutated` definition (lines 502-521):

```scheme
;; (co-mutated field ...) — checks whether all named fields are stored
;; together in the function.
;; Skips receiver-type disambiguation: the site selector (stores-to-fields)
;; already filtered to functions that store to this struct's fields.
;; Returns: 'co-mutated or 'partial
(define (co-mutated . field-names)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if (not ssa-fn) 'partial
        (let* ((all-field-addrs (collect-field-addrs ssa-fn))
               (stores (collect-stores ssa-fn))
               (store-addrs (map car stores))
               ;; Collect stored field names without disambiguation
               (stored (unique (filter-map
                         (lambda (fa)
                           (let ((reg (car fa))
                                 (field (cadr fa)))
                             (and (member? reg store-addrs)
                                  (member? field field-names)
                                  field)))
                         all-field-addrs))))
          (if (= (length stored) (length field-names))
            'co-mutated
            'partial))))))
```

**Step 3: Update `checked-before-use`**

Replace the `checked-before-use` definition (lines 526-548):

```scheme
;; (checked-before-use value-pattern) — checks whether a value is
;; tested before use via SSA + CFG dominance.
;; Returns: 'guarded or 'unguarded
(define (checked-before-use value-pattern)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if (not ssa-fn) 'unguarded
        (let* ((blocks (nf ssa-fn 'blocks))
               (all-instrs (if (pair? blocks)
                             (flat-map
                               (lambda (b) (let ((is (nf b 'instrs)))
                                             (if (pair? is) is '())))
                               blocks)
                             '()))
               (uses (filter-map
                       (lambda (instr)
                         (let ((ops (nf instr 'operands)))
                           (and (pair? ops) (member? value-pattern ops)
                                instr)))
                       all-instrs))
               (has-guard (let loop ((us uses))
                            (cond ((null? us) #f)
                                  ((tag? (car us) 'ssa-if) #t)
                                  (else (loop (cdr us)))))))
          (if has-guard 'guarded 'unguarded))))))
```

**Step 4: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBelief -v`
Expected: ALL PASS.

**Step 5: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/belief.scm
git commit -m "fix: update SSA checkers to use package-qualified lookup"
```

---

### Task 5: Fix `ordered` checker

The `ordered` checker calls `(go-cfg fname)` with one argument, but `go-cfg` requires two: `(go-cfg pattern func-name)`. Use the site's `pkg-path` as the pattern.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm:459-475` (`ordered`)

**Step 1: Fix the implementation**

Replace the `ordered` definition (lines 459-475):

```scheme
;; (ordered op-a op-b) — checks whether op-a's block dominates op-b's block.
;; Requires CFG layer.
;; Returns: 'a-dominates-b, 'b-dominates-a, 'same-block, 'unordered, or 'missing
(define (ordered op-a op-b)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (cfg (and pkg-path (go-cfg pkg-path fname))))
      (if (not cfg) 'missing
        (let* ((blocks (nf cfg 'blocks))
               (a-blocks (find-call-blocks blocks op-a))
               (b-blocks (find-call-blocks blocks op-b)))
          (cond
            ((or (null? a-blocks) (null? b-blocks)) 'missing)
            ((= (car a-blocks) (car b-blocks)) 'same-block)
            ((go-cfg-dominates? cfg (car a-blocks) (car b-blocks)) 'a-dominates-b)
            ((go-cfg-dominates? cfg (car b-blocks) (car a-blocks)) 'b-dominates-a)
            (else 'unordered)))))))
```

**Step 2: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBelief -v`
Expected: ALL PASS.

**Step 3: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/belief.scm
git commit -m "fix: ordered checker passes package pattern to go-cfg"
```

---

### Task 6: Package-qualified display names

Update `site-display-name` to show `pkg.FuncName` for func-decl sites so deviations from multi-package runs are locatable.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm:614-623` (`site-display-name`)

**Step 1: Update `site-display-name`**

Replace the `site-display-name` definition (lines 614-623):

```scheme
;; Extract a display name from a site (func-decl or caller edge).
(define (site-display-name site)
  (cond
    ((and (pair? site) (tag? site 'func-decl))
     (let ((name (or (nf site 'name) "<anonymous>"))
           (pkg-path (nf site 'pkg-path)))
       (if pkg-path
         (string-append (package-short-name pkg-path) "." name)
         name)))
    ((and (pair? site) (pair? (car site)))
     ;; Caller site: (caller-name edge)
     (let ((name (car site)))
       (if (string? name) name (display-to-string name))))
    (else
     (display-to-string site))))
```

**Step 2: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBelief -v`
Expected: ALL PASS. The `TestBeliefDefineAndRun` output now shows `goast.PrimGoParseFile` etc. in deviation lines, which doesn't affect test assertions (they only check for no errors).

**Step 3: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/belief.scm
git commit -m "add: package-qualified display names in belief output"
```

---

### Task 7: Multi-package integration test

Write a test that runs a belief across multiple packages simultaneously via a `...` pattern, verifying that sites from multiple packages are collected and that SSA lookup disambiguates correctly.

**Files:**
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the multi-package integration test**

```go
func TestBeliefMultiPackage(t *testing.T) {
	engine := newBeliefEngine(t)

	// Run a belief that matches functions named "Prim*" across both
	// goast and goastssa packages. The ... pattern loads both.
	// Verify the belief sees sites from multiple packages.
	result := evalMultiple(t, engine, `
		(import (wile goast belief))

		(define-belief "prim-has-body"
		  (sites (functions-matching (name-matches "Prim")))
		  (expect (custom (lambda (site ctx)
		    (if (nf site 'body) 'has-body 'no-body))))
		  (threshold 0.90 3))

		;; Count distinct pkg-path values across all matching sites.
		;; If multi-package works, we should see sites from at least
		;; goast and goastssa (both have Prim* functions).
		(let* ((ctx (make-context "github.com/aalpar/wile-goast/..."))
		       (funcs (all-func-decls (ctx-pkgs ctx)))
		       (prims (filter-map
		                (lambda (fn)
		                  (and (nf fn 'name)
		                       (string-contains (nf fn 'name) "Prim")
		                       (nf fn 'pkg-path)))
		                funcs))
		       (unique-pkgs (unique prims)))
		  (length unique-pkgs))
	`)
	c := qt.New(t)
	// Should find Prim* functions in at least 2 packages (goast, goastssa, goastcfg, etc.)
	c.Assert(result, qt.Not(qt.Equals), nil)
}
```

Note: The exact assertion depends on `result` type. The Wile engine returns `values.Value`. If the result is an integer >= 2, multi-package loading works. Adjust the assertion based on how `wile.Value` exposes integers:

```go
	// The result is the count of unique packages with Prim* functions.
	// With ... pattern, should be >= 2 (goast, goastssa, goastcfg, goastcg, goastlint).
	str := result.String()
	count := 0
	fmt.Sscanf(str, "%d", &count)
	c.Assert(count >= 2, qt.IsTrue, qt.Commentf("expected >= 2 packages with Prim* functions, got %d", count))
```

**Step 2: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefMultiPackage -v -timeout 120s`
Expected: PASS — the `...` pattern loads all project packages, and sites from multiple packages are found.

Note: Use longer timeout — loading all packages with SSA may take time.

**Step 3: Commit**

```bash
git add goast/belief_integration_test.go
git commit -m "test: multi-package belief integration test"
```

---

### Task 8: Update k8s kubelet beliefs script

Convert the per-package `for-each` loop to a single `run-beliefs` call with a `...` pattern. This is not strictly required for the feature but validates the design against the original motivating use case.

**Files:**
- Modify: `examples/goast-query/k8s-kubelet-beliefs.scm:67-89`

**Step 1: Replace the per-package loop**

Replace lines 67-89 with:

```scheme
;; ── Run against all kubelet packages ─────────────────────

(run-beliefs "k8s.io/kubernetes/pkg/kubelet/...")
```

**Step 2: Verify manually**

Run from a Kubernetes checkout:
```bash
cd /path/to/kubernetes && wile-goast -f /path/to/k8s-kubelet-beliefs.scm
```

Expected: Output now shows `kuberuntime.methodName`, `status.methodName` etc. in deviation reports. All beliefs produce results (no regressions).

**Step 3: Commit**

```bash
git add examples/goast-query/k8s-kubelet-beliefs.scm
git commit -m "refactor: kubelet beliefs use ... pattern for multi-package analysis"
```

---

### Task 9: Run full CI and final commit

**Step 1: Run full test suite**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: ALL PASS — lint, build, test, coverage.

**Step 2: Verify examples still work**

Run: `cd /Users/aalpar/projects/wile-workspace/wile && wile-goast -f /path/to/belief-comutation.scm`
Expected: Output matches previous runs (backward compatibility).

**Step 3: If any failures, fix and re-run**

Common issues:
- Coverage threshold: new code in `belief.scm` is Scheme, not Go — won't affect Go coverage.
- Lint: no Go code changed (except test file) — unlikely to trigger.
- Test timeout: multi-package test may need longer timeout.
