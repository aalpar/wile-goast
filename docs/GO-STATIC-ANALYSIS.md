# Go Static Analysis with wile-goast

Cross-layer static analysis of Go source code, scripted in Scheme. Five
extension libraries expose Go's compiler toolchain as composable primitives.
A sixth library (the belief DSL) provides declarative consistency checking.

## Installation

```bash
go install github.com/aalpar/wile-goast/cmd/wile-goast@latest
```

After install, the binary is self-contained — all Scheme libraries and
built-in scripts are embedded.

## Invocation

```bash
# Evaluate a Scheme expression
wile-goast '(go-parse-expr "1 + 2")'

# Run an embedded script
wile-goast --run goast-query

# List available built-in scripts
wile-goast --list-scripts

# Run a script file
wile-goast -f my-analysis.scm
```

## The Six Layers

| Library | Import | What it answers |
|---------|--------|-----------------|
| AST | `(wile goast)` | "What is the shape of this code?" — syntax, structure, types |
| SSA | `(wile goast ssa)` | "Where does this value flow?" — data flow, field stores |
| Call Graph | `(wile goast callgraph)` | "Who calls whom?" — inter-procedural relationships |
| CFG | `(wile goast cfg)` | "Must this check happen before that return?" — control flow ordering |
| Lint | `(wile goast lint)` | "What do standard analyzers report?" — pluggable diagnostics |
| Belief DSL | `(wile goast belief)` | "What implicit conventions exist?" — statistical deviation detection |

All layers share one node format: **tagged alists** `(tag (key . val) ...)`.
The same `assoc`, `walk`, `filter-map` patterns work across every layer.

## Quick Start

### Parse Go source and extract function names

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

### Type-check a package and find error-returning functions

```scheme
(define pkgs (go-typecheck-package "my/package"))

(define (returns-error? func)
  (let* ((ftype (cdr (assoc 'type (cdr func))))
         (results (cdr (assoc 'results (cdr ftype)))))
    (and (pair? results)
         (let loop ((rs results))
           (cond ((null? rs) #f)
                 ((let ((t (cdr (assoc 'type (cdr (car rs))))))
                    (and (eq? (car t) 'ident)
                         (equal? (cdr (assoc 'name (cdr t))) "error")))
                  #t)
                 (else (loop (cdr rs))))))))
```

### Build a call graph and query callers

```scheme
(import (wile goast callgraph))

(define cg (go-callgraph "." 'cha))
(define callers (go-callgraph-callers cg "(*Server).Handle"))

;; What's reachable from main?
(define reachable (go-callgraph-reachable cg "command-line-arguments.main"))
```

### Check control flow dominance

```scheme
(import (wile goast cfg))

(define cfg (go-cfg "." "ProcessRequest"))
(define dom (go-cfg-dominators cfg))

;; Does the auth check dominate the handler?
(go-cfg-dominates? dom auth-block handler-block)
```

### Run lint analyzers

```scheme
(import (wile goast lint))

(define diags (go-analyze "./..." "nilness" "shadow"))

(for-each
  (lambda (d)
    (display (cdr (assoc 'pos (cdr d))))
    (display ": ")
    (display (cdr (assoc 'message (cdr d))))
    (newline))
  diags)
```

### Define a consistency belief

```scheme
(import (wile goast belief))

(define-belief "lock-unlock"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.90 5))

(run-beliefs "./...")
```

## Node Format

Every node across all layers is a tagged alist:

```
(tag (key1 . val1) (key2 . val2) ...)
```

Access fields with standard Scheme:

```scheme
(car node)                          ; tag symbol (e.g. 'func-decl)
(cdr (assoc 'name (cdr node)))     ; field value (e.g. "Add")
```

Or use the `nf` utility from `(wile goast utils)`:

```scheme
(import (wile goast utils))
(nf node 'name)    ; => "Add" or #f
(tag? node 'ident) ; => #t or #f
```

## Cross-Layer Analysis

The power is in combining layers. A single script can:

1. **AST**: Find structs with boolean field clusters
2. **SSA**: Check if those fields are mutated independently
3. **CFG**: Verify dominance ordering between field accesses

Cross-referencing between layers uses position strings (`"file:line:col"`).
Enable `'positions` on the relevant primitives to include these fields.

### Layer Selection Guide

| Question | Layer |
|----------|-------|
| What functions exist? What are their signatures? | AST |
| Where does this value come from? What uses it? | SSA |
| Does A always happen before B within a function? | CFG |
| Who calls this function? What's reachable? | Call Graph |
| What do standard vet checks report? | Lint |
| Is there an implicit convention being violated? | Belief DSL |

## Shared Sessions

`go-load` creates a GoSession — an opaque value holding loaded packages with
lazy SSA and callgraph. Pass it to any package-loading primitive instead of a
pattern string to avoid redundant `packages.Load` calls:

```scheme
(define s (go-load "my/pkg/a" "my/pkg/b"))
(go-typecheck-package s)   ;; reuses loaded state
(go-ssa-build s)           ;; same packages, no reload
(go-cfg s "MyFunc")        ;; same SSA program
(go-callgraph s 'cha)      ;; same snapshot
```

Seven primitives accept this dual-accept pattern: `go-typecheck-package`,
`go-ssa-build`, `go-ssa-field-index`, `go-cfg`, `go-callgraph`, `go-analyze`,
and `go-interface-implementors`.

`go-list-deps` uses lightweight loading (`NeedName | NeedImports` only) for
dependency discovery before committing to a full load.

## Transformation

Two primitives support AST/SSA transformation:

**`go-cfg-to-structured`** restructures guard-if-return blocks into single-exit
if/else trees. Returns the block unchanged if no early returns; returns `#f` for
blocks containing `goto` or labeled statements.

**`go-ssa-canonicalize`** reorders SSA blocks by dominator pre-order and
alpha-renames all registers, producing a deterministic representation for
structural comparison.

The `(wile goast utils)` library provides `ast-transform` (depth-first tree
rewriter) and `ast-splice` (flat-map list rewriter) for custom transformations.

## MCP Server

`wile-goast --mcp` starts a stdio MCP server. One persistent Wile engine
serves all `eval` tool calls within the session.

Three prompts provide guided workflows: `goast-analyze` (structural analysis),
`goast-beliefs` (belief DSL), and `goast-refactor` (unification detection).

```json
{"mcpServers": {"wile-goast": {"command": "wile-goast", "args": ["--mcp"]}}}
```

## Per-Project Beliefs

Create a `.goast-beliefs/` directory at your project root with `.scm` files
defining beliefs. Each file imports the DSL and defines beliefs, but does
**not** call `run-beliefs` — the runner supplies the target at invocation:

```
myproject/
├── .goast-beliefs/
│   ├── lock-unlock.scm
│   └── error-handling.scm
├── go.mod
└── ...
```

Example `.goast-beliefs/lock-unlock.scm`:

```scheme
(import (wile goast belief))

(define-belief "lock-unlock-pairing"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.90 5))
```

Run all beliefs:

```bash
wile-goast '(begin
  (import (wile goast belief))
  (load ".goast-beliefs/lock-unlock.scm")
  (run-beliefs "./..."))'
```

## Reference

- [PRIMITIVES.md](PRIMITIVES.md) — Complete primitive reference for all layers
- [AST-NODES.md](AST-NODES.md) — Field reference for all 50+ AST node tags
- [EXAMPLES.md](EXAMPLES.md) — Annotated walkthroughs of example scripts
