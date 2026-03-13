# wile-goast

Go static analysis extensions for [Wile](https://github.com/aalpar/wile) — parse, type-check, and analyze Go source code using Scheme s-expressions.

## Extensions

Five extension packages, each loadable independently via `wile.WithExtension()`:

| Package | R7RS Library | Primitives | Purpose |
|---------|-------------|------------|---------|
| `goast` | `(wile goast)` | `go-parse-file`, `go-parse-string`, `go-parse-expr`, `go-format`, `go-node-type`, `go-typecheck-package` | Parse and format Go source as s-expression ASTs |
| `goastssa` | `(wile goast ssa)` | `go-ssa-build` | SSA intermediate representation |
| `goastcfg` | `(wile goast cfg)` | `go-cfg`, `go-cfg-dominators`, `go-cfg-dominates?`, `go-cfg-paths` | Control flow graph, dominator trees, path enumeration |
| `goastcg` | `(wile goast callgraph)` | `go-callgraph`, `go-callgraph-callers`, `go-callgraph-callees`, `go-callgraph-reachable` | Call graph construction (static, CHA, RTA) |
| `goastlint` | `(wile goast lint)` | `go-analyze`, `go-analyze-list` | `go/analysis` framework (~40 built-in analyzers) |

## Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/aalpar/wile"
    "github.com/aalpar/wile-goast/goast"
    "github.com/aalpar/wile-goast/goastssa"
    "github.com/aalpar/wile-goast/goastcfg"
    "github.com/aalpar/wile-goast/goastcg"
    "github.com/aalpar/wile-goast/goastlint"
)

func main() {
    ctx := context.Background()
    engine, err := wile.NewEngine(ctx,
        wile.WithSafeExtensions(),
        wile.WithExtension(goast.Extension),
        wile.WithExtension(goastssa.Extension),
        wile.WithExtension(goastcfg.Extension),
        wile.WithExtension(goastcg.Extension),
        wile.WithExtension(goastlint.Extension),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer engine.Close()

    val, err := engine.Eval(ctx, `
        (let ((file (go-parse-string "package demo\nfunc Add(a, b int) int { return a + b }")))
          (go-format file))
    `)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(val)
}
```

## Examples

The `examples/goast-query/` directory contains Scheme scripts demonstrating multi-layer analysis:

| Script | Description |
|--------|-------------|
| `goast-query.scm` | Parse Go source, extract function names, separate exported/unexported |
| `state-trace-detect.scm` | Detect bounded state variables split across multiple comparisons |
| `state-trace-full.scm` | Full multi-layer analysis combining AST, SSA, CFG, and callgraph |
| `unify-detect.scm` | Detect function pairs that are candidates for unification |
| `unify-detect-pkg.scm` | Package-level unification detection |

## Build & Test

```bash
make build       # Build cmd/wile-goast to ./dist/{os}/{arch}/
make test        # Run all tests
make lint        # Run golangci-lint
make ci          # Full CI: lint + build + test + covercheck + verify-mod
make cover       # Coverage report
make covercheck  # Enforce 80% coverage threshold
```

## Dependency

Single runtime dependency: `github.com/aalpar/wile` (for registry, values, werr).

Analysis tooling dependency: `golang.org/x/tools` (for `go/analysis`, `go/ssa`, `go/callgraph`, `go/cfg`).

## Documentation

| File | Purpose |
|------|---------|
| [`docs/GO-STATIC-ANALYSIS.md`](docs/GO-STATIC-ANALYSIS.md) | Full guide to Go static analysis with Scheme |
| [`plans/`](plans/) | Design documents and implementation plans |
