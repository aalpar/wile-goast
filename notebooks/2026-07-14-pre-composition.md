---
notebook: 2026-07-14-pre-composition
thesis: A tool earns an accuracy advantage exactly where the agent's own derivation is unreliable, and pre-computing the fact rather than having the agent reconstruct it is what buys that reliability.
falsified-by: On an arm-2 corpus not used to formulate the criterion (one where the local syntactic reading provably disagrees with the semantics), treatment accuracy minus the strongest conventional arm (the higher of control and gopls+grep) is < 5 percentage points, at n >= 60.
created: 2026-07-14
status: open
verdict:
---

# Pre-composition as substrate for agent-facing code analysis

Source thesis: `docs/THESIS.md` (wile-goast), revised 2026-07-13. Experimental record:
`~/ClaudeProjects/LLMAccuracy`, corpora under `code-restructuring/` and
`algebra-accuracy/`. Model: `claude-opus-4-8` throughout, caching off.

The thesis decomposes into two arms, and their standing differs sharply:

- **Arm 1, the step is a fallible computation.** Per-step error compounds with the
  number of steps. **Never measured** (E17).
- **Arm 2, the step applies a wrong rule.** The local syntactic reading disagrees with
  the semantics, so the agent fails at depth one and depth is irrelevant. **Measured
  once**, +35.0 points (E6).

The `falsified-by` line binds the thesis to arm 2's generalization, because arm 2 is
the only arm with evidence and that evidence rests on a single pattern. E11 is the
experiment that settles it, and E11 has not been run.

## E1 — Budget confound: is the negative treatment delta starvation, or interference?
**When:** 2026-06-09 15:25
**Bearing:** serves [[E5]]
**Inputs:** `evaluate.py --condition both --total-budget 5000`, then `--condition treatment --total-budget 10000`; corpus `gradient_problems_v2.json` (90 problems, algebra, not Go); model `claude-opus-4-8`; `max_rounds=30`. Raw results `algebra-accuracy/results_step0_opus48_b{5000,10000}.json`.
**Description:** An earlier run had the tool-using arm scoring *below* the no-tool
control, which if taken at face value would sink the whole programme before it
started. Two explanations were confounded: (A) tool orchestration eats the context
budget, leaving no room for the final answer, or (B) tools genuinely interfere with
the model's reasoning. This run isolates (A) by re-running the identical 90 problems
with one variable moved, the total token budget, doubled from 5,000 to 10,000. It is
a validity check on the harness, not a test of the thesis: it exists so that the
later Go measurements ([[E5]], [[E6]], [[E7]]) can be read as measuring what they
claim to measure. Note the corpus is the algebra set, not Go source, and the tool is
not wile-goast, so nothing here bears on arm 1.
**Result:** Control 67/90 (74.4%). Treatment 56/90 (62.2%) at 5k, rising to 65/90
(72.2%) at 10k: +10.0 points from budget alone. Starved runs (`budget_exhausted` plus
`max_tokens`) halve, 30 to 15. **The negative delta was budget starvation (A), not
reasoning interference (B).** Confound resolved; the protocol fix gates every later
run.
**Raw:** `2026-07-14-pre-composition.d/E1-step0-budget.txt`

## E2 — Interface dispatch, may-union corpus: control is already perfect
**When:** 2026-06-09 16:00
**Bearing:** supports
**Inputs:** `generate_interface_dispatch.py` (Family-C2), three corpora (`problems_reach_iface.json`, `..._nest.json`, `..._scale.json`), 33 problems total; control arm only. Raw results `code-restructuring/results_iface_{control_pilot,nest_control_pilot,scale_control}.json`. Protocol predates `--withhold-source` and tool-call adoption recording (`3feacd5`, 2026-07-11).
**Description:** Interface method dispatch (`x.Foo()`) names no concrete
implementation, and is the canonical case of Go hiding a callee behind syntax. This
corpus builds it in the *may-union* shape: the factory `makeIa(n)` returns **every**
implementor, so a sound answer must union every implementor's method at each dispatch
site, and excluding one would genuinely be unsound. The generator's own docstring
records the consequence, and the ground truth is oracle-verified against
`go-callgraph 'cha` at generation time: **CHA is exact here, by construction.** So the
naive syntactic rule ("any implementor could be called") *is* the correct semantics.
Only the control arm was ever run; no tool arm exists, and no margin can be computed
from this run at all.
**Result:** Control 33/33 (100.0%), unaided, across all three corpora (12/12 flat,
12/12 nested, 9/9 scaled, with reachable sets up to 152 functions). Where syntax tells
the truth, unaided control is at ceiling and there is **no headroom for any tool to
win**. This is the criterion's negative half, confirmed a second time (compare
[[E5]]). It is emphatically **not** the interface falsification test: that needs the
*decoy* variant, where CHA over-approximates and the truth excludes the decoy, and
that corpus does not exist. See [[E11]].
**Raw:** `2026-07-14-pre-composition.d/E2-iface-control.txt`

## E3 — A returned bound harmed its consumer: the tool scored 33 points below no tool
**When:** 2026-07-08 23:17
**Bearing:** refutes
**Inputs:** `evaluate_reachability.py` on a 15-problem corpus built so that `'vta` over-approximates by exactly one address-taken decoy; `'precise` withheld from both arms. Two runs, identical problems and identical ground truth, **only the prompt changes**: `results_reach_sub.json` (LLMAccuracy `a663024`) reports `'vta`'s result as the answer; `results_reach_sub_verify.json` (`a9a135b`) adds a mandate to re-derive each member's constant index by hand and drop the ones no invoked index selects (`evaluate_reachability.py:143`).
**Description:** The thesis says the way to buy reliability is to **pre-compute the
fact** rather than have the agent reconstruct it. This experiment handed the agent a
*sound over-approximation* instead of a fact, and measured what the agent did with it.
It is the sharpest cut in the record against the thesis as stated, and it is not
discharged by anything that follows.
**Result:** Control (no tool at all) 11/15 (73.3%). Treatment reporting `'vta`'s bound
6/15 (40.0%), a margin of **−33.3 points: the tool left the model worse than no tool.**
The model reported the bound as the answer; anchoring, not tool error. Adding the
re-derive-by-hand mandate moves treatment to 15/15 (100.0%). The comparison that
matters is paired, and it is decisive: across the same 15 problems the mandate **fixes
9 and breaks 0** (McNemar exact, p = 0.0039), control results are byte-identical across
the two runs, and the cases where the tool left the model worse than no tool go from
**6 to 0**. What recovered those 60 points was **not** a better tool description, since
both prompts describe `'vta` identically. It was an instruction to *distrust the tool
and redo its work by hand*. Pre-composition delivered a bound, and the points came from
the very step it was supposed to remove.
**Raw:** `2026-07-14-pre-composition.d/E3-anchoring-pair.txt`

## E4 — Scale axis: conventional arms degrade, the tool holds, cause unattributable
**When:** 2026-07-09 22:34
**Bearing:** serves [[E16]]
**Inputs:** `results_reach_breakzone_all3.json`, three arms, n=8 per rung, file sizes f128 / f192 / f256 / f384.
**Description:** A separate corpus from the depth ladder ([[E5]]): here the file grows
rather than the chain. It is recorded because a notebook that reports only the f80 null
while sitting on a contrary f384 observation is selecting its evidence. But it is
confounded and cannot carry a thesis claim on its own, which is why its bearing is
instrumental: it exists to motivate the disentangling corpus in [[E16]].
**Result:** Pooled at f>=192 (n=24): control 19/24 (79.2%), baseline gopls+grep 18/24
(75.0%), treatment 24/24 (100%). Treatment versus control, Fisher p = 0.0496. Both
conventional arms degrade at scale while the tool holds. **The confound:** the corpus
varies file size, reachable-set size, and depth *together* (`n_reachable` runs 55 at
f128 to 218 at f384), so it cannot attribute the cause and supports no estimate of the
per-step error rate. At n=24 and p about 0.05 it is suggestive, not established. It
also threatens the `O(answer)` cost conjecture: 218 of 384 functions were reachable, so
the answer set may grow as fast as the source (see [[E13]]).
**Raw:** `2026-07-14-pre-composition.d/E4-scale-breakzone.txt`

## E5 — Depth ladder at f80: the tool buys nothing over conventional tooling
**When:** 2026-07-11 20:47
**Bearing:** supports
**Inputs:** `results_reach_final.json` (LLMAccuracy `17a3e8d`), three arms, n=60 per rung, depths d=4 and d=47, source-withheld and adoption-gated. The generator **pins file size at 80 functions and answer size at 48 names**, so depth is the only variable.
**Description:** The flagship reachability query, run as a clean depth ladder: with file
size and answer size held constant, the only thing that moves is the number of
composition steps the agent must chain. This is the experiment that was once read as
falsifying the thesis outright, and the reading was half right. The tool-value null is
real and stands. The *composition* reading of it does not, and that correction is
[[E9]].
**Result:** At d=4, control 59/60 (98.3%), baseline 60/60, treatment 60/60. At d=47,
control 55/60 (91.7%), baseline 60/60, treatment 60/60. Treatment called a tool on
120/120 problems, so the adoption gate is satisfied and this is not the control arm in
a costume. **Two margins, saying different things: treatment minus baseline is +0.0 at
both rungs (the null); treatment minus control is +1.7 at d4 and +8.3 at d47 (Fisher
p = 0.057, not significant).** The honest denominator is the strongest alternative; at
an 80-function file grep is at ceiling and *is* the strongest alternative, so **the null
stands: on plain syntactic reachability the tool buys nothing over conventional
tooling.** This *supports* the thesis rather than cutting against it, because the thesis
claims a win **exactly where** derivation is unreliable: here syntax tells the truth and
the per-step error rate is tiny, so the criterion predicts no gain, and it got that
right. It is the negative half of a biconditional, confirmed. What it is **not** is a
result about composition; see [[E9]].
**Raw:** `2026-07-14-pre-composition.d/E5-depth-ladder.txt`

## E6 — Higher-order dispatch: the one clean win, +35.0 points over control
**When:** 2026-07-11 22:02
**Bearing:** supports
**Inputs:** `results_dispatch_subtle.json` (LLMAccuracy `6a2d887`), three arms, n=60, 50 functions per problem, exactly one never-invoked decoy, source-withheld and adoption-gated. Treatment uses wile-goast `'precise`.
**Description:** The corpus is constructed so the obvious syntactic reading is *wrong*.
A slice literal `t := []func(){f40, f34, f4}` is followed by `t[0](); t[1]()`. Three
function names sit in the body and the pattern-matching rule fires on all three, but
only two are invoked: `f4` is address-taken and never called. Getting this right means
tracking the slice as a value, finding which constant indices are invoked, and mapping
each index back to the element it selects. The literal is on one line and the
invocation on another, so the fact lives in the **join** of the two, and that join is
constant propagation through SSA. The model's rule is local; the inference is not. This
is the only measurement supporting arm 2, and therefore the only measurement supporting
the thesis's positive half.
**Result:** Control (clean source, no tools) 39/60 (65.0%). Baseline gopls+grep 14/60
(23.3%). Treatment 60/60 (100%). **+35.0 points over control, p < 1e-6.** The honest
headline is +35.0 over control, not +76.7 over grep: grep is line-oriented and the fact
spans two lines, so its collapse is a self-inflicted retrieval wound, not evidence about
the tool. **The failure mode is the finding.** Control's wrong answers were,
systematically, the true set *plus* the address-taken decoy: reasoning from syntax, the
model re-derived Class Hierarchy Analysis's exact over-approximation and was confidently
wrong. It does not hedge or flag uncertainty. From the inside, the 65% case and the 100%
case feel identical. **Standing charge against this entry:** `goastcg/precise.go:26-27`
describes the tool as resolving "a constant index into a literal `[]func()`", which is a
description of this benchmark. The circularity is real and the escape is [[E11]].
**Raw:** `2026-07-14-pre-composition.d/E6-dispatch-subtle.txt`

## E7 — Cost: against the honest denominator the tool buys no savings either
**When:** 2026-07-12 08:33
**Bearing:** serves [[E13]]
**Inputs:** `report_cost.py results_reach_final.json results_dispatch_subtle.json` (LLMAccuracy `3ee0cdb`). Metric is mean tokens per problem, input plus cache plus output, prompt caching off.
**Description:** If the tool does not buy accuracy on reachability ([[E5]]), the
remaining case for it on that axis is cost. This entry measures it, and corrects an
earlier claim that was wrong in two ways at once: it counted output tokens only, hiding
the largest cost term, and it used grep as the denominator, which is the weakest
alternative rather than the strongest. Its bearing is instrumental because cost is not
what the thesis claims. The thesis claims an *accuracy* advantage, so a cost result
neither supports nor refutes it. What this entry does is set the anchors for the
crossover sweep in [[E13]], which is the only surviving efficiency claim.
**Result:** Reachability at f80: control 2,228 tokens, grep 12,470, treatment 2,320.
Dispatch at f50: control 1,443, grep 19,586, treatment 2,867. Caching was off, verified
in the raw records rather than assumed. So the tool is 5.4x to 6.8x cheaper **than
grep**, but grep-dumping an 80-function file is a bad strategy and beating a bad strategy
is not differentiation. Against the honest denominator the tool is **cost-neutral (2,320
against 2,228) or 2x worse (2,867 against 1,443)**: the tool round-trip costs more than
reading a 1.2k-token file. On this axis the tool currently buys **neither accuracy nor
savings**. The surviving conjecture is that control's cost is `O(source)` while the
tool's is `O(answer)`, implying a crossover at larger n. It is **unmeasured**, and
[[E4]] suggests it may not survive contact with an answer set that grows with the source.
**Raw:** `2026-07-14-pre-composition.d/E7-cost.txt`

## E8 — The experiment meant to measure the MCP surface has never spoken MCP
**When:** 2026-07-12 11:08
**Bearing:** serves [[E14]]
**Inputs:** LLMAccuracy `2413a54`, `feat(surface): real-MCP arm — GoastMCPSession cwd + shipped tool descriptions`; audit of `evaluate_reachability.py:170` and `goast_session.py:73` against wile-goast `cmd/wile-goast/mcp.go:213`.
**Description:** Every accuracy figure in this notebook was obtained with a hand-written
harness prompt that does not exist in the shipped product. Building the arm that would
measure the *real* surface turned up something worse than the known hole, and this entry
records it because it changes what the pending surface experiment ([[E14]]) has to
control for. It is groundwork, not a thesis claim, hence the instrumental bearing.
**Result:** The external-validity hole is **wider than documented**. It was recorded as
one fabrication, the hand-written `PRECISE_TREATMENT_PROMPT`. There are three. (1) The
system prompt, known. (2) The **tool description fed to the API**:
`evaluate_reachability.py:170` hand-writes `{"name": "eval", "description": "Evaluate a
Scheme expression..."}`, which is *not* the string the server actually ships in
`mcp.go:213`. (3) The **transport**: the arm shells out to `wile-goast -e` by subprocess,
so **the MCP server is never started.** The harness built to measure the MCP surface has
never once spoken MCP. `list_tools()` was already returning the shipped descriptions and
throwing them away. This is now fixed in the harness, which makes [[E14]] runnable.

## E9 — The per-step error rate is 0.0019, and the point is that it is not zero
**When:** 2026-07-13 21:45
**Bearing:** serves [[E12]]
**Inputs:** Re-fit of `accuracy ≈ (1 − ε)^N` to the clean depth ladder already in hand ([[E5]]: f80 and answer size pinned, so N is the only variable). No new run.
**Description:** Until 2026-07-13 the project held that reading a call name off a
function body is a *lookup* whose error rate is zero, and concluded that "you cannot
compound an error rate of zero." That line was a clean idealization of a measurement
that contradicts it, and this entry retires it. The fit is a sketch to organize results,
not a fitted curve: steps are not independent and the model does not literally multiply
out. Its consequence, however, is what makes the decisive experiment ([[E12]]) worth
running at all, which is why this entry is instrumental rather than a claim.
**Result:** A single **ε = 0.0019** reproduces both rungs of the ladder: 99.2% at N=4
(observed 98.3%, a one-problem difference at n=60) and **91.4% at N=47 (observed
91.7%)**. **The measurement does not show ε = 0. It shows ε ≈ 0.002.** So the correct
verdict on the composition hypothesis is **not falsified, but underpowered**: the
experiment drove N to 47, at which a per-step ε of 0.002 buys about 8 points of decay,
below the resolution of 60 samples (p = 0.057, and the d4-to-d47 decline is itself
p = 0.207). Held at f80, that same ε entails control accuracy of **75.2% at N=150** and
**46.7% at N=400**, which is what [[E12]] runs. The scale corpus ([[E4]]) additionally
rejects the assumption that ε is fixed by step type alone: the same kind of step taken
against a larger file is not equally reliable.

## E10 — Provenance re-check: every headline figure recomputed from the raw JSONs
**When:** 2026-07-14 00:14
**Bearing:** serves [[E6]]
**Inputs:** Direct reads of `results_reach_final.json`, `results_dispatch_subtle.json`, `results_reach_sub{,_verify}.json`, `results_reach_breakzone_all3.json`, `results_iface_*.json`, `results_step0_opus48_b{5000,10000}.json`; per-arm tallies and margins derived independently; cost table reproduced with the project's own `report_cost.py`.
**Description:** THESIS.md was revised on 2026-07-13 after auditing the harness and raw
results against the prose, and five claims were retired. Not one of them was wrong about
a *number*. They were wrong about **which run the number came from, what it was a number
of, or what it was being compared against**: failures of provenance, invisible to anyone
reading the prose alone. A notebook that transcribes those figures from prose inherits
exactly that risk, so before any figure was written into this notebook it was recomputed
from the raw result files. This entry is instrumental: it does not add evidence, it
establishes that the evidence cited in [[E1]] through [[E7]] is what those entries say it
is.
**Result:** **Every headline figure reproduces exactly.** Depth ladder 98.3/100/100 at d4
and 91.7/100/100 at d47, margin +0.0 against baseline at both rungs. Dispatch
65.0/23.3/100.0, +35.0 over control. Anchoring pair −33.3 then +26.7, with 9 fixed, 0
broken, and byte-identical controls. Scale f>=192 pooled 19/24, 18/24, 24/24. Cost
2,228/12,470/2,320 and 1,443/19,586/2,867 with caching verified off rather than assumed.
Budget confound 74.4% control against 62.2% at 5k rising to 72.2% at 10k. Adoption gates
hold (120/120 and 60/60 treatment tool calls). **One gap found, and it is a reporting gap
rather than an error:** per-arm token counts are *nested* inside the arm record, not flat
fields, so a naive extractor reports no token fields at all and silently drops the entire
cost axis. The figures are correct; the shape of the file invites precisely the
provenance mistake the thesis had just finished correcting.
**Raw:** `2026-07-14-pre-composition.d/E10-provenance-recheck.txt`

## E11 — Interface dispatch, decoy variant: the falsification test of the thesis
**When:** 2026-07-14 00:14
**Bearing:** open
**Inputs:** *Pre-registered, not yet run.* Corpus: a new generator mode, decoy structure mirroring `--mode subtle`, in which exactly one implementing type is allocated but **never flows to the call site**, so CHA folds it in and the truth excludes it. Arms: control (clean source, no tools) / baseline (read-file, **not** grep, which is settled as a bad strategy) / treatment (`eval`, prompt exposing `'cha`, `'rta`, `'vta` with honest soundness labels). n >= 60, source-withheld, adoption-gated. Source: THESIS.md hypothesis 6, `LLMAccuracy/TODO.md` item 2.
**Description:** **This entry carries the notebook's `falsified-by` line.** The thesis's
only supporting measurement ([[E6]]) is on a single pattern, a constant index into a
`[]func()` literal, and the tool's own docstring describes that pattern, so the
circularity charge is live. This experiment asks whether the criterion generalizes to the
*other* place Go hides callees behind syntax. It is not a repeat of [[E6]], because of an
asymmetry that has to be understood before the arms are designed: **`'precise` does not
handle interface dispatch at all.** `goastcg/precise.go:66` returns `nil` for
`IsInvoke()` and falls back to CHA edges. So for interfaces there is **no exact
algorithm**; `'vta` is the tightest available and it remains a *sound
over-approximation*. Note carefully that this is **not** the corpus already run in
[[E2]]: that one is the *may-union* shape, where every implementor genuinely is
reachable and CHA is exact by construction. The generator for the decoy variant **does
not exist yet** and must be written. Three outcomes, and the middle one is the most
valuable. (a) `'vta` is exact on the corpus and treatment reaches about 100%: the
criterion holds. (b) `'vta` over-approximates and the model reports the bound: **the −33
point anchoring trap of [[E3]] reproduces**, the tool *hurts*, and a must/may split
becomes load-bearing on evidence rather than taste. A bound-shaped return harming its
consumer twice stops being a coincidence and starts being a design law. (c) Control
scores at ceiling unaided: **the criterion is falsified, and this notebook says so.**

## E12 — The deep-N test: does composition actually compound?
**When:** 2026-07-14 00:14
**Bearing:** open
**Inputs:** *Pre-registered, not yet run.* The clean depth ladder ([[E5]]) extended to N=150 and N=400, holding file size at f80 and answer size at 48 names so that N remains the only variable. Control arm is the one under test. Source: THESIS.md hypothesis 1.
**Description:** The decisive experiment for arm 1's *premise*. The ε = 0.0019 fit
([[E9]]) entails control accuracy of **75.2% at N=150** and **46.7% at N=400**, which is a
falsifiable forecast rather than a guess, and the two rungs above are where the curve
becomes visible. If control tracks the curve, **composition is vindicated** and the d47
null of [[E5]] was a power failure rather than a refutation. If control holds above 95% at
N=150, the compounding model is **wrong**, "lookups are free at any depth" wins, and arm
1's motivation weakens further. Either way, the retraction of "you cannot compound an
error rate of zero" already stands on [[E9]] and does not depend on this outcome. Note
what this experiment does *not* settle: it measures the *control* arm's degradation, so it
tests whether the error-rate model is right, not whether the tool is worth using.

## E13 — Cost crossover: sweep f50 to f384 and find where the curves cross
**When:** 2026-07-14 00:14
**Bearing:** open
**Inputs:** *Pre-registered, not yet run.* `generate_reachability.py --mode adversarial --funcs 50 80 128 192 256 384 --count 10 --seed 31337`, then `evaluate_reachability.py --condition both --withhold-source`, then `report_cost.py`. No grep arm (settled as a bad strategy). Needs a per-rung group-by in `report_cost.py`, about 10 lines. Source: THESIS.md §Cost, `LLMAccuracy/TODO.md` item 1.
**Description:** The only surviving efficiency claim, and it is currently unmeasured and
must not be asserted until it is. [[E7]] gives the anchors: at f80 the two arms are within
92 tokens and control is *slightly cheaper*. Treatment carries roughly 800 tokens of fixed
overhead (the system prompt describing the tool) but pays only about 2.4 tokens per
function (a name, `p.f123`) where control pays about 15.5 (a whole definition line), so the
slope gap is about 13 tokens per function and a crossover should exist. **Pre-registered
prediction: crossover at n = 85 to 100**, with treatment about 2.3x cheaper at f384
(control about 6,900 tokens against treatment about 3,100). If treatment never overtakes
control, the `O(answer)` story is **wrong**, most likely because the returned set grows as
fast as the source, and the efficiency claim dies entirely. [[E4]] is the warning sign: 218
of 384 functions were reachable at f384. Watch `treatment_starved` at f384.

## E14 — Surface A/B: how much accuracy does the shipped MCP surface throw away?
**When:** 2026-07-14 00:14
**Bearing:** open
**Inputs:** *Designed, not yet run.* Corpus `problems_dispatch_subtle.json`, where the tool provably wins ([[E6]]), so any shortfall is attributable to the surface and nothing else. Four arms: **A0**, the shipped surface before wile-goast `16653eb`; **A1**, after it; **B**, A1 with `reference` force-loaded; **C**, the hand-tuned `PRECISE_TREATMENT_PROMPT`, the known 100% ceiling. Requires the real-MCP arm from [[E8]]. Source: THESIS.md hypothesis 9, `LLMAccuracy/TODO.md` item 3.
**Description:** Every accuracy figure in this notebook was obtained with a prompt the
user never sees, and [[E8]] showed the gap is wider than a prompt: the harness never
started the MCP server at all. This experiment measures the surface a real Claude Code
user actually gets. The mechanism under suspicion is *selection*: `'precise` is a
wile-goast invention absent from the model's training priors, while `static`, `cha`,
`rta` and `vta` are `x/tools` names it knows. Before `16653eb` the shipped `eval`
description named no call-graph algorithms and the `reference` cheatsheet omitted
`'precise` entirely, leaving one under-approximation, three over-approximations, and **no
exact option**, which is exactly the configuration measured at −33 points in [[E3]]. **A
model not told an algorithm exists cannot select it.** A1 to C is the headroom purchasable
with zero new analysis capability, and it is the number that decides how much MCP work is
worth. **Instrument the failure chain, not just accuracy:** adoption (did it call `eval` at
all), selection (*which algorithm did it pick*, the predicted failure point), invocation
(first-call success rate, since `docs/MCP.md` already concedes that a cold `eval` tends to
fail on arity), interpretation (did it report the tool's set faithfully or hand-filter it),
and only then accuracy. Predictions: A0 never selects `'precise` and lands near 40 to 65%;
B > A1 would mean the cheatsheet is good but unread, so the fix belongs in `mcp.go`; A1 ≈ C
would mean the cheatsheet fix alone closed the gap, so ship it and stop. If A0 scores at
ceiling anyway, the "surface dominates" claim is **wrong** and this notebook says so. Note
what makes this cheap to get wrong: the −33/+27 pair in [[E3]] is **not** evidence for this
hypothesis. It ran on a different corpus, at n=15, with a rigged tool, and its swing came
from a verification mandate rather than from a tool description.

## E15 — Abstention: does the surface cause over-invocation where the tool adds nothing?
**When:** 2026-07-14 00:14
**Bearing:** open
**Inputs:** *Designed, not yet run.* `problems_reach_final.json`, the d=4 rung (plain syntactic call graph, where the tool is known to add nothing), real MCP surface, control against treatment. Metric is **invocation rate, not accuracy**. Pairs with [[E14]] on the same harness. Source: `LLMAccuracy/TODO.md` item 4.
**Description:** [[E5]] established that the tool adds nothing on syntactic reachability
(margin +0.0 against conventional tooling) and [[E7]] that it is cost-neutral to 2x worse
there. A good tool description must therefore say **when not to reach for the tool**, and
over-invocation on this class is a pure cost leak: a round-trip bought for zero accuracy.
This is newly answerable only because there is finally a criterion for where the tool
helps. A high invocation rate means the surface oversells, and the fix is a "when not to
reach for this" clause, after which cost falls and accuracy is unchanged. A low rate means
the model already abstains correctly and no fix is needed. Note this is a measurable defect
of the shipped surface and it is currently unmeasured.

## E16 — Why does the tool hold at scale? Pin answer size and depth, vary file size
**When:** 2026-07-14 00:14
**Bearing:** open
**Inputs:** *Not yet designed in detail.* A corpus that varies file size with **answer size and depth pinned**, in contrast to [[E4]], which varied all three together. Source: THESIS.md §Open questions.
**Description:** [[E4]] found both conventional arms degrading at f>=192 while the tool
held at 24/24, but it cannot say *why*, because file size, reachable-set size and depth all
moved together. This corpus separates distractor-count effects from output-length effects,
and it is the experiment that would let the scale observation carry weight instead of
merely being suggestive. It has a second payoff: it settles whether the `O(answer)` cost
conjecture ([[E13]]) survives an answer set that grows with the source, which is the
specific way that conjecture is most likely to die. It also tests the claim in [[E9]] that
ε is a property of the step *and its context* rather than of the step type alone.

## E17 — Arm 1 has never been tested: build the corpus that would settle it
**When:** 2026-07-14 00:14
**Bearing:** open
**Inputs:** *Open. The corpus does not exist and must be built.* A test of arm 1 must satisfy four conditions **simultaneously**: a fallible-computation task **on Go source**, **with wile-goast**, **against a non-algebraic tool baseline**, **on the current model**. No existing corpus in LLMAccuracy meets all four. Source: THESIS.md hypothesis 2 and §Arm 1 is unmeasured.
**Description:** **The highest-value experiment the project could run next, and the one
with no data.** Arm 1 claims the tool wins where the step is a fallible computation, so
that error compounds with the number of steps: lattice and fixpoint computations, abstract
interpretation, dataflow joins. Arm 1 may well be true, since abstract interpretation
exists precisely because fixpoint computation is hard. But it has never been measured, and
it is a load-bearing wall nobody has inspected. Say the coincidence out loud rather than
gloss it: **the one arm that would vindicate the algebraic design bet is the one arm with
no data** (see [[2026-07-14-algebraic-bet]]). If the tool does not beat control under those
four conditions, arm 1 is dead and with it the strongest motivation for the algebra.
**Open design problem, and it is not a formality.** A lattice deep enough that the answer is
not readable off the page also makes the answer *long*, which re-introduces the file-size
and answer-size confound that already muddies [[E4]]. A join small enough to avoid that
confound may be one where syntax tells the truth and ε is tiny, in which case the tool
cannot win by construction. **Naming a task that stays fallible at short answer length is
the unsolved part of this experiment**, and it belongs to the author. Note that [[E1]] is
*not* a measurement of arm 1: it ran on the algebra corpus, not Go source, and not through
wile-goast.

## E18 — Flat APIs suffice: build a genuinely cross-layer query corpus
**When:** 2026-07-14 00:14
**Bearing:** open
**Inputs:** *Untested at the level stated. The corpus must be built.* A cross-layer query corpus (for example, "find functions that acquire a lock without releasing it on at least one path", which needs the call graph, the CFG and SSA together), against an agent with grep, read-file and a long context. Source: THESIS.md hypothesis 3.
**Description:** If an agent with flat tools matches wile-goast on **cross-layer** queries,
the pre-composition claim is wrong. This hypothesis is live and unaddressed, and the reason
it matters is easy to miss: **the reachability null of [[E5]] did not test it.** Reachability
is a *single-layer* query, over the call graph alone. **No cross-layer query has ever been
benchmarked**, which means the central compositional claim of the thesis, the one that names
the gap in existing tooling, rests on no measurement at all. Every corpus in this notebook
tests a single layer.

## E19 — Does uplift survive contact with real code?
**When:** 2026-07-14 00:14
**Bearing:** open
**Inputs:** *A plan, not a record.* Two corpora: **wile itself** (about 40k lines of Go, self-contained, well understood by the author, intended as a known-answer corpus) and the **Kubernetes universe** (large, heterogeneous, externally reviewed, intended as a generalization test). Source: THESIS.md hypothesis 7 and §Scope of validation.
**Description:** **Every corpus in this notebook is machine-generated** by
`generate_reachability.py`. If the primitives improve accuracy on generated Go but not on
Kubernetes, Prometheus or etcd, the substrate is doing decorative work. Nothing yet
distinguishes these cases, and until one of these corpora produces a number, the external
validity of every figure in this notebook is unestablished. **Open choice, and it is the
author's:** Kubernetes has particular conventions (generated code, the operator pattern,
heavy interface use) that may bias findings, so one more corpus of a different flavour
(etcd, Prometheus, Vitess, Docker) would strengthen the generalization claim. Which one, and
why, is not yet decided.

## E20 — ε is a property of the model, not the task: does the margin decay as models improve?
**When:** 2026-07-14 00:14
**Bearing:** open
**Inputs:** *Unaddressed, and it has an expiry date.* Hold every corpus in this notebook fixed and re-run it on each model generation, watching whether the margins decay. Source: THESIS.md hypothesis 8.
**Description:** Both arms are **error-rate claims, not information-theoretic ones**: every
fact the tool computes is already present in the source the model was handed. None of these
is an information gap. So if model capability drives ε toward zero, **both arms close**, and
ε is already small at 0.002 ([[E9]]). **Nothing in the current criterion is durable under
model improvement**, and that is a statement about the thesis, not about the benchmark. If a
durable case exists, a whole-program fact that no local reading can contain at any
capability, then *that* is the case the thesis wants, and it is not the one currently argued.
This entry is the standing reason the notebook cannot simply accumulate wins and stop.

## E21 — The algebraic bet needs its own notebook
**When:** 2026-07-14 00:14
**Bearing:** open
**Spawned:** [[2026-07-14-algebraic-bet]]
**Description:** wile-goast's primitives are built on an *algebraic* substrate: lattices and
closure operators, semirings, symbolic rewriting, boolean algebra. THESIS.md states plainly
that this is "a design choice for which the project has produced no discriminating evidence",
and deliberately isolates it so that the thesis survives its refutation. This notebook honours
that separation by spawning it.

**How the child bears on this thesis, and why the write-back inverts.** The child's thesis is
**not a sub-claim** of this one. This notebook claims that *pre-composing* a fact the agent
would otherwise derive unreliably buys accuracy; it says nothing about *how* that fact is
computed. A Datalog engine, a hand-written pass, or an algebraic library would all satisfy it.
`goastcg/precise.go`, which produced the only clean win in this notebook ([[E6]]), is **not
algebraic**: it is CHA plus an SSA constant-index resolution pass, written directly against
`golang.org/x/tools`. So **refuting the child leaves this thesis standing**, and the correct
write-back into this entry on a `refuted` child is *not* `refutes`. That inference is
available only because the two claims were separated before the evidence came in, and it is
exactly the inference that would be wrong if they had not been.

**The one place the two claims touch is arm 1.** Lattice and fixpoint computation is both the
untested arm of *this* thesis ([[E17]]) and the strongest motivation for the algebra. If
[[E17]] kills arm 1, it damages both notebooks at once. That coupling is the reason this entry
stays `open` rather than being written off as an unrelated branch. This entry settles when
[[2026-07-14-algebraic-bet]] returns a verdict, and the reasoning for whichever bearing it
then takes gets written here.

## E22 — The E11 decoy corpus is built, and it rules out the anchoring trap in advance
**When:** 2026-07-14 01:04
**Bearing:** serves [[E11]]
**Inputs:** New `--decoy` mode in `generate_interface_dispatch.py` (LLMAccuracy, `code-restructuring/`), an additive extension of the existing may-union generator. Corpus generated with `--depths 3 6 9 --impls 2 4 6 --count 7 --seed 31337 --dead 8`, giving 63 problems into `problems_iface_decoy.json` and `packages_iface_decoy/`. Regression-checked: the pre-existing may-union corpus regenerates byte-identical from the same seed, so the other modes are unchanged.
**Description:** [[E11]] carries this notebook's `falsified-by` line and could not run,
because the corpus it needs did not exist: the interface generator already in the repository
builds the *may-union* shape of [[E2]], where CHA is exact by construction and syntax tells
the truth. This entry builds the shape [[E11]] actually calls for, and is instrumental to it.
**The construction.** One implementing type, `TaD`, is allocated in **reachable** code and
converted to the interface (`sink(TaD{})` inside the reachable spine), but the factory
`makeIa` can never return it and `sink` never invokes its method, so `TaD`'s leaf is
unreachable while every local syntactic cue says it is not. The rule that includes it,
"`TaD` implements `Ia`, `TaD` is constructed, this is an `Ia` dispatch, therefore `TaD.M` is a
possible target", **is Class Hierarchy Analysis**, which is precisely the over-approximation
the model reinvented and was confidently wrong with on the `[]func()` corpus ([[E6]]).
Allocating the decoy in reachable rather than dead code is load-bearing: had it been dead,
RTA would exclude it too, RTA would be exact, and the trap would degrade into an ordinary
dead-code decoy. **The oracle is not `'vta`.** Truth is constructional, and the independent
check is `cha == truth ∪ {decoy}`, computed by the real Go toolchain from the rendered
source. Validating against `'vta` would presuppose the experiment's own answer.
**Result:** 63 problems, mean 22.0 functions and 14.0 reachable, exactly one decoy each, and
no decoy leaks into any answer. **The CHA oracle validated 63/63**, so the construction makes
CHA over-approximate by exactly the decoy and nothing else. Probing the three algorithms
directly confirms the intended disagreement: `static` misses the dispatch entirely
(under-approximates), `cha` folds in the decoy (over-approximates), and `vta` matches truth.
**`'vta` is exact on 63/63, on every rung, up to 6 implementors and depth 9.** The
consequence narrows the pre-registration before any budget is spent: **outcome (b) is dead on
this corpus.** The tool will hand the treatment arm an exact fact rather than a bound, so the
−33 point anchoring trap of [[E3]] cannot reproduce here, and the must/may split is not put on
trial by this run. [[E11]] therefore reduces to a clean two-way test: **outcome (a)**, the tool
wins and the criterion generalizes beyond the one pattern it was formulated on, or **outcome
(c)**, control is at ceiling unaided and the criterion is falsified. This is a fact about the
corpus, not about the thesis, which is why this entry is instrumental. It is recorded so that
a later reader who finds no anchoring trap in the [[E11]] results can see the trap was excluded
by construction rather than by luck.
**Raw:** `2026-07-14-pre-composition.d/E22-decoy-corpus.txt`

## E23 — The decoy corpus rescaled to E6, so a control ceiling cannot be blamed on size
**When:** 2026-07-14 06:44
**Bearing:** serves [[E11]]
**Corrects:** [[E22]]
**Inputs:** `generate_interface_dispatch.py --decoy --depths 15 20 25 --impls 2 4 6 --count 7 --seed 31337 --dead 22 --out-dir packages_iface_decoy --output problems_iface_decoy.json`. Reachable-set size is `depth + k + 4`, so the depth band sets answer length and the dead-chain length sets distractor count; both are pinned to [[E6]]'s corpus. Also removes a construction bias present in [[E22]] (see below). This corpus, not [[E22]]'s, is the one [[E11]] runs on.
**Description:** [[E22]] built the decoy corpus correctly but small: 22.0 functions and 14.0
reachable names on average. [[E6]], the corpus whose +35 point result [[E11]] exists to
generalize, was 50 functions with 26.6 reachable. **That gap would have wrecked the
falsifier.** Had control landed at ceiling on the smaller corpus, outcome (c), the criterion
is falsified, would have been indistinguishable from "the corpus was simply easier than
[[E6]]'s", and the notebook's `falsified-by` line would have been unusable at exactly the
moment it mattered. Matching [[E6]] on both axes costs nothing and removes the objection
before anyone can raise it. This entry supersedes [[E22]]'s corpus; the construction, the
oracle discipline (CHA as check, never VTA as oracle) and the reading are unchanged from it.
**The construction bias, fixed.** [[E22]]'s generator applied `sorted()` to the spine sample,
so the decoy's allocation appeared textually *before* the dispatch in every single problem.
Reachability is order-independent, so the sort bought nothing analytically and merely baked a
positional regularity into all 63 problems. It is removed: the allocation now falls before the
dispatch in 37 problems and after it in 26.
**Result:** 63 problems at **mean 50.0 functions (range 43 to 57) and mean 28.0 reachable
(range 21 to 35)**, against [[E6]]'s 50 and 26.6. Neither working-memory load nor answer
length now differs materially between the two corpora; what differs is only the *mechanism* by
which syntax lies, an address-taken function in a `[]func()` literal in [[E6]] against an
allocated type that implements the interface but cannot reach the dispatch site here. **The
CHA oracle validated 63/63**, one decoy per problem, no decoy leaking into any truth set.
**`'vta` remains exact on 63/63**, on every rung, up to 6 implementors and depth 25. So the
[[E22]] reading survives rescaling: **outcome (b) is dead on this corpus**, the tool hands the
treatment arm an exact fact rather than a bound, the [[E3]] anchoring trap cannot reproduce
here, and [[E11]] is a clean two-way test between the tool winning and **the criterion being
falsified by an unaided control ceiling, a verdict that can no longer be dismissed as an
artifact of corpus size.**
**Raw:** `2026-07-14-pre-composition.d/E23-decoy-corpus-rescaled.txt`

## E24 — Correction: an exact algorithm does exist, and we simply do not ship it
**When:** 2026-07-14 07:19
**Bearing:** serves [[E11]]
**Corrects:** [[E11]]
**Inputs:** `golang.org/x/tools/go/pointer` at x/tools **v0.9.0** (Andersen-style, inclusion-based, **field-sensitive** points-to), built under `go1.21.13`, run over all 126 problems of both interface corpora. Each library package rooted by synthesizing `func main() { f0() }`, which does not change what is reachable from `f0`. Also: an audit of which x/tools releases still contain `go/pointer`, and of wile-goast's algorithm menu at `goastcg/prim_callgraph.go:55-86`.
**Description:** [[E11]] asserts, and the `--bound` generator repeated, that "for interfaces
there is **no exact algorithm**." **That is false, and this entry retracts it.** The claim
silently conflated three different things. **(i)** Exact may-reachability is undecidable *in
general*: true, but vacuous, since it holds of every Go program and is not what these corpora
exhibit. **(ii)** No exact algorithm exists *for these corpora*: **false**, as measured below.
Corpus B's truth is trivially computable and a human reads it straight off the page, because
`TaD` is passed only to `hold`, `hold` appends only to `spare`, and nothing ever ranges over
`spare`. **(iii)** No exact algorithm exists *among the ones wile-goast ships*: **true**, and
this is the only load-bearing claim. The menu is exactly `{static, cha, rta, vta}`, which is
also the whole of x/tools' callgraph menu.
**Result:** **Andersen is exact on 126/126**, 63/63 on each corpus, with zero over- and zero
under-approximations. On corpus B specifically: `static` under-approximates, `cha`
over-approximates by the decoy, `rta` returns empty (it needs a `main`, and these are
libraries), `vta` over-approximates by the decoy, and **Andersen alone matches the truth.**
The reason VTA fails is specific and mundane: x/tools' VTA is **field-based**, abstracting
container elements *by type* rather than by variable, so every `[]Ia` in the program shares one
abstract element node, `registry` and `spare` become the same node, and the decoy leaks across.
That is a scalability trade-off in one implementation, not a law of static analysis.
**The fact that actually matters:** `go/pointer` was **removed from x/tools after v0.9.0**
(verified absent from v0.10 through v0.46), so the maintained Go toolkit genuinely tops out at
VTA. But that is an **availability** fact, not a **computability** fact, and the distinction
inverts the reading of corpus B. It is not "a case where the fact cannot be pre-computed." It
is **a case where the fact is perfectly computable and no shipped tool computes it**, which is
not an argument against pre-composition but the strongest argument for it: closing that gap is
precisely what a pre-composition substrate is for.
**What this forces into the open.** "Pre-compute *the fact*" assumes the tool *can* compute the
fact, and that assumption hides two separable claims which have now come apart. **P1, the
method:** hand the agent the exact fact and it beats the agent deriving it (**supported**,
[[E6]], +35 points). **P2, the coverage:** the tool can actually compute the exact fact for the
questions that matter (**never examined**). [[E3]] and corpus B are both **P2 failures**, where
the tool hands over a bound, the agent anchors on it, and does worse than with no tool at all.
The remedy for a P2 failure is to close the coverage gap, not to document the damage; that is
[[E26]]. **What survives:** [[E11]]'s experiment is untouched, because `'vta` *is* exact on
corpus A, so the tool sells a fact there and the criterion still gets a fair test. [[E25]]'s
measurement stands. Only the words "there is no exact algorithm" are withdrawn.
**Raw:** `2026-07-14-pre-composition.d/E24-andersen-correction.txt`

## E25 — Corpus B: the tool is made to sell a bound rather than a fact
**When:** 2026-07-14 07:19
**Bearing:** serves [[E26]]
**Inputs:** New `--bound` mode in `generate_interface_dispatch.py`. `--depths 15 20 25 --impls 2 4 6 --count 7 --seed 31337 --dead 22`, giving 63 problems into `problems_iface_bound.json` and `packages_iface_bound/`. The generator refuses `--decoy --bound` together and discards any problem whose VTA property contradicts its corpus.
**Description:** On corpus A ([[E23]]) `'vta` is exact, so the tool sells the agent a *fact*.
This corpus makes it sell a *bound* instead, holding everything else fixed, so that "does the
tool win" and "does a bound harm its consumer" can be asked separately rather than mixed into
one uninterpretable accuracy number. **The mechanism, measured rather than assumed.** x/tools'
VTA merges container element nodes by type, so two distinct `[]Ia` globals share one abstract
node. Route the real implementors into `registry` (via `register`) and the decoy into `spare`
(via `hold`), and dispatch by ranging over `registry` only. At run time the receiver can never
be `TaD`, so the truth excludes its leaf, but VTA cannot keep the two containers apart. This
is the registry pattern, which is ordinary Go, and the fact that decides the answer lives in
the join of two distant places, exactly as in [[E6]].
**Result:** 63 problems at mean 51.0 functions and 28.0 reachable, matching [[E6]] (50, 26.6)
and corpus A (50.0, 28.0), with one decoy each and no decoy leaking into any truth set. The CHA
oracle validated 63/63. **`'vta` over-approximates on 63/63, by exactly the decoy**, so
`vta == truth ∪ {decoy} == cha`: the tightest answer the shipped tool can give *is* the
over-approximation. The two corpora are therefore matched on every axis (n, functions,
reachable-set size, decoy count) and differ **only in what the tool can sell**: a fact on A, a
bound on B. Note what [[E24]] then establishes about this corpus, and it is not what I first
claimed: the bound is not forced by the problem, it is forced by the analysis we ship. Andersen
resolves all 63 exactly.
**Raw:** `2026-07-14-pre-composition.d/E24-andersen-correction.txt`

## E26 — Close the coverage gap, then measure: expose field-sensitive points-to
**When:** 2026-07-14 07:19
**Bearing:** open
**Inputs:** *Not yet run. Engineering prerequisite, then a three-arm experiment.* **Step 1:** expose an exact points-to-backed call graph in wile-goast, either by vendoring the deleted `go/pointer` (Apache-2.0, x/tools v0.9.0) or by implementing inclusion-based points-to directly, and use it to lift `goastcg/precise.go:66`, which currently returns `nil` for `IsInvoke()` and declines interface dispatch outright. Library packages need a synthetic root, since `go/pointer` requires a `main` and that is why `'rta` returns empty on these corpora. **Step 2:** run corpus B ([[E25]]) with **three arms on the same 63 problems**: control (no tools), treatment-`'vta` (the tool sells a **bound**), and treatment-points-to (the tool sells a **fact**).
**Description:** [[E24]] showed that corpus B's bound is an artifact of the analysis wile-goast
ships, not of the problem. That converts a finding about harm into an **engineering
obligation**, and it is P2, the coverage claim, stated as a task: *make the tool able to compute
the fact.* This entry is that work plus the measurement that would settle what it bought.
**Why the three-arm design is worth more than either corpus alone.** It answers [[E3]]'s
question properly for the first time. [[E3]] found that a returned bound left the model 33
points *below* no tool, but on a corpus that its own harness calls "a rigged tool, not the tool
wile-goast actually ships", at n=15, and the recovery came from a prompt mandate to distrust the
tool rather than from the tool improving. Here the same 63 problems are held fixed and only the
*algorithm behind the tool* moves, which is the one variable [[E3]] could never isolate. If
control < treatment-`'vta` fails and treatment-points-to > control, then **bounds harm, exactness
repairs, and the repair is a tool change rather than a prompt change** — which is the claim
[[E3]] gestured at and could not support. If treatment-points-to does *not* beat control, then
P1 itself is in trouble on interfaces, and that is a far more serious result than anything corpus
B could have produced on its own.
**The arm-1 connection, and it is not decorative.** Inclusion-based points-to is constraint
propagation to a **least fixed point over a points-to lattice**. That is precisely the
fallible-computation category of arm 1, which [[E17]] records as never measured and whose blocker
is that no one could name a task that stays fallible at short answer length. **This is such a
task, and it arrived on its own.** Implementing points-to inside the algebra layer rather than
vendoring it would make [[E17]] concrete and would put the algebra ([[2026-07-14-algebraic-bet]])
on trial at the same time, on a computation whose correctness is independently checkable against
the Andersen oracle of [[E24]]. Which of the two routes to take, vendor or implement, is the open
decision in this entry.

## E27 — The Andersen oracle is exact on generics, but one SSA flag silently switches it off
**When:** 2026-07-14 07:38
**Bearing:** serves [[E26]]
**Status:** retracted
**Retracted-by:** [[E28]]
**Inputs:** Vendored `go/pointer` (x/tools v0.9.0, Apache-2.0) plus the since-deleted `typeparams` package it depends on, lifted into a module built against x/tools v0.46 / go1.25. Probe `vend/genprobe/main.go` prints every `IsInvoke()` call site and the callees Andersen resolves for it, under two SSA builder modes (default, and `ssa.InstantiateGenerics`). Three subjects, each an interface-dispatch problem of the [[E25]] registry/spare shape with one decoy implementor that is stored but never dispatched: `plain` (no generics), `generic` (generic container `Box[T]`), and `hard` (generic container + generic function `Each[T Ia]` + nested `Pair[A,B]` instantiation).
**Description:** [[E26]] proposes to lift `goastcg/precise.go:66` by implementing points-to, and to validate that implementation against the Andersen analysis that [[E24]] established as an exact oracle (126/126 on corpora A and B). That plan has an unexamined dependency, and this entry examines it. `go/pointer` was removed from x/tools in mid-2023 and its code predates the maturity of Go generics, so before any implementation is validated against it, the question must be settled: **does the oracle still tell the truth about generic code?** It matters because corpora A and B happen to contain no generics at all, so [[E24]]'s perfect score is silent on the question, and because real Go — the target the implementation is now scoped to — is full of generics.

The answer has two halves, and the first is a trap rather than a limitation.

**Half one: the failure mode is silence, not a crash.** Built with the default SSA builder mode, generic code contributes **zero** interface-dispatch edges to the Andersen call graph. Not a wrong edge, not a panic, not a diagnostic: the dispatch sites simply are not there, and `pointer.Analyze` returns success. This is an *under*-approximation that presents as a clean result, which is the one failure an oracle may not have. Run against generic code in this configuration, the oracle would have reported a perfect score for an analysis that had quietly stopped analysing, and it would have certified a broken points-to implementation as exact.

**Half two: given the flag, the oracle is exact, and exact in the way that matters.** Built with `ssa.InstantiateGenerics`, Andersen resolves every dispatch site correctly on all three subjects. In `hard`, `registry` and `spare` are two globals of the *same instantiated type* `Box[Ia]`; Andersen keeps them apart and excludes the decoy. That is precisely the discrimination that x/tools' field-*based* VTA cannot make, which is the mechanism [[E24]] identified as the reason VTA over-approximates on corpus B — and it survives monomorphization intact.

**This corrects a claim that was made verbally and never written down**, namely that `go/pointer` is compromised on real code by generics and survives only by the accident that the corpora avoid them. That is wrong in its mechanism, and the truth is both narrower and more useful: the oracle is compromised by **one harness flag**, the flag costs nothing, and with it set the oracle is sound on exactly the language feature it was suspected of failing. [[E24]]'s result stands unchanged; what changes is that its soundness on generics-free corpora was luck rather than design, and the harness must now set the flag explicitly rather than rely on that luck.
**Result:** `plain` resolves `TaA.M` exactly under both modes. `generic` and `hard` resolve **no interface-dispatch edges at all** under the default builder mode (unsound, silent), and are **exact** under `ssa.InstantiateGenerics` — `hard` yielding `TaA.M` and `TaC.M` while correctly excluding the decoy `TaB.M` stored in `spare`. The oracle is therefore usable on real Go, conditional on the flag, and the flag is now a standing requirement of any harness that validates against it.
**Raw:** `2026-07-14-pre-composition.d/E27-andersen-generics.txt`

## E28 — go/pointer cannot analyze real generic Go, so there is no oracle for real code
**When:** 2026-07-14 09:24
**Bearing:** serves [[E26]]
**Corrects:** [[E27]]
**Inputs:** The vendored `go/pointer` oracle (`goastcg/testdata/oracle/`, x/tools v0.9.0) run against **the wile interpreter**, a real Go program carrying 36 generic function declarations, under both SSA builder modes (default, and `ssa.InstantiateGenerics`). `GODEBUG=gotypesalias=0` set, which is required and not sufficient. Contrast with [[E27]], whose subjects were three synthetic packages.
**Description:** [[E27]] claimed the Andersen oracle is exact on generic code provided SSA is built with `ssa.InstantiateGenerics`, and concluded that the oracle "is not compromised for real code; it is compromised by one harness flag, and the flag is free." **That is false, and this entry retracts it.** [[E27]] measured three toy packages and never ran the oracle on a real Go program at all.

Run against wile, the oracle **panics**, in both SSA modes: `internal error in pointer analysis: ForEachElement called on type containing *types.TypeParam`. The cause is structural. `pointer/gen.go:1335` iterates `prog.RuntimeTypes()`, and any type still carrying a `*types.TypeParam` panics `ForEachElement`. `InstantiateGenerics` creates the instantiated copies but does not remove the generic *templates* from the program, so their type-parameter-bearing types reach `RuntimeTypes()` regardless. The panic is inside x/tools, not in vendored code that could be patched.

**Why the toy passed, which is the part worth keeping.** [[E27]]'s subject used a generic container, a generic function, and a nested generic instantiation, and the oracle resolved it exactly. That was an accident of the sample: it never converts a *generic* type into an interface, so no type-parameter-bearing type is ever registered as a runtime type, so `RuntimeTypes()` stays clean and the panicking path is never entered. A narrow class of generic Go therefore does survive `go/pointer`. The condition is invisible from the call site, it is not checkable in advance, and real Go crosses it immediately. It is not something an oracle can be built on.

**What survives, and it is most of the experiment.** Corpora A and B contain no generics, so [[E24]]'s 126/126 is untouched and [[E26]]'s three-arm measurement can still be validated exactly. The shipped tool is also fine: `'cha` and `'vta` build wile's call graph in under half a second (~11.9k nodes). It is the *oracle* that cannot run on real code, not wile-goast.

**What it costs.** There is now **no oracle for real Go.** [[E26]]'s step 6, running the new points-to analysis against real code, has nothing to differentially test against, and correctness there must be established some other way. That gap is open.

**What it buys, and this inverts the route decision.** Implementing points-to ourselves is no longer the *purer* of two available routes. It is the **only** route. The one off-the-shelf exact, inclusion-based, field-sensitive points-to analysis for Go was deleted from x/tools and predates generics badly enough that it cannot run on modern code. So for any Go program using generics, which is most of them, **VTA genuinely is the ceiling of what exists off the shelf.** [[E24]] said the fact is computable and no shipped tool computes it; that was true but too weak. The fact is computable in principle, and the only implementation that ever computed it is dead.

**The process failure is part of the finding.** This error was made twice on the same code, one session apart. The earlier session hit the panic on wile, drew the correct conclusion, and wrote down the lesson: *"I told you vendoring was proven. It was proven on generics-free toy code, and I should have tested it against real Go before saying so."* That transcript was never read after a context clear; the finding was re-derived from a fresh toy; the toy passed; and [[E27]] recorded the inverted conclusion as fact. **The claim a harness cannot exercise is the claim it will not refute.** Any future oracle check runs against real generic Go before it is believed.
**Result:** The oracle **panics on wile in both SSA modes** (`ForEachElement called on type containing *types.TypeParam`, `pointer/gen.go:1335`); `ssa.InstantiateGenerics` does not help. `go/pointer` cannot analyze generic Go. [[E24]]'s 126/126 on the generics-free corpora A and B **stands**, so the [[E26]] experiment is intact, but **no oracle exists for real Go**, and implementing points-to is therefore the only route to the fact rather than the cleaner of two.
**Raw:** `2026-07-14-pre-composition.d/E28-oracle-cannot-do-generics.txt`

## E29 — Andersen on wile: extraction is free, the solve does not converge, and the result is confounded
**When:** 2026-07-14 11:26
**Bearing:** serves [[E26]]
**Inputs:** `scratchpad/csize`, a Go probe that extracts the six constraint forms of `plans/2026-07-14-andersen-pointsto-design.md` from SSA (built with `ssa.InstantiateGenerics`) and solves `lfp(F)` under two schedules, both permitted by the design: chaotic iteration, and difference propagation. Neither uses cycle elimination or hash-value numbering, which the design bars. Target: the wile interpreter, 29,130 functions. Both solvers were validated against the vendored `go/pointer` oracle on all 126 corpus packages before being run on wile. Wile throughput measured separately with a 1,000,000-iteration membership-test benchmark against a 5-element list set.
**Description:** The design's headline risk ([[E26]] step 6) is that a least fixpoint in Scheme cannot run on a real Go program. That risk had never been tested. It was also asserted from the other direction, that whole-program points-to is simply intractable, and that assertion had never been tested either: what actually killed the vendored oracle was a generics panic ([[E28]]), not slowness, at 6 to 11 milliseconds. This entry measures the thing both claims are about.

The measurement decomposes into three numbers. **Problem size** on wile, which needs a constraint extractor. **Propagation volume**, the count of set-membership tests the fixpoint performs, which is schedule-dependent but algorithm-intrinsic and is the unit of work a Scheme solver must also pay. And **Wile's throughput** on that unit, which converts one into the other.

**Extraction is free, and that half of the seam is settled.** 146 ms to turn wile into 755,947 constraints over 737,842 nodes and 47,809 objects. The `gep` form dominates at 52%, which is field sensitivity doing its job.

**The solve does not converge.** In *Go*, not Scheme. Chaotic iteration times out at 5 minutes having performed 25.6 billion membership tests. Difference propagation, the schedule optimization the design explicitly permits, does not rescue it: it times out at 10 minutes at 31.4 billion tests, and it fails *further into the blowup*, with mean points-to set size climbing to 527 and still growing at the cutoff. Wile runs the same operation at roughly 838,000 tests per second, about 62 times slower than Go, so 31.4 billion tests projects to about 10.4 hours, for a run that did not finish. For the Scheme solver to be viable, the work must fall by three to four orders of magnitude.

**Three bugs had to be fixed first, and each produced a plausible wrong answer rather than an error.** Globals were given no objects, so corpus B, which is entirely about two globals, resolved zero dispatch sites. A first solver treated `load` and `store` as static, copying `pts(o)` into `p` when `o` entered `pts(q)` but never recording the edge `o → p`, so later growth of `pts(o)` never reached `p`; it under-approximated, and it *converged in 3.3 seconds*, which is precisely how a wrong answer flatters you. And `append` is variadic and allocating: `registry = append(registry, v)` packs `v` into a temp array and appends a *slice*, so the appended element's provenance flows only if the varargs' contents are loaded and then stored, and since `var registry []Ia` starts nil with no `MakeSlice` anywhere, the backing array is born at the `append` itself.

**The result is confounded, and the confound is not a footnote.** Mean points-to set size is 527 on wile against about 5 on the corpora, a hundredfold gap that is more likely my imprecision than Andersen's intrinsic price. `*ssa.Extract` currently copies the *whole tuple*, merging every return value of every multi-return call, and Go is saturated with `(v, err)`. Indirect calls through function values emit a bogus load and never wire parameters. Closures are barely modelled. **The 126/126 validation does not cover any of this**: the corpora contain no closures, no tuples, and no indirect calls, which are exactly the constructs the model is crudest at. Solve cost is superlinear in set size, so precision loss compounds into cost. This entry therefore does **not** establish that Andersen is too slow for wile, and it does **not** establish that the algebra fails as an implementation substrate. It establishes that *this* analysis, at *this* precision, does not converge. Recording it as anything stronger would repeat the error [[E27]] was retracted for: concluding about a thing from a harness that never exercised it.

**What the reference implementation does instead, and the question it raises.** `pointer/doc.go` states that `go/pointer` does *not* perform online cycle detection. Its tractability comes from HVN (`pointer/hvn.go`; Hardekopf and Lin, SAS'07), a **pre-solver** pass that identifies *pointer-equivalent* variables and collapses them before the fixpoint runs. So the optimization this design bars is exactly the one the only working reference implementation depends on. But HVN quotients the constraint graph by an equivalence relation, and **if that relation is a congruence with respect to the transfer functions, then solving on the quotient and lifting back is a standard algebraic construction**, quotient by a congruence, and therefore operator-instantiation at a coarser carrier rather than a hand-written trick. It would buy tractability *without* forfeiting the bet. Whether pointer-equivalence is a lattice congruence is a checkable question and nobody in this project has asked it.
**Result:** Extraction of wile costs **146 ms** and yields 755,947 constraints / 737,842 nodes / 47,809 objects. The solve **does not converge in Go** under either permitted schedule: chaotic iteration times out at 5 min (25.6e9 membership tests, mean |pts| 78.3), difference propagation times out at 10 min (31.4e9 tests, 135.8M propagations, 7.65M copy edges, mean |pts| **526.9 and still climbing**). Wile sustains ~838,000 membership tests/sec (~62× slower than Go), projecting the non-converged 31.4e9 tests to **~10.4 hours**. Both solvers agree with the `go/pointer` oracle **126/126** on the corpora. **The result is confounded**: mean |pts| of 527 on wile versus ~5 on the corpora points at unvalidated imprecision in `*ssa.Extract` (whole-tuple copy), indirect calls, and closures, none of which the corpora exercise. No cost conclusion is attributable to the algebra until that is fixed and re-measured.
**Raw:** `2026-07-14-pre-composition.d/E29-andersen-cost-on-wile.txt`

## E30 — The E29 confound is dead: the cost is intrinsic, not my imprecision
**When:** 2026-07-14 11:57
**Bearing:** serves [[E26]]
**Inputs:** `scratchpad/csize` v2, which repairs every precision defect [[E29]] named as its confound: `*ssa.Extract` now yields **per-component** tuple nodes rather than merging a call's whole result (Go is saturated with `(v, err)`); functions and closures are **objects**, so a func value carries provenance; indirect calls through a func value get a real dynamic `icall` constraint that wires parameters to arguments, replacing the bogus load v1 emitted; and closure bindings flow to the callee's **free variables**. Re-validated against the vendored `go/pointer` oracle on all 126 corpus packages, then re-run on wile under difference propagation, the best schedule the design permits.
**Description:** [[E29]] measured that a whole-program inclusion-based points-to solve does not converge on wile, and then refused to draw a conclusion from it, because the extractor was crudest exactly where the validation corpora were silent. The corpora contain no tuples, no closures, and no indirect calls. Mean points-to set size was 527 on wile against about 5 on the corpora, a hundredfold gap that could plausibly have been entirely my own imprecision, and solve cost is superlinear in set size, so imprecision compounds into cost. This entry removes that alternative explanation.

**It was not imprecision.** With every defect fixed, and with correctness *unchanged* at 126/126 against the oracle, mean set size falls from 526.9 to 486.1. That is 7.8%. Viability of a Scheme solver required a drop of roughly two orders of magnitude. The membership-test count does not fall at all; it rises slightly, to 32.3 billion, and the solve still times out at ten minutes **in Go**.

So the blowup is a property of the analysis, not of my rendering of it. **Context-insensitive inclusion-based points-to on wile does not converge when the solve is restricted to `lfp`-preserving schedules alone**, which is precisely the restriction `plans/2026-07-14-andersen-pointsto-design.md` imposes so that any result would speak to the algebraic bet rather than to hand-written solver engineering.

The reason no schedule can rescue it is that **the solution itself is large**: about 486 objects per node across 310,862 nodes is 151 million points-to pairs, still growing at the cutoff. Difference propagation, the best permitted schedule, does not merely fail; it reaches the timeout *further into* the blowup than naive chaotic iteration does. The work is not being wasted on a poor visit order. The answer is simply big.

**What this does not settle, and the distinction matters twice over.** It does not show that exact points-to on real Go is impossible: `go/pointer` did it for years. Its tractability comes from HVN (`pointer/hvn.go`; Hardekopf and Lin, SAS'07), a **pre-solver** pass that identifies *pointer-equivalent* variables and collapses them before the fixpoint runs, and `pointer/doc.go` records that it does not even perform online cycle detection. HVN is the whole game, and it attacks exactly the quantity that is killing us: it shrinks the node count by merging nodes that provably share a points-to set. And it does not touch any **accuracy** claim. Both notebooks' theses are about accuracy. This is a cost result: it says the fact is expensive to precompute, and says nothing about whether precomputing it buys the agent reliability. [[E26]]'s three-arm experiment runs on the corpora and is unaffected.

**The decisive question is now on the critical path, and it is checkable.** The design bars HVN because a constraint-graph rewrite looked like solver engineering rather than operator-instantiation. That judgment is now the single load-bearing assumption in the plan, and it may be wrong. HVN quotients the constraint graph by an equivalence relation. **If that relation is a congruence with respect to the transfer functions**, then solving on the quotient and lifting back is quotient-by-a-congruence: a standard algebraic construction, which is to say *instantiating an existing operator at a coarser carrier*, which is the bet's own stated promise. HVN would then not be a defection from the algebra; it would **be** the algebra, and adopting it would buy tractability at no cost to the measurement. If the relation is not a congruence, the design's line was right, the algebra cannot compute this query on real Go under its own rules, and that is a genuine count against the bet, the second after `(wile algebra cfl)`. Tracked as [[2026-07-14-algebraic-bet/C6]].
**Result:** With every [[E29]] precision defect repaired and oracle agreement unchanged at **126/126**, mean |pts| falls only **526.9 → 486.1 (−7.8%)**, membership tests **rise** to 32.3e9, and the solve still **times out at 10 minutes in Go** (849,982 nodes / 76,938 objects / 831,965 constraints; 151.1M propagations; 8.46M copy edges; solution still growing at cutoff). Viability needed ~100×; it got 1.08×. **The confound is eliminated: the cost is intrinsic to context-insensitive inclusion-based points-to on this program, not an artifact of the extractor.** No permitted schedule can help, because the solution itself is 151M+ points-to pairs. The design's bar on HVN is therefore the load-bearing assumption, and whether HVN's pointer-equivalence is a lattice congruence now decides the route.
**Raw:** `2026-07-14-pre-composition.d/E30-confound-dead-cost-intrinsic.txt`

## E31 — Representation, not algebra: the 32 billion operations are the container, not the fixpoint
**When:** 2026-07-14 12:38
**Bearing:** open
**Corrects:** [[E30]]
**Inputs:** *Declared, not yet run.* Replace the points-to set representation in `scratchpad/csize` with a **sparse bit vector**, leaving everything else fixed: the same six constraint forms, the same `lfp(F)`, the same difference-propagation schedule, no HVN, no cycle elimination. Re-validate against the `go/pointer` oracle on the 126 corpus packages, then re-run on wile and compare wall time and operation count against [[E30]]'s `map[int]bool` baseline. Source of the hypothesis: `pointer/doc.go` in the vendored oracle, and `pointer/hvn.go`'s PERFORMANCE section.
**Description:** [[E30]] eliminated two explanations for why the whole-program solve does not converge on wile. It was not extractor imprecision: repairing every known defect moved mean set size by 7.8% while oracle agreement held at 126/126. It was not the schedule: difference propagation, the best schedule the design permits, reaches the timeout further into the blowup than naive chaotic iteration does. Those two eliminations stand. What [[E30]] then concluded from them, that "the cost is intrinsic," **overreached**, because a third explanation was never considered and it is the one the reference implementation names out loud.

**`go/pointer` represents points-to sets as sparse bit vectors.** Its own documentation says so, and says why: *"similar to those used in LLVM and gcc, which are more space- and time-efficient than sets based on Go's built-in map type."* The solver in [[E30]] represents points-to sets as `map[int]bool`, which is precisely the representation that sentence names as the slow one. So the 32.3 billion "membership tests" are an artifact of the **container**, not a property of the fixpoint. Under a bit vector you do not perform per-element membership tests at all: a set union is a word-parallel OR. At the measured mean set size of 486, that is roughly 8 word operations where the map performs 486 hash lookups, and each word operation is far cheaper than a lookup.

The distinction the entry turns on: **the solution size is intrinsic, the operation count is not.** 151 million points-to pairs is what the analysis means, and no representation avoids computing it. But 151 million pairs is about 19 MB of bits. That is not an intractable object. It is an object for which a hash map is the wrong container, chosen by me and not by the mathematics.

**This also demotes HVN, which [[E30]] put on the critical path.** `pointer/hvn.go`'s own PERFORMANCE section reports that HVN eliminates about two thirds of nodes and two thirds of constraints, and reduces **solving time by 30 to 42 percent**, against about 15 percent paid to run HVN itself. Net, call it 1.3x. The gap that must be closed is roughly 100x. HVN was never the lever, and [[E30]] promoted it without reading the numbers underneath it. The question of whether HVN is algebraic remains interesting, and is answered in [[2026-07-14-algebraic-bet/C7]], but it is not the route to tractability.

**Why this is worth running rather than assuming.** It is the cheapest remaining experiment (one container swap, no change to the algorithm, the schedule, or the constraint forms), it is validated by an oracle that already exists, and it is decisive in both directions. If the bit vector converges, then [[E30]]'s blowup was mine and not Andersen's, the whole-program route is alive, and the design's risk section is wrong. If it still does not converge, then the cost really is intrinsic, that claim will have survived the three explanations that could have defeated it, and the design must pivot to demand-driven CFL or abandon real-code coverage.

**And it relocates the risk for the Scheme side, which is the part that matters to the thesis.** The operator laws say nothing about how a lattice element is stored. Choosing a sparse bit vector over a hash set is a choice strictly *below* the algebra, invisible to the composition rules, and it cannot forfeit the bet however clever it gets. But Wile's native set is a **list**, and for this carrier a list is the worst available choice. So the sharp question for `(wile goast andersen)` was never whether the algebra is tractable. It is whether `P(Object)` can be *represented* efficiently inside Wile, and that is a representation question wearing an algebra question's clothes.
