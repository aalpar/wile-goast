# Interface Dispatch as Located, Justified Findings — Design

> **Status:** Design (2026-07-12). Approved in brainstorm; implementation plan to follow.
> **Scope:** How `go-callgraph` reports interface dispatch. Two provenance fixes in
> Go (`goastssa/mapper.go`, `goastcg/mapper.go`) plus a new Scheme library
> `(wile goast dispatch)`.
> **Parent:** [`2026-06-01-auditable-categorization-design.local.md`](2026-06-01-auditable-categorization-design.local.md).
> This design is that note applied to the call graph, which its violation table omits.

## The defect

`cgMapper.mapEdge` (`goastcg/mapper.go:78`) returns `caller`, `callee`, `pos`,
`description`. It has a *where*; it has no *why*. `description` reports the call's
**kind** ("dynamic method call"), never why **this callee**. At an interface site the
justification — which concrete type flows here, and where it entered the interface —
is computed by VTA and thrown away.

The parent note's language applies verbatim: a result born located, amputated on the
way up. The call graph is a missing row in its violation table.

This is not cosmetic. A CHA guess and a provably-resolved call are emitted in
byte-identical shape, so a consumer cannot ask "is this edge a fact or a bound?" That
is the measured mechanism of the LLMAccuracy `-33%` result: a bound arrived *formatted
as a fact* and was reported verbatim.

## Measurement (2026-07-12)

Fan-out per interface call site, `cha` → `vta`. "Interface dispatch" = the
`dynamic method call` edge description (SSA invoke mode).

| codebase | sites | median | p90 | max |
|---|---|---|---|---|
| rclone `fs` | 729 → 315 | 2 → 1 | 6 → 3 | 172 → **10** |
| client-go `tools/cache` | 270 → 208 | 3 → 1 | 7 → 2 | 10 → **4** |
| apimachinery `pkg/runtime` | 241 → 170 | 8 → 2 | 29 → **27** | 72 → **27** |

Candidate-count distribution under VTA:

| codebase | sites | `must` (n=1) | may 2–4 | may >4 |
|---|---|---|---|---|
| client-go | 137 | **96 (70%)** | 40 | 1 |
| rclone | 298 | **160 (54%)** | 129 | 9 |
| apimachinery | 152 | **57 (38%)** | 50 | 45 |

VTA costs nothing over CHA (0.51s vs 0.55s on a 15k-node repo) and returns a *smaller*
graph. Three findings drive the design:

1. **38–70% of interface call sites resolve to a single candidate.** A sound
   over-approximation of size 1 means the true callee set is a subset of a singleton:
   if the call executes, it calls that function. **`must` falls out of `|candidates| == 1`.
   No new analysis.**
2. **"Genuine polymorphism" is not decidable and must not be claimed.** Given 27
   candidates, the tool cannot know whether the site is truly 27-way or whether VTA
   failed to narrow. Only 12 of apimachinery's 152 sites are *unnarrowed*
   (`|vta| == |cha|`); the 27-candidate tail is narrowed-but-still-large. So
   "narrowed" and "small" are independent axes, and no evidence separates real
   polymorphism from residual imprecision. Asserting a `polymorphic` class would be a
   verdict the tool cannot support.
3. **The tail is real but local.** `may >4` is 30% of sites in apimachinery, under 4%
   elsewhere. Half of apimachinery's worst sites are in `zz_generated.deepcopy.go`.

## The finding

The unit is the **call site**, not the edge. An edge list structurally cannot express
"this site is 27-way" — it can only emit 27 rows, which makes the tail larger, not
smaller.

```scheme
(dispatch-site
  (caller         . "(*versioning.codec).doEncode")
  (where          . "versioning.go:256:33")
  (iface          . "runtime.Object")
  (method         . "DeepCopyObject")
  (class          . may)                 ; none | must | may
  (n              . 27)                  ; ALWAYS the true count
  (narrowed-from  . 72)                  ; CHA's count here — evidence VTA worked
  (scope          . "./pkg/runtime/...")
  (iface-exported . #t)                  ; => external impls possible
  (why            . "27 of 72 CHA candidates flow here")
  (detail         . elided))             ; full | elided; no `candidates` key present

(dispatch-site
  (caller . "(*cache.sharedIndexInformer).HandleDeltas")
  (where . "shared_informer.go:412:18")
  (iface . "cache.Store") (method . "Add")
  (class . must) (n . 1) (narrowed-from . 9)
  (scope . "./tools/cache/...") (iface-exported . #t)
  (why . "sole concrete type flowing here: *cache.cache")
  (detail . full)
  (candidates
    ((callee   . "(*cache.cache).Add")
     (concrete . "*k8s.io/client-go/tools/cache.cache")
     (witness  . ("store.go:88:12" "shared_informer.go:41:9")))))
```

### `class` is a pure function of `n`

| n | class | meaning |
|---|---|---|
| 0 | `none` | no concrete type flows here *within scope* |
| 1 | `must` | if this call executes, it calls that function |
| >1 | `may` | one of these `n` |

No judgment enters. `class = f(n)` is the whole rule, which is what keeps the tool
free of verdicts while still answering the question.

`none` is load-bearing, not an edge case: apimachinery has 241 CHA invoke sites and
170 under VTA — **71 sites have zero candidates.** Either the dispatch is dead, or the
implementing type lives outside the analyzed package set. Omitting these sites would
hide the tool's own blind spot. Emitting them turns a silence into a finding.

### Encoding rules (each closes a footgun by construction)

- **`candidates` is *absent*, not empty, when elided.** `(nf site 'candidates)` returns
  `#f`, never `'()`. An empty list would let a consumer read "no candidates" from a
  27-way site — the silent false negative, reintroduced through the encoding.
- **`n` is always the true count**, independent of `detail`. The knob can never make a
  site *look* smaller than it is.
- **`witness` is deliberately weaker than a flow path.** It lists `ssa.MakeInterface`
  positions: *where this concrete type entered this interface*, not *how it reached
  this site*. VTA's type-flow graph is not exported by `x/tools`, so any stronger claim
  would be fabricated.

### The knob

`K` (default 8) controls **detail, never sites**. Every dispatch site is always
returned. `n <= K` gets the full candidate list with witnesses; `n > K` keeps every
field and drops only the enumeration, with `n` stating exactly what was elided.
Degradation is graded, not lossy.

The rejected alternative — "return `must` sites only by default" — is the literal
confirmatory-pathology default, but applied to the wrong axis: it hides 30–62% of the
dispatch surface, and a consumer asking "what can `doEncode` call?" would receive
silence, unable to distinguish "nothing" from "withheld".

**The preferred error direction depends on the question, not the tool.** For
enumeration ("which functions does `f` call?") a false positive pollutes. For an
existential ("can `f` reach this sink?") a false negative is a missed vulnerability —
which is why `evaluate_soundness_mode.py` grades under-approximation (`'static`) as
*the* error. A global "prefer false negatives" default would make that probe's error
the tool's default. The finding shape resolves this by reporting what is known and
letting the consumer project.

## Architecture

### Go change 1 — `goastssa/mapper.go:586`

`mapMakeInterface` emits `name`, `x`, `type`, `operands`. It discards the two facts
that make it a witness:

```go
goast.Field("concrete", ...v.X.Type()...)   // NEW: what type entered the interface
goast.Field("pos",      ...v.Pos()...)      // NEW: where it entered
```

*Open item:* verify `ssaMapper` carries an `fset` (the cg mapper does, `mapper.go:89`).
If not, that is plumbing, not a design change.

### Go change 2 — `goastcg/mapper.go:78`

On an invoke site, `mapEdge` knows the interface, the method, and the receiver, and
reports none of them:

```go
if c := e.Site.Common(); c != nil && c.IsInvoke() {
    goast.Field("iface",  ...c.Value.Type()...)                    // "runtime.Object"
    goast.Field("method", ...c.Method.Name()...)                   // "DeepCopyObject"
    goast.Field("recv",   ...e.Callee.Func.Signature.Recv()...)    // "*pkg.cache"
}
```

`recv` exists so Scheme joins against the witness index on a type string rather than
string-parsing `"(*pkg.T).M"` — a name that is not a contract.

Both changes are additive; existing fields are unchanged and no consumer breaks.
Neither is *for* this feature. Both are rows in the parent note's violation table, and
every consumer benefits.

**A consequence worth naming:** once `iface` exists, "is this an interface dispatch?"
is a *field-presence test*. Today it requires matching `description` against
`"dynamic method call"` — the same class of syntactic heuristic that
`soundness_mode_grade.scan_modes` documents as a known blind spot. The heuristic dies.

### Scheme — `(wile goast dispatch)`

A new library beside `dataflow`, `fca`, `path-algebra`, `unify`. It introduces no
analysis; it folds facts that already exist.

```
dispatch-sites(pattern, K=8)
  vta ← (go-callgraph pattern 'vta)
  cha ← (go-callgraph pattern 'cha)
  ssa ← (go-ssa-build pattern)

  invoke-edges(g) = edges WHERE the `iface` field is present
  site-key        = (caller . where)
  candidates[key] = invoke-edges(vta) grouped by site-key
  narrowed-from   = |invoke-edges(cha) at same key|
  witness-index   = ssa-make-interface nodes indexed by `concrete` → [pos]
  class           = f(n)
```

Cost is 2× the call graph (both CHA and VTA): ~1s on a 15k-node repo.

## Soundness

- **`scope` and `iface-exported` ride in every finding.** `must` means *must within
  this package set*. On an exported interface in a library, an external caller can
  inject a type VTA never saw. The finding says so rather than asserting a proof it
  does not have.
- **Invalid positions.** Synthetic and generated SSA has no `Pos()`; `witness` is then
  `'()` — an honest empty witness, never a fabricated one.
- **Generics — unresolved risk.** Instantiated methods produce type strings that may
  not join cleanly against the witness index. A testdata case must settle this. If it
  breaks, the failure must be a *missing* witness, never a wrong one.

## Testing

Go units: `mapMakeInterface` emits `pos`/`concrete`; `mapEdge` emits
`iface`/`method`/`recv` on invoke sites and **not** on static calls.

Golden testdata package: single-impl → `must`; three impls all flowing → `may n=3`;
**an impl allocated but never converted to the interface** (the decoy — proves VTA
prunes what CHA folds in); a zero-candidate site → `none`; a site above `K` → `elided`;
a generic instantiation.

**The invariant that is the design:**

> The knob may only remove DETAIL, never TRUTH. For any `K`, at every site: `n` is
> identical (K-invariant); only `detail` and the presence of `candidates` may differ;
> `candidates` is *absent* when elided, never `'()`.

Asserted over the golden package at `K = 1, 8, 1000`. If it fails, the knob has become
the silent false negative the finding shape exists to make impossible.

## Relationship to prior work

- **Closes the must/may question** left open by LLMAccuracy `6a2d887`, which concluded
  must/may was not load-bearing — a conclusion scoped to `[]func()`, where `'precise`
  *is* exact. Interfaces re-opened it. The answer is that must/may needs **no new
  analysis**: it is `|VTA candidates| == 1`.
- **`'precise` cannot help here.** `goastcg/precise.go:66-68` declines `IsInvoke` and
  returns CHA's edges unrefined — while being *named* "precise". On an interface corpus
  `'precise` returns exactly CHA. The honest soundness labels must say so.
- **Feeds LLMAccuracy experiment #2** (interface dispatch). That experiment's outcome
  (b) — "the tool hands the model a bound, and the model reports the bound" — is the
  hypothesis this design exists to defeat.
