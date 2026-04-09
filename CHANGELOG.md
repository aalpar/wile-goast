# Changelog

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
