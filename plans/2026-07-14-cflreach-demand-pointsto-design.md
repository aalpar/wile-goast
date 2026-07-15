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
