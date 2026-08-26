# Changelog

## v0.5.340 — MCP Pipeline Tools, Belief Suppression, Validated Pareto Directions

Everything accumulated since v0.5.111, in the order it landed (newest first).
The through-line is that the pipeline tools stopped being a thin shell over
`eval` — they gained result-view paging, a `'dir` escape from the host working
directory, and a `threshold` that means what it says — while the ranking
underneath them stopped accepting specs it could not check.

### A misspelled direction key raises instead of inverting an axis

The `wile` dependency moves to **v1.20.0**. The pin had said `v1.19.1` since
July while the code tracked master, so `GOWORK=off go build` had been failing
on `registry.PrimitiveRegistry`, `machine.NewParameter` and
`machine.MutableBase` since early August: workspace mode was the only
configuration that built. Both configurations now build and test green.
Restoring the typecheck also un-masked five lint findings the broken load had
been swallowing — four compound if-inits in `goastssa/mapper.go` and one
`strings.SplitN` that `strings.Cut` says better — so `make lint` is green
again rather than green-because-blind.

The release brings the direction validation. `(wile algebra pareto)` now
checks a direction spec eagerly:
`dominates?` and `pareto-frontier` run `check-directions` against every
candidate's factors and raise on a direction naming a factor that is not
present, on a value that is neither `'up` nor `'down`, and on a spec mixing
bare names with `(name . dir)` pairs. Rankings are unchanged — every key in
`split-directions`, `merge-directions`, `extract-directions`,
`measure-directions` and `cost-directions` is one its candidate actually
emits, and `TestDirectionTablesMatchFrontierFactors` /
`TestDirectionTablesMatchEmittedMeasures` already pin that join.

Two consequences for callers building their own tables, recorded in
`dup-detect.scm` and `docs/LIBRARIES.md` where the old rationale said the
opposite. A near-miss spelling used to fall back to `'up` and silently invert
the axis; it is now an error at the ranking call. (`factor-direction` on its
own still answers `'up` for an absent key — the fallback moved behind the
ranking entry points rather than going away.) And the containment runs the
other way too: a table may name only keys **every** candidate carries, so
`cost-directions` ranks the 5c surface or the append with
`measure-directions`, never the 5b surface alone.

### Every `pareto-frontier` caller states its factor directions

`fca-recommend`'s three frontiers passed bare factor-name lists, which
`pareto-frontier` treats as documentation only and ranks all-up. Reading each
factor off the computation that produces it, all-up is in fact correct for all
eleven, so this changes no ranking:

- **split** (`fca-recommend.scm:137-141`) ranks how badly a function wants
  splitting, so size is evidence of the problem rather than a cost to minimise.
  More `incomparable-count`, more `intent-disjointness`, `no-cross-flow` `#t`
  (and `#f < #t` puts the safe end high), more `pattern-balance`, more
  `stmt-count`.
- **merge** (`:181-184`) measures how much the functions genuinely share, and
  more sharing is a stronger case that they are one operation.
- **extract** (`:238-240`) — `sub-concept-depth` is `up` because `concept-depth`
  (`:216`) counts only the steps where intent size grows, so a deeper broad
  concept is a *more specific* shared sub-operation, not a more generic one.

They are stated anyway, as `split-directions` / `merge-directions` /
`extract-directions`, for two reasons. The direction is not derivable from the
values: `stmt-count` and `dup-detect`'s `type-params` are both plain counts and
point opposite ways. And leaving it to the `'up` default means the first
lower-is-better factor anyone adds is ranked backwards while the frontier still
looks like a frontier — the failure the directions argument exists to remove,
and the one that already shipped once in `cost-directions`.

This also makes the two consumers of `pareto-frontier` consistent: with
`dup-detect` naming its directions, `fca-recommend` naming its own means
"all up" reads as a finding about these factors rather than as a caller that
never asked the question.

`TestDirectionTablesMatchFrontierFactors` pins each table's key set against the
factors its candidate function actually emits, in both directions, so a factor
added without a direction and a direction for a factor that no longer exists
both fail — by name. Verified as a ratchet: each drift was introduced and the
failure names the offending key.

No algebra changed. `(wile algebra pareto)` is untouched; this is only how the
results are evaluated.

### `find_duplicates` selects on similarity, not on tier

`find_duplicates`' `threshold` now means what its name says: a floor on
effective similarity, in the same units each row reports. Every scored pair at
or above it is returned, carrying its own `equiv_tier`. The tier is reported
and tallied; it no longer decides what the caller is allowed to see.

It used to be the filter, and that was wrong in a way no threshold could work
around. `unifiable?` is categorical in its second clause — the effective
similarity must clear `threshold` **and** no remaining diff may fall outside
`{type-name, derived-type, identifier, register}`. The second clause has no
knob. So a pair could sit at 0.94 similarity and still be `divergent`, and
lowering `threshold` never retrieved it: measured on wile's quasi cluster at
its pre-refactor commit, `quasiquoteNeedsRuntime` vs
`quasisyntaxNeedsRuntimeGuarded` scores 0.94 with `unifiable?` `#f` at 0.90,
0.60 *and* 0.10. The tool reported `total_count` 0, which reads exactly like a
clean tree.

The deleted rows were the valuable ones. That pair's four `structural` diffs
were, item for item, three latent bugs a human review found in the same file
hours later: a missing vector case, a dropped improper tail, and an
element-wise dotted tail. A high-similarity divergent pair's structural diffs
are precisely where two implementations of one algorithm disagree.

This restores the contract the library states for itself (`dup-detect.scm`,
`candidate->verdict`): the measure surface is the default, the verdict is
requested, never imposed.

The payload concern the tier gate existed to answer is now answered in the
right currency. Volume is bounded by the floor the caller sets: on the wile
quasi cluster, `threshold` 0.6 keeps 1258 of 5855 scored pairs. Raise
`threshold` to narrow; filter the reported `equiv_tier` to recover the old
view. Every stage touching surviving rows was already a tail loop and
rendering already ran after paging, so the call-depth ceiling `view-sort`
exists to avoid does not re-bind.

The envelope `version` stays 2: the result *shape* is unchanged (same fields
per row), and `version` is documented as bumping on shape changes only. The
row *population* is what moved.

`measure-directions` and `cost-directions` are new exports from
`(wile goast dup-detect)`, naming each rankable measure's direction of
improvement for `(wile algebra pareto)`'s new directions argument. `benefit`
and `similarity` are `up`; `type-params` and `value-params` are `down`.
Hand-negating a lower-is-better axis before ranking is no longer required.
It still ranks correctly on its own, so existing code is not wrong; what it
must not do is negate *and* name a direction for the same axis, which
double-inverts. `equiv-tier` and `locality` are deliberately absent from both
tables: one is a categorical verdict and the other is a nested alist, and
`factor-leq?` orders only booleans and numbers.

### `go-load` gains a `'dir` option

`go-load` accepts `'dir <path>`, resolving its patterns against `<path>`
instead of the host process's working directory:

```scheme
(go-load "./..." 'dir "/path/to/other/module")
```

`LoadPackagesChecked` (`goast/load.go`) gains a `dir string` parameter
threaded through all ten call sites; every caller other than `go-load`
passes `""`, leaving behavior unchanged. When `dir` is set, an additional
`ResourceFile`/`ActionRead` security check gates the directory before load,
and a pre-check with `os.Stat` fails with a clear message when the path
does not exist or is not a directory — this closes the gap where an MCP
server rooted in this repo could not analyze a module outside this
workspace without building the CLI and invoking it from inside the target
checkout.

### Result-View Parameters (find_duplicates, find_false_boundaries)

`find_duplicates` and `find_false_boundaries` gain five shared optional
parameters, uniformly filtered, ranked, paged, and rendered by `apply-view`
(`lib/wile/goast/pipelines.scm`):

- `scope` / `symbol` — glob filters over the row's function(s), pushed down
  before scoring on `find_duplicates`, post-filtered on finished concepts on
  `find_false_boundaries`.
- `detail` — `summary` (default) or `full` row width.
- `limit` / `offset` — paging, defaults 100 and 0.

Both tools move to envelope `version` 2: `detail=summary` is now the
default, so a default call's row shape changed (`find_duplicates` drops the
`measures` sub-object; `find_false_boundaries` drops its three lattice-
relationship lists). `types_count` — `find_false_boundaries`' primary rank
key — is present at both detail levels, so `full` is ordered by a field it
also exposes. The provenance key `candidate_count` is retired in favor of
the shared `total_count` (rows after filtering, before paging), alongside
new `returned_count`, `offset`, `limit`, `detail`, and `truncated` (whether
rows were withheld).

### MCP Pipeline Tools (Phase 1)

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

Phases 2–4 of the tool surface (`filter_concepts`, `explain_function`,
`restructure_block`, `trace_path`) will ship under their own plans.

### Belief Suppression

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

[Unreleased]: https://github.com/aalpar/wile-goast/compare/v0.5.340...HEAD
[0.5.340]: https://github.com/aalpar/wile-goast/compare/v0.5.111...v0.5.340
[0.5.111]: https://github.com/aalpar/wile-goast/compare/v0.5.108...v0.5.111
