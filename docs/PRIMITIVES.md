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
| `(go-typecheck-package target . options)` | list of tagged alists | Load and type-check Go package(s) |
| `(go-interface-implementors name target)` | tagged alist | Find types implementing a named interface |
| `(go-load pattern ... . options)` | GoSession | Load packages into a reusable session |
| `(go-session? v)` | boolean | Type predicate for GoSession |
| `(go-list-deps pattern ...)` | list of strings | Transitive import path discovery |
| `(go-cfg-to-structured block [func-type])` | block or `#f` | Restructure block into single-exit form: goto elimination, loop return rewriting, guard folding |

### Session Management

`go-load` creates a GoSession that holds loaded packages and lazily builds SSA.
All package-loading primitives (`go-typecheck-package`, `go-ssa-build`,
`go-ssa-field-index`, `go-cfg`, `go-callgraph`, `go-analyze`,
`go-interface-implementors`) accept either a pattern string (load fresh) or a
GoSession (reuse loaded state). The `target` parameter in the signatures above
accepts both types.

```scheme
;; Load once, query many — all layers see the same source snapshot
(define s (go-load "my/pkg/a" "my/pkg/b"))
(define pkgs (go-typecheck-package s))
(define ssa  (go-ssa-build s))
(define cfg  (go-cfg s "MyFunc"))
(define cg   (go-callgraph s 'cha))

;; Old style still works — loads fresh each time
(go-ssa-build "my/pkg/a")
```

**Options for `go-load`:**

| Symbol | Effect |
|--------|--------|
| `'lint` | Upgrade to `LoadAllSyntax` for `go-analyze` support |

**`go-list-deps`** uses lightweight loading (`NeedName | NeedImports` only) for
dependency discovery before committing to a full load.

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
| `go-interface-implementors` (pattern) | `ResourceProcess` | `ActionLoad` |
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

### Interface Implementors

`go-interface-implementors` finds all concrete types implementing a named
interface within the loaded packages. Uses `go/types` to check both `T` and
`*T` against the interface method set.

**Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Interface name — short (`"Store"`) or qualified (`"pkg.Store"`) |
| `target` | string or GoSession | Package pattern or reusable session |

If the short name is ambiguous across packages, an error lists the candidates.
Interfaces with no methods (e.g. type-constraint interfaces) are rejected.

**Return value:** A tagged alist:

```scheme
(interface-info
  (name . "Store")
  (pkg . "github.com/example/pkg")
  (methods . ("Get" "Set" "Delete"))
  (implementors . (((type . "MemoryStore") (pkg . "github.com/example/pkg"))
                   ((type . "SimpleStore") (pkg . "github.com/example/pkg")))))
```

**Usage:**

```scheme
;; Find implementors of a project-local interface
(define info (go-interface-implementors "Store" "my/pkg/..."))

;; With a shared session
(define s (go-load "my/pkg/..."))
(define info (go-interface-implementors "Store" s))

;; Use in belief DSL via selectors (preferred)
(define-belief "store-handles-errors"
  (sites (interface-methods "Store" "Get"))
  (expect (contains-call "ErrNotFound"))
  (threshold 0.80 2))
```

### Transformation

`go-cfg-to-structured` takes a block s-expression and returns a restructured
block where all early returns and gotos are eliminated, producing a single-exit
block. Every return in the output is at a leaf of an if/else tree.

Optional second argument: a func-type s-expression. When provided, loop-local
return values are assigned to synthesized variables (`_r0`, `_r1`, ...) declared
before the loop, ensuring return values reference variables visible after loop exit.

Four phases run in sequence:

- **Phase 0a (backward goto):** `label: stmt ... if cond { goto label }` patterns
  become `for { stmt ... if !cond { break } }`. Bare `goto label` becomes an
  infinite `for` loop.
- **Phase 0b (forward goto):** `if cond { goto L } ... L: stmt` patterns become
  `if !cond { ... } stmt`. Multiple gotos to the same label are handled via
  fixpoint (last-to-first iteration).
- **Phase 1 (loop returns):** Returns inside `for`/`range` are rewritten as
  `_ctl<N> = K; break` with guard-if-return statements after the loop. Returns
  inside `switch`/`select` within loops use labeled break (`break _loop<N>`).
  Supports nested loops (bottom-up) and multiple return sites per loop.
- **Phase 2 (linear guards):** `if cond { return X }` patterns are folded into
  nested if/else chains via right-fold.

Returns the block unchanged if there are no early returns or gotos. Returns `#f`
if the block contains control flow it cannot restructure (cross-branch gotos,
gotos targeting labels inside nested blocks). **Callers must check for `#f`
before chaining** — passing it to `ast-transform` or `subst-idents` will produce
wrong results.

```scheme
;; Phase 0b: forward goto → if !cond wrapping
;; if x > 0 { goto end }; println(x); end: println(0)
;;   → if !(x > 0) { println(x) }; println(0)

;; Phase 1: loop returns → _ctl + break + guard
;; for _, v := range items { if v < 0 { return v } }; return 0
;;   → var _ctl0 int; for ... { _ctl0 = 1; break }; if _ctl0 == 1 { return v } else { return 0 }

;; Phase 1 with func-type: result variable synthesis
;; (go-cfg-to-structured body ftype)
;;   → var _ctl0 int; var _r0 int; for ... { _r0 = v; _ctl0 = 1; break }
;;     if _ctl0 == 1 { return _r0 } else { return 0 }

;; Phase 2: linear guards → if/else
;; if x < lo { return lo }; if x > hi { return hi }; return x
;;   → if x < lo { return lo } else if x > hi { return hi } else { return x }
```

**Limitations:**
- Top-level only — does not recurse into nested blocks
- Cross-branch gotos (into/out of if/switch) return `#f`
- Naked returns and multi-value call returns (`return f()`) bail when func-type is provided

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
| `(go-ssa-field-index pattern)` | list of `ssa-field-summary` nodes | Pre-correlated per-function field access index |
| `(go-ssa-canonicalize ssa-func)` | `ssa-func` node | Canonicalize blocks (dominator pre-order) and registers (alpha-rename) |

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
`comment` and `idom` (immediate dominator block index; absent on the entry
block).

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

### `go-ssa-canonicalize`

Canonicalizes an `ssa-func` s-expression for structural comparison:

1. **Block ordering** — reorders blocks by pre-order DFS of the dominator tree
2. **Register renaming** — alpha-renames parameters to `p0, p1, ...`, free variables to `fv0, fv1, ...`, and instruction definitions to `r0, r1, ...` in canonical block order
3. **Cross-reference update** — reindexes all `preds`, `succs`, `idom`, phi edges, jump/if targets

```scheme
(define funcs (go-ssa-build "my/pkg"))
(define fn (car funcs))
(define canonical (go-ssa-canonicalize fn))
;; canonical is an ssa-func node with deterministic block order and register names
```

### `go-ssa-field-index`

Returns a pre-correlated field access index: one entry per function that
reads or writes at least one struct field. Functions with no field accesses
are omitted. This is orders of magnitude faster than walking SSA trees in
Scheme for field-access queries.

Each `(ssa-field-summary ...)` node contains:

| Field | Type | Description |
|-------|------|-------------|
| `func` | string | Function short name |
| `pkg` | string | Package import path |
| `fields` | list of `ssa-field-access` | Field access entries |

Each `(ssa-field-access ...)` contains:

| Field | Type | Description |
|-------|------|-------------|
| `struct` | string | Short struct type name (from Go type system) |
| `struct-pkg` | string | Import path of package defining the struct |
| `field` | string | Field name |
| `recv` | string | SSA receiver register name |
| `mode` | symbol | `read` or `write` |

### Security

| Primitive | Resource | Action |
|-----------|----------|--------|
| `go-ssa-build` | `ResourceProcess` | `ActionLoad` |
| `go-ssa-field-index` | `ResourceProcess` | `ActionLoad` |

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

;; Field index: find which functions write to a specific field
(define index (go-ssa-field-index "."))
(define writers
  (filter-map
    (lambda (summary)
      (let ((writes (filter-map
                      (lambda (access)
                        (and (eq? (cdr (assoc 'mode (cdr access))) 'write)
                             (equal? (cdr (assoc 'field (cdr access))) "Name")
                             access))
                      (cdr (assoc 'fields (cdr summary))))))
        (and (pair? writes)
             (cdr (assoc 'func (cdr summary))))))
    index))
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

---

## Belief DSL -- `(wile goast belief)`

**Implementation:** Pure Scheme library (embedded in binary)

Declarative consistency deviation detection. Define beliefs as patterns
extracted from code (Engler et al., "Bugs as Deviant Behavior"). The DSL
handles layer selection, data loading, and statistical comparison.

### Core Form

```scheme
(define-belief <name:string>
  (sites <site-selector>)
  (expect <property-checker>)
  (threshold <min-adherence:number> <min-sites:number>))

(run-beliefs <target:string>)
```

- **name** -- string identifier, used in output and for `sites-from` references.
- **sites** -- enumerates code locations to analyze. Returns a list of sites.
- **expect** -- classifies each site into a category symbol. The majority
  category becomes the belief; minorities are deviations.
- **threshold** -- minimum adherence ratio and minimum site count for reporting.
- **target** -- a `go list`-compatible pattern (e.g. `"."`, `"./..."`,
  `"my/package/..."`).

### Site Selectors

| Selector | Description |
|----------|-------------|
| `(functions-matching pred ...)` | Functions matching all predicates |
| `(callers-of "func")` | All callers of a function (call graph layer); returns AST func-decl nodes via `ssa-short-name` matching |
| `(methods-of "Type")` | All methods on a receiver type |
| `(implementors-of "Interface")` | All func-decls whose receiver implements the interface |
| `(interface-methods "Interface" [method])` | Func-decls implementing interface methods, optionally narrowed to one method |
| `(all-func-decls)` | All function declarations across all packages |
| `(sites-from "belief" 'which 'adherence)` | Results from a prior belief (bootstrapping) |

### Selector Predicates

Used as arguments to `functions-matching`:

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

| Checker | Returns | Description |
|---------|---------|-------------|
| `(contains-call "func" ...)` | `'present` / `'absent` | Call present in body? |
| `(paired-with "A" "B")` | `'paired-defer` / `'paired-call` / `'unpaired` | A paired with B? |
| `(ordered "A" "B")` | `'a-dominates-b` / `'b-dominates-a` / `'unordered` / `'missing` | SSA block dominance; same-block resolved by instruction position |
| `(co-mutated "field" ...)` | `'co-mutated` / `'partial` / `'missing` | Fields stored together? |
| `(checked-before-use "val")` | `'guarded` / `'unguarded` / `'missing` | Value checked before use? Product lattice fixpoint via `(wile algebra)` with early exit. 4-hop def-use reachability |
| `(custom (lambda (site ctx) ...))` | user-defined symbol | Escape hatch |

### Context Accessors

Available in `custom` lambdas:

| Accessor | Description |
|----------|-------------|
| `(ctx-pkgs ctx)` | Lazy-loaded type-checked packages |
| `(ctx-ssa ctx)` | Lazy-loaded SSA functions |
| `(ctx-callgraph ctx)` | Lazy-loaded call graph |
| `(ctx-field-index ctx)` | Lazy-loaded field access index |
| `(ctx-find-ssa-func ctx pkg-path name)` | Look up SSA function by package + name |

### Utility Functions

Re-exported from `(wile goast utils)` for use in `custom` lambdas:

`nf`, `tag?`, `walk`, `filter-map`, `flat-map`, `member?`, `unique`

### Usage

```scheme
(import (wile goast belief))

;; Define a co-mutation belief
(define-belief "status-fields"
  (sites (functions-matching
           (stores-to-fields "Status" "State" "Message")))
  (expect (co-mutated "State" "Message"))
  (threshold 0.66 3))

;; Define a pairing belief
(define-belief "lock-unlock"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.90 5))

;; Run all beliefs against target package(s)
(run-beliefs "./...")
```

---

## Shared Utilities -- `(wile goast utils)`

**Implementation:** Pure Scheme library (embedded in binary)

Traversal utilities for the tagged-alist node format shared by all layers.

| Function | Description |
|----------|-------------|
| `(nf node 'key)` | Get field value by key, or `#f` if absent |
| `(tag? node 'tag)` | Test whether node has a given tag |
| `(walk val visitor)` | Depth-first walk; collect non-`#f` visitor results |
| `(filter-map f lst)` | Map, keeping only non-`#f` results |
| `(flat-map f lst)` | Map (f returns list), concatenate results |
| `(member? x lst)` | Membership test using `equal?` |
| `(unique lst)` | Remove duplicates, preserving order |
| `(has-char? s c)` | Does string `s` contain character `c`? |
| `(ordered-pairs lst)` | All unordered pairs from a list (each pair once) |
| `(take lst n)` | First n elements |
| `(drop lst n)` | Drop first n elements |
| `(ast-transform node f)` | Depth-first pre-order tree rewriter. `f` returns replacement or `#f` (keep). Note: `#f` cannot be used as a replacement value |
| `(ast-splice lst f)` | Flat-map rewriter for lists. `f` returns list (splice) or `#f` (keep). Note: `#f` cannot be a splice element |

### Usage

```scheme
(import (wile goast utils))

;; Extract a field
(nf some-node 'name)  ; => "Add" or #f

;; Find all call-expr nodes in a function body
(walk (nf func-decl 'body)
  (lambda (node)
    (and (tag? node 'call-expr) node)))
```

---

## SSA Normalization -- `(wile goast ssa-normalize)`

**Implementation:** Pure Scheme library (embedded in binary)

Algebraic normalization rules for SSA binop nodes. Integer-type scoped to avoid IEEE 754 issues. Extensible via `ssa-rule-set`.

| Function | Description |
|----------|-------------|
| `(ssa-normalize node)` | Apply default rules to a node |
| `(ssa-normalize node rules)` | Apply custom rule set to a node |
| `(ssa-rule-commutative)` | Sort operands lexicographically for commutative ops |
| `(ssa-rule-identity)` | `x + 0 -> x`, `x * 1 -> x`, etc. (integer types only) |
| `(ssa-rule-annihilation)` | `x * 0 -> 0`, `x & 0 -> 0` (integer types only) |
| `(ssa-rule-set rule ...)` | Compose rules: first non-`#f` wins |

### Usage

```scheme
(import (wile goast ssa-normalize))

;; Apply default normalization
(ssa-normalize some-binop-node)

;; Custom rule set (commutative only)
(define my-rules (ssa-rule-set (ssa-rule-commutative)))
(ssa-normalize some-binop-node my-rules)
```

---

## Unification Detection -- `(wile goast unify)`

**Implementation:** Pure Scheme library (embedded in binary)

Shared diff/scoring library for AST and SSA structural comparison. Pluggable classifier design: the core `tree-diff` is generic; a classifier function determines how string diffs are categorized.

| Function | Description |
|----------|-------------|
| `(ast-diff node-a node-b)` | Diff two AST nodes with path-based classification |
| `(ssa-diff node-a node-b)` | Diff two SSA nodes with tag-based classification |
| `(tree-diff node-a node-b classifier)` | Generic diff with custom classifier |
| `(diff-result-similarity r)` | Extract similarity (0.0--1.0) from diff result |
| `(diff-result-shared r)` | Shared node count |
| `(diff-result-diff-count r)` | Differing node count |
| `(diff-result-diffs r)` | List of `(category path val-a val-b)` entries |
| `(score-diffs shared diff-count diffs)` | Compute effective similarity with substitution collapsing |
| `(unifiable? result threshold)` | Verdict: `#t` when effective similarity >= threshold and all remaining diffs are type/register |

### Usage

```scheme
(import (wile goast unify))

;; Compare two SSA functions after canonicalization
(let* ((r (ssa-diff canon-a canon-b))
       (sim (diff-result-similarity r)))
  (display sim))

;; Full verdict
(unifiable? (ssa-diff canon-a canon-b) 0.80)
```

---

## Cross-References

- [GO-STATIC-ANALYSIS.md](GO-STATIC-ANALYSIS.md) -- Usage guide with
  architecture overview and cross-layer examples.
- [AST-NODES.md](AST-NODES.md) -- Field reference for all 50+ AST node tags.
- [EXAMPLES.md](EXAMPLES.md) -- Annotated walkthroughs of example scripts.
