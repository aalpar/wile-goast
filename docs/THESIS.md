# Thesis: Algebraic Composition as Substrate for Agent-Facing Code Analysis

**Status:** Working draft. Some sections marked `TODO(aalpar)` contain the
firsthand observations and design decisions that only the author can articulate.

## Short form

Each static analysis layer (AST, SSA, CFG, call graph, lint) produces facts
in its own representation. The questions that matter for AI agents reasoning
about real codebases — *are these functions equivalent under type renaming?*,
*does this field cluster cross struct boundaries?*, *does every Lock pair with
an Unlock on all paths?* — are cross-layer and compositional. Flat tool APIs
(grep, read-file, LSP-style navigation) force the agent to compose those
facts at inference time, under a token budget, at which it is unreliable.

wile-goast's bet: the right substrate for *pre-composing* those facts is
algebraic — lattices, closure operators, semirings, symbolic rewriting —
exposed as first-class primitives the agent composes at tool-time rather than
reconstructing at inference-time. The benchmark measures whether this shift
produces capability uplift on tasks where flat tooling currently fails.

## Amendment (2026-07-12): the "unreliable at inference-time composition" claim is too broad

The Short form asserts that flat tooling forces the agent to compose facts at
inference time, "at which it is unreliable." Measurement says that is **not
unconditionally true**, and the correction sharpens the bet rather than sinking it.

Three results (LLMAccuracy; see `docs/when-tools-win.md` for the full argument):

| task | one step is | control | tool | verdict |
|---|---|---|---|---|
| call-graph reachability, 47-hop scrambled chain | a syntactic **lookup** | 91.7–100% | 100% | **margin over conventional tooling = 0.0%** |
| `powerset_lattice` (hard) | a set **computation** | 33% | 57% | tool wins (+23) |
| constant-index `[]func()` dispatch | a **wrong rule** | 65% | 100% | tool wins (+35, p<1e-6) |

The agent composes **47 sequential call-graph hops essentially perfectly**, and a
grep-armed baseline matches the tool exactly (margin 0.0 at every depth, n=60/rung,
source-withheld, adoption-gated). Composition *per se* is not the gap. The
reachability differentiation claim is **falsified**.

**And the consolation prize is smaller than it first looked.** Total tokens
(input+output; caching off):

| | control (read-file) | baseline (grep) | treatment |
|---|---|---|---|
| reachability | **2,228** | 12,470 | 2,320 |
| dispatch | **1,443** | 19,586 | 2,867 |

The tool is 5.4–6.8× cheaper than **grep** — grep's cost is nearly all *input*, since
each noisy round-trip re-sends the history. But grep-dumping an 80-function file is a
*bad strategy*, and beating a bad strategy is not differentiation. Against the honest
denominator — an agent that simply reads the file — the tool is **cost-neutral
(2,320 vs 2,228) or 2× worse (2,867 vs 1,443)**, because the tool round-trip costs
more than reading a ~1.2k-token file.

So on this axis the tool currently buys **neither accuracy nor cost**. That claim has
an expiry date, though: control's cost is `O(source)` while the tool's is
`O(answer)` — the tool returns a compact set no matter how big the file is. A
crossover must exist at larger `n`. It is **unmeasured**, and should not be asserted
until it is.

What actually predicts a tool win is the model's **per-step error rate**, not the
number of steps. Error only compounds if there is error to compound. Two things
make it non-zero:

1. **The step is a fallible computation** (lattice joins). Error compounds with
   depth; the tool wins. The original thesis is *correct here*.
2. **The step is a wrong rule** — the local syntactic reading disagrees with the
   semantics. The model fails at depth one; depth is irrelevant. Higher-order
   dispatch: the model treats *address-taken* as *invoked*, i.e. it re-derives
   CHA's exact over-approximation by hand, and is confidently wrong.

Where syntax tells the truth and each step is a lookup (direct call graphs), no
pre-composition can help, because there is nothing to get wrong.

**Restated bet:** the substrate earns accuracy where the agent's own derivation is
*unreliable* — not merely where it is *long*. Cross-layer facts qualify mostly
because they are arm-1 or arm-2, not because they are compositional.

## The compositional gap

Existing agent tooling for code understanding falls into three shapes:

1. **Text-shaped.** grep, read-file, sed. Operates on bytes; the agent
   reconstructs structure.
2. **Symbol-shaped.** LSP, gopls, IDE navigation. Exposes go-to-definition,
   find-references, rename. The agent sees symbols but not structure across
   symbols.
3. **Analysis-shaped (pre-packaged).** golangci-lint, staticcheck, semgrep.
   Exposes *conclusions* from fixed rule sets; the agent can't compose new
   queries.

None of these let an agent express a query like "find functions that acquire
a lock without releasing it on at least one path" without either (a) running
a pre-written analyzer for exactly that check or (b) reading tens of
thousands of tokens of source and composing the answer itself. The first
requires that the check already exists; the second is where accuracy
collapses.

### Intentional deferral of use cases

The project sequences tooling, then capability bound, then use cases — in
that order, deliberately. Committing to specific use cases before the
algebraic substrate has sufficient expressive range would pre-constrain
the research in a way that is hard to detect: an LLM collaborator given
a small tool palette tends to reformulate problems to fit it, and the
resulting narrowed problem looks like the natural question rather than
an artifact of what was available. The tooling-first order is a defense
against that framing pressure.

The coupled pair of false-boundary detection and boundary-aware
duplicate detection is the first target planned once the toolset is
judged complete. The two problems are entangled: standard clone
detection assumes function boundaries are written correctly, while
structural similarity is evidence that some existing boundaries are
wrong. Answering either cleanly requires answering both together, which
is itself an argument for a compositional substrate.

## The algebraic substrate

The primitives wile-goast exposes fall into four algebraic families:

- **Lattices and closure operators** (`(wile algebra lattice)`,
  `(wile algebra closure)`). Foundation for FCA: formal contexts, Galois
  connections, concept lattices. Used to discover field-access concepts
  that cross struct boundaries, and to rank refactoring candidates by
  Pareto dominance.
- **Semirings** (`(wile goast path-algebra)`). Bellman-Ford over call
  graphs parameterized by an arbitrary semiring — reachability, shortest
  path, must-call analysis, taint propagation all specialize from the
  same machinery.
- **Symbolic rewriting** (`(wile algebra symbolic)` via
  `(wile goast ssa-normalize)`). Normalization rules for SSA binops
  produce canonical forms; `discover-equivalences` checks semantic
  equivalence modulo a theory.
- **Boolean algebra** (`(wile goast boolean-simplify)`). Normalizes Go
  AST conditions and belief selector predicates so that structurally
  different expressions with the same truth function compare equal.

Each family solves a composition problem that cuts across the five analysis
layers. FCA composes SSA field-access data with AST struct declarations to
surface false boundaries. Path algebra composes call-graph topology with
edge-level predicates. Symbolic rewriting composes SSA operations with type
information to decide equivalence. Boolean normalization composes AST
expressions into a form where structural equality tracks semantic equality.

Operationally, two capabilities do most of the heavy lifting:

- **Normal forms for equivalence** — term rewriting reduces expressions
  to canonical form, so structural equality tracks semantic equality.
  Decides *local* equivalence.
- **Monotone transfer functions over lattices** — abstract interpretation
  (Cousot 1977) propagates abstract states across control flow with
  guaranteed convergence. Decides *non-local* behavior within an abstract
  domain.

Normal forms decide when two expressions mean the same thing; monotone
transfer functions decide what abstract property holds at a given program
point. Together they let the substrate answer the compositional question
that LLMs cannot answer reliably on their own: *are these two programs
equivalent in abstract domain X?* That question is unification modulo an
abstract theory, and it sits at the intersection of both capabilities.

### Why an extensible algebraic substrate

The library was induced by problem-level pressure, not chosen top-down. Each
new structural question — is this duplicate of that?, does this field
cluster cross a struct boundary?, does every call site check the error
return? — tended to need a new operator. Bolting operators on ad-hoc
produces a pile of special-purpose analyses with no shared structure.
Investing in a solid algebraic base (lattices, closure operators,
semirings, symbolic rewriting) meant each new question composes from
primitives that already satisfy their laws, rather than inventing fresh
machinery each time.

Algebra is not exclusive of logic. The ambient Wile environment ships with
logic engines; they are available in-process and can be exposed as MCP
primitives alongside the algebraic ones whenever a use case demands it.
The reason wile-goast's analysis primitives are algebraic today is that
the questions the project has focused on so far — unification modulo
renaming, concept lattices over field access, consistency deviations,
SSA equivalence — all reduce naturally to operator laws and normal
forms. No current use case has pushed back hard enough to need
Datalog-style logical inference.

Where logic has strong empirical advantage — whole-program points-to
(Doop), taint tracking and security queries (CodeQL), complex
interprocedural reachability — those analyses are not currently part of
wile-goast's scope. If they become scope, the substrate is already in
the box.

There is also a cognitive-fit component to this choice worth naming.
The author's background — early exposure to number theory, long habit
of thinking about programs in algebraic terms — makes algebraic
primitives the mental model in which correctness can be verified by
inspection. That matters when correctness is the binding constraint
(see below). Picking a substrate that matches the implementer's mental
model is not a weak reason; it is evidence for where careful work will
actually land.

### Two-stage development: correctness, then performance

The current algebra library is a reference implementation. Elegance is
instrumental: the algorithms have to be legible enough that their
correctness can be verified by inspection, because correctness —
specifically, accuracy of downstream agent queries — is the current
binding constraint. Efficiency is not yet a concern, and optimizing
before the use-cases settle would risk locking in the wrong interface.

Once a handful of structural questions are reliably answered on real
codebases, the follow-up is a BLAS-style optimized implementation of the
same algebra: same operator laws, same library surface, replaced
inner loops. Splitting the work this way decouples two concerns that
tend to entangle early in a project — *does this compose correctly?* and
*does this compose fast enough?* — and lets each be answered in its own
stage against independent evidence.

## Prior art

This thesis is a synthesis, not an invention. The individual pieces are
well-trodden.

- **Abstract interpretation** (Cousot 1977) established the algebraic-lattice
  foundation for all modern static analysis: abstract domains are lattices,
  transfer functions are monotone, fixed points exist by Tarski. Everything
  in wile-goast's abstract-domains library (sign, interval, reaching defs,
  liveness, constant propagation) is textbook abstract interpretation.

- **Interprocedural analysis as semiring** (Reps, Horwitz, Sagiv, IFDS 1995;
  IDE 1996). Interprocedural dataflow problems with distributive transfer
  functions reduce to graph reachability over a semiring. wile-goast's
  path-algebra layer follows this design.

- **Formal Concept Analysis applied to software.** Lindig and Snelting
  (ICSE '97) applied FCA to module recovery: functions as objects, imports
  as attributes, concept lattice reveals cluster structure. This is
  wile-goast's direct theoretical basis for cross-boundary concept
  detection — see the FCA section of `BIBLIOGRAPHY.md`.

- **Bugs as deviant behavior** (Engler et al., SOSP '01). Statistical
  deviation from local patterns as a bug signal, without explicit
  specification. Direct inspiration for the belief DSL.

- **Logic-based code analysis.** Doop (Bravenboer and Smaragdakis 2009),
  CodeQL, Semmle — Datalog engines that compose facts across layers and
  let humans write queries in a declarative logic. The most direct
  competitor to the thesis's compositional claim, working with a different
  substrate (logic programming) and a different audience (humans).

Against this prior work, wile-goast's actual contribution is narrower:

1. **Target audience.** The interface is shaped for LLMs: s-expressions
   (which LLMs write fluently), MCP tool protocol (which agents consume
   natively), algebraic primitives exposed at the library surface rather
   than hidden behind pre-canned analyses.

2. **Composable substrate, not composable queries.** CodeQL and Doop let
   humans compose queries in a logical language. wile-goast lets agents
   compose algebraic primitives in Scheme — different expressive power,
   different ergonomics for the intended caller.

3. **Measurement-first development.** The accompanying benchmark
   (`LLMAccuracy`) measures capability uplift on tasks designed to stress
   specific failure modes. Most analysis tools are evaluated on human
   ergonomics or analysis precision; few are evaluated on whether they
   make a downstream agent more accurate.

The synthesis claim is: *agent-facing* code analysis is a distinct
interface-design problem from either human-facing analysis (CodeQL, gopls)
or automated analysis (lint, abstract-interpreter dashboards), and algebraic
primitives on a Scheme/MCP surface are a plausible substrate for it.

## Falsifiable predictions

A thesis that cannot be broken is not a thesis. Predictions that would
falsify this one:

1. **Capability uplift does not generalize beyond synthetic benchmarks.**
   If wile-goast's primitives improve Sonnet's accuracy on hand-designed
   algebra problems but not on structural questions about real Go
   codebases (Kubernetes, Prometheus, etcd), the algebra is doing
   decorative work, not compositional work.

2. **Flat APIs suffice.** If an agent equipped with grep + read-file + a
   sufficiently long context window matches wile-goast's accuracy on
   cross-layer queries, the compositional-substrate claim is wrong: the
   bottleneck was context, not composition.

3. **Logic beats algebra.** If the same questions are answered more
   accurately by a Datalog-based agent interface (CodeQL over MCP, Doop
   as a tool), the substrate claim is wrong in direction: logic is the
   better fit for agents, not algebra.

4. **The benchmark is circular.** If the questions wile-goast answers
   well are exactly the questions designed to exercise wile-goast's
   primitives, the measurement doesn't generalize. Defense: questions
   must be motivated by independent criteria (hardness arises from
   token budget, state tracking, or combinatorial structure — not from
   "my tools match this shape").

TODO(aalpar): Your predictions. What findings, if produced by the
benchmark or by external users, would make you abandon or significantly
revise the thesis? This is the single most useful paragraph in the
document and only you can write it honestly.

## Scope of validation

The thesis is tested on two corpora:

- **Wile itself** — ~40k lines of Go, self-contained, well-understood by
  the author. Serves as a known-answer corpus.
- **Kubernetes universe** — large, heterogeneous, externally reviewed.
  Serves as a generalization test.

TODO(aalpar): Additional corpora worth including? Kubernetes has particular
conventions (generated code, operator pattern, heavy interface use) that
may bias findings. One more corpus of different flavor (e.g., etcd,
Prometheus, Vitess, Docker) would strengthen the generalization claim.
Which do you want to commit to, and why?

## Open questions

- **Where does the algebra stop earning its keep?** Not every question
  benefits from lattice-theoretic framing. When does the algebraic
  substrate become ceremony rather than leverage?

- **How do we know the primitives cover the interesting failure surface?**
  The current library grew organically from specific questions. A
  systematic account — here are the cross-layer query shapes, here are
  the algebraic tools that compose to answer them — would make the
  coverage claim auditable.

- **What's the right unit for the interface?** Primitives exposed to
  agents vs. high-level analyses (`recommend-split`, `cross-boundary-
  concepts`) vs. prompts that drive multi-step analyses. The trade-off
  between composability and discoverability isn't obviously settled.

TODO(aalpar): Other open questions you actually want answered. These
should be the questions you're uncertain about, not the ones you already
know the answer to. Reviewers trust researchers who name their
uncertainty.

## Related documents

- `README.md` — project overview and refactoring-session walkthrough
- `CLAUDE.md` — architecture and primitive reference
- `BIBLIOGRAPHY.md` — citations for the individual layers
- `plans/CONSISTENCY-DEVIATION.md` — belief DSL validation results
- `plans/UNIFICATION-DETECTION.md` — ongoing work on SSA equivalence v2
