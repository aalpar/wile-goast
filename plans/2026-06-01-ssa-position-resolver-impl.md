# SSA Position Resolver Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn an SSA instruction (and a `(block, func-name)` call) into a resolved `"file:line:col"` source position, so belief checkers, FCA, and unification can stop discarding the provenance they already compute.

**Architecture:** The Go SSA mapper *already* injects a `(pos . "file:line:col")` field per instruction when `go-ssa-build` is called with the `'positions` option (`goastssa/mapper.go:103-118`), omitting it for positionless instructions. So no Go change is required. This plan adds a small Scheme library `(wile goast provenance)` with two pure accessors that read those positions, and flips the belief context's SSA build to request `'positions` so the field is present. The resolver is deliberately *not* placed in `belief-checkers.scm` — it lives in a shared module because FCA and unification will reuse it (this resolves design open-question #3 toward the shared-module option).

**Tech Stack:** Wile (R7RS Scheme), `(wile goast utils)` for `nf`/`tag?`, Go test harness (`newBeliefEngine` + the Scheme-`eval` test helper, quicktest), `golang.org/x/tools/go/ssa` (already wired).

**Spec:** [`2026-06-01-auditable-categorization-design.md`](2026-06-01-auditable-categorization-design.md) — the position resolver is the one genuinely-new build named there. This plan implements only that first slice; extending the *checker contract* to carry the evidence tail is a separate, later plan.

**Scope boundary (YAGNI):** This plan delivers the resolver and the positions-enabled SSA it reads. It does **not** modify the checker contract, `evaluate-belief`, FCA reports, or add a renderer. Those consume the resolver later.

---

## File Structure

| File | Responsibility | Action |
|------|----------------|--------|
| `lib/wile/goast/provenance.scm` | The resolver: `ssa-instr-pos`, `ssa-call-position` (+ internal `ssa-call-to?`) | Create |
| `lib/wile/goast/provenance.sld` | R7RS library `(wile goast provenance)`, imports `(wile goast utils)` | Create |
| `lib/wile/goast/belief.scm:144` | `ctx-ssa` builds SSA with `'positions` so instructions carry `pos` | Modify |
| `goast/provenance_integration_test.go` | Tests (package `goast_test`, reuses `newBeliefEngine` and the `eval` helper) | Create |
| `CLAUDE.md` | Document the new library in the layer/library tables | Modify |

No `embed.go` change: `//go:embed lib` (root `embed.go:23`) globs the directory; tests load `lib/` from disk via `os.DirFS("..")`.

---

## Task 1: Create `(wile goast provenance)` with `ssa-instr-pos`

`ssa-instr-pos` reads the `pos` field off any SSA instruction node, returning the `"file:line:col"` string or `#f`. The `(if (string? p) p #f)` guard makes it robust whether `nf` returns `#f` or `#!void` for an absent field.

**Files:**
- Create: `lib/wile/goast/provenance.scm`
- Create: `lib/wile/goast/provenance.sld`
- Test: `goast/provenance_integration_test.go`

- [ ] **Step 1: Write the failing test**

Create `goast/provenance_integration_test.go`:

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
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestProvenanceInstrPos(t *testing.T) {
	engine := newBeliefEngine(t)

	// ssa-instr-pos returns the pos string when present, #f when absent.
	result := eval(t, engine, `
		(import (wile goast provenance))
		(and (equal? (ssa-instr-pos '(ssa-call (pos . "foo.go:10:3") (func . "Lock")))
		             "foo.go:10:3")
		     (eq? (ssa-instr-pos '(ssa-call (func . "Lock"))) #f))
	`)
	qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run TestProvenanceInstrPos -v`
Expected: FAIL — `(import (wile goast provenance))` errors (library not found).

- [ ] **Step 3: Write the library files**

Create `lib/wile/goast/provenance.scm`:

```scheme
;; Copyright 2026 Aaron Alpar
;;
;; Licensed under the Apache License, Version 2.0 (the "License");
;; you may not use this file except in compliance with the License.
;; You may obtain a copy of the License at
;;
;;     http://www.apache.org/licenses/LICENSE-2.0
;;
;; Unless required by applicable law or agreed to in writing, software
;; distributed under the License is distributed on an "AS IS" BASIS,
;; WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
;; See the License for the specific language governing permissions and
;; limitations under the License.

;; Provenance: resolve SSA instructions to source positions.
;;
;; The SSA mapper injects a (pos . "file:line:col") field per instruction when
;; go-ssa-build is called with the 'positions option (goastssa/mapper.go). The
;; field is omitted for synthetic/positionless instructions. These accessors
;; surface that position so analyses stop discarding the provenance they hold.

;; ssa-instr-pos: the resolved source position of an SSA instruction node, or
;; #f when the instruction carries no position. The string? guard normalizes a
;; missing field (nf may return #f or #!void) to #f.
(define (ssa-instr-pos instr)
  (let ((p (nf instr 'pos)))
    (if (string? p) p #f)))
```

Create `lib/wile/goast/provenance.sld`:

```scheme
;; Copyright 2026 Aaron Alpar
;;
;; Licensed under the Apache License, Version 2.0 (the "License");
;; you may not use this file except in compliance with the License.
;; You may obtain a copy of the License at
;;
;;     http://www.apache.org/licenses/LICENSE-2.0
;;
;; Unless required by applicable law or agreed to in writing, software
;; distributed under the License is distributed on an "AS IS" BASIS,
;; WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
;; See the License for the specific language governing permissions and
;; limitations under the License.

(define-library (wile goast provenance)
  (export ssa-instr-pos)
  (import (wile goast utils))
  (include "provenance.scm"))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./goast/ -run TestProvenanceInstrPos -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lib/wile/goast/provenance.scm lib/wile/goast/provenance.sld goast/provenance_integration_test.go
git commit -m "feat(provenance): add ssa-instr-pos resolver"
```

(Note: a pre-commit hook auto-bumps `VERSION`; let it ride.)

---

## Task 2: Add `ssa-call-position`

`ssa-call-position` finds the first call to `func-name` in a block's instruction list and returns its resolved position (or `#f`). It is the SSA analog of `find-call-position` (`belief-checkers.scm:149`), returning a *location* instead of a block-relative index — matching call detection across `ssa-call`/`ssa-go`/`ssa-defer`, static (`func`) and method (`method`) calls.

**Files:**
- Modify: `lib/wile/goast/provenance.scm`
- Modify: `lib/wile/goast/provenance.sld` (export `ssa-call-position`)
- Test: `goast/provenance_integration_test.go`

- [ ] **Step 1: Write the failing test**

Add to `goast/provenance_integration_test.go`:

```go
func TestProvenanceCallPosition(t *testing.T) {
	engine := newBeliefEngine(t)

	// ssa-call-position finds the first matching call's position, #f otherwise.
	// A literal block with one non-call instr and one positioned call.
	result := eval(t, engine, `
		(import (wile goast provenance))
		(let ((block '(ssa-block (index . 0)
		                (instrs (ssa-binop (name . "t0"))
		                        (ssa-call (pos . "bar.go:7:5") (func . "Unlock"))))))
		  (and (equal? (ssa-call-position block "Unlock") "bar.go:7:5")
		       (eq? (ssa-call-position block "Nope") #f)))
	`)
	qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run TestProvenanceCallPosition -v`
Expected: FAIL — `ssa-call-position` unbound.

- [ ] **Step 3: Add the implementation**

Append to `lib/wile/goast/provenance.scm`:

```scheme
;; ssa-call-to?: does NODE call FUNC-NAME? Matches static calls (func field)
;; and method calls (method field) across ssa-call/ssa-go/ssa-defer. Mirrors
;; the call-matching predicate in find-call-position (belief-checkers.scm).
(define (ssa-call-to? node func-name)
  (and (or (tag? node 'ssa-call) (tag? node 'ssa-go) (tag? node 'ssa-defer))
       (or (equal? (nf node 'func) func-name)
           (equal? (nf node 'method) func-name))))

;; ssa-call-position: resolved source position of the first call to FUNC-NAME
;; in BLOCK's instruction list, or #f when the call is absent or positionless.
(define (ssa-call-position block func-name)
  (let loop ((is (or (nf block 'instrs) '())))
    (cond
      ((null? is) #f)
      ((ssa-call-to? (car is) func-name) (ssa-instr-pos (car is)))
      (else (loop (cdr is))))))
```

Update the export line in `lib/wile/goast/provenance.sld`:

```scheme
  (export ssa-instr-pos ssa-call-position)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./goast/ -run TestProvenance -v`
Expected: PASS (both `TestProvenanceInstrPos` and `TestProvenanceCallPosition`)

- [ ] **Step 5: Commit**

```bash
git add lib/wile/goast/provenance.scm lib/wile/goast/provenance.sld goast/provenance_integration_test.go
git commit -m "feat(provenance): add ssa-call-position block resolver"
```

---

## Task 3: Make the belief context build SSA with positions

The resolver is inert until the belief context's SSA actually carries `pos` fields. `ctx-ssa` (`belief.scm:141-146`) currently builds without positions. Flip it on — this makes positions load-bearing for analysis, per the design's stance. Adding a field is order-independent and field-keyed access (`nf`) is unaffected, so existing checkers and tests keep passing.

**Files:**
- Modify: `lib/wile/goast/belief.scm:144`
- Test: `goast/provenance_integration_test.go`

- [ ] **Step 1: Write the failing test**

Add to `goast/provenance_integration_test.go`:

```go
func TestProvenanceContextPositions(t *testing.T) {
	engine := newBeliefEngine(t)

	// After ctx-ssa builds with positions, real instructions carry a
	// resolvable "file:line:col" that ssa-instr-pos surfaces.
	result := eval(t, engine, `
		(import (wile goast utils))
		(import (wile goast provenance))
		(import (wile goast belief))
		(let* ((ctx (make-context "github.com/aalpar/wile-goast/goast"))
		       (fn  (ctx-find-ssa-func ctx
		              "github.com/aalpar/wile-goast/goast"
		              "github.com/aalpar/wile-goast/goast.PrimGoParseFile"))
		       (blocks (and fn (nf fn 'blocks)))
		       (positions (flat-map
		                    (lambda (b)
		                      (filter-map ssa-instr-pos (or (nf b 'instrs) '())))
		                    (or blocks '()))))
		  (and (pair? positions)
		       (string-contains? (car positions) ".go:")))
	`)
	qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run TestProvenanceContextPositions -v`
Expected: FAIL — `positions` is empty (`(pair? positions)` is `#f`) because `ctx-ssa` builds SSA without the `pos` field. Result is `#f`.

- [ ] **Step 3: Add positions to the SSA build**

In `lib/wile/goast/belief.scm`, change the `ctx-ssa` build call (line 144) from:

```scheme
      (let ((ssa (go-ssa-build (ctx-session ctx))))
```

to:

```scheme
      (let ((ssa (go-ssa-build (ctx-session ctx) 'positions)))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./goast/ -run TestProvenanceContextPositions -v`
Expected: PASS

- [ ] **Step 5: Verify no regression in the belief suite**

Run: `go test ./goast/ -run TestBelief -v`
Expected: PASS (all existing belief integration tests — the added `pos` field is field-keyed and order-independent, so `ordered`/`paired-with`/etc. are unaffected).

- [ ] **Step 6: Commit**

```bash
git add lib/wile/goast/belief.scm goast/provenance_integration_test.go
git commit -m "feat(belief): build context SSA with positions for provenance"
```

---

## Task 4: Document the new library

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add the library to the Scheme-library tables**

In `CLAUDE.md`, add a row to the "Key Files" library table (the section listing `lib/wile/goast/*.scm` libraries):

```markdown
| `lib/wile/goast/provenance.scm` | Provenance: resolve SSA instructions to source positions (`ssa-instr-pos`, `ssa-call-position`); first primitive of the auditable-finding facility (embedded in binary) |
```

Add a short section after the boolean-simplify/path-algebra sections:

```markdown
## Provenance — `(wile goast provenance)`

Resolve SSA instructions to source positions. The SSA mapper injects
`(pos . "file:line:col")` per instruction under `go-ssa-build`'s `'positions`
option; the belief context now builds with it on. These accessors surface that
position so analyses stop discarding the provenance they already compute. First
primitive of the auditable-categorization facility
(`plans/2026-06-01-auditable-categorization-design.md`).

| Export | Description |
|--------|-------------|
| `ssa-instr-pos` | Source position `"file:line:col"` of an SSA instruction node, or `#f` |
| `ssa-call-position` | Position of the first call to a named function in a block, or `#f` |
```

- [ ] **Step 2: Run the full test suite as a final check**

Run: `make test`
Expected: PASS (covercheck for `goast/` includes the new library's tests).

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document (wile goast provenance) library"
```

---

## Self-Review

**Spec coverage:** The design names "SSA-instruction-index to file:line resolution" as the one genuinely-new build. Task 1 (`ssa-instr-pos`) + Task 2 (`ssa-call-position`) deliver the resolver; Task 3 supplies the positions-enabled SSA it reads. The checker-contract evidence tail, `evaluate-belief`, FCA/unification adoption, and the renderer are explicitly out of scope (deferred to later plans) — consistent with the user's "position resolver" framing. Open-question #3 (placement) is resolved here toward the shared `(wile goast provenance)` module.

**Placeholder scan:** No TBD/TODO/"add error handling" steps. Every code step shows complete code; every run step gives an exact command and expected result.

**Type consistency:** `ssa-instr-pos` (instr -> string|#f) is reused by `ssa-call-position` and the Task 3 test via `filter-map`. `ssa-call-to?` matches the exact tags/fields of `find-call-position` (`ssa-call`/`ssa-go`/`ssa-defer`; `func`/`method`). The `'positions` symbol matches `prim_ssa.go:56`. Library name `(wile goast provenance)` and exports are identical across `.sld`, tests, and docs.

**Known risk:** If `nf` returns `#!void` (not `#f`) for an absent field, the `(if (string? p) p #f)` guard still yields `#f` — covered by Task 1's absent-field assertion.
