# Receiver-Parameter Asymmetry Detection — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `receiver-parameter-asymmetry` property checker to the belief DSL that flags Go methods reading exactly one receiver field, writing none, with >=1 non-receiver parameter — the "receiver as namespace" / Connascence-of-Meaning anti-pattern (design: `2026-04-20-receiver-parameter-asymmetry-design.md`).

**Architecture:** A new *named* property checker in `lib/wile/goast/belief-checkers.scm` (same namespace and shape as `ordered`/`co-mutated`/`checked-before-use`), exported from `belief.sld`. Pure Scheme over existing SSA primitives — **no new Go primitives**. The checker walks the SSA function's flat instruction list, distinguishes receiver field *reads* (`ssa-field-addr`/`ssa-field` with `x` = receiver param name, not feeding a store) from *writes* (a receiver `ssa-field-addr` whose register is some `ssa-store`'s `addr`), classifies each method, and emits a located finding for the `candidate` category. Implementation proceeds L1 (mechanical rule) -> interface-member exclusion -> L2 (joint-use refinement).

**Tech Stack:** Wile Scheme (R7RS), the `(wile goast belief)` library, SSA s-expressions from `go-ssa-build`, `(wile goast provenance)` findings, Go test harness (`newBeliefEngine` + the project's Scheme-test runner + quicktest).

---

## Background facts (verified against source 2026-06-05)

- **Checker shape** (`belief-checkers.scm:247` `co-mutated`): `(define (NAME . args) (lambda (site ctx) -> category | (category . evidence)))`. Evidence = `(list (cons 'where POS) (cons 'why (list TAG (cons k v) ...)) (cons 'score #f))`.
- **Site fields**: `(nf site 'name)` Form-3 name (`"(*pkg.Server).formatError"`), `(nf site 'pkg-path)` import path, `(nf site 'recv)` receiver list or `#f`.
- **SSA access**: `(ctx-find-ssa-func ctx pkg-path fname)` -> ssa-func or `#f`. `(nf ssa-fn 'params)` -> list of `ssa-param` nodes; `(car ...)` is the receiver; `(nf param 'name)` is its register name. `(ssa-all-instrs ssa-fn)` (from `(wile goast dataflow)`, already a belief dependency) flattens all instructions.
- **SSA instruction shapes** (`goastssa/mapper.go:275-311`):
  - `(ssa-store (addr . R) (val . R) (operands . (R R)))`
  - `(ssa-field-addr (name . R) (x . BASE) (struct . S) (field . F) (field-index . N) (type . T) (operands . (BASE)))` — pointer-receiver field access (address); read unless its `name` register is an `ssa-store` `addr`.
  - `(ssa-field (name . R) (x . BASE) (struct . S) (field . F) ...)` — value-receiver direct field read.
- **Position helper** (`provenance.scm:52` `ssa-first-pos`): `(ssa-first-pos ssa-fn (lambda (i) PRED))` -> `"file:line:col"` | `#f`.
- **Exported helpers in scope** (`belief.sld:42`): `nf tag? walk filter-map flat-map member? unique`; plus standard `length append car cdr cons positive? not equal?`.
- **Export point**: `belief.sld:39` Property-checkers export line — currently `paired-with ordered co-mutated`.
- **Test harness** (`goast/belief_checker_evidence_test.go:49`): `package goast_test`, `newBeliefEngine(t)`, the project's `eval` Scheme-runner helper returning a value whose `.SchemeString()` is asserted with `strings.Contains(out, ...)`. `render-category` produces a `LABEL (N)` header + one `render-finding` line per finding.
- **Testdata convention**: `examples/goast-query/testdata/<name>/<name>.go`, module `github.com/aalpar/wile-goast`.

## File Structure

- **Create** `examples/goast-query/testdata/recvasym/recvasym.go` — calibration corpus (one method per category).
- **Modify** `lib/wile/goast/belief-checkers.scm` — add `receiver-parameter-asymmetry` checker (after `co-mutated`, before `checked-before-use`).
- **Modify** `lib/wile/goast/belief.sld:39` — export `receiver-parameter-asymmetry`.
- **Create** `goast/receiver_asymmetry_test.go` — evidence/classification test.
- **Modify** `CLAUDE.md` — Property Checkers table row + Cross-Layer note.

---

## Task 1: Calibration testdata package

**Files:**
- Create: `examples/goast-query/testdata/recvasym/recvasym.go`

- [ ] **Step 1: Write the testdata Go file**

```go
// Package recvasym is calibration data for the receiver-parameter-asymmetry
// belief checker. Each method exercises one classification category.
package recvasym

import "fmt"

type Server struct {
	name string
	host string
}

// candidate: reads s.name exactly once, writes no field, has one parameter.
func (s *Server) formatError(e error) string {
	return fmt.Sprintf("%s: %v", s.name, e)
}

// accessor: zero non-receiver parameters.
func (s *Server) Name() string {
	return s.name
}

// mutation: writes a receiver field — the receiver is state-bearing.
func (s *Server) SetHost(h string) {
	s.host = h
}

// multi-read: reads two distinct receiver fields.
func (s *Server) describe(x int) string {
	return fmt.Sprintf("%s/%s/%d", s.name, s.host, x)
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./examples/goast-query/testdata/recvasym/`
Expected: no output, exit 0.

- [ ] **Step 3: Commit**

```bash
git add examples/goast-query/testdata/recvasym/recvasym.go
git commit -m "test(belief): recvasym calibration testdata for receiver asymmetry"
```

---

## Task 2: L1 checker — mechanical rule + located finding

**Files:**
- Modify: `lib/wile/goast/belief-checkers.scm` (insert after `co-mutated`, ends at line 266)
- Modify: `lib/wile/goast/belief.sld:39`
- Test: `goast/receiver_asymmetry_test.go`

- [ ] **Step 1: Write the failing test**

Create `goast/receiver_asymmetry_test.go` (`package goast_test`; imports `strings`, `testing`, quicktest). Define `TestReceiverAsymmetryL1`: build `newBeliefEngine(t)`, run the project Scheme-test helper on this body, capture `.SchemeString()` as `out`:

```scheme
(import (wile goast belief))
(import (wile goast provenance))
(reset-beliefs!)
(define-belief "recv-asym"
  (sites (methods-of "Server"))
  (expect (receiver-parameter-asymmetry))
  (threshold 0 1))
(define res (car (run-beliefs "github.com/aalpar/wile-goast/examples/goast-query/testdata/recvasym")))
(render-category "recv-asym" (cdr (assoc 'findings res)))
```

Assertions (use `c.Assert(strings.Contains(out, ...), qt.IsTrue, qt.Commentf("%s", out))`):
- subtest "all four categories present": `out` contains each of `"candidate"`, `"mutation"`, `"accessor"`, `"multi-read"`.
- subtest "candidate located with field+receiver why": `out` contains `"recvasym.go"`, `"receiver-asymmetry"`, and `"name"`.

Mirror the exact harness in `goast/belief_checker_evidence_test.go` (the project's `eval` Scheme-runner helper + `.SchemeString()`); copy the Apache license header from that file.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestReceiverAsymmetryL1 -v`
Expected: FAIL — the Scheme errors with an unbound identifier `receiver-parameter-asymmetry`.

- [ ] **Step 3: Implement the checker**

In `lib/wile/goast/belief-checkers.scm`, insert immediately after the `co-mutated` definition (after line 266, before the `checked-before-use` comment block at line 268):

```scheme
;; (receiver-parameter-asymmetry) — flags methods whose receiver is read
;; exactly once, written never, with at least one non-receiver parameter:
;; the "receiver as namespace" anti-pattern (Connascence of Meaning hidden
;; by method syntax). The single receiver read is a convert-to-function
;; signal. See plans/2026-04-20-receiver-parameter-asymmetry-design.md.
;; Receiver field reads are ssa-field-addr/ssa-field whose x is the receiver
;; param; a receiver ssa-field-addr whose register is some ssa-store's addr
;; is a write, not a read. Returns one of:
;;   'candidate    — read set singleton, write set empty, >=1 param (the flag)
;;   'mutation     — a receiver field is written (receiver is state-bearing)
;;   'accessor     — zero non-receiver parameters
;;   'multi-read   — more than one distinct receiver field read
;;   'unused-recv  — receiver never read (pure namespace / dispatch)
;;   'no-receiver  — not a method, or SSA receiver not resolvable
;;   'missing      — SSA lookup failed
;; The 'candidate verdict carries located evidence (the receiver read);
;; other verdicts stay bare (located only where it matters, as paired-with).
(define (receiver-parameter-asymmetry)
  "Property checker: flag receiver-as-namespace methods.\nReturns 'candidate (read 1 field, write none, has params), 'mutation,\n'accessor, 'multi-read, 'unused-recv, 'no-receiver, or 'missing.\nThe 'candidate verdict carries the located receiver read as evidence.\n\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (receiver-parameter-asymmetry)\n\nSee also: `co-mutated', `stores-to-fields'."
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if (not ssa-fn) 'missing
        (let* ((params (nf ssa-fn 'params))
               (recv (and (pair? params) (car params)))
               (recv-name (and recv (nf recv 'name)))
               (nparam (if (pair? params) (- (length params) 1) 0)))
          (if (not recv-name) 'no-receiver
            (let* ((instrs (ssa-all-instrs ssa-fn))
                   (store-addrs
                     (filter-map (lambda (i) (and (tag? i 'ssa-store) (nf i 'addr)))
                                 instrs))
                   ;; receiver field-addr instrs as (field . register)
                   (recv-faddr
                     (filter-map
                       (lambda (i)
                         (and (tag? i 'ssa-field-addr)
                              (equal? (nf i 'x) recv-name)
                              (cons (nf i 'field) (nf i 'name))))
                       instrs))
                   ;; value-receiver direct field reads (field names)
                   (recv-field
                     (filter-map
                       (lambda (i)
                         (and (tag? i 'ssa-field)
                              (equal? (nf i 'x) recv-name)
                              (nf i 'field)))
                       instrs))
                   (write-fields
                     (unique (filter-map
                               (lambda (fa) (and (member? (cdr fa) store-addrs) (car fa)))
                               recv-faddr)))
                   (read-fields
                     (unique (append
                               (filter-map
                                 (lambda (fa) (and (not (member? (cdr fa) store-addrs)) (car fa)))
                                 recv-faddr)
                               recv-field))))
              (cond
                ((positive? (length write-fields)) 'mutation)
                ((= nparam 0) 'accessor)
                ((= (length read-fields) 0) 'unused-recv)
                ((> (length read-fields) 1) 'multi-read)
                (else
                  (let* ((field (car read-fields))
                         (pos (ssa-first-pos ssa-fn
                                (lambda (i)
                                  (and (or (tag? i 'ssa-field-addr) (tag? i 'ssa-field))
                                       (equal? (nf i 'x) recv-name)
                                       (equal? (nf i 'field) field))))))
                    (if (not pos) 'candidate
                      (cons 'candidate
                            (list (cons 'where pos)
                                  (cons 'why (list 'receiver-asymmetry
                                                   (cons 'field field)
                                                   (cons 'receiver recv-name)
                                                   (cons 'relation 'candidate)))
                                  (cons 'score #f))))))))))))))
```

- [ ] **Step 4: Export the checker**

In `lib/wile/goast/belief.sld`, change the Property-checkers export line (line 39) from `paired-with ordered co-mutated` to `paired-with ordered co-mutated receiver-parameter-asymmetry`.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./goast/ -run TestReceiverAsymmetryL1 -v`
Expected: PASS (both subtests).

- [ ] **Step 6: Run the full belief suite (no regressions)**

Run: `go test ./goast/ -run 'Belief|Receiver|Checker' -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add lib/wile/goast/belief-checkers.scm lib/wile/goast/belief.sld goast/receiver_asymmetry_test.go
git commit -m "feat(belief): receiver-parameter-asymmetry checker (L1 + located finding)"
```

---

## Task 3: Interface-member exclusion (not-IM)

Interface-satisfying methods are the dominant L1 false positive (the method form is *forced* by the interface, not a smell). A fully precise check needs per-interface implementor resolution; for this slice use a conservative AST proxy: collect the method-name set of every `interface` type declared in the loaded packages, and reclassify a `candidate` whose short name is in that set as `'interface-method`. Over-excludes (a method named `Get` is spared even if its interface is single-impl) — conservative, documented.

**Files:**
- Modify: `lib/wile/goast/belief-checkers.scm` (the checker + a small helper)
- Modify: `examples/goast-query/testdata/recvasym/recvasym.go`
- Test: `goast/receiver_asymmetry_test.go`

- [ ] **Step 1: Add testdata + failing test**

Append to `recvasym.go`:

```go
// Stringer is a single-method interface; its members are excluded.
type Stringer interface {
	Render(prefix string) string
}

type Tag struct {
	label string
}

// interface-method: Render satisfies Stringer. Without the exclusion it
// would read as a candidate (one field read, one param, joint use); the
// interface-member filter reclassifies it.
func (t *Tag) Render(prefix string) string {
	return fmt.Sprintf("%s<%s>", prefix, t.label)
}
```

Add `TestReceiverAsymmetryInterfaceExcluded`: belief over `(methods-of "Tag")`, assert `(cdr (assoc 'deviations res))` rendered string contains `"interface-method"` and does NOT contain `"candidate"`.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./goast/ -run TestReceiverAsymmetryInterfaceExcluded -v`
Expected: FAIL — `Render` currently classifies as `candidate`.

- [ ] **Step 3: Implement the exclusion**

Add a helper above `receiver-parameter-asymmetry`:

```scheme
;; Collect the set of method names declared by any interface type in the
;; loaded packages. Conservative not-IM proxy for receiver-parameter-asymmetry.
(define (interface-method-names ctx)
  (let ((acc '()))
    (for-each
      (lambda (pkg)
        (walk pkg
          (lambda (node)
            (when (and (tag? node 'interface-type) (nf node 'methods))
              (for-each
                (lambda (m)
                  (let ((nm (nf m 'name)))
                    (when nm (set! acc (cons nm acc)))))
                (nf node 'methods))))))
      (ctx-pkgs ctx))
    (unique acc)))
```

> **NOTE for implementer:** confirm the AST tag/field before coding: `grep -n "interface-type\|InterfaceType\|methods" goast/mapper.go`. Adjust `'interface-type`/`'methods`/`'name` to the actual tags the mapper emits. Confirm `walk`, `when`, `for-each`, `set!` are available (utils + base); if `when` is missing use `(if c (begin ...))`. `walk`'s callback arity must match its definition at `utils.scm:56` — verify it is `(walk node proc)` invoking `proc` on each node.

In the checker, compute `(define short (ssa-short-name fname))` near the top, and in the `candidate` branch, before emitting, guard: `(if (member? short (interface-method-names ctx)) 'interface-method <candidate-finding>)`.

- [ ] **Step 4: Run both receiver tests + rebuild testdata**

Run: `go build ./examples/goast-query/testdata/recvasym/ && go test ./goast/ -run TestReceiverAsymmetry -v`
Expected: PASS (L1 unchanged; interface-excluded passes).

- [ ] **Step 5: Commit**

```bash
git add lib/wile/goast/belief-checkers.scm goast/receiver_asymmetry_test.go examples/goast-query/testdata/recvasym/recvasym.go
git commit -m "feat(belief): exclude interface members from receiver-asymmetry candidates"
```

---

## Task 4: L2 joint-use refinement (candidate vs forwarder)

L1 flags both *subject inversion* (receiver field used inside an operation whose other inputs are parameters — the real smell) and *forwarders* (`c.store.Get(k)` — receiver field is the subject being delegated to). Split them: keep `candidate` only if the single receiver-read value and >=1 non-receiver parameter flow into the **same** instruction (`UC_mechanical`); otherwise `'forwarder`.

**Files:**
- Modify: `lib/wile/goast/belief-checkers.scm`
- Modify: `examples/goast-query/testdata/recvasym/recvasym.go`
- Test: `goast/receiver_asymmetry_test.go`

- [ ] **Step 1: Add forwarder testdata + failing test**

Append to `recvasym.go`:

```go
type inner struct{ data map[string]string }

func (i *inner) Get(k string) string { return i.data[k] }

type Cache struct {
	store *inner
}

// forwarder: c.store is read once and is itself the subject of the call;
// the parameter k is its argument, not jointly used with the receiver read.
func (c *Cache) Get(k string) string {
	return c.store.Get(k)
}
```

Add `TestReceiverAsymmetryForwarder`: belief over `(methods-of "Cache")`, assert deviations contain `"forwarder"` and NOT `"candidate"`. Also re-assert `TestReceiverAsymmetryL1` still shows `formatError` as `candidate` (joint use of `s.name` and `e` inside `Sprintf`).

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./goast/ -run TestReceiverAsymmetryForwarder -v`
Expected: FAIL — `Cache.Get` is currently `candidate`.

- [ ] **Step 3: Implement joint-use**

In the `candidate` branch, before emitting, compute the receiver-read value register `rv` and test joint use against non-receiver param names. `rv`: for a value-receiver read it is the `ssa-field` register; for a pointer-receiver read it is the `ssa-unop` (op `*`) whose `x` is the field-addr register. `pnames` = `(map (lambda (p) (nf p 'name)) (cdr params))`. `joint?` = some instruction's `(nf i 'operands)` list contains `rv` and at least one name in `pnames`.

```scheme
(let* ((fa (let loop ((xs recv-faddr))
             (cond ((null? xs) #f)
                   ((equal? (car (car xs)) field) (cdr (car xs)))
                   (else (loop (cdr xs))))))
       (rv (or
             (let loop ((is instrs))
               (cond ((null? is) #f)
                     ((and (tag? (car is) 'ssa-field)
                           (equal? (nf (car is) 'x) recv-name)
                           (equal? (nf (car is) 'field) field))
                      (nf (car is) 'name))
                     (else (loop (cdr is)))))
             (and fa
                  (let loop ((is instrs))
                    (cond ((null? is) #f)
                          ((and (tag? (car is) 'ssa-unop)
                                (equal? (nf (car is) 'x) fa))
                           (nf (car is) 'name))
                          (else (loop (cdr is))))))))
       (pnames (map (lambda (p) (nf p 'name)) (cdr params)))
       (joint?
         (and rv
              (let loop ((is instrs))
                (cond ((null? is) #f)
                      ((let ((ops (nf (car is) 'operands)))
                         (and (list? ops)
                              (member? rv ops)
                              (let pl ((ps pnames))
                                (cond ((null? ps) #f)
                                      ((member? (car ps) ops) #t)
                                      (else (pl (cdr ps)))))))
                       #t)
                      (else (loop (cdr is))))))))
  (if (not joint?) 'forwarder
    (if (not pos) 'candidate
      (cons 'candidate
            (list (cons 'where pos)
                  (cons 'why (list 'receiver-asymmetry
                                   (cons 'field field)
                                   (cons 'receiver recv-name)
                                   (cons 'relation 'candidate)))
                  (cons 'score #f))))))
```

> **NOTE for implementer:** `map`/`list?` are R7RS base. If `formatError`'s `e` is wrapped (interface conversion / varargs packing) before the `Sprintf` instruction, direct operand matching may miss it. If `TestReceiverAsymmetryL1` regresses (formatError -> forwarder), relax the join to per-block: iterate `(block-instrs b)` for each block in `(nf ssa-fn 'blocks)` and accept if `rv` and any `pname` both appear as operands within one block. This is the design's acknowledged `UC_mechanical` proxy, not exact dataflow.

- [ ] **Step 4: Run the full receiver suite**

Run: `go test ./goast/ -run TestReceiverAsymmetry -v`
Expected: PASS — L1 (`formatError` still candidate), interface-excluded, forwarder.

- [ ] **Step 5: Commit**

```bash
git add lib/wile/goast/belief-checkers.scm goast/receiver_asymmetry_test.go examples/goast-query/testdata/recvasym/recvasym.go
git commit -m "feat(belief): L2 joint-use split (candidate vs forwarder) for receiver asymmetry"
```

---

## Task 5: Documentation + dogfood

**Files:**
- Modify: `CLAUDE.md` (Property Checkers table; Cross-Layer Notes / provenance prose)

- [ ] **Step 1: Add the checker to the Property Checkers table**

In `CLAUDE.md` `### Property Checkers` table, add:

```
| `(receiver-parameter-asymmetry)` | SSA | `'candidate` / `'forwarder` / `'mutation` / `'accessor` / `'multi-read` / `'unused-recv` |
```

In the prose paragraph listing which checkers emit located evidence, add `receiver-parameter-asymmetry` (the `candidate` verdict locates the receiver read; non-candidate verdicts stay bare).

- [ ] **Step 2: Dogfood on wile-goast itself (exploratory, not asserted)**

`make build`, then run a script that defines the belief over receiver-bearing functions in `github.com/aalpar/wile-goast/goast` and prints `render-category` of the findings, filtering for `candidate`. Capture the candidate count in the commit message and hand-triage 2-3 to confirm they read as real convert-to-function opportunities.

> **NOTE:** per memory `mcp-eval-stale-binary`, do NOT use the MCP `eval` tool here — use the freshly `make build`'d `./dist/*/*/wile-goast` CLI. If `(has-receiver "")` does not match-all, wrap site selection in a `custom` predicate testing `(nf site 'recv)` truthy, or enumerate concrete receiver type names.

- [ ] **Step 3: Build + full test**

Run: `make build && make test`
Expected: build succeeds; tests PASS.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document receiver-parameter-asymmetry checker"
```

---

## Task 6 (optional): catalog refactoring entry

Closes the loop with the detector-completion catalog: the refactoring this detector *feeds* has no catalog file.

**Files:**
- Create: `refactorings/convert-method-to-function.md`

- [ ] **Step 1:** Write `convert-method-to-function.md` in the verified-Go format of `refactorings/normalize-divergent-sites.md` (Name/inverse -> Precondition -> numbered Transform -> optimizes/sacrifices -> **Detector status:** could-build, backing `receiver-parameter-asymmetry`). Use the design's `wrapMidParseEOF` before/after as the verified Go pair.
- [ ] **Step 2:** Verify the before/after Go pair builds + vets + gofmt-clean in a throwaway module (`/tmp/refverify3`), as in catalog Passes 1-2.
- [ ] **Step 3:** Commit `docs(refactorings): convert-method-to-function (feeds receiver-asymmetry)`.

---

## Self-Review

**Spec coverage** (against `2026-04-20-...-design.md`):
- L1 mechanical rule (RR singleton, RW empty, >=1 param) -> Task 2. DONE.
- not-IM interface-member exclusion -> Task 3 (conservative AST proxy; precise per-implementor version deferred, documented). DONE.
- not-MV method-value exclusion -> **not implemented.** Rare, lower-value; deliberate gap (findings are advisory, not auto-rewrites). Add as follow-up if dogfood shows method-value false positives.
- L2 joint-use (`UC_mechanical`) -> Task 4. DONE.
- Severity heuristics (high/medium/low) -> **not implemented** this slice; `why` carries `field`+`receiver` so severity is derivable later. Noted gap.
- Located findings (belief output format) -> Tasks 2/4 emit `(candidate . evidence)`. DONE.
- Dogfood on wile-goast -> Task 5. DONE. (Running on `wile` itself is cross-repo; out of scope here.)
- Calibration corpus (20+/20-) -> aspirational design target; this slice ships a corpus covering every category. Expanding is a follow-up, noted.

**Placeholder scan:** the three `NOTE for implementer` blocks are genuine verification gates (AST tag confirmation, has-receiver match-all behavior, varargs operand visibility), each naming the exact probe and fallback — not hidden TODOs.

**Type/name consistency:** checker name `receiver-parameter-asymmetry` (no args) consistent across belief.sld export, checker def, and all test files. Category symbols (`candidate`/`mutation`/`accessor`/`multi-read`/`unused-recv`/`forwarder`/`interface-method`/`missing`/`no-receiver`) consistent between checker and test assertions. `why` tag `receiver-asymmetry` consistent between checker and Task 5 docs.

## Execution Handoff

Two execution options after approval:
1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks.
2. **Inline Execution** — execute in this session with checkpoints.
