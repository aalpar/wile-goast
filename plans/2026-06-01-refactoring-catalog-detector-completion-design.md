# Refactoring catalog — detector completion

**Date:** 2026-06-01
**Status:** design draft, pending spec review
**Output:** ~15 new files in `./refactorings/`, same format as `pull.md`
**Sibling:** `2026-05-31-refactoring-catalog-orthogonal-extensions-design.md`
(the prior pass that grew the catalog from 19 → 37 along four coverage axes)

## Motivation

The prior pass asked *"which axes of the refactoring space are empty?"* and
filled them: structure/dispatch, API/package boundaries, concurrency,
flow/errors. This pass asks a different question:

> For each analysis layer wile-goast uniquely has, is there a refactoring in the
> catalog that exploits it?

Read the catalog as the "tooling roadmap" the prior doc said it was, and a
mis-emphasis surfaces: **the catalog is densest where wile-goast is least
differentiated, and emptiest where it is most differentiated.** The entries
marked `Detector status: existing` lean on AST and lint — the analyses gopls
and golangci-lint already perform. The layers that distinguish this tool —
consistency-deviation (belief DSL), the liveness/sign/interval abstract
domains, call-graph reachability, FCA cross-boundary detection, path-algebra
recursion — back almost nothing.

### Capability → refactoring matrix (the gap)

| Capability (primitive) | Refactorings exploiting it | Coverage |
|------------------------|----------------------------|----------|
| AST structural clones (`ast-diff`, `unifiable?`) | extract-function, parameterize-by-difference, reroll-loop | dense |
| AST syntax shape | dispatch + conditional bundles | dense |
| SSA reaching-defs / dead store | dead-store-elimination | ok |
| SSA constant prop (`make-constant-propagation`) | constant-folding, pull.md per-path | ok |
| **SSA liveness (`make-liveness`, `domains.scm:102`)** | — | **empty** |
| **SSA sign/interval (`make-sign-analysis:210`, `make-interval-analysis:277`)** | — | **empty** |
| FCA field clusters (`cross-boundary-concepts`, `fca.scm:258`) | extract-class (split only) | **half** |
| CFG dominance/paths | guard-clause-flattening, branch-fusion, tail-merging | dense |
| Call-graph reachability (`path-query-all`; `go-callgraph-reachable` relocated to path-algebra, `goastcg/register.go:97`) | — | **empty** |
| Path-algebra SCC/recursion (`path-node-in-cycle?`, `path-algebra.scm:79`) | collapse-mutual-recursion | **one direction** |
| **Belief DSL / consistency deviation (`run-beliefs`)** | — | **empty** |
| `go-func-refs` (`prim_funcrefs.go`) / `types.Info.Uses` | replace-primitive-with-value-object, introduce-parameter-object | interface-narrowing is **not** here: `go-func-refs` emits a flat per-function ref set, drops same-package objects (`prim_funcrefs.go:113`), and never binds a method to the value it was called on — that binding is SSA's (next row) |
| **SSA call structure (`recv`/`method`/`args`, `goastssa/mapper.go:212`)** | accept-smallest-interface, encapsulate-collection, change-value-to-reference | **empty → Pass 2**: the method-set invoked on a parameter value — the interface-narrowing signal `go-func-refs` cannot supply |
| `go-interface-implementors` (`prim_goast.go`) | extract-interface | **one direction** |

The empty/half rows are the work of this pass. The belief DSL row is the
sharpest: Engler et al.'s premise — *deviation from a statistical norm is a
likely bug* — is the belief DSL's reason to exist, and the corresponding
refactoring ("normalize the deviant site to the dominant pattern") has no
catalog entry.

## Sequencing decision

Three options were considered for what should drive the next batch:

1. **Maximize tool-differentiation** — fill the empty matrix rows first.
2. **Complete the Fowler/Go-idiom set** — the familiar, high-frequency moves.
3. **Both, in two passes** — differentiation first, then idiom completion.

**Chosen: option 3.** Differentiation-first, because:

- It has the lowest marginal cost. Several of these detectors already ship; the
  gap is that no document points a user at them (see honesty note below).
- It corrects the strategic mis-emphasis directly — the catalog stops being
  dense exactly where the tool adds no value over an IDE.
- Idiom completion is more familiar and lower-risk, so it can safely follow.

This sequencing is the durable rationale: if a future reader asks "why did the
detector-backed refactorings land before the obvious Fowler ones?", the answer
is that the obvious ones were *already partly served by IDEs*, while the
detector-backed ones were the unique, unclaimed value.

## Detector-status taxonomy (unchanged from prior pass)

- **existing** — a primitive already finds the pattern (name it).
- **could-build** — existing layers suffice; no detector wired up yet (name the
  layer).
- **out-of-scope** — needs analysis wile-goast does not have (say which).

### Honesty note on "the detectors already exist"

An earlier informal framing claimed "six of seven Pass-1 detectors already
exist." Verified against source, the honest accounting is **2 `existing`, 2
`existing (partial)`, 3 `could-build`** (table below). The distinction:
`run-beliefs` and `cross-boundary-concepts` *emit the deviant/cross-boundary
sites directly* — the pattern is the output. The abstract domains
(`make-liveness`, `make-sign-analysis`, `make-interval-analysis`) emit
per-block lattice values; turning "this block's lattice value determines the
branch / outlives the variable's last use" into a *detected refactoring site*
is a small query not yet wired up. That query is cheap (the hard part — the
fixpoint solver — is done), but it is not zero, and the doc should not pretend
otherwise. Pass 1 is *lower-cost* than Bundles 1–4, not *free*.

### Two tiers of "could-build"

The `could-build` label spans two genuinely different costs, and the cheapness
argument above covers only the first:

- **Tier A — wire a query over an existing primitive's output.** The analysis
  already runs; the work is a predicate over its result. `prune-provably-dead-branch`
  (read the sign/interval lattice at a branch), `shrink-variable-scope` (read the
  liveness range), `accept-smallest-interface` (read SSA `recv`/`method`),
  `deduplicate-with-type-parameters` (read `unify`'s existing type/register diff
  classification), `encapsulate-collection`, `change-value-to-reference`,
  `replace-manual-init-with-sync.Once`.
- **Tier B — build a new classifier the codebase does not have.**
  `recursion ⇄ iteration` is the lone Tier-B entry (and the lone Tier-B item in
  Pass 1): `path-node-in-cycle?` detects *that* a function recurses, but
  recognizing the *convertible shape* (tail vs. accumulator) is a new AST+SSA
  recognizer with no existing primitive to query.

Verified against source: SSA already exposes the call-receiver binding
(`goastssa/mapper.go:212`) and `unify` already separates type diffs, so the two
entries that read as Tier-B on a first pass — `accept-smallest-interface` and
`deduplicate-with-type-parameters` — are actually Tier A. The marginal-cost leg
of the sequencing decision therefore holds for all of Pass 1 except
`recursion ⇄ iteration`; the durable rationale remains the strategic
mis-emphasis correction, not cost.

## Format

Identical to the prior pass: title + inverse note, **Precondition**, numbered
**Transform**, **What it optimizes / sacrifices**, verified before/after Go,
and a `**Detector status:**` line. New files use `Detector status:` (the newer
convention), not the older `Prior art in this repo:` used by the original 19.

## Pass 1 — fill the empty detector rows (differentiation)

| File | Detector status | Backing primitive / pattern detected |
|------|-----------------|--------------------------------------|
| `normalize-divergent-sites` | **existing** | `run-beliefs` — the `deviations` list *is* the site set; rewrite outliers to the dominant `adherence` pattern |
| `move-field` | **existing** | `cross-boundary-concepts` (`fca.scm:258`) — fields from different structs that co-vary are the move signal |
| `remove-unreachable-function` | **existing (partial)** | path-algebra reachability (`path-query-all`); unreachable = all − reachable(roots). Root-set selection (main + exported + tests + reflection) is the judgment |
| `merge-struct` (inline-class) | **existing (partial)** | same FCA report read inverse — a concept whose extent spans one struct's whole field set plus another's; promotes the extract-class "inline class" mention to its own file |
| `prune-provably-dead-branch` | **could-build (Tier A)** | `make-sign-analysis` / `make-interval-analysis` + CFG; a branch whose condition is determined by the block's lattice value. Strictly stronger than constant-folding (path-sensitive) |
| `shrink-variable-scope` | **could-build (Tier A)** | `make-liveness` (`domains.scm:102`) — declaration site earlier than the start of the live range ⇒ slide the declaration to first use |
| `recursion ⇄ iteration` | **could-build (Tier B)** | `path-node-in-cycle?` detects the recursion; recognizing the convertible shape (tail / accumulator) needs an AST+SSA classifier |

`normalize-divergent-sites` is the on-thesis centerpiece — it is the refactoring
the belief DSL was built to feed. `prune-provably-dead-branch` is the first
refactoring to consume the abstract-interpretation domains, which currently have
no catalog presence at all.

## Pass 2 — complete the Go-idiom set

| File | Detector status | Backing primitive / pattern detected |
|------|-----------------|--------------------------------------|
| `deduplicate-with-type-parameters` | **could-build (Tier A)** | `ast-diff` + a "differs only in type" classifier — the type-level twin of parameterize-by-difference; collapses `maxInt`/`maxFloat` clones via Go 1.18 generics |
| `accept-smallest-interface` | **could-build (Tier A)** | SSA call structure (`recv`/`method` for interface calls, `func`/`args[0]` for static method calls; `goastssa/mapper.go:212`) — the method-set invoked on a parameter value. Takes `*os.File` but only calls `.Read` ⇒ narrow to `io.Reader` ("accept interfaces, return structs"). *Not* `go-func-refs`, which emits a flat per-function ref set and drops same-package objects, so it cannot bind a method to its receiver value |
| `collapse-single-impl-interface` | **existing** | `go-interface-implementors` count == 1 with no mock need ⇒ speculative generality; the inverse of extract-interface |
| `encapsulate-collection` | **could-build (Tier A)** | SSA: a getter returns a field of slice/map type ⇒ return a copy or read-only view (aliasing hazard) |
| `change-value-to-reference` / receiver-consistency | **could-build (Tier A)** | AST receiver kinds + SSA mutation — value receiver that mutates, or a type with mixed receiver kinds |
| `introduce-defer-for-cleanup` (pair-acquire-release) | **existing (partial)** | `paired-with` belief checker — manual close/unlock on every return path ⇒ one `defer`. The lifecycle axis the prior four did not name |
| `replace-manual-init-with-sync.Once` | **could-build (Tier A)** | AST: a double-checked init guard ⇒ `sync.Once` |
| `replace-mutex-counter-with-atomic` | **out-of-scope** | needs happens-before reasoning (same caveat as Bundle 3); the transform is substantive enough to document with the honesty caveat, unlike `preallocate-slice` |

### The unnamed axis Pass 2 introduces: resource lifecycle / pairing

`introduce-defer-for-cleanup` adds a coverage axis orthogonal to the prior four
(structure, boundaries, concurrency, flow): **time / lifecycle.** It is also the
most direct consumer of the belief DSL's flagship `paired-with` checker
(Lock/Unlock, acquire/release), making it a second on-thesis entry alongside
`normalize-divergent-sites`.

## Inverse / dual pairs completed

The catalog's stated organizing principle is inverse/dual pairs; several are
currently one-sided. This pass closes them:

- extract-class ⇄ **`merge-struct`** (inline-class) — was a mention only
- extract-interface ⇄ **`collapse-single-impl-interface`** — was absent
- encapsulate-field ⇄ **expose-field** — folded as a mention (already present)
- collapse-mutual-recursion ⇄ **`recursion ⇄ iteration`** — recursion was
  detected but had no transform in either direction

## Folded in as mentions (not their own files)

Mirroring the prior pass's policy (avoid near-duplicate files):

- **expose-field** ≈ the inverse note already inside `encapsulate-field`.
- **introduce-recursion** ≈ the reverse direction inside
  `recursion ⇄ iteration`.
- **replace-counter-with-atomic** detection caveats ≈ a note inside
  `replace-mutex-with-channel` if the standalone file proves too thin.

## Deliberately excluded

- **Allocation / escape** (`preallocate-slice`, `reduce-escape`) — still
  out-of-scope (escape analysis the tool lacks). The prior doc's reasoning holds:
  do not write a file whose only content is "the tool can't see this."
  `replace-mutex-counter-with-atomic` is the boundary case that *is* written,
  because its transform is substantive and only its *detection* is out-of-scope
  (the Bundle 3 pattern), whereas `preallocate-slice` is uninteresting as a
  transform.
- **Trivial IDE-served moves** (rename, extract-variable, inline-variable) —
  excluded on the differentiation principle: gopls already does these well, and
  they carry no detector wile-goast uniquely provides. (Listed here so a future
  reader knows the omission is deliberate, not an oversight.)
- **Separate-query-from-modifier (CQS)**, **hide-delegate / remove-middle-man** —
  deferred, not rejected. Detectable (SSA side-effect analysis; call-graph
  delegation chains) but lower-frequency in idiomatic Go; revisit if a third
  pass is warranted.

## Verification

Same code-first discipline as the prior pass: every Go before/after snippet must
`go build` and `go vet` clean before it is written into a file (build all pairs
as a throwaway module, then document the verified code). For
`replace-mutex-counter-with-atomic` (out-of-scope detection), the same Bundle 3
honesty caveat applies: `vet` proves well-formed Go, not race freedom.

## Final count

37 existing + ~15 new = **~52 refactoring files** (Pass 1: 7, Pass 2: 8). The
catalog then has at least one refactoring exploiting every analysis layer
wile-goast provides — the matrix has no empty rows.

## Location

Per `CLAUDE.local.md` ("ALL plan documents belong in `plans/` or `private/`"),
this design doc lives in `plans/`. Implementation (writing the verified
refactoring files) is a separate step, taken pass-by-pass after this design is
reviewed.
