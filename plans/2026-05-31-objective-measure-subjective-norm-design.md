# Objective Measure, Subjective Norm — Design Note

> **Status:** Design note (2026-05-31). General-architecture statement, not an
> implementation plan. No new module proposed — it names a pattern that two
> existing draft plans already instantiate.
> **Scope:** The division of labor between wile-goast (symbolic measurement)
> and an LLM (subjective categorization) across the analysis suite.
> **Spec source:** Conversation 2026-05-31 on conceptual/organizational
> refactoring (colocation, naming uniformity), generalized from the user's
> statement of wile-goast's reason for existing.

## Why this note exists

Two draft plans —
[`2026-04-19-llm-concept-filter-design.md`](2026-04-19-llm-concept-filter-design.md)
and
[`2026-04-20-receiver-parameter-asymmetry-design.md`](2026-04-20-receiver-parameter-asymmetry-design.md)
— look unrelated. One is an FCA post-filter; the other is a method-shape
belief. They are the **same architecture run in opposite order**. This note
states that architecture once, so the relationship is explicit rather than
latent in two separate documents.

## The thesis

Refactorings split by what kind of result they achieve:

- **Objective refactorings** produce a *measured, quantifiable* result. Two
  functions are algebraically equivalent; a min-cut weight drops; instruction
  count falls. The verdict is a fact in the program, reproducible and
  exhaustive. wile-goast's existing suite (AST diff, SSA equivalence, FCA
  min-cut, path algebra, the optimization catalog in `refactorings/`) lives
  here.
- **Subjective refactorings** produce a *qualitative* result. This method
  belongs with that type; this converter should be named `ToX`; these eight
  functions form one concept. There is **no ground truth in the program** —
  the result is a judgment against a *norm*, and the norm is chosen, not
  derived.

wile-goast exists to combine the two: use the LLM's ability to **categorize on
subjective conditions** with wile-goast's ability to **measure deviation from
that category**. The measures come from a mixture of wile-goast (cheap,
exhaustive, reproducible) and the LLM's inspection (semantic, fuzzy,
expensive).

## The two operators

Every subjective refactoring decomposes into two operators with different
epistemics:

| Operator | What it does | Epistemics | Who can hold it |
|----------|--------------|------------|-----------------|
| **C — categorize** | Fix the equivalence class / norm. "These belong together." "This is the conversion family." | Subjective — a *choice*, not a derivation | Human (design time) or LLM (runtime) |
| **M — measure** | Count deviation from the norm. "6 of 8 use the `To` prefix; 2 don't." | Objective — a *fact*, given C | wile-goast (AST/SSA/CFG/FCA/lint) |

The system does **not** measure subjectivity. It **freezes a subjective choice
(C) into an objective measure (M), then runs M mechanically.** The subjectivity
is paid once, when C is fixed; everything after is arithmetic.

This maps directly onto the belief DSL's existing **site-selector /
property-checker** split. Engler's "Bugs as Deviant Behavior" derives C
*statistically from syntax* — the majority of `Lock` sites pair with `Unlock`.
The thesis upgrades C to be *semantic*: the LLM defines the class that syntax
can't, and M (wile-goast) measures deviation over the LLM's class instead of
over raw tokens.

## The guard against collapse

The split earns its keep only where **both** operators are load-bearing:

1. C genuinely needs semantics the structure can't supply, **and**
2. M is genuinely mechanical *once C is fixed*.

If the LLM does C *and* you trust it to eyeball the deviations, wile-goast adds
nothing — you have asked the LLM twice. The architecture pays off precisely
where the LLM is good at the fuzzy boundary but **bad, expensive, or
non-reproducible at exhaustive measurement**, and wile-goast is the inverse.

Naming uniformity is the clean case: the LLM labels the conversion family
*once* (C); wile-goast counts the prefix distribution across all N sites and
names the outliers (M). The LLM never looks at all N; the count is identical on
every run.

## Two placements, two existing instantiations

C and M can run in either order, and C can be held by a human (frozen at design
time) or an LLM (live at runtime). The two draft plans occupy opposite cells:

| Plan | C — who / when | M — what | Order |
|------|----------------|----------|-------|
| `receiver-parameter-asymmetry` | **Human, design time.** A person named the anti-pattern; it is frozen in the L1 rule (`\|RR\|=1 ∧ \|RW\|=0 ∧ \|Params\|≥1 ∧ ¬IM ∧ ¬MV`). | wile-goast SSA: count receiver reads/writes/params; L2 joint-use via same-instruction flow. | **C → M** |
| `llm-concept-filter` | **LLM, runtime.** FCA proposes every structurally-dense concept; the LLM judges which are semantically coherent. Stochastic. | wile-goast FCA: concept lattice + structural filters produce the candidate set M operates on. | **M → C** |

- `receiver-parameter-asymmetry` is the **frozen** end: subjectivity paid once,
  by a human, encoded as a deterministic rule. Fully reproducible thereafter.
- `llm-concept-filter` is the **live** end: subjectivity paid per run, by the
  LLM, advisory by default (`annotate-only`).

A third case from the originating conversation — **naming uniformity** (`To` /
`From` / `As` / `Into`) — sits between them: one cheap M finds candidate sites
(functions returning an owned `T`), the LLM applies C once ("these are
conversions"), then M measures the prefix distribution. Order **(M) → C → M**.

## The lifecycle is the conveyor between the two ends

The frozen end and the live end are not separate designs — they are connected
by the **discover → review → commit → enforce** lifecycle, which is already
built (`with-belief-scope`, `load-committed-beliefs`, `suppress-known`,
`emit-beliefs`, shipped 2026-04-23):

1. **discover** — the LLM proposes a subjective category (live end).
2. **review** — a human inspects it. This gate is where a bad category is
   caught *before* it hardens into a measure.
3. **commit** — the approved category is emitted as a `define-belief` artifact
   (frozen end). Subjectivity is now paid; the measure is deterministic.
4. **enforce** — the committed belief runs as a reproducible M, and
   `suppress-known` keeps re-discovery from resurfacing it.

So `llm-concept-filter` (live) and `receiver-parameter-asymmetry` (frozen) are
the two ends of one conveyor; the suppression lifecycle is the belt that moves
a category from the first to the second.

## The failure mode

M is only as objective as C is clean. **A garbage category yields a precise
measurement of garbage.** If the LLM's "conversion" class wrongly admits three
constructors, wile-goast will faithfully and reproducibly report them as naming
deviants — and the precision of the number disguises the rot in the premise.
The threshold knob couples to category quality: a noisy class inflates apparent
deviation.

The mitigation is structural, not a tuning parameter: the **review gate**
between discover and commit. It is not optional polish — it is the defense that
keeps a stochastic C from silently corrupting a deterministic M. The
`annotate-only` default in `llm-concept-filter` serves the same purpose: a bad
C cannot cull data, only annotate it.

## Candor on prior art

The "statistical deviation from a learned norm" half is well-trodden. The
contribution is narrow and should be stated as such.

| Prior art | What it did | What it shares |
|-----------|-------------|----------------|
| Engler et al., *Bugs as Deviant Behavior* (2001) | Norm from statistical majority over syntax | The belief DSL's basis; M-over-syntactic-C |
| Allamanis et al., *Learning Natural Coding Conventions* (NATURALIZE, 2014) | Learns naming/formatting conventions from a corpus; flags deviations | **Directly the naming-uniformity case**, via an n-gram LM over tokens |
| Ammons et al., *Mining Specifications* (2002) | Infer the norm, then check against it | The discover→enforce shape |
| Neuro-symbolic generate-and-filter (CodeQL+LLM triage, RAG rerank, LLM proof-step selection) | Symbolic candidates, LLM filter | The M→C placement (see `llm-concept-filter` §What's New) |

What is plausibly new, stated minimally:

1. **C is semantic, not lexical.** NATURALIZE clusters by token context; the
   LLM clusters by *meaning*, which lets the site set be defined across the
   type/SSA layers, not just the token stream.
2. **C and M are different systems with different failure modes.** NATURALIZE's
   evidence is the same LM that proposes. Here the proposer (LLM) and the
   measurer (wile-goast) are independent — which is what makes M a check on C
   rather than the model marking its own homework.

Neither point is a new algorithm. The contribution is the *placement* and the
*lifecycle that freezes C into M*, not a novel measurement technique.

## Open questions

All four are resolved or scoped below. The resolution sections follow in
dependency order (the codomain result #1 underpins #2's filter, which #3 and #4
consume), not numeric order — each carries a `Resolves #N` callout.

| # | Question | Status | One-line verdict |
|---|----------|--------|------------------|
| 1 | Common deviation metric across families? | Resolved | Metric stays per-family; the shared thing is the verdict **codomain** (`eq?`-symbol) + the one aggregator. Deliverable is a *constraint* (discretize to a small ordered category set), not an abstraction. → [§Resolution #1](#resolution-the-shared-interface-is-the-verdict-codomain-not-the-metric) |
| 2 | Threshold semantics under a stochastic C? | Resolved | Frozen-C is the `confidence = 1` case of live-C. Keep the ratio; add a `c_min` membership pre-filter that vanishes for frozen-C; surface the dropped count. → [§Resolution #2](#resolution-threshold-semantics-under-a-stochastic-c) |
| 3 | Where does declaration ordering go? | Resolved | Bifurcates on this doc's own axis: dependency order → objective catalog (M-only); conventional order → belief architecture (C→M), blocked on a container selector + #2's `c_min`. → [§Resolution #3](#resolution-where-declaration-ordering-goes) |
| 4 | Review-gate cost? | Scoped, not closed | Empirical — needs one real `discover_beliefs` run. Commit to measuring `survivors-after-suppression` and adding confidence-ranked emission; build no batch-review machinery speculatively. → [§Resolution #4](#resolution-review-gate-cost-is-empirical-instrument-before-optimizing) |

## Resolution: threshold semantics under a stochastic C

> Resolves open question #2.

### The mechanism today

Two steps in `lib/wile/goast/belief.scm`. The statistical comparison
(~line 619) finds the majority category and its share:

```scheme
(ratio (/ maj-count total))                       ; exact rational
(list name maj-cat ratio total adherence deviations)
```

The per-site runner (~line 754) then gates strong/weak against the configured
threshold:

```scheme
(min-adh   (belief-min-adherence belief))         ; (car  threshold)
(min-n     (belief-min-sites     belief))         ; (cadr threshold)
(is-strong (and (>= ratio min-adh) (>= total min-n)))
(status    (if is-strong 'strong 'weak))
```

The ratio measures **agreement among verdicts within the selected sites**. It
presumes the site set is ground truth: every selected site is treated as a
full member of the class. The per-site verdict (M) is deterministic; the only
uncertainty the formula models is conformance (`maj-count / total`), not
membership.

### Why a stochastic C breaks the assumption — not the formula

The `threshold` form is correct as written. What a live-C changes is *upstream*
of it: site **membership** becomes uncertain. A site the LLM wrongly admits to
the class still casts a full verdict — inflating adherence if it lands in the
dominant group, inflating deviation if it lands in a minority group. The noise
is in C (selection), not in M (the ratio). So the fix belongs at selection
time, leaving the ratio untouched.

### Decision

**Frozen-C is the `confidence = 1` degenerate case of live-C.** One
computation serves both (satisfies the "verify by substitution" rule):

1. **Keep `maj-count / total` unchanged.** Exact rational preserved for
   `emit-beliefs` review and serialization.
2. **Add a per-site membership confidence `c_i ∈ [0,1]` and a `c_min`
   pre-filter** applied to `sites` *before* the ratio. For frozen-C (syntactic
   selector) every `c_i = 1`, so the filter is a no-op and current semantics is
   recovered exactly. The frozen/live difference is a vanishing preprocessing
   step, not a second formula.
3. **Report the dropped count** as `category-uncertain` alongside `ratio`, so
   category noise stays visible instead of folding into the adherence number
   (the note's "a precise M must not disguise a noisy C" defense; the project's
   "no silent caps" discipline). Prior art: aggregate beliefs already carry
   `(confidence . HIGH)`; this extends a confidence channel to the per-site
   path rather than inventing one.

### Interaction with suppression

No conflict. `suppress-known` matches structurally on `sites-expr` /
`expect-expr` and *ignores thresholds and ratios* (per `BELIEF-DSL.md`). Adding
`c_min` as a threshold parameter is therefore orthogonal to suppression — a
committed belief and its discovery twin still match regardless of `c_min`.

### Open sub-choice (left for implementation)

Hard filter vs. soft weighting is a genuine fork, deferred to the impl plan:

| | **A — hard filter (recommended)** | **B — confidence weighting** |
|---|---|---|
| ratio | `\|dominant ∩ kept\| / \|kept\|` after dropping `c_i < c_min` | `Σ_{dominant} c_i / Σ c_i` |
| min-sites gate | integer kept count | `Σ c_i` (fractional effective sample) |
| frozen reduction | clean (filter no-op) | clean (all `c_i = 1`) |
| preserves gradation | no | yes |
| serialization | exact rational | forces floats |

A fits the codebase (exact rationals, integer `min-sites`, single formula after
a vanishing filter); B preserves gradation at the cost of fractional sample
sizes that complicate the `min-sites` gate and emit serialization. **Recommend
A**; the `confidence-gate` predicate (~6 lines) is left as the implementation
author's call — it is the one place a real trade-off is exercised.

## Resolution: where declaration ordering goes

> Resolves open question #3.

### The decision

**Ordering is admitted, but bifurcated — and the cut runs exactly along this
document's own objective/subjective seam.** That the family splits cleanly on
the doc's own axis is corroboration the axis is real, not a sign ordering is a
bad fit.

| Sub-family | Example | Ground truth? | Home | Operators |
|------------|---------|---------------|------|-----------|
| **Dependency order** | "foundational types first"; const declared before use | **Yes** — the use-def DAG | Objective catalog (`refactorings/`) — *leaves this architecture* | M only |
| **Conventional order** | constructor before methods; exported before unexported; getters grouped | **No** — a chosen convention | Belief architecture — *stays* | C → M |

### Dependency order is objective; it leaves

"Foundational types first" is not a convention — it is a **linear extension of
the dependency DAG**. Type `T` used by type `U` *should* precede `U`, and the
graph says so. The check is: *is the file's declaration order a topological
sort of its intra-file use-def graph?* No norm is chosen; M alone decides.

This is the intra-file sibling of `break-import-cycle` (the cross-package
version of the same DAG-linearization idea, already in the catalog). It belongs
in `refactorings/`, detector status **could-build** — AST preserves declaration
order (the parse is an ordered list), and `go-typecheck-package` supplies the
use-def edges. It does **not** belong in the C/M architecture, because there is
no C to pay.

### Conventional order is subjective — and it *does* fit, after a granularity fix

The open question's objection ("sequence position is not a deviation from a
majority") is a **granularity error**, not a disqualifier. It implicitly made
each *declaration* a site and its *position* the verdict — and positions have
no majority. The fix is to move up one level:

- **Site** = the **container** (type-cluster or file), not the declaration.
- **Verdict** = the container's **ordering signature** — a canonicalized
  permutation-class, e.g. `fields → constructor → exported-methods →
  unexported-methods`.
- **Deviation** = minority signatures, measured by the *unchanged*
  `maj-count / total` ratio.

This is the **same move already in the codebase**: the `ordered` checker
(`belief-checkers.scm:96`) casts an SSA dominance *relation* between two
calls into a per-site *category* (`a-dominates-b` / `b-dominates-a` / …).
Conventional ordering casts a declaration *permutation* into a per-container
*category*. Relation-as-verdict is prior art here; this just raises the
granularity from intra-procedural to container-level.

### What it costs (the real gap to name)

Two pieces are genuinely missing — this is not free:

1. **A container-granular site selector** (`types-in` / `files-in`). Every
   current selector — `functions-matching` (`belief.scm:285`), `callers-of`
   (`:304`), `methods-of` (`:324`), `all-functions-in` (`:333`) — is
   *function*-granular. A site whose unit is a type-cluster or file is new
   machinery.
2. **A signature canonicalizer** — the function that decides which permutations
   count as "the same order." **This is the C operator.** It is where
   subjectivity enters, so it sits at one of the two ends from §The two
   operators: a frozen human rule (committed) or an LLM verdict (discovery).
   Same conveyor as everything else in this note.

### Honest caveat: low signal in Go specifically

`gofmt` deliberately does not reorder declarations, and Go has no canonical
declaration order. So conventional-order beliefs will run **high category
noise** in Go — many containers will have idiosyncratic, blameless orderings.
This makes ordering the family **most exposed** to the membership/category
noise that [open question #2's resolution](#resolution-threshold-semantics-under-a-stochastic-c)
addresses: it is the first natural consumer of the `c_min` confidence
pre-filter. A conventional-order belief with no confidence gate would report a
precise ratio over a noisy class — exactly the failure mode the review gate and
`c_min` exist to catch. Recommend shipping conventional-order beliefs *only*
once the `c_min` pre-filter from #2 exists.

### Net

- Dependency order → **objective catalog**, M-only, could-build. Out of this
  architecture.
- Conventional order → **stays in the belief architecture**, C → M, blocked on
  (a) a container-granular selector and (b) the #2 `c_min` pre-filter. Lower
  priority than naming-uniformity (open question #1's clean case) because of
  Go's intrinsically high ordering noise.

## Resolution: the shared interface is the verdict codomain, not the metric

> Resolves open question #1.

### The question was mis-posed

"Is there a common deviation *metric* across families?" assumes the metric is
the thing that wants unifying. It isn't. The metric is intrinsically
per-family — measuring naming conformance and measuring colocation are
different acts, and forcing them through one numeric type would be the
accidental-unification the project's refactoring rules warn against (same
nouns, not same verbs). The metric *should* stay independent.

What is already shared — and was shared before this note existed — is one level
up: the **codomain** every metric reports into.

### What the aggregator actually consumes

`evaluate-belief` (`belief.scm:598`) maps each site to a `(site . category)`
pair via the checker (`expect-fn`, `belief.scm:606`), normalizing only `#t`/`#f`
to `present`/`absent` (`belief.scm:610-612`). `majority-category`
(`belief.scm:615`) picks the mode; adherence/deviation split by
`(eq? (cdr p) maj-cat)` (`belief.scm:621,624`). The aggregator's entire
contract with M is therefore:

> **a verdict that is a symbol, comparable by `eq?`.**

Nothing numeric. Every existing checker already honors this — `paired-with` →
`paired-defer`/`unpaired`, `ordered` → `a-dominates-b`/…, `contains-call` →
`present`/`absent`. The "common interface" the question asked for is the
**checker contract `(site ctx) → category-symbol`**, and it is load-bearing
today.

### Decision

**One interface (existing), one aggregator (existing), and a discipline: each
family discretizes its metric to a small ordered finite category set.** No
metric-abstraction layer is built.

| Family | Per-family metric (the checker — independent) | Discretized verdict (shared codomain) |
|--------|-----------------------------------------------|---------------------------------------|
| Naming | prefix / signature match against the LLM-named class | `to-form` / `from-form` / `as-form` / `none` |
| Colocation | file-or-package membership vs. the type's home | `colocated` / `separated` |
| Conventional order (#3) | canonicalized permutation class of the container | one symbol per signature |

Each row's middle column is a *different* M — that asymmetry is real and kept.
The right column is the *same* codomain — that's the unification, and it
already exists.

### Why discretize rather than add a continuous aggregator

The verify-by-substitution test (the project's own rule): can the categorical
aggregator replace a continuous one *for the action this system drives*? The
action is binary — refactor this site or not. A three-bucket discretization
(`conforms` / `near` / `far`, e.g. edit-distance 0 / 1 / ≥2 to the canonical
name) carries every bit of signal the binary decision consumes. The extra
resolution of a raw distance does not change the verdict, so by substitution
the continuous aggregator is unnecessary. This also keeps `emit-beliefs`, the
`maj-count / total` ratio, the `c_min` filter from #2, and `suppress-known` all
working unchanged — discretization buys reuse of the entire downstream.

It also obeys "prefer broad predicates over specific values": `≥2` as one
bucket, not a continuum.

### The one deferred exception (do not build speculatively)

`eq?`-majority assumes a **unimodal** distribution — one norm, a minority of
deviants. Discretizing a genuinely **bimodal** metric (two legitimate
sub-conventions, e.g. a package that deliberately runs two naming schemes) into
buckets would let majority-vote crown one mode and mislabel the other half as
deviant. That is the sole case where a continuous outlier aggregator (median ±
k·MAD, deviants = outliers) is more honest than the mode.

It is **deferred, not adopted**: build it only if a real run produces a
demonstrably bimodal metric, and even then add it as a second named aggregator
selectable per belief — never as a replacement. Until a bimodal case is
observed, adding it would be speculative machinery for a distribution shape not
yet seen. Detecting bimodality is itself the trigger: a discretized belief
whose deviation set is large *and* internally homogeneous (the "deviants" all
share one non-majority category) is the signal to revisit.

### Net

The families stay independent exactly where they should — the checker (M) — and
share exactly what already exists — the verdict codomain and the one
aggregator. The deliverable of #1 is a **constraint on new checkers** ("return
a small ordered category set"), not a new abstraction. Zero new machinery;
naming-uniformity (#1's clean case from the originating conversation) can ship
on the existing aggregator the moment its checker exists.

## Resolution: review-gate cost is empirical; instrument before optimizing

> Scopes open question #4. Unlike #1–#3, this one is **not closeable on paper** —
> it asks for a number (discovery volume at which a human gate saturates) that
> only a real run produces. The resolution is a measurement commitment and one
> pre-identified lever, not an answer.

### Why this one resists a paper answer

#1–#3 were design questions: each had a right shape derivable from the existing
code and the architecture's own axis. #4 is an *operational* question — "at what
volume does the human bottleneck?" — whose answer is a property of a particular
codebase, a particular reviewer, and the precision of live-C discovery on that
codebase. None of those exist until `discover_beliefs` runs on something large.
Answering it by reasoning would be the unsupported-premise trap the project's
candor rule warns against: inventing a threshold to feel resolved.

### The honest version of the question

The gate's load is not "beliefs discovered." Two filters already cut it before a
human sees anything:

```
review-load  =  discovered
             −  weak / no-sites / error      (emit-beliefs skips; belief.scm:810,814,817)
             −  structurally-committed        (suppress-known; belief.scm:1065)
             =  survivors-after-suppression
```

So the measurable quantity is **`survivors-after-suppression` per run**, and its
*trend across runs* — because suppression makes this a decaying series. The
first run on a virgin package pays the full categorization tax; each subsequent
run only surfaces what changed since the last commit. The bottleneck question is
really: **does the first-run spike exceed a single review sitting, and does the
steady-state tail stay near zero?**

### What is already built vs. the one missing lever

| Conveyor control | Status |
|------------------|--------|
| Status filter (drop weak/no-sites/error) | **Exists** — `emit-beliefs`, `belief.scm:810-817` |
| Structural suppression (drop committed) | **Exists** — `suppress-known`, `belief.scm:1065` |
| `c_min` category-confidence pre-filter (#2) | Specified in #2; cuts noisy-category survivors before review |
| **Confidence-ranked emission** | **Missing** — `emit-beliefs` walks results in *registration order* (`belief.scm:800`), not ranked |

The single cheap lever the conveyor still lacks is **ordering the emitted
survivors by confidence** (ratio × `total`, or the #2 confidence channel), so a
human reviewing top-down hits the highest-signal beliefs first and can stop at a
budget without missing the best ones. This is a sort over `results` inside
`emit-beliefs` — a few lines, no new data, no schema change. It does not reduce
*total* load; it makes a **partial** review (stop after N) maximally valuable,
which is the actual mitigation when volume exceeds one sitting.

### Decision

1. **Do not build batch-review machinery now.** Triage UIs, sampling, auto-commit
   thresholds — all are speculative until a real `survivors-after-suppression`
   curve exists. Building them blind would optimize a bottleneck not yet
   observed.
2. **Instrument the conveyor to emit the number.** `run-beliefs` /
   `discover_beliefs` should report `survivors-after-suppression` (and the
   pre-suppression count) per run. This is the "no silent caps" discipline:
   the conveyor must say how much it handed the human, not just hand it.
3. **Add confidence-ranked emission** as the one pre-justified lever — it is
   cheap, helps regardless of the eventual curve, and is the prerequisite for
   *any* later "review the top N" policy.
4. **Set the trigger for revisiting.** If a first run's
   `survivors-after-suppression` exceeds what one reviewer clears in a sitting
   (a real number, measured, not assumed), *then* reopen this question with
   data and choose among sampling / stricter `c_min` / auto-commit of
   high-confidence survivors. Until then, #4 stays scoped, not solved.

### Net

The conveyor already sheds load two ways (status + suppression) and decays over
runs via suppression. The design owes one cheap addition now (ranked emission),
one measurement now (survivor count), and **no speculative machinery**. The
volume threshold is deferred to the first real run — naming it before then would
be a fabricated premise, not a resolution.

## Relation to other plans

- [`2026-04-19-llm-concept-filter-design.md`](2026-04-19-llm-concept-filter-design.md)
  — the **M → C, live** instantiation.
- [`2026-04-20-receiver-parameter-asymmetry-design.md`](2026-04-20-receiver-parameter-asymmetry-design.md)
  — the **C → M, frozen** instantiation.
- [`2026-04-17-fca-duplicate-detection-design.md`](2026-04-17-fca-duplicate-detection-design.md)
  — pair-level LLM judge at Phase 5: another M → C placement, one level below
  concept granularity.
- [`BELIEF-DSL.md`](BELIEF-DSL.md) — the site-selector/property-checker split
  that C/M map onto; the suppression lifecycle that is the conveyor.
- [`2026-05-31-refactoring-catalog-orthogonal-extensions-design.md`](2026-05-31-refactoring-catalog-orthogonal-extensions-design.md)
  — the objective-refactoring catalog. The subjective family discussed here is
  the axis that catalog does not cover; its detector column would read
  **belief**, not existing/could-build/out-of-scope.
