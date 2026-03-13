# Primitive Reference

Complete reference for all primitives across the five wile-goast extension
libraries. Each primitive is documented with its exact signature, return type,
options, and security requirements as implemented in the Go source.

For the full guide with architecture and design rationale, see
[GO-STATIC-ANALYSIS.md](GO-STATIC-ANALYSIS.md).

---

## AST Layer -- `(wile goast)`

**Go package:** `goast`

Parses Go source code into s-expression ASTs and converts them back to Go
source. Optionally type-checks packages via `go/packages`.

### Primitives

| Primitive | Returns | Description |
|-----------|---------|-------------|
| `(go-parse-file filename . options)` | tagged alist | Parse a Go source file from disk |
| `(go-parse-string source . options)` | tagged alist | Parse a Go source string as a file |
| `(go-parse-expr source)` | tagged alist | Parse a single Go expression |
| `(go-format ast)` | string | Convert s-expression AST back to Go source |
| `(go-node-type ast)` | symbol | Return the tag symbol of an AST node |
| `(go-typecheck-package pattern . options)` | list of tagged alists | Load and type-check Go package(s) |

### Options

`go-parse-file`, `go-parse-string`, and `go-typecheck-package` accept optional
trailing symbols:

| Symbol | Effect |
|--------|--------|
| `'positions` | Include `(pos . "file:line:col")` fields on nodes |
| `'comments` | Include `(doc ...)`, `(comment ...)`, and `(comments ...)` fields |

`go-parse-expr` accepts no options.

### Parameters

- **filename** (string): Filesystem path to a `.go` file.
- **source** (string): Go source text. For `go-parse-string`, must be a
  complete file (with `package` clause). For `go-parse-expr`, a single
  expression.
- **pattern** (string): A `go list`-compatible pattern: `"."`, `"./..."`, or a
  full import path.
- **ast** (tagged alist): An s-expression AST node as returned by the parse
  primitives.

### Return values

- `go-parse-file`, `go-parse-string`: A `(file ...)` tagged alist.
- `go-parse-expr`: A single expression node (e.g. `(binary-expr ...)`,
  `(call-expr ...)`).
- `go-format`: A string of formatted Go source. Falls back to unformatted
  output for partial ASTs.
- `go-node-type`: A symbol (e.g. `func-decl`, `if-stmt`, `ident`).
- `go-typecheck-package`: A list of `(package ...)` nodes. Each package node
  contains `(name . "...")`, `(path . "...")`, and `(files ...)`. When type
  info is available, expression nodes gain `(inferred-type . "...")` fields
  and identifiers gain `(obj-pkg . "...")` fields.

### AST node tags

The mapper produces 50+ distinct node tags. The major categories:

**Declarations:** `file`, `func-decl`, `gen-decl`, `bad-decl`

**Specs:** `import-spec`, `value-spec`, `type-spec`

**Statements:** `block`, `return-stmt`, `expr-stmt`, `assign-stmt`, `if-stmt`,
`for-stmt`, `range-stmt`, `branch-stmt`, `decl-stmt`, `inc-dec-stmt`,
`go-stmt`, `defer-stmt`, `send-stmt`, `labeled-stmt`, `switch-stmt`,
`type-switch-stmt`, `case-clause`, `select-stmt`, `comm-clause`, `bad-stmt`

**Expressions:** `ident`, `lit`, `binary-expr`, `unary-expr`, `call-expr`,
`selector-expr`, `index-expr`, `index-list-expr`, `star-expr`, `paren-expr`,
`composite-lit`, `kv-expr`, `func-lit`, `type-assert-expr`, `slice-expr`,
`ellipsis`, `bad-expr`

**Types:** `array-type`, `map-type`, `struct-type`, `interface-type`,
`func-type`, `chan-type`, `field`

### Security

| Primitive | Resource | Action |
|-----------|----------|--------|
| `go-parse-file` | `ResourceFile` | `ActionRead` |
| `go-typecheck-package` | `ResourceProcess` | `ActionLoad` |
| `go-parse-string`, `go-parse-expr`, `go-format`, `go-node-type` | none | none |

### Usage

```scheme
(import (wile goast))

;; Parse a file with positions and comments
(define ast (go-parse-file "main.go" 'positions 'comments))

;; Extract function names
(define names
  (filter-map
    (lambda (decl)
      (and (eq? (car decl) 'func-decl)
           (cdr (assoc 'name (cdr decl)))))
    (cdr (assoc 'decls (cdr ast)))))

;; Round-trip: AST to source
(define src (go-format ast))

;; Type-check a package
(define pkgs (go-typecheck-package "./..." 'positions))
```

---

## SSA Layer -- `(wile goast ssa)`

**Go package:** `goastssa`

Builds SSA (Static Single Assignment) intermediate representation for Go
packages. SSA exposes data flow: every value is defined exactly once, control
flow is explicit via basic blocks, and phi nodes merge values at join points.

### Primitives

| Primitive | Returns | Description |
|-----------|---------|-------------|
| `(go-ssa-build pattern . options)` | list of `ssa-func` nodes | Build SSA for a Go package |

### Options

| Symbol | Effect |
|--------|--------|
| `'positions` | Include `(pos . "file:line:col")` on instructions with valid source positions |

Unrecognized option symbols produce an error.

### Parameters

- **pattern** (string): A `go list`-compatible pattern.

### Return value

A list of `(ssa-func ...)` nodes. Each function contains:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Function name |
| `signature` | string | Go type signature |
| `params` | list of `ssa-param` | Parameters with name and type |
| `free-vars` | list of `ssa-free-var` | Captured variables (closures) |
| `blocks` | list of `ssa-block` | Basic blocks |
| `pkg` | string | Package path (when available) |

Each `ssa-block` contains `index`, `preds`, `succs`, `instrs`, and optionally
`comment`.

### SSA instruction tags

The mapper handles 36 SSA instruction types. Every instruction node includes
an `(operands ...)` field listing its SSA value operands by name.

**Arithmetic/logic:** `ssa-binop`, `ssa-unop`

**Memory:** `ssa-alloc`, `ssa-store`, `ssa-field-addr`, `ssa-field`,
`ssa-index-addr`, `ssa-index`

**Calls:** `ssa-call`, `ssa-go`, `ssa-defer`, `ssa-run-defers`

**Control flow:** `ssa-phi`, `ssa-if`, `ssa-jump`, `ssa-return`, `ssa-panic`

**Collections:** `ssa-make-map`, `ssa-map-update`, `ssa-lookup`, `ssa-extract`,
`ssa-make-slice`, `ssa-slice`

**Channels:** `ssa-make-chan`, `ssa-send`, `ssa-select`

**Iteration:** `ssa-range`, `ssa-next`

**Type operations:** `ssa-change-type`, `ssa-convert`, `ssa-change-interface`,
`ssa-slice-to-array-ptr`, `ssa-make-interface`, `ssa-type-assert`,
`ssa-multi-convert`

**Closures:** `ssa-make-closure`

**Debug:** `ssa-debug-ref`

**Fallback:** `ssa-unknown` (unmapped instruction types)

### Security

| Primitive | Resource | Action |
|-----------|----------|--------|
| `go-ssa-build` | `ResourceProcess` | `ActionLoad` |

### Usage

```scheme
(import (wile goast ssa))

(define funcs (go-ssa-build "." 'positions))

;; Find all store instructions across all functions
(define stores
  (apply append
    (map (lambda (fn)
           (apply append
             (map (lambda (blk)
                    (filter (lambda (i) (eq? (car i) 'ssa-store))
                            (cdr (assoc 'instrs (cdr blk)))))
                  (cdr (assoc 'blocks (cdr fn))))))
         funcs)))
```

---

## Call Graph Layer -- `(wile goast callgraph)`

**Go package:** `goastcg`

Builds whole-program call graphs using four algorithms of varying precision and
cost. Queries return callers, callees, and transitive reachability.

### Primitives

| Primitive | Returns | Description |
|-----------|---------|-------------|
| `(go-callgraph pattern algorithm)` | list of `cg-node` | Build call graph |
| `(go-callgraph-callers graph func-name)` | list of `cg-edge` or `#f` | Direct callers of a function |
| `(go-callgraph-callees graph func-name)` | list of `cg-edge` or `#f` | Direct callees of a function |
| `(go-callgraph-reachable graph root-name)` | list of strings | Transitive closure from a root |

### Parameters

- **pattern** (string): A `go list`-compatible pattern.
- **algorithm** (symbol): One of `'static`, `'cha`, `'rta`, `'vta`.
- **graph** (list): The call graph returned by `go-callgraph`.
- **func-name** (string): Fully qualified function name as it appears in the
  graph's `name` fields (e.g. `"(*pkg.Type).Method"`).
- **root-name** (string): Starting function for reachability.

### Algorithms

| Algorithm | Precision | Requirements | Description |
|-----------|-----------|--------------|-------------|
| `'static` | Lowest | Any package | Direct calls only (no virtual dispatch) |
| `'cha` | Medium | Any package | Class Hierarchy Analysis -- resolves interface calls |
| `'rta` | High | Requires `main` | Rapid Type Analysis -- interface + reachability |
| `'vta` | Highest | Any package | Variable Type Analysis -- refines CHA with flow |

`'rta` raises an error if the loaded packages contain no `main` function.

### Return values

`go-callgraph` returns a list of `(cg-node ...)` nodes:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Fully qualified function name |
| `id` | integer | Unique node ID |
| `edges-in` | list of `cg-edge` | Incoming call edges |
| `edges-out` | list of `cg-edge` | Outgoing call edges |
| `pkg` | string | Package path (when available) |

Each `(cg-edge ...)` contains:

| Field | Type | Description |
|-------|------|-------------|
| `caller` | string | Caller function name |
| `callee` | string | Callee function name |
| `pos` | string | Call site position (when valid) |
| `description` | string | Edge description from the analysis |

`go-callgraph-callers` and `go-callgraph-callees` return the `edges-in` or
`edges-out` list directly, or `#f` if the function is not in the graph.

`go-callgraph-reachable` returns a sorted list of function name strings.

### Security

| Primitive | Resource | Action |
|-----------|----------|--------|
| `go-callgraph` | `ResourceProcess` | `ActionLoad` |
| `go-callgraph-callers`, `go-callgraph-callees`, `go-callgraph-reachable` | none | none |

The query primitives operate on the in-memory s-expression graph and require no
authorization.

### Usage

```scheme
(import (wile goast callgraph))

(define cg (go-callgraph "." 'vta))

;; Who calls ProcessRequest?
(define callers (go-callgraph-callers cg "(*Server).ProcessRequest"))

;; What's reachable from main?
(define reachable (go-callgraph-reachable cg "command-line-arguments.main"))
```

---

## CFG Layer -- `(wile goast cfg)`

**Go package:** `goastcfg`

Builds the control flow graph for a single function, computes dominator trees,
tests dominance, and enumerates paths between basic blocks.

### Primitives

| Primitive | Returns | Description |
|-----------|---------|-------------|
| `(go-cfg pattern func-name . options)` | list of `cfg-block` | Build CFG for a named function |
| `(go-cfg-dominators cfg)` | list of `dom-node` | Build dominator tree from CFG |
| `(go-cfg-dominates? dom-tree a b)` | boolean | Does block `a` dominate block `b`? |
| `(go-cfg-paths cfg from to)` | list of lists of integers | Enumerate simple paths between blocks |

### Options (for `go-cfg`)

| Symbol | Effect |
|--------|--------|
| `'positions` | Include `(pos . "file:line:col")` on blocks with valid source positions |

Unrecognized option symbols produce an error.

### Parameters

- **pattern** (string): A `go list`-compatible pattern.
- **func-name** (string): Function name (unqualified). Searches package-level
  functions and methods on named types.
- **cfg** (list): The CFG block list returned by `go-cfg`.
- **dom-tree** (list): The dominator tree returned by `go-cfg-dominators`.
- **a**, **b** (integer): Block indices.
- **from**, **to** (integer): Block indices for path enumeration.

### Return values

`go-cfg` returns a list of `(cfg-block ...)` nodes:

| Field | Type | Description |
|-------|------|-------------|
| `index` | integer | Block index |
| `preds` | list of integers | Predecessor block indices |
| `succs` | list of integers | Successor block indices |
| `idom` | integer or `#f` | Immediate dominator index (`#f` for entry block) |
| `recover` | `#t` | Present only on the recover block |
| `comment` | string | Block comment (when available) |
| `pos` | string | Position of first instruction (with `'positions` option) |

`go-cfg-dominators` returns a list of `(dom-node ...)`:

| Field | Type | Description |
|-------|------|-------------|
| `block` | integer | Block index |
| `idom` | integer or `#f` | Immediate dominator (`#f` for entry) |
| `children` | list of integers | Dominated block indices |

`go-cfg-paths` returns a list of paths, where each path is a list of block
index integers. Capped at 1024 paths to bound cost.

### Security

| Primitive | Resource | Action |
|-----------|----------|--------|
| `go-cfg` | `ResourceProcess` | `ActionLoad` |
| `go-cfg-dominators`, `go-cfg-dominates?`, `go-cfg-paths` | none | none |

### Usage

```scheme
(import (wile goast cfg))

;; Build CFG for function Run
(define cfg (go-cfg "." "Run"))

;; Build dominator tree
(define dom (go-cfg-dominators cfg))

;; Does block 1 dominate block 5?
(go-cfg-dominates? dom 1 5)  ; => #t or #f

;; Enumerate paths from entry (0) to exit block
(define paths (go-cfg-paths cfg 0 3))
```

---

## Lint Layer -- `(wile goast lint)`

**Go package:** `goastlint`

Runs `go/analysis` passes on Go packages. Wraps the `checker.Analyze` driver
which handles prerequisite resolution and cross-package fact propagation.

### Primitives

| Primitive | Returns | Description |
|-----------|---------|-------------|
| `(go-analyze pattern analyzer-name ...)` | list of `diagnostic` | Run named analyzers on a package |
| `(go-analyze-list)` | list of strings | List available analyzer names |

### Parameters

- **pattern** (string): A `go list`-compatible pattern.
- **analyzer-name** (string): One or more analyzer names. Must be strings, not
  symbols. Unknown names produce an error referencing `go-analyze-list`.

### Return values

`go-analyze` returns a list of `(diagnostic ...)` nodes:

| Field | Type | Description |
|-------|------|-------------|
| `analyzer` | string | Name of the analyzer that produced this diagnostic |
| `pos` | string | Source position (`file:line:col`) |
| `message` | string | Diagnostic message |
| `category` | string | Diagnostic category (may be empty) |

Returns an empty list if no diagnostics are found or if no analyzer names are
provided.

`go-analyze-list` returns a sorted list of analyzer name strings.

### Available analyzers (25)

`assign`, `bools`, `composite`, `copylocks`, `defers`, `directive`,
`errorsas`, `httpresponse`, `ifaceassert`, `loopclosure`, `lostcancel`,
`nilfunc`, `nilness`, `printf`, `shadow`, `shift`, `sortslice`,
`stdmethods`, `stringintconv`, `structtag`, `testinggoroutine`, `tests`,
`timeformat`, `unmarshal`, `unreachable`

### Security

| Primitive | Resource | Action |
|-----------|----------|--------|
| `go-analyze` | `ResourceProcess` | `ActionLoad` |
| `go-analyze-list` | none | none |

### Usage

```scheme
(import (wile goast lint))

;; List available analyzers
(go-analyze-list)

;; Run nilness and shadow on the current package
(define diags (go-analyze "." "nilness" "shadow"))

;; Print diagnostics
(for-each
  (lambda (d)
    (display (cdr (assoc 'pos (cdr d))))
    (display ": ")
    (display (cdr (assoc 'message (cdr d))))
    (newline))
  diags)
```

---

## S-Expression Node Format

All five libraries share a single node representation: **tagged alists**.

```
(tag (key1 . val1) (key2 . val2) ...)
```

- The **car** of the outer pair is a symbol identifying the node type (the
  tag).
- The **cdr** is an association list of `(key . value)` pairs.
- Values are strings, symbols, integers, booleans (`#t`/`#f`), lists, or
  nested tagged alists.
- Absent optional values are represented as `#f`.
- Lists (e.g. of declarations, instructions, edges) are proper Scheme lists.

### Querying nodes

Standard Scheme list operations work on all node types uniformly:

```scheme
;; Get the tag
(car node)             ; => 'func-decl

;; Get a field value
(cdr (assoc 'name (cdr node)))  ; => "Add"

;; Test the tag
(eq? (car node) 'ssa-store)

;; Walk all fields
(for-each (lambda (pair) ...) (cdr node))
```

### Cross-referencing between layers

Layers share position strings (`"file:line:col"`) as the common join key. A
position from an AST node can be matched against an SSA instruction's `pos`
field or a call graph edge's `pos` field. Enable `'positions` on the relevant
primitives to include these fields.

---

## Cross-References

- [GO-STATIC-ANALYSIS.md](GO-STATIC-ANALYSIS.md) -- Full guide: architecture,
  design rationale, cross-layer example.
