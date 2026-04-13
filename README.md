# wile-goast

You have a large Go codebase. Hundreds of functions across dozens of packages.
You suspect there are duplicates, inconsistent conventions, and struct boundaries
that don't match how the code actually works. grep can find string matches. gopls
can find references. golangci-lint can find known anti-patterns. None of them can
answer:

- Are these two functions structurally identical except for types?
- Does every caller of `Step` handle the returned error?
- Does every function that acquires a lock also release it — on every path?
- Which struct boundaries are contradicted by actual field access patterns?
- Do these 30 methods follow the same calling convention? Which ones deviate?

wile-goast is an MCP server that exposes Go's compiler internals — AST, SSA,
control flow graph, call graph, and lint — as analysis primitives. Your AI
assistant writes the queries. You describe what you're looking for.

## Setup

Install the binary:

```bash
go install github.com/aalpar/wile-goast/cmd/wile-goast@latest
```

Add it to your MCP client configuration:

```json
{
  "mcpServers": {
    "wile-goast": {
      "command": "wile-goast",
      "args": ["--mcp"]
    }
  }
}
```

The binary is self-contained. All analysis libraries are embedded — no external
dependencies at runtime.

## A refactoring session

You're cleaning up a Go service. Here's what a session looks like.

### Find duplicate functions

You ask: *"Scan my/service/... for duplicate or near-duplicate functions."*

The assistant uses the `goast-refactor` prompt to drive the analysis. Behind the
scenes, it parses every function, groups them by signature shape, and runs a
structural diff on each pair. Substitution collapsing separates real differences
from type propagation noise — if `FooStore` and `BarStore` have identical method
bodies except for the type name, the tool recognizes that as a single type
parameter, not dozens of scattered diffs.

The output is a scored list of candidates:

```
  store.CreateFoo  <->  store.CreateBar
    similarity:      0.94
    type params:     1
      ?T0: FooRecord -> BarRecord
    literals:        0
    operators:       0
    cosmetic:        12
    structural:      0
```

94% similar, one type parameter, zero structural differences. That's a
unification candidate — extract a generic function, delete the duplicate.

A pair at 0.65 similarity with two structural diffs and a literal difference?
That's compression, not simplification. The tool flags the difference so you can
decide.

### Check for convention violations

You ask: *"Do all callers of raft.Step handle the returned error?"*

The assistant defines a belief — a statistical consistency check — and runs it
against your package:

```
  step-error-handling
    sites:     callers-of "Step"
    adherence: 87% (13/15)
    deviations:
      processMessage  (file.go:142)  — missing error handling
      handleRaftReady (file.go:308)  — missing error handling
```

87% of callers handle the error. Two don't. Those are the ones to investigate.

Beliefs work because conventions are statistical. You don't write a rule that
says "every caller must do X." You say "find the callers, check this property,
report the minority." The codebase defines the convention; the tool finds who
breaks it.

The same mechanism handles lock/unlock pairing, ordered operations, field
co-mutation, nil checks before dereference — any property that most code follows
but some code doesn't.

### Check lock discipline

You ask: *"Is Lock always paired with Unlock? Check all paths, including
deferred."*

Two beliefs, chained:

```
  lock-unlock-pairing
    sites:     functions calling Lock
    adherence: 91% (42/46)
    deviations:
      acquireLease  (lease.go:87)   — unpaired
      startElection (raft.go:203)   — unpaired
      ...
```

The deviations are functions that call `Lock` without calling `Unlock`. But
maybe their callers handle it. A second belief chains off the first — it takes
the deviating functions and checks one level up the call stack:

```
  lock-unlock-callers
    sites:     deviations from lock-unlock-pairing
    adherence: 75% (3/4)
    deviations:
      handleTimeout (server.go:445) — caller also missing Unlock
```

Three of the four deviations are handled by their callers. One isn't.
That's your bug.

### Find false struct boundaries

You ask: *"Which struct boundaries don't match actual field access patterns?"*

The assistant builds a concept lattice from SSA field-store data — which
functions write to which fields of which types. Natural groupings emerge from
the data. When a grouping spans multiple struct types, it means functions are
treating fields from different types as a unit. Those are false boundary
candidates.

```
  Cross-boundary concept:
    types:  MachineContext, vmState
    fields: pc, sp, callStack, continuation
    functions (7): step, resume, pushFrame, popFrame, ...
    
    These 7 functions access fields from both types together.
    Consider: colocate fields, extract shared type, or confirm
    the coupling is intentional.
```

This is Formal Concept Analysis (Ganter & Wille, 1999) — not a heuristic. The
concept lattice is the mathematical structure that describes all valid field
groupings given the access data. Cross-boundary concepts are groupings the data
supports but the type system doesn't reflect.

### Verify after refactoring

You've merged the duplicate functions, fixed the lock bug, restructured the
types. Now verify:

*"Run the same beliefs again. Also run nilness, unusedresult, and shadow
analysis on the changed packages."*

The beliefs confirm the conventions still hold — or catch new deviations your
refactoring introduced. The lint passes catch mechanical issues. The call graph
confirms all callers of the old functions now reference the unified replacement.

## What else can I ask

The MCP server includes three guided prompts that structure the analysis:

| Prompt | Use when you want to... |
|--------|------------------------|
| `goast-analyze` | Query code structure: AST shape, data flow, control flow, call relationships |
| `goast-beliefs` | Define and run consistency checks across a package |
| `goast-refactor` | Find unification candidates and verify refactoring correctness |

Beyond the walkthrough above, the analysis layers support:

- **Call graph queries** — who calls this function? What's reachable from main?
  Static, CHA, and RTA algorithms.
- **Control flow** — does statement A dominate statement B? Enumerate all paths
  between two blocks. Build dominator trees.
- **SSA data flow** — reaching definitions, liveness analysis, constant
  propagation, sign analysis, interval analysis. Worklist-based forward/backward
  analysis over SSA blocks.
- **Algebraic equivalence** — are two SSA expressions equivalent under
  commutativity, identity, and annihilation rules?
- **Boolean simplification** — normalize complex Go conditions to detect
  equivalent or redundant logic.
- **Path algebra** — semiring-parameterized shortest paths over call graphs
  (coupling distance, error propagation depth, etc.)
- **40+ lint analyzers** — the full `go/analysis` suite, invocable by name.

## As a Go library

If you want to embed the analysis in your own tooling:

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

## Build and test

```bash
make build       # Build to ./dist/{os}/{arch}/wile-goast
make test        # Run all tests
make lint        # Run golangci-lint
make ci          # Full CI: lint + build + test + covercheck + verify-mod
```

## Documentation

| Document | Content |
|----------|---------|
| [docs/PRIMITIVES.md](docs/PRIMITIVES.md) | Complete reference for all analysis primitives |
| [docs/AST-NODES.md](docs/AST-NODES.md) | Field reference for all 50+ Go AST node types |
| [docs/EXAMPLES.md](docs/EXAMPLES.md) | Annotated walkthroughs of example scripts |
| [docs/GO-STATIC-ANALYSIS.md](docs/GO-STATIC-ANALYSIS.md) | Cross-layer usage guide |

## Dependencies

| Dependency | Purpose |
|-----------|---------|
| [github.com/aalpar/wile](https://github.com/aalpar/wile) | R7RS Scheme interpreter and extension API |
| golang.org/x/tools | SSA, call graph, CFG, go/analysis framework |
| mark3labs/mcp-go | MCP server (JSON-RPC over stdio) |

Built on [Wile](https://github.com/aalpar/wile), an R7RS Scheme interpreter.
Go's AST is already a tree — s-expressions are the natural representation.
The AI writes Scheme fluently; you don't need to.
