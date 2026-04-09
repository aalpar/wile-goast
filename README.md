# wile-goast

Cross-layer static analysis of Go source code, scripted in Scheme. Exposes
Go's compiler toolchain — AST, SSA, call graph, control flow graph, and
lint diagnostics — as composable Scheme primitives.

Built on [Wile](https://github.com/aalpar/wile), an R7RS Scheme interpreter.

## Installation

```bash
go install github.com/aalpar/wile-goast/cmd/wile-goast@latest
```

The binary is self-contained — all Scheme libraries and built-in scripts are
embedded.

## Quick Start

```bash
# Evaluate a Scheme expression
wile-goast '(go-parse-expr "1 + 2")'

# Run a built-in script
wile-goast --run goast-query

# List available scripts
wile-goast --list-scripts

# Run a script file
wile-goast -f my-analysis.scm
```

## Seven Layers

| Library | Import | What it answers |
|---------|--------|-----------------|
| AST | `(wile goast)` | What is the shape of this code? |
| SSA | `(wile goast ssa)` | Where does this value flow? |
| Call Graph | `(wile goast callgraph)` | Who calls whom? |
| CFG | `(wile goast cfg)` | Must this check happen before that return? |
| Lint | `(wile goast lint)` | What do standard analyzers report? |
| Belief DSL | `(wile goast belief)` | What implicit conventions are being violated? |
| FCA | `(wile goast fca)` | Are these the right boundaries? |

All layers share one node format — tagged alists `(tag (key . val) ...)` —
queryable with standard Scheme list operations.

## Primitives

### AST — `(wile goast)`

| Primitive | Description |
|-----------|-------------|
| `(go-parse-file path . options)` | Parse a `.go` file to s-expression AST |
| `(go-parse-string source . options)` | Parse Go source string |
| `(go-parse-expr source)` | Parse a single Go expression |
| `(go-format ast)` | Convert s-expression AST back to Go source |
| `(go-node-type ast)` | Return the tag symbol of an AST node |
| `(go-typecheck-package pattern . options)` | Load and type-check a package |
| `(go-interface-implementors name target)` | Find types implementing an interface |
| `(go-load pattern ... . options)` | Load packages into a reusable session |
| `(go-session? v)` | Type predicate for GoSession |
| `(go-list-deps pattern ...)` | Transitive import path discovery |
| `(go-cfg-to-structured block)` | Restructure early returns into if/else tree |

Options: `'positions` (include source positions), `'comments` (include doc comments).

### SSA — `(wile goast ssa)`

| Primitive | Description |
|-----------|-------------|
| `(go-ssa-build pattern . options)` | Build SSA; returns list of `ssa-func` nodes |
| `(go-ssa-field-index pattern)` | Pre-correlated per-function field access index |
| `(go-ssa-canonicalize ssa-func)` | Canonicalize blocks and registers for structural comparison |

### Call Graph — `(wile goast callgraph)`

| Primitive | Description |
|-----------|-------------|
| `(go-callgraph pattern algorithm)` | Build call graph (`'static`, `'cha`, `'rta`, `'vta`) |
| `(go-callgraph-callers graph func-name)` | Direct callers of a function |
| `(go-callgraph-callees graph func-name)` | Direct callees of a function |
| `(go-callgraph-reachable graph root-name)` | Transitive closure from a root |

### CFG — `(wile goast cfg)`

| Primitive | Description |
|-----------|-------------|
| `(go-cfg pattern func-name . options)` | Build CFG for a named function |
| `(go-cfg-dominators cfg)` | Build dominator tree |
| `(go-cfg-dominates? dom-tree a b)` | Does block `a` dominate block `b`? |
| `(go-cfg-paths cfg from to)` | Enumerate simple paths between blocks |

### Lint — `(wile goast lint)`

| Primitive | Description |
|-----------|-------------|
| `(go-analyze pattern name ...)` | Run named analyzers on a package |
| `(go-analyze-list)` | List available analyzer names (~25 built-in) |

### Belief DSL — `(wile goast belief)`

```scheme
(import (wile goast belief))

(define-belief "lock-unlock-pairing"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.90 5))

(run-beliefs "./...")
```

Site selectors: `functions-matching`, `callers-of`, `methods-of`, `sites-from`,
`implementors-of`, `interface-methods`, `all-func-decls`

Predicates: `has-params`, `has-receiver`, `name-matches`, `contains-call`,
`stores-to-fields`, `all-of`/`any-of`/`none-of`

Property checkers: `contains-call`, `paired-with`, `ordered`, `co-mutated`,
`checked-before-use`, `custom`

See [`docs/PRIMITIVES.md`](docs/PRIMITIVES.md) for the complete reference.

## Examples

### Parse and query Go source

```scheme
(define file (go-parse-string
  "package demo
   func Add(a, b int) int { return a + b }
   func helper() {}"))

(define names
  (filter-map
    (lambda (decl)
      (and (eq? (car decl) 'func-decl)
           (cdr (assoc 'name (cdr decl)))))
    (cdr (assoc 'decls (cdr file)))))

names ; => ("Add" "helper")
```

### Build a call graph

```scheme
(import (wile goast callgraph))

(define cg (go-callgraph "." 'cha))
(go-callgraph-callers cg "(*Server).Handle")
(go-callgraph-reachable cg "command-line-arguments.main")
```

### Check control flow dominance

```scheme
(import (wile goast cfg))

(define cfg (go-cfg "." "ProcessRequest"))
(define dom (go-cfg-dominators cfg))
(go-cfg-dominates? dom 0 3)  ; does entry dominate block 3?
```

### Run lint analyzers

```scheme
(import (wile goast lint))

(define diags (go-analyze "./..." "nilness" "shadow"))
```

### Module-wide unification detection

The built-in `unify-detect-pkg` script scans an entire Go module for
function pairs that are candidates for unification — functions with the
same structure differing only in types, identifiers, or literals:

```bash
cd /path/to/go/module
wile-goast --run unify-detect-pkg
```

Uses recursive AST diff with substitution collapsing to find minimal root
type substitutions that explain all derived differences.

See [`docs/EXAMPLES.md`](docs/EXAMPLES.md) for annotated walkthroughs.

### False boundary detection

Discover struct boundaries that prevent simplification. FCA builds a concept
lattice from field access patterns and compares against actual type boundaries:

```scheme
(import (wile goast fca))

(let* ((s   (go-load "my/pkg/..."))
       (idx (go-ssa-field-index s))
       (ctx (field-index->context idx 'write-only 'cross-type-only))
       (lat (concept-lattice ctx))
       (xb  (cross-boundary-concepts lat 'min-extent 3)))
  (boundary-report xb))
```

Returns structured evidence: which struct types are coupled, which fields,
and which functions treat them as a unit. The user decides whether to
colocate, extract a new type, or leave as-is.

## MCP Server

`wile-goast --mcp` starts a stdio MCP server (JSON-RPC). One persistent Wile
engine serves all tool calls within the session.

**Tool:** `eval` — takes a `code` string (Scheme expression), returns the result.

**Prompts:** `goast-analyze`, `goast-beliefs`, `goast-refactor` — guided workflows
for structural analysis, belief DSL, and unification detection.

```json
{"mcpServers": {"wile-goast": {"command": "wile-goast", "args": ["--mcp"]}}}
```

## Shared Sessions

`go-load` creates a GoSession holding loaded packages with lazy SSA/callgraph.
All package-loading primitives accept either a pattern string or a GoSession:

```scheme
(define s (go-load "my/pkg/a" "my/pkg/b"))
(go-typecheck-package s)   ;; reuses loaded state
(go-ssa-build s)           ;; same packages, no reload
(go-cfg s "MyFunc")        ;; same SSA program
```

## As a Go Library

```go
engine, err := wile.NewEngine(ctx,
    wile.WithSafeExtensions(),
    wile.WithExtension(goast.Extension),
    wile.WithExtension(goastssa.Extension),
    wile.WithExtension(goastcfg.Extension),
    wile.WithExtension(goastcg.Extension),
    wile.WithExtension(goastlint.Extension),
)
defer engine.Close()

val, err := engine.Eval(ctx, `(go-parse-expr "1 + 2")`)
```

## Build & Test

```bash
make build       # Build to ./dist/{os}/{arch}/wile-goast
make test        # Run all tests
make lint        # Run golangci-lint
make ci          # Full CI: lint + build + test + covercheck + verify-mod
```

## Dependencies

| Dependency | Purpose |
|-----------|---------|
| [`github.com/aalpar/wile`](https://github.com/aalpar/wile) | R7RS Scheme interpreter, extension API |
| `golang.org/x/tools` | `go/ssa`, `go/callgraph`, `go/cfg`, `go/analysis` |
| `mark3labs/mcp-go` | MCP server (JSON-RPC stdio) |

## Documentation

| Document | Purpose |
|----------|---------|
| [`docs/PRIMITIVES.md`](docs/PRIMITIVES.md) | Complete primitive reference for all layers |
| [`docs/AST-NODES.md`](docs/AST-NODES.md) | Field reference for all 50+ AST node tags |
| [`docs/EXAMPLES.md`](docs/EXAMPLES.md) | Annotated walkthroughs of example scripts |
| [`docs/GO-STATIC-ANALYSIS.md`](docs/GO-STATIC-ANALYSIS.md) | Usage guide with cross-layer examples |

## Version

v0.5.5 — see [`CHANGELOG.md`](CHANGELOG.md) for release history.
Zero external consumers. API may change without notice.
