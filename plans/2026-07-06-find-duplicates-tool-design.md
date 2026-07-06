# Design: `find_duplicates` command-level MCP tool

Date: 2026-07-06
Status: Design complete. Implementation plan: `2026-07-06-find-duplicates-tool-impl.md`.
Depends on: `plans/2026-07-05-goast-mcp-reachability-design.md` (shipped; friction
removal on the MCP surface). This is the first *feature-differentiation* follow-up.

## Problem

wile-goast adoption depends on unique features worth reaching for over gopls,
serena, and grep, not just lower friction. **Semantic duplicate detection** was
selected first during brainstorming (2026-07-06) on expected value × plausible
query frequency (ranking criterion is an open decision below, not a measured
fact). Frequency is *assumed*, not measured; the A/B protocol tests value, not
frequency. The capability is **machine-verified** SSA/AST function equivalence:
`dupl` (token-level) and gopls/serena (reference navigation) cannot produce it
*as verified equivalence*. An agent reading through those tools can still guess
at duplication; it cannot certify it. That capability is already shipped as the
`(wile goast dup-detect)` library, but it has **no command-level MCP tool**: it
is reachable only through `eval` + Scheme fluency, which the reachability
grounding showed agents avoid. The capability exists; the task-named surface does
not.

## Decision summary

Locked during brainstorming (2026-07-06):

1. **Scope:** surface the shipped library as **one** command-level tool
   (`find_duplicates`), then validate it in the LLMAccuracy A/B harness *before*
   adding any sibling tool. Protects the "one wedge proven first" bet against
   sliding back into breadth.
2. **Output contract:** the **measure surface is the default; the categorical
   verdict is opt-in.** Mirrors the library's existing `candidate->verdict`
   design and the recalibrated strategic bet (advisory; guides, never makes, the
   decision). "Validation" means the equivalence tier is machine-computed by the
   unify engine, not an LLM guess: that part is non-negotiable regardless.
3. **Deliverable boundary:** this spec delivers the tool **and a written A/B
   protocol**. Building and running the LLMAccuracy scenario is a separate
   follow-up in that repo (`~/ClaudeProjects/LLMAccuracy`), keeping this spec
   single-repo and shippable.
4. **Tool shape:** thin scan wrapper (Approach A below). Pairwise and cluster-
   report elaborations are deferred until the A/B proves the wedge.

## Prior art (surface, do not rebuild)

Verified present and shipped in `lib/wile/goast/dup-detect.scm`:
- `(find-scored-candidates target . opts)`: package scan: `go-func-refs` →
  IDF-filtered context → concept lattice → duplicate-candidate concepts →
  within-cluster pairs scored by unification measures. `opts[0]` = threshold
  (default `0.6`).
- `(candidate->verdict cand)`: **opt-in** projection of a candidate's
  `equiv-tier` (`proven`/`structural`/`divergent`) to
  `duplicate`/`likely-duplicate`/`distinct`.
- `(wile goast unify)`: the AST/SSA diff engine underneath the measures.
- Tested at `goast/dup_detect_test.go`; the measure surface + opt-in verdict are
  marked shipped in `plans/2026-06-01-auditable-categorization-design.md` (slice 5b).

Pipeline-tool pattern in `lib/wile/goast/pipelines.scm` +
`cmd/wile-goast/mcp_tools.go`: each tool is a `(pipeline-<name> target …)` that
returns `(pipeline-envelope version provenance result)`; a Go handler marshals it
to JSON via `cmd/wile-goast/marshal.go` (kebab→snake, nested alists→objects,
lists→arrays). Five tools exist; `find_duplicates` is the sixth, wired
identically.

## Approaches considered

- **A: Thin scan wrapper (chosen).** `find_duplicates(target, [threshold],
  [verdict])` wraps `find-scored-candidates` 1:1. Smallest build: one pipeline
  entry, one handler, one fixture, tests. Matches the shipped library exactly.
- **B: Scan + pairwise modes.** Add `functions:[A,B]` for a direct pairwise
  unify. Two code paths, ~2× tests, and pairwise is the lower-frequency query.
  Deferred; the obvious first follow-up if A validates.
- **C: Cluster-report aggregate.** FCA-cluster-annotated report like
  `recommend_boundaries`. Re-treads `find_false_boundaries` machinery; heavier;
  premature before validation.

## Design

### Tool surface

`find_duplicates`:
- `target` (required, string): Go package pattern, e.g. `my/pkg/...`.
- `threshold` (optional, number, default `0.6`): minimum effective similarity
  to keep a scored pair. A good starting value for testing; tunable later.
- `verdict` (optional, bool, default `false`): when true, attach the opt-in
  categorical verdict per candidate.

Task-indexed description leading with the question it answers (e.g. "Are
functions in this package semantic duplicates? SSA/AST-verified equivalence, not
token similarity: answers what grep/dupl and gopls can't").

### Scheme pipeline (`lib/wile/goast/pipelines.scm`)

`(define (pipeline-find-duplicates target opts) …)`:
- Read `threshold` (default `0.6`) and `verdict` (default `#f`) from `opts` (flat
  plist, same convention as `pipeline-recommend-split`).
- `(find-scored-candidates target threshold)` → candidate list.
- If `verdict` is set, map each candidate to itself plus a `verdict` field via
  `candidate->verdict`; otherwise leave candidates unchanged (measure surface
  only).
- `result` = the candidate list. `provenance` = `((target . …) (threshold . …)
  (candidate-count . N) (verdict-included . bool))`. `version` = `1`.

### Output contract

Default `result` is a list of scored candidates. Each candidate carries:
- two **located findings** (file:line for each function in the pair),
- `score` (effective similarity),
- a `measures` sub-object including `equiv-tier`
  (`proven`/`structural`/`divergent`),
- `why` provenance (`(unify-candidate …)`).

The equivalence tier is the machine-checked structural fact: the validation. No
`verdict` field unless `verdict:true`, in which case each candidate gains a
`verdict` of `duplicate`/`likely-duplicate`/`distinct`.

### Go handler (`cmd/wile-goast/mcp_tools.go`)

`handleFindDuplicates`, registered in `registerPhase1Tools` alongside the others:
declares the three params, builds the opts plist, invokes
`pipeline-find-duplicates`, returns the marshalled envelope. Errors surface via
MCP `isError`, not the envelope (existing convention).

### Error handling

Reuses the pipeline-tool convention: a Scheme error (bad target, load failure)
propagates to the handler and is returned as an MCP tool error. An empty result
(no candidates over threshold) is a **success** with `candidate-count: 0`, not an
error: "no duplicates found" is a valid, useful answer.

## Acceptance: two gates

"The tool works" and "the wedge is proven" are different questions, tested
differently, gating different things. Keep them separate.

| | Gate 1: product correctness | Gate 2: wedge value |
|---|---|---|
| Question | Does the tool do what it claims, deterministically? | Does surfacing it measurably help an agent? |
| Where | In-repo, `make ci` | Cross-repo, LLMAccuracy A/B |
| Kind | Deterministic pass/fail | Statistical, pre-registered margin |
| Gates | Shipping the tool | Building tool #2 |
| Failure means | Tool is broken; fix it | This surface, under this baseline and task set, didn't pay; do not expand |

A correct tool ships on Gate 1 alone and is useful to whoever reaches for it,
regardless of the Gate 2 outcome. Gate 2 gates *the next tool*, not this one.

### Gate 1 acceptance criteria (blocks ship)

Deterministic, verified by `make ci`:
1. **Envelope:** every response is a valid `{version, provenance, result}`;
   `version = 1`.
2. **Determinism:** identical `(target, threshold, verdict)` → byte-identical
   `result`.
3. **True-positive:** the planted duplicate pair surfaces with `score ≥
   threshold`, `equiv-tier ∈ {proven, structural}`, and **both** located
   findings carry `file:line`.
4. **True-negative:** the distinct function does not pair with its unrelated
   peer.
5. **Verdict discipline:** no `verdict` field by default; present and `∈
   {duplicate, likely-duplicate, distinct}` iff `verdict:true`.
6. **Empty success:** no candidates over threshold → success, `candidate-count:
   0`, not an error.
7. **Error path:** bad target / load failure → MCP `isError`, not a malformed
   envelope.
8. **Registration:** `find_duplicates` advertised alongside the five existing
   tools.
9. **Marshalling fidelity:** tier symbols (`'proven` etc.), nested
   `measures`/`why` alists, and the located-finding pair round-trip to JSON with
   no field loss or symbol mangling. *Verify against `marshal.go` before assuming
   "wired identically" covers this shape; the five shipped tools may never have
   emitted symbol-valued or doubly-nested fields.*

Criterion 3 is only meaningful if the planted pair actually clears `0.6`. Pin the
fixture's expected score (see Testing) rather than relying on the "or a high
threshold" hedge; if the pair doesn't clear threshold, criterion 3 tests nothing.

## Testing

- **New testdata fixture** `cmd/wile-goast/testdata/dups/`: a package with
  one genuine duplicate pair (two functions SSA-equivalent modulo renaming) and
  one clearly-distinct function. The existing `phase1` fixture is deliberately
  too cohesive to produce candidates.
- **Integration tests** (mirroring `TestCheckBeliefs_*` in
  `mcp_tools_integration_test.go`):
  - valid `{version, provenance, result}` envelope;
  - ≥1 candidate whose `measures.equiv-tier` is present and whose located
    findings carry file positions;
  - **verdict absent by default**;
  - **verdict present and well-formed** (`duplicate`/`likely-duplicate`/
    `distinct`) when `verdict:true`;
  - the distinct function does not surface as a candidate against its unrelated
    peer (guards against threshold/precision regressions);
  - `candidate-count: 0` success path on a fixture with no duplicates (or a high
    threshold).
- **Registration test:** `find_duplicates` advertised alongside the existing five
  tools.
- **Docs:** a row in `docs/MCP.md`'s pipeline-tools table.

## A/B validation protocol (Gate 2; defined here, built as a follow-up in LLMAccuracy)

Specified, not run in this repo:
- **Task set:** N refactoring/deduplication tasks over Go packages with known
  duplicate structure (labeled ground truth: which function sets are true
  duplicates). *Labeling protocol is an open decision below — labels must be
  defined independently of the tool's own threshold, else the tool grades its
  own homework.*
- **Arms:** A = `find_duplicates` available; B = baseline. *Baseline strength is
  an open decision below: B must be the agent's realistic no-tool behavior
  (grep/`dupl`/gopls **and free to read and reason**), not a tool-chain the agent
  can't reason around. A weak B inflates A.*
- **Primary metric:** correctness of duplicate identification: precision /
  recall / F1 against the labels, scored by the harness oracle. Report a
  precision/recall **curve across thresholds**, not a single `0.6` point, so a
  threshold mismatch shows as a curve shift rather than as "tool wrong"; pick the
  operating point after seeing the curve.
- **Secondary metric:** at equal F1, **cost-to-reach** correct identification
  (tokens/turns). A verified-equivalence tool most plausibly wins on cost against
  a reasoning baseline; F1-only + budget-cap would discard exactly that axis and
  false-negative a tool that ties on accuracy while spending less.
- **Budget-confound control:** cap or match the token/turn budget across arms so
  A cannot win on F1 merely by spending more (per the LLMAccuracy budget-confound
  finding on the critical path). Cost-to-reach is measured *within* the matched
  budget.
- **Go/no-go bar:** A must beat B by a **pre-registered margin** (open decision
  below) on F1, or on cost-to-reach at equal F1, to justify building tool #2. A
  null result does **not** by itself falsify the capability — it has four causes,
  and only the first justifies "stop, do not tune":
  1. **Capability low-value** — the only cause that warrants stopping.
  2. **Baseline already achieves it** — distinguish by reporting B's *absolute*
     F1: a null gap at high B means "baseline suffices"; at low B means "neither
     helped."
  3. **Surface too poor to be reached** — the failure the reachability work
     exists to fix. Log arm A's tool-*invocation* rate; if the agent rarely
     called `find_duplicates`, fix the description and rerun. Do not falsify.
  4. **Task set doesn't discriminate** — confirm the labeled set contains
     duplication the baseline provably misses before trusting a null.
  Rule out 2–4 before concluding 1.

The default `threshold` (`0.6`) is a starting operating point for the fixture
tests, not the pre-registered A/B operating point (that is chosen from the curve).

## Open decisions (author to set before the A/B is built)

These are bets or definitions only the author can fix; each must be written down
*before* the A/B run to mean anything.

- **TODO — ranking criterion.** What actually put duplicate detection first?
  Value-per-hit, query frequency, defensibility vs competitors, or "already
  shipped and cheap to surface"? Name it honestly in the Problem section; it
  bounds how much that section can claim.
- **TODO — pre-registered margin.** What F1 delta (or cost-to-reach delta at
  equal F1) over baseline makes tool #2 worth building? A bet-sizing call; record
  it before the run.
- **TODO — baseline strength (arm B).** Pin B as the agent's realistic no-tool
  behavior, including free reading and reasoning, not just `dupl` + gopls. The
  tool's marginal value concentrates in **large, cross-file** packages; for small
  co-located pairs the reading baseline ties it. Design the task set to sit in the
  regime where the claim holds, and state that boundary.
- **TODO — ground-truth labeling protocol.** Define "true duplicate" at the
  function-set level, independently of the tool's threshold (e.g. "behaviorally
  interchangeable modulo renaming"). Two SSA-equivalent functions kept separate
  on purpose (ownership, coupling — cf. the boundaries work) are "duplicates" to
  the metric but "correctly separate" to a human; decide how the labels treat
  that case.

## Out of scope (YAGNI)

- Pairwise mode (`functions:[A,B]`): Approach B; the first post-validation
  follow-up.
- Cluster-report aggregate: Approach C.
- Any change to `dup-detect.scm` or `unify.scm`: pure surfacing.
- Building/running the LLMAccuracy scenario: separate cross-repo follow-up.
- Additional command-level tools (call_path, etc.): out of scope here, but **not
  strictly gated on this wedge's A/B**. A null result for `find_duplicates` is
  duplicate-specific (its own value, frequency, and baseline) and does not
  transfer to a different capability with different economics. This A/B decides
  whether *this* wedge earns a sibling; it is evidence toward — not a veto over —
  tools that answer a different question. Each new tool carries its own
  value/frequency case; a capability with an independently strong case is not
  blocked by a weak result here.

## Files touched

- `lib/wile/goast/pipelines.scm`: +1 pipeline (`pipeline-find-duplicates`).
- `cmd/wile-goast/mcp_tools.go`: +1 handler + registration.
- Test fixtures: reused `examples/goast-query/testdata/dupcluster` (duplicate-bearing); added `cmd/wile-goast/testdata/nodups/` (empty-success). [impl decision superseded the originally-planned `testdata/dups/`]
- `cmd/wile-goast/mcp_tools_integration_test.go`: +tests.
- `docs/MCP.md`: +1 table row.
