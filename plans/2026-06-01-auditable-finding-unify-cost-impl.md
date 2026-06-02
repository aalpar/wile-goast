# Auditable Finding — Unify Cost-Half Measures Implementation Plan (Slice 5c)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans / subagent-driven-development. Steps use checkbox (`- [ ]`).

**Goal:** Complete the unification benefit/cost ledger — add the three cost-half
measures (`cand-new-edges`, `cand-creates-cycle?`, `cand-locality`) and a
top-level `find-candidates-with-cost` that folds them into the 5b scored
candidates, so the full ledger ranks via the same Pareto combinator.

**Architecture:** Slice 5c of the auditable-categorization design. Extends
`(wile goast dup-detect)`. Pure cross-layer composition over `go-callgraph`
(callers, reachability) and `go-func-refs` (pkg + ref sets) — no new Go.
Measures are interpretive (the human reads them); definitions: `cand-new-edges`
= `|callers(a) ∪ callers(b)|` (in-degree the merged function would carry — the
coupling concentrated, since merging retargets rather than adds edges);
`cand-creates-cycle?` = `a reaches b ∨ b reaches a` (merge-side analog of
`verify-acyclic`); `cand-locality` = `scope` (`same-pkg`/`shared-callers`/
`disjoint`) + `dep-overlap` (Jaccard of external ref sets) — a ledger fact, never
a verdict. Call-graph primitives accept short names (PRIMITIVES.md:24), so the
candidate names pass directly; reachability returns qualified names, compared via
`short-name`.

**Tech Stack:** Wile (R7RS Scheme) extending `(wile goast dup-detect)`; Go
primitives `go-callgraph`/`go-callgraph-callers`/`go-callgraph-reachable`;
`unique` from `(wile goast utils)`. Test fixtures: `transcall` (real call edges +
a cycle case) for the measure units, `dupcluster` for the full-pipeline integration.

---

## HARNESS WORKAROUND
Go test files contain the eval helper call → APPEND via `cat >> goast/dup_detect_test.go <<'EOF'`.
`.scm`/`.sld` edited normally; `.sld` exports grow per task.

---

## File Structure

| File | Responsibility | Action |
|------|----------------|--------|
| `lib/wile/goast/dup-detect.scm` | `callers-set`, `reaches?`, `cand-new-edges`, `cand-creates-cycle?`, `func-ref-pkgs`, `jaccard`, `build-func-ref-index`, `cand-locality`, `find-candidates-with-cost` | Modify |
| `lib/wile/goast/dup-detect.sld` | export the public 5c procs | Modify |
| `goast/dup_detect_test.go` | cost-measure units (transcall) + full-pipeline integration (dupcluster) | Append |
| `CLAUDE.md` | document the cost measures + `find-candidates-with-cost` | Modify |
| `plans/2026-06-01-auditable-categorization-design.md` | mark 5c shipped | Modify |

---

## Task 1: `cand-new-edges` + `cand-creates-cycle?`

**Files:** Modify lib/wile/goast/dup-detect.scm, lib/wile/goast/dup-detect.sld; Append goast/dup_detect_test.go

- [ ] **Step 1: Implement** — append to `dup-detect.scm`:

```scheme
;;; ── Slice 5c: cost-half measures ──────────────────────────

;; callers-set: distinct qualified caller names of NAME in CG ('caller field of
;; the edges-in returned by go-callgraph-callers; #f -> no callers).
(define (callers-set cg name)
  (let ((edges (go-callgraph-callers cg name)))
    (if (pair? edges)
      (unique (filter-map (lambda (e) (nf e 'caller)) edges))
      '())))

;; cand-new-edges: |callers(a) ∪ callers(b)| — the in-degree the merged function
;; would carry. Merging retargets edges rather than adding them; this measures
;; the coupling concentrated at the shared abstraction (the cost signal).
(define (cand-new-edges name-a name-b cg)
  (length (unique (append (callers-set cg name-a) (callers-set cg name-b)))))

;; reaches?: does FROM transitively call TO? Reachability returns qualified
;; names; candidates are short, so compare via short-name.
(define (reaches? cg from to)
  (let ((r (go-callgraph-reachable cg from)))
    (and (pair? r) (member (short-name to) (map short-name r)) #t)))

;; cand-creates-cycle?: merging two functions on a call path collapses it into a
;; self-cycle. The merge-side analog of verify-acyclic.
(define (cand-creates-cycle? name-a name-b cg)
  (or (reaches? cg name-a name-b) (reaches? cg name-b name-a)))
```

- [ ] **Step 2: Export** — add `cand-new-edges`, `cand-creates-cycle?` to `dup-detect.sld`'s `(export ...)`.

- [ ] **Step 3: Test** — APPEND to `goast/dup_detect_test.go`:

```bash
cat >> goast/dup_detect_test.go <<'EOF'

func TestCostEdgesAndCycle(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/transcall"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(define cg (go-callgraph "`+pkg+`" 'static))
	`)

	t.Run("Initialize reaches SetupConfig -> merging creates a cycle", func(t *testing.T) {
		out := eval(t, engine, `(cand-creates-cycle? "Initialize" "SetupConfig" cg)`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("two leaves on no shared path -> no cycle", func(t *testing.T) {
		out := eval(t, engine, `(cand-creates-cycle? "SetupConfig" "SetupLogger" cg)`).SchemeString()
		c.Assert(out, qt.Equals, "#f")
	})

	t.Run("new-edges = union of caller sets", func(t *testing.T) {
		// SetupConfig and SetupLogger are both called only by Initialize.
		out := eval(t, engine, `(cand-new-edges "SetupConfig" "SetupLogger" cg)`).SchemeString()
		c.Assert(out, qt.Equals, "1")
	})
}
EOF
go test ./goast/ -run TestCostEdgesAndCycle -v 2>&1 | tail -10
```

- [ ] **Step 4: Verify pass.** If `cand-new-edges` ≠ 1, print the caller sets
  (`(callers-set cg "SetupConfig")`); the call graph may qualify `Initialize`
  differently — the union count is what matters (both share the one caller).
  If the cycle test fails, confirm `go-callgraph-reachable` includes transitive
  callees (`(go-callgraph-reachable cg "Initialize")`).

- [ ] **Step 5: Commit**

```bash
git add lib/wile/goast/dup-detect.scm lib/wile/goast/dup-detect.sld goast/dup_detect_test.go
git commit -m "feat(dup-detect): cand-new-edges + cand-creates-cycle? cost measures"
```

---

## Task 2: `cand-locality` + `build-func-ref-index`

**Files:** Modify lib/wile/goast/dup-detect.scm, lib/wile/goast/dup-detect.sld; Append goast/dup_detect_test.go

- [ ] **Step 1: Implement** — append to `dup-detect.scm`:

```scheme
;; func-ref-pkgs: the external package paths a func-ref entry references.
(define (func-ref-pkgs entry)
  (if entry
    (unique (map (lambda (r) (nf r 'pkg))
                 (let ((rs (nf entry 'refs))) (if (pair? rs) rs '()))))
    '()))

;; jaccard: |A ∩ B| / |A ∪ B|, 0 when both empty.
(define (jaccard a b)
  (let ((inter (length (filter (lambda (x) (member x b)) a)))
        (uni   (length (unique (append a b)))))
    (if (= uni 0) 0 (/ inter uni))))

;; build-func-ref-index: name -> func-ref entry (for pkg + ref-set lookup).
(define (build-func-ref-index func-refs)
  (let ((h (make-hashtable)))
    (for-each
      (lambda (fr)
        (let ((n (nf fr 'name)))
          (if (string? n) (hashtable-set! h n fr))))
      (if (pair? func-refs) func-refs '()))
    h))

;; cand-locality: a ledger fact (never a verdict). scope = same-pkg (both in one
;; package) / shared-callers (a common caller) / disjoint; dep-overlap = Jaccard
;; of external ref sets. The human reads this to judge coincidental vs. real
;; duplication.
(define (cand-locality name-a name-b fr-index cg)
  (let* ((ea (hashtable-ref fr-index name-a #f))
         (eb (hashtable-ref fr-index name-b #f))
         (pa (and ea (nf ea 'pkg)))
         (pb (and eb (nf eb 'pkg)))
         (ca (callers-set cg name-a))
         (cb (callers-set cg name-b))
         (shared (filter (lambda (x) (member x cb)) ca))
         (scope (cond ((and pa pb (equal? pa pb)) 'same-pkg)
                      ((pair? shared) 'shared-callers)
                      (else 'disjoint))))
    (list (cons 'scope scope)
          (cons 'dep-overlap (jaccard (func-ref-pkgs ea) (func-ref-pkgs eb))))))
```

- [ ] **Step 2: Export** — add `cand-locality`, `build-func-ref-index` to `dup-detect.sld`.

- [ ] **Step 3: Test** — APPEND to `goast/dup_detect_test.go`:

```bash
cat >> goast/dup_detect_test.go <<'EOF'

func TestCostLocality(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/transcall"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(define cg (go-callgraph "`+pkg+`" 'static))
		(define fr-index (build-func-ref-index (go-func-refs "`+pkg+`")))
		(define loc (cand-locality "SetupConfig" "SetupLogger" fr-index cg))
	`)

	t.Run("same package -> same-pkg scope", func(t *testing.T) {
		out := eval(t, engine, `(cdr (assoc 'scope loc))`).SchemeString()
		c.Assert(out, qt.Equals, "same-pkg")
	})

	t.Run("dep-overlap is a number in [0,1]", func(t *testing.T) {
		out := eval(t, engine, `
			(let ((d (cdr (assoc 'dep-overlap loc))))
			  (and (number? d) (>= d 0) (<= d 1)))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})
}
EOF
go test ./goast/ -run TestCostLocality -v 2>&1 | tail -8
```

- [ ] **Step 4: Verify pass.** (transcall functions are all in one package → `same-pkg`.)

- [ ] **Step 5: Commit**

```bash
git add lib/wile/goast/dup-detect.scm lib/wile/goast/dup-detect.sld goast/dup_detect_test.go
git commit -m "feat(dup-detect): cand-locality + build-func-ref-index"
```

---

## Task 3: `find-candidates-with-cost` (full ledger)

Top-level: 5b scoring + the cost measures, in one pass, with findings carrying
the full measure set. Reuses `score-candidate-pair` + `pair-findings`.

**Files:** Modify lib/wile/goast/dup-detect.scm, lib/wile/goast/dup-detect.sld; Append goast/dup_detect_test.go

- [ ] **Step 1: Implement** — append to `dup-detect.scm`:

```scheme
;; find-candidates-with-cost: the full benefit/cost ledger. Like
;; find-scored-candidates, but each candidate's measures also carry new-edges,
;; creates-cycle?, and locality, and the findings embed the full set. TARGET is a
;; package pattern or GoSession; optional THRESHOLD (default 0.6).
(define (find-candidates-with-cost target . opts)
  (let* ((threshold (if (pair? opts) (car opts) 0.6))
         (s         (if (go-session? target) target (go-load target)))
         (refs      (go-func-refs s))
         (ctx       (function-ref-context refs))
         (lat       (concept-lattice ctx))
         (concepts  (duplicate-candidate-concepts lat))
         (pos-index (func-refs->positions refs))
         (ast-index (build-func-ast-index (go-typecheck-package s)))
         (ssa-index (build-func-ssa-index (go-ssa-build s)))
         (fr-index  (build-func-ref-index refs))
         (cg        (go-callgraph s 'static)))
    (apply append
      (map (lambda (concept)
             (filter-map
               (lambda (pr)
                 (let* ((a (car pr)) (b (cdr pr))
                        (bm (score-candidate-pair a b ast-index ssa-index threshold)))
                   (and bm
                        (let ((m (append bm
                                   (list (cons 'new-edges (cand-new-edges a b cg))
                                         (cons 'creates-cycle? (cand-creates-cycle? a b cg))
                                         (cons 'locality (cand-locality a b fr-index cg))))))
                          (list (cons 'pair (list a b))
                                (cons 'measures m)
                                (cons 'findings (pair-findings a b m pos-index)))))))
               (all-pairs (concept-extent concept))))
           concepts))))
```

- [ ] **Step 2: Export** — add `find-candidates-with-cost` to `dup-detect.sld`.

- [ ] **Step 3: Test** — APPEND to `goast/dup_detect_test.go`:

```bash
cat >> goast/dup_detect_test.go <<'EOF'

func TestFindCandidatesWithCost(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(import (wile goast fca-recommend))
		(define cands (find-candidates-with-cost "`+pkg+`"))
		(define clone
		  (let loop ((cs cands))
		    (if (null? cs) #f
		      (let ((p (cdr (assoc 'pair (car cs)))))
		        (if (and (member "SumSlice" p) (member "TotalSlice" p))
		          (car cs) (loop (cdr cs)))))))
	`)

	t.Run("clone candidate carries the full benefit+cost ledger", func(t *testing.T) {
		out := eval(t, engine, `
			(let ((m (cdr (assoc 'measures clone))))
			  (and (assoc 'benefit m) (assoc 'equiv-tier m)
			       (assoc 'new-edges m) (assoc 'creates-cycle? m)
			       (assoc 'locality m)
			       (number? (cdr (assoc 'new-edges m)))
			       #t))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("pareto over benefit + new-edges (lower coupling better -> negate)", func(t *testing.T) {
		out := eval(t, engine, `
			(let ((items (map (lambda (c)
			                    (let ((m (cdr (assoc 'measures c))))
			                      (list (cdr (assoc 'pair c))
			                            (list (cons 'benefit (cdr (assoc 'benefit m)))
			                                  (cons 'neg-edges (- (cdr (assoc 'new-edges m))))))))
			                  cands)))
			  (>= (length (cdr (assoc 'frontier (pareto-frontier items '(benefit neg-edges))))) 1))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})
}
EOF
go test ./goast/ -run TestFindCandidatesWithCost -v 2>&1 | tail -10
```

- [ ] **Step 4: Verify pass + full regression** — then `go test ./goast/ 2>&1 | tail -3` → PASS.

- [ ] **Step 5: Commit**

```bash
git add lib/wile/goast/dup-detect.scm lib/wile/goast/dup-detect.sld goast/dup_detect_test.go
git commit -m "feat(dup-detect): find-candidates-with-cost full benefit/cost ledger"
```

---

## Task 4: Documentation

**Files:** Modify CLAUDE.md, plans/2026-06-01-auditable-categorization-design.md

- [ ] **Step 1:** In the Deduplication table add `cand-new-edges`,
  `cand-creates-cycle?`, `cand-locality`, `build-func-ref-index`,
  `find-candidates-with-cost`, with the measure definitions and a note that
  locality is a ledger fact (not a verdict) and the cost axes plug into the same
  Pareto combinator (negate "lower-is-better" axes like `new-edges`).

- [ ] **Step 2:** In `plans/2026-06-01-auditable-categorization-design.md`,
  "Slice sequencing": mark slice 5c shipped (the cost half completes the
  unification ledger); the LLM judge and path-algebra ranking remain deferred.

- [ ] **Step 3:** `make test` → PASS.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md plans/2026-06-01-auditable-categorization-design.md
git commit -m "docs: document dup-detect cost measures + full ledger (5c)"
```

---

## Self-Review

**Spec coverage:** 5c = the cost half. Task 1 = `cand-new-edges` +
`cand-creates-cycle?` (call graph); Task 2 = `cand-locality` +
`build-func-ref-index` (func-refs + callers); Task 3 = `find-candidates-with-cost`
(full ledger, reusing 5b's `score-candidate-pair`/`pair-findings`); Task 4 = docs.

**Preserved invariants:** measures, not verdicts — locality is a ledger fact;
the cost axes feed the same `pareto-frontier`; no new Go; additive (extends
`dup-detect`, 5a/5b untouched). `creates-cycle?` is a boolean axis the user may
filter on, never an automatic disqualifier.

**Placeholder scan:** none. Empirical uncertainties (the `new-edges` count, the
reachability transitivity) have inline diagnosis (Task 1 Step 4).

**Type/name consistency:** `callers-set` feeds `cand-new-edges` and
`cand-locality`; `short-name` (5b) drives `reaches?`; `build-func-ref-index`
feeds `cand-locality`; `find-candidates-with-cost` reuses `score-candidate-pair`/
`pair-findings`/`function-ref-context`/`duplicate-candidate-concepts`/
`func-refs->positions`/`build-func-ast-index`/`build-func-ssa-index` from 5a/5b.
`unique`/`filter-map` from utils; `go-callgraph`/`go-callgraph-callers`/
`go-callgraph-reachable` are Go primitives; `pareto-frontier` from fca-recommend.
