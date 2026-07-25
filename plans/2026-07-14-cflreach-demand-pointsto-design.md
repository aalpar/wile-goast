# E35 — demand-driven field-sensitive points-to (design note)

Probe for notebook `2026-07-14-pre-composition`, entry E35. Tests the algorithm E31
pre-registered and E34 cleared the runway for: does *demand-driven* points-to sidestep the
hot-allocator flood that makes batch context-insensitive Andersen intractable on real Go?

## Thesis under test

Interface object discovery is a set of point queries (one per dispatch site), not a
whole-program fixpoint. A demand-driven engine that computes `pts(v)` only for queried
nodes and their backward cone should avoid materializing the `errors.errorString` flood
(E33) unless an error interface is actually queried.

## Algorithm (v1)

Context-insensitive, field-sensitive CFL-reachability degenerates to the Andersen relation
(the only Dyck language is field-balance, realized by routing load/store through shared
object nodes; the call/return Dyck language is v2). So v1 = "lazy Andersen": same relation
as the E31 batch solver, seeded from a query set, propagated only among **activated** nodes.

- `pts(v)` activates `v` and its backward cone: `copy p=q` -> activate `q`; `gep` -> object
  sub-nodes at `base+off`; `load v=*q` -> activate `q`, and for each `o in pts(q)` activate
  the store-sources feeding `contents(o)`.
- A demand worklist propagates pts among activated nodes to a fixpoint. Activation is
  dynamic: growing `pts(q)` can activate new store-sources, so it is a co-routine of
  "grow the activated set" and "propagate within it". Memoized across queries.
- Query all nodes -> batch Andersen exactly. Query only dispatch sites -> a subset.

Interface dispatch resolution: `pts(recv)` at each `invoke`/`icall` site, then the method
table (reuse E31's `methodOf` / `objFunc`).

## Correctness contract

`demand-pts(v) == batch-pts(v)` for every `v` is a theorem (same relation), hence a
tripwire: an activation-closure bug under-approximates and the check fires. Same role E34's
per-class pts-uniformity gate played.

## Validation targets

1. **126 corpora** (regenerated via `LLMAccuracy/code-restructuring/generate_interface_dispatch.py`,
   corpus A + B): demand dispatch resolution must equal the E31 batch solver on all 126.
   Named benchmark; transitively pinned to go/pointer via E29's 126/126 chain.
2. **soundfix** (E34 fixture): full per-node `pts` equality on a converged, heap-exercising
   target -- covers the load/store path the dispatch-focused corpora leave blind (E33 lesson).
3. **wile**: batch times out, so no full equivalence. Measure materialization -- query only
   dispatch-site interface values, count nodes touched vs the full ~737k. The flood-avoidance
   measurement, i.e. the actual hypothesis.

## Deliverable

`notebooks/2026-07-14-pre-composition.d/E35-cflreach/`, standalone module (`GOWORK=off`),
x/tools pinned v0.42.0:
- `extract.go`, `solve.go` -- frozen copies from E34 (extractor + batch-Andersen oracle).
- `cfl.go` -- the demand engine (new; the one variable).
- `main.go` -- driver: extract -> query -> compare vs batch -> report agreement + materialization.

## v1 boundary

Proves correctness (== Andersen) and measures demand's flood-avoidance. Does NOT prove
precision *gains* -- those need v2 (k-bounded context sensitivity), deferred. If v1 shows
demand touches ~all 737k nodes even for domain queries, the hypothesis is refuted cheaply.

## v2 (E36-rsm/) -- the alias-matching RSM + on-demand context sensitivity

v1 result (E35): correct == batch on 126/126, but the ENGINE over-activates (loose
writer-finding forward-activates a loaded object's whole forward cone, ~12-19% footprint)
and misses multi-hop heap (7/513 soundfix DISPATCH failures). The optimal footprint (backward
slice) is a median of ~11 nodes, but v1 does not realize it. v2 fixes both, then adds context
sensitivity to attack the flooded error-typed tail.

### Phase 1 -- the field-sensitive alias-matching RSM (context-insensitive)

Replace v1's cone activation with targeted alias matching. `pointsTo` and `pointedToBy`
become mutually recursive, memoized (tabled), meeting at `alias`:
- `pointsTo(x)`: addr -> {o}; copy -> pointsTo(y); gep -> {base(o)+off}; load x=*q -> for each
  sub-node s in pointsTo(q): contents(s).
- `contents(s)` = objects stored into s = for each store cStore{p,y} with p a WRITER of s:
  pointsTo(y).
- `writers(s)` = store-pointers p with s in pointsTo(p) = pointedToBy(s) ∩ store-pointers,
  found by following ONLY offset-off's flow (field-sensitive tightening) and the symmetric
  heap-forward (closes the multi-hop misses).
Over E31 the "field f" is the offset (gep = &base.off), so matched put[f]/get[f] = store/load
through the SAME sub-node. Tabling worklist for the fixpoint (pointsTo can depend on itself).
Gate: `rsm-pts == batch-pts` on 126 corpora + soundfix; DISPATCH 513/513 (7 closed); engine
footprint ≈ optimal backward slice (~11-node median).

### Phase 2 -- on-demand context sensitivity (attack the flooded tail)

Add a k-bounded call-site context (regular abstraction of the call/return Dyck language;
interleaving two Dyck languages is undecidable, so fields stay CFL-exact and calls are
k-approximated). Heap cloning gives `errors.New`'s result a per-call-site identity so the
flood never forms. Query-scoped: add context only along the queried receiver's chain.
Validation: context-sensitive pts is a REFINEMENT (subset) of context-insensitive, so it does
NOT equal batch -- instead check (a) SOUNDNESS: no real callee dropped on the 126 corpora
(dispatch still matches go/pointer via the E29 chain, since the corpora have no context-
dependent dispatch so the answer is unchanged); (b) PAYOFF: the flooded receivers' slices
shrink on wile/etcd/ollama (the E35 p90 28k-40k tail comes down).

## References

Terminology note, for honesty: "RSM" (recursive state machine) was loose shorthand in the
E35 write-up. The machinery below is more precisely *CFL-reachability* (Reps), where field-
sensitivity and context-sensitivity are each a balanced-parenthesis (Dyck) language;
recursive state machines / pushdown systems are the equivalent operational model. Our Phase 1
`pt`/`pb`/`ct` mutual recursion is the demand-driven field-sensitive points-to of Sridharan
et al.; Phase 2's on-demand context is Sridharan-Bodik's refinement; the two-Dyck
interleaving that forces an approximation is Reps' undecidability result, and Späth et al.'s
synchronized pushdown systems are the modern way to handle it. Citations verified via web
search on 2026-07-14.

### The CFL-reachability framework

- **Reps, T.** "Program analysis via graph reachability." *Information and Software Technology*
  40(11-12):701-726, 1998. DOI [10.1016/S0950-5849(98)00093-7](https://doi.org/10.1016/S0950-5849(98)00093-7).
  TR: `research.cs.wisc.edu/wpis/papers/tr1386.pdf`.
  The framework: express points-to as graph reachability constrained by a context-free
  language; fields (load/store) and calls (call/return) are each a Dyck language. This is the
  source of the design's "the only Dyck language is field-balance" framing (Phase 1) and the
  call/return Dyck language of Phase 2.
- **Reps, T., Horwitz, S., Sagiv, M.** "Precise interprocedural dataflow analysis via graph
  reachability." *POPL 1995*, pp. 49-61. DOI [10.1145/199448.199462](https://doi.org/10.1145/199448.199462).
  IFDS; the call/return balanced-parenthesis language that Phase 2's context-sensitivity uses.

### Demand-driven points-to — the core of Phases 1 and 2

- **Sridharan, M., Gopan, D., Shan, L., Bodik, R.** "Demand-driven points-to analysis for
  Java." *OOPSLA 2005*, pp. 59-76. PDF: `manu.sridharan.net/files/oopsla05.pdf`.
  The field-sensitive demand-driven `flowsTo` / `pointsTo` / `alias` CFL formulation. Phase 1's
  `pt` / `pb` (pointedToBy) / `ct` (contents) mutual recursion is exactly this, mapped onto
  E31's offset-as-field object model.
- **Sridharan, M., Bodik, R.** "Refinement-based context-sensitive points-to analysis for
  Java." *PLDI 2006*. DOI [10.1145/1133981.1134027](https://doi.org/10.1145/1133981.1134027).
  TR UCB/EECS-2006-31. Adds context sensitivity *on demand* by refinement (add the call Dyck
  language only where it sharpens the answer). The blueprint for Phase 2, including the
  query-scoped "add context only along the receiver's chain" idea.

### Why an approximation is unavoidable (the undecidability)

- **Reps, T.** "Undecidability of context-sensitive data-dependence analysis." *ACM TOPLAS*
  22(1):162-186, 2000. DOI [10.1145/345099.345137](https://doi.org/10.1145/345099.345137).
  The free interleaving of two Dyck languages (fields x calls) is not context-free and its
  reachability is undecidable -- so Phase 2 must approximate one dimension (fields stay exact,
  calls are k-bounded, or the SPDS split below).

### Related demand-driven pointer/alias analysis (C)

- **Heintze, N., Tardieu, O.** "Demand-driven pointer analysis." *PLDI 2001*, pp. 24-34.
  DOI [10.1145/378795.378802](https://doi.org/10.1145/378795.378802).
  Field-*based* (not field-sensitive), context-insensitive demand pointer analysis for C; the
  "do only the work each query needs" philosophy the whole line rests on.
- **Zheng, X., Rugina, R.** "Demand-driven alias analysis for C." *POPL 2008*, pp. 197-208.
  DOI [10.1145/1328897.1328464](https://doi.org/10.1145/1328897.1328464).
  Demand alias via CFL-reachability; shows alias queries can do strictly less work than
  points-to queries -- relevant since E35's flood question is really an alias question.

### Modern handling of the field x context interleaving (Phase 2 options)

- **Spath, J., Ali, K., Bodden, E.** "Context-, flow-, and field-sensitive data-flow analysis
  using synchronized pushdown systems." *PACMPL* 3(POPL):48:1-48:29, 2019.
  DOI [10.1145/3290361](https://doi.org/10.1145/3290361).
  SPDS: encode calls and fields as two *separate, synchronized* pushdown systems, each solved
  exactly, approximating only where they genuinely interleave (rare in practice). A cleaner
  alternative to naive k-bounding for Phase 2.
- **Spath, J., Nguyen Quang Do, L., Ali, K., Bodden, E.** "Boomerang: Demand-driven flow- and
  context-sensitive pointer analysis for Java." *ECOOP 2016*. LIPIcs vol. 56, 22:1-22:26.
  DOI [10.4230/LIPIcs.ECOOP.2016.22](https://doi.org/10.4230/LIPIcs.ECOOP.2016.22).
  The closest existing *system* to what E36 targets: a practical demand-driven, context- and
  flow-sensitive pointer analysis. Worth reading before reimplementing, to steal its
  engineering (the query strategy, the boomerang/backward-then-forward structure).

### The inclusion-based baseline (the equivalence theorem's other side)

- **Andersen, L.O.** "Program analysis and specialization for the C programming language."
  PhD thesis, DIKU, University of Copenhagen, 1994.
  The inclusion-based points-to that context-insensitive CFL-reachability equals -- the other
  half of Phase 1's `rsm-pt == batch-pt` gate.
- **Hardekopf, B., Lin, C.** "Exploiting pointer and location equivalence to optimize pointer
  analysis." *SAS 2007*. The HVN of [[E34]] (pointer/location equivalence); same authors' PLDI
  2007 "The Ant and the Grasshopper" is the scalable inclusion-based solver.
