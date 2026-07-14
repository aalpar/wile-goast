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
