# MCP Tool Surface — Post-Filter Architecture Design Note

> **Status:** Design draft (2026-04-19). Short-form architectural note —
> not yet a full implementation plan.
> **Scope:** Redesign of the wile-goast MCP tool surface from a single
> `eval` tool to a set of pipeline-shaped tools, motivated by the
> symbolic-pipeline-with-LLM-post-filter pattern.
> **Spec source:** Conversation 2026-04-19 on LLMAccuracy A/B results and
> architectural implications.

## Goal

Expose wile-goast's existing deterministic pipelines as first-class MCP
tools that return structured reports. For the structural-analysis query
class, the LLM's role on each tool becomes report consumption
(classify, filter, rank, name) rather than pipeline orchestration
across multiple rounds. The existing `eval` tool continues to serve
exploratory, custom, and extension work in parallel, backed by ongoing
findability investments. Adding pipeline tools does not reduce `eval`'s
importance; the two modes are co-equal and address different query
classes.

## Non-Goals

- **No replacement for serena.** Serena is an LLM-driven symbol
  navigation and editing tool. This surface answers a different class of
  questions (structural analysis and recommendations). They are
  complementary, not competing.
- **No removal or demotion of the `eval` tool.** `eval` and the
  pipeline tools are co-equal modes serving different query classes,
  not a tiered "default plus fallback" arrangement. See "Relationship
  to Findability Work" below.
- **No new analysis pipelines.** This plan exposes pipelines that
  already exist in the Scheme codebase. Missing pipelines
  (`find_duplicates`, `filter_concepts`) are tracked in other plans.
- **No code editing capability.** The tool surface reports and
  recommends; it does not apply edits. Editing is a separate concern,
  handled by serena-class tools or by humans.
- **No language-agnostic abstraction.** Go-only, as wile-goast itself is
  today.

## Motivation

### Evidence

The 2026-04-19 LLMAccuracy gradient run (90 algebra problems,
claude-opus-4-7, control vs treatment-with-tools) produced a headline
treatment delta of -6%. Per-stop-reason breakdown shows the signal is
dominated by token-budget failures in the treatment arm:

- `max_tokens` always wrong on both arms (0/47 combined).
- Treatment uses ~8x input tokens per query (26K vs 3K).
- Treatment completions (`end_turn`) are *more* accurate than control
  completions (91.8% vs 86.5%) — tools help when sessions finish, but
  orchestration overhead ejects many sessions before they finish.

The failure mode is **LLM-as-orchestrator of symbolic tools under fixed
token budget**. Every round the LLM re-establishes helper functions,
re-discovers APIs, re-parses prior tool output. Context fills; output
budget shrinks; the model runs out of room before producing a final
answer.

The current wile-goast MCP surface (`eval` only) is the *most
orchestrator-shaped* possible. It inherits this failure mode directly:
the LLM writes Scheme, drives the interpreter, accumulates state in
context across rounds.

### Architectural response

Invert the authority. Move pipeline orchestration from the LLM into the
Go/Scheme layer. Expose each pipeline as a single MCP tool that:

- Runs end-to-end deterministically.
- Returns a bounded, structured report in one call.
- Carries provenance so the LLM can reason about result validity.
- Composes via parameters, not via LLM round-trips.

The LLM's role per tool call becomes short-horizon judgment on
structured input: the regime where its strengths (coherence judgment,
naming, classification) dominate and its weaknesses (long-horizon
stateful tool use) are absent.

## Proposed Tool Surface

| Tool | Pipeline | Default output | LLM's role |
|------|----------|----------------|------------|
| `find_duplicates` | FCA on refs → IDF filter → AST/SSA diff → triage (bucket A/B/C) | Ranked list of candidate pairs with bucket + similarity | Judge bucket-C pairs; name clusters |
| `find_false_boundaries` | FCA on struct fields → cross-boundary concepts + algebraic annotations | Boundary report with relationship annotations | Classify semantic coherence of concepts |
| `recommend_split` | Package IDF-FCA → min-cut → cycle check → confidence | Split proposal with rationale, confidence, blocker analysis | Review proposal; name packages |
| `recommend_boundaries` | Function-level FCA → Pareto frontiers for split/merge/extract | Three frontiers with rationale per candidate | Confirm which are structurally sensible |
| `check_beliefs` | Load committed beliefs → `run-beliefs` over pattern → structured results | Belief report with adherence/deviation per site | Judge reported deviations; suppress false positives |
| `discover_beliefs` | Statistical pattern discovery → patterns above threshold → `emit-beliefs` Scheme source | Draft `define-belief` forms with stats | Review before commit |
| `explain_function` | AST + SSA + CFG + callers/callees → structured report | Structured function summary | Synthesize human-readable explanation |
| `filter_concepts` | Batch of concept summaries → coherence verdict per concept | Annotated concepts (coherent/incoherent/uncertain) | The LLM *is* this tool — the canonical post-filter |
| `restructure_block` | `go-cfg-to-structured`: single-exit rewrite | Restructured AST + diff summary | Review the result |
| `trace_path` | Semiring path algebra between two functions | Semiring values + witness path | Interpret shortest/min/max result |

### Cross-cutting design choices

1. **Coarse-grained, not fine-grained.** Each tool call does substantial
   work. Reduces round-trips. Predictable budget per call.

2. **Structured output.** JSON-shaped alists the LLM can iterate over
   deterministically. No free-text tool output.

3. **Provenance included.** Which pipeline stages ran, thresholds used,
   what was filtered out, confidence where applicable.

4. **Parameter composition, not LLM orchestration.** Example:
   `find_duplicates(llm_filter: true, min_similarity: 0.8)`. The LLM
   chooses parameterization; the pipeline runs as one atomic operation.

5. **Session as handle.** Tools accept either a Go package pattern (load
   fresh) or a `session-id` from a prior `go_load` call (reuse
   snapshot). No implicit session accumulation across tool calls.

6. **`eval` as a peer, not a fallback.** `eval` serves a different
   query class — exploratory Scheme, custom aggregations, ad-hoc
   structural queries — backed by ongoing findability investments
   (see "Relationship to Findability Work"). Its presence and
   continued improvement are a distinct axis of development, not a
   consolation prize for gaps in the pipeline surface.

## Comparison to Serena

| | Serena | wile-goast post-filter surface |
|--|--------|-------------------------------|
| Design center | LLM-driven symbol navigation and editing | Structural analysis pipelines |
| Tool granularity | Primitive (one symbol, one edit) | Composed (one pipeline, one report) |
| LLM rounds per task | Many (navigate → read → read → edit) | Few (one pipeline call, one judgment) |
| Edits code | Yes | No — separate concern |
| Language scope | Multi-language via LSP | Go only |
| Answers "what exists" | Yes | Indirectly (via `explain_function`) |
| Answers "what is structurally wrong" | No | Yes |
| Answers "what should be changed" | No | Yes (recommendations with rationale) |

These are **different classes of tool**, not competing designs. A
realistic agent workflow uses wile-goast-class tools to *find* and
*propose*, then serena-class tools to *apply*. Both can coexist on the
same MCP client.

Where wile-goast's class is specifically load-bearing: large-codebase
refactoring, architectural audits, pre-merge structural review,
consistency enforcement across modules, regression detection on
structural invariants. These are tasks where serena's
navigate-one-symbol-at-a-time approach blows budget (same failure mode
the A/B data identified) and where a pipeline-shaped tool produces a
finite report cheaply.

## Existing Coverage Audit

Most pipelines exist; the work is MCP exposure, not analysis code.

| Tool | Pipeline status | Go handler status |
|------|------------------|-------------------|
| `find_duplicates` | Designed (`plans/2026-04-17-fca-duplicate-detection-design.md`), not implemented | Not started |
| `find_false_boundaries` | Implemented: `boundary-report`, `annotated-boundary-report` | Not exposed as MCP tool |
| `recommend_split` | Implemented: `(wile goast split)` | Not exposed as MCP tool |
| `recommend_boundaries` | Implemented: `(wile goast fca-recommend)` | Not exposed as MCP tool |
| `check_beliefs` | Implemented: `run-beliefs` | Not exposed as MCP tool |
| `discover_beliefs` | Implemented: `emit-beliefs` | Not exposed as MCP tool |
| `explain_function` | Pieces exist (AST, SSA, CFG, callgraph); not composed | Not started |
| `filter_concepts` | Designed (`plans/2026-04-19-llm-concept-filter-design.md`), not implemented | Not started |
| `restructure_block` | Implemented: `go-cfg-to-structured` | Not exposed as MCP tool |
| `trace_path` | Implemented: `(wile goast path-algebra)` | Not exposed as MCP tool |

The bulk of the work is in `cmd/wile-goast/mcp.go`: each pipeline gets
an MCP tool handler that wraps the Scheme call and shapes the result as
JSON. The Scheme pipelines run on the embedded engine and produce their
reports directly; they remain reachable via `eval` as well for users
who want to compose them with custom Scheme.

## Relationship to Findability Work

Significant ongoing investment in wile-goast (and in Wile generally)
targets **LLM findability** of Scheme primitives, libraries, and
idioms. This work is independently valuable, not a consequence of
pipeline-tool gaps. It has its own rationale and its own users.

### Why findability is load-bearing regardless of architecture

LLMs are writing a lot of Scheme. They write it to:

- Extend the pipeline set itself (custom analyses not yet factored into
  tools).
- Answer ad-hoc structural queries the pipeline surface does not
  anticipate.
- Explore a codebase interactively before a pipeline tool is the right
  next step.
- Implement new wile-goast features, libraries, and examples.

For every one of these uses, "the LLM can find the right primitive or
library fast" is a direct accuracy and budget input. The A/B data
makes this concrete: `pset-hard-007` burned a round on `apropos` to
discover `filter` before it could proceed. That round was not wasted
— it was the mechanism that let the model proceed — but reducing its
cost pays for itself across every `eval`-using session, and across
every non-wile-goast Wile use as well.

### Current findability investments

| Mechanism | What it helps the LLM find |
|-----------|----------------------------|
| `apropos` | Scheme primitives matching a pattern, with import status |
| Structured docstrings | Signatures, option lists, example snippets inline |
| MCP prompt `goast-scheme-ref` | Wile Scheme reference: available / missing / idioms / gotchas |
| MCP prompt `goast-analyze` | Which wile-goast layer fits a structural question |
| MCP prompts `goast-beliefs`, `goast-refactor`, `goast-split` | Task-specific playbooks |
| `features` procedure (wile v1.9.9) | Runtime capability discovery |
| SRFI registration | Discoverability of SRFI libraries through `apropos` |

Each of these has cost-per-use in context tokens, but amortizes across
all future LLM-written Scheme. They are a standing investment, not
overhead.

### How the two modes interact

`eval`-mode and pipeline-mode answer different query classes:

| Query shape | Mode | Example |
|-------------|------|---------|
| "Find X throughout this codebase" where X is structurally known | Pipeline | `find_false_boundaries`, `recommend_split` |
| "Run this analysis I just thought of" | `eval` | "functions calling `foo` whose callers are in package `bar`" |
| "Extend wile-goast with a new belief" | `eval` + findability | Write `(define-belief ...)` against a discovered API |
| "Check one specific invariant" | Pipeline if a belief exists; `eval` to write one | `check_beliefs` or discovery via `goast-beliefs` prompt |
| "Get a structured report of this codebase's state" | Pipeline | `recommend_boundaries`, `find_duplicates` |
| "Explore unfamiliar Scheme or Go API" | `eval` + findability | `apropos`, structured docstrings |

Neither mode replaces the other. A capable MCP surface has both,
tuned to their respective strengths. The post-filter architecture
identified by the A/B data does not reduce the importance of
findability — it changes the distribution of *when the LLM is
writing Scheme* (less during structural analysis, still a lot for
extension, exploration, and writing new pipelines).

## Trade-offs and Open Questions

### What the LLM gives up when using pipeline tools

Flexibility. Pipeline tools run fixed analyses with parameter knobs.
The LLM cannot compose a novel query within a pipeline call. For
novel queries, `eval` is the right mode; the LLM loses nothing
because the two modes are independently available.

### What the user gives up when using pipeline tools

The ability to ask questions the pipeline set does not anticipate. If
a user wants "functions that store to field X and are called from
package Y but not from package Z," that is an `eval`-mode query
today. This is not a gap — it is a correct division of labor: the
pipeline set targets analyses that carry structural weight; `eval`
mode handles bespoke queries.

### Report shape stability

Once pipelines are exposed as MCP tools, the JSON shape of their output
becomes an API surface. Pipeline changes that alter report structure
break LLM consumers. Mitigation: versioned report schemas, or a
consistent envelope (`{version, provenance, result}`) that lets the
LLM detect and adapt.

### Pipeline composition across tools

A workflow like "find false boundaries, then for each boundary check if
the involved functions are duplicates" requires two tool calls. Two
options:

1. **LLM composes.** Two tool calls; LLM threads results. This is
   minimal orchestration — one hop, not many — and fits the
   post-filter pattern.
2. **Pipeline composes.** A third tool combines the two. Cleaner for
   common combinations but leads to tool-set proliferation.

Recommendation: start with (1). Add combined tools only when a
workflow is both common and shows measurable benefit from combination.

### `eval` usage as signal, not failure

If LLMs reach for `eval` *when a pipeline tool could have served*, that
is useful signal — either (a) the pipeline set is incomplete, or (b)
the reports are not structured well enough for the LLM to use without
re-querying. This is worth monitoring as a health metric on the
pipeline surface specifically.

Reaching for `eval` *for queries the pipeline set does not cover* is
not failure — it is the correct mode for those queries. Continued
investment in findability (`apropos` completeness, docstring
coverage, prompt quality, SRFI discoverability) is the mechanism that
keeps this mode viable.

## Implementation Phases

The phases are incremental; each is independently useful.

**Phase 1: Expose already-implemented pipelines.** Ship as MCP tools:
`check_beliefs`, `discover_beliefs`, `recommend_split`,
`recommend_boundaries`, `find_false_boundaries`. Define report shapes
and provenance envelopes. No new analysis code.

**Phase 2: Build `filter_concepts` per the 2026-04-19 design.**
Canonical post-filter. Starts with stub transport for tests; MCP prompt
transport for production. Validate on hand-labeled corpus from
`plans/2026-04-09-fca-findings-goast.md`.

**Phase 3: Build `find_duplicates` per the 2026-04-17 design.** Compose
FCA, structural diff, algebraic equivalence, triage. Integrate
`filter_concepts` optionally for bucket-C.

**Phase 4: Round out the surface.** `explain_function`,
`restructure_block`, `trace_path`, and any others revealed by Phase 1-3
usage.

Each phase produces a shippable surface. Phase 1 alone is already a
meaningful redesign.

## Validation

1. **Self-analysis.** Run Phase 1 tools on wile-goast itself. Verify
   reports are consumable by a real MCP client (Claude Code).
2. **Round-trip latency measurement.** Per-tool latency, output size,
   compare to equivalent `eval`-based workflow. Target: single-call
   pipelines should be faster end-to-end than equivalent multi-call
   `eval` sessions.
3. **LLM post-filter effectiveness.** Once `filter_concepts` ships,
   measure verdict stability and hand-label agreement on wile-goast's
   own FCA findings.
4. **Public case study.** Run the full pipeline set on one real Go
   repository outside wile-goast. Produce findings. This artifact
   validates the architecture and creates the legibility the project
   currently lacks.

## Status of Adjacent Plans

- `2026-04-17-fca-duplicate-detection-design.md` — design for
  `find_duplicates`. This plan depends on that one for Phase 3.
- `2026-04-19-llm-concept-filter-design.md` — design for
  `filter_concepts`. This plan depends on that one for Phase 2.
- `2026-04-17-belief-suppression-design.md` — belief suppression with
  `with-belief-scope` etc. Affects `check_beliefs` report shape. Resolve
  suppression design before exposing the MCP tool.
- `docs/THESIS.md` — the architectural hypothesis this plan is the
  concrete MCP realization of.

## Next Step

Phase 1 implementation plan as `2026-04-20-mcp-phase-1-impl.md`.
Estimated 8-12 TDD tasks: tool handler per pipeline, report shape
definition, provenance envelope, MCP registration, integration tests
against an embedded MCP client.
