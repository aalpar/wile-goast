# Belief Suppression — Design

> **Status:** Approved design (2026-04-17). Implementation plan forthcoming.
> **Spec source:** Distilled from `plans/BELIEF-DSL.md §Suppression`.
> **Scope:** Scheme procedures only. No CLI flags this round (consistent with
> how `emit-beliefs` shipped — scripts compose the discovery workflow).

## Goal

Close the `discover → review → commit → enforce` lifecycle for the belief DSL.
After a user commits a belief to a `.scm` file, re-running discovery on the same
codebase should not resurface that belief. Only *new* findings should appear in
`emit-beliefs` output.

## Non-Goals

- No CLI flags (`--emit`, `--suppress`). Users write discovery scripts that
  compose `with-belief-scope`, `load-committed-beliefs`, `suppress-known`, and
  `emit-beliefs`. Matches the pattern `emit-beliefs` itself follows.
- No semantic equivalence. Matching is structural (`equal?` on S-expressions).
  Two selectors that select the same sites but are spelled differently are
  treated as different beliefs. Conservative by design — avoids false
  suppression, keeps cost out of the hot path.
- No severity ranking, composition, or cross-package dedup (those are separate
  items under `plans/BELIEF-DSL.md §Limitations`).

## Architecture

Three new procedures in `cmd/wile-goast/lib/wile/goast/belief.scm`, all exported
from `belief.sld`:

```
┌─ discovery.scm (user-written) ──────────────────────────┐
│  (define results                                        │
│    (with-belief-scope                                   │
│      (lambda ()                                         │
│        <...user's discovery beliefs...>                 │
│        (run-beliefs "my/pkg/..."))))                    │
│                                                         │
│  (define committed (load-committed-beliefs "beliefs/")) │
│  (display (emit-beliefs (suppress-known results         │
│                                         committed)))    │
└─────────────────────────────────────────────────────────┘
```

### `with-belief-scope thunk`

Save `*beliefs*` and `*aggregate-beliefs*`, reset both, invoke `thunk`, restore
the saved state via `dynamic-wind`. Returns `thunk`'s return value.

```scheme
(define (with-belief-scope thunk)
  (let ((saved-per-site *beliefs*)
        (saved-aggregate *aggregate-beliefs*))
    (dynamic-wind
      (lambda () (reset-beliefs!))
      thunk
      (lambda ()
        (set! *beliefs* saved-per-site)
        (set! *aggregate-beliefs* saved-aggregate)))))
```

Rationale: the registry is process-global. Loading committed beliefs without
isolation would clobber the user's discovery beliefs (and vice versa).
`dynamic-wind` guarantees restoration even on early exit.

### `load-committed-beliefs path`

Accepts a directory path or a single `.scm` file. For a directory, loads
`*.scm` files at the **top level only** (no recursion). Returns a pair:

```scheme
(cons <per-site-snapshot> <aggregate-snapshot>)
```

where each snapshot is the list of belief tuples exactly as they appear in
`*beliefs*` / `*aggregate-beliefs*` (7-tuples and 5-tuples respectively).

Behavior:
1. Validate `path` exists; raise error if not (caller bug — stderr warning
   inappropriate for a nonexistent root).
2. If `path` is a file, load it directly. If directory, list entries,
   keep those ending in `.scm`, sort for determinism.
3. Execute the loads inside `with-belief-scope` so committed beliefs don't
   leak into the caller's registry.
4. Each file is wrapped in `guard` (R7RS exception handler). On failure, write
   `"wile-goast: skipping <path>: <condition-message>\n"` to
   `(current-error-port)` and continue with remaining files.
5. After all files processed, capture `(cons *beliefs* *aggregate-beliefs*)`
   (inside the scope) as the return value.

### `suppress-known results committed`

Pure function. `results` is the list returned by `run-beliefs`. `committed` is
the pair from `load-committed-beliefs`.

```scheme
(define (suppress-known results committed)
  (let ((per-site (car committed))
        (aggregate (cdr committed)))
    (filter (lambda (r) (not (result-matches-any? r per-site aggregate)))
            results)))
```

Match predicate dispatches on the result's `type` field:
- `'per-site` → match against `per-site` committed by comparing
  `(sites-expr, expect-expr)` via `equal?`.
- `'aggregate` → match against `aggregate` committed by comparing
  `(sites-expr, analyze-expr)` via `equal?`.
- Unknown or missing `type` → pass through unfiltered (conservative).

Names, thresholds, and all other fields are ignored for matching.

## Edge Cases

| Case | Behavior | Rationale |
|------|----------|-----------|
| Renamed committed belief | Suppressed | Match ignores names; spec §Suppression. |
| Threshold-only change between committed (0.80) and discovery (0.90) | Suppressed | Same expression → same pattern. Human edits threshold in committed file. |
| Committed belief whose selector matches no sites now | Not in discovery output, irrelevant | `run-beliefs` reports `status = no-sites`; doesn't appear in findings to suppress. |
| Committed `.scm` file uses unavailable import | Skip + stderr warning | Partial suppression better than none. |
| Nonexistent `path` | Raise error | Caller bug, not data issue. |
| Empty directory | Returns `(cons '() '())` | Degenerate but valid. |
| Result alist without `type` key | Passes through | Don't drop what we can't classify. |
| File loads fine but registers zero beliefs | Silently fine | Just contributes nothing to the snapshot. |

## Testing

Go integration tests in `goast/belief_integration_test.go` using the existing
`eval` / `newBeliefEngine` harness.

1. **`TestWithBeliefScope_Restores`** — define a belief, enter scope, register
   a belief inside, confirm it's present during `thunk`, confirm registry
   returns to pre-scope state after.
2. **`TestWithBeliefScope_RestoresOnEscape`** — trigger an error inside the
   thunk, confirm registry is still restored (validates `dynamic-wind`).
3. **`TestLoadCommittedBeliefs_Directory`** — tempdir with two `.scm` files,
   each registering one per-site belief; confirm snapshot contains both.
4. **`TestLoadCommittedBeliefs_File`** — single `.scm` file path.
5. **`TestLoadCommittedBeliefs_SkipsBadFiles`** — tempdir with one good file
   and one with a syntax error; confirm good one loads, stderr contains
   skip message, return value contains only the good belief.
6. **`TestLoadCommittedBeliefs_NonexistentPath`** — expect error.
7. **`TestSuppressKnown_PerSiteMatch`** — committed belief matches a result,
   result is filtered.
8. **`TestSuppressKnown_RenameIgnored`** — committed and result have same
   `sites-expr` / `expect-expr` but different names, still filtered.
9. **`TestSuppressKnown_ThresholdIgnored`** — committed at threshold 0.80,
   result at threshold 0.90, same expressions, still filtered.
10. **`TestSuppressKnown_AggregateMatch`** — aggregate result filtered
    against committed aggregate belief.
11. **`TestSuppressKnown_NoMatch`** — result with different expressions
    passes through.
12. **`TestSuppressKnown_EndToEnd`** — discovery → load committed → suppress
    → emit produces only new findings.

## Files Changed

| File | Change |
|------|--------|
| `cmd/wile-goast/lib/wile/goast/belief.scm` | Add `with-belief-scope`, `load-committed-beliefs`, `suppress-known` and helpers (`result-matches-any?`, `belief-expressions-match?`). |
| `cmd/wile-goast/lib/wile/goast/belief.sld` | Export the three new procedures. |
| `goast/belief_integration_test.go` | Add the 12 tests above. |
| `plans/BELIEF-DSL.md` | Flip §Suppression checkbox to DONE (if using that convention). |
| `CHANGELOG.md` | New entry under unreleased. |

## Trade-offs

- **Structural over semantic matching.** Cheap, predictable, no false
  suppression. Cost: a human rewriting `(contains-call "Lock")` as
  `(any-of (contains-call "Lock"))` creates a duplicate. Acceptable — tooling
  can canonicalize later if needed.
- **Pair return from `load-committed-beliefs`** instead of separate procedures
  for per-site and aggregate. One less round-trip for callers, simple shape.
  Alternative: a record type. Overkill for two fields.
- **Top-level directory load (no recursion).** Covers the typical flat
  `beliefs/` layout. Recursion can be added later with a second arg without
  breaking the signature.
- **Scope hygiene via `dynamic-wind`** vs. explicit save/restore at call
  sites. `dynamic-wind` is correct under continuations and early exit; the
  cost is nil in this code path (no call/cc in the belief DSL).

## Open Questions

None. Proceeding to implementation plan.
