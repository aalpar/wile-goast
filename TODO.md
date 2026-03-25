# TODO

Top-level task: composable API for wile-goast analysis and transformation (Go code inlining).
Two independent tracks — shared sessions and transformation primitives — converge
at the inlining pipeline. See `plans/2026-03-24-transformation-primitives-design.md`.

## Track A: Shared Session API — DONE (v0.5.0)

Completed 2026-03-24. GoSession holds loaded packages, lazy SSA/callgraph.
All 7 package-loading primitives accept GoSession or string. Belief DSL
creates session in `make-context`. See `plans/2026-03-24-shared-session-impl.md`.

## Track B: Transformation Primitives

Scheme-level tree rewriting and Go-level control flow restructuring for
refactoring operations (inlining, extraction, code motion).

### B1. Scheme utils — DONE

- [x] Implement `ast-transform` in `cmd/wile-goast/lib/wile/goast/utils.scm`
- [x] Implement `ast-splice` in `cmd/wile-goast/lib/wile/goast/utils.scm`
- [x] Add `take` and `drop` to `utils.scm`
- [x] Export new functions from `utils.sld`

### B2a. go-cfg-to-structured — Case 1 (no dependencies) — DONE

- [x] Linear early returns → nested if/else

### B2b. go-cfg-to-structured — Case 2 (depends on B2a) — DONE

Completed 2026-03-25. Returns inside for/range are rewritten as
`_ctl<N> = K; break` with guard-if-return statements after the loop.
Composes with Case 1 (guard folding) in a single call. Supports nested
loops (bottom-up) and multiple return sites per loop.
See `plans/2026-03-25-loop-return-restructuring.md`.

### B3. go-cfg-to-structured improvements (depends on B2)

- [ ] Handle goto / labeled branches (currently returns #f)
- [ ] Handle switch/select with early returns inside loops (needs labeled break)
- [ ] Handle multiple return values (_r0, _r1, ...)

## Track C: Static Analysis Forms (depends on Wile algebra library)

Wile gets a general-purpose algebra library (`(wile algebra)` or similar).
wile-goast builds static-analysis combinators on top. Items below are
wile-goast consumers — they migrate to or are built on the Wile algebra API
once it exists.

### C1. Migrate existing hand-rolled algebra

- [ ] `checked-before-use` Kleene iteration (`belief.scm:698`) → fixpoint over powerset lattice
- [ ] `ssa-normalize` rewrite rules (commutativity, identity, annihilation) → monoid/ring axiom application
- [ ] `score-diffs` similarity accumulation (`unify.scm`) → semiring-like weighted scoring

### C2. Dataflow analysis framework

- [ ] Define transfer function interface (per SSA instruction type)
- [ ] Forward/backward analysis combinator over CFG blocks (reverse postorder)
- [ ] Worklist algorithm integrated with CFG block ordering
- [ ] Per-variable analysis via map lattice (vars → lattice values)
- [ ] Product lattice for combining analysis dimensions
- [ ] Monotonicity assertion (debug mode) — detect buggy transfer functions

### C3. Pre-built abstract domains

- [ ] Powerset lattice — liveness, reaching definitions
- [ ] Flat lattice (⊥ < concrete values < ⊤) — constant propagation
- [ ] Sign lattice ({⊥, -, 0, +, ⊤})
- [ ] Interval lattice — range analysis

### C4. Path algebra on call graphs

- [ ] Boolean semiring — reachability (generalize `go-callgraph-reachable`)
- [ ] Tropical semiring — shortest/longest call chains
- [ ] CFL-reachability — context-sensitive analysis

### C5. Galois connections for abstract interpretation

- [ ] Abstraction/concretization pair interface
- [ ] Soundness check (alpha ∘ gamma ⊒ id)
- [ ] Connect Go concrete values to abstract domains

### C6. Belief DSL integration

- [ ] Belief graduation — 100% adherence beliefs become dataflow assertions
- [ ] Belief-defined lattices — express belief checkers as lattice transfer functions

## Other

- [ ] Move `stores-to-fields` predicate to Go side
  - Sub-tree matching (fragment detection within functions)
  - CFG isomorphism as a standalone tool
  - Call graph context pre-filtering
  - Integration into the belief DSL
  - --emit mode for the unification detector

