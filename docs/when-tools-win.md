# When Does a Static-Analysis Tool Actually Help an LLM?

Here is a puzzle worth sitting with.

You hand a language model a Go file. Every fact about that program is *in the
file*. The call edges are there. The types are there. Which branch is dead, which
variable shadows which — all of it, in the text, right in front of the model.

So why would a static-analysis tool ever help? The tool reads the same file. It
cannot conjure information the text does not contain. At best it is a faster
reader of something the model can already read.

That question has a real answer, and getting it right took us two experiments
that pointed in opposite directions.

## The Problem: our first answer was wrong

The obvious hypothesis, and the one wile-goast was built on, goes like this:

> *Some facts require chaining many steps together. The model loses track partway
> through. The tool computes the whole chain at once, so it wins.*

Call this the **composition hypothesis**. It sounds right. Tracing "what can `f0`
eventually call?" through a forty-hop chain of function calls is exactly the kind
of tedious bookkeeping we expect a machine to beat a human at, and by extension a
model at.

So we tested it. We built call graphs where the reachable set is a single
scrambled chain: `f0` calls `f40`, which calls `f13`, which calls `f7`, and so on
for **47 hops**, with the function indices shuffled so there is no source-order
locality to lean on. We held everything else fixed — same file size (80
functions), same answer length (48 names) — so the *only* thing changing was how
many steps had to be chained.

Then we gave the model nothing but the source and asked it to trace.

It got **100%** at 4 hops. It got **100%** at 16 hops. At 47 hops it got 91.7%.
A tool arm with `go-callgraph-reachable` also got 100%. The margin between them:
**zero, at every depth.**

The composition hypothesis is dead. The model chains 47 facts together
essentially perfectly. Length is not the enemy.

And here is the trap. The tempting conclusion, staring at that result, is:
*"models can do this themselves; the tool only saves tokens."* That conclusion is
false, and the second experiment shows exactly how false.

## The Key Insight: it was never about length

Let us look closely at *what the model actually does* when it traces a call graph.

It applies a rule. Roughly:

> If the text `f40()` appears inside the body of `f0`, then `f0` calls `f40`.

Now: **is that rule correct?** For ordinary direct calls — yes. Perfectly,
exactly correct. It is not an approximation or a heuristic that usually works. The
syntactic pattern *is* the semantic truth.

And if you apply a correct rule 47 times and compose the results, what do you get?
A correct answer. There is no error to accumulate, because there was no error to
begin with. That is why depth did nothing: **you cannot compound an error rate of
zero.**

### Careful: this is not "models are good at composition"

Here is where it would be very easy to over-learn the lesson, so let us not.

The reachability result does **not** say "long chains are fine for LLMs." It says
long chains of *this particular step* are fine. And the step in question — "look
at this function body, read off the names it calls" — is a **lookup**. It is
retrieval, not computation. Its error rate is essentially zero.

We have direct counter-evidence from a different domain. On `powerset_lattice`
problems at hard difficulty (folding joins over a lattice of sets), the model
scores **33%** unaided, and a tool lifts it to 57%. That is a *composition*
failure, and it is severe. Each step there is not a lookup — it is an actual
computation (a set join), and the model gets each one right only *most* of the
time.

So write down the model that explains all of it. Let ε be the model's error rate
on a **single step**, and N the number of steps. Roughly:

```
accuracy  ≈  (1 − ε)^N
```

Now everything falls out of *which term dominates*:

| task | what one step is | ε | N | result |
|---|---|---|---|---|
| call-edge tracing | a syntactic **lookup** | ≈ 0 | 47 | (1−0)^47 = 1. Depth is **free** |
| powerset-lattice fold | a set **computation** | > 0 | large | compounds → **33%** |
| higher-order dispatch | a **wrong rule** | large | 1 | fails at **depth one** |

**Depth only hurts when ε > 0.** Our reachability corpus drove N to its ceiling
while holding ε at zero, so of course nothing happened. That was the flaw in the
experiment's premise, not a discovery about composition.

So the real question is never *how long is the chain?* It is:

> **What is the model's error rate on one step — and why?**

Two different things can make ε large, and they call for the same remedy:
- The step is a **fallible computation** (the lattice join). Error compounds with
  depth.
- The step is a **wrong rule** — the model's local syntactic reading disagrees
  with the semantics. Then it fails at depth one, and depth is irrelevant.

Higher-order dispatch is the second kind, and that is what we test next.

## How It Works: constructing a disagreement

To test this, we need code where the obvious syntactic reading is *wrong*. Go
gives us one easily. Here is a function:

```go
func f0() {
	t := []func(){f40, f34, f4}
	t[0](); t[1]();
}
```

Read it the way the model reads it. Three function names — `f40`, `f34`, `f4` —
sit right there in `f0`'s body. The pattern-matching rule fires: all three are
called.

But look again. The slice holds three functions, and only **two** of them are ever
invoked: `t[0]` and `t[1]`. Index 2 is never called. **`f4` is
address-taken but never invoked.** It is mentioned, not called.

To get this right you must do something the syntactic rule cannot do. You must
track the slice as a *value*, find which constant indices are actually invoked,
and map each index back to the element it selects. The slice literal is on one
line; the invocation is on another. The fact lives in the *join* of the two.

That join has a name in compiler literature: constant propagation through SSA
form. But notice that we earned the term by needing it, rather than opening with
it. The point is not the name. The point is that this is a *non-local* inference,
and the model's rule is *local*.

So we predicted the model would answer `truth ∪ {f4}`.

## Seeing It In Action

We ran it: 60 problems, 50 functions each, one never-invoked decoy per problem,
same protocol as before.

| arm | what it has | score |
|---|---|---|
| control | the clean source, no tools | **65.0%** |
| baseline | grep + gopls | 23.3% |
| treatment | wile-goast `'precise` | **100%** |

**+35 points over control, p < 1e-6.** And when we opened up the failures, the
model's wrong answers were exactly what we predicted: the true set, plus the
address-taken decoy. Not confusion. Not garbage. A clean, systematic
over-approximation.

Here is the part worth pausing on. That over-approximation — *"if a function's
address is taken and flows toward a call site, assume it can be called"* — is not
some quirk of the model. It is precisely what the classical `cha` and `vta`
call-graph algorithms compute. **The model, reasoning from syntax, reinvented
Class Hierarchy Analysis and made CHA's exact mistake.** It arrived at a sound
over-approximation when the question demanded an exact answer.

wile-goast's `'precise` algorithm resolves the constant index from SSA and drops
the spurious edge. One tool call. 60 out of 60.

## The Subtle Parts

**First: the failure is silent.** The model is not confused at 65%. It does not
hedge, flag uncertainty, or ask for help. It confidently returns an answer that is
wrong by exactly one element. From the inside, the 65% case and the 100% case feel
identical. This is what makes the tool valuable — not that it is *faster*, but
that it is *right where the model is confidently wrong*.

**Second, and this is the tricky bit: what we falsified was a mechanism, not a
value.** The reachability experiment killed the *explanation* ("tools help because
composition is hard"). It did not kill the *phenomenon* (tools sometimes help). A
false explanation can be replaced by a true one without the conclusion collapsing.
Confusing "your reason is wrong" with "your conclusion is wrong" is how a good
negative result gets over-read into a bad one.

**Third: the honest denominator matters.** Notice grep scored 23.3% — *worse than
having no tools at all*. Why? grep is line-oriented, and this fact spans two lines.
grep returned the slice literal but not the invocation site, so the model saw three
names address-taken and folded in all three. That is a self-inflicted wound from
the retrieval strategy, not evidence about the tool. Which is why the honest
comparison is against **control at 65%**, not grep at 23.3% — and why the headline
is +35 points, not the flashier +77.

## An Analogy

Your visual system is fast, reliable, and correct about almost every scene you
will ever look at. Then someone shows you the Müller-Lyer illusion — two lines
with arrowheads — and you confidently perceive one as longer. It isn't.

A ruler does not help you see ordinary lines better. You already see them fine; a
ruler would be pure overhead. The ruler earns its keep in exactly one place: where
your perception *systematically lies to you*, and where you cannot tell from the
inside that it is lying.

Direct call graphs are ordinary lines. Higher-order dispatch is the illusion. The
model's syntactic reading is the visual system. wile-goast is the ruler.

## What Would Break: the criterion, and how to falsify it

This gives us a criterion sharp enough to make predictions — which means sharp
enough to be wrong.

> **A tool earns an accuracy advantage exactly where the model's per-step error
> rate is non-zero. Where every step is a reliable lookup, the tool buys
> efficiency only, at any depth.**

And ε becomes non-zero for two distinct reasons, which is why the criterion has
two arms:

**Arm 1 — the step is a fallible computation.** Error compounds with depth. This
is the algebra case (`powerset_lattice` hard: 33% unaided). Here depth *is* the
enemy, and a tool that computes the fold exactly is worth having. **Do not
generalize the reachability null to this arm.** It does not apply.

**Arm 2 — the step is a wrong rule**, because the local syntactic reading
disagrees with the semantics. Here the model fails at depth one and depth is
beside the point. This is higher-order dispatch (+35 points).

Where the tool should **not** help — syntax tells the truth *and* each step is a
lookup:

- Direct call graphs. *Measured: margin 0.* Falsified as a differentiator.
- Anything where "the text says X" and "the program does X" agree, and reading X
  off the page is the whole job.

Where the tool **should** help:

- Higher-order dispatch (arm 2). *Measured: +35 points.*
- Interface method dispatch — `x.Foo()` names no concrete implementation (arm 2).
- Semantic duplicates — two functions with different text and identical meaning.
  Syntax doesn't merely stay silent; it *actively misleads*, because
  different-looking things get assumed to be different things (arm 2).
- "Is this value checked on **every** path?" — syntax shows you *a* check; the
  question is about *all paths* (arm 2).
- Lattice/fixpoint computations over a program — abstract interpretation, dataflow
  joins. These are arm 1, and the algebra data already says the model struggles.

If we test interface dispatch and the model scores at ceiling unaided, the
criterion is wrong and we should say so. That is the point of stating it this
sharply.

Now the honest limits, and they matter. This rests on **three** data points: one
null (call-tracing), one positive from a wrong rule (dispatch, n=60, a single
constant-index `[]func()` pattern, one scale, one model), and one positive from a
fallible computation (powerset lattices). Three points sketch a shape; they do not
make a law. The `(1−ε)^N` model above is a *sketch* to organize the results, not a
fitted curve — steps are not independent and the model does not literally
multiply out.

But they are enough to kill two wrong lessons at once. The reachability null does
*not* mean the model can do static analysis; it means the model can do *the static
analysis whose answer was already written on the page*. And it does *not* mean
composition is safe — compose enough fallible steps and the model breaks, as the
lattice problems show. What was special about call-tracing was never the
composition. It was that there was nothing to get wrong.
