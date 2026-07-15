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
**Bearing:** serves [[E26]]
**Corrects:** [[E30]]
**Inputs:** *Declared, not yet run.* Replace the points-to set representation in `scratchpad/csize` with a **sparse bit vector**, leaving everything else fixed: the same six constraint forms, the same `lfp(F)`, the same difference-propagation schedule, no HVN, no cycle elimination. Re-validate against the `go/pointer` oracle on the 126 corpus packages, then re-run on wile and compare wall time and operation count against [[E30]]'s `map[int]bool` baseline. Source of the hypothesis: `pointer/doc.go` in the vendored oracle, and `pointer/hvn.go`'s PERFORMANCE section.
**Description:** [[E30]] eliminated two explanations for why the whole-program solve does not converge on wile. It was not extractor imprecision: repairing every known defect moved mean set size by 7.8% while oracle agreement held at 126/126. It was not the schedule: difference propagation, the best schedule the design permits, reaches the timeout further into the blowup than naive chaotic iteration does. Those two eliminations stand. What [[E30]] then concluded from them, that "the cost is intrinsic," **overreached**, because a third explanation was never considered and it is the one the reference implementation names out loud.

**`go/pointer` represents points-to sets as sparse bit vectors.** Its own documentation says so, and says why: *"similar to those used in LLVM and gcc, which are more space- and time-efficient than sets based on Go's built-in map type."* The solver in [[E30]] represents points-to sets as `map[int]bool`, which is precisely the representation that sentence names as the slow one. So the 32.3 billion "membership tests" are an artifact of the **container**, not a property of the fixpoint. Under a bit vector you do not perform per-element membership tests at all: a set union is a word-parallel OR. At the measured mean set size of 486, that is roughly 8 word operations where the map performs 486 hash lookups, and each word operation is far cheaper than a lookup.

The distinction the entry turns on: **the solution size is intrinsic, the operation count is not.** 151 million points-to pairs is what the analysis means, and no representation avoids computing it. But 151 million pairs is about 19 MB of bits. That is not an intractable object. It is an object for which a hash map is the wrong container, chosen by me and not by the mathematics.

**This also demotes HVN, which [[E30]] put on the critical path.** `pointer/hvn.go`'s own PERFORMANCE section reports that HVN eliminates about two thirds of nodes and two thirds of constraints, and reduces **solving time by 30 to 42 percent**, against about 15 percent paid to run HVN itself. Net, call it 1.3x. The gap that must be closed is roughly 100x. HVN was never the lever, and [[E30]] promoted it without reading the numbers underneath it. The question of whether HVN is algebraic remains interesting, and is answered in [[2026-07-14-algebraic-bet/C7]], but it is not the route to tractability.

**Why this is worth running rather than assuming.** It is the cheapest remaining experiment (one container swap, no change to the algorithm, the schedule, or the constraint forms), it is validated by an oracle that already exists, and it is decisive in both directions. If the bit vector converges, then [[E30]]'s blowup was mine and not Andersen's, the whole-program route is alive, and the design's risk section is wrong. If it still does not converge, then the cost really is intrinsic, that claim will have survived the three explanations that could have defeated it, and the design must pivot to demand-driven CFL or abandon real-code coverage.

**And it relocates the risk for the Scheme side, which is the part that matters to the thesis.** The operator laws say nothing about how a lattice element is stored. Choosing a sparse bit vector over a hash set is a choice strictly *below* the algebra, invisible to the composition rules, and it cannot forfeit the bet however clever it gets. But Wile's native set is a **list**, and for this carrier a list is the worst available choice. So the sharp question for `(wile goast andersen)` was never whether the algebra is tractable. It is whether `P(Object)` can be *represented* efficiently inside Wile, and that is a representation question wearing an algebra question's clothes.

**Result:** **REFUTED.** Run 2026-07-14 14:00–14:53. The container is not the lever, and the ranking is the *opposite* of the one predicted above: the hash map is the **fastest** of three carriers, and none of the three converges.

Three arms, one variable. The extractor (`main.go`) is byte-for-byte identical across all three (`diff -q` verified), as are the six constraint forms, `lfp(F)`, and the difference-propagation schedule. No HVN, no cycle elimination. All pinned to `x/tools` v0.42.0. **All three validated 126/126 against the `go/pointer` oracle before being run on wile** — mandatory, because [[E29]]'s bug #2 under-approximated and *converged in 3.3 seconds*, and a fast wrong answer is the failure mode this experiment is most exposed to. The map arm reproduces [[E30]]'s extraction exactly (849,982 nodes / 76,938 objects / 831,965 constraints), which is what licenses calling it the [[E30]] baseline.

On wile (29,151 functions / 850,295 nodes / 77,001 objects / 832,233 constraints), same 10-minute budget, same machine, one arm at a time. Since no arm reaches `lfp`, the only sound cross-arm progress metric is **total pts pairs** — the Kleene chain is monotone increasing, so more pairs means further along.

| | map (`map[int]bool`) | bitset (node ids) | bitset (dense object ids) |
|---|---|---|---|
| status | TIMED OUT | TIMED OUT | TIMED OUT |
| **total pts pairs** | **158,992,115** | 76,840,622 | 142,764,258 |
| mean `\|pts\|` | 509.28 | 258.01 | 460.86 |
| dispatch resolved | 121,628 | 79,207 | 104,095 |
| peak RSS | 8.61 GB | 2.65 GB | **2.18 GB** |
| instructions retired | 8.12e12 | 1.94e12 | 2.82e12 |
| IPC | 3.31 | 0.76 | 1.06 |

A third arm was added that this entry did not declare, and it was added because the second arm indicted itself. The naive swap measured **4.84 bits set per 256-bit block** (1.9% occupancy): `main.go` draws value nodes and object nodes from one counter, so the 77,001 object ids are sprayed across the entire 850,295-wide node space, and a bit vector indexed by node id is ~90% structurally guaranteed empty. A bit vector only beats a hash map when co-occurring elements are **numerically adjacent**, so arm 2 imported the container without the numbering that makes it work. Arm 3 compacts the element space (a bijection: same lattice, same fixpoint, same schedule, still 126/126), and it does everything it was supposed to do — 9.41× compaction, block occupancy up to 21.70, **2.6 bytes per pair, four times less memory than the hash map**. It still propagates 10% fewer pairs than the map, and it still does not converge.

**Why the bitset loses, which is the actual finding: sparse bitsets and difference propagation are antagonistic.** `intsets` holds blocks in a sorted linked list, so `Difference(delta, pts[dst])` + `UnionWith` is a merge walk costing `O(blocks(delta) + blocks(pts[dst]))` — dominated by the size of the **target**, not the delta. The hash map costs `O(|delta|)` and never touches the target's other elements. So the bitset wins only when `|pts[dst]|/256 < |delta|`, and **difference propagation exists precisely to make deltas tiny**. At mean `|pts|` = 461 (~21 blocks) the bitset walks ~21 blocks, one cache miss each, to insert the two or three elements the map inserts in two or three hash ops. The counters say it outright: the dense arm retires **2.9× fewer instructions** and gets **10% less done** in the same cycles — IPC 1.06 against 3.31. It is not compute-bound, it is stalled on memory. The better the schedule, the worse the bitset's relative position; this entry assumed the two optimizations would compound, and they are subtractive.

`go/pointer` escapes this because its points-to sets are *small* (single digits), so `blocks(pts) ≈ 1` and the merge walk is free. It did not pick a magic container: it runs HVN first, on non-adversarial targets, which keeps `|pts|` small enough for the container to pay. **The container was never doing the work this entry credited it with.** The error above is precise and worth naming: it reasoned from a sentence in `pointer/doc.go` ("more space- and time-efficient than ... map") to a 100×, without asking what that efficiency is a *function of*. It is a function of `|pts|` and of adjacency, and on this target both are hostile.

**The Wile projection, which was the point.** The storage claim in the `Description` above is true and irrelevant — the dense arm confirms it at 2.6 bytes/pair. The *operation* count is untouched: the dense arm performs **1,586,229,172 set unions** in 600 s (2.6M/sec). Wile runs ~62× slower ([[E29]]), so ~42k unions/sec puts those same unions at **~10.5 hours** — for a run that still has not converged. Representation buys a 4× memory win and **zero** operations. It does not touch a 100× gap.

So [[E30]]'s core claim **survives a third elimination**: the blowup is not extractor precision, not the schedule, and not the carrier's representation. What this entry corrects in [[E30]] is narrower than it claimed: [[E30]] had indeed not considered the container, and the question was cheap and decisive — but it falls [[E30]]'s way.

**Two bounds on the claim, both load-bearing.** First, this says nothing about *accuracy*; this notebook's thesis and [[2026-07-14-algebraic-bet]]'s are both accuracy theses, and this is a cost result — hence `serves [[E26]]`, as with [[E29]] and [[E30]]. Second, **the target is adversarial**: wile is a Scheme interpreter (one `eval`, a universal `Value` type, generic environments), and a mean points-to set of ~500 is what context-*insensitivity* does to an interpreter. The supported claim is "whole-program context-insensitive Andersen does not converge on wile within 10 minutes under any of three representations," **not** "Andersen is intractable on Go." A less self-referential Go program may converge; that is untested.

The solution is large because the analysis is **imprecise** on this program, and the remedy for imprecision is neither a container nor a schedule. What remains is to stop materializing the whole solution — demand-driven CFL-reachability, which the `Description` above pre-registered as the fallback if the container failed. It failed. HVN is not the alternative either: [[2026-07-14-algebraic-bet/C7]] already priced it at a net ~1.3× against a ~100× gap.

**Raw:** `2026-07-14-pre-composition.d/E31-representation-refuted.txt`

## E32 — 5c: the target is not adversarial; ordinary Go blows up identically
**When:** 2026-07-14 15:40
**Bearing:** serves [[E26]]
**Inputs:** The map-carrier solver from [[E31]] (its fastest arm), run at a 60-second budget with `ssa.InstantiateGenerics` over seven real Go programs: two interpreters (wile, and wazero, a WebAssembly runtime) and five ordinary programs (ollama, etcd, kopia, kubectl, rclone). Metric: mean `|pts|` and convergence at the cutoff. Design step 5c.
**Description:** [[E31]] closed by bounding its own result: *"whole-program context-insensitive Andersen does not converge on **wile**"*, with an explicit caveat that wile is a Scheme interpreter (one `eval`, a universal `Value` type, generic environments) and *"a less self-referential Go program may well converge; that is untested."* This entry runs that untested control — and adds a second interpreter (wazero) so the experiment discriminates *interpreter* from *ordinary program* rather than just re-testing wile.

**The caveat is refuted.** At a 60-second budget, **all seven programs time out**, ordinary CLIs and servers exactly like the two interpreters. Mean `|pts|` at the cutoff runs 52 (kubectl) to 243 (wazero), with the ordinary programs (ollama 150, etcd 151, rclone 88, kopia 69) squarely inside the interpreters' range (wile 170, wazero 243), not below it. There is no interpreter/ordinary split. Whatever makes the solve intractable is a property of context-insensitive Andersen on real Go, not a property of interpreters and not a property of wile.

**And the head-to-head that would calibrate this is impossible.** Running `go/pointer` (the oracle) on any of these seven to compare mean `|pts|` fails on every one: a generics panic, or `*types.Alias`, or a nil dereference, even with `GODEBUG=gotypesalias=0`. This is the design's own stated hole — *there is no oracle for real Go* — biting exactly where it would be most useful. So E31's cost claim generalizes from wile to real Go, but the question of whether that cost is *necessary* (does the reference implementation also produce sets this large?) cannot be answered with the oracle on real targets.

What this corrects in [[E31]] is only the *scope* of its bound, and E31 flagged that bound as untested, so this is the resolution E31 asked for, not a reversal. E31's core finding (the container is not the lever; the hash map wins) is untouched.
**Result:** All 7 targets TIMED OUT at 60s. Mean `|pts|` @60s: wazero 243.33, wile 169.62, etcd 150.50, ollama 150.33, rclone 88.01, kopia 68.84, kubectl 52.64. No interpreter/ordinary separation. `go/pointer` cannot run on any of them. **Raw:** `2026-07-14-pre-composition.d/E32-5c-not-adversarial.txt`

## E33 — The blowup has a signature, and it is none of the levers tried: context-insensitive hot-allocator flooding
**When:** 2026-07-14 15:55
**Bearing:** serves [[E26]]
**Inputs:** Three probes on the map-carrier solver, all preserving 126/126 oracle agreement. (1) Added the two missing constraint forms — `typeFilter` and `untag` — mirroring `go/pointer`'s `typeAssert(T, dst, src, exact=true)` (`pointer/gen.go:388`), and re-measured wile. (2) A points-to-set-size histogram plus the single most-pointed-to object, on wile @60s. (3) Tarjan SCC over the static copy (subset) graph — Phi, Convert, ChangeType, ChangeInterface, Slice, static-call parameter bindings — on wile, ollama, etcd, kubectl.
**Description:** [[E32]] established that context-insensitive Andersen does not converge on real Go generally. This entry asks *why*, and eliminates every remaining candidate lever except two that the design does not currently reach.

**Probe 1 — a genuinely missing constraint form, which is nonetheless not the cause.** `go/pointer` has eight constraint kinds; the design (§5) has six. The two absent ones both *filter* by dynamic type: `typeFilter` (`x.(Iface)` keeps only tagged objects assignable to the interface) and `untag` (`x.(Concrete)` keeps only the payload of type-exact objects). The extractor instead passed every type assertion through as an unfiltered copy, so `x.(*Foo)` propagated the entire interface points-to set rather than just the `*Foo`-typed objects — 3,949 such sites on wile, up to 11,088 on kopia. Adding both forms (our `MakeInterface` already carries the tagged-object model the filter needs) preserved 126/126 and improved precision — wile mean `|pts|` 169.62 → 148.54, dispatch edges 74,145 → 55,427 — but the solve still times out. A 12% precision gain, not a convergence fix. It also refines [[E30]]: E30 declared "the confound is dead" after fixing four extractor defects; there was a fifth, invisible to the dispatch-only corpora, and fixing it still does not change E30's conclusion.

**Probe 2 — the blowup is concentrated, not uniform.** On wile @60s (47.9M pairs over 322,315 non-empty nodes): 238,550 nodes have `|pts| ≤ 1` and are essentially exact, while ~20,700 nodes (6%) carry sets of ~2,000–4,000 elements and hold nearly the entire mass. The single most-pointed-to object is one `*errors.errorString`, present in the points-to sets of **28,671 distinct nodes**. That is one allocation site (`errors.New`/`fmt.Errorf` returns `&errorString{...}`) yielding one abstract object under context insensitivity, which then flows to every error-typed value in the program. This is the classic Andersen hot-allocator flood: the sets are large because a handful of hot allocation sites each collapse to a single abstract object that reaches everywhere.

**Probe 3 — it is not copy-cycles, so cycle elimination is not the lever.** Cycle elimination collapses strongly-connected components of the copy graph. The static copy graph is nearly acyclic: largest SCC 21 (wile) to 37 (ollama/etcd/kubectl), with only 1–3% of nodes in any cycle. Cycle elimination would collapse almost nothing.

**What is now eliminated as the lever:** container/representation ([[E31]]), schedule ([[E30]]), extractor precision ([[E30]]), the type filter (here), and cycle elimination (here). What remains touches the *set size at its source*, and neither option is a container or a schedule: **context sensitivity** (heap cloning gives each call site its own abstract object — the direct cure for the `errorString` flood, and expensive), or **HVN** *if* the ~20,700 god-set nodes are pointer-equivalent (HVN's equivalence is broader than cycle-equivalence, so probe 3 does not rule it out; confirming it needs an offline HVN implementation). If they are equivalent, HVN stores the god-set once rather than ~20,000 times — a storage collapse far larger than the 1.3× solve-time figure [[2026-07-14-algebraic-bet/C7]] quoted, which was measured on inputs that already fit. HVN is algebraic (a sublattice restriction; [[2026-07-14-algebraic-bet/C7]]), so adopting it need not forfeit the bet.
**Result:** Type filter added (126/126 preserved), wile mean `|pts|` 169.62 → 148.54 — not a convergence fix. Blowup concentrated in ~20,700 nodes with ~3,000-element sets; one `*errors.errorString` reaches 28,671 nodes. Copy graph nearly acyclic (largest SCC ≤ 37), so cycle elimination is not the lever. Remaining candidates: context sensitivity, or HVN if the god-set nodes are pointer-equivalent (untested). **Raw:** `2026-07-14-pre-composition.d/E33-blowup-signature.txt`

## E34 - The god-set nodes are not pointer-equivalent: offline HVN collapses them ~2x, not to a handful
**When:** 2026-07-14 16:35
**Bearing:** serves [[E26]]
**Inputs:** A faithful offline HVN (Hardekopf & Lin, SAS'07; a port of `go/pointer`'s `hvn.go`), built over [[E31]]'s eight-form constraint representation in `E34-hvn/`. It reuses E31's frozen extractor and difference-propagation solver unchanged and adds `hvn.go`: a two-pass computation (iterative Tarjan SCC over the offline graph, then a topological sweep assigning each SCC one pointer-equivalence label from its indirect flag and the labels reaching it along real edges), semantically equivalent to `hvn.go`'s online coalescing. It labels only; it never renumbers or re-solves. Validated for soundness on a converged 153,502-node fixture exercising every form the flood flows through (interfaces, errors, type assertions, closures, field store/load, phi chains). Run on three of [[E33]]'s targets (wile, etcd, ollama) at a 60-second solve budget.
**Description:** [[E33]] eliminated every candidate lever for the Andersen blowup except two, and left one untested: HVN, *if* the ~20,700 god-set nodes are pointer-equivalent (identical points-to sets provable from graph topology alone). E33 could not settle it, because its probe 3 (Tarjan SCC over the copy graph) tests only cycle-equivalence, while HVN's pointer-equivalence is strictly broader: it also merges DAG-equivalent nodes, those combining the same set of predecessor labels with no cycle between them. So "the copy graph is acyclic" does not imply "HVN cannot help." This entry builds the offline HVN and measures the collapse directly.

**No oracle, so the validation checks soundness instead.** `go/pointer` cannot run on any real target ([[E32]]: generics panic, `*types.Alias`, nil deref), so the head-to-head that would validate the PE-class count is impossible. But HVN's defining property is oracle-free: two nodes share a PE label if and only if they must have identical points-to sets, so on a *converged* solve every PE class must be uniform in the solved pts. That is the exact analog of the 126/126 dispatch gate, run against the solver's own fixpoint. On the fixture (converged, 84 ms) all 14,952 multi-member PE classes are pts-uniform: zero over-merges, zero non-pointer floods. The check earned its keep by catching one genuine bug first (the [[E29]] failure mode, a plausible wrong answer): 10 closure/thunk parameters were under-marked indirect, because a closure wires its params through the `MakeClosure` value node rather than through the function's own value node, so the address-taken test missed them. Under-marking over-merges, and over-merge *inflates* an "HVN collapses the god-set" reading, the dangerous direction. Fixed (a function's params are captured offline only if it is exclusively a static callee) and re-validated clean before any real target was measured.

**The god-set does not collapse, on all three targets.** The single most-pointed-to object is `*errors.errorString` everywhere. Its carriers collapse only ~2x: wile 28,662 -> 13,557 (2.11x), etcd 42,443 -> 14,839 (2.86x), ollama 37,511 -> 14,416 (2.60x). The fat-set (|pts| >= 1024, E33's ~20,700 nodes) collapses 1.65x-1.91x. HVN stores the flood set ~12,500-14,800 times, not once. The nodes are provably not pointer-equivalent: distinct pointers sharing a large *overlapping* pts because of the flood, not identical pts. Two structural reasons, both in the numbers: (1) an indirect floor, 18-29% of carriers flow through the constructs HVN marks indirect (interface unwrap, invoke/icall R-blocks, dynamically-wired params, address-taken struct fields), each getting a fresh unmergeable label; this floor is a pure function of the constraint graph and no HVN tuning can lower it. (2) Fan-in diversity: even the direct carriers fragment, because HVN merges two nodes only when they receive the *exact* same set of pointer sources, and errors reach their carriers through many different source combinations.

**C7 stands; this confirms it rather than overturning it.** E33 speculated HVN might store the god-set "once rather than ~20,000 times, a storage collapse far larger than the 1.3x solve-time figure [[2026-07-14-algebraic-bet/C7]] quoted, which was measured on inputs that already fit." That speculation is refuted: the collapse is ~2x on inputs that do *not* fit, the same order as C7's 1.3x (the gap between the two is storage-count versus solve-time, both far from the ~100x / non-convergence gap). HVN is not broken, global node reduction is 54-59%, matching the H&L paper's ~2/3; it simply does not *concentrate* its benefit on the god-set, which is the single weakest place for it. And a 2x node reduction does not change convergence: the fixpoint still propagates the huge sets to ~13,000 representatives and the errorString object still floods. With HVN eliminated, the only remaining lever is context sensitivity (heap cloning / k-CFA / object sensitivity), which gives `errors.New`'s result a per-call-site identity so the flood never forms, attacks the god-set at its source, and is the expensive real fix.
**Result:** Offline HVN, validated sound on a converged 153,502-node fixture (14,952 PE classes all pts-uniform; one under-marking bug found and fixed first). On wile/etcd/ollama the `*errors.errorString` god-set collapses only ~2.1x/2.9x/2.6x (28,662->13,557, 42,443->14,839, 37,511->14,416), not to a handful; 18-29% of carriers are structurally indirect and unmergeable. HVN refuted as the lever; C7's ~1.3x order confirmed. Full elimination now: representation ([[E31]]), schedule + extractor precision ([[E30]]), type filter + cycle elimination ([[E33]]), HVN (here). Only context sensitivity remains. **Raw:** `2026-07-14-pre-composition.d/E34-hvn/E34-hvn-diagnostic.txt`

## E35 - Demand-driven points-to: interface discovery is cheap for the median dispatch site, flood-sized for the error-typed tail
**When:** 2026-07-14 18:02
**Bearing:** serves [[E26]]
**Inputs:** A demand-driven, field-sensitive, context-insensitive points-to engine in `E35-cflreach/`, reusing [[E31]]'s frozen extractor and batch difference-propagation solver (the equivalence oracle) unchanged and adding `cfl.go` (the demand engine) + `slice.go` (the footprint measurement). Context-insensitive field-sensitive CFL-reachability degenerates to the Andersen relation, so the engine computes the same relation as the batch solver, seeded from a query set and propagated only among activated nodes. Validated by the equivalence theorem `demand-pts(v) == batch-pts(v)` (any mismatch is a bug) against the batch solver on three families: the 126-package interface-dispatch benchmark (`packages_iface_decoy`), the converged 153k-node `soundfix` fixture, and the real targets wile/etcd/ollama at a 60-second batch budget.
**Description:** [[E31]] pre-registered "stop materializing the whole solution, demand-driven CFL-reachability" as the fallback, and [[E34]] closed HVN, the last cheap *batch* lever. This entry builds the demand engine and asks [[E33]]'s standing question: interface object discovery is a set of point queries (one per dispatch site), not a whole-program fixpoint, so does querying only dispatch sites sidestep the `errors.errorString` flood that makes batch context-insensitive Andersen time out?

**The engine is exact on the benchmark.** The equivalence theorem is the test oracle: an activation-closure bug under-approximates and the check fires, which drove the engine from 40 to 7 to 0 mismatches by naming each missing feeder (a param is fed by `wire()` only when a call targeting its function fires, so backward from a param the matching invoke/icall receivers are activated; a load of a struct sub-node `box+off` has no `&(box+off)`, so writer-finding keys off the base object). Result: EQUIV 126/126 and DISPATCH 126/126 on the corpora, and EQUIV pass on soundfix (89,303 nodes). The completeness boundary is 7 of 513 soundfix receivers, all deep runtime-heap chains (an object reaching its writers through multi-hop heap indirection), which need the full alias-matching recursive state machine (Sridharan-Bodik) deferred to v2; the dispatch-shaped corpora do not exercise it.

**The footprint is measured at the optimum, not at the engine.** The engine's own traversal over-activates (its writer-finding forward-activates a loaded object's whole forward cone, and common runtime objects flow to ~18k nodes, giving a uniform ~12-19% footprint that is an artifact of loose writer-finding, not a property of demand). So footprint is measured by the resolved-graph backward slice: the exact set of nodes whose pts feeds a receiver, computed from batch's own dynamic-edge-resolved pts, which is what a tight demand engine touches with zero over-activation.

**The result is bimodal.** The median dispatch query touches 11 nodes on wile (12 etcd, 37 ollama), 0.001 to 0.003 percent of the program, against batch Andersen materializing all ~322,000 non-empty nodes and timing out. For the ~80-90 percent of dispatch sites that are domain-typed, demand sidesteps the flood entirely. But the p90 slice is 28,000 to 40,000 nodes, and 11 to 19 percent of receivers' slices contain the god object: these are the error-typed or error-adjacent receivers (`err.Error()`-style sites, or dispatch on an interface that can hold an error), for which the `errorString` genuinely feeds the answer and demand cannot avoid it. Receiver `|pts|` confirms the split, mean 180 to 328 (dragged up by the flooded tail) but median tiny. So the answer to E33's question is a qualified yes: demand relocates the cost from program-wide to the sites that actually query errors, making interface discovery tractable for the common case (11-node median query) while the error-typed minority stays flood-sized. Consistent with [[E34]]: the flood is intrinsic to context-insensitive analysis of error values, and only context sensitivity removes it at the source. Two bounds: the v1 engine does not yet *realize* the optimal footprint (its writer-finding over-activates; the RSM in v2 both tightens it and closes the 7 heap failures), and v1 is context-insensitive so it does not improve *precision*, only *cost distribution*.
**Result:** Demand-driven field-sensitive points-to, exact (== batch Andersen) on 126/126 dispatch corpora (EQUIV + DISPATCH) and EQUIV on soundfix; completeness boundary at 7/513 soundfix receivers (runtime-heap chains, need the v2 RSM). Optimal per-query footprint (resolved-graph backward slice) on wile/etcd/ollama: median 11/12/37 nodes (0.001-0.003% of N) vs batch materializing ~322k and timing out, but p90 28k-40k and 11-19% of receivers' slices reach the `*errors.errorString` flood. Demand transforms the common dispatch case, not the error-typed tail; consistent with [[E34]]'s "only context sensitivity removes the flood at its source." **Raw:** `2026-07-14-pre-composition.d/E35-cflreach/E35-cflreach-diagnostic.txt`
