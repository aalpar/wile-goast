# CFL Consumer — Composable Interprocedural Taint (IFDS-over-CFL) — Design

**Date**: 2026-06-06.
**Status**: Design draft. Implementation plan (`-impl.md`) to follow after approval.
**New libraries**: `(wile goast ifds)` (generic engine) + `(wile goast taint)` (first instantiation).
**Consumer of**: `(wile algebra cfl)` (shipped in wile, PR #766) — the CFL-reachability substrate.
**Closes**: wile-goast Track C4 adoption follow-up (CFL-reachability — context-sensitive analysis).

## Motivation

wile shipped `(wile algebra cfl)` — CFG-grammar-constrained reachability over labeled
graphs. This builds the first wile-goast consumer: a **composable interprocedural
taint analysis** for security (untrusted-source → dangerous-sink flow), with its
internals factored as a **generic, domain-parameterized IFDS engine** so taint is the
first *instantiation* rather than a one-off.

Two design goals drive the shape:

1. **Initially useful + generic.** A reusable IFDS-over-CFL core, with taint (the
   smallest, highest-value security instantiation) shipped on top.
2. **LLM-composable queries.** Primitives take *predicates* in and return *data* out, so
   an LLM can author source/sink/sanitizer sets (or reuse existing belief/AST queries),
   run a flow query, and combine the result with the call graph, FCA, or belief checkers.

## Why CFL (and not the shipped boolean reachability)

Boolean-semiring reachability already ships (`go-callgraph-reachable`) and answers "can
`f` reach `g`." CFL earns its keep only on **precision**: a value flows *into* a callee
(call = open bracket) and *back out* (return = close bracket), and CFL enforces that
returns match their calls — filtering interprocedurally **un**realizable paths (a call
that "returns" to the wrong caller). A pure forward call-graph walk has no return edges,
so CFL there is vacuous; therefore any non-trivial consumer is, at minimum, **lite-IFDS**
(model param-in / return-out).

**Realizable-path grammar (not strict Dyck).** A taint flow may end *deep in the call
stack* — returns must match, but trailing un-returned calls are fine. So taint uses a
*matched-but-left-open* grammar, **not** the `dyck-grammar` preset:

```
R -> eps | R R | O R | O R C       ; matched, with un-returned (open) calls allowed
;; built from the general cfl kernels: cfl-epsilon / cfl-binary / cfl-terminal,
;; with O_i -> call_i and C_i -> return_i terminals per call site.
```

This is exactly the non-Dyck CFG the general `(wile algebra cfl)` kernels were built for.
(The exact normalized productions are pinned in the impl plan.)

## Architecture — two layers

### Layer 1 — `(wile goast ifds)` (generic engine)

Domain-parameterized realizable-path data-flow, reducing to a `(wile algebra cfl)` solve.

```
(make-ifds-problem
  domain          ; finite set of data-flow facts (list); the engine explodes nodes to (point . fact)
  flow-functions  ; how facts transfer along intra / call / return edges (see below)
  supergraph)     ; the interprocedural graph the facts flow over
  -> <ifds-problem>

(ifds-solve problem source-facts)   ; source-facts: list of (point . fact)
  -> <ifds-result>                  ; queryable (point, fact) reachability relation

(ifds-holds? result point fact)     ; is FACT reachable at POINT along a realizable path?
(ifds-facts-at result point)        ; -> list of facts holding at POINT
(ifds-witness result point fact)    ; -> a realizable witness path (or #f)
```

Internally: explode the supergraph to `(point . fact)` nodes, label call/return edges
with per-call-site brackets, build the realizable-path grammar, run `cfl-solve`, and read
back via `cfl-reachable?`/`cfl-derives?`. The engine is the only place that touches
`(wile algebra cfl)`.

### Layer 2 — `(wile goast taint)` (security flagship)

Instantiates the engine with the one-bit domain `{tainted}` over the call graph.

```
(taint-flows cg sources sinks [sanitizers])
  -> list of (source-fn sink-fn . witness-path)     ; structured, composable data

;; sources / sinks / sanitizers are predicates (cg-node -> bool).
```

**Function-summary model.** Nodes = functions. Each call site `f -call-> g` contributes a
`call_i` (open) edge `(f, call_i, g)` and a `return_i` (close) edge `(g, return_i, f)`. A
function is **taint-transparent** (passes taint through) unless it satisfies the sanitizer
predicate. A source function introduces the `tainted` fact; a sink is a query target.

### Scale strategy — boolean pre-slice, then CFL-refine

CFL is `O(N^3 |G|)`; the goast call graph alone is ~16k nodes, so whole-program CFL is
infeasible. `taint-flows` therefore:

1. Uses the **shipped boolean `go-callgraph-reachable`** (cheap) to compute the **slice** of
   functions that lie on a path from some source to some sink (forward-reach from sources ∩
   backward-reach into sinks).
2. Runs the cubic CFL/IFDS pass **only on that slice** (usually far smaller than the whole
   program).

This bounds `N` and *is* the "combine a cheap query with an expensive one" composition the
design is built around. If a slice is still large, `taint-flows` logs the bound it hit (no
silent truncation) — see `wile/CLAUDE.md` "No silent caps."

## Composability surface (LLM-facing)

Predicate **builders** so sources/sinks/sanitizers can be supplied as data or reused from
existing queries:

```
(taint-from-names '("os/exec.Command" "database/sql.(*DB).Query" ...))  ; exact FQNs
(taint-from-pattern "net/http.*FormValue")                              ; glob over FQN
(taint-from-belief belief-result)                                       ; wrap a belief/AST query
```

Outputs are plain data (lists of flows with witness paths), so results compose with the
call graph, FCA concepts, belief checkers, and the MCP query surface.

### Default Go security source/sink set

Ship a small, curated starter set (`taint-default-sources` / `-sinks` / `-sanitizers`) so
the analysis is useful out of the box:

- **Sources** (untrusted input): `net/http` request accessors (`FormValue`, `Query`,
  `Header.Get`, body reads), `os.Args`, `os.Getenv`, flag values, `bufio`/stdin reads.
- **Sinks** (dangerous): `os/exec.Command`/`CommandContext`, `database/sql` `Query`/`Exec`
  (string-built SQL), `os.Open`/`ReadFile` with a path argument, `html/template` /
  `text/template` execution, `net/http` redirect/`Write` of attacker data.
- **Sanitizers**: a small, conservative set (e.g. `strconv.Atoi`, `filepath.Clean`,
  parameterized-query markers) — extensible by the caller.

The default set is a *starting point*, explicitly overridable/extendable via the predicate
builders.

## Honest limitations (documented, not hidden)

- **Function granularity over-approximates.** No intraprocedural def-use, so the analysis
  cannot tell *which* argument is tainted or whether a function actually routes its tainted
  input into its dangerous call. It treats functions as taint-transparent ⇒ **false
  positives** (sound-ish, not precise). This is stated in the docstrings and the output.
- **The v1 instantiation exercises only a thin slice of the engine.** A one-bit domain at
  function granularity makes the `(point, fact)` explosion nearly degenerate — taint-v1 is
  effectively *realizable-path reachability between source and sink sets, with sanitizers as
  cut nodes*. Building the engine generic anyway is the deliberate, approved choice: the full
  `(point, fact)` machinery is what richer domains and statement-level precision will exercise
  later. We are not claiming v1 stresses the general engine — only that it seeds it.
- **Statement/SSA-level precision is a follow-up** (own design), as is whole-program
  scalability via IFDS **tabulation** (procedure summaries) — `(wile algebra cfl)`'s
  all-pairs solve is the textbook-but-cubic formulation, fine on slices, not whole-program.
- **Release builds:** wile-goast resolves the local wile via `go.work`; a *release* build
  needs a wile tag containing `(wile algebra cfl)` + a `go.mod` bump. Tracked, not blocking
  local development.

## The canary (core acceptance criterion)

A program where **boolean reachability says the source reaches the sink, but the only call
path is interprocedurally unrealizable**, so taint correctly reports **no flow** — the
precision boolean reachability cannot produce. Mirrors the `(wile algebra cfl)` canary.

Shape: two callers `A`, `B` of a shared helper `p`; `A` calls a source, `B` reaches a sink;
the only `source ⇝ sink` call sequence requires entering `p` from `A` and returning into
`B` (mismatched call/return). Boolean reachability: flow. Realizable taint: no flow.

## Files

- `lib/wile/goast/ifds.scm` + `.sld` — generic engine.
- `lib/wile/goast/taint.scm` + `.sld` — taint instantiation + predicate builders + default
  source/sink/sanitizer set.
- Tests mirroring the suite's `*-test.scm` convention, including the canary.
- Doc/MCP surfacing consistent with existing goast analyses (e.g. a `taint-flows` entry in
  the query surface) — scope of surfacing decided in the impl plan.

## Out of scope (follow-ups)

- Statement/SSA-level (precise) taint.
- Additional IFDS instantiations (reaching-definitions, null/nil-flow, simple typestate).
- IFDS tabulation (summary-based) for whole-program scale.
- IDE (edge-function) problems (constant propagation, etc.).

## References

- Reps, Horwitz, Sagiv (1995). "Precise interprocedural dataflow analysis via graph
  reachability." POPL. (IFDS = finite distributive subset data-flow ⟶ CFL-reachability.)
- Reps (2000). "Undecidability of context-sensitive data-dependence analysis." (Why one
  pushdown stack is the ceiling — context XOR field-sensitivity, not both unbounded.)
- `(wile algebra cfl)` — `wile/plans/2026-06-05-cfl-reachability-{design,impl}.md`.
- `lib/wile/goast/path-algebra.scm` — the sibling boolean/tropical reachability consumer
  this parallels (and whose `go-callgraph-reachable` the slice step reuses).
