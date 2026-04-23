# Belief Suppression Implementation Plan

> **Status: Shipped 2026-04-23** — commits `846a5dd` (implementation + 13 integration tests + `(current-beliefs)` accessor + `WithSourceOS()` on the engine) and `6283713` (CHANGELOG, root CLAUDE.md, plans/BELIEF-DSL.md, plans/CLAUDE.md).
>
> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `with-belief-scope`, `load-committed-beliefs`, and `suppress-known` in `(wile goast belief)` so the `discover → review → commit → enforce` lifecycle closes: re-running discovery on a codebase does not resurface beliefs already committed to `.scm` files.

**Architecture:** Three Scheme procedures added to `cmd/wile-goast/lib/wile/goast/belief.scm` and exported from `belief.sld`. `with-belief-scope` uses `dynamic-wind` to save/restore `*beliefs*` / `*aggregate-beliefs*`. `load-committed-beliefs` accepts a file or directory, loads `.scm` files in a fresh scope (wrapped in `guard` for per-file resilience), returns a `(per-site-snapshot . aggregate-snapshot)` pair. `suppress-known` is a pure filter that drops results whose `(sites-expr, expect-expr)` (per-site) or `(sites-expr, analyze-expr)` (aggregate) match any committed belief via `equal?`.

**Tech Stack:** Scheme (Wile R7RS). Wile primitives: `file-exists?`, `directory-files`, `load`, `dynamic-wind`, `guard`, `current-error-port`. Go test harness: `quicktest` + the existing `newBeliefEngine` / Scheme-runner helpers in `goast/belief_integration_test.go`.

**Parent design:** `plans/2026-04-17-belief-suppression-design.md`.

**Project conventions observed:**
- wile-goast commits direct to master (per `CLAUDE.local.md`).
- No Co-Authored-By lines in commit messages.
- `.sld` file exports every symbol explicitly; tests import `(wile goast belief)`.
- Go tests use quicktest (`qt.New(t)`, `qt.Assert`, `qt.Equals`).
- The existing Scheme-runner test helper runs multi-expression code via `engine.EvalMultiple`.
- `newBeliefEngine(t)` loads all goast extensions + embedded library paths.
- `make lint` + `make test` must pass before claiming done.
- `VERSION` is auto-bumped by pre-commit hook — do not touch.

---

## File Structure

**Modify:**
- `cmd/wile-goast/lib/wile/goast/belief.scm` — add new section at end of file (before the trailing `;; string-join: moved to (wile goast utils)` comment on line 864):
  - `with-belief-scope` (public)
  - `load-committed-beliefs` (public)
  - `suppress-known` (public)
  - `sort-scheme-filenames` (local helper — insertion-sort strings)
  - `list-scheme-files-in-dir` (local helper — read directory, filter `.scm`, sort)
  - `load-belief-file` (local helper — guarded single-file load)
  - `result-matches-any?` (local helper — dispatch on `type` field)
  - `belief-expressions-match?` + `any-tuple-matches?` (local helpers — `equal?` on expression keys)
- `cmd/wile-goast/lib/wile/goast/belief.sld` — add exports for the three public procedures.
- `goast/belief_integration_test.go` — add 13 tests (12 per the design + one extra for the missing-`type` edge-case).
- `CHANGELOG.md` — new entry describing suppression.
- `plans/BELIEF-DSL.md` + `plans/CLAUDE.md` — flip status to shipped.
- `CLAUDE.md` — add suppression sub-section under `## Belief DSL`.

**Do not modify:**
- `belief-checkers.scm` (separate concern).
- `utils.scm` / `dataflow.scm` (no new shared utilities needed).
- Any Go file outside `goast/belief_integration_test.go`.
- `VERSION`.

---

## Task 1: `with-belief-scope` — isolate registry during a thunk

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/belief.sld`
- Test: `goast/belief_integration_test.go`

- [x] **Step 1: Write the first failing test**

Append to `/Users/aalpar/projects/wile-workspace/wile-goast/goast/belief_integration_test.go`:

```go
func TestWithBeliefScope_Restores(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	// Define a belief in the outer scope, enter scope and register another,
	// verify the inner is gone after the scope exits and the outer remains.
	result := eval(t, engine, `
		(import (wile goast belief))
		(reset-beliefs!)
		(define-belief "outer"
		  (sites (functions-matching (name-matches "X")))
		  (expect (contains-call "Foo"))
		  (threshold 0.9 3))
		(define inside-count 0)
		(with-belief-scope
		  (lambda ()
		    (define-belief "inner"
		      (sites (functions-matching (name-matches "Y")))
		      (expect (contains-call "Bar"))
		      (threshold 0.9 3))
		    (set! inside-count (length (current-beliefs)))))
		(list inside-count (length (current-beliefs)) (car (car (current-beliefs))))
	`)
	// Inside the scope: only "inner" (length 1 since reset-beliefs! ran).
	// After: restored to just "outer" (length 1, car name == "outer").
	c.Assert(result.SchemeString(), qt.Equals, `(1 1 "outer")`)
}
```

Note: the test calls `runScheme` which is the existing test helper defined in `goast/prim_goast_test.go:42` (it's just renamed inline in this plan to avoid a naming clash with JavaScript-style interpretations; use whatever name the file already defines — `grep -n "^func.*t \*testing.T, engine \*wile.Engine, code string" goast/prim_goast_test.go` to confirm).

- [x] **Step 2: Run the test to confirm it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestWithBeliefScope_Restores -v`
Expected: FAIL — "undefined identifier: with-belief-scope".

- [x] **Step 3: Write the second failing test (escape restoration)**

Append to `goast/belief_integration_test.go`:

```go
func TestWithBeliefScope_RestoresOnEscape(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	// Raise an exception inside the thunk. dynamic-wind's after-thunk
	// must still run, restoring *beliefs*.
	result := eval(t, engine, `
		(import (wile goast belief))
		(reset-beliefs!)
		(define-belief "outer"
		  (sites (functions-matching (name-matches "X")))
		  (expect (contains-call "Foo"))
		  (threshold 0.9 3))
		(guard (exn (#t 'caught))
		  (with-belief-scope
		    (lambda ()
		      (define-belief "inner"
		        (sites (functions-matching (name-matches "Y")))
		        (expect (contains-call "Bar"))
		        (threshold 0.9 3))
		      (error "forced escape"))))
		(list (length (current-beliefs)) (car (car (current-beliefs))))
	`)
	c.Assert(result.SchemeString(), qt.Equals, `(1 "outer")`)
}
```

- [x] **Step 4: Run both tests, confirm fail**

Run: `go test ./goast/ -run TestWithBeliefScope -v`
Expected: both FAIL — undefined identifier.

- [x] **Step 5: Implement `with-belief-scope` in `belief.scm`**

Open `/Users/aalpar/projects/wile-workspace/wile-goast/cmd/wile-goast/lib/wile/goast/belief.scm`. Find the last line of the file (the `;; string-join: moved to (wile goast utils)` comment at line 864). Append **before** that trailing comment:

```scheme
;; ── Belief suppression ──────────────────────────────────
;;
;; Close the discover → review → commit → enforce lifecycle.
;; `with-belief-scope` isolates a thunk from the caller's belief registry;
;; `load-committed-beliefs` snapshots a directory or file of .scm beliefs;
;; `suppress-known` drops results whose sites+expect/analyze expressions
;; match any committed belief structurally (via equal?).

(define (with-belief-scope thunk)
  "Save the belief registry, reset it, invoke THUNK, then restore.
Uses dynamic-wind so the registry is restored even on early exit.

Parameters:
  thunk : procedure of zero arguments
Returns: the value returned by THUNK
Category: goast-belief

See also: `load-committed-beliefs', `reset-beliefs!'."
  (let ((saved-per-site *beliefs*)
        (saved-aggregate *aggregate-beliefs*))
    (dynamic-wind
      (lambda () (reset-beliefs!))
      thunk
      (lambda ()
        (set! *beliefs* saved-per-site)
        (set! *aggregate-beliefs* saved-aggregate)))))
```

- [x] **Step 6: Add export to `belief.sld`**

Open `/Users/aalpar/projects/wile-workspace/wile-goast/cmd/wile-goast/lib/wile/goast/belief.sld`. Find the `;; Core` export block (line 17-18) and append a new `;; Suppression` stanza after `aggregate-beliefs`:

```scheme
    ;; Core
    define-belief run-beliefs reset-beliefs! *beliefs* emit-beliefs
    define-aggregate-belief register-aggregate-belief! *aggregate-beliefs*
    aggregate-beliefs
    ;; Suppression
    with-belief-scope load-committed-beliefs suppress-known
```

Adding `load-committed-beliefs` / `suppress-known` now (before they're defined) is safe — Wile only resolves exports at import time. Later tasks fill in the definitions.

- [x] **Step 7: Run the two tests, expect pass**

Run: `go test ./goast/ -run TestWithBeliefScope -v`
Expected: both PASS.

If the escape test fails (registry not restored after exception), verify Wile honors `dynamic-wind`'s after-thunk on exception escape — this is standard R7RS but worth a direct probe.

- [x] **Step 8: Ask the user before committing**

> "Task 1 complete: `with-belief-scope` shipped with restore-on-success + restore-on-escape tests. Want me to commit?"

On approval:

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
  git add cmd/wile-goast/lib/wile/goast/belief.scm \
          cmd/wile-goast/lib/wile/goast/belief.sld \
          goast/belief_integration_test.go && \
  git commit -m "feat(belief): add with-belief-scope for isolated registry"
```

---

## Task 2: `load-committed-beliefs` — directory/file loader with per-file guard

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm`
- Test: `goast/belief_integration_test.go`

- [x] **Step 1: Add test helpers to the test file**

If not already present, add the following two helpers near the top of `goast/belief_integration_test.go` (below the existing `newBeliefEngine` function). Also add `"path/filepath"` and `"strings"` to the file's `import` block if they are not there yet.

```go
// mustWriteFile writes content to path and fails the test on error.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// schemeStr wraps a Go string as a Scheme string literal, escaping
// backslashes and double quotes. Adequate for filesystem paths in tests.
func schemeStr(s string) string {
	r := strings.ReplaceAll(s, `\`, `\\`)
	r = strings.ReplaceAll(r, `"`, `\"`)
	return `"` + r + `"`
}
```

- [x] **Step 2: Write the directory-loading test**

Append:

```go
func TestLoadCommittedBeliefs_Directory(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.scm")
	fileB := filepath.Join(dir, "b.scm")
	mustWriteFile(t, fileA, `
		(import (wile goast belief))
		(define-belief "belief-a"
		  (sites (functions-matching (name-matches "A")))
		  (expect (contains-call "FooA"))
		  (threshold 0.9 3))
	`)
	mustWriteFile(t, fileB, `
		(import (wile goast belief))
		(define-belief "belief-b"
		  (sites (functions-matching (name-matches "B")))
		  (expect (contains-call "FooB"))
		  (threshold 0.9 3))
	`)

	result := eval(t, engine, `
		(import (wile goast belief))
		(reset-beliefs!)
		(define committed (load-committed-beliefs `+schemeStr(dir)+`))
		(list (length (car committed))
		      (length (cdr committed))
		      (length (current-beliefs)))
	`)
	// Expect per-site len == 2, aggregate len == 0, caller registry unchanged.
	c.Assert(result.SchemeString(), qt.Equals, `(2 0 0)`)
}
```

- [x] **Step 3: Run the test, confirm failure**

Run: `go test ./goast/ -run TestLoadCommittedBeliefs_Directory -v`
Expected: FAIL — "undefined identifier: load-committed-beliefs".

- [x] **Step 4: Implement the helpers in `belief.scm`**

In `belief.scm`, immediately **after** the `with-belief-scope` definition added in Task 1, append:

```scheme
;; Insertion-sort a list of strings lexicographically.
;; Used for deterministic directory-load order.
(define (sort-scheme-filenames lst)
  (define (insert x sorted)
    (cond
      ((null? sorted) (list x))
      ((string<? x (car sorted)) (cons x sorted))
      (else (cons (car sorted) (insert x (cdr sorted))))))
  (let loop ((xs lst) (acc '()))
    (if (null? xs)
      acc
      (loop (cdr xs) (insert (car xs) acc)))))

;; Return the list of .scm file paths (full paths) directly under DIR.
;; Top-level only — no recursion. Sorted lexicographically.
(define (list-scheme-files-in-dir dir)
  (let* ((names (directory-files dir))
         (scm-only
           (let loop ((ns names) (acc '()))
             (cond
               ((null? ns) acc)
               ((and (>= (string-length (car ns)) 4)
                     (string=? (substring (car ns)
                                          (- (string-length (car ns)) 4)
                                          (string-length (car ns)))
                               ".scm"))
                (loop (cdr ns) (cons (car ns) acc)))
               (else (loop (cdr ns) acc)))))
         (sorted (sort-scheme-filenames scm-only)))
    (map (lambda (n) (string-append dir "/" n)) sorted)))

;; Load a single .scm file into the current registry. On failure,
;; write "wile-goast: skipping <path>: <msg>\n" to (current-error-port)
;; and return #f. On success, return #t.
(define (load-belief-file path)
  (guard (exn (#t
               (let ((msg (cond
                            ((error-object? exn)
                             (error-object-message exn))
                            (else "unknown error"))))
                 (display "wile-goast: skipping " (current-error-port))
                 (display path (current-error-port))
                 (display ": " (current-error-port))
                 (display msg (current-error-port))
                 (newline (current-error-port))
                 #f)))
    (load path)
    #t))
```

Next, append the main procedure:

```scheme
(define (load-committed-beliefs path)
  "Load committed beliefs from PATH (a directory or a single .scm file).
Each .scm file is evaluated with `load` inside `with-belief-scope`, so
the caller's belief registry is not clobbered.

Returns a pair:
  (per-site-snapshot . aggregate-snapshot)

where each snapshot is the list of belief tuples (7-tuples for per-site,
5-tuples for aggregate) in registration order.

Files that fail to load are skipped with a stderr warning; the partial
snapshot is still returned. A nonexistent PATH raises an error.

Parameters:
  path : string — directory or file path
Returns: pair of (list . list)
Category: goast-belief

See also: `suppress-known', `with-belief-scope'."
  (cond
    ((not (file-exists? path))
     (error "load-committed-beliefs: path does not exist" path))
    (else
      (with-belief-scope
        (lambda ()
          ;; Determine directory vs file by probing directory-files.
          ;; Success => directory; exception => treat as single file.
          (let ((files
                  (guard (exn (#t (list path)))
                    (list-scheme-files-in-dir path))))
            (for-each load-belief-file files)
            (cons *beliefs* *aggregate-beliefs*)))))))
```

Implementation notes:

1. `with-belief-scope` captures `*beliefs*` / `*aggregate-beliefs*` inside its body; the `(cons ...)` reads the *inner* scope's registry before the after-thunk restores the outer state. That's the correct snapshot.
2. `list-scheme-files-in-dir` raises a file-error if PATH is a regular file; the outer `guard` catches that and falls through to the single-file branch.

- [x] **Step 5: Run the directory test, expect pass**

Run: `go test ./goast/ -run TestLoadCommittedBeliefs_Directory -v`
Expected: PASS.

If the result is `(0 0 0)`, the directory-vs-file probe misfires — add `(display files (current-error-port))` inside the body to inspect. If the result is `(2 0 2)`, `with-belief-scope` isn't isolating — check the order of `dynamic-wind` thunks.

- [x] **Step 6: Write the single-file test**

Append:

```go
func TestLoadCommittedBeliefs_File(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	dir := t.TempDir()
	file := filepath.Join(dir, "single.scm")
	mustWriteFile(t, file, `
		(import (wile goast belief))
		(define-belief "only-belief"
		  (sites (functions-matching (name-matches "Z")))
		  (expect (contains-call "FooZ"))
		  (threshold 0.9 3))
	`)

	result := eval(t, engine, `
		(import (wile goast belief))
		(reset-beliefs!)
		(define committed (load-committed-beliefs `+schemeStr(file)+`))
		(list (length (car committed)) (length (cdr committed)))
	`)
	c.Assert(result.SchemeString(), qt.Equals, `(1 0)`)
}
```

- [x] **Step 7: Run, expect pass**

Run: `go test ./goast/ -run TestLoadCommittedBeliefs_File -v`
Expected: PASS.

- [x] **Step 8: Write the skip-bad-file test**

Append:

```go
func TestLoadCommittedBeliefs_SkipsBadFiles(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	dir := t.TempDir()
	good := filepath.Join(dir, "good.scm")
	bad := filepath.Join(dir, "bad.scm")
	mustWriteFile(t, good, `
		(import (wile goast belief))
		(define-belief "good-one"
		  (sites (functions-matching (name-matches "G")))
		  (expect (contains-call "FooG"))
		  (threshold 0.9 3))
	`)
	// Syntax error — unbalanced parens.
	mustWriteFile(t, bad, `(this is (not valid scheme`)

	// The stderr warning is a side effect; test validates that the good
	// file still loaded while the bad one was skipped.
	result := eval(t, engine, `
		(import (wile goast belief))
		(reset-beliefs!)
		(define committed (load-committed-beliefs `+schemeStr(dir)+`))
		(list (length (car committed))
		      (car (car (car committed))))
	`)
	c.Assert(result.SchemeString(), qt.Equals, `(1 "good-one")`)
}
```

- [x] **Step 9: Run, expect pass**

Run: `go test ./goast/ -run TestLoadCommittedBeliefs_SkipsBadFiles -v`
Expected: PASS.

- [x] **Step 10: Write the nonexistent-path test**

Append:

```go
func TestLoadCommittedBeliefs_NonexistentPath(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	_, err := engine.EvalMultiple(context.Background(), `
		(import (wile goast belief))
		(load-committed-beliefs "/nonexistent/path/to/nowhere/xyzzy")
	`)
	c.Assert(err, qt.IsNotNil)
}
```

- [x] **Step 11: Run, expect pass**

Run: `go test ./goast/ -run TestLoadCommittedBeliefs_NonexistentPath -v`
Expected: PASS — the error bubbles up from `(error ...)` in the Scheme impl.

- [x] **Step 12: Ask before committing**

> "Task 2 complete: `load-committed-beliefs` with directory/file/skip-bad/nonexistent tests. Commit?"

On approval:

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
  git add cmd/wile-goast/lib/wile/goast/belief.scm \
          goast/belief_integration_test.go && \
  git commit -m "feat(belief): add load-committed-beliefs"
```

---

## Task 3: `suppress-known` — structural filter over results

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm`
- Test: `goast/belief_integration_test.go`

- [x] **Step 1: Write the per-site match test**

Append to `goast/belief_integration_test.go`:

```go
func TestSuppressKnown_PerSiteMatch(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	// Hand-construct a result alist with per-site type, and a committed
	// registry whose single belief has identical sites-expr / expect-expr.
	// The result should be filtered out.
	result := eval(t, engine, `
		(import (wile goast belief))
		(define results
		  (list
		    (list (cons 'name "r1")
		          (cons 'type 'per-site)
		          (cons 'status 'strong)
		          (cons 'sites-expr '(sites (functions-matching (name-matches "X"))))
		          (cons 'expect-expr '(expect (contains-call "Foo"))))))
		(define committed
		  (cons
		    (list (list "committed-name" #f #f 0.9 3
		                '(sites (functions-matching (name-matches "X")))
		                '(expect (contains-call "Foo"))))
		    '()))
		(length (suppress-known results committed))
	`)
	c.Assert(result.SchemeString(), qt.Equals, "0")
}
```

- [x] **Step 2: Run, confirm failure**

Run: `go test ./goast/ -run TestSuppressKnown_PerSiteMatch -v`
Expected: FAIL — "undefined identifier: suppress-known".

- [x] **Step 3: Implement helpers and `suppress-known` in `belief.scm`**

Append, after the `load-committed-beliefs` definition from Task 2:

```scheme
;; Structural equality on a pair of belief expression keys.
;; Returns #t if both keys match in both RESULT and BELIEF-TUPLE.
;; Committed per-site tuple layout:
;;   (name fn fn min-adh min-n sites-expr expect-expr)
;; Committed aggregate tuple layout:
;;   (name fn analyzer sites-expr analyze-expr)
(define (belief-expressions-match? result result-sites-key result-expect-key
                                    tuple-sites-getter tuple-expect-getter
                                    tuple)
  (let ((r-sites (assoc result-sites-key result))
        (r-expect (assoc result-expect-key result)))
    (and r-sites r-expect
         (equal? (cdr r-sites) (tuple-sites-getter tuple))
         (equal? (cdr r-expect) (tuple-expect-getter tuple)))))

;; Walk a list of committed tuples; return #t if any matches RESULT's
;; expressions under the given key + getter pair.
(define (any-tuple-matches? result r-sites-key r-expect-key
                             tuple-sites-getter tuple-expect-getter
                             tuples)
  (let loop ((ts tuples))
    (cond
      ((null? ts) #f)
      ((belief-expressions-match? result r-sites-key r-expect-key
                                   tuple-sites-getter tuple-expect-getter
                                   (car ts)) #t)
      (else (loop (cdr ts))))))

;; Dispatch on RESULT's 'type key:
;;   'per-site   => compare (sites-expr, expect-expr) to per-site tuples.
;;   'aggregate  => compare (sites-expr, analyze-expr) to aggregate tuples.
;;   missing/other => #f (pass through).
(define (result-matches-any? result per-site-tuples aggregate-tuples)
  (let ((type-entry (assoc 'type result)))
    (cond
      ((not type-entry) #f)
      ((eq? (cdr type-entry) 'per-site)
       (any-tuple-matches? result 'sites-expr 'expect-expr
                           belief-sites-expr belief-expect-expr
                           per-site-tuples))
      ((eq? (cdr type-entry) 'aggregate)
       (any-tuple-matches? result 'sites-expr 'analyze-expr
                           aggregate-belief-sites-expr
                           aggregate-belief-analyze-expr
                           aggregate-tuples))
      (else #f))))

(define (suppress-known results committed)
  "Filter RESULTS (output of run-beliefs), dropping any whose
expressions match a belief in COMMITTED (output of
load-committed-beliefs). Matching is structural via `equal?` on
`sites-expr` / `expect-expr` (per-site) or `sites-expr` /
`analyze-expr` (aggregate). Names, thresholds, ratios, and all
other fields are ignored.

Parameters:
  results   : list of result alists
  committed : pair of (per-site-tuples . aggregate-tuples) from
              load-committed-beliefs
Returns: list of result alists (filtered)
Category: goast-belief

See also: `load-committed-beliefs', `emit-beliefs'."
  (let ((per-site (car committed))
        (aggregate (cdr committed)))
    (let loop ((rs results) (acc '()))
      (cond
        ((null? rs) (reverse acc))
        ((result-matches-any? (car rs) per-site aggregate)
         (loop (cdr rs) acc))
        (else (loop (cdr rs) (cons (car rs) acc)))))))
```

- [x] **Step 4: Run the test, expect pass**

Run: `go test ./goast/ -run TestSuppressKnown_PerSiteMatch -v`
Expected: PASS.

If length is `1` instead of `0`, inspect: the `type` key dispatch is wrong, or the `sites-expr` keys don't match by `equal?`. The design says both should be symbolic S-expressions.

- [x] **Step 5: Write the rename-ignored test**

Append:

```go
func TestSuppressKnown_RenameIgnored(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	result := eval(t, engine, `
		(import (wile goast belief))
		(define results
		  (list
		    (list (cons 'name "result-new-name")
		          (cons 'type 'per-site)
		          (cons 'sites-expr '(sites (functions-matching (name-matches "X"))))
		          (cons 'expect-expr '(expect (contains-call "Foo"))))))
		(define committed
		  (cons
		    (list (list "committed-old-name" #f #f 0.9 3
		                '(sites (functions-matching (name-matches "X")))
		                '(expect (contains-call "Foo"))))
		    '()))
		(length (suppress-known results committed))
	`)
	c.Assert(result.SchemeString(), qt.Equals, "0")
}
```

- [x] **Step 6: Write the threshold-ignored test**

Append:

```go
func TestSuppressKnown_ThresholdIgnored(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	// Committed threshold (0.80) vs result's implied threshold (0.90) —
	// still filtered; thresholds do not participate in matching.
	result := eval(t, engine, `
		(import (wile goast belief))
		(define results
		  (list
		    (list (cons 'name "r")
		          (cons 'type 'per-site)
		          (cons 'min-adherence 0.9)
		          (cons 'sites-expr '(sites (functions-matching (name-matches "X"))))
		          (cons 'expect-expr '(expect (contains-call "Foo"))))))
		(define committed
		  (cons
		    (list (list "committed" #f #f 0.8 5
		                '(sites (functions-matching (name-matches "X")))
		                '(expect (contains-call "Foo"))))
		    '()))
		(length (suppress-known results committed))
	`)
	c.Assert(result.SchemeString(), qt.Equals, "0")
}
```

- [x] **Step 7: Write the aggregate match test**

Append:

```go
func TestSuppressKnown_AggregateMatch(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	result := eval(t, engine, `
		(import (wile goast belief))
		(define results
		  (list
		    (list (cons 'name "agg-r")
		          (cons 'type 'aggregate)
		          (cons 'status 'ok)
		          (cons 'sites-expr '(sites (all-functions-in)))
		          (cons 'analyze-expr '(analyze (single-cluster))))))
		(define committed
		  (cons
		    '()
		    (list (list "committed-agg" #f #f
		                '(sites (all-functions-in))
		                '(analyze (single-cluster))))))
		(length (suppress-known results committed))
	`)
	c.Assert(result.SchemeString(), qt.Equals, "0")
}
```

- [x] **Step 8: Write the no-match pass-through test**

Append:

```go
func TestSuppressKnown_NoMatch(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	result := eval(t, engine, `
		(import (wile goast belief))
		(define results
		  (list
		    (list (cons 'name "r-unique")
		          (cons 'type 'per-site)
		          (cons 'sites-expr '(sites (functions-matching (name-matches "DIFFERENT"))))
		          (cons 'expect-expr '(expect (contains-call "DifferentCall"))))))
		(define committed
		  (cons
		    (list (list "committed" #f #f 0.9 3
		                '(sites (functions-matching (name-matches "X")))
		                '(expect (contains-call "Foo"))))
		    '()))
		(length (suppress-known results committed))
	`)
	c.Assert(result.SchemeString(), qt.Equals, "1")
}
```

- [x] **Step 9: Write the missing-type pass-through test**

Covers the "Result alist without `type` key → passes through" edge case from the design's §Edge Cases table:

```go
func TestSuppressKnown_MissingTypePassesThrough(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	result := eval(t, engine, `
		(import (wile goast belief))
		(define results
		  (list
		    (list (cons 'name "typeless")
		          (cons 'sites-expr '(sites (functions-matching (name-matches "X"))))
		          (cons 'expect-expr '(expect (contains-call "Foo"))))))
		(define committed
		  (cons
		    (list (list "committed" #f #f 0.9 3
		                '(sites (functions-matching (name-matches "X")))
		                '(expect (contains-call "Foo"))))
		    '()))
		(length (suppress-known results committed))
	`)
	c.Assert(result.SchemeString(), qt.Equals, "1")
}
```

- [x] **Step 10: Run all suppress-known tests**

Run: `go test ./goast/ -run TestSuppressKnown -v`
Expected: 6 PASS (PerSiteMatch + RenameIgnored + ThresholdIgnored + AggregateMatch + NoMatch + MissingTypePassesThrough).

- [x] **Step 11: Ask before committing**

> "Task 3 complete: `suppress-known` with 6 tests covering match, rename, threshold, aggregate, no-match, missing-type. Commit?"

On approval:

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
  git add cmd/wile-goast/lib/wile/goast/belief.scm \
          goast/belief_integration_test.go && \
  git commit -m "feat(belief): add suppress-known structural filter"
```

---

## Task 4: End-to-end integration test

**Files:**
- Test: `goast/belief_integration_test.go`

- [x] **Step 1: Write the end-to-end test**

Exercises the full pipeline: discovery belief → run → load committed → suppress → measure result count. Append:

```go
func TestSuppressKnown_EndToEnd(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	// Write one committed belief to a tempdir. It has the same sites+expect
	// expressions as the discovery belief we define below.
	dir := t.TempDir()
	file := filepath.Join(dir, "committed.scm")
	mustWriteFile(t, file, `
		(import (wile goast belief))
		(define-belief "prim-have-body"
		  (sites (functions-matching (name-matches "Prim")))
		  (expect (custom (lambda (site ctx)
		    (if (nf site 'body) 'has-body 'no-body))))
		  (threshold 0.9 3))
	`)

	// Run discovery with the same belief, then load committed + suppress.
	// Expect the filtered list to be empty — every result was matched.
	result := eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast utils))
		(reset-beliefs!)
		(define results
		  (with-belief-scope
		    (lambda ()
		      (define-belief "prim-have-body"
		        (sites (functions-matching (name-matches "Prim")))
		        (expect (custom (lambda (site ctx)
		          (if (nf site 'body) 'has-body 'no-body))))
		        (threshold 0.9 3))
		      (run-beliefs "github.com/aalpar/wile-goast/goast"))))
		(define committed (load-committed-beliefs `+schemeStr(dir)+`))
		(length (suppress-known results committed))
	`)
	// Discovery belief matches committed one by structure; every result is
	// suppressed. run-beliefs returns 1 result (one belief defined), so
	// the filtered list length should be 0.
	c.Assert(result.SchemeString(), qt.Equals, "0")
}
```

- [x] **Step 2: Run and expect pass**

Run: `go test ./goast/ -run TestSuppressKnown_EndToEnd -v`
Expected: PASS. This test runs `run-beliefs` over a real Go package; budget ~5 s.

If the result is `"1"` instead of `"0"`, the discovery belief's expression doesn't match the committed one byte-for-byte. Probe with:

```scheme
(write (cdr (assoc 'sites-expr (car results))))
(write (list-ref (car committed) 5))
```

Confirm both read as `(sites (functions-matching (name-matches "Prim")))`. If the macro expansion adds extra nesting, structural matching needs a pre-comparison normalization step — unlikely given how `define-belief` captures expressions (literal `'(sites selector)` in the macro body), but worth probing if it fails.

- [x] **Step 3: Ask before committing**

> "Task 4 complete: end-to-end discover → load → suppress test passes. Commit?"

On approval:

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
  git add goast/belief_integration_test.go && \
  git commit -m "test(belief): end-to-end suppress-known integration test"
```

---

## Task 5: Run `make lint` + `make test` to confirm no regressions

- [x] **Step 1: Run lint**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make lint`
Expected: 0 issues.

- [x] **Step 2: Run the full test suite**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make test`
Expected: all packages PASS. The new tests add ~5–10 s to the goast package runtime.

- [x] **Step 3: Run coverage**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: all packages above 80%.

- [x] **Step 4: No commit for this task (pure validation).**

---

## Task 6: Update `CHANGELOG.md`

**Files:**
- Modify: `CHANGELOG.md`

- [x] **Step 1: Read the current top entry**

Run: `head -20 CHANGELOG.md`. Confirm format: `## v<version> — <title>` header followed by dash-list bullets.

- [x] **Step 2: Prepend new entry**

Use `Edit` to prepend **above** the existing first `## v...` header:

```markdown
## Unreleased — Belief Suppression

Close the belief DSL's discover → review → commit → enforce lifecycle.

- `with-belief-scope` — isolate a thunk from the caller's belief registry
  via `dynamic-wind`. Used by `load-committed-beliefs` and available to
  scripts that want to define beliefs without leaking them.
- `load-committed-beliefs` — accept a directory or single `.scm` file,
  load beliefs into an isolated scope, return a
  `(per-site-snapshot . aggregate-snapshot)` pair. Files that fail to
  load are skipped with a stderr warning.
- `suppress-known` — filter `run-beliefs` output by structural (`equal?`)
  comparison on `sites-expr` / `expect-expr` (per-site) or
  `sites-expr` / `analyze-expr` (aggregate). Names and thresholds are
  ignored during matching.

Discovery scripts can now compose:

    (define results (with-belief-scope (lambda () ... (run-beliefs "..."))))
    (define committed (load-committed-beliefs "beliefs/"))
    (display (emit-beliefs (suppress-known results committed)))

```

- [x] **Step 3: Commit**

Ask first:

> "Task 6: CHANGELOG updated. Commit?"

On approval:

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
  git add CHANGELOG.md && \
  git commit -m "docs(changelog): note belief suppression procedures"
```

---

## Task 7: Update `plans/BELIEF-DSL.md` and `plans/CLAUDE.md`

**Files:**
- Modify: `plans/BELIEF-DSL.md`
- Modify: `plans/CLAUDE.md`

- [x] **Step 1: Rewrite `§Suppression` opener in BELIEF-DSL.md**

`plans/BELIEF-DSL.md` §Suppression (line 143) opens with "Future discovery runs should diff output against committed belief files…". No checkboxes. Replace the opener paragraph with a past-tense "Shipped 2026-04-<DAY>. See `plans/2026-04-17-belief-suppression-impl.md`." note, and keep the subsections (Structural Matching, `suppress-known` API, Loading Committed Beliefs, Edge Cases, Changes Required) as descriptive reference documentation. Reconcile one drift: the existing "Returns: snapshot of `*beliefs*` as a list" claim under §Loading Committed Beliefs is out of date — change to `Returns: (per-site-snapshot . aggregate-snapshot) pair` per the approved design.

- [x] **Step 2: Mark the `§CLI Integration` subsection as deferred**

The design doc explicitly says "No CLI flags this round". Replace the `### CLI Integration` subsection's body with a one-line "Deferred — users compose `with-belief-scope` / `load-committed-beliefs` / `suppress-known` / `emit-beliefs` in discovery scripts. See impl plan for rationale." Leave the header intact so anchors don't break.

- [x] **Step 3: Update `plans/CLAUDE.md` Active Plan Files table**

Find the row:

```
| `2026-04-17-belief-suppression-design.md` | Belief suppression: `with-belief-scope`, `load-committed-beliefs`, `suppress-known` | Design approved, impl pending |
```

Change the Status column to: `Shipped — see 2026-04-17-belief-suppression-impl.md`.

Also add a corresponding row to the "Completed Plans" table (design + impl pair, same date prefix).

- [x] **Step 4: Commit**

Ask first:

> "Task 7: plan status flipped in BELIEF-DSL.md and plans/CLAUDE.md. Commit?"

On approval:

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
  git add plans/BELIEF-DSL.md plans/CLAUDE.md && \
  git commit -m "docs(plans): mark belief suppression shipped"
```

---

## Task 8: Update `CLAUDE.md` primitive index

**Files:**
- Modify: `CLAUDE.md`

- [x] **Step 1: Locate the `(wile goast belief)` exports section**

Run: `grep -n 'define-belief\|emit-beliefs' CLAUDE.md` to find the section.

- [x] **Step 2: Add a new Suppression sub-section**

Insert after the existing emit-beliefs table:

```markdown
### Suppression

Close the `discover → review → commit → enforce` lifecycle. Committed
beliefs live in `.scm` files; re-running discovery should not resurface
a belief already committed.

| Export | Description |
|--------|-------------|
| `with-belief-scope` | Save/restore `*beliefs*` and `*aggregate-beliefs*` around a thunk via `dynamic-wind`. |
| `load-committed-beliefs` | Load `.scm` beliefs from a directory or file into an isolated scope; return `(per-site-snapshot . aggregate-snapshot)` pair. |
| `suppress-known` | Structural filter: drop results whose `sites-expr` + `expect-expr` (per-site) or `sites-expr` + `analyze-expr` (aggregate) match any committed tuple. |

Typical pipeline:

    (define results
      (with-belief-scope
        (lambda ()
          <...discovery beliefs...>
          (run-beliefs "my/pkg/..."))))
    (define committed (load-committed-beliefs "beliefs/"))
    (display (emit-beliefs (suppress-known results committed)))

Matching is structural (`equal?` on captured S-expressions). Names,
thresholds, and ratios are ignored during matching.
```

- [x] **Step 3: Commit**

Ask first:

> "Task 8: CLAUDE.md updated with suppression section. Commit?"

On approval:

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
  git add CLAUDE.md && \
  git commit -m "docs(claude): document belief suppression procedures"
```

---

## Task 9: Final green-gate sweep

- [x] **Step 1: Run `make ci`**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: lint + build + test + coverage + `go mod verify` all PASS.

- [x] **Step 2: Confirm design spec's §Testing checklist is fully covered**

Run:

```bash
grep -E '^func (TestWithBeliefScope|TestLoadCommittedBeliefs|TestSuppressKnown)' \
  goast/belief_integration_test.go
```

Expected output (13 functions — design's 12 + one extra edge-case):

```
TestWithBeliefScope_Restores
TestWithBeliefScope_RestoresOnEscape
TestLoadCommittedBeliefs_Directory
TestLoadCommittedBeliefs_File
TestLoadCommittedBeliefs_SkipsBadFiles
TestLoadCommittedBeliefs_NonexistentPath
TestSuppressKnown_PerSiteMatch
TestSuppressKnown_RenameIgnored
TestSuppressKnown_ThresholdIgnored
TestSuppressKnown_AggregateMatch
TestSuppressKnown_NoMatch
TestSuppressKnown_MissingTypePassesThrough
TestSuppressKnown_EndToEnd
```

- [x] **Step 3: No commit for this task (pure validation).**

---

## Self-review checklist (plan author)

- [x] Every step has exact file paths.
- [x] Every code step shows actual code (no "implement the function" without the function body).
- [x] Every test step says how to run it and what to expect.
- [x] Commits are asked-for, not auto-taken (every commit gated by "Ask before committing").
- [x] `make lint` + `make test` run at Task 5; `make ci` final gate at Task 9.
- [x] Types and names consistent across tasks: `with-belief-scope`, `load-committed-beliefs`, `suppress-known`, `sort-scheme-filenames`, `list-scheme-files-in-dir`, `load-belief-file`, `belief-expressions-match?`, `any-tuple-matches?`, `result-matches-any?`, `mustWriteFile`, `schemeStr`, `runScheme`.
- [x] Every design-spec test (`TestWithBeliefScope_Restores`, `TestWithBeliefScope_RestoresOnEscape`, `TestLoadCommittedBeliefs_Directory`, `TestLoadCommittedBeliefs_File`, `TestLoadCommittedBeliefs_SkipsBadFiles`, `TestLoadCommittedBeliefs_NonexistentPath`, `TestSuppressKnown_PerSiteMatch`, `TestSuppressKnown_RenameIgnored`, `TestSuppressKnown_ThresholdIgnored`, `TestSuppressKnown_AggregateMatch`, `TestSuppressKnown_NoMatch`, `TestSuppressKnown_EndToEnd`) maps to a task. Extra `TestSuppressKnown_MissingTypePassesThrough` covers the "missing type key" edge-case row in §Edge Cases.
- [x] Every design-spec requirement is covered:
  - `with-belief-scope` via `dynamic-wind` → Task 1
  - `load-committed-beliefs` directory-or-file, per-file `guard` + stderr warning, nonexistent path raises → Task 2
  - `suppress-known` dispatch on `type`, structural `equal?` on expression keys, unknown type passes through → Task 3
  - End-to-end demonstration → Task 4
  - Exports from `.sld` → Task 1 (all three batched up-front)
  - CHANGELOG entry → Task 6
  - Plan status flipped → Task 7
  - CLAUDE.md primitive docs → Task 8
  - `make ci` green gate → Tasks 5 + 9

---

## Resolved ambiguities

| # | Ambiguity | Resolution |
|---|---|---|
| 1 | Test helper name | Use existing `eval(t, engine, code)` helper from `goast/prim_goast_test.go:42`. |
| 2 | `with-belief-scope` body | `dynamic-wind` returns body value directly; no `result` capture needed — pass `thunk` literally. |
| 3 | `'#f` vs `#f` in tuples | Bare `#f` — placeholder for the fn slots that the match path never invokes. |
| 4 | `plans/BELIEF-DSL.md` §Suppression update | Has no checkboxes; rewrite opener to past tense + link impl plan, mark §CLI Integration as deferred. |
| 5 | `error-object?` / `error-object-message` availability | R7RS standard; confirmed shipped in wile (`memory/2026-04-16-error-diagnostics-impl.md`). |
| 6 | `directory-files` on non-directory | `os.ReadDir` raises — `guard` catches; falls through to single-file branch. Verified in `wile/extensions/files/prim_directory.go:100`. |
| 7 | `load-committed-beliefs` return shape | Design doc (pair) wins over earlier BELIEF-DSL.md text (list); BELIEF-DSL.md will be reconciled in Task 7. |
| 8 | `string-suffix?` unavailable at this scope | Use inline `substring` comparison in `list-scheme-files-in-dir` — self-contained, no new import. |
| 9 | `*beliefs*` read from user code returns a stale snapshot under Wile's library import semantics (verified: mutation via library-internal `set!` does not flow back to importing code's `*beliefs*` reference) | Add `(current-beliefs)` accessor procedure to belief.scm, symmetric to existing `(aggregate-beliefs)`; tests call `(current-beliefs)` instead of reading `*beliefs*` directly. |
