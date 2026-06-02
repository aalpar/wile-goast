# CLAUDE.md

Cross-layer static analysis of Go source code, scripted in Scheme. Designed for AI agents — LLMs write Scheme fluently, and s-expressions are the natural representation for tree-structured compiler data.

Built on [Wile](https://github.com/aalpar/wile), a Scheme (R7RS) interpreter with an extension API.

## Thesis

The five layers exist to serve a single goal: **code simplification through unification**. Less code means fewer bugs, less surface area for defects, and lower maintenance cost. Each layer detects a different kind of redundancy — structural clones (AST), algebraic equivalences (SSA), isomorphic control flow (CFG), shared dependency signatures (call graph), known anti-patterns (lint). The tools surface candidates where unification reduces total complexity; they do not propose merges that merely compress code at the cost of added indirection.

## Project Overview

Five extension packages exposing Go's analysis toolchain (`go/ast`, `go/types`, `golang.org/x/tools/go/ssa`, `go/callgraph`, `go/cfg`, `go/analysis`) as Scheme primitives through the Wile extension API.

## Architecture

```
goast/          Core: parse, format, type-check Go source as s-expression alists
goastssa/       SSA intermediate representation (depends on goast)
goastcfg/       Control flow graph, dominators, path enumeration (depends on goast)
goastcg/        Call graph: static, CHA, RTA (depends on goast)
goastlint/      go/analysis framework, ~40 built-in analyzers (depends on goast)
cmd/wile-goast/ Demo binary composing wile + all goast extensions
examples/       Scheme scripts demonstrating multi-layer analysis
```

### Dependency Graph

```
goastssa ──┐
goastcfg ──┤
goastcg  ──┼── goast ──── wile/{registry,values,werr}
goastlint ─┘
```

All sub-extensions depend on the base `goast` package for shared mapper/helper infrastructure. All five depend on `wile` for the extension registration API.

## Primitives

### goast — `(wile goast)`
| Primitive | Description |
|-----------|-------------|
| `go-parse-file` | Parse Go source file to s-expression AST |
| `go-parse-string` | Parse Go source string to s-expression AST |
| `go-parse-expr` | Parse single Go expression |
| `go-format` | Convert s-expression AST back to Go source |
| `go-node-type` | Return the tag symbol of an AST node |
| `go-typecheck-package` | Load package with type annotations |
| `go-interface-implementors` | Find types implementing an interface |
| `go-load` | Load packages into a GoSession for reuse |
| `go-session?` | Type predicate for GoSession |
| `go-list-deps` | Lightweight transitive dependency discovery |
| `go-func-refs` | Per-function external reference profiles via types.Info.Uses |
| `go-cfg-to-structured` | Restructure block into single-exit form: goto elimination, loop return rewriting, guard folding. Optional func-type for result variable synthesis. |

### goastssa — `(wile goast ssa)`
| Primitive | Description |
|-----------|-------------|
| `go-ssa-build` | Build SSA for a Go package |
| `go-ssa-canonicalize` | Canonicalize SSA function: dominator-order blocks, alpha-renamed registers |

### goastcfg — `(wile goast cfg)`
| Primitive | Description |
|-----------|-------------|
| `go-cfg` | Build CFG for a named function |
| `go-cfg-dominators` | Build dominator tree |
| `go-cfg-dominates?` | Test dominance relation |
| `go-cfg-paths` | Enumerate simple paths between blocks |

### goastcg — `(wile goast callgraph)`
| Primitive | Description |
|-----------|-------------|
| `go-callgraph` | Build call graph (static, CHA, RTA) |
| `go-callgraph-callers` | Incoming edges of a function |
| `go-callgraph-callees` | Outgoing edges of a function |

### goastlint — `(wile goast lint)`
| Primitive | Description |
|-----------|-------------|
| `go-analyze` | Run named analysis passes on a package |
| `go-analyze-list` | List available analyzer names |

See [`docs/PRIMITIVES.md`](docs/PRIMITIVES.md) for complete signatures, options, and examples.

### Session sharing

Package-loading primitives (`go-typecheck-package`, `go-ssa-build`, `go-ssa-field-index`, `go-cfg`, `go-callgraph`, `go-analyze`, `go-interface-implementors`) accept either a pattern string (load fresh) or a GoSession from `go-load` (reuse). A session provides snapshot consistency and avoids redundant `packages.Load` calls.

```scheme
(define s (go-load "my/pkg"))
(go-typecheck-package s)  ;; reuses loaded state
(go-ssa-build s)          ;; same packages, no reload
(go-cfg s "MyFunc")       ;; same SSA program
```

## Dataflow Analysis — `(wile goast dataflow)`

Two facilities: def-use reachability (`defuse-reachable?`) and a general worklist-based dataflow analysis framework (`run-analysis`). Both operate on SSA block graphs from `go-ssa-build`.

| Export | Description |
|--------|-------------|
| `run-analysis` | Worklist-based forward/backward analysis: `(run-analysis direction lattice transfer ssa-fn protocol [initial-state] ['check-monotone])`. Re-exported from `(wile algebra dataflow)`. |
| `analysis-in` | Query in-state at block: `(analysis-in result block-idx)`. Re-exported. |
| `analysis-out` | Query out-state at block: `(analysis-out result block-idx)`. Re-exported. |
| `analysis-states` | Full result alist: `((idx in out) ...)`. Re-exported. |
| `ssa-cfg-protocol` | CFG-protocol adapter bridging SSA-function shape to the generic MFP solver. Pass as `protocol` argument to `run-analysis`. |
| `block-instrs` | Extract instruction list from SSA block |
| `defuse-reachable?` | Bounded def-use chain reachability via product lattice fixpoint |
| `make-reachability-transfer` | Product-lattice transfer closure used by `defuse-reachable?` |
| `boolean-lattice` | `{#f, #t}` lattice (utility) |
| `ssa-all-instrs` | Flatten all instructions from SSA function |
| `ssa-instruction-names` | All named values in SSA function |

## Abstract Domains — `(wile goast domains)`

Pre-built abstract domains that plug into C2's `run-analysis`. Each domain is a factory function that constructs a lattice and transfer function, calls `run-analysis`, and returns the result alist.

| Export | Description |
|--------|-------------|
| `go-concrete-eval` | Evaluate Go SSA integer opcodes on Scheme integers |
| `make-reaching-definitions` | Forward reaching definitions (powerset lattice) |
| `make-liveness` | Backward liveness analysis (powerset lattice) |
| `make-constant-propagation` | Forward constant propagation (flat + map-lattice) |
| `sign-lattice` | Construct the 5-element sign lattice: {bot, neg, zero, pos, top}. Re-exported from `(wile algebra)`. |
| `make-sign-analysis` | Forward sign analysis with transfer tables |
| `make-interval-analysis` | Forward interval analysis with per-block widening. Interval lattice supplied by `(wile algebra interval)`. |

## SSA Normalization — `(wile goast ssa-normalize)`

Algebraic normalization rules for SSA binop nodes. Axioms are declared as `named-axiom` objects via `(wile algebra symbolic)`, compiled into rewrite rules through a term protocol that abstracts over SSA node structure. Integer-type scoped for identity/absorbing to avoid IEEE 754 issues. Extensible via `ssa-rule-set` (flat normalization) or custom theories with `discover-equivalences` (algebraic equivalence).

| Export | Description |
|--------|-------------|
| `ssa-normalize` | Apply default rules to a node (case-lambda: 1 or 2 args) |
| `ssa-rule-commutative` | Sort operands lexicographically for commutative ops |
| `ssa-rule-identity` | `x + 0 → x`, `x * 1 → x`, etc. (integer types only) |
| `ssa-rule-annihilation` | `x * 0 → 0`, `x & 0 → 0` (integer types only) |
| `ssa-rule-idempotence` | `x & x → x`, `x \| x → x` (integer types only) |
| `ssa-rule-absorption` | `x & (x \| y) → x`, `x \| (x & y) → x` (integer types only) |
| `ssa-rule-associativity` | Right-associate chained operations for canonical form |
| `ssa-rule-set` | Compose rules: first non-`#f` wins |
| `ssa-theory` | Named theory for `discover-equivalences` (all SSA axioms) |
| `ssa-binop-protocol` | Term protocol for SSA binop nodes |

## Unification Detection — `(wile goast unify)`

Shared diff/scoring library for AST and SSA structural comparison. Extracted from `unify-detect-pkg.scm` with pluggable classifier design.

| Export | Description |
|--------|-------------|
| `ast-diff` | Diff two AST nodes with path-based classification |
| `ssa-diff` | Diff two SSA nodes with tag-based classification |
| `tree-diff` | Generic diff with custom classifier |
| `score-diffs` | Compute effective similarity with substitution collapsing |
| `unifiable?` | Verdict: `#t` when effective similarity >= threshold and all remaining diffs are type/register |
| `diff-result-similarity` | Extract similarity from diff result |
| `ssa-equivalent?` | Algebraic equivalence via `discover-equivalences`: checks if two SSA nodes share a normal form under any sub-theory |

## Belief DSL — `(wile goast belief)`

Declarative consistency deviation detection. Beliefs are patterns extracted statistically from code (Engler et al., "Bugs as Deviant Behavior"). The DSL lets you define beliefs in 3-5 lines instead of writing 70+ line scripts per belief.

```scheme
(import (wile goast belief))

(define-belief "lock-unlock-pairing"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.90 5))

(run-beliefs "my/package/...")
;; => list of result alists (one per belief)
```

### Return Shape

`run-beliefs` returns a flat list of self-describing alists:

```scheme
;; Per-site belief
((name . "lock-unlock") (type . per-site) (status . strong)
 (pattern . paired-defer) (ratio . 9/10) (total . 10)
 (adherence . ("pkg.Foo" "pkg.Bar" ...))
 (deviations . (("pkg.Baz" . unpaired) ...))
 (findings . (#<finding> ...))   ;; one located finding per site (see below)
 (min-adherence . 0.9) (min-sites . 5)
 (sites-expr . (sites (functions-matching (contains-call "Lock"))))
 (expect-expr . (expect (paired-with "Lock" "Unlock"))))

;; Aggregate belief
((name . "pkg-cohesion") (type . aggregate) (status . ok)
 (sites-expr . (sites (all-functions-in)))
 (analyze-expr . (analyze (single-cluster 'idf-threshold 0.36)))
 (verdict . SPLIT) (confidence . HIGH) ...)
```

Status values: `strong`, `weak`, `no-sites`, `error` (per-site); `ok`, `error` (aggregate).

The `findings` field is *additive* (it sits beside the unchanged `adherence`/`deviations`):
a list of `finding` objects from `(wile goast provenance)`, one per site, each carrying
`value` (the category), `where` (`"file:line:col"` or `#f`), `why` (structured
`(reason-tag . data-alist)`, projected by `render-why`), and `score` (number or `#f`).
The category alone still drives voting; evidence rides alongside it. A finding is only as
located/justified as its checker chose to retain — `ordered` is the first to emit real
evidence (the two call positions); bare-symbol checkers yield unlocated findings.

### Emit Mode

`emit-beliefs` takes `run-beliefs` output and produces Scheme source code — `define-belief` forms for strong per-site beliefs, `define-aggregate-belief` forms for ok aggregates. Closes the discover → review → commit → enforce lifecycle.

```scheme
(define emitted (emit-beliefs (run-beliefs "my/package/...")))
(display emitted)
;; => (define-belief "lock-unlock" ...)
```

| Export | Description |
|--------|-------------|
| `emit-beliefs` | Format strong/ok belief results as Scheme source code |

### Suppression

Close the `discover → review → commit → enforce` lifecycle. Committed
beliefs live in `.scm` files; re-running discovery should not resurface
a belief already committed. Matching is structural (`equal?` on captured
S-expressions). Names, thresholds, and ratios are ignored.

```scheme
(define results
  (with-belief-scope
    (lambda ()
      ;; ...discovery beliefs...
      (run-beliefs "my/pkg/..."))))
(define committed (load-committed-beliefs "beliefs/"))
(display (emit-beliefs (suppress-known results committed)))
```

| Export | Description |
|--------|-------------|
| `with-belief-scope` | Save/restore `*beliefs*` + `*aggregate-beliefs*` around a thunk via `dynamic-wind`. |
| `load-committed-beliefs` | Load `.scm` beliefs from a directory or single file into an isolated scope; return `(per-site-snapshot . aggregate-snapshot)` pair. Per-file `guard`: skip bad files with stderr warning. |
| `load-beliefs!` | Load `.scm` beliefs from a directory or single file into the **current** scope (activating them for `run-beliefs`), returning the count loaded. The load-and-run counterpart to `load-committed-beliefs`'s isolate-and-snapshot. Wrap in `with-belief-scope` to confine. |
| `suppress-known` | Structural filter: drop results whose `sites-expr`/`expect-expr` (per-site) or `sites-expr`/`analyze-expr` (aggregate) match any committed tuple. |
| `current-beliefs` | Live snapshot of `*beliefs*`, symmetric to `aggregate-beliefs`. Necessary because user-code `*beliefs*` reads return a stale snapshot under Wile's import semantics. |

### Belief Definition Form

```scheme
(define-belief <name>
  (sites <selector>)      ;; where to look
  (expect <checker>)       ;; what to verify
  (threshold <ratio> <n>)) ;; when to report

(reset-beliefs!)  ;; clear all defined beliefs
```

### Site Selectors

| Selector | Layer | Description |
|----------|-------|-------------|
| `(functions-matching pred ...)` | AST | Functions matching all predicates |
| `(callers-of "func")` | Call Graph | All callers of a function |
| `(methods-of "Type")` | AST | All methods on a receiver type |
| `(all-functions-in)` | AST | All functions in context's loaded packages |
| `(sites-from "belief" 'which 'adherence)` | — | Bootstrapping from another belief's results |

### Selector Predicates

| Predicate | Description |
|-----------|-------------|
| `(has-params "type" ...)` | Signature contains these param types |
| `(has-receiver "type")` | Method receiver matches |
| `(name-matches "pattern")` | Function name substring match |
| `(contains-call "func" ...)` | Body calls any of these |
| `(stores-to-fields "Struct" "field" ...)` | SSA: stores to these fields |
| `(all-of pred ...)` | All predicates match |
| `(any-of pred ...)` | Any predicate matches |
| `(none-of pred ...)` | No predicate matches |

### Property Checkers

| Checker | Layer | Returns |
|---------|-------|---------|
| `(contains-call "func" ...)` | AST | `'present` / `'absent` |
| `(paired-with "A" "B")` | AST+CFG | `'paired-defer` / `'paired-call` / `'unpaired` |
| `(ordered "A" "B")` | SSA | `'a-dominates-b` / `'b-dominates-a` / `'unordered` (+ evidence tail) |
| `(co-mutated "field" ...)` | SSA | `'co-mutated` / `'partial` |
| `(checked-before-use "val")` | SSA+CFG | `'guarded` / `'unguarded` |
| `(custom (lambda (site ctx) ...))` | any | user-defined symbol |

A checker may return either a bare category symbol or `(symbol . evidence)` where
`evidence = ((where . W) (why . Y) (score . S))`; a bare symbol stays valid (it yields an
unlocated finding). The category alone drives voting; the evidence becomes the per-site
`finding`. All five SSA-aware checkers emit the tail: `ordered` (the two call
positions), `paired-with` (op-a's call site; `unpaired` lands exactly at the
operation needing a pair), `co-mutated` (the first field-store), `checked-before-use`
(the comparison feeding the guard — the `ssa-if` itself carries no position), and
`contains-call` (the matched call on `present`; bare `#f` on absent, preserving its
dual-use as a `functions-matching` predicate). Verdicts with no resolvable position
stay bare symbols (`'unordered`/`'missing`/`'malformed-ssa`, unlocated `unguarded`).

### Aggregate Beliefs

Aggregate beliefs evaluate whole-package properties instead of per-site patterns.

```scheme
(define-aggregate-belief "package-cohesion"
  (sites (all-functions-in))
  (analyze (single-cluster 'idf-threshold 0.36)))
```

| Analyzer | Description |
|----------|-------------|
| `(single-cluster . opts)` | Package cohesion via `recommend-split` |

### Key Files

| File | Purpose |
|------|---------|
| `lib/wile/goast/belief.sld` | R7RS library definition (embedded in binary) |
| `lib/wile/goast/belief.scm` | Complete DSL implementation |
| `lib/wile/goast/utils.sld` + `utils.scm` | Shared traversal utilities (`nf`, `walk`, `tag?`, etc.) |
| `plans/BELIEF-DSL.md` | Design: graduation model, bootstrapping, trade-offs |
| `plans/BELIEF-DSL-IMPL.md` | Implementation plan |

### Cross-Layer Notes

- SSA functions use qualified names (`(*Type).Method`); AST uses short names (`Method`). The SSA index normalizes via `ssa-short-name`.
- `stores-to-fields` disambiguates receivers against the full struct field set (from `struct-field-names`), not just the target fields.
- `co-mutated` skips receiver disambiguation — `stores-to-fields` already filtered.
- Analysis layers load lazily: AST always, SSA/callgraph only when a belief needs them.
- Categories 1-4 validated against synthetic testdata (`examples/goast-query/testdata/`).
  Three bugs fixed during validation: `ordered` (moved from CFG to SSA), `callers-of`
  (returns func-decls), `checked-before-use` (follows comparison data flow).

## AST Representation

Go AST nodes map to tagged alists: `(tag (key . val) ...)`. Field access via `(assoc key (cdr node))` or `(nf node 'key)` from `(wile goast utils)`. The mapper is bidirectional — `go-parse-string` produces s-expressions, `go-format` converts them back to Go source. All five layers share this format.

**Field types vary by node tag** — see [`docs/AST-NODES.md`](docs/AST-NODES.md) for the complete field reference (types, optionality, and descriptions for all 50+ tags).

## Build & Test

```bash
make build       # Build cmd/wile-goast to ./dist/{os}/{arch}/
make test        # Run all tests
make lint        # Run golangci-lint
make ci          # Full CI: lint + build + test + covercheck + verify-mod
```

## Project Conventions

- **Commits:** No Co-Authored-By lines. Direct push to master at this stage.
- **Dependencies:** `github.com/aalpar/wile` + `golang.org/x/tools` + `mark3labs/mcp-go`. Prefer standard library otherwise.
- **Version:** v0.5.189 (see `VERSION`).
- **Coverage:** 80% threshold enforced by `tools/sh/covercheck.sh`. `cmd/wile-goast` excluded.
- **Error handling:** Follow wile's sentinel + wrap pattern (`werr.WrapForeignErrorf`).

## MCP Server

Two transports, same tool + prompts:

- `wile-goast --mcp` — stdio MCP server (JSON-RPC). One session keyed `"stdio"`.
- `wile-goast --http[=ADDR]` — Streamable HTTP MCP server. `--http` alone binds
  `127.0.0.1:8080` (loopback); `--http=:9000` binds all interfaces. Endpoint is
  `/mcp`. Stateful sessions; graceful shutdown on SIGINT/SIGTERM.

**Engine model:** one `*wile.Engine` per MCP session, keyed by `SessionID()`,
built lazily on first `eval` and closed on session unregister (clean DELETE or
30-min idle sweep). This isolates each HTTP client's state (`go-load` sessions,
defined beliefs) and serializes concurrent `eval`s within a session via a
per-engine mutex. stdio is the degenerate one-session case. See
`plans/2026-05-30-http-mcp-server-design.md`.

### Tool

Single tool: `eval` — takes a `code` string (Scheme expression), returns the evaluation result.

### Pipeline Tools (Phase 1)

Five coarse-grained tools wrap already-implemented analyses, returning a
`{version, provenance, result}` JSON envelope (via `NewToolResultJSON`,
both text + `structuredContent`) instead of requiring `eval`-driven
orchestration. Registered in `newServer` (`registerPhase1Tools`), so they
appear on both stdio and HTTP. Implemented in `lib/wile/goast/pipelines.scm`
(library `(wile goast pipelines)`) with handlers in `cmd/wile-goast/mcp_tools.go`.

| Tool | Parameters | Result |
|------|------------|--------|
| `check_beliefs` | `target`, `beliefs_path` | per-belief adherence/deviation list |
| `discover_beliefs` | `target`, `discovery_path`, `committed_path?` | `{emitted_source, filtered_results}` |
| `recommend_split` | `target`, `idf_threshold?`, `refine?`, `max_attributes?` | split proposal + `confidence` |
| `recommend_boundaries` | `target`, `mode?` | `{splits, merges, extracts}` Pareto frontiers |
| `find_false_boundaries` | `target`, `mode?`, `min_extent?`, `min_intent?`, `min_types?` | cross-boundary report |

- Envelope `version` is a per-tool integer (bumped only on breaking
  `result`-shape changes); errors surface via MCP's `isError`, not the
  envelope. JSON keys are snake_case; Scheme alists stay kebab-case
  (converted by `cmd/wile-goast/marshal.go`).
- `mode` is the FCA field-access mode: `write-only` (default),
  `read-write`, or `type-only`.
- Prefer a pipeline tool for its known structural query; use `eval` for
  open-ended exploration. Design: `plans/2026-04-19-mcp-tool-surface-{design,impl}.md`.

### Prompts

| Prompt | Description |
|--------|-------------|
| `goast-analyze` | Structural analysis — layer selection, primitive reference, examples |
| `goast-beliefs` | Belief DSL — define and run consistency beliefs |
| `goast-refactor` | Unification detection — find duplicates, verify refactoring |
| `goast-scheme-ref` | Wile Scheme reference — primitives, idioms, exports, gotchas |
| `goast-split` | Package cohesion analysis and split recommendations |

Prompt content lives in `cmd/wile-goast/prompts/*.md` (embedded in binary).

### Client Config

stdio:

```json
{"mcpServers": {"wile-goast": {"command": "wile-goast", "args": ["--mcp"]}}}
```

Streamable HTTP (server started separately via `wile-goast --http`):

```json
{"mcpServers": {"wile-goast": {"type": "streamable-http", "url": "http://127.0.0.1:8080/mcp"}}}
```

## False Boundary Detection — `(wile goast fca)`

Formal Concept Analysis (Ganter & Wille, 1999) applied to Go struct field access patterns. Discovers natural field groupings from SSA data, then compares against actual struct boundaries. Mismatches are false boundary candidates — boundaries whose removal enables unification or simplifies state.

| Export | Description |
|--------|-------------|
| `make-context` | Build formal context from objects, attributes, incidence function |
| `context-from-alist` | Convenience: context from `((obj attr ...) ...)` entries |
| `context-objects` | Extract object set from context |
| `context-attributes` | Extract attribute set from context |
| `field-index->context` | Convert `go-ssa-field-index` output to formal context (modes: `'write-only`, `'read-write`, `'type-only`) |
| `field-index->positions` | Build a name→source hashtable from a field index. The Go↔source join that keeps `(wile algebra fca)` position-agnostic: the algebra's extent is opaque object names; positions live on the Go side (`ssa-field-summary.pos`) and are re-attached here by name. |
| `intent` | Galois connection: objects → shared attributes |
| `extent` | Galois connection: attributes → objects having all |
| `concept-lattice` | Compute all formal concepts via NextClosure (Ganter 1984) |
| `concept-extent` | Extract extent (object set) from concept |
| `concept-intent` | Extract intent (attribute set) from concept |
| `cross-boundary-concepts` | Filter concepts spanning multiple struct types (opts: `'min-extent`, `'min-intent`, `'min-types`) |
| `boundary-report` | Structured alist report for cross-boundary concepts |
| `boundary-findings` | Finding-shaped sibling of `boundary-report`: each extent member becomes a located `finding` (`value` = qualified func name, `where` = source position via a `field-index->positions` index, `why` = the shared intent as `(cross-boundary (fields . …) (types . …))`, `score` = `#f`). `boundary-report` is left unchanged so the `find_false_boundaries` MCP marshaller is unaffected. |

## FCA Algebraic Annotation — `(wile goast fca-algebra)`

Bridges FCA concept lattices with `(wile algebra lattice)` and `(wile algebra closure)` from wile's algebra library. The Galois closure operator `intent ∘ extent` is formalized via `make-closure-operator` on the attribute powerset lattice. Constructs an algebraic lattice from FCA concepts (with join/meet via closure application), and annotates boundary reports with lattice-theoretic relationships.

| Export | Description |
|--------|-------------|
| `concept-lattice->algebra-lattice` | Construct `(wile algebra lattice)` from FCA context + concepts |
| `concept-relationship` | Classify pair: `subconcept` / `superconcept` / `equal` / `incomparable` |
| `annotated-boundary-report` | Extend boundary report with `subconcept-of`, `superconcept-of`, `incomparable-with` |

## Function Boundary Recommendations — `(wile goast fca-recommend)`

Analyzes FCA concept lattices to produce ranked split/merge/extract recommendations for function boundaries. SSA data flow filtering distinguishes intentional coordination from accidental aggregation. Pareto dominance ranking with separate frontiers per type.

| Export | Description |
|--------|-------------|
| `dominates?` | Pareto dominance: X >= Y on all factors, > on at least one |
| `pareto-frontier` | Compute Pareto frontier and dominated groups |
| `concept-signature` | Map function name to its concepts in the lattice |
| `incomparable-pairs` | Find incomparable concept pairs in a signature |
| `split-candidates` | Functions serving incomparable state clusters |
| `merge-candidates` | Functions maintaining shared state separately |
| `extract-candidates` | Sub-operations shared by more callers than the full op |
| `boundary-recommendations` | Top-level: three Pareto frontiers (split/merge/extract) |
| `string-suffix?` | Test if a string ends with a given suffix |

## Boolean Simplification — `(wile goast boolean-simplify)`

Boolean normalization for Go AST conditions and belief selector predicates. Uses `(wile algebra symbolic)` recursive normalizer with a Boolean algebra theory (absorption, involution, idempotence, commutativity).

| Export | Description |
|--------|-------------|
| `boolean-normalize` | Normalize boolean S-expression; returns `(values normal-form trace)` |
| `boolean-equivalent?` | Check if two terms normalize to the same form |
| `selector->symbolic` | Project belief selector combinators (`all-of`→`and`, `any-of`→`or`, `none-of`→`not`) |
| `ast-condition->symbolic` | Project Go AST conditions (`&&`→`and`, `||`→`or`, `!`→`not`, comparisons→opaque atoms) |

Note: Go's `&&`/`||` become control flow in SSA, so `ast-condition->symbolic` works at the AST level (from `go-parse-expr`/`go-parse-file`), not SSA.

## Package Splitting — `(wile goast split)`

Import signature analysis for Go package decomposition. Discovers natural package boundaries using IDF-weighted FCA on per-function dependency profiles.

| Export | Description |
|--------|-------------|
| `import-signatures` | Extract per-function package dependency sets from `go-func-refs` output |
| `compute-idf` | IDF weights for dependency informativeness |
| `filter-noise` | Remove ubiquitous (low-IDF) dependencies |
| `build-package-context` | FCA context at package granularity |
| `refine-by-api-surface` | FCA context at (package, object) granularity |
| `find-split` | Min-cut two-way partition via concept lattice |
| `verify-acyclic` | Check proposed split for Go import cycles |
| `recommend-split` | Top-level: IDF + FCA + min-cut + cycle check + confidence |

## Deduplication — `(wile goast dup-detect)`

The FCA audit trace for deduplication — the exact twin of `(wile goast fca)`'s
`boundary-findings`, on a `function × external-ref` concept lattice instead of
`function × field`. Functions sharing a maximal informative reference set (an FCA
concept with extent ≥ 2) are duplicate candidates; each extent member becomes a
located `finding` whose `why` is the shared ref intent. Composes the `split.scm`
clustering chain (objects are function names) with `fca` + `provenance`. Default
output is the audit trace; structural scoring (`ast-diff`/`ssa-diff`), the
benefit/equivalence measures, and the opt-in `candidate->verdict` are slice 5b;
the LLM judge is deferred. `go-func-refs` now carries an optional `pos`
(`"file:line:col"`, present when the function position is valid) — the name→source
data this module joins on.

| Export | Description |
|--------|-------------|
| `function-ref-context` | `function × external-ref` FCA context, IDF-filtered (reuses the `split.scm` chain at function granularity) |
| `duplicate-candidate-concepts` | Concepts with extent ≥ 2 and a non-empty intent — by FCA closure, duplicate-candidate clusters |
| `func-refs->positions` | Name→source hashtable from `go-func-refs` output (the `field-index->positions` twin; exact-match keys) |
| `dup-candidate-findings` | Per candidate concept, each extent member → a located `finding`; `why` = `(duplicate-candidate (refs . intent))`, `score` = `#f`. The `boundary-findings` twin |
| `find-duplicate-candidates` | Top-level (5a): `go-func-refs` → IDF-filtered context → concept lattice → candidate concepts → located findings |
| `score-candidate-pair` | (5b) Benefit measures (`benefit`, `type-params`, `value-params`, `similarity`) + `equiv-tier` for a candidate pair, joined to AST/SSA via `short-name`. Tier: `proven` (SSA-canonical `unifiable?`) / `structural` (AST `unifiable?`) / `divergent`. Returns `#f` when names don't resolve |
| `scored-candidates` / `find-scored-candidates` | (5b) Each within-cluster pair → a scored candidate: two located findings (`score` = effective similarity), `why` = `(unify-candidate (peer . other) (measures . M))`. `find-scored-candidates` is the top-level (clusters → scored pairs) |
| `candidate->verdict` | (5b) **Opt-in** projection of a candidate's `equiv-tier` to `duplicate`/`likely-duplicate`/`distinct` — the categorical analog of `finding->scalar`; the measure surface is the default, the verdict is requested, never imposed |
| `cand-new-edges` | (5c) `\|callers(a) ∪ callers(b)\|` — the in-degree the merged function would carry (coupling concentrated; merging retargets edges, doesn't add them) |
| `cand-creates-cycle?` | (5c) `a reaches b ∨ b reaches a` (BFS over `go-callgraph-callees`) — merging a call-path pair collapses it into a self-cycle. Merge-side analog of `verify-acyclic` |
| `cand-locality` | (5c) Ledger fact (never a verdict): `scope` (`same-pkg`/`shared-callers`/`disjoint`) + `dep-overlap` (Jaccard of external ref sets) |
| `build-func-ref-index` | (5c) name → `func-ref` entry, for `cand-locality`'s pkg + ref-set lookup |
| `find-candidates-with-cost` | (5c) Top-level full ledger: 5b scoring + the cost measures, findings embedding the full set |

Cost axes plug into the same `pareto-frontier`; negate lower-is-better axes
(e.g. `new-edges`) since dominance treats higher as better. (Note: there is no
`go-callgraph-reachable` primitive — reachability is computed by BFS over
`go-callgraph-callees`.)

Rank the measure surface with `pareto-frontier`/`dominates?` from
`(wile goast fca-recommend)` — the one documented combinator (no dedup-specific
ranking). Each item is `(id numeric-factors-alist)`; project to numeric measures
(e.g. `benefit`, `similarity`) since dominance ignores no key. The cost-half
measures (`cand-new-edges`, `cand-creates-cycle?`, `cand-locality`) are slice 5c.
`short-name` reconciliation collides only when two methods share a short name
across receiver types (rare; index keeps the last). The LLM judge is deferred.

## Path Algebra — `(wile goast path-algebra)`

Semiring-parameterized path computation over call graphs. Lazy single-source Bellman-Ford with per-source caching. SCC side-query API exposes mutual-recursion clusters; fast-path introspection reports when the bigint-counting kernel is active.

| Export | Description |
|--------|-------------|
| `make-path-analysis` | Construct path analysis from semiring, call graph, and optional edge-weight function |
| `path-analysis?` | Type predicate |
| `path-query` | Query semiring value between source and target (lazy, cached) |
| `path-query-all` | Return distance alist for all reachable nodes from source |
| `path-analysis-sccs` | Force SCC decomposition; returns a `<graph-scc>` record |
| `path-node-in-cycle?` | True iff function lies in a non-trivial SCC (i.e., is recursive or mutually recursive). Raises if name is not in the call graph. |
| `path-cyclic-nodes` | List of function names that lie in non-trivial SCCs |
| `path-analysis-fast-path?` | True iff bigint-counting kernel is attached |
| `path-analysis-fast-path-kind` | Symbol naming the fast-path strategy (`'bigint-counting`) or `#f` |

## Provenance — `(wile goast provenance)`

Resolve SSA instructions to source positions. The SSA mapper injects
`(pos . "file:line:col")` per instruction under `go-ssa-build`'s `'positions`
option; the belief context now builds with it on. These accessors surface that
position so analyses stop discarding the provenance they already compute. It
also defines the auditable *finding* — a value (category or measure) paired with
`where`/`why`/`score` — and its human rendering; `why` is structured
(`(reason-tag . data-alist)`) so downstream Scheme can filter/aggregate on it.
First primitives of the auditable-categorization facility
(`plans/2026-06-01-auditable-categorization-design.md`).

| Export | Description |
|--------|-------------|
| `ssa-instr-pos` | Source position `"file:line:col"` of an SSA instruction node, or `#f` |
| `ssa-call-position` | Position of the first call to a named function in a block, or `#f` |
| `ssa-first-pos` | Position of the first instruction in an SSA function matching a predicate (and carrying a position), or `#f` — lets a checker locate a store/guard it identified |
| `ssa-func-call-position` | Position of the first call to a named function anywhere in an SSA function — the function-level companion to `ssa-call-position` |
| `make-finding` | Construct an auditable finding `(value, where, why, score)` |
| `finding-value` / `finding-where` / `finding-why` / `finding-score` | Finding accessors |
| `render-why` | Project a structured reason `(reason-tag . data-alist)` to a human string |
| `render-finding` | One-line audit string `"where — why [score]"` |
| `render-category` | Editor-walkable report for a category: a `LABEL (N)` header then one indented `render-finding` line per member. Generic over any finding list (belief adherence/deviations, an FCA concept's extent, …). The literal "show me every X and a one-line why." |

## Key Files

| File | Purpose |
|------|---------|
| `cmd/wile-goast/mcp.go` | MCP server: eval tool handler, prompt handlers |
| `cmd/wile-goast/prompts/*.md` | MCP prompt content (embedded) |
| `goast/mapper.go` | Go AST to s-expression conversion |
| `goast/unmapper.go` | S-expression to Go AST conversion (dispatch) |
| `goast/unmapper_{decl,stmt,expr,types,comments}.go` | Unmapper by AST category |
| `goast/helpers.go` | Shared utilities (list building, field extraction) |
| `goast/prim_goast.go` | Primitive implementations |
| `goast/register.go` | Extension registration |
| `goast{ssa,cfg,cg,lint}/mapper.go` | IR-specific s-expression mappers |
| `goast{ssa,cfg,cg,lint}/register.go` | Sub-extension registration |
| `lib/wile/goast/belief.scm` | Belief DSL implementation (embedded in binary) |
| `lib/wile/goast/dataflow.scm` | Def-use reachability + worklist dataflow analysis framework (embedded in binary) |
| `lib/wile/goast/utils.scm` | Shared traversal utilities (`nf`, `walk`, `tag?`) and tree rewriters (`ast-transform`, `ast-splice`) |
| `lib/wile/goast/ssa-normalize.scm` | SSA algebraic normalization rules (embedded in binary) |
| `lib/wile/goast/unify.scm` | AST/SSA diff engine with pluggable classifiers (embedded in binary) |
| `lib/wile/goast/fca.scm` | Formal Concept Analysis: false boundary detection via concept lattices (embedded in binary) |
| `lib/wile/goast/fca-algebra.scm` | FCA algebraic annotation: concept lattice as `(wile algebra lattice)`, relationship classification (embedded in binary) |
| `lib/wile/goast/fca-recommend.scm` | Function boundary recommendations: split/merge/extract via FCA + SSA cross-flow (embedded in binary) |
| `lib/wile/goast/split.scm` | Package splitting analysis: IDF-weighted FCA on import signatures (embedded in binary) |
| `goast/prim_funcrefs.go` | Per-function external reference extraction (`go-func-refs`) |
| `lib/wile/goast/boolean-simplify.scm` | Boolean normalization for Go AST conditions and belief selectors via `(wile algebra symbolic)` (embedded in binary) |
| `lib/wile/goast/path-algebra.scm` | Semiring path algebra: Bellman-Ford over call graphs (embedded in binary) |
| `lib/wile/goast/domains.scm` | Pre-built abstract domains: reaching defs, liveness, constant prop, sign, interval (embedded in binary) |
| `lib/wile/goast/provenance.scm` | Provenance: resolve SSA instructions to source positions (`ssa-instr-pos`, `ssa-call-position`); first primitive of the auditable-finding facility (embedded in binary) |
| `goast/prim_restructure.go` | Block restructuring: goto elimination, loop return rewriting, guard folding (`go-cfg-to-structured`) |
| `goastssa/prim_canonicalize.go` | SSA function canonicalization (`go-ssa-canonicalize`) |

## Documentation

| Document | Purpose |
|----------|---------|
| [`README.md`](README.md) | Project overview, motivation |
| [`docs/CLAUDE.md`](docs/CLAUDE.md) | Documentation conventions: notation, citations, organization |
| [`docs/PRIMITIVES.md`](docs/PRIMITIVES.md) | Complete primitive reference for all layers + belief DSL |
| [`docs/AST-NODES.md`](docs/AST-NODES.md) | AST node field reference (types, optionality for all tags) |
| [`docs/EXAMPLES.md`](docs/EXAMPLES.md) | Annotated walkthroughs of example scripts |
| [`docs/GO-STATIC-ANALYSIS.md`](docs/GO-STATIC-ANALYSIS.md) | Usage guide with cross-layer examples |
| [`BIBLIOGRAPHY.md`](BIBLIOGRAPHY.md) | Static analysis references |
| [`plans/CLAUDE.md`](plans/CLAUDE.md) | Active plan files, naming conventions, pre-work checklist |
| [`plans/UNIFICATION-DETECTION.md`](plans/UNIFICATION-DETECTION.md) | Remaining: SSA equivalence v2 pass |
| [`plans/CONSISTENCY-DEVIATION.md`](plans/CONSISTENCY-DEVIATION.md) | Belief categories 1-4: validation results, bug fixes, known limitations |
| [`plans/BELIEF-DSL.md`](plans/BELIEF-DSL.md) | Remaining: graduation --emit mode, suppression |
