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

### B2b. go-cfg-to-structured — Case 2 (depends on B2a)

- [ ] Early returns inside loops → _r0/_done/break rewrite

### B3. go-cfg-to-structured improvements (depends on B2)

- [ ] Handle goto / labeled branches (currently returns #f)
- [ ] Handle switch/select with early returns
- [ ] Handle multiple return values (_r0, _r1, ...)

## Other

- [ ] Move `stores-to-fields` predicate to Go side
