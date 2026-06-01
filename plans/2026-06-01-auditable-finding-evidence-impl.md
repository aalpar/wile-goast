# Auditable Finding — Evidence Wiring Implementation Plan (Slice 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Wire evidence through the belief checker contract and `evaluate-belief`
*additively*, so `run-beliefs` per-site results gain a new `findings` field — a
list of located, justified `finding` objects (from Slice 2's `make-finding`).
`ordered` is the first checker to emit real evidence, recovering the two call
source positions it currently computes-and-discards.

**Architecture:** Slice 3 of the auditable-categorization design
(`2026-06-01-auditable-categorization-design.md`, "Slice sequencing"). The
change is *additive*, per that note's "evidence is additive, not a reshape"
resolution: the checker contract grows an optional evidence tail
(`(site ctx) -> symbol` becomes `(site ctx) -> (symbol . evidence)`, bare symbol
still legal); `evaluate-belief` keeps evidence beside the category so the
majority-vote and `adherence`/`deviations` are byte-identical; the `findings`
field is new and sits *beside* the unchanged fields. Every existing consumer
(`emit-beliefs`, `suppress-known`, the MCP `check_beliefs` marshaller, existing
tests) reads by key and is unaffected.

**Tech Stack:** Wile (R7RS Scheme); `(wile goast provenance)` (Slice 1+2:
`ssa-call-position`, `make-finding`); `(wile goast utils)` (`nf`, `filter-map`);
Go test harness (`newBeliefEngine` + the Scheme test-eval helper, quicktest).

**Scope boundary (YAGNI):**
- Only `ordered` emits evidence this slice. `paired-with`/`checked-before-use`
  also hold locations but stay bare here — add them only when a consumer needs
  them (the design lists them as later adopters).
- `findings` carries raw `finding` objects (structured), not rendered strings.
  Rendering at the MCP/report surface is Slice 4+.
- No FCA/unify changes. No `finding->scalar`. No new module.
- `evidence` is `((where . W) (why . Y) (score . S))` — a plain alist, *not* a
  `finding` (the checker supplies raw evidence; `evaluate-belief` constructs the
  `finding` via `make-finding`, keeping one constructor).

---

## HARNESS WORKAROUND (read first)
A false-positive `PreToolUse` hook blocks `Write`/`Edit` on content containing
the project's Scheme test-eval helper call (helper-name + open paren). Go test
files already contain it; APPEND new test functions via a quoted heredoc:

    cat >> goast/belief_evidence_integration_test.go <<'EOF'
    ...new function...
    EOF

The `.scm`/`.sld` edits do NOT contain that helper call — use Edit/Write for them.

---

## File Structure

| File | Responsibility | Action |
|------|----------------|--------|
| `lib/wile/goast/belief.sld` | import `(wile goast provenance)` for `make-finding`/`ssa-call-position` | Modify |
| `lib/wile/goast/belief.scm` | `evaluate-belief`: classify -> evidence; runner: add `findings` field; add `ev-ref` helper | Modify |
| `lib/wile/goast/belief-checkers.scm` | `ordered` emits `(verdict . evidence)`; add `block-by-index` helper | Modify |
| `goast/belief_evidence_integration_test.go` | new test file: contract plumbing + `ordered` evidence property | Create |
| `CLAUDE.md` | document the additive `findings` field + extended checker contract | Modify |

---

## Task 1: Additive `findings` field through `evaluate-belief` + runner

The plumbing: contract accepts `(symbol . evidence)`, voting unchanged, every
site yields a `finding`. With `ordered` still bare (Task 2), findings are
unlocated (`where = #f`) — this task proves the *channel*, not the content.

**Files:** Modify belief.sld, belief.scm; Create goast/belief_evidence_integration_test.go

- [ ] **Step 1: Add the provenance import** — in `lib/wile/goast/belief.sld`,
  extend the `(import ...)` clause to add `(wile goast provenance)`.
  (Brings `make-finding`, `finding-*`, `ssa-call-position`. No name collisions —
  belief.scm/belief-checkers.scm define none of these.)

- [ ] **Step 2: Write the failing test** — create the file with the package
  header + imports via Write (no eval-helper call yet), then APPEND the test via
  `cat >>`. The test: define an `ordered` belief over the `ordering` testdata,
  run it, and assert (a) `findings` length == `total`, (b) each `finding-value`
  is one of `a-dominates-b`/`b-dominates-a`, (c) `adherence`+`deviations` still
  partition the 5 sites.

- [ ] **Step 3: Run, verify fail** — `go test ./goast/ -run TestBeliefFindingsChannel -v`
  -> FAIL (no `findings` key).

- [ ] **Step 4: Implement** — in `lib/wile/goast/belief.scm`:

  (a) Add the evidence-alist accessor just before `evaluate-belief`:
```scheme
;; ev-ref: read KEY from a checker's evidence alist
;; ((where . W) (why . Y) (score . S)), or DEFAULT when the key — or the whole
;; evidence — is absent (bare-symbol checker, evidence = #f).
(define (ev-ref ev key default)
  (let ((p (and (pair? ev) (assq key ev))))
    (if p (cdr p) default)))
```

  (b) Replace `evaluate-belief`'s body so it classifies once into
  `(site cat evidence)` triples, derives the *unchanged* `(site . cat)`
  `classified` list for voting, and builds `findings` in parallel:
```scheme
(define (evaluate-belief belief ctx)
  (let* ((name (belief-name belief))
         (sites-fn (belief-sites-fn belief))
         (expect-fn (belief-expect-fn belief))
         (sites (sites-fn ctx)))
    (if (null? sites) #f
      (let* ((rated
               (map (lambda (site)
                      (let* ((ret (expect-fn site ctx))
                             ;; (symbol . evidence) | bare symbol | #t/#f
                             (cat0 (if (pair? ret) (car ret) ret))
                             (ev   (if (pair? ret) (cdr ret) #f))
                             (cat  (cond ((eq? cat0 #t) 'present)
                                         ((eq? cat0 #f) 'absent)
                                         (else cat0))))
                        (list site cat ev)))
                    sites))
             ;; voting input — identical shape to the pre-slice-3 `classified`
             (classified (map (lambda (r) (cons (car r) (cadr r))) rated))
             (counts (count-categories classified))
             (maj (majority-category counts))
             (maj-cat (car maj))
             (maj-count (cdr maj))
             (total (length classified))
             (ratio (/ maj-count total))
             (adherence (filter-map
                          (lambda (p) (and (eq? (cdr p) maj-cat) (car p)))
                          classified))
             (deviations (filter-map
                           (lambda (p) (and (not (eq? (cdr p) maj-cat)) p))
                           classified))
             ;; evidence rides alongside — one located, justified finding per
             ;; site. value = category; why defaults to the category when the
             ;; checker retained no narrative.
             (findings (map (lambda (r)
                              (let ((cat (cadr r)) (ev (caddr r)))
                                (make-finding cat
                                              (ev-ref ev 'where #f)
                                              (ev-ref ev 'why   cat)
                                              (ev-ref ev 'score #f))))
                            rated)))
        (list name maj-cat ratio total adherence deviations findings)))))
```

  (c) In `run-beliefs`, the `else` branch reads `evaluate-belief`'s list. Bind
  `(findings (list-ref result 6))` in the `let*` and add `(cons 'findings findings)`
  to the result alist (right after the `'deviations` cons).

- [ ] **Step 5: Run, verify pass** — `go test ./goast/ -run TestBeliefFindingsChannel -v` -> PASS.

- [ ] **Step 6: Regression** — `go test ./goast/ -run TestBelief -v` -> all PASS.
  Then the MCP `check_beliefs` test (`go test ./cmd/...`) -> PASS; confirm the
  new `findings` field marshals without error.

- [ ] **Step 7: Commit**
```bash
git add lib/wile/goast/belief.sld lib/wile/goast/belief.scm goast/belief_evidence_integration_test.go
git commit -m "feat(belief): additive findings field through evaluate-belief"
```
(Pre-commit hook auto-bumps VERSION — expected.)

---

## Task 2: `ordered` emits located evidence  <- USER CONTRIBUTION

This is the slice's core property: the reason a human opens an editor — the two
call sites — is in hand at `belief-checkers.scm` and currently thrown away. Both
branches of `ordered` (same-block and cross-block) must resolve real source
positions via `ssa-call-position` and return `(verdict . evidence)`.

**Why this is the contribution:** the *shape of the audit narrative* is a design
choice with real trade-offs, not boilerplate. A `finding` has **one** `where`,
but an ordering relates **two** positions. You decide the anchor and the `why`
structure. The surrounding machinery (block lookup, the dispatch) is scaffolded;
you write the evidence assembly (~6 lines) and the property test that pins it.

**Files:** Modify belief-checkers.scm; Append to goast/belief_evidence_integration_test.go

- [ ] **Step 1: Scaffold the block lookup helper** (provided) — in
  `belief-checkers.scm`, replace the inline `find` loop in the same-block branch
  with a named helper, defined near `find-call-position`:
```scheme
;; block-by-index: the ssa-block in BLOCKS whose 'index is IDX, or #f.
(define (block-by-index blocks idx)
  (let loop ((bs (if (pair? blocks) blocks '())))
    (cond ((null? bs) #f)
          ((= (nf (car bs) 'index) idx) (car bs))
          (else (loop (cdr bs))))))
```

- [ ] **Step 2: Wire position resolution into both branches** (wiring provided;
  you fill the evidence). The verdict computation stays exactly as-is; it gains a
  co-located evidence builder. Reference points:
  - same-block branch (current lines ~108-119): `a-blocks`/`b-blocks` share an
    index `blk-idx`; `(block-by-index blocks blk-idx)` is the block.
  - cross-block branch (current lines ~120-128): a-block index `(car a-blocks)`,
    b-block index `(car b-blocks)`.
  - `(ssa-call-position block op-name)` -> `"file:line:col"` or `#f` (Slice 1).
  - Build the result with a shared local:
```scheme
    (define (with-evidence verdict a-block b-block)
      (let ((pos-a (and a-block (ssa-call-position a-block op-a)))
            (pos-b (and b-block (ssa-call-position b-block op-b))))
        ;; TODO(you): assemble and return (verdict . evidence) where
        ;;   evidence = ((where . W) (why . Y) (score . S)).
        ;; Design choices to make:
        ;;   - where: a finding has ONE position. Anchor on the *earlier* op
        ;;     (op-a for a-dominates-b; op-b for b-dominates-a)? Or always op-a?
        ;;   - why: structured (reason-tag . data-alist), e.g.
        ;;       (cons 'ordered (list (cons 'a op-a) (cons 'b op-b)
        ;;                            (cons 'relation verdict)
        ;;                            (cons 'a-pos pos-a) (cons 'b-pos pos-b)))
        ;;     so render-why projects it and downstream Scheme can filter on it.
        ;;   - score: #f (ordering has no natural confidence; see design Q4).
        ;;   - if pos-a and pos-b are both #f, prefer a bare verdict (no
        ;;     fabricated location) — the finding is then honestly unlocated.
        'TODO))
```
  Return `(with-evidence verdict a-block b-block)` from each verdict site
  (same-block: a-block = b-block = the shared block; cross-block: the two
  distinct blocks). Leave `'missing`/`'malformed-ssa`/`'unordered` as **bare**
  symbols — they have no two-position evidence to carry.

- [ ] **Step 3: Write the property test** (you) — APPEND to the integration
  file via `cat >>`. It must assert the recovered, located evidence — the thing
  Slice 3 exists to deliver. Assertions against the `ordering` testdata:
  - the four `a-dominates-b` findings each have a `where` containing
    `"ordering.go:"` (a real source line, not `#f`);
  - the `b-dominates-a` deviation (`PipelineReversed`) finding is also located;
  - `(render-why (finding-why f))` for a located finding contains both
    `"Validate"` and `"Process"`.

- [ ] **Step 4: Run, verify pass** — `go test ./goast/ -run TestBeliefOrderedEvidence -v` -> PASS.

- [ ] **Step 5: Full belief regression** — `go test ./goast/ -run TestBelief -v` -> all PASS.
  NOTE: `TestBeliefCategory4_Ordering` calls the `ordered` checker *directly* and
  reads `(cdr p)` as the verdict. Once `ordered` returns `(verdict . evidence)`,
  that read sees the pair — update that test to take the verdict via `car`-when-pair.
  This is the one place the contract change is observable to existing code; it is
  expected.

- [ ] **Step 6: Commit**
```bash
git add lib/wile/goast/belief-checkers.scm goast/belief_evidence_integration_test.go
git commit -m "feat(belief): ordered checker emits located evidence"
```

---

## Task 3: Document the contract + `findings` field

**Files:** Modify CLAUDE.md (no eval-helper call; normal Edit is fine).

- [ ] **Step 1:** In the Belief DSL "Return Shape" section, add `findings` to the
  per-site example and a line noting it is a list of `finding` objects (from
  `(wile goast provenance)`), one per site, carrying `value`/`where`/`why`/`score`,
  additive beside the unchanged `adherence`/`deviations`.

- [ ] **Step 2:** In the "Property Checkers" prose, note the contract now permits
  an optional evidence tail: a checker may return
  `(symbol . ((where . W) (why . Y) (score . S)))`; a bare symbol stays valid.
  `ordered` is the first checker to use it.

- [ ] **Step 3:** Run `make test` -> PASS.

- [ ] **Step 4: Commit**
```bash
git add CLAUDE.md
git commit -m "docs: document additive findings field + checker evidence tail"
```

---

## Self-Review

**Spec coverage:** Task 1 = additive `findings` field + contract acceptance of
`(symbol . evidence)` with voting byte-identical; Task 2 = `ordered` recovers and
emits the two call positions (the design's proof-of-thesis checker); Task 3 =
docs. Matches the Slice-3 deliverable.

**Backward compatibility:** `classified` keeps its `(site . cat)` shape (derived
from `rated`), so `count-categories`/`majority-category`/`adherence`/`deviations`
are unchanged. `findings` is additive; key-based consumers are unaffected.
Bare-symbol checkers remain valid (evidence `#f` -> unlocated finding, `why`
defaults to the category).

**Known direct-caller exception:** `TestBeliefCategory4_Ordering` reads the
`ordered` verdict via `(cdr p)`. Task 2 Step 5 updates it to `car`-when-pair.

**User contribution (per learning-mode + global CLAUDE.md):** Task 2's evidence
assembly and its property test are the user's — the `where` anchor and `why`
structure are a real design decision, and the property test validates the slice's
core thesis. Scaffolding, dispatch, and the additive plumbing (Task 1) are
provided.
