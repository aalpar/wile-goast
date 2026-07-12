---
name: goast-semiring-queries
description: >
  Use when a Go structural question reduces to a path or relationship over the
  call graph — "can X reach Y", "what's the blast radius of changing Y",
  "cheapest/shortest call chain", "path that touches/avoids Z", "how many call
  paths", "is X recursive / mutually recursive". Maps the question to the right
  wile-goast semiring recipe (boolean / tropical / counting / SCC).
---

# Semiring queries over the Go call graph

A path question over a call graph is an algebra question. Pick the semiring and
the query writes itself. Do not hand-roll a traversal.

## Routing: derive the semiring in three lines

1. **What do you accumulate along a path?** A yes/no, a cost, a count.
2. **What is ⊕ (combine two alternative paths)?** OR, min, +.
3. **What is ⊗ (extend a path by one edge)?** AND, +, ×.

Those two operators name the semiring: OR/AND → boolean · min/+ → tropical ·
+/× → counting. If your answer doesn't fit, it is probably not a semiring
question — see [Not a semiring question](#not-a-semiring-question).

## Setup, once

```scheme
(import (wile goast callgraph)     ; go-callgraph
        (wile goast path-algebra)  ; make-path-analysis, path-query, ...
        (wile goast utils)         ; nf — field access on cg-node / cg-edge
        (wile algebra semiring))   ; boolean-semiring, tropical-semiring, ...

(define cg (go-callgraph "«pattern»" 'vta))
```

Build the graph **once** and pass it around; name the algorithm explicitly
(`'static` < `'cha` < `'rta` < `'vta` in precision and cost; `'rta` errors
without a `main`). Rebuilding per query is the dominant cost in a slow session.

Call-graph records are accessed with `nf`, not per-field accessors:
`(nf edge 'callee)`, `(nf edge 'caller)`, `(nf edge 'pos)`, `(nf node 'name)`.
There is no `cg-edge-callee`.

---

## R1 · Reachability — boolean

**Intent triggers:** can X reach Y · what does X reach · what reaches Y · does
this handler ever hit the DB · blast radius of changing Y

**Reduction principle:** accumulate *reachable-or-not*. ⊕ = OR (any one path
suffices), ⊗ = AND (a path exists only if every edge on it does). → **boolean**.

**Structure:** `(boolean-semiring)`, unit weights (`#f`).

**Recipe:**

```scheme
(go-callgraph-reachable cg "«root»")    ; forward: everything root can reach
(go-callgraph-reaching  cg "«target»")  ; backward: transitive callers = blast radius

(path-query (make-path-analysis (boolean-semiring) cg #f)
            "«src»" "«dst»")            ; point-to-point => #t / #f
```

**Projection:** the two pre-built reductions return **sorted name lists**,
already answer-shaped — including the node itself, or `'()` if it isn't in the
graph. Blast-radius size is `(length …)`. The point-to-point form is a bare
boolean.

**Escalation:** need the *cost* → R2 · *how many* routes → R5 · reach along only
*some* edges → R3.

---

## R2 · Cheapest / shortest path — tropical (min-plus)

**Intent triggers:** fewest hops from X to Y · shortest call chain · cheapest
path where each call costs W

**Reduction principle:** accumulate a *cost*. ⊕ = min (keep the cheaper
alternative), ⊗ = + (extending a path adds the edge's cost). → **tropical**.

**Structure:** `(tropical-semiring)`, edge-weight `(lambda (e) «cost»)`.
A `#f` weight means unit weights, so the distance is a **hop count**.

**Recipe:**

```scheme
(path-query (make-path-analysis (tropical-semiring) cg (lambda (e) 1))
            "«src»" "«dst»")
```

**Projection:** the returned number *is* the answer. The semiring's zero is
`tropical-inf` — an infinite distance means **unreachable**, not "cost 0".

**Escalation:** you want the *witnessing nodes*, not just the cost — the kernel
returns the distance only; path reconstruction does not exist today. Fall back
to R1 to confirm reachability, then narrow by hand.

---

## R3 · Predicate-weighted path — tropical + custom edge weight

**Intent triggers:** cheapest path that touches a lock · shortest chain that
avoids the cache · prefer paths through the validated API

**Reduction principle:** the R2 min-plus skeleton, with the predicate folded
**into the edge weight** so that the minimum-cost answer encodes the property.
Weight the edges you want 0 and the ones you don't 1. This is the lever: a
property question becomes a cost question.

**Structure:** `(tropical-semiring)`, edge-weight encodes `«pred?»`.

**Recipe:**

```scheme
(make-path-analysis (tropical-semiring) cg
                    (lambda (e) (if («pred?» (nf e 'callee)) 0 1)))
```

**Projection:** the distance means whatever your encoding says it means, and
nothing else. **State the interpretation before reporting a number** — see the
worked trace below. This recipe will happily emit a runnable query that answers
a different question than the one asked.

**Escalation:** the predicate is over a *path*, not an edge ("acquires **then**
releases", "validated before use") → ordering is not expressible in an edge
weight. That needs a product lattice / dataflow analysis: leave the semiring
family and use `(wile goast dataflow)`'s `run-analysis`.

---

## R4 · Cycles / mutual recursion — SCC side-API

**Intent triggers:** is X recursive · which functions are mutually recursive ·
are there call cycles

**Reduction principle:** strongly-connected components partition the graph, and
a *non-trivial* SCC (more than one node, or a self-loop) is exactly a
mutual-recursion cluster. Structural decomposition rather than a path query —
but it is the SCC face of the *same* path-analysis object, so it costs nothing
extra once you've built one.

**Structure:** any path analysis; `(boolean-semiring)` with `#f` weights is the
cheap default.

**Recipe:**

```scheme
(define pa (make-path-analysis (boolean-semiring) cg #f))

(path-cyclic-nodes    pa)          ; every node on some cycle
(path-node-in-cycle?  pa "«fn»")   ; membership => #t / #f
(path-analysis-sccs   pa)          ; the clusters themselves
```

**Projection:** names, and a boolean. Report clusters, not raw indices.

**Escalation:** counting paths over a graph that has cycles → R5's caveat.

---

## R5 · Distinct path count — counting

**Intent triggers:** how many distinct call paths from X to Y · is there more
than one route to Y

**Reduction principle:** accumulate a *count*. ⊕ = + (alternative paths sum),
⊗ = × (independent choices along a path multiply). → **counting**.
`bigint-counting-semiring` is the same arithmetic with a carrier annotation that
opts into a bignum fast-path kernel.

**Structure:** `(bigint-counting-semiring)`, unit weights (`#f` — the fast path
has no edge-data slot; a non-`#f` weight silently falls back to the slow loop).

**Recipe:**

```scheme
(define pa (make-path-analysis (bigint-counting-semiring) cg #f))
(path-analysis-fast-path? pa)      ; confirm the kernel attached
(path-query pa "«src»" "«dst»")    ; => count
```

**Caveat — this is the recipe that bites.** Path counts are only finite on a
**DAG**; on a cyclic subgraph the count diverges. Boolean and tropical converge
on cycles because their ⊕ is idempotent (`a ⊕ a = a`); plain `+` is not, so
counting has no such guarantee. The library encodes this:

```scheme
(semiring-cycle-safe? (bigint-counting-semiring))       ; => #f
(semiring-cycle-safe? (saturating-counting-semiring 2)) ; => #t (absorbing top at cap)
```

Check `(path-node-in-cycle? pa "«src»")` (R4) before trusting a count. If the
region is cyclic, either condense the SCCs first, or switch to
`(saturating-counting-semiring «cap»)`, which converges because the cap is an
absorbing top. Note it takes the cap as an argument — unlike the other
constructors, it is not nullary. A cap of 2 answers "one route or many" and
nothing finer, which is usually the question anyway.

---

## Worked trace — R3, encoding pinned

*"What's the cheapest call path from `svc.Handle` to `db.Query` that touches a
lock?"*

**1 — build the graph once.**

```scheme
(define cg (go-callgraph "./..." 'vta))
```

**2 — choose the predicate.** It must be a fact about a *single edge*, decidable
from the edge's fields. Here: does this call land on a lock acquisition?

```scheme
(define (lock-touching? callee)
  (or (string-contains? callee ").Lock")
      (string-contains? callee ").RLock")))
```

**3 — wire it into the edge weight.** Edges we want cost nothing; every other
edge costs one.

```scheme
(define pa
  (make-path-analysis (tropical-semiring) cg
                      (lambda (e) (if (lock-touching? (nf e 'callee)) 0 1))))

(define d (path-query pa "svc.Handle" "db.Query"))
```

**4 — state what `d` means under *this* wiring, before reporting it.**

> Weight 0 iff the callee acquires a lock, 1 otherwise. So `d` is the number of
> **non-lock hops** on the cheapest path from `svc.Handle` to `db.Query`.
> `d = 0` means a path exists on which *every* call is a lock acquisition.
> `d > 0` does **not** mean "no lock is touched" — it counts the ordinary hops
> and says nothing about whether a lock was crossed along the way.
> `d = tropical-inf` means there is no path at all.

That last paragraph is the deliverable, not the number. **The min-cost path
under this encoding is the one that maximizes lock edges, which is not the same
question as "is there a lock-touching path."** If the real question is the
latter, this encoding cannot answer it: reach for R1 restricted to a
lock-touching subgraph, or filter the edges first. Pin the interpretation before
you trust the query — an encoding that answers a neighboring question runs
perfectly and lies quietly.

---

## Not a semiring question

Escalate out rather than forcing a fit:

| Question shape | Go there instead |
|---|---|
| Ordering along a path ("acquired *then* released") | `(wile goast dataflow)` — `run-analysis`, product lattices |
| Are these two functions duplicates | `find_duplicates` / AST diff |
| Which struct fields are a false boundary | `find_false_boundaries`, `(wile goast fca)` |
| Does every Lock have an Unlock (a consistency invariant) | belief DSL — `check_beliefs` |
| Is this value checked before use | `(wile goast dataflow)` — `defuse-reachable?` |
| Anything textual | grep |

When unsure which analysis *layer* a question belongs to, use the
`goast-analyze` prompt.

## Grounding rules

- **This skill names recipes, not arities.** Confirm exact call signatures with
  the `reference` tool before running. Do not guess a primitive into existence —
  edge fields come from `nf`, and there is no `cg-edge-callee`, no
  `graph-query` on the goast surface (it is `path-query`), and
  `count-paths-in-dag` is an internal kernel the dispatcher picks, not something
  you call.
- **Verify recipe edits with `go test`, not `mcp__wile-goast__eval`** — the MCP
  server can serve a stale binary and will happily run yesterday's library.
- **Project before returning.** Reduce to names and positions; an unprojected
  path analysis or raw cg-node list will be truncated and is unreadable anyway.
