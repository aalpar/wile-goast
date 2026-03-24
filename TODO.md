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

### B1. Scheme utils (no dependencies)

- [ ] Implement `ast-transform` in `cmd/wile-goast/lib/wile/goast/utils.scm`
- [ ] Implement `ast-splice` in `cmd/wile-goast/lib/wile/goast/utils.scm`
- [ ] Add `take` and `drop` to `utils.scm`
- [ ] Export new functions from `utils.sld`

### B2. go-cfg-to-structured (no dependencies)

- [ ] Implement `go-cfg-to-structured` Go primitive in `goast/`
  - Case 1: linear with early returns → nested if/else
  - Case 2: early returns inside loops → _r0/_done/break rewrite

### B3. go-cfg-to-structured improvements (depends on B2)

- [ ] Handle goto / labeled branches (currently returns #f)
- [ ] Handle switch/select with early returns
- [ ] Handle multiple return values (_r0, _r1, ...)

## Other

- [ ] Move `stores-to-fields` predicate to Go side
