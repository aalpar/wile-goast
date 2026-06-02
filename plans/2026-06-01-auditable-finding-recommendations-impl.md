# Auditable Finding — Recommendation Provenance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans. Steps use checkbox (`- [ ]`).

**Goal:** Make the two remaining opinion-producers that report bare names —
`recommend-split` (package split) and `boundary-recommendations` (function
split/merge/extract) — emit located `finding`s, closing the provenance audit:
every soft conclusion in the suite is now walkable.

**Architecture:** The slice-4 name→source join applied to recommendations.
Additive siblings (the existing producers are unchanged; their MCP tools keep
working). `split` gets `recommend-split-findings` (locates `group-a`/`group-b`
func names via the `func-ref.pos` data, joined inline — `split` must not import
`dup-detect`, which would be circular). `fca-recommend` gets `locate-recommendations`
(locates each candidate's functions — `function` for split, `functions` for merge,
`broad-extent` for extract — via `field-index->positions`, already exported from
`(wile goast fca)` which fca-recommend imports). Both add a `(wile goast provenance)`
import for `make-finding` (provenance imports only `utils`, so no cycle).

**Tech Stack:** Wile (R7RS Scheme). `(wile goast provenance)` (`make-finding`),
`(wile goast fca)` (`field-index->positions`). Fixtures: `iface` (split),
`falseboundary` (fca-recommend). Go test harness (`newBeliefEngine`).

---

## HARNESS WORKAROUND
Go test files contain the eval helper → APPEND via `cat >> goast/recommendation_findings_test.go <<'EOF'`.
`.scm`/`.sld` edited normally; `.sld` exports grow per task.

---

## File Structure

| File | Responsibility | Action |
|------|----------------|--------|
| `lib/wile/goast/split.scm` | `recommend-split-findings` (locate group-a/group-b) | Modify |
| `lib/wile/goast/split.sld` | import provenance; export `recommend-split-findings` | Modify |
| `lib/wile/goast/fca-recommend.scm` | `recommendation-functions`, `locate-recommendations` | Modify |
| `lib/wile/goast/fca-recommend.sld` | import provenance; export the two | Modify |
| `goast/recommendation_findings_test.go` | split + fca-recommend located-findings tests | Create |
| `CLAUDE.md` | document the two located variants | Modify |

---

## Task 1: `recommend-split-findings`

**Files:** Modify lib/wile/goast/split.scm, lib/wile/goast/split.sld; Create goast/recommendation_findings_test.go

- [ ] **Step 1: Implement** — append to `lib/wile/goast/split.scm`:

```scheme
;; recommend-split-findings: located findings for a recommend-split REPORT.
;; group-a/group-b function names become findings located via the func-ref 'pos
;; (built inline — split must not depend on dup-detect's func-refs->positions,
;; which would be circular). why = (split-group (side . a|b)); score = #f.
(define (recommend-split-findings report func-refs)
  (let* ((pos-index
           (let ((h (make-hashtable)))
             (for-each
               (lambda (fr)
                 (let ((n (nf fr 'name)) (p (nf fr 'pos)))
                   (if (and (string? n) (string? p)) (hashtable-set! h n p))))
               (if (pair? func-refs) func-refs '()))
             h))
         (groups (let ((g (assoc 'groups report))) (if g (cdr g) '())))
         (ga (let ((x (assoc 'group-a groups))) (if x (cdr x) '())))
         (gb (let ((x (assoc 'group-b groups))) (if x (cdr x) '())))
         (mk (lambda (side)
               (lambda (fn)
                 (make-finding fn (hashtable-ref pos-index fn #f)
                               (list 'split-group (cons 'side side)) #f)))))
    (list (cons 'group-a (map (mk 'a) ga))
          (cons 'group-b (map (mk 'b) gb)))))
```

- [ ] **Step 2: Import + export** — in `lib/wile/goast/split.sld`: add
  `(wile goast provenance)` to the imports; add `recommend-split-findings` to exports.

- [ ] **Step 3: Create test header** (Write `goast/recommendation_findings_test.go`):

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

- [ ] **Step 4: Test** — APPEND:

```bash
cat >> goast/recommendation_findings_test.go <<'EOF'

func TestRecommendSplitFindings(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/goast/testdata/iface"
	out := eval(t, engine, `
		(import (wile goast split))
		(import (wile goast provenance))
		(define refs (go-func-refs "`+pkg+`"))
		(define report (recommend-split refs))
		(define f (recommend-split-findings report refs))
		(define all (append (cdr (assoc 'group-a f)) (cdr (assoc 'group-b f))))
		(render-category "split" all)
	`).SchemeString()

	t.Run("group functions are located at iface.go", func(t *testing.T) {
		c.Assert(strings.Contains(out, "iface.go"), qt.IsTrue, qt.Commentf("%s", out))
		c.Assert(strings.Contains(out, "split-group"), qt.IsTrue, qt.Commentf("%s", out))
	})
}
EOF
go test ./goast/ -run TestRecommendSplitFindings -v 2>&1 | tail -10
```

- [ ] **Step 5: Verify pass.** If `all` is empty (no groups / confidence NONE on
  this fixture), print `report` and pick a fixture/threshold that yields groups
  (e.g. `(recommend-split refs 'idf-threshold 0.0)` to keep more attributes), or
  use a package that splits. The located-finding logic is what's under test, not
  the split quality.

- [ ] **Step 6: Commit**

```bash
git add lib/wile/goast/split.scm lib/wile/goast/split.sld goast/recommendation_findings_test.go
git commit -m "feat(split): recommend-split-findings locates group members"
```

---

## Task 2: `locate-recommendations` (fca-recommend)

**Files:** Modify lib/wile/goast/fca-recommend.scm, lib/wile/goast/fca-recommend.sld; Append goast/recommendation_findings_test.go

- [ ] **Step 1: Implement** — append to `lib/wile/goast/fca-recommend.scm`:

```scheme
;; recommendation-functions: the function names a candidate concerns, by type —
;; split: the one 'function; merge: the 'functions group; extract: the
;; 'broad-extent (the over-broad operation's functions; 'sub-operation is an
;; attribute set, not functions).
(define (recommendation-functions c)
  (let ((type (cdr (assoc 'type c))))
    (cond ((eq? type 'split)   (list (cdr (assoc 'function c))))
          ((eq? type 'merge)   (cdr (assoc 'functions c)))
          ((eq? type 'extract) (cdr (assoc 'broad-extent c)))
          (else '()))))

;; locate-recommendations: attach located findings to each candidate. CANDIDATES
;; are from split-candidates/merge-candidates/extract-candidates (each carries a
;; function set). FIELD-INDEX is the go-ssa-field-index output that built the
;; lattice; its 'func names match the lattice objects, so field-index->positions
;; joins exactly. why = (recommendation (type . T)); score = #f. The candidate is
;; returned unchanged with a prepended 'findings entry (additive).
(define (locate-recommendations candidates field-index)
  (let ((pos-index (field-index->positions field-index)))
    (map (lambda (c)
           (let* ((type (cdr (assoc 'type c)))
                  (fns (recommendation-functions c))
                  (findings (map (lambda (fn)
                                   (make-finding fn (hashtable-ref pos-index fn #f)
                                                 (list 'recommendation (cons 'type type))
                                                 #f))
                                 fns)))
             (cons (cons 'findings findings) c)))
         candidates)))
```

- [ ] **Step 2: Import + export** — in `lib/wile/goast/fca-recommend.sld`: add
  `(wile goast provenance)` to imports; add `recommendation-functions` and
  `locate-recommendations` to exports.

- [ ] **Step 3: Test** — APPEND:

```bash
cat >> goast/recommendation_findings_test.go <<'EOF'

func TestLocateRecommendations(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/falseboundary"
	out := eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))
		(import (wile goast provenance))
		(define idx (go-ssa-field-index "`+pkg+`"))
		(define ctx (field-index->context idx 'write-only))
		(define lat (concept-lattice ctx))
		(define merges (merge-candidates lat))
		(define located (locate-recommendations merges idx))
		;; render the findings of the first located merge candidate
		(if (null? located) "NONE"
		  (render-category "merge" (cdr (assoc 'findings (car located)))))
	`).SchemeString()

	t.Run("merge candidate functions are located at falseboundary.go", func(t *testing.T) {
		c.Assert(out, qt.Not(qt.Equals), "NONE")
		c.Assert(strings.Contains(out, "falseboundary.go"), qt.IsTrue, qt.Commentf("%s", out))
		c.Assert(strings.Contains(out, "recommendation"), qt.IsTrue, qt.Commentf("%s", out))
	})
}
EOF
go test ./goast/ -run TestLocateRecommendations -v 2>&1 | tail -10
```

- [ ] **Step 4: Verify pass.** If `merges` is empty on falseboundary, try
  `split-candidates`/`extract-candidates`, or print the candidate count; the
  fixture co-mutates Cache+Index across UpdateBoth/Invalidate/Rebuild, which
  should yield a merge concept (extent ≥ 2 maintaining shared state).

- [ ] **Step 5: Full regression** — `go test ./goast/ 2>&1 | tail -3` → PASS
  (existing `boundary-recommendations`/`recommend-split` and their MCP tools
  untouched — the new functions are additive).

- [ ] **Step 6: Commit**

```bash
git add lib/wile/goast/fca-recommend.scm lib/wile/goast/fca-recommend.sld goast/recommendation_findings_test.go
git commit -m "feat(fca-recommend): locate-recommendations locates candidate functions"
```

---

## Task 3: Documentation

**Files:** Modify CLAUDE.md

- [ ] **Step 1:** In the Function Boundary Recommendations (`(wile goast
  fca-recommend)`) table add `recommendation-functions` and `locate-recommendations`
  (attach located findings to split/merge/extract candidates via
  `field-index->positions`).

- [ ] **Step 2:** In the Package Splitting (`(wile goast split)`) table add
  `recommend-split-findings` (locate group-a/group-b members).

- [ ] **Step 3:** In the Provenance section intro, note the audit is now closed:
  belief checkers, FCA boundary, deduplication, and now the split/boundary
  *recommendations* all emit located findings; lint carries positions natively.

- [ ] **Step 4:** `make test` → PASS.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: split + boundary recommendations now emit located findings"
```

---

## Self-Review

**Spec coverage:** the two name-only recommendation producers gain located-finding
siblings (Tasks 1-2), closing the provenance audit (Task 3 doc).

**Preserved invariants:** additive — `recommend-split`/`boundary-recommendations`
and their MCP tools are unchanged; the located variants are new functions. `split`
avoids a `dup-detect` cycle by joining func-ref positions inline. `score = #f`
(recommendations have no per-member confidence here).

**Placeholder scan:** none. Empirical uncertainties (whether the fixtures yield
groups/merge candidates) have diagnosis fallbacks (Task 1 Step 5, Task 2 Step 4).

**Type/name consistency:** `recommend-split-findings` reads the `groups`/`group-a`/
`group-b` shape `recommend-split` produces. `locate-recommendations` reads the
`type`/`function`/`functions`/`broad-extent` shape the candidate functions produce.
`make-finding` is `(wile goast provenance)`; `field-index->positions` is
`(wile goast fca)`; `go-func-refs`/`go-ssa-field-index` are Go primitives.
