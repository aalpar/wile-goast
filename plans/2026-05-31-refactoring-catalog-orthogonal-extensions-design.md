# Refactoring catalog — orthogonal extensions

**Date:** 2026-05-31
**Status:** design approved, pending spec review
**Output:** 18 new files in `./refactorings/`, same format as `pull.md`

## Motivation

`./refactorings/` documents 19 refactorings (`pull.md` + 18 others). All 19 share
three unstated traits:

1. **Single-function or single-type** — none cross a package or process boundary.
2. **Single-process** — no concurrency.
3. **Backed by a detector** — each carries a `Prior art in this repo` line
   pointing at a wile-goast primitive that finds the pattern.

These traits define the *shape* of what is covered, not the *space* of
refactorings. The catalog is dense in intra-procedural control/data-flow and
expression-level algebra, and empty everywhere else. This document adds 18
refactorings on four axes orthogonal to the existing set.

### Coverage map of the existing 19

| Axis | Files |
|------|-------|
| Intra-procedural control flow | inverse-if-conversion (`pull`), loop-unswitching, tail-merging, guard-clause-flattening, branch-fusion |
| Intra-procedural data flow / algebra | CSE, constant-folding, dead-store, algebraic-simplification, boolean-simplification |
| Structural clones (AST) | extract-function, parameterize-by-difference, reroll-loop |
| Conditional shape | consolidate-conditional, decompose-conditional |
| Type / call structure | state-variable-collapse, extract-class, inline-function, collapse-mutual-recursion |

## Format delta

Identical to `pull.md` (title + inverse note, **Precondition**, numbered
**Transform**, **What it optimizes / sacrifices**, verified before/after Go) with
one change:

- `**Prior art in this repo:**` → `**Detector status:** <status> — <detail>`

`<status>` is one of:

- **existing** — a primitive already finds the pattern (name it).
- **could-build** — existing layers (AST, SSA, CFG, call graph, FCA, lint)
  suffice; no detector wired up yet (name the layer).
- **out-of-scope** — needs analysis wile-goast does not have (say which:
  happens-before, escape analysis, etc.).

This turns the catalog into a tooling roadmap: the `could-build` set is the
backlog, the `out-of-scope` set is the frontier.

## Verification

Every Go snippet must compile and `go vet` clean before it is written into a
file — the same code-first discipline used for the existing 18 (build all
before/after pairs as a throwaway module, then document the verified code).

**Honesty caveat for concurrency (Bundle 3):** `go build` + `go vet` prove the
snippet is well-formed Go, not that the transform preserves behavior under
concurrency. `vet` catches `copylocks` and `loopclosure` but not deadlock or
race freedom. Each Bundle 3 file states this explicitly: correctness of these
transforms is a human/runtime concern, which is *why* their detector status is
out-of-scope.

## Location

Per `CLAUDE.local.md` ("ALL plan documents belong in `plans/` or `private/`"),
this design doc lives in `plans/`, not the brainstorming skill's default
`docs/superpowers/specs/`.

## The 18 files

### Bundle 1 — Structure & dispatch (4)

How data and decisions are shaped. Orthogonal to the existing set: those rewrite
*within* a chosen representation; these change the representation or dispatch
mechanism.

| File | Detector status |
|------|-----------------|
| `replace-conditional-with-polymorphism` | could-build — AST type-switch detection + `go-interface-implementors` |
| `replace-switch-with-dispatch-table` | could-build — AST (switch → `map[K]func`) |
| `replace-primitive-with-value-object` | could-build — `types.Info.Uses` / `go-func-refs` |
| `encapsulate-field` | existing (partial) — lint analyzers on exported fields |

`replace-conditional-with-polymorphism` and `replace-switch-with-dispatch-table`
are two answers to the same smell (a type/tag switch); documented separately
because the trade-offs differ (open vs closed set of cases).

### Bundle 2 — Boundaries: API + packages (5)

Cross-function and cross-package shape. The existing set never crosses a function
signature or package boundary; this bundle is entirely about those boundaries.

| File | Detector status |
|------|-----------------|
| `introduce-parameter-object` | could-build — signature analysis over `go-func-refs` |
| `remove-flag-argument` (split-by-flag) | could-build — **caller-side dual of `pull.md`** |
| `extract-interface` | existing — `go-interface-implementors` |
| `break-import-cycle` | existing — `verify-acyclic` / `go-list-deps` (split.scm) |
| `move-function-across-package` | existing (partial) — `recommend_split` |

`remove-flag-argument` is the dual of inverse-if-conversion: `pull.md` hoists a
predicate inside a function; this hoists a *boolean parameter* out to the call
site by splitting the function into two named functions.

### Bundle 3 — Concurrency (4)

Sharing vs communication. Entirely absent from the existing set. All out-of-scope
for detection today.

| File | Detector status |
|------|-----------------|
| `narrow-lock-scope` | out-of-scope (partial) — lock/unlock pairing belief exists; lock-span analysis does not |
| `replace-mutex-with-channel` | out-of-scope — needs happens-before reasoning |
| `confine-shared-state-to-goroutine` | out-of-scope — needs escape/ownership analysis |
| `replace-polling-with-notification` | out-of-scope — needs concurrency model |

### Bundle 4 — Flow, errors & allocation (5)

Iteration strategy and error/value propagation.

| File | Detector status |
|------|-----------------|
| `split-loop` | could-build — SSA/CFG (one loop, two independent concerns) |
| `loop-fusion` | could-build — **dual of split-loop** (two loops, same range) |
| `replace-loop-with-pipeline` | could-build — AST |
| `replace-sentinel-string-with-typed-error` | existing (partial) — lint `errorsas`; matches CLAUDE.md error policy |
| `replace-nil-check-with-null-object` | could-build — SSA nil-flow |

## Inverse / dual pairs introduced

The new set adds several pairs (mirroring the existing extract/inline,
merge/duplicate pairs):

- `split-loop` ⇄ `loop-fusion`
- `remove-flag-argument` ⇄ inverse-if-conversion (`pull.md`) — across the
  call boundary
- `replace-conditional-with-polymorphism` ‖ `replace-switch-with-dispatch-table`
  — sibling answers to one smell

## Folded in as mentions (not their own files)

Documented as a sentence inside a related file rather than a standalone file,
because each is a near-duplicate of an entry above or an existing entry:

- `replace-type-code-with-subtype` ≈ replace-primitive-with-value-object +
  replace-conditional-with-polymorphism.
- `preserve-whole-object` ≈ introduce-parameter-object (mention in that file).
- `consolidate-error-handling` / `wrap-at-boundary` ≈ a note in
  replace-sentinel-string-with-typed-error.
- `preallocate-slice` / `reduce-escape` — out-of-scope (escape analysis); noted
  but not written, to avoid a file whose only content is "the tool can't see
  this."

## Final count

19 existing + 18 new = **37 refactoring files**, spanning intra-procedural flow,
structural clones, type/call structure, dispatch, API/package boundaries,
concurrency, and error/iteration strategy.
