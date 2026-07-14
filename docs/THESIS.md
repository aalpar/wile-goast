# Thesis: Pre-Composition as Substrate for Agent-Facing Code Analysis

**Status:** Working draft, revised 2026-07-13 after reconciling the prose against the
raw results in `LLMAccuracy`. Several earlier claims fell to that reconciliation;
[Superseded claims](#superseded-claims) records them rather than deleting them.
Sections marked `TODO(aalpar)` contain firsthand judgments only the author can supply.

## Short form

Each static analysis layer (AST, SSA, CFG, call graph, lint) produces facts in its
own representation. The questions that matter for AI agents reasoning about real
codebases (*are these functions equivalent under type renaming?*, *does this field
cluster cross struct boundaries?*, *does every Lock pair with an Unlock on all
paths?*) are cross-layer. Flat tool APIs (grep, read-file, LSP-style navigation)
force the agent to derive those facts at inference time.

**The bet:** a tool earns an accuracy advantage exactly where the agent's own
derivation is *unreliable*, and the way to buy that reliability is to *pre-compute*
the fact rather than have the agent reconstruct it.

Two things make a derivation unreliable, and the distinction organizes everything
below:

- **Arm 1: the step is a fallible computation.** Per-step error compounds with the
  number of steps. *Status: conjecture, **unmeasured**. No corpus has tested it (see
  [Arm 1 is unmeasured](#arm-1-is-unmeasured)).*
- **Arm 2: the step applies a wrong rule.** The local syntactic reading disagrees
  with the semantics, so the agent fails at depth one and depth is irrelevant.
  *Status: measured, +35.0 points, p < 1e-6, on one pattern.*

Note what this thesis does **not** say. It does not say the substrate must be
*algebraic*. That is a separate, currently unevidenced design bet, isolated in [The
algebraic bet](#the-algebraic-bet) so it can be attacked on its own.

## What is measured

All figures are recomputed from the raw result JSONs, not quoted from prose. Model:
`claude-opus-4-8` throughout. Results from superseded models are omitted here, in
either direction. Caching off. Unqualified `results_*.json`, `*.py`, and short commit
hashes below refer to the **`LLMAccuracy` repository**, not to this one; hashes for
this repository are marked as such where they appear.

Two terms recur. "Source-withheld" means the file was not pasted into the prompt, so a
tool arm must actually fetch it. "Adoption-gated" means the harness asserts
`tool_calls > 0`, so it discards a tool arm that silently never called the tool rather
than scoring it as the control arm in a costume.

### Reachability, depth axis (clean)

`results_reach_final.json`, n=60/rung, source-withheld, adoption-gated. The
generator pins **file size at 80 functions and answer size at 48 names**, so
depth `N` is the only variable.

| depth | control (read-file) | baseline (gopls+grep) | treatment (wile-goast) |
|---|---|---|---|
| d=4 | 98.3% (59/60) | 100% | 100% |
| d=47 | **91.7%** (55/60) | 100% | 100% |

Two margins, and they say different things:

- treatment − **baseline** = **+0.0 at both rungs.** This is the null.
- treatment − **control** = +1.7 at d4, **+8.3 at d47** (Fisher p = 0.057).

The document's own methodology says the honest denominator is the *strongest*
alternative. At an 80-function file, grep is at ceiling and is the strongest
alternative, so **the null stands: the tool buys nothing over conventional tooling
on plain syntactic reachability.** That claim is not in doubt.

What *is* in doubt is the explanation (§The error-rate model).

### Reachability, scale axis (confounded)

`results_reach_breakzone_all3.json`, n=8/rung.

| | control | baseline (gopls+grep) | treatment |
|---|---|---|---|
| f128 | 8/8 | 8/8 | 8/8 |
| f192 | 6/8 | 8/8 | 8/8 |
| f256 | 7/8 | 5/8 | 8/8 |
| f384 | 6/8 | 5/8 | 8/8 |
| **pooled f≥192** | **19/24 (79.2%)** | 18/24 (75.0%) | **24/24 (100%)** |

Treatment vs control at f≥192: Fisher p = 0.0496.

**Read this result narrowly.** The corpus varies file size, reachable-set size
(`n_reachable` runs 55 at f128 to 218 at f384), and depth *together*. It shows both
conventional arms degrading at scale while the tool holds, but it **cannot attribute
the cause**, and it supports no estimate of the per-step error rate. n=24 and p≈0.05
make it suggestive, not established. It is listed here because a document
that reports only the f80 null while sitting on a contrary f384 observation is
selecting its evidence.

### Higher-order dispatch (clean)

The corpus is constructed so the obvious syntactic reading is *wrong*:

```go
func f0() {
	t := []func(){f40, f34, f4}
	t[0](); t[1]();
}
```

Three function names sit in `f0`'s body, and the pattern-matching rule ("the text
`f40()` appears in `f0`, so `f0` calls `f40`") fires on all three. But the slice holds
three functions and only two are invoked. **`f4` is address-taken, never called.**

Getting this right requires tracking the slice as a *value*, finding which constant
indices are invoked, and mapping each index back to the element it selects. The slice
literal is on one line and the invocation on another: the fact lives in the **join**
of the two. That join is constant propagation through SSA. The model's rule is
*local*; the inference is not.

`results_dispatch_subtle.json`, n=60, 50 functions/problem, one never-invoked decoy.

| arm | score |
|---|---|
| control (clean source, no tools) | **65.0%** (39/60) |
| baseline (gopls + grep) | 23.3% (14/60) |
| treatment (wile-goast `'precise`) | **100%** (60/60) |

**+35.0 points over control, p < 1e-6.** The honest headline is +35 over control,
not +76.7 over grep: grep is line-oriented and the fact spans two lines, so its
collapse is a self-inflicted retrieval wound, not evidence about the tool.

The failure mode is the finding. Control's wrong answers were, systematically, the
true set plus the address-taken decoy. Reasoning from syntax, the model re-derived
Class Hierarchy Analysis's exact over-approximation and was confidently wrong. The
model does not hedge or flag uncertainty. From the inside, the 65% case and the 100%
case feel identical.

### Cost

Total tokens (input + output), caching off.

| | control (read-file) | baseline (grep) | treatment |
|---|---|---|---|
| reachability (f80, mean over both rungs) | **2,228** | 12,470 | 2,320 |
| dispatch (f50) | **1,443** | 19,586 | 2,867 |

The tool is 5.4–6.8× cheaper than grep, because grep's cost is nearly all *input*:
each noisy round-trip re-sends the history. But grep-dumping an 80-function file is
a bad strategy, and beating a bad strategy is not differentiation. Against the
honest denominator, the tool is **cost-neutral (2,320 vs 2,228) or 2× worse (2,867
vs 1,443)**: the tool round-trip costs more than reading a ~1.2k-token file.

On this axis the tool currently buys **neither accuracy nor savings**. A standing
conjecture holds that control's cost is `O(source)` while the tool's is `O(answer)`,
implying a crossover at larger `n`. That crossover is **pre-registered at n ≈ 85–100
and unmeasured.** Assert it only after measuring it. The conjecture dies outright if
the returned set grows as fast as the source, which the scale corpus above suggests it
may: `n_reachable` was 218 of 384 functions.

## The error-rate model

Let ε be the model's error rate on a **single step** and N the number of steps:

```
accuracy  ≈  (1 − ε)^N
```

This is a sketch to organize results, not a fitted curve. Steps are not independent
and the model does not literally multiply out.

### ε is small, but it is not zero

Fit ε to the clean depth ladder, where file size and answer size are pinned and only
N moves:

| depth | control observed | implied per-step ε |
|---|---|---|
| d=4 | 98.3% (59/60) | 0.0042 |
| d=47 | 91.7% (55/60) | 0.0018 |

A single **ε = 0.0019**, essentially the d=47 estimate, reproduces both rungs: it
gives 99.2% at N=4 (observed 98.3%, a one-problem difference at n=60) and
**91.4% at N=47 (observed 91.7%)**. The d=4 rung has one miss and so constrains ε
only weakly; the fit is carried by d=47.

This matters more than it looks. Until 2026-07-13 the project held that reading a call
name off a function body is a *lookup* whose error rate is **zero**, and concluded "you
cannot compound an error rate of zero." **The measurement does not show ε = 0. It
shows ε ≈ 0.002.** The compounding model accounts for exactly the 8-point decline that
was observed, and for its invisibility at n=60 (p = 0.057, and the d4→d47 decline
itself is p = 0.207).

So the correct verdict on the composition hypothesis is **not falsified, but
underpowered**. The experiment drove N to 47, at which a per-step ε of 0.002 buys
about 8 points of decay: below the resolution of 60 samples. The reachability null
is a real and useful result about *tool value at f80* (the tool beats no conventional
alternative there). It is not a result about *composition*, and the earlier document
read it as one.

### ε is a property of the step **and** its context

The scale corpus shows both conventional arms degrading while the tool holds. It is
confounded (see above), so it cannot tell us *why*. But it is enough to reject the
assumption that ε is fixed by the step type alone: the same kind of step, taken
against a larger file, is not equally reliable. Whether the driver is distractor
count, answer length, or unrecorded depth is **open, and directly testable**.

### What this implies

The two arms remain the right decomposition, but their standing differs sharply:

1. **Arm 1 (fallible computation).** ε > 0 by construction; error compounds; depth is
   the enemy. **Currently a conjecture with no supporting measurement.**
2. **Arm 2 (wrong rule).** The local syntactic reading disagrees with the semantics.
   ε is large at N=1. **Measured: +35 points, on one pattern.**

Where syntax tells the truth and ε is merely *tiny*, pre-composition buys nothing at
realistic N, and the tool must justify itself on cost or on scale, neither of which
it currently can.

### Where the criterion says the tool should and should not help

Stated sharply enough to be wrong. Each entry is a hypothesis, not a result, except
where marked *measured*.

**Should not help (syntax tells the truth).** Reading the text off the page is the
whole job:

- Direct call graphs at modest scale. *Measured: margin 0.0 vs grep at f80.* Note the
  scope: this is not "at any depth" (the `(1−ε)^N` fit puts control's degradation near
  N≈150) and not "at any scale" (control drops to 19/24 at f≥192).
- Anything where "the text says X" and "the program does X" agree.

A tool description that fails to say **when not to reach for it** is a pure cost leak:
a round-trip bought for zero accuracy. Over-invocation on this class is a measurable
defect of the shipped surface, and it is unmeasured.

**Should help (arm 2, syntax actively misleads).**

- Higher-order dispatch through a constant index. *Measured: +35 points.*
- Interface method dispatch: `x.Foo()` names no concrete implementation. **This is the
  criterion's falsification test** (experiment 6), and `'precise` does not even handle
  it.
- Semantic duplicates: two functions, different text, identical meaning. Syntax does
  not merely stay silent; it misleads, because the model assumes different-looking
  things are different things.
- "Is this value checked on **every** path?" Syntax shows you *a* check; the question
  is about *all paths*.

**Should help (arm 1, the computation is fallible).** Lattice and fixpoint
computations over a program: abstract interpretation, dataflow joins. **This arm has
never been measured** (§Arm 1). Listing it here is a hypothesis, and the honest state
of the evidence is that the project does not know.

Note what this list does *not* claim. Every fact above is already present in the
source the model was handed. None of these is an information-theoretic gap; all are
error-rate gaps, and error rates fall as models improve. See hypothesis 8.

### Arm 1 is unmeasured

**No measurement of arm 1 exists.** The claim "the tool wins where the step is a
fallible computation" has never been tested under conditions that could support it:
no corpus in `LLMAccuracy` puts a fallible-computation task on Go source, through
wile-goast, against a non-algebraic tool baseline, on the current model.

Arm 1 may well be true. Abstract interpretation exists because fixpoint computation
is hard. But **this project has not measured it**, and until it does, arm 1 is a
load-bearing wall nobody has inspected. It is also the arm that would justify the
algebra. Say that coincidence out loud rather than gloss over it: the one arm that
would vindicate the design bet is the one arm with no data. Building the corpus that
would settle it is the project's highest-value open experiment; experiment 2 states
its four conditions.

## The compositional gap

Existing agent tooling for code understanding falls into three shapes:

1. **Text-shaped.** grep, read-file, sed. Operates on bytes; the agent reconstructs
   structure.
2. **Symbol-shaped.** LSP, gopls, IDE navigation. Exposes go-to-definition,
   find-references, rename. The agent sees symbols but not structure across symbols.
3. **Analysis-shaped (pre-packaged).** golangci-lint, staticcheck, semgrep. Exposes
   *conclusions* from fixed rule sets; the agent can't compose new queries.

None of these let an agent express a query like "find functions that acquire a lock
without releasing it on at least one path" without either (a) running a pre-written
analyzer for exactly that check or (b) reading tens of thousands of tokens of source
and deriving the answer itself. The first requires that the check already exists. The
second is arm 1, arm 2, or neither, and which it is decides whether a tool helps.

### Intentional deferral of use cases

The project deliberately sequences tooling, then capability bound, then use cases.
Committing to specific use cases before the substrate has sufficient expressive range
would pre-constrain the research in a way hard to detect: an LLM collaborator given a
small tool palette tends to reformulate problems to fit it, and the resulting narrowed
problem looks like the natural question rather than an artifact of what was available.
The tooling-first order is a defense against that framing pressure.

False-boundary detection and boundary-aware duplicate detection are the first target,
once the toolset is judged complete. The two problems are entangled: standard clone
detection assumes function boundaries are written correctly; structural similarity is
evidence that some existing boundaries are wrong. Answering either cleanly requires
answering both together.

## The algebraic bet

**This section states a design choice for which the project has produced no
discriminating evidence.** It is separated from the thesis above so that the thesis
survives its refutation, and so that it can be refuted.

The primitives wile-goast exposes fall into five families:

- **Lattices and closure operators** (`(wile algebra lattice)`, `(wile algebra
  closure)`). Foundation for FCA: formal contexts, Galois connections, concept
  lattices. Used to discover field-access concepts that cross struct boundaries, and
  to rank refactoring candidates by Pareto dominance.
- **Semirings** (`(wile goast path-algebra)`). Bellman-Ford over call graphs
  parameterized by an arbitrary semiring: reachability, shortest path, must-call
  analysis, and taint propagation all specialize from the same machinery.
- **Symbolic rewriting** (`(wile algebra symbolic)` via `(wile goast
  ssa-normalize)`). Normalization rules for SSA binops produce canonical forms;
  `discover-equivalences` checks semantic equivalence modulo a theory.
- **Boolean algebra** (`(wile goast boolean-simplify)`). Normalizes Go AST conditions
  and belief selector predicates so that structurally different expressions with the
  same truth function compare equal.
- **Context-free reachability** (`(wile algebra cfl)` via `(wile goast ifds)`, `(wile
  goast taint)`). The Reps-Horwitz-Sagiv-Rosay valid-path grammar: a return must
  match its own call. Matched call/return is a Dyck language, so no choice of
  semiring makes Bellman-Ford reject the unrealizable paths; distinguishing
  "reachable" from "reachable along a path that could actually execute" needs a
  grammar, not a weight.

### What "algebraic" is stipulated to mean

**Stipulation.** A substrate is *algebraic* in this document's sense iff its
composition operators are **specified by equational laws that the implementation is
required to satisfy** (associativity, idempotence, absorption, distributivity of
`⊗` over `⊕`, monotonicity, and so on), so that a new query comes from instantiating
an existing operator at a new carrier, not from writing a new algorithm.

Under this stipulation:

- Semirings, lattices, closure operators, boolean algebra: **algebraic.**
- `(wile algebra cfl)`: **not algebraic.** It is a grammar-directed reachability
  algorithm. It is in the library because the problem demanded it, and its presence
  is a *count against* the bet, not evidence for it. Listing it as a sixth algebraic
  family was a category error.
- Datalog / Doop / CodeQL: **not algebraic.** Composition is by logical inference over
  relations, not by operator laws.
- `goastcg/precise.go`: **not algebraic.** It is CHA plus an SSA constant-index
  resolution pass, written directly against `golang.org/x/tools`.

### The honest reason for the choice

Problem-level pressure induced the library. Each new structural question tended to
need a new operator, and bolting operators on ad-hoc produces a pile of
special-purpose analyses with no shared structure. Investing in an algebraic base
meant each new question composes from primitives that already satisfy their laws.
That is an argument about **maintainability of the analysis library**, which is a
real benefit, and it is *not* an argument that agents get more accurate answers. The
two have been run together and should not be.

The choice also has a cognitive-fit component. The author's background makes algebraic
primitives the mental model in which correctness can be verified by inspection. That
is evidence about **where careful work will land**, not evidence that the substrate
is correct. Naming it as the latter would be a category error of the same shape as
the CFL one above.

Logic engines ship in the ambient Wile environment and could be exposed as MCP
primitives. This does *not* make the substrate claim safe either way: a bet that
absorbs its own falsifier is not a bet. If Datalog answers these questions more
accurately than algebra, **the algebraic bet is wrong**, and adopting Datalog is
losing it, not hedging it.

### Two-stage development: correctness, then performance

The current algebra library is a reference implementation. Elegance is instrumental:
the algorithms have to be legible enough that their correctness can be verified by
inspection, because accuracy of downstream agent queries is the binding constraint.
Efficiency is not yet a concern, and optimizing before the use-cases settle would
risk locking in the wrong interface.

Note the assumption this rests on: *verifiable by inspection* is not *verified*. This
document cites no law-checking or property-test evidence for the algebra library, in a
project whose stated third contribution is measurement-first development. An incorrect
lattice join would silently corrupt exactly the arm-1 results that are already the
weakest.

Once a handful of structural questions are reliably answered on real codebases, the
follow-up is a BLAS-style optimized implementation of the same algebra: same operator
laws, same library surface, replaced inner loops.

## Prior art

This thesis is a synthesis, not an invention. The individual pieces are well-trodden.

- **Abstract interpretation** (Cousot and Cousot 1977) established the
  algebraic-lattice foundation for all modern static analysis: abstract domains are
  lattices, transfer functions are monotone, fixed points exist by Tarski. Everything
  in wile-goast's abstract-domains library (sign, interval, reaching defs, liveness,
  constant propagation) is textbook abstract interpretation.

- **Interprocedural analysis as semiring** (Reps, Horwitz, and Sagiv, IFDS 1995;
  Sagiv, Reps, and Horwitz, IDE 1996). Interprocedural dataflow problems with
  distributive transfer functions reduce to graph reachability over a semiring.
  wile-goast's path-algebra layer follows this design.

- **Formal Concept Analysis applied to software.** Lindig and Snelting (ICSE '97)
  applied FCA to module recovery: functions as objects, imports as attributes,
  concept lattice reveals cluster structure. This is wile-goast's direct theoretical
  basis for cross-boundary concept detection.

- **Bugs as deviant behavior** (Engler et al., SOSP '01). Statistical deviation from
  local patterns as a bug signal, without explicit specification. Direct inspiration
  for the belief DSL.

- **Logic-based code analysis.** Doop (Bravenboer and Smaragdakis 2009), CodeQL,
  Semmle. Datalog engines that compose facts across layers and let humans write
  queries in a declarative logic. The most direct competitor to the compositional
  claim, on a different substrate and for a different audience.

Against this prior work, wile-goast's contribution is narrower:

1. **Target audience.** The interface is shaped for LLMs: s-expressions, MCP tool
   protocol, primitives exposed at the library surface rather than hidden behind
   pre-canned analyses. (The premise that LLMs write s-expressions more fluently than
   alternatives is **assumed, not measured**, in a project whose method is
   measurement.)

2. **Composable substrate, not composable queries.** CodeQL and Doop let humans
   compose queries in a logical language. wile-goast lets agents compose primitives
   in Scheme. Note that for the one comparison this document actually draws, logic is
   **strictly more expressive**: Datalog handles the Dyck-language valid-path problem
   that no semiring can, which is why `(wile algebra cfl)` had to be written by hand.

3. **Measurement-first development.** The benchmark (`LLMAccuracy`) measures
   capability uplift on tasks designed to stress specific failure modes. Few analysis
   tools are evaluated on whether they make a downstream agent more accurate. This
   contribution is real, and this revision of the document is the evidence:
   measurement retired three of the project's own claims outright, and auditing the
   harness and raw results against the prose retired five more.

The surviving synthesis claim: *agent-facing* code analysis is a distinct
interface-design problem from either human-facing analysis (CodeQL, gopls) or
automated analysis (lint, dashboards), and pre-composing facts the agent would
otherwise derive unreliably is a plausible substrate for it.

## The external-validity hole

Every accuracy figure in this document was obtained with a hand-written harness prompt
**that does not exist in the shipped product**: `TREATMENT_PROMPT` for the depth ladder
(`evaluate_reachability.py:49`), `PRECISE_TREATMENT_PROMPT` for dispatch (`:116`). The
surface a real Claude Code user gets has never been measured against any of these
corpora, so **how much that surface costs is unmeasured** (hypothesis 9). Any claim of
the form "the tool gives +35" is conditional on a prompt the user never sees.

What *is* measured is narrower than that, and it is enough to make the hole worth
naming.

### A tool that returns a bound can harm its consumer

`results_reach_sub.json` (`a663024`) and `results_reach_sub_verify.json` (`a9a135b`):
the same fifteen problems, identical ground truth, identical control results. The
corpus is built so `'vta` over-approximates by exactly one address-taken decoy.
`'precise` is withheld from both arms. Only the prompt changes.

| treatment prompt | treatment | control | margin |
|---|---|---|---|
| `'vta`, report its result | 6/15 (40.0%) | 11/15 (73.3%) | **−33.3** |
| `'vta`, plus a mandate to re-derive each member's constant index by hand and drop the ones no invoked index selects | **15/15 (100%)** | 11/15 (73.3%) | +26.7 |

Neither margin is significant alone (Fisher p = 0.139 and p = 0.100 at n=15). The
comparison that matters is paired, and it is: across the same fifteen problems the
mandate fixes nine and breaks none (McNemar exact, **p = 0.0039**), and the cases where
the tool left the model *worse than no tool* go from six to zero.

Two things follow. The second is the uncomfortable one.

- **Exactness is not a courtesy.** Handed a sound over-approximation, the model
  reported the bound as the answer and scored below control. Anchoring, not tool error.
- **What recovered the sixty points was not a better tool description.** Both prompts
  describe `'vta` identically. The second adds an instruction to distrust the tool's
  output and re-derive the answer by hand (`evaluate_reachability.py:143`). The tool
  narrowed the candidate set; the model performed the fallible computation over it
  anyway. On this corpus pre-composition delivered a bound, not the fact, and the
  points came from the step it was supposed to remove.

### What this does not show

The corpus was **engineered to make the tool hurt.** `'vta` is flow-tracked and so
"pinnable to precise ∪ {fS}" (`a663024`); the decoy was tuned until the
over-approximation was off by exactly one function. The harness says as much:
`evaluate_reachability.py:108-109` calls it "a rigged tool, not the tool wile-goast
actually ships."

The inference that the *shipped* surface reproduced that rigged configuration is a good
one. `'precise` is a wile-goast invention absent from the model's training priors;
`static`/`cha`/`rta`/`vta` are `x/tools` names it knows. Before wile-goast `16653eb` the shipped
`eval` description named no call-graph algorithms and the `reference` cheatsheet omitted
`'precise` entirely, leaving one under-approximation, three over-approximations, and no
exact option. **A model not told an algorithm exists cannot select it.** But an
inference is not a measurement, and this one already has an experiment waiting:
hypothesis 9, `LLMAccuracy/TODO.md` item 3. Until its A0 arm runs, "the shipped surface
throws away N points" has no N.

Both surface runs also predate this document's own gates. `--withhold-source` and
tool-call adoption recording landed in `3feacd5`, three days after `a9a135b`; neither
run records `treatment_tool_calls`, and neither has a baseline arm. They are the only
figures here not subject to §What is measured's protocol, and they appear in this
section rather than that one for exactly that reason.

### Exactness is assumed to be what the caller wants

`goastcg/precise.go:30-31` states the result is "never less sound than CHA, only more
precise." Control's failure mode is a *sound over-approximation*. For a
refactoring-safety question ("might I break this caller?"), the model's conservative
answer is the one to act on, and the benchmark scores it wrong. The benchmark's
exact-match metric and the downstream utility function have not been shown to agree.

## Hypotheses and future experiments

A thesis that cannot be broken is not a thesis. Each hypothesis names the experiment
that would settle it, and that experiment's status. None of these is a forecast; the
point is the experiment, not the guess.

1. **Composition compounds: the deep-N test.** *(Pre-registered, unrun. The decisive
   experiment.)* Fitting `(1−ε)^N` to the clean depth ladder gives ε = 0.0019. Held at
   f80 and answer size 48, that model entails control accuracy of **75.2% at N=150**
   and **46.7% at N=400**. The experiment runs those two rungs.
   - If control tracks the curve, **composition is vindicated** and the d47 null was
     a power failure, not a refutation.
   - If control holds above 95% at N=150, the compounding model is **wrong**, "lookups
     are free at any depth" wins, and arm 1's motivation weakens further.
   - Either way, "you cannot compound an error rate of zero" is already retracted: the
     measured ε is 0.002, not 0.

2. **Arm 1 has never been tested.** *(Open. Needs a new corpus. The highest-value
   experiment the project could run next.)* A test of arm 1 must satisfy four
   conditions simultaneously: a fallible-computation task **on Go source**, **with
   wile-goast**, **against a non-algebraic tool baseline**, **on the current model**.
   No existing corpus meets all four, so the corpus has to be built. If the
   tool does not beat control under those four conditions, arm 1 is dead and with it
   the strongest motivation for the algebra.

   TODO(aalpar): what is the task? A lattice deep enough that the answer is not
   readable off the page also makes the answer *long*, re-introducing the file-size /
   answer-size confound that already muddies the f≥192 result. A join small enough to
   avoid that confound may be one where syntax tells the truth and ε is tiny. Name a
   task that stays fallible at short answer length.

3. **Flat APIs suffice.** *(Untested at the level stated.)* If an agent with grep +
   read-file + a long context matches wile-goast on **cross-layer** queries, the
   pre-composition claim is wrong. Note that the reachability null did **not** test
   this: reachability is a *single-layer* (call-graph) query. No cross-layer query has
   been benchmarked. The experiment is to build one. This hypothesis is live and
   unaddressed.

4. **Logic beats algebra.** *(Untested. Now armed.)* If the same questions are
   answered more accurately by a Datalog agent interface (CodeQL over MCP, Doop as a
   tool), **the algebraic bet is lost.** Per §The algebraic bet, adopting logic in
   response is conceding, not hedging. Under the stipulated definition this is now a
   real test rather than a distinction the project can absorb.

5. **The benchmark is circular.** *(Currently a live concern, not a hypothetical.)*
   The dispatch corpus is `t := []func(){...}; t[0]()`. `goastcg/precise.go:26-27`
   describes the tool as resolving "a constant index into a literal `[]func()`." The
   tool's docstring is a description of the benchmark. The stated defense (hardness
   must arise from token budget, state tracking, or combinatorial structure) licenses
   **none** of this corpus's hardness, which arises from "syntax disagrees with
   semantics," a criterion formulated *after* the result. The escape is experiment 6.

6. **The criterion generalizes beyond one pattern: interface dispatch.** *(Designed,
   unrun. `LLMAccuracy/TODO.md` item 2.)* `'precise` explicitly **does not handle**
   interface dispatch (`goastcg/precise.go:66` returns `nil` for `IsInvoke()`), so
   `vta` is the tightest available and it is a *sound over-approximation*. Three
   outcomes, and the middle one is the most valuable:
   - `vta` is exact on the corpus, treatment ≈100%: criterion holds.
   - `vta` over-approximates and the model reports the bound: **the −33% anchoring
     trap reproduces**, the tool *hurts*, and a must/may split becomes load-bearing on
     evidence rather than taste.
   - Control at ceiling unaided: **the criterion is falsified.** Say so.

7. **Uplift does not generalize beyond synthetic corpora.** *(Antecedent already half
   satisfied.)* Every corpus in this document is machine-generated. If the primitives
   improve accuracy on generated Go but not on Kubernetes, Prometheus, or etcd, the
   substrate is doing decorative work. Nothing yet distinguishes these cases.

8. **ε depends on the model, not the task.** *(Unaddressed, and it has an expiry
   date.)* Both arms are error-rate claims, not information-theoretic ones: every fact
   the tool computes is already present in the source the model was given. If model
   capability drives ε toward zero, both arms close, and ε is already small (0.002).
   **Nothing in the current criterion is durable under model improvement.** The
   experiment is to hold every corpus fixed and re-run it on each model generation,
   watching whether the margins decay. If a durable case exists (a whole-program fact
   no local reading can contain, at any capability), that is the case the thesis
   wants, and it is not the one currently argued.

9. **The prompt surface dominates the capability.** *(Designed, unrun.
   `LLMAccuracy/TODO.md` item 3. This is the demoted form of a claim this document
   previously asserted.)* Four arms on `problems_dispatch_subtle.json`, where the tool
   provably wins, so any shortfall is attributable to the surface and nothing else:
   **A0**, the shipped surface before wile-goast `16653eb`; **A1**, after it; **B**, A1 with
   `reference` force-loaded, separating *"was the cheatsheet read"* from *"is the
   cheatsheet any good"*; **C**, the hand-tuned `PRECISE_TREATMENT_PROMPT`, the known
   100% ceiling. A1 → C is the headroom purchasable with zero new analysis capability,
   and it is the number that decides how much MCP work is worth.
   - If A0 lands near control, the surface, not the analysis, is what this project has
     been measuring, and every margin in §What is measured is an artifact of a prompt
     no user sees.
   - If A1 ≈ C, the cheatsheet fix alone closed the gap. Ship it and stop.
   - Note what makes this cheap to get wrong: the −33/+27 pair (§The external-validity
     hole) is **not** evidence for this hypothesis. It ran on a different corpus, at
     n=15, with a rigged tool, and its swing came from a verification mandate rather
     than from a tool description.

TODO(aalpar): Experiments 1, 2, and 6 are now specified sharply enough to run. What
else would make you abandon the thesis rather than revise it? The distinction between
"my explanation was wrong" and "my conclusion was wrong" is the one this document has
now made twice; name the finding that would collapse both at once.

## Scope of validation

**Nothing in [What is measured](#what-is-measured) comes from either corpus below.**
Every reported result was obtained on machine-generated Go
(`generate_reachability.py`). The following is a *plan*, not a record.

- **Wile itself.** ~40k lines of Go, self-contained, well-understood by the author.
  Intended as a known-answer corpus.
- **Kubernetes universe.** Large, heterogeneous, externally reviewed. Intended as a
  generalization test.

Until one of these produces a number, hypothesis 7 cannot be evaluated, and the
external validity of every figure above is unestablished.

TODO(aalpar): Kubernetes has particular conventions (generated code, operator
pattern, heavy interface use) that may bias findings. One more corpus of different
flavor (etcd, Prometheus, Vitess, Docker) would strengthen the generalization claim.
Which do you want to commit to, and why?

## Open questions

- **Why does the tool hold at scale?** The f≥192 result (treatment 24/24, control
  19/24) is confounded across file size, answer size, and depth. Which one drives it?
  A corpus that varies file size with **answer size and depth pinned** would separate
  distractor-count effects from output-length effects, and would also settle whether
  the `O(answer)` cost conjecture survives an answer set that grows with the source.

- **Where does the algebra stop earning its keep?** Not every question benefits from
  lattice-theoretic framing. The project has **no measurement at all** on this axis,
  for or against. The first honest step is experiment 2's corpus.

- **Do the primitives cover the interesting failure surface?** The library grew
  organically from specific questions. A systematic account (here are the cross-layer
  query shapes, here are the tools that compose to answer them) would make the
  coverage claim auditable. Note that `(wile algebra cfl)` exists because the
  semiring family *did not* cover a query shape that mattered.

- **What's the right unit for the interface?** Primitives vs. high-level analyses
  (`recommend-split`, `cross-boundary-concepts`) vs. prompts that drive multi-step
  analyses. Per §The external-validity hole, this may not be a taste question: on one
  n=15 corpus, prompt wording alone moved the same tool 60 points. Whether that
  generalizes to the shipped surface is hypothesis 9.

TODO(aalpar): Other open questions you actually want answered. These should be the
ones you're uncertain about, not the ones you already know the answer to.

## Superseded claims

Retained because the project's third stated contribution is measurement-first
development, and this record is the evidence for it.

Fourteen claims below, and one row that is bookkeeping rather than a claim. Not every
claim fell to a measurement, and the split is itself the finding: **3** were retired
by new numbers, **1** by re-fitting numbers already in hand, **5** by auditing the
harness and the raw results against the prose, **1** as a direct consequence of another
row, **1** by the absence of any discriminating result, and **3** by reasoning that
needed no data at all.

The last group is the uncomfortable one: a claim that reasoning alone can kill should
not have survived long enough to be measured. The five-row audit group is the
instructive one. Not one of those claims was wrong about a number. They were wrong
about *which run the number came from*, *what it was a number of*, or *what it was
being compared against*: failures of provenance, not of measurement, and invisible to
anyone reading the prose alone.

Hashes in *Where it stood* are this repository's; hashes in *What retired it* are
`LLMAccuracy`'s.

| Claim | Where it stood | What retired it |
|---|---|---|
| Flat APIs "force the agent to compose those facts at inference time, under a token budget, **at which it is unreliable**." | Short form, `930923a` | Control chains 47 hops at 91.7%. Overbroad as stated. |
| "The reachability differentiation claim is **falsified**." | Amendment, `70e2fc8` | Half right. The *tool-value* null at f80 stands (margin 0.0 vs grep). The *composition* reading does not: ε=0.0019 > 0, and `(1−ε)^N` fits both rungs. Underpowered, not falsified. |
| "Margin over conventional tooling = 0.0%" presented beside a control column. | Amendment table, `70e2fc8` | `report_reach.py:59` computes margin as treatment − **grep-baseline**. vs control it is +1.7 (d4) and +8.3 (d47, p=0.057). The two denominators were mixed within one section. |
| "Margin 0.0 **at every depth.**" | Amendment, `70e2fc8` | Only two rungs ran three arms: d=4 and d=47. The `{1,2,4,8,16}` ladder saturated at 100% on all three arms and was recorded as a **failed calibration**, not a null. |
| "The agent composes 47 hops **essentially perfectly**." | Amendment, `70e2fc8` | 55/60 = 91.7%. |
| "You cannot compound an error rate of zero." | `docs/when-tools-win.md`, `70e2fc8` | ε ≈ 0.0019, not 0. The line was a clean idealization of a measurement that contradicts it. |
| The whole of `docs/when-tools-win.md`. | Deleted 2026-07-13 | Its title question's answer ("it was never about length"), its pivot, its criterion's "efficiency only, at any depth" clause, and its Müller-Lyer analogy all encoded ε = 0. Not patchable; the assertion *was* the thesis. Recover at `70e2fc8`. |
| — what was salvaged from it | — | The `(1−ε)^N` model (§The error-rate model), the constructed dispatch example and the CHA-reinvention reading (§Higher-order dispatch), the honest-denominator rule (§Cost), and the should/should-not list (§Where the criterion says…). The Müller-Lyer analogy was **not** salvaged: it presumes a perceptual system that is exact on ordinary cases, which is the retracted ε = 0 in metaphor. |
| "A crossover **must** exist at larger n." | Amendment, `d53a009` | Asserted in the sentence before "should not be asserted until measured." Requires answer size sublinear in source size; `n_reachable` = 218/384 at f384 suggests it may not be. |
| "If they become scope, the substrate is already in the box." | §Why an extensible algebraic substrate, `930923a` | A bet that absorbs its own falsifier is not a bet. Withdrawn; hypothesis 4 re-armed. |
| Context-free reachability listed as an algebraic family. | §The algebraic bet, the family list | The same bullet states it "needs a grammar, not a weight." Category error; the family is retained in the list and now counts against the bet. |
| Title: "Algebraic Composition as Substrate." | Title, through `d53a009` | No result discriminates algebra from any other substrate. The evidenced claim is pre-composition. |
| "Every accuracy figure in this document was obtained with `PRECISE_TREATMENT_PROMPT`." | §The external-validity hole | Only the dispatch figure was: `'precise` appears in the treatment transcripts of `results_dispatch_{subtle,smoke}.json` and nowhere else. The depth ladder's transcripts name no algorithm at all (`TREATMENT_PROMPT`, `evaluate_reachability.py:49`); the scale corpus's invoke `'cha`. The scope qualifier was lost in transcription from `LLMAccuracy/TODO.md:135`, which says "Every result we have (+35 pts, 100% on dispatch)." |
| "Same task, same tool, same model; only the prompt changes," said of a three-row table. | §The external-validity hole | True of the −33/+27 rows (`results_reach_sub{,_verify}.json`: same 15 ids, same ground truth, same control results). The 100% row is `results_dispatch_subtle.json`: a different corpus (f50, n=60), a different tool (`'precise`, which both other arms withhold), and an *absolute accuracy* where the others are *margins*. Mixed denominators, again. |
| "A ~65-point swing … **the prompt surface dominates every capability result in this document**." | §The external-validity hole | The swing is **60.0** points (`a9a135b` says so). "Dominates" set a paired within-treatment swing (n=15, f8–f24, `'vta`) against a treatment−control margin (n=60, f50, `'precise`); the two are not commensurable, and the 100% it cited is the same run as the 35 it was compared to. Demoted to hypothesis 9. |

## Related documents

- `README.md` — project overview and refactoring-session walkthrough
- `docs/PRIMITIVES.md` — complete API reference for every layer, plus the belief DSL
- `docs/LIBRARIES.md` — the higher-level analysis libraries: FCA, path-algebra,
  `unify` (SSA equivalence), dataflow, taint, ifds
- `BIBLIOGRAPHY.md` — citations for the individual layers
- `LLMAccuracy/TODO.md` — pre-registered runs: cost crossover, interface dispatch,
  surface A/B, abstention
