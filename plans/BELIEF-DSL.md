# Belief DSL — Remaining Work

> **Incomplete items:**
> 1. ~~**Discovery `--emit` mode**~~ -- DONE (v0.5.112)
> 2. **Suppression** -- diff discovery output against committed belief files to suppress known findings (§ below)

**Current state**: Implemented as `(wile goast belief)`. Core form (`define-belief`, `run-beliefs`), all site selectors, all property checkers, multi-package support, `go-ssa-field-index` performance optimization, aggregate beliefs, structured return values — all working.

**Reference**: `cmd/wile-goast/lib/wile/goast/belief.sld`, `cmd/wile-goast/lib/wile/goast/belief.scm`

---

## Discovery `--emit` Mode

Discovery scripts should gain an `emit-beliefs` procedure that outputs `define-belief`
forms instead of human-readable reports. This closes the discover → review → commit
lifecycle (distinct from C6 "belief graduation" which promotes beliefs to dataflow assertions):

```
discover → review → commit → enforce
   │                  │         │
   │  human judgment  │  CI     │
   ▼                  ▼         ▼
 candidates       belief file  run-beliefs
 (stdout)         (.scm)       (exit code)
```

### Problem: Expression Recovery

Currently beliefs store compiled lambdas:

```scheme
;; belief.scm internal representation (5-element list)
(name sites-fn expect-fn min-adherence min-sites)
```

To emit `define-belief` source code, the original selector and checker **expressions**
must survive compilation. A lambda can't be decompiled.

### Design: Expression Metadata

Extend belief storage to 7 elements:

```scheme
(name sites-fn expect-fn min-adherence min-sites sites-expr expect-expr)
```

The `define-belief` macro already receives the literal `(sites ...)` and `(expect ...)`
forms. It just needs to **quote** them alongside the compiled functions.

Change in `define-belief` macro (belief.scm, line 74):

```scheme
;; Before (line 76-77):
((_ name (sites selector) (expect checker) (threshold min-adh min-n))
 (register-belief! name selector checker min-adh min-n))

;; After:
((_ name (sites selector) (expect checker) (threshold min-adh min-n))
 (register-belief! name selector checker min-adh min-n
                   '(sites selector)
                   '(expect checker)))
```

New accessors:

```scheme
(define (belief-sites-expr b) (list-ref b 5))
(define (belief-expect-expr b) (list-ref b 6))
```

Attach expression metadata to `run-beliefs` result alists:

```scheme
;; Each result alist gains two keys:
((name . "lock-unlock") (type . per-site) (status . strong)
 (pattern . paired-defer) (ratio . 9/10) (total . 10)
 (adherence . (...)) (deviations . (...))
 (sites-expr . (functions-matching (contains-call "Lock")))
 (expect-expr . (paired-with "Lock" "Unlock")))
```

### `emit-beliefs` API

```scheme
(emit-beliefs results)
;; results: return value of run-beliefs (list of result alists with expression metadata)
;; Returns: string of Scheme source code
```

For each result with `status = strong`:
1. Emit a comment header: belief name, adherence ratio, pattern, deviation list
2. Emit a `define-belief` form using stored expressions
3. Threshold in emitted form: use the belief's original `min-adherence` and `min-sites`

For `status = weak`, `no-sites`, or `error`: skip (not candidates).

For `type = aggregate`: skip (aggregate beliefs have different semantics; they don't
produce per-site enforcement).

### Output Format

```scheme
;; lock-unlock-pairing
;; Adherence: 90% (9/10), Pattern: paired-defer
;; Deviations: pkg.Baz (unpaired)
;;
(define-belief "lock-unlock-pairing"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.90 5))
```

### Edge Cases

**Custom checkers.** `(expect (custom (lambda (site ctx) ...)))` — the quoted lambda
form is syntactically valid Scheme, so it emits correctly. But the lambda captures nothing
from the discovery context, so the emitted form is self-contained. This works.

**Bootstrapped selectors.** `(sites (sites-from "other-belief" 'deviation))` — the
emitted form preserves this reference. If the upstream belief isn't defined in the
enforcement context, `run-beliefs` will fail at runtime with an informative error
(belief not found). This is correct — the human reviewer should resolve the dependency
or replace with a concrete selector.

**`contains-call` dual use.** `contains-call` appears both as a selector predicate
and as a property checker. The emitted expression distinguishes them by position:
`(sites (functions-matching (contains-call "X")))` vs `(expect (contains-call "X"))`.

### Changes Required

| File | Change |
|------|--------|
| `belief.scm` line 74 | `define-belief` macro: quote `sites` and `expect` expressions, pass to `register-belief!` |
| `belief.scm` line 47 | `register-belief!`: accept 7 args, store 7-element tuple |
| `belief.scm` (accessors) | Add `belief-sites-expr`, `belief-expect-expr` |
| `belief.scm` (evaluate-belief) | Attach `sites-expr` and `expect-expr` keys to result alists |
| `belief.scm` (new) | Add `emit-beliefs` procedure |
| `belief.sld` | Export `emit-beliefs` |

---

## Suppression

**Shipped 2026-04-23.** See `plans/2026-04-17-belief-suppression-impl.md`.

Discovery runs diff output against committed belief files. A belief whose
selector and checker expressions structurally match an existing committed
belief is suppressed — discovery only reports *new* findings.

### Structural Matching

Two beliefs structurally match when their selector and checker expressions are
`equal?` after quoting:

```scheme
(define (belief-expressions-match? a b)
  (and (equal? (belief-sites-expr a) (belief-sites-expr b))
       (equal? (belief-expect-expr a) (belief-expect-expr b))))
```

Names and thresholds are **ignored** for matching. Rationale:
- **Names:** A human might rename "lock-unlock" to "mutex-pairing" — same belief.
- **Thresholds:** A committed threshold of 0.80 and a discovery finding of 0.90 are
  the same pattern at different confidence. Suppressing prevents duplicating work.

Why structural, not semantic: Two selectors might select the same sites but be expressed
differently (`(contains-call "Lock")` vs `(any-of (contains-call "Lock") (contains-call "RLock"))`).
Structural matching is conservative — it only suppresses exact expression duplicates,
avoiding false suppression. Semantic equivalence would require evaluating both selectors
against the same package, which is the full analysis cost.

### `suppress-known` API

```scheme
(suppress-known results committed-beliefs)
;; results:    list of result alists from run-beliefs (with expression metadata)
;; committed:  list of belief tuples from the registry after loading committed files
;; Returns:    filtered results — only findings not matching any committed belief
```

Implementation: for each result, check if any committed belief has matching
expressions. If so, remove from output.

### Loading Committed Beliefs

```scheme
(load-committed-beliefs path)
;; path: directory path or single .scm file
;; Returns: pair (per-site-snapshot . aggregate-snapshot), where each
;;          snapshot is the list of belief tuples registered while the
;;          file(s) loaded. Per-file load failures are logged to stderr
;;          and skipped — partial suppression is better than none.
```

The loading sequence:

1. Save current `*beliefs*` state
2. Call `(reset-beliefs!)`
3. Load and evaluate the committed file(s) — each `define-belief` form populates the registry
4. Snapshot the registry → `committed-beliefs`
5. Restore original `*beliefs*` state
6. Return snapshot

Alternative: `(with-belief-scope thunk)` that saves/restores `*beliefs*` around
execution. More composable, and `load-committed-beliefs` becomes a thin wrapper.

```scheme
(define (with-belief-scope thunk)
  (let ((saved *beliefs*))
    (dynamic-wind
      (lambda () (reset-beliefs!))
      thunk
      (lambda () (set! *beliefs* saved)))))
```

### CLI Integration

**Deferred.** Users compose `with-belief-scope`, `load-committed-beliefs`,
`suppress-known`, and `emit-beliefs` in discovery scripts. Matches how
`emit-beliefs` shipped (no dedicated flag). See
`plans/2026-04-17-belief-suppression-design.md` §Non-Goals for rationale.

Typical script shape:

```scheme
(import (wile goast belief))
(define results
  (with-belief-scope
    (lambda ()
      ;; ...discovery beliefs...
      (run-beliefs "my/pkg/..."))))
(define committed (load-committed-beliefs "beliefs/"))
(display (emit-beliefs (suppress-known results committed)))
```

### Edge Cases

**Threshold-only changes.** A committed belief at threshold 0.80 and a discovery
result at 0.90 for the same pattern: suppressed. The human already committed this
belief. If they want to update the threshold, they edit the committed file directly.

**Renamed beliefs.** Match ignores names → renaming doesn't break suppression.

**Stale committed beliefs.** A committed belief whose selector no longer matches any
sites: irrelevant for suppression (it won't appear in discovery results). Does NOT
need cleanup — `run-beliefs` already reports `status = no-sites` for these.

**File evaluation errors.** If a committed `.scm` file fails to evaluate (e.g., uses
an import not available in the wile-goast binary): skip with a warning to stderr.
Don't abort — partial suppression is better than none.

### Changes Required

| File | Change |
|------|--------|
| `belief.scm` (new) | Add `with-belief-scope`, `load-committed-beliefs`, `suppress-known` |
| `belief.sld` | Export `with-belief-scope`, `load-committed-beliefs`, `suppress-known` |
| `cmd/wile-goast/main.go` | Add `--emit` and `--suppress` CLI flags |

---

## Limitations Worth Addressing

- **No severity ranking** — a deviation in a critical path matters more than in a debug utility. The tool reports deviations uniformly.
- **AST-level call detection** — `contains-call` and `paired-with` miss indirect calls (method values, interfaces, closures). Acceptable for convention detection but limits coverage.
- **The majority assumption** — if the majority behavior is wrong, deviations are the correct code. The tool detects inconsistency, not incorrectness.
