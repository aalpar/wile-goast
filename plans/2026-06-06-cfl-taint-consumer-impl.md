# CFL Taint Consumer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the first wile-goast consumer of `(wile algebra cfl)` — a composable interprocedural taint analysis (`taint-flows`) for security, built on a generic valid-path reachability engine `(wile goast ifds)`.

**Architecture:** `(wile goast ifds)` builds the Reps–Horwitz–Sagiv–Rosay **valid-path grammar** over per-call-site brackets and solves it with `cfl-solve`, exposing context-sensitively-valid reachability over a call/return-labeled graph. `(wile goast taint)` instantiates it: converts a `go-callgraph` into call (open) / return (close) edges, cuts sanitizer nodes, and reports source→sink flows that respect call/return realizability.

**Tech Stack:** R7RS Scheme; `(wile algebra cfl)` (wile PR #766); `(srfi 1)`; wile-goast `go-callgraph` tagged-alist shape; Go test harness (`newBeliefEngine`/`eval` helper, quicktest).

**Design:** `plans/2026-06-06-cfl-taint-consumer-design.md`.

**Feasibility:** `newBeliefEngine` uses `WithProfile(KitchenSink)` + `WithSourceFS(StdLibFS)`, and `go.work` resolves local wile, so `(wile algebra cfl)` is importable. Release build needs a wile tag + go.mod bump (out of scope).

---

## The valid-path grammar (normative — load-bearing)

Per call-site id `i`: terminal `cfl-open-i` (call) and `cfl-close-i` (return). Start `VP`:

```
VP -> Ci VP | VP Oi | B            ; valid path
B  -> eps | B B | Oi B Ci          ; balanced (same-index)
```

Normalized to cfl arity-<=2 kernels (binary RHS are nonterminals; terminals via cfl-terminal),
per id `i` with nonterminals `Oi`/`Ci`/`Bxi`:

```
(cfl-epsilon 'B) (cfl-binary 'B 'B 'B) (cfl-unary 'VP 'B)
(cfl-binary 'B Oi Bxi) (cfl-binary Bxi 'B Ci)        ; B -> Oi B Ci
(cfl-binary 'VP Ci 'VP) (cfl-binary 'VP 'VP Oi)      ; VP unmatched close / open
(cfl-terminal Oi (open-label i)) (cfl-terminal Ci (close-label i))
```

Acceptance oracle: `open-i close-i` YES, `close-i open-j` YES, `close-i` YES,
**`open-i close-j` (i!=j) NO** (wrong-caller return — the precision over boolean reachability).

---

## File structure

- Create `lib/wile/goast/ifds.scm` + `ifds.sld` — engine (only file importing `(wile algebra cfl)`).
- Create `lib/wile/goast/taint.scm` + `taint.sld` — call-graph -> flows, predicate builders, default set.
- Create `goast/ifds_test.go`, `goast/taint_test.go` — Go-embeds-Scheme tests via `newBeliefEngine`/`eval`.

Tests run with `go test ./goast/ -run TestIFDS` etc. (Scheme libs load from disk via StdLibFS). `make test` runs all.

---

## Task 1: `(wile goast ifds)` — valid-path engine

**Files:** Create `lib/wile/goast/ifds.sld`, `lib/wile/goast/ifds.scm`; Test `goast/ifds_test.go`.

- [ ] **Step 1: `ifds.sld`.**

```scheme
(define-library (wile goast ifds)
  (export ifds-open-label ifds-close-label
          make-valid-path-grammar make-ifds-analysis ifds-reachable?)
  (import (scheme base) (srfi 1) (wile algebra cfl))
  (include "ifds.scm"))
```

- [ ] **Step 2: failing Go test (grammar-level canary).** In `goast/ifds_test.go` (package `goast_test`),
import `testing`, `qt "github.com/frankban/quicktest"`, `"github.com/aalpar/wile/values"`. The test
loads the lib via the `eval` helper, builds a 3-call-site analysis where a1 and a2 share call-site id 1
into p and b2 uses id 2, then asserts `(ifds-reachable? A "a1" "a2")` is `values.TrueValue` (matched
open-1/close-1) and `(ifds-reachable? A "a1" "b2")` is `values.FalseValue` (open-1 then close-2 —
mismatched). Scheme set up via the helper:

```
(import (wile goast ifds))
(define nodes '("a1" "a2" "b2" "p"))
(define call-sites (list (list "a1" 1 "p") (list "a2" 1 "p") (list "b2" 2 "p")))
(define A (make-ifds-analysis nodes call-sites))
```

- [ ] **Step 3: run, verify FAIL.** `go test ./goast/ -run TestIFDS_ValidPathCanary -v` — `make-ifds-analysis` unbound.

- [ ] **Step 4: implement `ifds.scm`.**

```scheme
;;; (wile goast ifds) — valid-path (realizable interprocedural path) reachability
;;; over (wile algebra cfl). Reps-Horwitz-Sagiv-Rosay valid-path grammar: every
;;; return matches its own call; returns to ancestors and descents into callees
;;; are allowed, but returning from an uncalled function is not.

(define (%id->string id)
  (cond ((string? id) id)
        ((symbol? id) (symbol->string id))
        ((number? id) (number->string id))
        (else (error "ifds: call-site id must be string/symbol/number" id))))

(define (ifds-open-label id)  (string->symbol (string-append "cfl-open-"  (%id->string id))))
(define (ifds-close-label id) (string->symbol (string-append "cfl-close-" (%id->string id))))
(define (%nt prefix id)       (string->symbol (string-append prefix (%id->string id))))

(define (make-valid-path-grammar call-site-ids)
  "Valid-path <cfl-grammar> (start VP) over CALL-SITE-IDS (distinct ids)."
  (let loop ((ids call-site-ids)
             (prods (list (cfl-epsilon 'B) (cfl-binary 'B 'B 'B) (cfl-unary 'VP 'B))))
    (if (null? ids)
        (make-cfl-grammar 'VP prods)
        (let* ((i (car ids)) (oi (%nt "O-" i)) (ci (%nt "C-" i)) (bx (%nt "Bx-" i)))
          (loop (cdr ids)
                (append (list (cfl-binary 'B oi bx) (cfl-binary bx 'B ci)
                              (cfl-binary 'VP ci 'VP) (cfl-binary 'VP 'VP oi)
                              (cfl-terminal oi (ifds-open-label i))
                              (cfl-terminal ci (ifds-close-label i)))
                        prods))))))

(define (make-ifds-analysis nodes call-sites)
  "Solve valid-path reachability over NODES with CALL-SITES (list of
(from-node id to-node)). Each site adds open edge from->to and close edge to->from."
  (let* ((ids (delete-duplicates (map cadr call-sites)))
         (edges (append-map
                  (lambda (cs)
                    (let ((from (car cs)) (id (cadr cs)) (to (caddr cs)))
                      (list (list from (ifds-open-label id) to)
                            (list to (ifds-close-label id) from))))
                  call-sites)))
    (cfl-solve (make-valid-path-grammar ids) (make-cfl-graph nodes edges))))

(define (ifds-reachable? analysis from to)
  "True iff TO is reachable from FROM along a valid interprocedural path."
  (cfl-reachable? analysis from to))
```

- [ ] **Step 5: run, verify PASS.** `go test ./goast/ -run TestIFDS_ValidPathCanary -v` — a1->a2 true,
a1->b2 false. If a1->b2 is true, the grammar admits a wrong-caller return; recheck against the oracle.

- [ ] **Step 6: commit.** `git add lib/wile/goast/ifds.sld lib/wile/goast/ifds.scm goast/ifds_test.go && git commit -m "feat(goast/ifds): valid-path reachability engine over (wile algebra cfl)"`

---

## Task 2: `(wile goast taint)` — flows + interprocedural canary

**Files:** Create `lib/wile/goast/taint.sld`, `lib/wile/goast/taint.scm`; Test `goast/taint_test.go`.

- [ ] **Step 1: `taint.sld`.**

```scheme
(define-library (wile goast taint)
  (export taint-flows)
  (import (scheme base) (srfi 1) (wile goast ifds))
  (include "taint.scm"))
```

- [ ] **Step 2: failing canary test.** In `goast/taint_test.go`, build a call graph as a tagged-alist
(helpers `(node name . outs)` -> `(cg-node (name . X) (id . 0) (edges-in . ()) (edges-out . outs))`;
`(edge from to)` -> `(cg-edge (caller . from) (callee . to) (description . "static"))`). The fixture must
make boolean reachability report source reaches sink while `taint-flows` returns `'()` (the only path is a
mismatched return — see Step 4). Initially assert `(taint-flows cg src? sink?)` returns `'()`.

- [ ] **Step 3: run, verify FAIL.** `go test ./goast/ -run TestTaint_InterproceduralCanary -v` — `taint-flows` unbound.

- [ ] **Step 4: implement `taint.scm`; finalize the canary fixture.**

```scheme
;;; (wile goast taint) — interprocedural taint flows over a Go call graph.
;;; Function-summary fidelity: nodes are functions; each call site f->g is a
;;; call (open) + return (close) edge; functions taint-transparent unless a
;;; sanitizer. taint-flows reports (source-name . sink-name) pairs connected by
;;; a VALID interprocedural path (see (wile goast ifds)).
;;; LIMITATION: function granularity over-approximates (no intraprocedural
;;; def-use) — sound-ish with false positives.

(define (%name n)      (let ((e (assq 'name (cdr n))))      (and e (cdr e))))
(define (%edges-out n) (let ((e (assq 'edges-out (cdr n)))) (if e (cdr e) '())))
(define (%callee e)    (let ((c (assq 'callee (cdr e))))    (and c (cdr c))))

(define (taint-flows cg sources sinks . opt)
  "Report taint flows over call graph CG. SOURCES/SINKS are predicates
(cg-node -> bool); optional SANITIZER predicate cuts flow through matches.
Returns a list of (source-name . sink-name) pairs joined by a valid
interprocedural path. Over-approximate (function granularity)."
  (let* ((san? (if (pair? opt) (car opt) (lambda (n) #f)))
         (live (filter (lambda (n) (not (san? n))) cg))
         (live-names (filter values (map %name live)))
         (live? (lambda (nm) (and (member nm live-names) #t)))
         (call-sites
           (let loop ((ns live) (i 0) (acc '()))
             (if (null? ns) (reverse acc)
                 (let ((f (%name (car ns))))
                   (let inner ((es (%edges-out (car ns))) (i i) (acc acc))
                     (cond ((null? es) (loop (cdr ns) i acc))
                           ((live? (%callee (car es)))
                            (inner (cdr es) (+ i 1)
                                   (cons (list f i (%callee (car es))) acc)))
                           (else (inner (cdr es) i acc))))))))
         (analysis (make-ifds-analysis live-names call-sites))
         (srcs (filter sources live))
         (snks (filter sinks live)))
    (append-map
      (lambda (s)
        (filter-map
          (lambda (t)
            (and (ifds-reachable? analysis (%name s) (%name t))
                 (cons (%name s) (%name t))))
          snks))
      srcs)))
```

Finalize the canary so the only source->sink connection is a mismatched return: assert
`(null? (taint-flows cg src? sink?))` is `#t`, and contrast with `(wile goast path-algebra)`'s
`go-callgraph-reachable` reporting the sink reachable from the source (boolean includes it).

- [ ] **Step 5: run, verify PASS.** `go test ./goast/ -run TestTaint -v`.

- [ ] **Step 6: commit.** `git add lib/wile/goast/taint.sld lib/wile/goast/taint.scm goast/taint_test.go && git commit -m "feat(goast/taint): interprocedural taint-flows on (wile goast ifds); canary"`

---

## Task 3: Composability — predicate builders + default Go security set

**Files:** Modify `lib/wile/goast/taint.scm`, `taint.sld`; Test `goast/taint_test.go`.

- [ ] **Step 1: failing test** for `taint-from-names` (exact-name predicate; a 2-node FormValue->Command cg
yields one flow) and that `taint-default-sources`/`-sinks` are procedures.

- [ ] **Step 2: run, verify FAIL** — `taint-from-names` unbound.

- [ ] **Step 3: implement + export.**

```scheme
;; --- Predicate builders (composable, LLM-authorable) ---
(define (taint-from-names names)
  "Predicate matching cg-nodes whose name is in NAMES (exact list of strings)."
  (lambda (n) (and (member (%name n) names) #t)))

(define (%string-contains? s sub)
  (let ((ls (string-length s)) (lsub (string-length sub)))
    (let loop ((i 0))
      (cond ((> (+ i lsub) ls) #f)
            ((string=? (substring s i (+ i lsub)) sub) #t)
            (else (loop (+ i 1)))))))

(define (taint-from-pattern substr)
  "Predicate matching cg-nodes whose name CONTAINS SUBSTR."
  (lambda (n) (let ((nm (%name n))) (and (string? nm) (%string-contains? nm substr)))))

;; --- Default Go security sets (starter; overridable) ---
(define taint-default-sources
  (taint-from-names '("net/http.Request.FormValue" "net/http.Request.PostFormValue"
                      "net/url.Values.Get" "os.Getenv" "bufio.Reader.ReadString")))
(define taint-default-sinks
  (taint-from-names '("os/exec.Command" "os/exec.CommandContext"
                      "database/sql.DB.Query" "database/sql.DB.Exec" "os.OpenFile" "os.ReadFile")))
(define taint-default-sanitizers
  (taint-from-names '("strconv.Atoi" "path/filepath.Clean")))
```

Add to `taint.sld` exports: `taint-from-names taint-from-pattern taint-default-sources
taint-default-sinks taint-default-sanitizers`.

- [ ] **Step 4: run, verify PASS.** `go test ./goast/ -run TestTaint -v`.

- [ ] **Step 5: commit.** `git add lib/wile/goast/taint.scm lib/wile/goast/taint.sld goast/taint_test.go && git commit -m "feat(goast/taint): predicate builders + default Go security source/sink set"`

---

## Task 4: Verification + close-out + PR

- [ ] **Step 1: full suite.** `make test`, then `make ci` (or `SKIP_LINT=1 make ci`). All green incl. `TestIFDS_*`/`TestTaint_*`.
- [ ] **Step 2: TODO.** In `TODO.md` Track C4, append to the CFL Follow-up note: `Follow-up SHIPPED (2026-06-06): (wile goast taint) interprocedural taint-flows on (wile goast ifds).`
- [ ] **Step 3: push + PR.** `git add TODO.md && git commit -m "docs(todo): CFL taint consumer shipped" && git push -u origin feat/cfl-taint && gh pr create --title "feat(goast): interprocedural taint via (wile algebra cfl)" --body "First wile-goast consumer of (wile algebra cfl): valid-path engine (wile goast ifds) + composable taint-flows (wile goast taint). Canary proves taint excludes call/return-infeasible paths boolean reachability includes. plans/2026-06-06-cfl-taint-consumer-*.md."`
- [ ] **Step 4: dual review** (Copilot + `/crosscheck:crosscheck all`); address findings; do NOT merge without instruction.

---

## Deferred to follow-up plans

- **Boolean pre-slice for scale** — slice the cg to source->sink-relevant functions via shipped
  `go-callgraph-reachable`, then CFL on the slice. v1 omits (correct on small graphs; infeasible whole-program). Log the bound (no silent truncation).
- **Statement/SSA-level taint; other IFDS domains (reaching-defs, null-flow); IFDS tabulation for scale.**

---

## Self-Review (spec coverage)

- Generic valid-path engine -> Task 1. Realizable grammar (non-Dyck) -> Task 1 normative section. taint-flows + sanitizer cut -> Task 2. Interprocedural canary -> Task 2 Step 4. Predicate builders + default set -> Task 3. Boolean pre-slice -> deferred (explicit). Verification + TODO + PR -> Task 4.

**Open implementer checkpoints (flagged):** (a) Task 2's canary fixture must encode a genuine
mismatched-return infeasibility (Step 4 instructs: boolean reaches, taint returns `'()`). (b) Confirm
`(srfi 1)` provides `append-map`/`filter-map`/`delete-duplicates`; if `filter-map` is absent, define it
locally as `(filter values (map f xs))`.
