# Auditable Categorization — Provenance as an Intrinsic Property of Analysis Results

> **Status:** Design note (2026-06-01). Property-first statement, not an
> implementation plan. It names a property the suite already half-produces and
> systematically amputates, and states the representational requirement that
> closes the gap.
> **Scope:** How analysis results are *represented* across the suite — every
> finding carries `(value · where · why · score?)` by construction. The belief
> checker is the first and clearest violation; FCA and unification are the next.
> **Spec source:** Conversation 2026-06-01, generalizing from the user's
> statement that wile-goast must *guide humans to produce decisions* over a
> frontier of categorization, audit, and scores — "if you say these behave like
> a 'dog', show me the locations and a short narrative of why."

## Why this note exists

The suite is finishing its *known* territory — objective, true/false-decidable
analyses (AST diff, SSA equivalence, FCA min-cut, path algebra, the optimization
catalog in `refactorings/`). The frontier ahead is different in kind:
**categorization, audit, and scores in service of a human decision the tool
refuses to make.** This note fixes the representational precondition for that
frontier. Without it, every "soft" conclusion the tool proposes is unfalsifiable
by the person who has to act on it.

## The thesis

**A static-analysis result that you cannot point at and justify is an
incomplete result.** Provenance — *where* the finding is and *why* the tool drew
it, plus a *score* when one exists — is not downstream of a finding. It is part
of the finding's identity. The location and the reason are present at the AST
leaf and should ride the result to the surface untouched.

This reframes "auditing." It is **not a layer, mode, or feature** bolted onto an
analysis. It is the default observable form of *any* result once the tool stops
discarding provenance. "Show me every dog and a one-line why" is then not a
feature request — it is the trivial consequence of results that never lost their
locations.

The corollary is sharp: the places where the suite reports a *name* or a *bare
symbol* are not "missing a nice-to-have." They are **defects** — a result born
located, amputated on the way up.

## The core object

Every finding is a located, justified value:

```scheme
(finding
  (value    . <number | symbol | category>)   ; the measurement or class
  (where    . "file:line:col")                 ; resolved source position
  (why      . "short narrative")               ; the reason the tool drew it
  (score    . <rational in [0,1] | #f>))        ; membership/confidence, when one exists
```

A **category** is a set of findings sharing a `value` (the class label). A pure
numeric **measure** is the degenerate case: one bucket, `score` carries the
number, `why` may be empty. A pure **class** is the other limit: `value` is the
label, `score` is membership confidence, `why` is the discriminating evidence.
One object, two limits — *categorization, audit, and scores are not three things.*

## Audit is principal; the scalar is a user projection

The finding is not "a value with optional evidence." **The audit is principal**;
the scalar is what remains after the audit is *removed*. The API contract
follows from this:

1. **Every primitive returns the audited finding** — the located, justified
   form is the canonical, default result at the API boundary. No primitive
   returns a pre-stripped scalar.
2. **Stripping to a scalar is a user-invoked projection**, e.g.
   `(finding->scalar f) -> score`. It is a *user decision*, never an automation
   default. Automation that needs the scalar internally (the majority-vote
   aggregator, a Pareto comparator) *reads through* the finding — it projects to
   compare, it does not destroy the audit. The audited form survives every
   internal step and is always recoverable at the boundary.
3. **The asymmetry is deliberate.** Audit → scalar is a cheap, lossy projection
   the user may take at any time. Scalar → audit is impossible — the evidence is
   gone. So the only safe default is to *retain* audit and let the user discard
   it knowingly. Removing provenance is a choice the user makes, not one the
   tool makes for them.

## The systemic violation

Provenance exists at the leaves and is severed at result boundaries. Concretely,
verified this session:

| Site | What it has at compute time | What it returns | Severed |
|------|-----------------------------|-----------------|---------|
| AST mapper | `pos`/`end` as `"file:line:col"` (`goast/mapper.go:799-822`) | only with opt-in `'positions` (`AST-NODES.md:59`, marked `†`) | provenance is *opt-in*, not load-bearing |
| Belief checker contract | — | `(site ctx) -> category symbol` (`belief.scm:31`) | the contract has no slot for evidence |
| `evaluate-belief` | the per-site verdict | `(site . category)` pair (`belief.scm:609`); result is `(name maj-cat ratio total adherence deviations)` (`belief.scm:626`) | adherence is *just the site* (`:621`); deviation is *site + symbol* (`:624`) — no location, no why |
| `ordered` checker | `pos-a`, `pos-b` — the actual call positions (`belief-checkers.scm:114-115`) | collapses to `a-dominates-b` (`:117-118`) | computes the evidence, then **throws it away** |
| `find-call-position` | — | a 0-based *SSA-block instruction index* (`belief-checkers.scm:149-159`) | not yet a source `file:line` |
| FCA report | concept extent (objects) + intent (shared attributes) | object/type *names* | extent members carry no location; intent is never rendered as the *why* it already is |

The `ordered` case is the proof of the thesis: the reason a human would open an
editor for — the two call sites — is in hand at `belief-checkers.scm:114`, and
gone by `:118`. **The narrative is free; the tool already paid for it and
discards it.** The only genuinely *new* cost is resolving an SSA instruction
index back to a source position.

## What changes

The change is constitutional, not additive. Nothing is named a "layer."

1. **The checker contract grows an evidence tail.**
   `(site ctx) -> symbol` becomes `(site ctx) -> (symbol . evidence)` where
   `evidence = ((where . "file:line") (why . "...") (score . c))`. A bare symbol
   stays legal as the degenerate, evidence-less finding — **every existing
   checker keeps compiling and running unchanged.** Checkers that already
   computed locations (`ordered`, `paired-with`, `checked-before-use`) stop
   discarding them.

2. **`evaluate-belief` keeps evidence beside the category.** The aggregator
   still reads *only* the `value`/category field, so the majority-vote and the
   discretized `eq?`-codomain from the C/M note's resolution #1
   (`2026-05-31-objective-measure-subjective-norm-design.md:366-377`) are
   untouched. `adherence` and `deviations` become lists of *findings*, not
   names.

3. **The score channel is the C/M note's `c_i`, made per-member and rendered.**
   That note's resolution #2 already introduced a per-site membership confidence
   `c_i ∈ [0,1]` as a pre-filter
   (`2026-05-31-objective-measure-subjective-norm-design.md:216-231`). This note
   does not invent a score — it *surfaces the one already specified*, attaching
   it to the finding so the confidence is auditable rather than swallowed by a
   pre-filter.

4. **A position resolver: SSA instruction index → `file:line`.** This is the one
   real build. `find-call-position` returns a block-relative index; the resolver
   maps it to source via the `pos` data the AST mapper already carries
   (`mapper.go:799`). Making `'positions` load-bearing for analysis — not
   opt-in — is part of this.

5. **A renderer: category → editor-walkable report.** Label, then per finding
   `file:line — why (score)`. This is the literal "show me every dog and why."
   It is a *display* of provenance-carrying results, not a separate computation.

6. **Secondary producers adopt the finding shape.** FCA: extent members become
   located findings, the *intent* renders directly as the shared `why` ("these
   write `pc`, `sp`, `callStack`"). Unification: a candidate pair is two located
   findings; its measures (below) render as the `why`.

## Relationship to the C/M note

This note sits **with**, not under,
`2026-05-31-objective-measure-subjective-norm-design.md`. That note's **M**
(objective measure) *already presupposes located findings* — its review gate
(`:131`) is only executable if an M-result can be pointed at, and its named
failure mode, *"a garbage category yields a precise measurement of garbage"*
(`:124`), is only catchable by a human if each member of the category is
walkable. **Provenance is the precondition M assumed and the code violates.**
This note supplies it and extends that note's resolution #2 `c_i` into the full
finding object. The belief-checker fix instantiates this note exactly as
`receiver-parameter-asymmetry` instantiates the C/M note.

## First consumer — unification ("simplification")

The frontier work that surfaced this note was the question: *when does unifying
two structurally-similar functions reduce total complexity rather than merely
compress at the cost of indirection?* The answer reshaped the question.

**There is no `simplify?` verdict.** The tool cannot decide "reduces total
complexity" — that requires the cost of the abstraction introduced and the
functions' reasons-to-change, neither of which is in the IR. Decisions are
necessarily subjective; the tool's job is to supply measures and suspend
judgment. So the deliverable is a **measure surface over unification
candidates**, each measure a finding:

| Measure | Returns | Source |
|---------|---------|--------|
| `cand-benefit` | nodes removed (≈ one copy) | `shared-count` — have it (`unify.scm:309`) |
| `cand-type-params` | int | `roots` from `score-diffs` (`unify.scm:346`) |
| `cand-value-params` | int | `unique-value-params` (`unify.scm:343`) |
| `cand-new-edges` | int | call graph — **build** |
| `cand-creates-cycle?` | bool | merge-side analog of `verify-acyclic` (`split.scm:277` is split-only) — **build** |
| `cand-locality` | `same-pkg`/`shared-callers`/`disjoint` + dep-overlap | `go-func-refs` — **build** |
| `cand-equiv-tier` | `proven`/`structural`/`divergent` | bridge `ssa-equivalent?` — **build** |

Three are free from the existing diff; four are the cross-layer build. Note that
`unify.scm`'s existing `weighted-cost` (`:335-336`) is the *gap between the two
candidates*, **not** the cost of the merged abstraction — the benefit ledger has
only ever had its benefit half. `cand-new-edges` / `cand-creates-cycle?` /
`cand-locality` are the missing cost half.

**Equivalence is a tiered confidence finding, never a veto:** `proven`
(`ssa-equivalent?` succeeds) / `structural` (`unifiable?` clean but unproven) /
`divergent` (structural diffs remain). `unifiable?` (`unify.scm:357`) already
gates "clean type-substitution only"; the tier carries that as a *score*, not a
filter. `divergent` is an axis value, not a disqualifier — because some
properties don't matter in some situations.

**Locality is a ledger fact, not a verdict.** The tool reports
`disjoint-subsystems` / `shared-callers` / `dep-overlap`; the human reads it to
judge *coincidental duplication* (same shape, different reason-to-change). The
tool never asserts intent it cannot verify.

## Composition — the decision is a user script

The tool ships the measures and the *existing* Pareto machinery
(`pareto-frontier` / `dominates?`; `boundary-recommendations` already returns
three Pareto frontiers, `fca-recommend.scm:254`) as **one** documented
combinator — and forces no default. A situational goal is 3–10 lines of Scheme:

```scheme
;; "minimize surface area, refuse unproven merges, ignore locality"
(define (my-goal cands)
  (sort (filter (lambda (c) (not (eq? (cand-equiv-tier c) 'divergent))) cands)
        (lambda (a b) (> (cand-benefit a) (cand-benefit b)))))

;; "just the trade-off frontier, no weighting" — drop-in prior art
(pareto-frontier cands (list cand-benefit cand-type-params cand-locality))
```

The script *is* the predicate. wile-goast provides the tooling for the human to
make the decision; it does not make it. Pareto frontier is one very
well-documented measure that is trivial to integrate — not the answer.

## Non-goals and preserved invariants

- **No verdict.** No `simplify?`, no simplify/compress label, no veto. A finding
  is a proposal with evidence, as a number is a measure without a decision.
- **No default weighting.** Ranking direction is always the user's script.
- **The aggregator is unchanged** — majority-vote over the discretized symbol
  (C/M note #1). Provenance rides *alongside* the category; it does not enter
  the vote.
- **Backward compatible.** Bare-symbol checkers remain valid; evidence is
  additive.
- **Not a layer.** No new module named "audit." A representational property of
  results, threaded through the producers that already exist.
- **No automated stripping.** No primitive returns a pre-stripped scalar; the
  audited finding is the default at the API boundary. `finding->scalar` exists,
  but reducing a finding to a bare score is a *user* projection, invoked
  deliberately — never an automation default. Audit → scalar is lossy and
  one-way, so retaining audit is the only safe default.

## Failure modes — candor

1. **A garbage category yields a precise measurement of garbage** (inherited from
   the C/M note, `:124`). Provenance is the mitigation *delivery mechanism*: it
   is what lets the discover→commit review gate actually be run, by making each
   member falsifiable in an editor. It does not make a bad category good; it
   makes a bad category *visible*.
2. **Narrative quality is bounded by the checker.** A `why` is only as good as
   what the checker chose to retain. For tool-derived narratives (FCA intent,
   `ordered`'s two positions) this is mechanical and faithful. For
   user-classifier narratives it is whatever the user emits — the tool does not
   police it.
3. **Position resolution can fail** — an SSA instruction with no clean source
   correspondence yields `where = #f`. The finding is still emitted, flagged
   unlocated, never silently dropped (the project's "no silent caps" discipline).

## Prior art

The "result carries its location" idea is unremarkable in isolation — every
language server and linter emits diagnostics with positions. What is specific
here, stated minimally:

1. **Provenance unified with categorization and score in one finding object**,
   so a *statistically-mined category* (belief deviation, FCA concept) is
   walkable the same way a lint diagnostic is — the soft conclusion gets the
   same auditability as the hard one.
2. **The narrative is the analysis's own discarded by-product**, not a generated
   explanation. `ordered` already computed the two positions; FCA's intent
   already *is* the discriminating reason. This note recovers existing evidence
   rather than synthesizing post-hoc rationale (and so cannot hallucinate it).

Neither is a new algorithm. The contribution is representational: refusing to
amputate provenance, and unifying measure/category/score under one auditable
object so the frontier analyses are falsifiable by the human who must decide.

## Open questions — resolved (2026-06-01)

| # | Question | Resolution |
|---|----------|------------|
| 1 | Narrative format — free string vs. structured `(reason . data)`? | **Structured** `(reason-tag . data-alist)` with a `render-why` string projection. Bias to *downstream Scheme can filter/aggregate on it* (user decision) — consistent with "the user composes in Scheme." |
| 2 | Does `sites-from` chaining (`belief.scm:398`) carry evidence forward? | **No, not speculatively.** Chaining keys on category; evidence is additive and need not thread. A chained belief that wants the upstream `why` opts in later — don't build forwarding before a consumer needs it (YAGNI). |
| 3 | Position resolver placement? | **Resolved during slice 1** → shared `(wile goast provenance)` module (`ssa-instr-pos`, `ssa-call-position`), consumed by belief/FCA/unify alike. |
| 4 | Score semantics when a checker has no natural confidence? | **`score = #f`** ("no score exists") — honest over a fabricated `1.0`. |

### Resolution: evidence is additive, not a reshape

The blast-radius decision that falls out of the code (`evaluate-belief`
belief.scm:604-626; runner belief.scm:777-781): voting does
`(eq? (cdr p) maj-cat)`, and the `deviations`/`adherence` result fields plus
their consumers (`emit-beliefs`, `suppress-known`, the MCP `check_beliefs`
marshaller, existing tests) all read the current shape. So **evidence rides
alongside the category, never replaces it.** The run-beliefs result gains a new
`findings` field (located findings) *beside* the unchanged `deviations`/`adherence`;
the checker contract gains an optional evidence tail (`(symbol . evidence)`),
with a bare symbol remaining valid (evidence `#f`). This contains the change to
additions, keeping every existing consumer working.

### Slice sequencing

1. **Slice 1 (shipped):** the position resolver — `(wile goast provenance)`.
2. **Slice 2 (shipped):** the *pure* finding/evidence representation in
   `(wile goast provenance)` — `make-finding` + accessors, structured `why` +
   `render-why`, `render-finding`. No belief-contract change; zero blast radius.
3. **Slice 3 (shipped):** wire evidence through the checker contract +
   `evaluate-belief` *additively* (new `findings` field), with `ordered` as the
   first checker to emit real evidence. Impl:
   `2026-06-01-auditable-finding-evidence-impl.md`. Note: `ordered` resolves
   positions via `ssa-call-position` in *both* branches (same-block and
   cross-block) — the design's "already holds `pos-a`/`pos-b`" was only the
   same-block case; the established testdata exercises the cross-block branch.
4. **Slice 4 (shipped, renderer + FCA half):** the editor-walk renderer
   (`render-category`, generic over any finding list) and FCA's adoption of the
   finding shape (`boundary-findings`, a finding-shaped sibling of
   `boundary-report` whose extent members are located findings and whose `why` is
   the shared intent). The one cross-layer build was locating extent members:
   `ssa-field-summary` now carries `pos` (Go side), re-attached by name in
   `field-index->positions` — keeping `(wile algebra fca)` position-agnostic.
   `boundary-report` and the `find_false_boundaries` MCP marshaller are unchanged.
   Impl: `2026-06-01-auditable-finding-fca-render-impl.md`.
5. **Slice 5a (shipped):** the deduplication FCA audit trace — `(wile goast
   dup-detect)`, the `boundary-findings` twin on a `function × external-ref`
   concept lattice. Functions sharing a maximal informative ref-set (FCA concept,
   extent ≥ 2) are duplicate candidates; each extent member is a located finding
   whose `why` is the shared ref intent. Reuses the `split.scm` clustering chain;
   one Go build (`func-ref.pos`). Default output is the audit trace — no verdict,
   no measures (those are 5b), so **no cross-layer name reconciliation**. Impl:
   `2026-06-01-auditable-finding-dedup-trace-impl.md`. This realizes Phase 2 of
   `2026-04-17-fca-duplicate-detection-design.md`; its Phases 3–6
   (structural scoring / triage / verify) become 5b, with the verdict demoted from
   default to an opt-in projection (`candidate->verdict`), per this note's
   principle #2.
6. **Slice 5b (shipped):** unification's structural measure surface over the 5a
   candidates — the cross-layer name reconciliation (finally paid, via the
   `ssa-short-name` collapse joining `go-func-refs`/AST/SSA names), `ast-diff`/
   `ssa-diff` scoring, the benefit measures (`benefit`, `type-params`,
   `value-params`, `similarity`), and `cand-equiv-tier` = `proven` (SSA-canonical
   `unifiable?`) / `structural` (AST `unifiable?`) / `divergent` — the prior-art
   pattern, NOT the binop-level `ssa-equivalent?`. Each within-cluster pair → two
   located findings with `score` = similarity; the opt-in `candidate->verdict`
   projects the tier (default stays the measure surface, per principle #2); the
   existing `pareto-frontier`/`dominates?` is the one documented ranking
   combinator. Impl: `2026-06-01-auditable-finding-unify-measures-impl.md`.
7. **Slice 5c (next):** the cost half of the unification ledger —
   `cand-new-edges` (call graph), `cand-creates-cycle?` (merge-side analog of
   `verify-acyclic`), `cand-locality` (`go-func-refs` dep-overlap + shared
   callers). Each is an underspecified cross-layer build needing its own
   merge-semantics; they plug into the same Pareto combinator as additional axes.
8. **Deferred (not a slice):** the LLM judge (`dup-detect` Phase 5) as the
   requestable escalation for `uncertain` candidates; path-algebra cluster ranking.

## Relation to other plans

- [`2026-05-31-objective-measure-subjective-norm-design.md`](2026-05-31-objective-measure-subjective-norm-design.md)
  — the C/M architecture whose M presupposes the provenance this note supplies;
  this note extends its resolution #2 `c_i` into the finding object.
- [`2026-04-19-llm-concept-filter-design.md`](2026-04-19-llm-concept-filter-design.md)
  — FCA concepts as live-C categories; their extent members are the first
  non-belief producer to adopt the finding shape.
- [`2026-04-10-function-boundary-recommendations-design.md`](2026-04-10-function-boundary-recommendations-design.md)
  — the Pareto machinery (`dominates?`, `pareto-frontier`) reused verbatim as
  the one documented ranking combinator.
- [`BELIEF-DSL.md`](BELIEF-DSL.md) — the checker contract this note extends; the
  host producer for the finding object.
- `refactorings/` — the objective M-only catalog; its findings are the other
  consumer that should carry provenance natively.
```
