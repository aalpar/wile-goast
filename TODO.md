# TODO

Top-level task: composable API for wile-goast analysis and transformation (Go code inlining).
Two independent tracks — shared sessions and transformation primitives — converge
at the inlining pipeline. See `plans/2026-03-24-transformation-primitives-design.md`.

## Track A: Shared Session API

Eliminate redundant `packages.Load()` calls by introducing a first-class session
object that all layers accept. Today each primitive independently loads, type-checks,
and builds SSA for the same package. A single `go-load` call should feed all of them.

### A1. Wile core: OpaqueValue (blocked — wile changes in progress)

- [ ] Add `OpaqueValue` type to `wile/values/` (~80 lines + tests)
  - `SchemeString()` → `#<tag:id>` display
  - `IsVoid()`, `EqualTo()` (identity-based)
  - Type predicate primitive: `(opaque? v)`
  - Tag accessor primitive: `(opaque-tag v)`

### A2. GoSession type (depends on A1)

- [ ] Define `GoSession` struct in `goast/` implementing `values.Value`
  - Wraps: `[]*packages.Package`, `*token.FileSet`
  - Lazy slots: `*ssa.Program`, `*callgraph.Graph`, field index
  - Builds SSA/callgraph on first demand, caches for reuse
- [ ] Implement `go-load` primitive: `(go-load pattern ... . options)` → GoSession
  - Accepts multiple patterns (loaded as roots into a single session)
  - `'lint` option upgrades to `LoadAllSyntax`
- [ ] Implement `go-list-deps` primitive: `(go-list-deps pattern ...)` → list of import paths
  - Lightweight: `NeedName | NeedImports` only, no type checking
  - Returns transitive closure for scope discovery before loading
- [ ] Add `go-session?` type predicate

### A3. Refactor existing primitives to accept GoSession (depends on A2)

Each primitive accepts either a pattern string (backward compatible, loads fresh)
or a GoSession (reuses loaded state).

- [ ] `go-typecheck-package`: accept GoSession or string
- [ ] `go-ssa-build`: accept GoSession or string
- [ ] `go-ssa-field-index`: accept GoSession or string
- [ ] `go-cfg`: accept GoSession or string
- [ ] `go-callgraph`: accept GoSession or string
- [ ] `go-analyze`: accept GoSession or string
- [ ] `go-interface-implementors`: accept GoSession or string

### A4. Update Scheme layer (depends on A3)

- [ ] Update belief DSL (`belief.scm`) to create GoSession in `make-context`, pass to all primitives
- [ ] Update `docs/PRIMITIVES.md` with `go-load`, `go-session?`, dual-accept signatures

### A5. Design doc

- [x] Write `plans/2026-03-24-shared-session-design.md`

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
