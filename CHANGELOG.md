# Changelog

## v0.5.111 — Lint and Session Cleanup

- Fix compound if-init lint violations across all session-dispatch sites
- Wrap GoSession in OpaqueValue (wile registry refactor)
- Extract algebra libraries to wile upstream: FCA, Pareto, interval, graph algorithms
- `run-beliefs` returns structured alists (name, type, status, ratio, deviations)

## v0.5.95 — Package Splitting and Aggregate Beliefs

New libraries: `(wile goast split)`, `(wile goast fca-recommend)`

Package splitting analysis via IDF-weighted Formal Concept Analysis on
per-function import signatures:

- `go-func-refs` — per-function external reference profiles via `types.Info.Uses`
- `import-signatures`, `compute-idf`, `filter-noise` — dependency extraction and weighting
- `build-package-context`, `refine-by-api-surface` — FCA at package and API granularity
- `find-split` — min-cut two-way partition via concept lattice
- `verify-acyclic` — import cycle check for proposed splits
- `recommend-split` — top-level entry point: IDF + FCA + min-cut + cycle check + confidence

Function boundary recommendations via FCA + SSA cross-flow:

- `split-candidates` — functions serving incomparable state clusters
- `merge-candidates` — functions maintaining shared state separately
- `extract-candidates` — sub-operations shared by more callers than the full op
- `boundary-recommendations` — three Pareto frontiers (split/merge/extract)

Belief DSL extensions:

- `define-aggregate-belief` — whole-package property evaluation
- `all-functions-in` site selector
- `single-cluster` aggregate analyzer (bridges `recommend-split`)
- `goast-split` MCP prompt for package cohesion analysis

## v0.5.49 — Abstract Domains and Path Algebra

New libraries: `(wile goast domains)`, `(wile goast path-algebra)`

Pre-built abstract domains for `run-analysis`:

- `make-reaching-definitions` — forward reaching definitions (powerset)
- `make-liveness` — backward liveness analysis (powerset)
- `make-constant-propagation` — forward constant propagation (flat + map-lattice)
- `make-sign-analysis` — forward sign analysis with transfer tables
- `make-interval-analysis` — forward interval analysis with per-block widening

Semiring-parameterized path computation over call graphs:

- `make-path-analysis` — lazy single-source Bellman-Ford with per-source caching
- `path-query`, `path-query-all` — query semiring values between functions

Other:

- `ssa-equivalent?` — algebraic equivalence via `discover-equivalences`
- SSA normalization migrated to `(wile algebra symbolic)` named-axiom/theory
- FCA closure operator formalized via `(wile algebra closure)`
- `LoadPackagesChecked` helper consolidates package loading across all sub-extensions
- Function name forms standardized: Form 3 qualified names in typed ASTs
- Restructurer refactored: `loopRewriter` struct, unified switch/typeswitch/select dispatch
- golangci-lint config with project-specific ruleguard rules
- Apache 2.0 license

## v0.5.5 — False Boundary Detection via FCA

New library: `(wile goast fca)`

Discovers natural field groupings from SSA access patterns using Formal
Concept Analysis (Ganter & Wille, 1999), then compares against actual
struct boundaries. Mismatches are false boundary candidates.

- `concept-lattice` — NextClosure algorithm enumerates all formal concepts
- `field-index->context` — bridge from `go-ssa-field-index` to FCA
- `cross-boundary-concepts` — filter concepts spanning multiple struct types
- `boundary-report` — structured output (MCP-compatible)
- `'cross-type-only` pre-filter for large codebases
- Hash table lookups in intent/extent for scalability

Validated on wile codebase: 24 cross-boundary findings across 131 cross-type
functions, including MachineContext/vmState continuation coupling and
Binding/BindingMeta co-mutation patterns.

## v0.5.4 — Dataflow Analysis Framework

- `run-analysis` — worklist-based forward/backward dataflow over SSA blocks
- `analysis-in`, `analysis-out`, `analysis-states` — query block-level results
- `block-instrs` — extract instruction list from SSA block
- `checked-before-use` migrated to `(wile goast dataflow)` product lattice
- Wile StdLibFS wired for `(wile algebra)` access in production

## v0.5.3 — Control Flow Restructuring

- `go-cfg-to-structured` handles early returns inside for/range loops
- Returns rewritten as control variables with guard-ifs after the loop
- Labeled break for switch/select returns in loops
- Result variable synthesis for multiple return values
- Forward and backward goto elimination

## v0.5.2 — Belief DSL Validation

Validated all 4 belief categories against synthetic testdata and etcd raft.

Bug fixes:
- `ordered` checker uses SSA blocks, not CFG (instruction-level precision)
- `callers-of` returns func-decls instead of call-graph edge pairs
- `checked-before-use` follows comparison data flow (transitive def-use BFS)
- `checked-before-use` returns `'missing` on SSA lookup failure
- Same-block ordering resolved via instruction position

## v0.5.1 — SSA Equivalence and Unification Libraries

- `go-ssa-canonicalize` — dominator-order blocks, alpha-renamed registers
- `(wile goast ssa-normalize)` — algebraic normalization via `(wile algebra rewrite)`
- `(wile goast unify)` — pluggable AST/SSA diff engine with substitution collapsing
- `(wile goast utils)` — `ast-transform`, `ast-splice` tree rewriters

## v0.5.0 — Shared Sessions and MCP Server

- `go-load` creates reusable GoSession; all package-loading primitives accept it
- `wile-goast --mcp` starts stdio MCP server with `eval` tool and 3 prompts
- `(wile goast belief)` — declarative consistency deviation detection DSL
- `(wile goast dataflow)` — def-use reachability via product lattice fixpoint

## v0.3.x — Core Layers

- AST parsing, formatting, type-checking (`go-parse-file`, `go-format`, `go-typecheck-package`)
- SSA construction and field index (`go-ssa-build`, `go-ssa-field-index`)
- Call graph construction (`go-callgraph`, callers/callees/reachable)
- CFG and dominators (`go-cfg`, `go-cfg-dominators`, `go-cfg-paths`)
- Lint framework (`go-analyze`, ~25 built-in analyzers)
