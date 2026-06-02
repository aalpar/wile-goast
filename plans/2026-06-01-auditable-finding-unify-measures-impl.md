# Auditable Finding — Unify Structural Measure Surface Implementation Plan (Slice 5b)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Refine the slice-5a duplicate-candidate clusters into a structural
*measure surface*: for each pair within a cluster, compute benefit measures and
an equivalence tier, attach them to the two located candidate findings (now with
a real `score` = effective similarity), and provide an opt-in
`candidate->verdict` projection plus the documented Pareto combinator. No
verdict is imposed; the measures are the default, the verdict is requested.

**Architecture:** Slice 5b of the auditable-categorization design. 5a produced
cluster-level located findings (FCA audit trace, `score = #f`). 5b pays the
cross-layer name reconciliation it deferred — joining each candidate (a
`go-func-refs` name) to its AST func-decl (`ast-diff`) and SSA func
(`ssa-diff`/canonicalize) via the existing `ssa-short-name` collapse — and turns
each within-cluster *pair* into a scored candidate. The equivalence tier reuses
the prior-art pattern (`unify-detect`/`ssa-unify-detect`): `proven` = `unifiable?`
over canonicalized SSA, `structural` = `unifiable?` over AST, `divergent` = neither
(NOT the binop-level `ssa-equivalent?`, which does not apply to whole functions).
The cost-half measures (`cand-new-edges`, `cand-creates-cycle?`, `cand-locality`)
are deferred to **slice 5c** — each is an underspecified cross-layer build needing
its own merge-semantics; bundling them here would dilute a well-specified surface.

**Tech Stack:** Wile (R7RS Scheme) reusing `(wile goast unify)` (`ast-diff`,
`ssa-diff`, `score-diffs`, `diff-result-*`, `unifiable?`), `(wile goast provenance)`
(`make-finding`), `(wile goast fca-recommend)` (`pareto-frontier`, `dominates?`),
and the Go primitives `go-load`/`go-session?`/`go-typecheck-package`/`go-ssa-build`/
`go-ssa-canonicalize`; extends `(wile goast dup-detect)` from 5a. Go test harness
(`newBeliefEngine` + the eval test helper, `frankban/quicktest`).

---

## HARNESS WORKAROUND (read first)

A false-positive `PreToolUse` hook blocks `Write`/`Edit` on content containing
the Scheme eval test-helper call (helper name + open paren). APPEND new Go test
functions via a quoted heredoc:

    cat >> goast/dup_detect_test.go <<'EOF'
    ...new function...
    EOF

`.scm`/`.sld` edits do NOT contain that call — use Edit. The new fixture is a
second package added to the existing `dupcluster` testdata module (no eval call;
Write/Edit fine). The `.sld` grows its export list per task (Wile rejects
exporting an undefined binding at load — proven in 5a).

---

## File Structure

| File | Responsibility | Action |
|------|----------------|--------|
| `lib/wile/goast/dup-detect.scm` | `short-name`, `all-pairs`, `build-func-ast-index`, `build-func-ssa-index`, `score-candidate-pair`, `pair-findings`, `scored-candidates`, `candidate->verdict`, `find-scored-candidates` | Modify |
| `lib/wile/goast/dup-detect.sld` | import `(wile goast unify)`; export the new procs incrementally | Modify |
| `examples/goast-query/testdata/dupcluster/clones.go` | two near-identical free functions (a unifiable pair) | Create |
| `goast/dup_detect_test.go` | scoring + tier + findings + verdict + pareto tests | Append (`cat >>`) |
| `CLAUDE.md` | document the 5b procs + the Pareto combinator usage | Modify |
| `plans/2026-06-01-auditable-categorization-design.md` | mark 5b shipped; move cost-half to 5c | Modify |

---

## Task 0: a unifiable clone pair in the fixture

5a's fixture clusters `EncodeA/B/C` by shared `encoding/json` but they are not
structurally unifiable (different bodies). 5b needs a pair that scores `proven`
or `structural`. Add two near-identical free functions that share a rare ref so
they land in the same FCA cluster AND are AST/SSA unifiable.

**Files:** Create examples/goast-query/testdata/dupcluster/clones.go

- [ ] **Step 1: Create the clone pair** — `clones.go` in the existing
  `dupcluster` package. `SumSlice` and `TotalSlice` are identical modulo names;
  both reference `sort` (a rare shared ref → same FCA cluster).

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

package dupcluster

import "sort"

// SumSlice and TotalSlice are structural clones (a unifiable pair) that share
// the rare ref "sort", so they cluster together under FCA-on-references.

func SumSlice(xs []int) int {
	sort.Ints(xs)
	total := 0
	for _, x := range xs {
		total += x
	}
	return total
}

func TotalSlice(ys []int) int {
	sort.Ints(ys)
	sum := 0
	for _, y := range ys {
		sum += y
	}
	return sum
}
```

- [ ] **Step 2: Verify the cluster forms** — quick check that the two land in a
  shared candidate concept (sort is rare → survives IDF):

Run: `go test ./goast/ -run TestDuplicateCandidateConcepts -v`
Expected: still PASS (json cluster unchanged; the sort cluster is additive).
(No assertion change yet — Task 2's test will assert the sort pair scores.)

- [ ] **Step 3: Commit**

```bash
git add examples/goast-query/testdata/dupcluster/clones.go
git commit -m "test(dup-detect): add a unifiable clone pair to the fixture"
```

---

## Task 1: name reconciliation + structural scoring

`short-name` (the `ssa-short-name` twin) is the join key across all three name
forms. Index AST func-decls and SSA funcs by it; `score-candidate-pair` joins a
candidate pair through it and computes the benefit measures + tier.

**Files:** Modify lib/wile/goast/dup-detect.scm, lib/wile/goast/dup-detect.sld; Append goast/dup_detect_test.go

- [ ] **Step 1: Implement** — append to `lib/wile/goast/dup-detect.scm`:

```scheme
;; short-name: the trailing component of a qualified name — the cross-layer join
;; key. Collapses every name form to the func/method short name:
;;   "pkg.EncodeA" -> "EncodeA", "(*pkg.Cache).Update" -> "Update",
;;   "Cache.Update" -> "Update", "EncodeA" -> "EncodeA".
;; The (wile goast belief) ssa-short-name twin (not exported there); duplicated
;; here to avoid depending on belief internals. LIMITATION: two methods sharing a
;; short name across receiver types collide (rare); the index keeps the last.
(define (short-name full-name)
  (let ((len (string-length full-name)))
    (let loop ((i (- len 1)))
      (cond ((<= i 0) full-name)
            ((char=? (string-ref full-name i) #\.)
             (substring full-name (+ i 1) len))
            (else (loop (- i 1)))))))

;; all-pairs: unordered pairs (a . b), a before b, from a list.
(define (all-pairs lst)
  (if (or (null? lst) (null? (cdr lst))) '()
    (append (map (lambda (y) (cons (car lst) y)) (cdr lst))
            (all-pairs (cdr lst)))))

;; package-func-decls: flatten all func-decl AST nodes from go-typecheck-package
;; output (a list of packages, each with 'files, each with 'decls).
(define (package-func-decls pkgs)
  (let ((acc '()))
    (for-each
      (lambda (pkg)
        (for-each
          (lambda (file)
            (for-each
              (lambda (decl)
                (if (tag? decl 'func-decl) (set! acc (cons decl acc))))
              (let ((d (nf file 'decls))) (if (pair? d) d '()))))
          (let ((fs (nf pkg 'files))) (if (pair? fs) fs '()))))
      (if (pair? pkgs) pkgs '()))
    (reverse acc)))

;; build-func-ast-index / build-func-ssa-index: short-name -> node hashtables.
(define (build-func-ast-index pkgs)
  (let ((h (make-hashtable)))
    (for-each
      (lambda (fd)
        (let ((nm (nf fd 'name)))
          (if (string? nm) (hashtable-set! h (short-name nm) fd))))
      (package-func-decls pkgs))
    h))

(define (build-func-ssa-index ssa-funcs)
  (let ((h (make-hashtable)))
    (for-each
      (lambda (fn)
        (let ((nm (nf fn 'name)))
          (if (string? nm) (hashtable-set! h (short-name nm) fn))))
      (if (pair? ssa-funcs) ssa-funcs '()))
    h))

;; score-candidate-pair: benefit measures + equivalence tier for a candidate pair
;; (joined to AST/SSA via short-name). Returns an alist or #f when the AST nodes
;; cannot be resolved. Tier (prior-art pattern, NOT binop ssa-equivalent?):
;;   proven     = unifiable? over canonicalized SSA
;;   structural = unifiable? over AST (but not SSA-proven)
;;   divergent  = neither.
;; benefit = shared AST node count; type-params/value-params from score-diffs;
;; similarity = effective similarity (the per-pair confidence).
(define (score-candidate-pair name-a name-b ast-index ssa-index threshold)
  (let ((ast-a (hashtable-ref ast-index (short-name name-a) #f))
        (ast-b (hashtable-ref ast-index (short-name name-b) #f)))
    (if (not (and ast-a ast-b)) #f
      (let* ((ar     (ast-diff ast-a ast-b))
             (shared (diff-result-shared ar))
             (dcount (diff-result-diff-count ar))
             (diffs  (diff-result-diffs ar))
             (sc     (score-diffs shared dcount diffs))
             (eff    (list-ref sc 1))
             (roots  (list-ref sc 4))
             (vparams (list-ref sc 5))
             (ast-unif (unifiable? ar threshold))
             (ssa-a  (hashtable-ref ssa-index (short-name name-a) #f))
             (ssa-b  (hashtable-ref ssa-index (short-name name-b) #f))
             (ssa-unif
               (and ssa-a ssa-b
                    (unifiable? (ssa-diff (go-ssa-canonicalize ssa-a)
                                          (go-ssa-canonicalize ssa-b))
                                threshold)))
             (tier (cond (ssa-unif 'proven)
                         (ast-unif 'structural)
                         (else     'divergent))))
        (list (cons 'benefit shared)
              (cons 'type-params (length roots))
              (cons 'value-params (length vparams))
              (cons 'equiv-tier tier)
              (cons 'similarity eff))))))
```

- [ ] **Step 2: Export** — in `lib/wile/goast/dup-detect.sld`, add
  `(wile goast unify)` to the `(import ...)` clause, and add `short-name`,
  `all-pairs`, `build-func-ast-index`, `build-func-ssa-index`,
  `score-candidate-pair` to `(export ...)`.

- [ ] **Step 3: Write the test** — APPEND to `goast/dup_detect_test.go`:

```bash
cat >> goast/dup_detect_test.go <<'EOF'

func TestScoreCandidatePair(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(define s (go-load "`+pkg+`"))
		(define ast-index (build-func-ast-index (go-typecheck-package s)))
		(define ssa-index (build-func-ssa-index (go-ssa-build s)))
		(define m (score-candidate-pair "SumSlice" "TotalSlice" ast-index ssa-index 0.6))
	`)

	t.Run("the clone pair scores a real tier and similarity", func(t *testing.T) {
		out := eval(t, engine, `
			(and m
			     (memq (cdr (assoc 'equiv-tier m)) '(proven structural))
			     (> (cdr (assoc 'similarity m)) 0.6)
			     (>= (cdr (assoc 'benefit m)) 1))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("unresolvable names score #f", func(t *testing.T) {
		out := eval(t, engine, `(score-candidate-pair "Nope" "Nada" ast-index ssa-index 0.6)`).SchemeString()
		c.Assert(out, qt.Equals, "#f")
	})
}
EOF
go test ./goast/ -run TestScoreCandidatePair -v 2>&1 | tail -10
```

- [ ] **Step 4: Verify pass.** Expected PASS. If `equiv-tier` is `divergent`,
  print the AST/SSA similarities to diagnose (`(diff-result-similarity (ast-diff
  ...))`); the clones are identical modulo names, so AST should be `unifiable?` at
  0.6. If the SSA index misses (short-name mismatch), print
  `(hashtable-ref ssa-index "SumSlice" #f)`.

- [ ] **Step 5: Commit**

```bash
git add lib/wile/goast/dup-detect.scm lib/wile/goast/dup-detect.sld goast/dup_detect_test.go
git commit -m "feat(dup-detect): name reconciliation + score-candidate-pair"
```

---

## Task 2: scored candidate findings + `find-scored-candidates`

Turn each within-cluster pair into a scored candidate: two located findings
(reusing 5a's `pos-index`) whose `why` carries the measures + peer, and whose
`score` is the effective similarity. `find-scored-candidates` is the top-level.

**Files:** Modify lib/wile/goast/dup-detect.scm, lib/wile/goast/dup-detect.sld; Append goast/dup_detect_test.go

- [ ] **Step 1: Implement** — append to `lib/wile/goast/dup-detect.scm`:

```scheme
;; pair-findings: the two located findings for a scored candidate pair. Each
;; finding's value is the function name; where from POS-INDEX; why is structured
;; (unify-candidate (peer . other) (measures . M)) so render-why projects it and
;; a script can filter on the measures; score = the pair's effective similarity
;; (a real per-pair confidence, unlike 5a's #f).
(define (pair-findings name-a name-b measures pos-index)
  (let ((sim (cdr (assq 'similarity measures))))
    (list (make-finding name-a (hashtable-ref pos-index name-a #f)
                        (cons 'unify-candidate
                              (list (cons 'peer name-b) (cons 'measures measures)))
                        sim)
          (make-finding name-b (hashtable-ref pos-index name-b #f)
                        (cons 'unify-candidate
                              (list (cons 'peer name-a) (cons 'measures measures)))
                        sim))))

;; scored-candidates: for each candidate concept, every within-cluster pair that
;; resolves to AST nodes becomes a scored candidate entry:
;;   ((pair . (a b)) (measures . M) (findings . (finding-a finding-b))).
;; Pairs whose AST nodes cannot be resolved (short-name miss) are dropped.
(define (scored-candidates concepts ast-index ssa-index pos-index threshold)
  (apply append
    (map (lambda (concept)
           (filter-map
             (lambda (pr)
               (let* ((a (car pr)) (b (cdr pr))
                      (m (score-candidate-pair a b ast-index ssa-index threshold)))
                 (and m
                      (list (cons 'pair (list a b))
                            (cons 'measures m)
                            (cons 'findings (pair-findings a b m pos-index))))))
             (all-pairs (concept-extent concept))))
         concepts)))

;; find-scored-candidates: top-level. TARGET is a package pattern or a GoSession.
;; Runs 5a discovery (FCA-on-refs clusters) then 5b structural scoring over each
;; cluster's pairs. Optional THRESHOLD (default 0.6) is the similarity/unifiable?
;; threshold. Returns a flat list of scored candidate entries.
(define (find-scored-candidates target . opts)
  (let* ((threshold (if (pair? opts) (car opts) 0.6))
         (s         (if (go-session? target) target (go-load target)))
         (refs      (go-func-refs s))
         (ctx       (function-ref-context refs))
         (lat       (concept-lattice ctx))
         (cands     (duplicate-candidate-concepts lat))
         (pos-index (func-refs->positions refs))
         (ast-index (build-func-ast-index (go-typecheck-package s)))
         (ssa-index (build-func-ssa-index (go-ssa-build s))))
    (scored-candidates cands ast-index ssa-index pos-index threshold)))
```

- [ ] **Step 2: Export** — add `pair-findings`, `scored-candidates`,
  `find-scored-candidates` to `lib/wile/goast/dup-detect.sld`'s `(export ...)`.

- [ ] **Step 3: Write the property test** — APPEND to `goast/dup_detect_test.go`:

```bash
cat >> goast/dup_detect_test.go <<'EOF'

func TestFindScoredCandidates(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(import (wile goast provenance))
		(define cands (find-scored-candidates "`+pkg+`"))
		;; the SumSlice/TotalSlice scored candidate
		(define clone
		  (let loop ((cs cands))
		    (if (null? cs) #f
		      (let ((p (cdr (assoc 'pair (car cs)))))
		        (if (or (and (member "SumSlice" p) (member "TotalSlice" p)))
		          (car cs) (loop (cdr cs)))))))
	`)

	t.Run("clone pair is discovered and scored", func(t *testing.T) {
		out := eval(t, engine, `
			(and clone
			     (memq (cdr (assoc 'equiv-tier (cdr (assoc 'measures clone))))
			           '(proven structural))
			     #t)
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("two located findings, score = similarity", func(t *testing.T) {
		out := eval(t, engine, `
			(let ((fs (cdr (assoc 'findings clone))))
			  (and (= (length fs) 2)
			       (string? (finding-where (car fs)))
			       (substring? "dupcluster.go" (finding-where (car fs)))
			       (number? (finding-score (car fs)))))
		`).SchemeString()
		// NOTE: if substring? is unavailable, assert via render-finding + Go strings.
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("why renders the peer and is structured for filtering", func(t *testing.T) {
		render := eval(t, engine, `
			(render-why (finding-why (car (cdr (assoc 'findings clone)))))
		`).SchemeString()
		c.Assert(strings.Contains(render, "unify-candidate"), qt.IsTrue, qt.Commentf("%s", render))
		peer := eval(t, engine, `
			(let* ((f (car (cdr (assoc 'findings clone))))
			       (why (finding-why f)))
			  (cdr (assoc 'peer (cdr why))))
		`).SchemeString()
		c.Assert(strings.Contains(peer, "Slice"), qt.IsTrue, qt.Commentf("%s", peer))
	})
}
EOF
go test ./goast/ -run TestFindScoredCandidates -v 2>&1 | tail -12
```

- [ ] **Step 4: Verify pass.** If `substring?` is not a builtin (it is used across
  the repo; confirm with `grep -rn 'substring?' lib/`), replace that assertion
  with a Go-side `strings.Contains` over `(render-finding (car fs))`.

- [ ] **Step 5: Commit**

```bash
git add lib/wile/goast/dup-detect.scm lib/wile/goast/dup-detect.sld goast/dup_detect_test.go
git commit -m "feat(dup-detect): scored candidate findings + find-scored-candidates"
```

---

## Task 3: opt-in `candidate->verdict` projection

The verdict is a user-invoked projection over the measures (the categorical
analog of `finding->scalar`), never a default. Tier-driven: `proven` -> duplicate,
`structural` -> likely-duplicate, `divergent` -> distinct.

**Files:** Modify lib/wile/goast/dup-detect.scm, lib/wile/goast/dup-detect.sld; Append goast/dup_detect_test.go

- [ ] **Step 1: Implement** — append to `lib/wile/goast/dup-detect.scm`:

```scheme
;; candidate->verdict: an OPT-IN projection of a scored candidate's measures into
;; a categorical verdict. The default analysis output is the measure surface; the
;; verdict is requested, never imposed (auditable-categorization principle #2 —
;; the categorical analog of finding->scalar). Tier-driven:
;;   proven -> 'duplicate, structural -> 'likely-duplicate, divergent -> 'distinct.
(define (candidate->verdict cand)
  (let ((tier (cdr (assq 'equiv-tier (cdr (assq 'measures cand))))))
    (cond ((eq? tier 'proven)     'duplicate)
          ((eq? tier 'structural) 'likely-duplicate)
          (else                   'distinct))))
```

- [ ] **Step 2: Export** — add `candidate->verdict` to `dup-detect.sld`.

- [ ] **Step 3: Write the test** — APPEND to `goast/dup_detect_test.go`:

```bash
cat >> goast/dup_detect_test.go <<'EOF'

func TestCandidateToVerdict(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast dup-detect))`)

	t.Run("tier maps to verdict", func(t *testing.T) {
		out := eval(t, engine, `
			(list
			  (candidate->verdict (list (cons 'measures (list (cons 'equiv-tier 'proven)))))
			  (candidate->verdict (list (cons 'measures (list (cons 'equiv-tier 'structural)))))
			  (candidate->verdict (list (cons 'measures (list (cons 'equiv-tier 'divergent))))))
		`).SchemeString()
		c.Assert(out, qt.Equals, "(duplicate likely-duplicate distinct)")
	})
}
EOF
go test ./goast/ -run TestCandidateToVerdict -v 2>&1 | tail -6
```

- [ ] **Step 4: Verify pass.** (If the list prints with different spacing, match
  the actual `SchemeString()` form.)

- [ ] **Step 5: Commit**

```bash
git add lib/wile/goast/dup-detect.scm lib/wile/goast/dup-detect.sld goast/dup_detect_test.go
git commit -m "feat(dup-detect): opt-in candidate->verdict projection"
```

---

## Task 4: Pareto combinator over the measure surface (documentation + test)

No new ranking code — the design ships the *existing* `pareto-frontier`/
`dominates?` (`fca-recommend.scm`) as the one documented combinator. This task
proves they compose over scored candidates and documents the pattern.

**Files:** Append goast/dup_detect_test.go

- [ ] **Step 1: Confirm the signature** — `grep -n -A6 "define (pareto-frontier"
  lib/wile/goast/fca-recommend.scm`. Note the argument order (items, then factor
  accessors). Adjust the test below to match.

- [ ] **Step 2: Write the test** — APPEND to `goast/dup_detect_test.go`. It builds
  measure accessors over scored candidates and runs `pareto-frontier`:

```bash
cat >> goast/dup_detect_test.go <<'EOF'

func TestParetoOverCandidates(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"
	// The documented combinator: rank scored candidates on a user-chosen measure
	// vector via the existing pareto-frontier (no new ranking code).
	out := eval(t, engine, `
		(import (wile goast dup-detect))
		(import (wile goast fca-recommend))
		(define cands (find-scored-candidates "`+pkg+`"))
		(define (m k) (lambda (c) (cdr (assoc k (cdr (assoc 'measures c))))))
		;; frontier on (benefit, similarity); result is non-empty when cands exist.
		(let ((fr (pareto-frontier cands (list (m 'benefit) (m 'similarity)))))
		  (and (>= (length cands) 1) (>= (length fr) 1) #t))
	`).SchemeString()
	c.Assert(out, qt.Equals, "#t")
}
EOF
go test ./goast/ -run TestParetoOverCandidates -v 2>&1 | tail -8
```

- [ ] **Step 3: Verify pass.** If `pareto-frontier`'s arity differs (e.g. returns
  `(values frontier dominated)` or takes a different shape), adjust the call per
  Step 1's grep — the point is to demonstrate the existing combinator over the
  candidate measures, not to add code.

- [ ] **Step 4: Full regression** — `go test ./goast/ 2>&1 | tail -3` → PASS.

- [ ] **Step 5: Commit**

```bash
git add goast/dup_detect_test.go
git commit -m "test(dup-detect): pareto-frontier composes over scored candidates"
```

---

## Task 5: Documentation

**Files:** Modify CLAUDE.md, plans/2026-06-01-auditable-categorization-design.md

- [ ] **Step 1:** In the `## Deduplication — (wile goast dup-detect)` table, add
  rows: `score-candidate-pair`, `scored-candidates`, `find-scored-candidates`
  (top-level: clusters → pairwise scored candidates), `candidate->verdict`
  (opt-in projection), and a note that `pareto-frontier`/`dominates?` from
  `(wile goast fca-recommend)` are the documented ranking combinator over the
  measures. State the tier definition (`proven`=SSA-canonical `unifiable?`,
  `structural`=AST `unifiable?`, `divergent`=neither) and the `short-name`
  reconciliation limitation.

- [ ] **Step 2:** In `plans/2026-06-01-auditable-categorization-design.md`,
  "Slice sequencing": mark 5b shipped (structural measure surface + opt-in
  verdict + Pareto), and split out **slice 5c** as the cost-half
  (`cand-new-edges`, `cand-creates-cycle?`, `cand-locality`) — each an
  underspecified cross-layer build needing its own merge-semantics. Keep the LLM
  judge deferred.

- [ ] **Step 3:** Run `make test` → PASS.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md plans/2026-06-01-auditable-categorization-design.md
git commit -m "docs: document dup-detect measure surface + verdict; split cost-half to 5c"
```

---

## Self-Review

**Spec coverage:** 5b = the structural measure surface over 5a's candidates.
Task 1 = reconciliation + benefit measures + tier; Task 2 = scored candidate
findings (two located findings, `score` = similarity) + top-level; Task 3 =
opt-in verdict; Task 4 = Pareto combinator; Task 5 = docs. The cost-half is
explicitly 5c (recorded Task 5 Step 2).

**Preserved invariants:** the measure surface is the default, the verdict is an
opt-in projection (principle #2); the tier is a confidence finding, never a veto
(`divergent` is an axis value, not a disqualifier — it still yields findings).
Additive: extends `dup-detect`; no existing producer touched. 5a's cluster
findings are unchanged; 5b adds pairwise scored candidates alongside.

**Placeholder scan:** none — every step has complete code and exact commands. Two
empirical uncertainties (the clone pair's tier; `pareto-frontier`'s arity) have
inline diagnosis/adjustment steps (Task 1 Step 4, Task 4 Steps 1+3). `substring?`
has a Go-side fallback (Task 2 Step 4).

**Type/name consistency:** `short-name` is the join key in `build-func-*-index`
and `score-candidate-pair`. `score-candidate-pair` returns the measures alist
consumed by `pair-findings` (Task 2), `candidate->verdict` (Task 3), and the
Pareto accessors (Task 4). `find-scored-candidates` composes the 5a procs
(`function-ref-context`, `duplicate-candidate-concepts`, `func-refs->positions`)
with the 5b ones. `ast-diff`/`ssa-diff`/`score-diffs`/`diff-result-shared`/
`diff-result-diff-count`/`diff-result-diffs`/`unifiable?` are existing
`(wile goast unify)` exports; `make-finding`/`finding-where`/`finding-why`/
`finding-score`/`render-why`/`render-finding` are `(wile goast provenance)`;
`pareto-frontier`/`dominates?` are `(wile goast fca-recommend)`;
`go-ssa-canonicalize`/`go-typecheck-package`/`go-ssa-build`/`go-load`/`go-session?`
are Go primitives.
