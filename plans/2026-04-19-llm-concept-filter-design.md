# LLM-as-Concept-Filter — Design Note

> **Status:** Design draft (2026-04-19). Short-form design note — not yet a
> full implementation plan.
> **Scope:** A cross-cutting technique for any FCA consumer in wile-goast.
> Not a new module; a design pattern and a thin library that existing FCA
> pipelines can opt into.
> **Spec source:** Conversation 2026-04-19 on FCA tuning and LLM integration.

## Goal

Use the LLM as a **precision-oriented semantic filter** applied to concepts
produced by FCA pipelines, not as a program analyzer. The symbolic layer
generates an exhaustive candidate set; the LLM prunes semantically incoherent
concepts the structural filters cannot distinguish from coincidence.

## Non-Goals

- **No LLM-driven analysis.** The LLM does not read Go source text, run
  queries, or build models. It only classifies already-computed concepts.
- **Not a soundness-preserving filter.** The filter is precision-oriented:
  on uncertainty or failure, fall back to *include*, not exclude. Correctness
  is the symbolic layer's job.
- **Not a replacement for structural filters.** IDF weighting, iceberg
  support thresholds, Pareto dominance, and min-extent/min-intent filters
  stay. The LLM filter runs *after* these, on what survives.
- **No new FCA consumer.** This plan does not introduce a new end-user
  analysis; it makes the existing ones (`fca-recommend`, duplicate
  detection, false-boundary reports, package splitting) optionally
  LLM-filtered.

## Motivation

FCA enumerates every closed concept in a context. Structural filters (IDF,
min-extent, support) prune by *syntactic* signal. They cannot distinguish:

- a semantically coherent concept — "functions managing request lifecycle"
- a structurally dense but meaningless concept — "functions that happen to
  both read `mu` and write `err`"

Semantic coherence is precisely what LLMs do well: judge "do these things
belong together?" given short, structured inputs. Symbolic filters can't
answer it; thresholds alone just trade false positives for false negatives.

The existing plan `2026-04-17-fca-duplicate-detection-design.md` places an
LLM judge at Phase 5 to decide *pair-level* duplication. This note
generalizes the same pattern one level up: a *concept-level* filter upstream
of every FCA consumer, not just duplicate detection.

## What's New vs. Prior Art

The technique is standard in adjacent fields:

| Field | Pattern |
|-------|---------|
| Information retrieval | Recall-oriented candidate retrieval, precision-oriented reranker |
| Theorem proving | Symbolic tactic search, LLM step selector (HyperTree, Draft-Sketch-Prove) |
| Code analysis | CodeQL / Semgrep candidates, LLM triage of false positives |
| RAG | Retrieval candidates, LLM answer filter |

What's new is applying it specifically to **FCA concepts as filter units**.
The unit of LLM judgment is a concept `(F, R)` — a pair of sets — not a
source-code pair, not a pattern match, not a candidate answer. This unit is
small, structured, and self-contained, which is the regime where LLM
judgment is both cheap and reliable.

The architectural claim: *FCA is the right symbolic partner for an LLM
filter because its output is already partitioned into semantically-loadable
units (concepts), not a stream of unrelated candidates.*

## Architecture

```
┌─ FCA pipeline (existing) ────────────────────────────────┐
│  context → concept lattice → structural filters          │
│  → surviving concepts [(F₁, R₁), (F₂, R₂), ...]          │
└──────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─ Concept summarization (new, pure Scheme) ───────────────┐
│  for each concept:                                       │
│    summarize extent: function names + short signatures   │
│    summarize intent: attribute names + sample values     │
│  → structured summary per concept (bounded size)         │
└──────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─ LLM filter (new, MCP prompt) ───────────────────────────┐
│  batched input: N concept summaries                      │
│  prompt: "Which of these are semantically coherent?"     │
│  output: per-concept verdict + optional name + reason    │
│  on error / ambiguity: default to INCLUDE (precision <1) │
└──────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─ Annotated output (existing consumer, unchanged) ────────┐
│  consumer receives concepts + (optional) LLM annotations │
│  consumer decides whether to use the annotations         │
└──────────────────────────────────────────────────────────┘
```

The LLM is one stage in a longer symbolic pipeline. It does not see raw Go
source. It does not produce concepts. It does not decide what analysis to
run. Its job is classification on a finite, pre-structured candidate set.

## Component Design

### New module: `(wile goast concept-filter)`

```scheme
;; Format a concept into a bounded-size structured summary suitable for LLM input.
(define (summarize-concept concept context metadata) ...)

;; Call the configured LLM endpoint with a batch of summaries.
;; Returns an alist: ((concept-id verdict [name] [reason]) ...)
;; verdict ∈ {coherent, incoherent, uncertain}
(define (llm-filter-concepts summaries) ...)

;; Apply the filter to a concept lattice. Concepts tagged 'uncertain are
;; retained (precision-oriented fall-back). 'incoherent concepts are culled
;; or flagged for review depending on mode.
(define (filter-concepts concepts context metadata mode) ...)
;;   mode ∈ {cull, annotate-only}
```

`annotate-only` mode returns concepts with LLM verdicts attached but keeps
everything. `cull` mode removes `incoherent` verdicts. The default should
be `annotate-only` — let downstream consumers decide whether to act on the
annotations. This keeps the symbolic pipeline auditable: a bug in the LLM
filter cannot silently cull valid concepts.

### Integration with existing FCA consumers

Each consumer gains an optional `llm-filter?` parameter:

```scheme
(recommend-split pkg 'idf-threshold 0.36 'llm-filter? #t)
(boundary-report lattice 'llm-filter? #t)
(duplicate-candidate-concepts lattice min-extent 'llm-filter? #t)
```

The parameter is off by default. Existing call sites need no change. A
caller that opts in gets concept verdicts attached to each reported concept;
they decide how to weight them.

### Transport

The LLM call itself is out of scope for the Scheme library. It returns a
record the consumer can treat as opaque data. Three transport options:

1. **MCP prompt** (default in the MCP server). A new prompt
   `goast-concept-filter` takes a batch of summaries and returns verdicts.
2. **External JSON** (batch mode, CLI use). Scheme writes summaries to a
   file; an out-of-band LLM invocation writes verdicts back; Scheme reads
   them. Used for offline analysis and reproducibility.
3. **Stub** (tests, CI). Deterministic verdict function for unit tests —
   no network calls, no LLM.

Only (3) is needed for the Scheme library to be testable. (1) and (2) are
separate concerns.

## Trade-offs and Open Questions

### What structural filters can't distinguish

| Concept shape | Structural filter verdict | LLM filter can help? |
|---------------|---------------------------|----------------------|
| Small extent × small intent | Pruned by min-extent/min-intent | Not reached |
| Large extent × large intent, coherent | Retained | N/A — already good |
| Large extent × large intent, incoherent | Retained (false positive) | **Yes** |
| Borderline extent/intent, coherent | Pruned by threshold (false negative) | No — LLM runs after pruning |

The filter addresses false positives that survive structural thresholds.
It does not recover false negatives that thresholds removed. To address
those, structural thresholds must be loosened — which increases the
LLM-filter load. The trade is thresholds-tight-plus-no-LLM vs
thresholds-loose-plus-LLM-filter; the latter has higher recall if the LLM
filter has acceptable precision.

### Precision vs recall of the LLM filter

The filter is precision-oriented by design: uncertainty → include. This
preserves recall (at the cost of some false positives still passing
through) rather than risking lost coherent concepts. A coherent concept
incorrectly culled is silent data loss; an incoherent concept incorrectly
retained is noise the downstream consumer can still reject.

### Non-determinism

LLM verdicts are stochastic. Re-running the pipeline can produce different
verdicts on borderline concepts. Mitigations:

1. `annotate-only` mode by default — the symbolic pipeline is deterministic;
   the LLM annotations are advisory.
2. Cache verdicts keyed on concept summary (same (F, R) → same verdict
   within a run).
3. Report verdict confidence; treat low-confidence verdicts as uncertain.

### Cost

Batched LLM calls dominate. A lattice of 200 concepts can be batched into
2-5 LLM calls depending on summary size. Per-call token budget is bounded
by the summary formatter. No growth in LLM cost as codebase grows beyond
what the lattice-growth itself dictates.

### Interaction with the existing duplicate-detection Phase 5 LLM judge

They compose cleanly:

- **Concept-filter** (this plan): LLM judges *concepts*. "Is this cluster
  semantically coherent?"
- **Duplicate-detect Phase 5**: LLM judges *pairs* within a cluster. "Are
  these two functions the same operation?"

A duplicate-detection run using both filters LLM-judges fewer things overall
because the concept-filter removes incoherent clusters before they reach
pair-level analysis.

### LLMAccuracy validation angle

The concept-filter is a concrete, measurable LLM task: given a batch of
summaries, classify coherence. It fits the LLMAccuracy benchmark structure
directly — A/B measurable with and without tool support, scoreable against
a hand-labeled corpus of concepts from wile-goast's own analysis output.
This makes it a good candidate for the first measured LLM-in-the-loop
feature.

## Files

| File | Purpose | Status |
|------|---------|--------|
| `cmd/wile-goast/lib/wile/goast/concept-filter.scm` | New: summarization + filter orchestration | Create |
| `cmd/wile-goast/lib/wile/goast/concept-filter.sld` | New: library definition | Create |
| `cmd/wile-goast/prompts/goast-concept-filter.md` | New: MCP prompt template | Create |
| `cmd/wile-goast/lib/wile/goast/fca-recommend.scm` | Add optional `llm-filter?` parameter | Modify |
| `cmd/wile-goast/lib/wile/goast/fca.scm` | Reuse `concept-extent`, `concept-intent`, `boundary-report` | Read-only |

## Validation

1. **Self-analysis.** Run the full FCA pipeline on wile-goast with and
   without the filter. Hand-label concept coherence. Measure filter
   precision/recall against labels.
2. **LLMAccuracy A/B.** Add concept-filtering as a task category. Compare
   Claude with and without the concept summary structure vs raw source.
3. **Stochasticity audit.** Re-run the same input N times; report verdict
   stability per concept.

## Status of Adjacent Plans

- `2026-04-17-fca-duplicate-detection-design.md` — pair-level LLM judge at
  Phase 5. This plan adds a concept-level filter upstream of it. Composes.
- `2026-04-10-function-boundary-recommendations-impl.md` — `fca-recommend`
  emits split/merge/extract concepts. Opt-in LLM filter would attach
  coherence verdicts to each recommendation.
- `2026-04-13-package-splitting-impl.md` — `recommend-split` emits split
  candidates as concepts. Same integration pattern.

## Next Step

Review and approve this design. Implementation plan
(`2026-04-19-llm-concept-filter-impl.md`) once approved. Estimate 6-8 TDD
tasks: summarizer, stub transport, filter orchestration, optional-param
integration across three consumers, MCP prompt, validation harness.
