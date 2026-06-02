# Auditable Finding — Remaining Belief Checkers Evidence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans. Steps use checkbox (`- [ ]`).

**Goal:** Give the four remaining belief checkers the located evidence tail that
`ordered` got in slice 3 — `paired-with`, `co-mutated`, `checked-before-use`,
`contains-call` stop discarding the positions they compute, so their per-site
findings carry `where`/`why` instead of being unlocated.

**Architecture:** Extends the auditable-categorization belief work
(`2026-06-01-auditable-finding-evidence-impl.md`). Each checker returns
`(verdict . evidence)` with `evidence = ((where . W) (why . Y) (score . S))`, or a
bare symbol when no position resolves — exactly `ordered`'s contract. Two shared
resolvers go in `(wile goast provenance)`: `ssa-first-pos` (first instruction
matching a predicate that has a position) and `ssa-func-call-position` (first call
to a function across all blocks). Positions come from the SSA the belief context
already builds with `'positions` on. `contains-call` is dual-use (predicate +
checker): it returns `(#t . evidence)` when present (predicate-truthy, checker
normalizes `#t`→present) and bare `#f` when absent (predicate-falsy) — verified
predicate-safe: `functions-matching`/`all-of`/`any-of`/`none-of` test truthiness,
not `(eq? _ #t)`.

**Tech Stack:** Wile (R7RS Scheme). `(wile goast provenance)` (`ssa-instr-pos`,
`ssa-call-to?`, `make-finding`), `(wile goast belief)` checkers + `ctx-find-ssa-func`.
Fixtures: `pairing`, `comutation`, `checking`. Go test harness (`newBeliefEngine`).

---

## HARNESS WORKAROUND
Go test files contain the eval helper → APPEND via `cat >> goast/belief_checker_evidence_test.go <<'EOF'`.
`.scm`/`.sld` edited normally; provenance `.sld` exports grow.

---

## File Structure

| File | Responsibility | Action |
|------|----------------|--------|
| `lib/wile/goast/provenance.scm` | `ssa-first-pos`, `ssa-func-call-position` | Modify |
| `lib/wile/goast/provenance.sld` | export the two resolvers | Modify |
| `lib/wile/goast/belief-checkers.scm` | `paired-with`, `co-mutated`, `checked-before-use` emit evidence | Modify |
| `lib/wile/goast/belief.scm` | `contains-call` emits evidence (present-located, absent-bare) | Modify |
| `goast/belief_checker_evidence_test.go` | per-checker evidence + contains-call predicate regression | Create |
| `CLAUDE.md` | note the four checkers now carry evidence | Modify |

---

## Task 1: shared position resolvers in provenance

**Files:** Modify lib/wile/goast/provenance.scm, lib/wile/goast/provenance.sld; Create goast/belief_checker_evidence_test.go

- [ ] **Step 1: Implement** — append to `lib/wile/goast/provenance.scm`:

```scheme
;; ssa-first-pos: source position of the first instruction in SSA-FN (across all
;; blocks) that satisfies PRED *and* has a resolved position, or #f. Lets
;; analyses locate the instruction they already identified (a store, a guard).
(define (ssa-first-pos ssa-fn pred)
  (let bloop ((blocks (or (nf ssa-fn 'blocks) '())))
    (if (null? blocks) #f
      (let iloop ((is (or (nf (car blocks) 'instrs) '())))
        (cond ((null? is) (bloop (cdr blocks)))
              ((and (pred (car is)) (ssa-instr-pos (car is)))
               => (lambda (p) p))
              (else (iloop (cdr is))))))))

;; ssa-func-call-position: source position of the first call to FUNC-NAME
;; anywhere in SSA-FN, or #f. The function-level companion to ssa-call-position
;; (which scopes to one block).
(define (ssa-func-call-position ssa-fn func-name)
  (ssa-first-pos ssa-fn (lambda (i) (ssa-call-to? i func-name))))
```

- [ ] **Step 2: Export** — add `ssa-first-pos ssa-func-call-position` to
  `provenance.sld`'s `(export ...)`.

- [ ] **Step 3: Create test header** (Write):

```go
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

	qt "github.com/frankban/quicktest"
)
```

- [ ] **Step 4: Verify the library still loads** —
  `go test ./goast/ -run TestBelief -count=1 2>&1 | tail -3` → PASS (provenance
  additions don't disturb existing beliefs).

- [ ] **Step 5: Commit**

```bash
git add lib/wile/goast/provenance.scm lib/wile/goast/provenance.sld goast/belief_checker_evidence_test.go
git commit -m "feat(provenance): ssa-first-pos + ssa-func-call-position resolvers"
```

---

## Task 2: `paired-with` emits located evidence

`where` = op-a's call position (the operation that needs pairing — the site to
inspect, and for `unpaired` exactly the bug). `why` carries both positions.

**Files:** Modify lib/wile/goast/belief-checkers.scm; Append goast/belief_checker_evidence_test.go

- [ ] **Step 1: Implement** — replace `paired-with`'s `cond` tail
  (`belief-checkers.scm` ~:86-89) so the verdict gains evidence:

```scheme
      (let* ((verdict (cond (has-defer-b 'paired-defer)
                            (has-call-b  'paired-call)
                            (else        'unpaired)))
             (fname (nf site 'name))
             (pkg-path (nf site 'pkg-path))
             (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname)))
             (pos-a (and ssa-fn (ssa-func-call-position ssa-fn op-a)))
             (pos-b (and ssa-fn (ssa-func-call-position ssa-fn op-b))))
        (if (not pos-a) verdict
          (cons verdict
                (list (cons 'where pos-a)
                      (cons 'why (list 'paired (cons 'a op-a) (cons 'b op-b)
                                       (cons 'relation verdict)
                                       (cons 'a-pos pos-a) (cons 'b-pos pos-b)))
                      (cons 'score #f))))))
```
(Wrap this around the existing `let*` that binds `has-defer-b`/`has-call-b`; keep
those bindings intact, just replace the final `cond`.)

- [ ] **Step 2: Test** — APPEND:

```bash
cat >> goast/belief_checker_evidence_test.go <<'EOF'

func TestPairedWithEvidence(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing"
	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast provenance))
		(reset-beliefs!)
		(define-belief "lock-pairing"
		  (sites (functions-matching (contains-call "Lock")))
		  (expect (paired-with "Lock" "Unlock"))
		  (threshold 0.5 1))
		(define res (car (run-beliefs "`+pkg+`")))
		(define fs (cdr (assoc 'findings res)))
	`)

	t.Run("findings are located at pairing.go with paired why", func(t *testing.T) {
		out := eval(t, engine, `
			(let loop ((xs fs))
			  (cond ((null? xs) #f)
			        ((let ((w (finding-where (car xs))))
			           (and (string? w) (substring? "pairing.go" w))) 
			         (render-why (finding-why (car xs))))
			        (else (loop (cdr xs)))))
		`).SchemeString()
		c.Assert(strings.Contains(out, "paired"), qt.IsTrue, qt.Commentf("%s", out))
		c.Assert(strings.Contains(out, "Unlock"), qt.IsTrue, qt.Commentf("%s", out))
	})
}
EOF
go test ./goast/ -run TestPairedWithEvidence -v 2>&1 | tail -10
```

- [ ] **Step 3: Verify pass.** If `substring?` is unbound, it is exported from
  `(wile goast utils)` as `string-contains?` — use that
  (`(string-contains? w "pairing.go")`). Confirm with
  `grep -n 'substring?\|string-contains?' lib/wile/goast/utils.sld`.

- [ ] **Step 4: Regression** — `go test ./goast/ -run TestBeliefCategory -count=1 2>&1 | tail -3` → PASS.

- [ ] **Step 5: Commit**

```bash
git add lib/wile/goast/belief-checkers.scm goast/belief_checker_evidence_test.go
git commit -m "feat(belief): paired-with emits located evidence"
```

---

## Task 3: `co-mutated` emits located evidence

`where` = the first field-store (`ssa-field-addr` to a named field). Verdict logic
unchanged (field index); SSA is consulted only for the location.

**Files:** Modify lib/wile/goast/belief-checkers.scm; Append goast/belief_checker_evidence_test.go

- [ ] **Step 1: Implement** — replace `co-mutated`'s body tail (`~:240-244`):

```scheme
      (if (not summary) 'missing
        (let* ((writes (writes-for-struct summary #f))
               (verdict (if (all-present? field-names writes) 'co-mutated 'partial))
               (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname)))
               (pos (and ssa-fn
                         (ssa-first-pos ssa-fn
                           (lambda (i) (and (tag? i 'ssa-field-addr)
                                            (member? (nf i 'field) field-names)))))))
          (if (not pos) verdict
            (cons verdict
                  (list (cons 'where pos)
                        (cons 'why (list 'co-mutated (cons 'fields field-names)
                                         (cons 'relation verdict)))
                        (cons 'score #f)))))))
```

- [ ] **Step 2: Test** — APPEND:

```bash
cat >> goast/belief_checker_evidence_test.go <<'EOF'

func TestCoMutatedEvidence(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/comutation"
	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast provenance))
		(reset-beliefs!)
		(define-belief "config-comutation"
		  (sites (functions-matching (stores-to-fields "Config" "Host")))
		  (expect (co-mutated "Host" "Port" "Timeout"))
		  (threshold 0.5 1))
		(define res (car (run-beliefs "`+pkg+`")))
		(define fs (cdr (assoc 'findings res)))
	`)

	t.Run("a finding is located at comutation.go with co-mutated why", func(t *testing.T) {
		out := eval(t, engine, `
			(let loop ((xs fs))
			  (cond ((null? xs) #f)
			        ((let ((w (finding-where (car xs))))
			           (and (string? w) (string-contains? w "comutation.go")))
			         (render-why (finding-why (car xs))))
			        (else (loop (cdr xs)))))
		`).SchemeString()
		c.Assert(strings.Contains(out, "co-mutated"), qt.IsTrue, qt.Commentf("%s", out))
	})
}
EOF
go test ./goast/ -run TestCoMutatedEvidence -v 2>&1 | tail -10
```

- [ ] **Step 3: Verify pass.** If no finding locates (field-addr names differ),
  print the SSA field-addr fields for a Config method to confirm `'field` values
  match `"Host"`/`"Port"`/`"Timeout"`.

- [ ] **Step 4: Commit**

```bash
git add lib/wile/goast/belief-checkers.scm goast/belief_checker_evidence_test.go
git commit -m "feat(belief): co-mutated emits located evidence"
```

---

## Task 4: `checked-before-use` emits located evidence

`where` = the guard (`ssa-if`) for `guarded`, or the value's def instruction when
it resolves; `unguarded` stays bare when nothing locates (no guard to point at).

**Files:** Modify lib/wile/goast/belief-checkers.scm; Append goast/belief_checker_evidence_test.go

- [ ] **Step 1: Implement** — replace `checked-before-use`'s `cond` (`~:258-263`):

```scheme
      (cond
        ((not ssa-fn) 'missing)
        ((defuse-reachable? ssa-fn (list value-pattern)
                            (lambda (i) (tag? i 'ssa-if)) fuel)
         (let ((pos (or (ssa-first-pos ssa-fn (lambda (i) (tag? i 'ssa-if)))
                        (ssa-first-pos ssa-fn
                          (lambda (i) (equal? (nf i 'name) value-pattern))))))
           (if (not pos) 'guarded
             (cons 'guarded
                   (list (cons 'where pos)
                         (cons 'why (list 'checked-before-use
                                          (cons 'value value-pattern)
                                          (cons 'relation 'guarded)))
                         (cons 'score #f))))))
        (else
          (let ((pos (ssa-first-pos ssa-fn
                       (lambda (i) (equal? (nf i 'name) value-pattern)))))
            (if (not pos) 'unguarded
              (cons 'unguarded
                    (list (cons 'where pos)
                          (cons 'why (list 'checked-before-use
                                           (cons 'value value-pattern)
                                           (cons 'relation 'unguarded)))
                          (cons 'score #f))))))))
```

- [ ] **Step 2: Test** — APPEND:

```bash
cat >> goast/belief_checker_evidence_test.go <<'EOF'

func TestCheckedBeforeUseEvidence(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"
	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast provenance))
		(reset-beliefs!)
		(define-belief "err-checked"
		  (sites (functions-matching (has-params "error")))
		  (expect (checked-before-use "err"))
		  (threshold 0.5 1))
		(define res (car (run-beliefs "`+pkg+`")))
		(define fs (cdr (assoc 'findings res)))
	`)

	t.Run("a guarded finding is located at checking.go", func(t *testing.T) {
		out := eval(t, engine, `
			(let loop ((xs fs))
			  (cond ((null? xs) #f)
			        ((let ((w (finding-where (car xs))))
			           (and (string? w) (string-contains? w "checking.go")))
			         (render-why (finding-why (car xs))))
			        (else (loop (cdr xs)))))
		`).SchemeString()
		c.Assert(strings.Contains(out, "checked-before-use"), qt.IsTrue, qt.Commentf("%s", out))
	})
}
EOF
go test ./goast/ -run TestCheckedBeforeUseEvidence -v 2>&1 | tail -10
```

- [ ] **Step 3: Verify pass.** If no finding locates, the `has-params "error"`
  selector may match no sites — confirm with
  `(length (run-beliefs ...))`/the `total` field; adjust the selector to
  `(name-matches "HandleSafe")` if needed.

- [ ] **Step 4: Commit**

```bash
git add lib/wile/goast/belief-checkers.scm goast/belief_checker_evidence_test.go
git commit -m "feat(belief): checked-before-use emits located evidence"
```

---

## Task 5: `contains-call` emits located evidence (present-located, absent-bare)

Dual-use safe: `(#t . evidence)` when present (a call resolves), bare `#f` when
absent. Predicate consumers test truthiness; the runner normalizes `#t`→present.

**Files:** Modify lib/wile/goast/belief.scm; Append goast/belief_checker_evidence_test.go

- [ ] **Step 1: Implement** — replace `contains-call`'s final lambda body
  (`belief.scm:525-526`):

```scheme
  (lambda (fn ctx)
    (if (not (pair? (walk (or (nf fn 'body) '()) call-matches?)))
        #f
        (let* ((fname (nf fn 'name))
               (pkg-path (nf fn 'pkg-path))
               (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname)))
               (pos (and ssa-fn
                         (let loop ((ns func-names))
                           (and (pair? ns)
                                (or (ssa-func-call-position ssa-fn (car ns))
                                    (loop (cdr ns))))))))
          (if (not pos)
              #t
              (cons #t (list (cons 'where pos)
                             (cons 'why (list 'contains-call
                                              (cons 'funcs func-names)
                                              (cons 'pos pos)))
                             (cons 'score #f))))))))
```

- [ ] **Step 2: Test** — APPEND (both the evidence path AND the predicate-safety
  regression):

```bash
cat >> goast/belief_checker_evidence_test.go <<'EOF'

func TestContainsCallEvidence(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing"
	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast provenance))
		(reset-beliefs!)
		;; checker use: expect contains-call -> present findings located at the call
		(define-belief "uses-lock"
		  (sites (methods-of "Service"))
		  (expect (contains-call "Lock"))
		  (threshold 0.5 1))
		(define res (car (run-beliefs "`+pkg+`")))
		(define fs (cdr (assoc 'findings res)))
	`)

	t.Run("present findings are located with contains-call why", func(t *testing.T) {
		out := eval(t, engine, `
			(let loop ((xs fs))
			  (cond ((null? xs) #f)
			        ((let ((w (finding-where (car xs))))
			           (and (string? w) (string-contains? w "pairing.go")))
			         (render-why (finding-why (car xs))))
			        (else (loop (cdr xs)))))
		`).SchemeString()
		c.Assert(strings.Contains(out, "contains-call"), qt.IsTrue, qt.Commentf("%s", out))
	})

	t.Run("predicate use still selects sites (truthiness preserved)", func(t *testing.T) {
		// contains-call as a functions-matching predicate must still filter.
		out := eval(t, engine, `
			(reset-beliefs!)
			(define-belief "lp"
			  (sites (functions-matching (contains-call "Lock")))
			  (expect (paired-with "Lock" "Unlock"))
			  (threshold 0.5 1))
			(let ((r (car (run-beliefs "`+pkg+`"))))
			  (>= (cdr (assoc 'total r)) 1))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})
}
EOF
go test ./goast/ -run TestContainsCallEvidence -v 2>&1 | tail -10
```

- [ ] **Step 3: Verify pass + full belief regression** —
  `go test ./goast/ -run TestBelief -count=1 2>&1 | tail -3` → PASS (the
  predicate-safety of `(#t . ev)` is the key risk; this proves selectors still work).

- [ ] **Step 4: Commit**

```bash
git add lib/wile/goast/belief.scm goast/belief_checker_evidence_test.go
git commit -m "feat(belief): contains-call emits located evidence (present), bare #f (absent)"
```

---

## Task 6: Documentation

**Files:** Modify CLAUDE.md

- [ ] **Step 1:** In the Property Checkers section, update the prose noting that
  `paired-with`, `co-mutated`, `checked-before-use`, and `contains-call` now also
  emit the evidence tail (joining `ordered`); `contains-call` carries evidence only
  on `present` (bare `#f` on absent, preserving predicate use).

- [ ] **Step 2:** In the Provenance table, add `ssa-first-pos` and
  `ssa-func-call-position`.

- [ ] **Step 3:** `make test` → PASS.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: four belief checkers now emit located evidence"
```

---

## Self-Review

**Spec coverage:** the four remaining checkers each gain `ordered`'s evidence
tail (Tasks 2-5), on two shared resolvers (Task 1). `contains-call`'s dual-use is
handled present-located/absent-bare with a predicate regression test.

**Preserved invariants:** bare-symbol returns stay valid (no position → bare);
voting unchanged (category = `car` of the pair); evidence is additive. The
`contains-call` predicate path is verified truthiness-safe (Task 5 Step 2/3).

**Placeholder scan:** none. Empirical uncertainties (`substring?` vs
`string-contains?`; selector matching; field-addr `'field` values) have inline
fallbacks (Task 2 Step 3, Task 3 Step 3, Task 4 Step 3).

**Type/name consistency:** `ssa-first-pos`/`ssa-func-call-position` (provenance,
Task 1) are consumed by all four checkers. `ssa-call-to?`/`ssa-instr-pos` are
existing provenance internals; `ctx-find-ssa-func`/`walk`/`tag?`/`nf`/`member?`/
`find-field-summary`/`writes-for-struct`/`all-present?`/`defuse-reachable?` are
existing belief internals.
