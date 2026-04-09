# wile-goast

Questions about Go code that grep, gopls, and golangci-lint cannot answer:

- Does every function that acquires a lock also release it — on all control flow paths, across call boundaries?
- Do these 30 functions follow the same calling convention? Which ones deviate?
- Are these two functions structurally identical except for types?
- Which struct boundaries are contradicted by actual field access patterns?
- Must this nil check happen before that dereference?

wile-goast exposes Go's compiler internals (AST, SSA, call graph, CFG, lint) as composable Scheme primitives. Short scripts query code structure directly.

Built on [Wile](https://github.com/aalpar/wile), an R7RS Scheme interpreter.

## Why Scheme

Go's AST is already a tree. S-expressions are trees. No marshaling, no schema, no custom query grammar — the representation *is* the query language.

The Go expression `x + 1` parses to:

```scheme
(binary-expr (op . +) (x ident (name . "x")) (y lit (kind . INT) (value . "1")))
```

Queryable with `car`, `cdr`, `assoc`, `case`. The same tagged-alist format `(tag (key . val) ...)` is shared across all layers — AST, SSA, CFG, call graph, lint results. Learn one representation, query everything.

The mapping is bidirectional: `go-parse-string` produces s-expressions, `go-format` converts them back to Go source. Transform code by transforming lists.

## Cross-function lock/unlock analysis

Lint tools check lock/unlock pairing within a single function body. But what about when `Lock` is called in one function and `Unlock` is the caller's responsibility? That is cross-function analysis — it requires the call graph.

```scheme
(import (wile goast belief))

;; Which functions that call Lock also pair it with Unlock?
(define-belief "lock-unlock-direct"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.90 5))

;; Deviations: functions that Lock without Unlock.
;; Do their callers handle the Unlock instead?
(define-belief "lock-unlock-callers"
  (sites (sites-from "lock-unlock-direct" 'which 'deviation))
  (expect (contains-call "Unlock"))
  (threshold 0.75 3))

(run-beliefs "my/package/...")
```

The first belief finds direct pairing: of all functions that call `Lock`, which ones also call `Unlock` (via defer, explicit call, or any path)? The threshold says: if at least 90% do, and at least 5 sites exist, report the deviations.

The second belief chains off the first. It takes those deviations — functions that `Lock` without `Unlock` — and checks one level up the call stack. Do their callers handle the `Unlock` instead? The `sites-from` selector with `'which 'deviation` feeds the non-conforming sites from the first belief into the second.

This is not "find Lock calls" (grep does that). It is "find the 10% of Lock callers that break the convention, then trace responsibility up the call chain." The threshold model makes it statistical: conventions are extracted from the codebase itself, and deviations are reported against that baseline.

## False boundary detection

Formal Concept Analysis (Ganter & Wille, 1999) applied to Go struct field access patterns:

```scheme
(import (wile goast fca))

(let* ((s   (go-load "my/pkg/..."))
       (idx (go-ssa-field-index s))
       (ctx (field-index->context idx 'write-only 'cross-type-only))
       (lat (concept-lattice ctx))
       (xb  (cross-boundary-concepts lat 'min-extent 3)))
  (boundary-report xb))
```

The pipeline builds a concept lattice from SSA field-store data, discovers natural field groupings, and compares them against actual struct boundaries. A concept that spans multiple struct types means those types share a field access pattern — functions that write to fields in type A also write to fields in type B, treating them as a unit. Those are candidates for restructuring: colocate the fields, extract a shared type, or confirm the coupling is intentional.

The output is a structured report: which struct types are coupled, which fields form the shared pattern, and which functions participate. No novelty claim — the technique is textbook FCA. The value is that it is a composable primitive in the same toolkit as AST queries, SSA analysis, CFG traversal, and the belief DSL.

See [docs/PRIMITIVES.md](docs/PRIMITIVES.md) for the complete primitive reference.

## Installation

```bash
go install github.com/aalpar/wile-goast/cmd/wile-goast@latest
```

The binary is self-contained — all Scheme libraries and built-in scripts are embedded.

## MCP Server

`wile-goast --mcp` starts a stdio MCP server (JSON-RPC). One persistent Wile engine serves all tool calls within the session.

**Tool:** `eval` — takes a `code` string (Scheme expression), returns the result.

**Prompts:** `goast-analyze`, `goast-beliefs`, `goast-refactor` — guided workflows for structural analysis, belief DSL, and unification detection.

```json
{"mcpServers": {"wile-goast": {"command": "wile-goast", "args": ["--mcp"]}}}
```

## Shared Sessions

`go-load` creates a GoSession holding loaded packages with lazy SSA/callgraph.  All package-loading primitives accept either a pattern string or a GoSession:

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

v0.5.6 — see [`CHANGELOG.md`](CHANGELOG.md) for release history.
