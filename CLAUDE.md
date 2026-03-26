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
| `go-cfg-to-structured` | Restructure early-return block into single-exit if/else tree |

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
| `go-callgraph-reachable` | Transitive reachability from root |

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

## SSA Normalization — `(wile goast ssa-normalize)`

Algebraic normalization rules for SSA binop nodes. Rules are generated from axiom declarations via `(wile algebra rewrite)` — identity, absorbing, and commutativity properties are declared per-operator, and `make-normalizer` compiles them into rewrite rules through a term protocol that abstracts over SSA node structure. Integer-type scoped for identity/absorbing to avoid IEEE 754 issues. Extensible via `ssa-rule-set`.

| Export | Description |
|--------|-------------|
| `ssa-normalize` | Apply default rules to a node (case-lambda: 1 or 2 args) |
| `ssa-rule-commutative` | Sort operands lexicographically for commutative ops |
| `ssa-rule-identity` | `x + 0 → x`, `x * 1 → x`, etc. (integer types only) |
| `ssa-rule-annihilation` | `x * 0 → 0`, `x & 0 → 0` (integer types only) |
| `ssa-rule-set` | Compose rules: first non-`#f` wins |

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

## Belief DSL — `(wile goast belief)`

Declarative consistency deviation detection. Beliefs are patterns extracted statistically from code (Engler et al., "Bugs as Deviant Behavior"). The DSL lets you define beliefs in 3-5 lines instead of writing 70+ line scripts per belief.

```scheme
(import (wile goast belief))

(define-belief "lock-unlock-pairing"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.90 5))

(run-beliefs "my/package/...")
```

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
| `(ordered "A" "B")` | SSA | `'a-dominates-b` / `'b-dominates-a` / `'same-block` / `'unordered` |
| `(co-mutated "field" ...)` | SSA | `'co-mutated` / `'partial` |
| `(checked-before-use "val")` | SSA+CFG | `'guarded` / `'unguarded` |
| `(custom (lambda (site ctx) ...))` | any | user-defined symbol |

### Key Files

| File | Purpose |
|------|---------|
| `cmd/wile-goast/lib/wile/goast/belief.sld` | R7RS library definition (embedded in binary) |
| `cmd/wile-goast/lib/wile/goast/belief.scm` | Complete DSL implementation |
| `cmd/wile-goast/lib/wile/goast/utils.sld` + `utils.scm` | Shared traversal utilities (`nf`, `walk`, `tag?`, etc.) |
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
- **Version:** v0.5.0 (see `VERSION`). Zero consumers — break freely.
- **Coverage:** 80% threshold enforced by `tools/sh/covercheck.sh`. `cmd/wile-goast` excluded.
- **Error handling:** Follow wile's sentinel + wrap pattern (`werr.WrapForeignErrorf`).

## MCP Server

`wile-goast --mcp` starts a stdio MCP server (JSON-RPC). One persistent Wile engine serves all tool calls within the session.

### Tool

Single tool: `eval` — takes a `code` string (Scheme expression), returns the evaluation result.

### Prompts

| Prompt | Description |
|--------|-------------|
| `goast-analyze` | Structural analysis — layer selection, primitive reference, examples |
| `goast-beliefs` | Belief DSL — define and run consistency beliefs |
| `goast-refactor` | Unification detection — find duplicates, verify refactoring |

Prompt content lives in `cmd/wile-goast/prompts/*.md` (embedded in binary).

### Client Config

```json
{"mcpServers": {"wile-goast": {"command": "wile-goast", "args": ["--mcp"]}}}
```

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
| `cmd/wile-goast/lib/wile/goast/belief.scm` | Belief DSL implementation (embedded in binary) |
| `cmd/wile-goast/lib/wile/goast/dataflow.scm` | Def-use reachability via `(wile algebra)` fixpoint (embedded in binary) |
| `cmd/wile-goast/lib/wile/goast/utils.scm` | Shared traversal utilities (`nf`, `walk`, `tag?`) and tree rewriters (`ast-transform`, `ast-splice`) |
| `cmd/wile-goast/lib/wile/goast/ssa-normalize.scm` | SSA algebraic normalization rules (embedded in binary) |
| `cmd/wile-goast/lib/wile/goast/unify.scm` | AST/SSA diff engine with pluggable classifiers (embedded in binary) |
| `goast/prim_restructure.go` | Early-return restructuring (`go-cfg-to-structured`) |
| `goastssa/prim_canonicalize.go` | SSA function canonicalization (`go-ssa-canonicalize`) |

## Documentation

| Document | Purpose |
|----------|---------|
| [`README.md`](README.md) | Project overview, motivation |
| [`docs/PRIMITIVES.md`](docs/PRIMITIVES.md) | Complete primitive reference for all layers + belief DSL |
| [`docs/AST-NODES.md`](docs/AST-NODES.md) | AST node field reference (types, optionality for all tags) |
| [`docs/EXAMPLES.md`](docs/EXAMPLES.md) | Annotated walkthroughs of example scripts |
| [`docs/GO-STATIC-ANALYSIS.md`](docs/GO-STATIC-ANALYSIS.md) | Usage guide with cross-layer examples |
| [`BIBLIOGRAPHY.md`](BIBLIOGRAPHY.md) | Static analysis references |
| [`plans/UNIFICATION-DETECTION.md`](plans/UNIFICATION-DETECTION.md) | Remaining: SSA equivalence v2 pass |
| [`plans/CONSISTENCY-DEVIATION.md`](plans/CONSISTENCY-DEVIATION.md) | Belief categories 1-4: validation results, bug fixes, known limitations |
| [`plans/BELIEF-DSL.md`](plans/BELIEF-DSL.md) | Remaining: graduation --emit mode, suppression |
