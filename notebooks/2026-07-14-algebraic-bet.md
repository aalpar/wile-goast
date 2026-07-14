---
notebook: 2026-07-14-algebraic-bet
thesis: An algebraic substrate is what makes agent-facing code analysis accurate: because composition operators are specified by equational laws the implementation must satisfy, a new query comes from instantiating an existing operator at a new carrier rather than from writing a new algorithm.
falsified-by: A Datalog agent interface (CodeQL or Doop exposed over MCP) answers the same cross-layer query corpus at an accuracy greater than or equal to wile-goast's, at n >= 60.
created: 2026-07-14
status: open
verdict:
parent: [[2026-07-14-pre-composition/E21]]
---

# The algebraic bet

Spawned from [[2026-07-14-pre-composition/E21]]. The parent thesis claims that
*pre-composing* a fact the agent would otherwise derive unreliably buys accuracy. It says
nothing about **how** that fact is computed. This notebook holds the separate, and so far
unevidenced, claim that computing it *algebraically* is what makes it work.

**The two claims are separated on purpose, and the write-back inverts.** Refuting this
notebook leaves the parent standing. The parent's only clean win (`goastcg/precise.go`,
[[2026-07-14-pre-composition/E6]]) came from code that is **not algebraic**: CHA plus an SSA
constant-index resolution pass, written directly against `golang.org/x/tools`.

**The stipulation.** A substrate is *algebraic* here iff its composition operators are
specified by equational laws the implementation is **required to satisfy** (associativity,
idempotence, absorption, distributivity of `⊗` over `⊕`, monotonicity), so that a new query
comes from instantiating an existing operator at a new carrier. Under it: semirings,
lattices, closure operators and boolean algebra are algebraic; Datalog, Doop and CodeQL are
**not** (composition is by logical inference over relations, not by operator laws); and
`goastcg/precise.go` is **not**.

The stipulation is what makes the `falsified-by` line a real test rather than a distinction
the project can absorb. **If Datalog answers these questions more accurately than algebra,
the bet is lost, and adopting Datalog is losing it, not hedging it.**

## C1 — No result discriminates algebra from any other substrate, in either direction
**When:** 2026-07-14 00:14
**Bearing:** serves [[C4]]
**Inputs:** Survey of every corpus in `~/ClaudeProjects/LLMAccuracy`: `code-restructuring/` (reachability depth ladder, scale, dispatch, interface may-union, impact, dominance, boolean) and `algebra-accuracy/` (the 90-problem gradient set).
**Description:** Establishes the state of the record before any experiment is proposed,
because the honest starting point for this notebook is not a weak result but the **absence**
of one. The question is narrow and answerable: does any run anywhere compare an algebraic
substrate against a non-algebraic one on the same task?
**Result:** **None does.** Not one corpus pits an algebraic substrate against a
non-algebraic one. The parent's measured wins and nulls all run through tools whose
algebraic status is either irrelevant to the measurement or, in the decisive case, plainly
absent: `'precise` is a hand-written `x/tools` pass. The `algebra-accuracy/` corpus tests
the *model's* accuracy on algebra problems, which is a different question entirely, and it
is not Go source. **The bet has neither a win nor a loss.** Every entry below is therefore
either an argument or an owed experiment.

## C2 — The algebra failed to cover a query that mattered, and the gap was filled by hand
**When:** 2026-07-14 00:14
**Bearing:** refutes
**Inputs:** Source inspection: `wile/pkg/stdlib/lib/wile/algebra/cfl.{sld,scm}` and its consumers, `wile-goast/lib/wile/goast/ifds.sld:18` (`(import (scheme base) (srfi 1) (wile algebra cfl))`) and `(wile goast taint)`, which builds on ifds.
**Description:** The bet's operative promise is that a new query comes from **instantiating
an existing operator at a new carrier**, not from writing a new algorithm. This entry tests
that promise against the library's own history: it asks whether every query the project
actually needed could be obtained that way. One could not, and the exception is not a corner
case. It is interprocedural valid-path reachability, which is the query shape the whole
`path-algebra` layer exists to serve.
**Result:** `(wile algebra cfl)` **exists, and it is not algebraic.** It is a
grammar-directed reachability algorithm, written by hand. The reason is structural rather
than incidental: distinguishing "reachable" from "reachable along a path that could actually
execute" requires that a return match its own call, matched call/return is a **Dyck
language**, and **no choice of semiring makes Bellman-Ford reject the unrealizable paths.**
The problem needs a *grammar*, not a *weight*. So at the one point where the algebraic
families provably could not express a query the project needed, the response was to abandon
operator-instantiation and write an algorithm. THESIS.md concedes the point in its own
words: its presence "is a *count against* the bet, not evidence for it", and listing it as a
sixth algebraic family was a category error. Note what this does **not** show: the algebra
still covers the queries it covers, and the library is not thereby wrong. What it shows is
that the bet's central promise has a known counterexample **inside the project's own
codebase**, and that Datalog, the substrate named in the `falsified-by` line, **handles this
case natively.** That is the direction of the refutation: the algebra is not merely
incomplete, it is incomplete exactly where the most direct competitor is strong.

## C3 — The operator laws are unverified, and arm 1 is what would depend on them
**When:** 2026-07-14 00:14
**Bearing:** open
**Inputs:** *Owed experiment, not designed.* Property tests or law checks over the algebra library's operators: associativity, idempotence, absorption, distributivity of `⊗` over `⊕`, monotonicity of transfer functions. Currently no such evidence is cited anywhere in the project.
**Description:** The library is a reference implementation, and its stated design rationale
is that "elegance is instrumental: the algorithms have to be legible enough that their
correctness can be verified by inspection." The assumption that rests on is worth naming:
**verifiable by inspection is not verified.** The project's third stated contribution is
measurement-first development, and this is the one component with no measurement of any kind
behind it. The exposure is specific rather than theoretical. An incorrect lattice join would
not announce itself; it would silently corrupt exactly the fixpoint and dataflow results
that constitute **arm 1**, which is already the parent's weakest arm
([[2026-07-14-pre-composition/E17]]) and the arm this notebook most needs to win. Running
[[2026-07-14-pre-composition/E17]] on an unverified algebra risks producing a number that
nobody can interpret: a loss would be indistinguishable from a bug. **This experiment
therefore ought to precede that one**, and it is cheap, since the laws are already written
down as the stipulation.

## C4 — Logic against algebra: the head-to-head that would settle the bet
**When:** 2026-07-14 00:14
**Bearing:** open
**Inputs:** *Untested, and now armed. The corpus does not exist.* The same cross-layer query corpus called for in [[2026-07-14-pre-composition/E18]], answered by (a) wile-goast's algebraic primitives and (b) a Datalog agent interface, CodeQL or Doop exposed over MCP. n >= 60, adoption-gated, honest denominator. Source: THESIS.md hypothesis 4.
**Description:** **This entry carries the notebook's `falsified-by` line.** Logic engines
already ship in the ambient Wile environment and could be exposed as MCP primitives, which
is precisely why the bet had to be stated so it could lose: an earlier version of the thesis
said that if logic engines became scope "the substrate is already in the box", and **a bet
that absorbs its own falsifier is not a bet.** That sentence has been withdrawn and this
hypothesis re-armed in its place. The comparison is not idle speculation about a rival: for
the one comparison the project actually draws, **logic is strictly more expressive.** Datalog
handles the Dyck-language valid-path problem that no semiring can, which is exactly why
`(wile algebra cfl)` had to be written by hand ([[C2]]). The experiment must be built on a
**cross-layer** corpus, because that is where the compositional claim lives and where neither
substrate is obviously favoured. Note the dependency: [[2026-07-14-pre-composition/E18]] must
build that corpus first, and no cross-layer query has ever been benchmarked by this project
at all. If Datalog wins, **the algebraic bet is lost, and this notebook closes `refuted`.**
Per [[2026-07-14-pre-composition/E21]], that outcome leaves the parent thesis standing.

## C5 — The one arm that would vindicate the algebra is the one arm with no data
**When:** 2026-07-14 00:14
**Bearing:** open
**Inputs:** *Depends on [[2026-07-14-pre-composition/E17]], which is unrun and whose task is not yet named.* A fallible-computation task on Go source, through wile-goast, against a non-algebraic tool baseline, on the current model.
**Description:** The parent thesis has two arms. **Arm 2** (the local syntactic reading
applies a wrong rule) is the one with evidence, and it is won by a hand-written `x/tools`
pass, so it lends this notebook nothing. **Arm 1** (the step is a fallible computation, so
error compounds over lattice joins and fixpoint iteration) is where an algebraic substrate
would earn its keep, and it is the arm nobody has ever measured. State the coincidence
plainly rather than glossing it: **the single arm that would vindicate the algebraic bet is
the single arm with no data.** This entry is where that debt is recorded, and it settles
only when [[2026-07-14-pre-composition/E17]] runs. Two ways it can go badly, and both matter
here. If arm 1 dies, the strongest motivation for the algebra dies with it, and this
notebook is left holding [[C2]] (a known coverage failure) and nothing on the other side of
the ledger. If arm 1 is never *tested*, because no task can be found that stays fallible at
short answer length, then the bet is not refuted but permanently unfalsifiable on the axis
that matters, which by this notebook's own standard is no better. **The honest reason the
algebra was chosen is on the record and is not an accuracy claim:** problem-level pressure
induced it (each new structural question needed a new operator, and bolting them on ad hoc
produces a pile of special-purpose analyses with no shared structure), and it fits the
author's mental model well enough that correctness can be checked by inspection. Both are
arguments about **maintainability of the analysis library** and about **where careful work
will land**. Neither is an argument that agents get more accurate answers, and the two have
been run together before.

## C6 — Operator-instantiation produced a computation that does not terminate usefully on real Go
**When:** 2026-07-14 11:57
**Bearing:** open
**Inputs:** *Owed experiment, and it is a proof obligation rather than a run.* The question: **is HVN's pointer-equivalence a congruence with respect to the transfer functions of the points-to lattice?** Sources: `pointer/hvn.go` in the vendored oracle (`goastcg/testdata/oracle/`), Hardekopf and Lin, SAS'07, and the operator laws stipulated by this notebook. Evidence that motivates it: [[2026-07-14-pre-composition/E30]].
**Description:** This notebook's operative promise is that a new query comes from **instantiating an existing operator at a new carrier** rather than from writing a new algorithm. Inclusion-based points-to was chosen partly to test that promise, and it is a fair test: the analysis really is a least fixpoint of a monotone function over a points-to lattice, the carrier is a product of two powersets, and interface dispatch falls out of the same fixpoint as a `load` with a method table. Nothing about the formulation is a fudge. This is what the bet says a new query should look like.

**Instantiated exactly that way, it does not terminate usefully on real code.** Measured in [[2026-07-14-pre-composition/E30]]: on the wile interpreter, with the solve restricted to `lfp`-preserving schedules alone (chaotic iteration, difference propagation), the fixpoint does not converge in ten minutes **in Go**, having performed 32.3 billion membership tests, and the solution is still growing at the cutoff. The restriction to schedules is not incidental; it is what the design imposed precisely so that a result would speak to *this* notebook rather than to hand-written solver engineering. The obvious escape, blaming the implementation, was ruled out: repairing every known precision defect in the extractor moved the mean points-to set size by 7.8% while leaving oracle agreement at 126/126, where viability needed a factor of a hundred.

Note carefully what is and is not claimed. This is a **cost** result, and this notebook's thesis and `falsified-by` line are both about **accuracy**. A slow analysis is not an inaccurate one, and nothing here bears on the Datalog head-to-head in [[C4]]. But it is not irrelevant either, and [[C5]] is the reason: [[C5]] records that the single arm which could vindicate the algebra is the single arm with no data. **This is data.** It is the first time in this project that a query has been posed to the algebra, instantiated honestly, and run on real Go, and what came back is that the operator-instantiation route does not finish.

**Whether that refutes the bet turns on one checkable question, and the bearing stays `open` until it is answered.** The only implementation that ever computed this query on real Go is `go/pointer`, and its tractability comes from HVN: a pre-solver pass that finds *pointer-equivalent* variables and **collapses** them before the fixpoint runs. (`pointer/doc.go` records that it does not even perform online cycle detection, so HVN really is carrying it.) HVN quotients the constraint graph by an equivalence relation `~`.

- **If `~` is a congruence** with respect to the transfer functions, then computing `lfp` on the quotient `D/~` and lifting back is **quotient-by-a-congruence**, a standard algebraic construction. That is *instantiating an existing operator at a coarser carrier*, which is this notebook's promise stated verbatim. HVN would then not be a defection from the algebra; it would **be** the algebra, the intractability would be an artifact of having barred an algebraic move by mistake, and the bet would take its **first genuine win**: an operator-instantiation that a hand-written competitor needs a special-purpose optimization to match.
- **If `~` is not a congruence**, then the algebra cannot compute a query the project needs, under the bet's own stipulation, and the fix that works is outside it. That is the same shape as [[C2]], where interprocedural valid-path reachability needed a grammar and no semiring would do, and it would be the **second** such counterexample inside the project's own codebase.

Either outcome is decisive, and it is far cheaper than any of the experiments this notebook currently owes: the laws are already written down as the stipulation, and HVN's equivalence is defined in one paper and one file. **It should be settled before another line of solver is written.** It also cannot be settled honestly until [[C3]] is discharged, since asking whether `~` is a congruence with respect to operators whose laws have never been verified is asking a question about an object nobody has checked exists.

## C7 — HVN *is* algebraic, and it is not the lever. The bet's real exposure is the carrier's representation
**When:** 2026-07-14 12:38
**Bearing:** open
**Corrects:** [[C6]]
**Inputs:** Source reading, `goastcg/testdata/oracle/pointer/{hvn.go,doc.go}` (the vendored `go/pointer`), against the operator laws stipulated by this notebook. Measured context: [[2026-07-14-pre-composition/E30]] (the solve does not converge), and the experiment declared in [[2026-07-14-pre-composition/E31]] (the container swap).
**Description:** [[C6]] recorded that instantiating the operator produced a computation that does not terminate usefully on real Go, and staked the bet's fate on one question: is HVN's pointer-equivalence a congruence, so that adopting it would be an algebraic move rather than a defection? That framing is wrong in two ways, and both are worth having on the record because they point in opposite directions.

**First, and in the bet's favour: HVN is algebraic, and the design barred it by mistake.** The construction is not, as [[C6]] guessed, a quotient of the value lattice. HVN's `~` is an equivalence on the **index set** (the variables), not on points-to sets. Take the map lattice `L = D^V`, and let `S ⊆ L` be the maps that are constant on `~`-classes. `S` is closed under `⊔` and `⊓` pointwise and contains `⊥`, so it is a **sublattice**, and `S ≅ D^(V/~)`. What HVN's soundness argument establishes is `F(S) ⊆ S`. Given that, the Kleene chain from `⊥` never leaves `S`, so `lfp(F) ∈ S` and `lfp(F) = lfp(F|S)`. That is **Knaster-Tarski restricted to an F-invariant sublattice**: the same operator instantiated at a coarser carrier, yielding the identical fixpoint. It is exactly the move this notebook's thesis says a new query should be made of, and the design forbade it on the mistaken belief that a constraint-graph rewrite must be hand-written solver engineering.

Two honest caveats. The implementation is a sound *approximation* of true pointer-equivalence rather than an exact one: `hvn.go` records that it can collapse `*p` cycles even when `pts(p)` is empty, losing precision in dead code. That makes it an abstraction, which is still an algebraic construction, but not a precision-preserving one. And it stays sound under on-the-fly dispatch only because it marks as `indirect`, and therefore never merges, *"the parameters of address-taken functions (which includes all functions in method sets)."* The nodes a dynamically added edge can write to are conservatively excluded from the equivalence.

**Second, and against the way [[C6]] framed the stakes: HVN is not the lever, so it was never decisive.** `hvn.go`'s own PERFORMANCE section reports that it removes about two thirds of nodes and constraints and reduces **solving time by 30 to 42 percent**, against roughly 15 percent paid to run HVN itself. That is a net factor of about 1.3. The gap [[2026-07-14-pre-composition/E30]] must close is about 100. [[C6]] put a 1.3x question on the critical path, and it did so without reading the numbers printed directly beneath the mechanism it was reasoning about.

**Where the exposure actually is, and it is the sharpest statement of this notebook's risk yet made.** `pointer/doc.go` says the solver represents points-to sets as **sparse bit vectors**, *"more space- and time-efficient than sets based on Go's built-in map type."* The measurement in [[2026-07-14-pre-composition/E30]] used precisely a hash map, so its 32.3 billion operations are a property of the container and not of the fixpoint. That is the hypothesis [[2026-07-14-pre-composition/E31]] will settle.

Note carefully what representation *is*, in this notebook's own terms. **The operator laws say nothing about how a lattice element is stored.** Associativity, idempotence, absorption, distributivity, monotonicity: none of them constrain the encoding of a set. Choosing a sparse bit vector over a hash set is a decision made strictly *below* the algebra, invisible to the composition rules, and it therefore **cannot forfeit the bet however clever it gets**. This is a genuinely different kind of escape hatch from the ones [[C2]] and [[C6]] contemplated, and it is the only one so far that costs the bet nothing.

Which relocates the risk, and does not remove it. **Wile's native set is a list.** For a carrier of this size a list is the worst available representation, and the entire cost question for `(wile goast andersen)` now reduces to whether `P(Object)` can be represented efficiently *inside Wile*. If it can, the algebra was never implicated and this notebook's cost worry evaporates. If it cannot, then the bet fails for a reason that has nothing to do with its operators and everything to do with the substrate they are written in, and that distinction must be stated plainly rather than scored as a defeat for the algebra. **A slow container is not a wrong theory.**
