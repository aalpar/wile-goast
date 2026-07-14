# Andersen points-to: `(wile goast andersen)`

**Status:** design complete, impl pending
**Notebook:** [[2026-07-14-pre-composition/E26]] (the obligation), [[2026-07-14-pre-composition/E27]] (the oracle), [[2026-07-14-algebraic-bet/C3]] (the prerequisite)

An inclusion-based, field-sensitive points-to analysis, scoped to real Go from the
outset. It lifts `goastcg/precise.go:66`, which today returns `nil` for `IsInvoke()` and
declines interface dispatch outright. It is also the first computation this project has
built that puts the algebraic bet on trial, and it was chosen partly for that.

## 1. The gap

`goastcg` ships exactly four call-graph algorithms: `{static, cha, rta, vta}`, which is
also x/tools' entire menu. On the interface-dispatch corpora, `'vta` over-approximates on
63/63, by exactly the decoy.

The reason is mechanical rather than fundamental. x/tools' VTA is **field-based**: it
abstracts container elements by *type*, so every `[]Ia` in a program shares one abstract
element node. Route the real implementors into a `registry` global and a decoy into a
`spare` global, dispatch by ranging over `registry` only, and VTA cannot keep the two
containers apart. The decoy leaks into the dispatched receiver.

An inclusion-based analysis that abstracts by *allocation site* keeps them apart and
resolves all 63 exactly. This was measured, not assumed: the vendored `go/pointer` is
exact on 126/126 across both corpora (E24).

So the bound wile-goast currently reports is an artifact of the analysis it ships, not a
property of the problem. That is what converts this from a finding into an engineering
obligation.

**And it cannot be discharged by vendoring, which is what makes it worth doing.** The
obvious shortcut is to lift the deleted `go/pointer` into the tree and be done. That was
tried and it fails: `go/pointer` panics on any program containing generic types, with no
workaround (E28). It cannot analyze wile, and it cannot analyze most real Go.

The consequence is worth stating plainly, because it is the strongest motivation this
work has. **For any Go program using generics, VTA genuinely is the ceiling of what
exists off the shelf.** The single exact, inclusion-based, field-sensitive points-to
analysis ever written for Go was deleted from x/tools and predates generics badly enough
that it cannot run on modern code. E24 said the fact is computable and no shipped tool
computes it. That was true but too weak: the fact is computable in principle, and the
only implementation that ever computed it is dead.

`(wile goast andersen)` is therefore not a reimplementation of something available. It is
the only route to the fact.

## 2. Prior art: `(wile goast pointsto)` is a different analysis, not a weaker one

The tree already holds `(wile goast pointsto)`, whose header says the interprocedural
edge is an unimplemented TODO in `make-param-seed`. It is tempting to read that TODO as
this work. It is not.

Andersen has four constraint forms. `pointsto.scm` implements two:

| Form | Andersen | `pointsto.scm` |
|------|----------|----------------|
| `p = &a` | `pts(p) ⊇ {a}` | yes (`ssa-alloc`) |
| `p = q` | `pts(p) ⊇ pts(q)` | yes (field-addr, convert, phi) |
| `p = *q` | `∀o ∈ pts(q): pts(p) ⊇ pts(o)` | **no heap model** |
| `*p = q` | `∀o ∈ pts(p): pts(o) ⊇ pts(q)` | **`ssa-store` unhandled** |

Loads and stores are the *dynamic* constraints: they add graph edges during the solve,
which is what makes Andersen a dynamic transitive closure rather than a dataflow pass.
Without them there is no model of what an object *contains*, so "what is in
`registry.xs`?" cannot be asked, let alone answered. Filling in `make-param-seed` would
yield an interprocedural *pointer-copy* analysis, still heap-less, still unable to
resolve corpus B.

The engines differ in shape too. `pointsto.scm` runs on `run-analysis`: flow-sensitive,
per-function, per-block in and out states. Andersen is flow-*insensitive* and
whole-program, one global `pts` map solved to a least fixpoint. Neither reaches the other
by adding a clause.

**Decision: a new sibling library.** `(wile goast pointsto)` keeps its flow-sensitive job
and its `lock-escape-measure`; its TODO stays open and is unrelated to this work. Two
libraries because there are genuinely two questions.

## 3. The seam

```
goastcg/prim_andersen.go       Go:     SSA -> constraints + method table, extracted ONCE
lib/wile/goast/andersen.scm    Scheme: the least fixpoint, over the product lattice
```

Go performs **static extraction only**. Scheme owns the entire dynamic closure. Nothing
crosses the seam mid-solve, and there is no foreign call in the inner loop.

That property is not free, and section 6 is where it is bought.

One primitive carries the seam:

    (go-andersen-constraints <ssa-program>)
      -> ((constraints . (<form> ...))       ; section 5, six forms
          (methodtab   . ((<type> . ((<method> . (<params> <result>)) ...)) ...))
          (objects     . ((<obj-id> <type> <fields>) ...))
          (defeaters   . ((<node> . <reason>) ...)))   ; section 7

Everything the solver needs is in that one value. `objects` carries each object's type,
which is what `invoke` reads to index the method table, and its field count, which is
what `gep` reads to form `o.f`. `defeaters` names the nodes Go could not model, so the
solver seeds their provenance rather than discovering the gap by accident.

## 4. Node universe and heap model

Mirror `go/pointer`. A divergence from the oracle must then be interpretable as a bug in
our analysis, rather than as a difference of model.

**Variables:** SSA registers, parameters, results, globals, closure free variables.

**Objects:** one per allocation site: `Alloc`, `MakeSlice`, `MakeMap`, `MakeChan`,
`MakeClosure`, `MakeInterface` boxing, each global, each function value.

**Field-sensitive on structs.** An object is a *block* of nodes, one per field offset:
`o.0`, `o.1`. Field-insensitivity would merge two pointer fields of one struct, which is
the registry/spare pattern occurring *within* a single struct, and it is everywhere in
real Go.

**Element-insensitive on slices, arrays, and maps.** All elements collapse to one node.
Unbounded indices cannot be tracked, and this is what the oracle does.

That combination is what resolves corpus B: `registry` and `spare` are distinct
*objects*, so their element nodes are distinct, and the decoy cannot leak. Element
insensitivity costs nothing here, because the discrimination that matters is between
containers, not within one.

## 5. Constraint forms

Six, all emitted statically by Go.

| Form | Meaning |
|------|---------|
| `(addr p o)` | `pts(p) ⊇ {o}` |
| `(copy p q)` | `pts(p) ⊇ pts(q)` |
| `(gep p q f)` | `pts(p) ⊇ {o.f : o ∈ pts(q)}` |
| `(load p q)` | `∀o ∈ pts(q): pts(p) ⊇ pts(o)` |
| `(store p q)` | `∀o ∈ pts(p): pts(o) ⊇ pts(q)` |
| `(invoke recv m args ret)` | `∀o ∈ pts(recv)`: look up `impl(type(o), m)`; wire args to params, result to ret |

Field sensitivity lives entirely in `gep`, so `load` and `store` stay field-free
dereferences.

The last three are dynamic: each quantifies over a points-to set and adds copy edges as
that set grows.

## 6. Interface dispatch is a load

Lifting `precise.go:66` requires an on-the-fly call graph: as `pts(receiver)` grows, new
callees become reachable, whose parameters need new copy constraints. The constraint set
therefore grows during the solve. With extraction in Go and the solve in Scheme, the
naive reading is that the solver must call back into Go mid-fixpoint.

It does not, because `invoke` has the same operator shape as `load`:

```
load:    ∀o ∈ pts(q):     pts(p) ⊇ pts(o)
invoke:  ∀o ∈ pts(recv):  pts(param_i(impl(o,m))) ⊇ pts(arg_i)
                          pts(ret) ⊇ pts(result(impl(o,m)))
```

Both quantify over a points-to set and add copy edges. The only thing `invoke` needs
beyond a load is a **method table**, mapping a concrete type and a method name to the
callee's parameter and result node IDs. That table is *static*: Go computes it once, up
front, alongside the constraints.

Given the table, Scheme discharges `invoke` entirely inside the fixpoint. **The call
graph is a byproduct of the same least fixpoint that computes the points-to sets**, and
every `invoke` discharge records an edge.

This is the design's strongest claim on the algebraic bet. Dispatch resolution is not a
separate algorithm bolted onto a points-to analysis; it is the same operator instantiated
at a different carrier, which is precisely the promise the bet makes and which
[[2026-07-14-algebraic-bet/C2]] records as having been broken once already.

## 7. The carrier

```
D  =  P(Object) × P(Defeater)
⊔  =  componentwise union
⊥  =  (∅, ∅)
```

**There is no `⊤`.** `pointsto.scm` uses a `⊤` sentinel for anything it cannot model, and
its own documentation records the consequence: because a method receiver is a parameter
and every parameter is seeded `⊤`, "the measure abstains on real code." Sound, and
useless. Scoping this work to real Go means designing against that outcome specifically.

Here, doubt is scoped to its cause instead of poisoning the lattice:

```
pts(v) = ({o1, o2}, ∅)                  exactly o1 and o2
pts(w) = ({o3},     {reflect@f.go:12})  at least o3, possibly more, for that reason
```

An empty defeater component **is** the exactness certificate. A consumer can tell a
precise answer from a doubtful one, and can tell *why* the doubtful one is doubtful,
without the doubt spreading to unrelated results.

Defeaters are `reflect`, `unsafe`, cgo, and external functions with no body. This follows
the convention `(wile goast dispatch)` already ships, whose governing principle is to
"degrade to a MISSING witness, never a WRONG one," and it refines it: `dispatch` carries
`reflection-in-scope` as a single scope-level flag, which on real Go is nearly always
`#t` and therefore casts a permanent blanket of doubt over every answer in the package.
Per-node defeater provenance says *which* answers are affected.

Product-of-lattices is a standard combinator, so this is operator instantiation at a new
carrier, which is the claim under test.

## 8. The solve

`lfp(F)` by chaotic iteration: a worklist over the constraint graph, propagating deltas.

`F` is monotone (every constraint only ever adds) and the lattice has finite height
(`|Object|` and `|Defeater|` are both finite), so Knaster-Tarski gives the fixpoint and
chaotic iteration provably reaches it.

**The line, and it is the point of the exercise.** The semantics must remain `lfp(F)` with
`F` composed from the algebra's operators. Schedules may be optimized; the operator may
not.

- **Permitted:** chaotic iteration, worklist ordering, delta propagation. These provably
  compute the same `lfp` and change only the visit order.
- **Permitted, and initially barred by mistake: HVN.** See section 8a.
- **Permitted, and orthogonal to the algebra entirely: the representation of a points-to
  set.** See section 8b. This is where the cost problem actually lives.
- **Barred:** cycle elimination (HCD/LCD), hybrid solvers. Those rewrite the constraint
  graph by an argument that is not a lattice construction. A win won by them would be a
  win for hand-written solver engineering, leaving the bet exactly where
  [[2026-07-14-algebraic-bet/C1]] found it: no win, no loss.

### 8a. HVN is algebraic. The original bar was wrong

An earlier version of this design barred hash-value numbering alongside cycle elimination,
on the belief that any constraint-graph rewrite must be hand-written solver engineering.
That was a mistake, and [[2026-07-14-algebraic-bet/C7]] records it.

HVN's equivalence `~` is on the **index set** (the variables), not on the value lattice.
Let `L = D^V` and let `S ⊆ L` be the maps constant on `~`-classes. `S` is closed under `⊔`
and `⊓` pointwise and contains `⊥`, so it is a **sublattice**, and `S ≅ D^(V/~)`. HVN's
soundness argument is exactly `F(S) ⊆ S`. Given that, the Kleene chain from `⊥` never
leaves `S`, so `lfp(F) ∈ S` and `lfp(F) = lfp(F|S)`.

That is Knaster-Tarski restricted to an **F-invariant sublattice**: the same operator at a
coarser carrier, identical fixpoint. It is operator-instantiation, which is what the bet
says a new query should be made of.

Two caveats worth carrying. `hvn.go` can collapse `*p` cycles even when `pts(p)` is empty,
so the implementation is a sound *abstraction* rather than an exact restriction (precision
loss, confined to dead code). And it survives on-the-fly dispatch only by marking as
`indirect`, and never merging, "the parameters of address-taken functions (which includes
all functions in method sets)" — the nodes a dynamically added edge can write to are
conservatively excluded.

**But do not put HVN on the critical path.** Its own PERFORMANCE section measures a 30 to
42 percent solve-time reduction against a 15 percent cost to run it: a net factor of about
1.3. The gap is about 100.

### 8b. The representation of a points-to set is below the algebra

`pointer/doc.go` states that `go/pointer` represents points-to sets as **sparse bit
vectors**, "more space- and time-efficient than sets based on Go's built-in map type."

The measurement in [[2026-07-14-pre-composition/E30]] used exactly a hash map, and spent
32.3 billion membership tests without converging. Under a bit vector there are no
per-element membership tests: a union is a word-parallel OR, roughly 8 word operations at
the measured mean set size of 486 where a map performs 486 hash lookups.

**The solution size is intrinsic; the operation count is not.** 151 million points-to pairs
is what the analysis *means*. It is also about 19 MB of bits. That is not an intractable
object; it is an object for which a hash map is the wrong container.

**The operator laws say nothing about how a lattice element is stored.** Associativity,
idempotence, absorption, distributivity, monotonicity: none of them constrain the encoding
of a set. Representation is therefore a decision made strictly *below* the algebra,
invisible to the composition rules, and it **cannot forfeit the bet however clever it
gets**. It is the only escape hatch identified so far that costs the bet nothing.

Which relocates the risk rather than removing it, and section 11 is where that lands.

The vendored oracle contains both barred optimizations (`pointer/hvn.go`,
`pointer/opt.go`). They stay as a reference for the fast path, and as the fallback if the
algebraic solve cannot run on real Go.

Note the contrast with [[2026-07-14-algebraic-bet/C2]], which is the bet's one recorded
counterexample. `(wile algebra cfl)` had to be hand-written because matched call/return is
a Dyck language and **no semiring rejects the unrealizable paths**: a semantic failure of
the algebraic families. Nothing of the kind is true here. The lattice formulation of
Andersen is semantically exact. Only the schedule is up for negotiation, and the schedule
is not the operator.

## 9. Validation, and the hole in it

**On the corpora: differential testing against the oracle.** The vendored `go/pointer`
lives at `goastcg/testdata/oracle/` (its own Go module; `GOWORK=off`).

**Gate: 126/126 exact on corpora A and B**, matching E24. Both corpora are generics-free,
which is the only reason this gate is available at all.

**On real Go: there is no oracle, and this is an open problem.** Per E28, `go/pointer`
panics on any program containing generic types (`ForEachElement called on type containing
*types.TypeParam`, `pointer/gen.go:1335`), with and without `ssa.InstantiateGenerics`. It
cannot analyze wile, and it cannot analyze most real Go. So step 6 has nothing to
differentially test against, and correctness on real code cannot be established the way
correctness on the corpora can.

Do not paper over this. The candidate substitutes, none of which is equivalent:

- **Soundness envelope.** Our result must be a subset of `'cha`'s and a superset of the
  dynamically-observed truth. Catches unsoundness, not imprecision, and imprecision is
  the entire point of the analysis.
- **Self-consistency under the defeater component.** Every node whose defeater set is
  empty claims exactness; those claims can be spot-checked by hand. Cheap, partial, and
  it does not scale.
- **Differential against `'vta` on the subset where VTA is provably exact.** Narrow, and
  circular in the region we care about.

The honest position: the corpora prove exactness against a real oracle; real Go gets a
soundness envelope plus audited spot checks, and any stronger claim needs a stronger
method than we currently have. **This gap is a work item, not a footnote.**

### The reason a passing result on generic code proves nothing

`gen/hard/` uses a generic container, a generic function, and a nested generic
instantiation, and `go/pointer` resolves it exactly. It survives only because it never
converts a *generic* type into an interface, so no type-parameter-bearing type reaches
`RuntimeTypes()`. The condition is invisible from the call site and cannot be checked in
advance. E27 mistook that accident for a general result and was retracted. **Any oracle
check runs against real generic Go before it is believed.**

## 10. Build order

C3 comes first, and this is not ceremony. If a lattice law is wrong, the fixpoint
silently yields a wrong answer and a loss becomes indistinguishable from a bug. That is
the same silent-failure shape E27 found in the oracle, one layer down.

0. **Verify the algebra's operator laws.** Property tests over `powerset-lattice`,
   `map-lattice`, and the new product lattice: associativity, commutativity, idempotence,
   absorption, monotonicity, finite height. Discharges
   [[2026-07-14-algebraic-bet/C3]], which currently records that the one component with no
   measurement behind it is the one this work depends on.
1. **Rescue the oracle.** Done: `goastcg/testdata/oracle/`. It had been living in a temp
   scratchpad, one cleanup away from taking E24's result with it.
2. **Go: constraint extraction.** SSA to the six forms, plus the method table.
3. **Scheme: the solver.** `lfp` over the product lattice.
4. **Validate on the corpora.** 126/126 against the oracle. This is the only place a hard
   exactness gate is available, because the corpora are generics-free.
5. **Lift `precise.go:66`.** Expose `'andersen` in the call-graph menu.
5b. **Settle the representation question before anything else about cost.**
   [[2026-07-14-pre-composition/E31]]: swap the points-to set from a hash map to a sparse
   bit vector in the Go reference solver, change nothing else, re-validate 126/126, re-run
   wile. One container swap, decisive in both directions, and it gates every cost judgment
   downstream. **Do not tune, do not adopt HVN, and do not pivot to demand-driven CFL until
   this has run** — all three are answers to a diagnosis that has not been confirmed.

   Then the Wile-side form of the same question: can `P(Object)` be represented efficiently
   *inside Wile*? Its native set is a list, which for this carrier is the worst available
   choice. If the answer is no, that is a fact about the substrate, not about the algebra
   (section 8b), and it must be recorded as such.

6. **Run on real Go** (wile itself, ~11.9k call-graph nodes under `'cha`/`'vta`). Two
   distinct questions, and only one of them has a method:
   - *Does it finish?* Measured, and today the answer is **no** (E29, E30). Gated on 5b
     before that answer means anything about the algebra.
   - *Is it right?* **No oracle exists here** (section 9). Soundness envelope plus
     audited spot checks is the best currently available, and closing that properly is
     its own work item.
7. **E26's three-arm experiment.** Control, treatment-`'vta` (the tool sells a bound),
   treatment-andersen (the tool sells a fact), on the same 63 problems. Runs on the
   corpora, so the oracle gap does not block it.

## 11. Risks, named

**Performance. This risk has FIRED, and it fired somewhere other than where this section
predicted.** [[2026-07-14-pre-composition/E29]] and [[2026-07-14-pre-composition/E30]]
measured it. Extraction is free (146 ms, 831,965 constraints on wile). The **solve** does
not converge: 32.3 billion membership tests, timed out at 10 minutes, **in Go**, under
difference propagation, with the solution still growing. Repairing every known precision
defect in the extractor moved mean set size by 7.8% while oracle agreement held at 126/126,
so it is not an artifact of the extractor. No permitted *schedule* helps; difference
propagation fails further into the blowup than naive iteration does.

**But the diagnosis is not what it looks like, and section 8b is why.** That measurement
represented points-to sets as `map[int]bool`, which is precisely the container
`pointer/doc.go` names as the slow one. The 32.3 billion operations are a property of the
container, not of the fixpoint. **The solution size (151M pairs, ~19 MB as bits) is
intrinsic; the operation count is not.**

The open experiment is [[2026-07-14-pre-composition/E31]]: swap in a sparse bit vector,
change nothing else, re-validate 126/126, re-run wile. It is decisive in both directions.
If it converges, the whole-program route is alive and this section is wrong. If it does
not, the cost is intrinsic, that claim will have survived the three explanations that could
have defeated it (precision, schedule, representation), and the design must pivot to
demand-driven CFL or abandon real-code coverage.

**Where the risk really sits, for the Scheme half.** Wile's native set is a **list**, and
for a carrier of this size a list is the worst available representation. The cost question
for `(wile goast andersen)` was never "is the algebra tractable." It is **"can `P(Object)`
be represented efficiently inside Wile."** That is a representation question wearing an
algebra question's clothes, and it is the one to answer.

**If the cost turns out to be intrinsic after all, record what it does and does not show.**
It would refute the algebra as an *implementation substrate* for whole-program analysis at
scale. It would say nothing whatever about the accuracy claim, which is what E26 set out to
measure, and the three-arm experiment would still run on the corpora, where the constraint
sets are small. **A slow container is not a wrong theory**, and neither is a slow
interpreter: if the algebra fails here it must be for a reason attributable to its
operators, not to the substrate they happen to be written in.

**The barred optimizations are a partial escape hatch, and taking them has a cost.** If
step 6 fails on performance, `hvn.go`/`opt.go` show the fast path. Adopting them buys a
usable tool and forfeits the measurement, and that trade must be made explicitly and
written into the notebook, not slid into quietly. Note it is only a *partial* hatch: they
are solver optimizations, so they do nothing about the generics panic. There is no
vendoring route back.

**No oracle for real Go, at step 6.** Ranked with performance as a co-equal risk, because
it is the one with no method behind it rather than merely a hard one. Exactness on real
code is currently unfalsifiable, and an analysis whose correctness cannot be checked where
it is used is worth much less than one whose can. Section 9 lists what is available and
says plainly that none of it is equivalent. **Treat this as a work item.**

**Do not let step 4 stand in for step 6.** Passing 126/126 on generics-free corpora is a
real result and it is not a claim about real Go. E27 was retracted for exactly that
substitution, having generalized a passing result on toy generics into a claim about
production code. The corpora prove what the corpora prove.
