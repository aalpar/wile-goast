# Changelog

## Unreleased — MCP Pipeline Tools (Phase 1)

Five pipeline-shaped MCP tools expose already-implemented analyses as
first-class tool calls, complementing the existing `eval` tool. Each
returns a `{version, provenance, result}` JSON envelope (via
`NewToolResultJSON`) and is registered on both the stdio and HTTP
transports.

- `check_beliefs` — run a directory of `.scm` beliefs against a Go
  package; returns the per-belief adherence/deviation report.
- `discover_beliefs` — run discovery beliefs, suppress any matching a
  committed belief, and emit the survivors as Scheme source.
- `recommend_split` — IDF-weighted FCA + min-cut package split
  recommendation with a confidence verdict.
- `recommend_boundaries` — function-level split/merge/extract Pareto
  frontiers from FCA over SSA struct-field access.
- `find_false_boundaries` — FCA concepts spanning multiple struct types,
  annotated with lattice relationships.

Supporting changes:

- `load-beliefs!` (`(wile goast belief)`) — load `.scm` beliefs from a
  directory or file into the *current* belief scope, activating them for
  `run-beliefs`. Distinct from `load-committed-beliefs`, which isolates
  loaded beliefs and returns only a snapshot.
- A Wile→JSON marshaller (`cmd/wile-goast/marshal.go`) bridges Scheme
  alists/lists/scalars to JSON-marshallable Go values.

Phases 2–4 of the tool surface (`filter_concepts`, `find_duplicates`,
`explain_function`, `restructure_block`, `trace_path`) will ship under
their own plans.

## Unreleased — Belief Suppression

Close the belief DSL's discover → review → commit → enforce lifecycle.

- `with-belief-scope` — isolate a thunk from the caller's belief registry
  via `dynamic-wind`. Used by `load-committed-beliefs` and available to
  scripts that want to define beliefs without leaking them.
- `load-committed-beliefs` — accept a directory or single `.scm` file,
  load beliefs into an isolated scope, return a
  `(per-site-snapshot . aggregate-snapshot)` pair. Files that fail to
  load are skipped with a stderr warning.
- `suppress-known` — filter `run-beliefs` output by structural (`equal?`)
  comparison on `sites-expr` / `expect-expr` (per-site) or
  `sites-expr` / `analyze-expr` (aggregate). Names and thresholds are
  ignored during matching.
- `current-beliefs` — accessor procedure returning the live per-site
  registry, symmetric to the existing `aggregate-beliefs`. Bypasses
  Wile's stale-snapshot semantics for identifiers imported from a library.

Engine construction now opts in to `wile.WithSourceOS()` so scripts can
reference arbitrary filesystem paths (e.g., committed belief directories
via absolute paths).

Discovery scripts can compose:

    (define results
      (with-belief-scope
        (lambda ()
          <...discovery beliefs...>
          (run-beliefs "my/pkg/..."))))
    (define committed (load-committed-beliefs "beliefs/"))
    (display (emit-beliefs (suppress-known results committed)))

### Wile v1.16.0 — registry Phase → PhaseSet

Bump `github.com/aalpar/wile` to v1.16.0 to pick up the registry rename
(`registry.Phase{Runtime,Expand,Compile}` → `registry.PhaseSet*` with the
new `PhaseSet` bitset type). All five sub-extension registrations
(`goast`, `goastssa`, `goastcfg`, `goastcg`, `goastlint`) already call
through to `registry.PhaseSetRuntime`; this bump matches the released
wile API. Restores green CI on master, which had been failing since
the rename was committed against the unreleased wile master.

### Wile v1.15.0 + standard string libraries

Bump `github.com/aalpar/wile` to v1.15.0 and retire wile-goast's
hand-rolled string helpers in favor of SRFI-13 and `(wile strings)`.

- `string-contains`, `string-join` — re-exported from `(srfi 13)` via
  `(wile goast utils)`.  Note: SRFI-13 `string-contains` returns the
  match index (or `#f`), not `#t`/`#f`.
- `string-contains?` — new boolean-predicate variant in
  `(wile goast utils)`, also re-exported from `(wile goast belief)`.
  Use this when a strict `#t`/`#f` is required (e.g., test assertions);
  conditional contexts can still use `string-contains` directly.
- `string-suffix?` — re-exported from `(srfi 13)` via
  `(wile goast fca-recommend)`.
- `string-index` (SRFI-13) replaces the local `string-index-of` in
  `(wile goast fca)`.
- `string-replace-all` — sourced from `(wile strings)` in
  `(wile goast unify)` and the two `unify-detect-pkg` scripts.

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

[Unreleased]: https://github.com/aalpar/wile-goast/compare/v0.5.111...HEAD
[0.5.111]: https://github.com/aalpar/wile-goast/compare/v0.5.108...v0.5.111
