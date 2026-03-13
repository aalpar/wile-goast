# CLAUDE.md

Go static analysis extensions for [Wile](https://github.com/aalpar/wile). Scriptable Go code analysis via Scheme s-expressions.

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

### goastssa — `(wile goast ssa)`
| Primitive | Description |
|-----------|-------------|
| `go-ssa-build` | Build SSA for a Go package |

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

## AST Representation

Go AST nodes map to tagged alists: `(tag (key . val) ...)`. Field access via `(assoc key (cdr node))`. The mapper is bidirectional — `go-parse-string` produces s-expressions, `go-format` converts them back to Go source.

## Build & Test

```bash
make build       # Build cmd/wile-goast to ./dist/{os}/{arch}/
make test        # Run all tests
make lint        # Run golangci-lint
make ci          # Full CI: lint + build + test + covercheck + verify-mod
```

## Project Conventions

- **Commits:** No Co-Authored-By lines. Branch + PR workflow.
- **Dependencies:** `github.com/aalpar/wile` + `golang.org/x/tools`. Prefer standard library otherwise.
- **Version:** v0.0.1 (see `VERSION`). Zero consumers — break freely.
- **Coverage:** 80% threshold enforced by `tools/sh/covercheck.sh`. `cmd/wile-goast` excluded.
- **Error handling:** Follow wile's sentinel + wrap pattern (`werr.WrapForeignErrorf`).

## Key Files

| File | Purpose |
|------|---------|
| `goast/mapper.go` | Go AST to s-expression conversion |
| `goast/unmapper.go` | S-expression to Go AST conversion (dispatch) |
| `goast/unmapper_{decl,stmt,expr,types,comments}.go` | Unmapper by AST category |
| `goast/helpers.go` | Shared utilities (list building, field extraction) |
| `goast/prim_goast.go` | Primitive implementations |
| `goast/register.go` | Extension registration |
| `goast{ssa,cfg,cg,lint}/mapper.go` | IR-specific s-expression mappers |
| `goast{ssa,cfg,cg,lint}/register.go` | Sub-extension registration |

## Documentation

| File | Purpose |
|------|---------|
| [`docs/GO-STATIC-ANALYSIS.md`](docs/GO-STATIC-ANALYSIS.md) | Full guide to multi-layer Go analysis |
| [`plans/GO-AST.md`](plans/GO-AST.md) | AST extension design and phases |
| [`plans/GO-STATIC-ANALYSIS.md`](plans/GO-STATIC-ANALYSIS.md) | SSA/callgraph/CFG/lint umbrella design |
| [`plans/UNIFICATION-DETECTION.md`](plans/UNIFICATION-DETECTION.md) | Procedure unification detection |
