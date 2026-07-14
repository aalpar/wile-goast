# Differentiation status — where the tool actually wins

**Date:** 2026-07-12
**One line:** Composition was the wrong variable. Per-step error rate is the right
one. Reachability is a dead axis; dispatch is a live one.

Full argument: [`docs/THESIS.md`](../docs/THESIS.md). (`docs/when-tools-win.md` was
deleted 2026-07-13: its central claim, ε = 0 for lookup steps, is contradicted by the
data. See THESIS.md §Superseded claims. Recover at `70e2fc8`.)
Experiments + raw results: `~/ClaudeProjects/LLMAccuracy` (all committed to master).

## The three results

All source-withheld, adoption-gated (a tool arm with 0 tool calls is the control
condition in disguise; every run below has 0 zero-call arms and 0 starved).

| task | one step is | control | baseline (grep+gopls) | tool | verdict |
|---|---|---|---|---|---|
| reachability, 47-hop scrambled chain (n=60/rung) | syntactic **lookup** | 91.7% | **100%** | 100% | **margin(t−b) = 0.0%** at every depth. FALSIFIED |
| `powerset_lattice` hard (n=30, algebra-accuracy) | set **computation** | 33% | — | 57% | tool wins **+23** |
| constant-index `[]func()` dispatch (n=60) | **wrong rule** | 65% | 23.3% | **100%** | tool wins **+35, p<1e-6** |

LLMAccuracy commits: `19fb47e` (corpus + pre-registration), `2557b6b` (depth effect
on control, p=0.006), `3feacd5` (`--withhold-source` + adoption gate), `17a3e8d`
(reachability H-hold), `6a2d887` (dispatch win).

## The criterion

> **A tool earns accuracy where the model's per-step error rate ε is non-zero — not
> where the derivation is merely long.** Error only compounds if there is error to
> compound. Roughly `accuracy ≈ (1−ε)^N`: reachability drove N to 47 with ε≈0 and
> nothing happened.

ε becomes non-zero two ways, and **both** are live:

1. **Fallible computation** (arm 1). Error compounds with depth. Lattice/fixpoint
   folds. *Do not generalize the reachability null here* — the algebra data
   (33% unaided) shows the model genuinely fails at composition when each step is a
   computation rather than a lookup.
2. **Wrong rule** (arm 2). The local syntactic reading disagrees with the semantics,
   so the model fails at depth one. Dispatch: it treats *address-taken* as
   *invoked*, re-deriving CHA's exact over-approximation by hand, confidently.

Where syntax tells the truth *and* each step is a lookup (direct call graphs),
nothing can help: there is nothing to get wrong.

## The cost axis — corrected 2026-07-12 (the first pass was wrong)

The original write-up claimed "the tool wins on cost, 5.7× fewer output tokens."
That was **output-only** and measured against the **weakest** denominator. Both
faults. Total tokens (input+output; prompt caching off, all zeros):

| | control (read-file) | baseline (grep) | treatment |
|---|---|---|---|
| reachability (n=120) | **2,228** | 12,470 | 2,320 |
| dispatch (n=60) | **1,443** | 19,586 | 2,867 |

- **vs grep: 5.4–6.8× cheaper.** Real, and the first pass *understated* it — grep's
  cost is nearly all *input* (11k–18k tokens), because each of its 3–6 noisy
  round-trips re-sends the accumulated history. Output-only reporting hid this.
- **vs control (just read the file): cost-neutral to 2× WORSE.** The tool round-trip
  (system prompt describing the tool + call + result back as input) costs more than
  reading a ~1.2k-token, 80-function file.

Grep-dumping a small file is a *bad strategy*; an agent with a read-file tool would
not do it. **Beating a bad strategy is not differentiation** — the same discipline
that discounted the dispatch accuracy margin from +76.7 to +35 applies here, and was
not applied the first time. On the reachability axis the tool currently buys
**neither accuracy nor cost**.

**Expiry date.** control cost is `O(source)`; tool cost is `O(answer)` — the tool
returns a compact set regardless of file size. A crossover must exist. **Unmeasured.**
Do not assert it. Cheap to settle: sweep n (f50→f384), control vs treatment, total
tokens, find where the curves cross. This is the same axis as the scale win.

## Consequences already applied

- **`'precise` decision closed by evidence.** The −33% regression was the *surface*
  (a `vta`-only exposure), not the tool. Exposing `'precise` → 100%. Ship it as the
  documented default for higher-order dispatch; never ship a `vta`-only surface.
  Recorded in `plans/2026-07-09-mcp-legibility-roadmap.md`.

- **must/may: NOT dismissed — scope correction (2026-07-12).** An earlier note here
  said must/may was "no longer load-bearing." **That was scoped to `[]func()` without
  saying so.** `goastcg/precise.go:68` bails on interface dispatch
  (`if common.IsInvoke() { return nil }`) and falls back to CHA. So for **interface
  dispatch there is no exact algorithm at all** — `vta` is the tightest available and
  is still a sound *over-approximation*. A bound-shaped return is precisely the
  condition that produced the −33% anchoring regression. **must/may is likely
  load-bearing for interfaces**, and experiment #2 below is what decides it.
- **THESIS.md amended.** "Unreliable at inference-time composition" was too broad.

## Next axes, ranked by the criterion

1. **Interface method dispatch** (arm 2). `x.Foo()` names no concrete impl. Same
   shape as the win we just measured; strongest prior. **This is the falsification
   test of the criterion**: if the model scores at ceiling unaided here, the
   criterion is wrong and we say so.
2. **Semantic duplicates** (arm 2, strongest form). Syntax does not merely stay
   silent — it *actively misleads*, since different-looking things are assumed
   different. `find_duplicates` + `equiv_tier` already ship; the legibility probe
   already passes. Needs an A/B.
3. **Checked-before-use / all-paths dominance** (arm 2). Syntax shows *a* check; the
   question is about *all* paths.
4. **Scale** (orthogonal, and the one real prior win). grep is line-oriented and
   floods; it truncated even at n=80. Serena 62%→12% at f384 while wile-goast held.
   Note this is a *retrieval* win, not a composition win, and it should be reported
   as such. Plan exists: `2026-07-09-deeper-ladder-marginal-lift-breakzone.md`.

**Dead:** reachability depth. Do not spend more there. Grep recovers the whole
adjacency list in one call and the model composes it perfectly.

## Method notes worth keeping

- **Pre-register the falsification condition.** The first ladder saturated (all arms
  100%); without a written H-break/H-hold criterion it would have been tempting to
  read `margin = 0` as "bet falsified" when it was a **ceiling effect** (failed
  calibration).
- **Adoption is a validity gate, not a statistic.** The first baseline made **0 tool
  calls in 8/8** because `natural_language` embeds the full source. A tool arm that
  never calls a tool is the control arm wearing a costume; its score means nothing.
- **Do not rig the baseline.** grep dumping the source is what grep *does*. Lowering
  `--grep-max-lines` to stop it would manufacture a win.
- **Pick the honest denominator.** Dispatch margin vs grep-baseline was +76.7, but
  baseline's collapse is partly a grep-fragmentation artifact. vs control (clean
  source) it is **+35**. Use +35.
