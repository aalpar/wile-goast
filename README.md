# wile-goast

Cross-layer static analysis of Go source code, scripted in Scheme. Designed for AI agents that need to reason about Go codebases at the level of ASTs, SSA, control flow graphs, call graphs, and lint diagnostics — from a single script.

The thesis behind these tools is **software maintenance through simplification**. Less code means fewer bugs, less surface area for defects, and lower maintenance cost. Each analysis layer detects a different kind of redundancy — structural clones, algebraic equivalences, isomorphic control flow, shared dependency signatures, known anti-patterns. The tools surface unification candidates only when merging actually simplifies; adding parameters that increase indirection without reducing complexity is not simplification.

## The Problem

Go has mature analysis infrastructure — `go/ast`, `go/types`, `go/ssa`, `go/callgraph`, `go/cfg`, `go/analysis` — but using it requires writing hundreds of lines of Go per analyzer. Each tool in the ecosystem addresses one slice:

| Tool | Strength | What it can't do |
|------|----------|-----------------|
| `golangci-lint` | 40+ fixed analyzers, CI integration | Compose ad-hoc queries; no cross-layer analysis |
| `gopls` | IDE-level incremental analysis | Single-query lookups, not scriptable |
| Semgrep | Syntactic pattern matching | No SSA, no CFG, no call graph |
| CodeQL | Rich query language, data flow | Proprietary, database build step, separate QL language |
| `go/analysis` | Full access to Go's compiler IRs | Requires Go, hundreds of lines of boilerplate per analyzer |

None of them let you ask: "Which struct fields are mutated independently (SSA), checked in cascading conditionals (AST), and accessed in a fixed dominance order (CFG)?" — a question that spans three IR layers.

wile-goast exposes all five layers through a uniform s-expression interface. One vocabulary — `walk`, `assoc`, `filter-map` — works across every layer. A 400-line Scheme script replaces what would be a multi-thousand-line custom Go analyzer.

## Why Scheme

S-expressions are the natural representation for tree-structured compiler data. Go AST nodes map directly to tagged alists — `(func-decl (name . "Add") (type . ...))` — with no serialization gap, no schema files, no query language to learn.

For AI agents, this matters:

- **LLMs generate valid Scheme more reliably than Go analysis passes.** An s-expression script is a flat sequence of definitions and queries. A Go analyzer requires package setup, type switches, `go/analysis` registration, and careful pointer handling.
- **Uniform format across all IR layers.** AST nodes, SSA instructions, CFG blocks, call graph edges, and lint diagnostics all use the same `(tag (key . val) ...)` encoding. An agent that can traverse one layer can traverse all five.
- **Scripts are self-contained and composable.** No build step, no dependency graph, no module initialization. A script loads a package and queries it.

For human developers, the examples are readable. The language is a natural fit for tree-structured data regardless of who writes it — and reading these scripts is often the fastest way to understand what a Go analysis *does*.

## Five Analysis Layers

| Package | R7RS Library | Primitives | What it answers |
|---------|-------------|------------|-----------------|
| `goast` | `(wile goast)` | `go-parse-file`, `go-parse-string`, `go-parse-expr`, `go-format`, `go-node-type`, `go-typecheck-package` | What is the shape of this code? (syntax, structure, types) |
| `goastssa` | `(wile goast ssa)` | `go-ssa-build` | Where does this value flow? Are mutations coordinated? (data flow) |
| `goastcfg` | `(wile goast cfg)` | `go-cfg`, `go-cfg-dominators`, `go-cfg-dominates?`, `go-cfg-paths` | Must this check happen before that return? (control flow) |
| `goastcg` | `(wile goast callgraph)` | `go-callgraph`, `go-callgraph-callers`, `go-callgraph-callees`, `go-callgraph-reachable` | Who calls whom? What's reachable? (inter-procedural) |
| `goastlint` | `(wile goast lint)` | `go-analyze`, `go-analyze-list` | What do existing analyzers report? (~40 built-in) |

See [`docs/PRIMITIVES.md`](docs/PRIMITIVES.md) for the complete reference.

## Belief DSL — `(wile goast belief)`

A declarative DSL for Engler-style consistency deviation detection. Instead of writing 70+ line scripts per belief, define beliefs in 3-5 lines:

```scheme
(import (wile goast belief))

(define-belief "lock-unlock-pairing"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.90 5))

(define-belief "stepping-mode-frame"
  (sites (functions-matching
           (stores-to-fields "Debugger" "stepMode" "stepFrame")))
  (expect (co-mutated "stepMode" "stepFrame"))
  (threshold 0.66 3))

(run-beliefs "my/package/...")
```

The DSL provides site selectors (`functions-matching`, `callers-of`, `methods-of`, `sites-from`), composable predicates (`has-params`, `has-receiver`, `name-matches`, `contains-call`, `stores-to-fields`, `all-of`/`any-of`/`none-of`), and property checkers (`paired-with`, `ordered`, `co-mutated`, `checked-before-use`, `custom`). The runner loads analysis layers lazily and reports deviations from statistically strong beliefs.

Validated against known results: the stepping-field co-mutation beliefs correctly identify `StepOver` (missing `stepFrame`) and `StepOut` (missing `stepFrameDepth`) as deviations from the `wile/machine` Debugger convention.

See [`plans/BELIEF-DSL.md`](plans/BELIEF-DSL.md) for the design and [`examples/goast-query/belief-comutation.scm`](examples/goast-query/belief-comutation.scm) for the validation script.

## Complex Examples

The `examples/goast-query/` directory contains analysis scripts that demonstrate what wile-goast enables. These are the kind of scripts an AI agent composes given access to the primitive reference.

### Cross-Layer Split-State Detection

[`state-trace-full.scm`](examples/goast-query/state-trace-full.scm) — 400 lines, 4 analysis layers

Detects **split state**: conceptually atomic values scattered across multiple struct fields, checked piecewise in distributed conditionals. No single existing Go tool can perform this analysis.

| Pass | Layer | Question |
|------|-------|----------|
| 1 | AST | Which structs have 2+ boolean fields? (enum candidates) |
| 2 | AST | Which if-chains check multiple fields of the same receiver? |
| 3 | SSA | Are those boolean fields mutated independently across functions? |
| 4 | CFG | Do reads of one field always dominate reads of the other? |

Sample output (run against `github.com/aalpar/wile/machine`):

```
-- Pass 3: Mutation Independence (SSA) --
  struct NativeTemplate:
    NewForeignClosure stores only: (isVariadic)
    computeNoCopyApply stores only: (noCopyApply)

-- Pass 4: Check Ordering (SSA + CFG) --
  struct NativeTemplate:
    func Copy:
      isVariadic [block 4] -> noCopyApply [block 4]: same-block
```

Pass 3 proves the fields are mutated independently. Pass 4 proves they're always accessed together in the same basic block — evidence of a discriminated union encoded as separate booleans.

### Module-Wide Unification Detection

[`unify-detect-pkg.scm`](examples/goast-query/unify-detect-pkg.scm) — 560 lines, recursive AST diff engine

Scans an entire Go module for function pairs that are candidates for unification — functions with the same structure that differ only in type names, identifiers, or literal values. Uses type-checked ASTs for accurate classification.

Key techniques:
- **Recursive AST diff** with category-aware classification (structural, type-name, identifier, literal, operator)
- **Substitution collapsing**: type annotations propagate root substitutions into every sub-expression; the engine finds minimal root substitutions that explain all derived diffs
- **Weighted scoring**: structural differences cost 100x, operator changes cost 2x, identifier renames cost 0 (free parameter)

Validated on the [crdt](https://github.com/aalpar/crdt) project (17 packages, 132 functions): found the ewflag/dwflag duality and pncounter/gcounter pattern — both confirmed as real unification candidates.

### More Examples

| Script | Layers | What it demonstrates |
|--------|--------|---------------------|
| [`goast-query.scm`](examples/goast-query/goast-query.scm) | AST | Parse source, extract function names, find error-returning functions |
| [`state-trace-detect.scm`](examples/goast-query/state-trace-detect.scm) | AST | 2-pass boolean cluster + if-chain detection |
| [`unify-detect.scm`](examples/goast-query/unify-detect.scm) | AST | Prototype diff engine comparing two inline Go functions |
| [`belief-comutation.scm`](examples/goast-query/belief-comutation.scm) | AST+SSA | Co-mutation beliefs via DSL, validated against known results |
| [`belief-example.scm`](examples/goast-query/belief-example.scm) | AST | Belief DSL smoke test against wile-goast itself |

See [`docs/EXAMPLES.md`](docs/EXAMPLES.md) for annotated walkthroughs of each script.

## Usage

### As a Go library

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
        wile.WithLibraryPaths(),  // enables (import ...) for Scheme libraries
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

### As a standalone binary

```bash
make build
./dist/wile-goast '(display (go-parse-string "package p\nfunc F() {}"))'
./dist/wile-goast -f examples/goast-query/state-trace-full.scm
```

## Build & Test

```bash
make build       # Build to ./dist/{os}/{arch}/wile-goast
make test        # Run all tests
make lint        # Run golangci-lint
make ci          # Full CI: lint + build + test + covercheck + verify-mod
make cover       # Coverage report
make covercheck  # Enforce 80% coverage threshold
```

## Dependencies

| Dependency | Purpose |
|-----------|---------|
| `github.com/aalpar/wile` | Scheme engine, extension API, value types |
| `golang.org/x/tools` | `go/ssa`, `go/callgraph`, `go/cfg`, `go/analysis` |

No other runtime dependencies.

## Documentation

| Document | Purpose |
|----------|---------|
| [`docs/PRIMITIVES.md`](docs/PRIMITIVES.md) | Complete primitive reference for all 5 layers |
| [`docs/EXAMPLES.md`](docs/EXAMPLES.md) | Annotated walkthroughs of example scripts |
| [`docs/GO-STATIC-ANALYSIS.md`](docs/GO-STATIC-ANALYSIS.md) | Full guide to multi-layer Go analysis with Scheme |
| [`BIBLIOGRAPHY.md`](BIBLIOGRAPHY.md) | Static analysis references |
| [`plans/`](plans/) | Design documents and implementation plans |

## Version

v0.1.0 — all five analysis layers complete. Zero external consumers. API may change without notice.
