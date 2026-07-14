# Primitive Reference

Complete reference for all primitives across the five wile-goast extension
libraries. Each primitive is documented with its exact signature, return type,
options, and security requirements as implemented in the Go source.

For the full guide with architecture and design rationale, see
[GO-STATIC-ANALYSIS.md](GO-STATIC-ANALYSIS.md).

---

## Function Name Convention

All `name` fields on typed AST, SSA, call graph, and field index nodes carry
the **SSA-qualified name** — the same string `ssa.Function.String()` produces:

- Top-level function: `"go.etcd.io/etcd/raft/v3.newRaft"`
- Pointer receiver method: `"(*go.etcd.io/etcd/raft/v3.raft).Step"`
- Value receiver method: `"(go.etcd.io/etcd/raft/v3.Config).validate"`

Untyped AST nodes (`go-parse-file`, `go-parse-string`) carry the short Go
name (e.g. `"Step"`) since no package or type information is available.

Primitives that accept function names as arguments (`go-cfg`,
`go-callgraph-callers`, `go-callgraph-callees`) accept both short names
and SSA-qualified names. Short names are resolved within the loaded package;
SSA-qualified names match exactly. This means function names from any layer
can be passed directly to any primitive without manual normalization.

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
| `(go-typecheck-package [target] . options)` | list of tagged alists | Load and type-check Go package(s) |
| `(go-interface-implementors name [target])` | tagged alist | Find types implementing a named interface |
| `(go-load [pattern]... . options)` | GoSession | Load packages into a reusable session |
| `(go-session? v)` | boolean | Type predicate for GoSession |
| `(go-list-deps pattern ...)` | list of strings | Transitive import path discovery |
| `(go-func-refs [target])` | list of tagged alists | Per-function external reference profiles |
| `(go-cfg-to-structured block [func-type])` | block | Restructure block into single-exit form: goto elimination, loop return rewriting, guard folding |

### Target Parameter

`current-go-target` is an R7RS parameter holding the default package pattern.
Primitives whose target is optional (`go-load`, `go-typecheck-package`,
`go-interface-implementors`, `go-func-refs`, `go-ssa-build`,
`go-ssa-field-index`) use it when called with no target argument. Initialized
from `WILE_GOAST_TARGET`, defaulting to `"./..."`. Override with `parameterize`.

```scheme
(current-go-target)                       ; => "./..."
(parameterize ((current-go-target "my/pkg/..."))
  (go-ssa-build))
```

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

**`go-func-refs`** extracts per-function external reference profiles. For each
function/method in the target package, returns the set of external `(package,
object-name)` pairs it references via `types.Info.Uses`. Accepts a package
pattern string or GoSession. Returns:

```scheme
((func-ref (name . "MyFunc")
           (pkg . "my/pkg")
           (refs . ((ref (pkg . "io") (objects . ("Reader" "Writer")))
                    (ref (pkg . "fmt") (objects . ("Println")))))))
```

Method receivers format as `"RecvType.Method"`. Functions with no body
(interface methods, external declarations) are excluded.

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

Returns the block unchanged if there are no early returns or gotos. Raises
`go restructure error` if the block contains control flow it cannot restructure.
Three distinct error messages identify the failure mode:

- `"cross-branch goto"` — goto targets a label inside a nested block (structurally impossible)
- `"goto pattern not recognized"` — gotos survived phase 0a/0b pattern matching
- `"unrewritable return in loop"` — naked return or multi-value call return with func-type

Use `guard` to handle restructuring failures gracefully:

```scheme
(guard (e (#t #f))  ;; fall back to #f on error
  (go-cfg-to-structured body))
```

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
- Cross-branch gotos and self-gotos raise an error
- Naked returns and multi-value call returns (`return f()`) raise an error when func-type is provided

---

## SSA Layer -- `(wile goast ssa)`

**Go package:** `goastssa`

Builds SSA (Static Single Assignment) intermediate representation for Go
packages. SSA exposes data flow: every value is defined exactly once, control
flow is explicit via basic blocks, and phi nodes merge values at join points.

### Primitives

| Primitive | Returns | Description |
|-----------|---------|-------------|
| `(go-ssa-build [target] . options)` | list of `ssa-func` nodes | Build SSA for a Go package |
| `(go-ssa-field-index [target])` | list of `ssa-field-summary` nodes | Pre-correlated per-function field access index |
| `(go-ssa-canonicalize ssa-func)` | `ssa-func` node | Canonicalize blocks (dominator pre-order) and registers (alpha-rename) |
| `(go-ssa-narrow ssa-func value-name)` | `narrow-result` node | Narrow an SSA value to its concrete producing types |

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

`ssa-make-interface` carries a `concrete` field: the type that entered the
interface, as a type string. It joins to `cg-edge`'s `recv` by string equality,
which is how a dispatch site gets a witness without parsing SSA value names.
The instruction's position is a valid source location only for an explicit
conversion; an implicit one has no position of its own.

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
| `func` | string | SSA-qualified function name |
| `pkg` | string | Package import path |
| `fields` | list of `ssa-field-access` | Field access entries |
| `pos` | string | Function position (when valid) |

Each `(ssa-field-access ...)` contains:

| Field | Type | Description |
|-------|------|-------------|
| `struct` | string | Short struct type name (from Go type system) |
| `struct-pkg` | string | Import path of package defining the struct |
| `field` | string | Field name |
| `recv` | string | SSA receiver register name |
| `mode` | symbol | `read` or `write` |

### `go-ssa-narrow`

Walks an SSA value's definition chain and returns the concrete types that can
produce it. `value-name` is a string matching `ssa.Value.Name()` (e.g. `"t0"`).

```scheme
(go-ssa-narrow fn "t0")
;; => (narrow-result (types . ("*foo.Bar")) (confidence . narrow) (reasons . ()))
```

| Field | Type | Description |
|-------|------|-------------|
| `types` | list of strings | Concrete producing type names |
| `confidence` | symbol | `narrow`, `widened`, or `no-paths` |
| `reasons` | list of symbols | Why the result widened |

`widened` means at least one path hit an untyped boundary. Reason symbols:
`parameter`, `global-load`, `field-load`, `nil-constant`, `cycle`,
`interface-method-dispatch`.

### Security

| Primitive | Resource | Action |
|-----------|----------|--------|
| `go-ssa-build` | `ResourceProcess` | `ActionLoad` |
| `go-ssa-field-index` | `ResourceProcess` | `ActionLoad` |
| `go-ssa-canonicalize`, `go-ssa-narrow` | none | none |

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

Builds whole-program call graphs using five algorithms of varying precision and
cost. Queries return callers and callees.

### Primitives

| Primitive | Returns | Description |
|-----------|---------|-------------|
| `(go-callgraph target algorithm)` | list of `cg-node` | Build call graph |
| `(go-callgraph-callers graph func-name)` | list of `cg-edge` or `#f` | Direct callers of a function |
| `(go-callgraph-callees graph func-name)` | list of `cg-edge` or `#f` | Direct callees of a function |

### Parameters

- **target** (string or GoSession): A `go list`-compatible pattern or a session.
- **algorithm** (string, or symbol for back-compat): One of `"static"`, `"cha"`,
  `"rta"`, `"vta"`, `"precise"`.
- **graph** (list): The call graph returned by `go-callgraph`.
- **func-name** (string): Function name as it appears in the graph's `name`
  fields (e.g. `"(*pkg.Type).Method"`); a short name is matched against them.

### Algorithms

| Algorithm | Precision | Requirements | Description |
|-----------|-----------|--------------|-------------|
| `static` | Lowest | Any package | Direct calls only; omits indirect calls (under-approximates) |
| `cha` | Medium | Any package | Class Hierarchy Analysis -- resolves interface calls |
| `rta` | High | Requires `main` | Rapid Type Analysis -- interface + reachability |
| `vta` | High | Any package | Variable Type Analysis -- refines CHA with flow |
| `precise` | High | Any package | CHA refined by statically resolving the decidable indirect calls (constant index into a literal `[]func()`); keeps CHA's edges elsewhere |

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
| `caller-synthetic` | string | Present only when the caller is compiler-generated; the reason (`$bound`, method-set wrapper, promoted-embedding stub, ...) |
| `callee` | string | Callee function name |
| `pos` | string | Call site position (when valid) |
| `description` | string | Edge description from the analysis |
| `iface` | string | Present only on invoke (interface) sites: the interface type |
| `method` | string | Present only on invoke sites: the method name |
| `recv` | string | Present only on invoke sites: the concrete receiver of the resolved callee |

A synthetic caller's single invoke has no source position, so a call site keyed
on one is a phantom: it does not exist in source. The field carries the reason
rather than a bool so a consumer can say why.

`recv` joins to `ssa-make-interface`'s `concrete` field by string equality. That
is the seam `(wile goast dispatch)` uses to attach a witness (where a concrete
type entered the interface) without parsing names.

`go-callgraph-callers` and `go-callgraph-callees` return the `edges-in` or
`edges-out` list directly, or `#f` if the function is not in the graph.
There is no `go-callgraph-reachable` primitive: transitive reachability lives in
the `(wile goast path-algebra)` Scheme layer, over the graph built here.

### Security

| Primitive | Resource | Action |
|-----------|----------|--------|
| `go-callgraph` | `ResourceProcess` | `ActionLoad` |
| `go-callgraph-callers`, `go-callgraph-callees` | none | none |

The query primitives operate on the in-memory s-expression graph and require no
authorization.

### Usage

```scheme
(import (wile goast callgraph))

(define cg (go-callgraph "." 'vta))

;; Who calls ProcessRequest?
(define callers (go-callgraph-callers cg "(*Server).ProcessRequest"))

;; What does main call directly?
(define callees (go-callgraph-callees cg "command-line-arguments.main"))
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

### Registry

| Export | Description |
|--------|-------------|
| `(current-beliefs)` | Live list of registered per-site beliefs |
| `(aggregate-beliefs)` | Live list of registered aggregate beliefs |
| `(reset-beliefs!)` | Clear both registries |
| `(register-aggregate-belief! name sites-fn analyzer sites-expr analyze-expr)` | Procedural form of `define-aggregate-belief` |

The `*beliefs*` / `*aggregate-beliefs*` variables are not exported: importing a
mutable variable copies its value at import time, so a second import would
diverge from the registry the DSL mutates. Read them via the procedures above.

### Site Selectors

| Selector | Description |
|----------|-------------|
| `(functions-matching pred ...)` | Functions matching all predicates |
| `(callers-of "func")` | All callers of a function (call graph layer); returns AST func-decl nodes via `ssa-short-name` matching |
| `(methods-of "Type")` | All methods on a receiver type |
| `(implementors-of "Interface")` | All func-decls whose receiver implements the interface |
| `(interface-methods "Interface" [method])` | Func-decls implementing interface methods, optionally narrowed to one method |
| `(all-functions-in)` | All func-decls in the context's loaded packages (scope set by `run-beliefs`' target) |
| `(sites-from "belief" 'which 'adherence)` | Results from a prior belief (bootstrapping) |

`(all-func-decls pkgs)` is exported too, but it is the underlying helper, not a
selector: it extracts func-decl nodes (annotated with `pkg-path`) from a package
list, e.g. `(all-func-decls (ctx-pkgs ctx))` inside a `custom` lambda.

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
| `(dominates-call "A" "B")` | `'dominates-all` / `'partial` / `'none` / `'missing` / `'malformed-ssa` | Does some A-block dominate *every* B-block? Multi-site generalization of `ordered`; block-granular |
| `(flows-to-all "A" "B")` | `'flows-all` / `'partial` / `'none` / `'missing` | Does one A-def's value reach every B call site? Value-flow analog of `dominates-call` |
| `(single-call-site "A")` | `'single` / `'multiple` / `'missing` | Exactly one SSA call to A? |
| `(reaches-call "A")` | `'reaches` / `'unreached` / `'unresolved` | Transitively calls A through the call graph (not just the body) |
| `(co-mutated "field" ...)` | `'co-mutated` / `'partial` / `'missing` | Fields stored together? |
| `(checked-before-use "val")` | `'guarded` / `'unguarded` / `'missing` | Value checked before use? Product lattice fixpoint via `(wile algebra)` with early exit. 4-hop def-use reachability |
| `(receiver-parameter-asymmetry)` | `'candidate` / `'forwarder` / `'mutation` / `'accessor` / `'multi-read` / `'unused-recv` / `'interface-method` | SSA: receiver-vs-parameter usage asymmetry |
| `(custom (lambda (site ctx) ...))` | user-defined symbol | Escape hatch |

A checker may return either a bare category symbol or `(symbol . evidence)` where
`evidence = ((where . W) (why . Y) (score . S))`; a bare symbol stays valid (it yields an
unlocated finding). The category alone drives voting; the evidence becomes the per-site
`finding`. Six checkers emit the tail: `ordered` (the two call
positions), `paired-with` (op-a's call site; `unpaired` lands exactly at the
operation needing a pair), `co-mutated` (the first field-store), `checked-before-use`
(the comparison feeding the guard — the `ssa-if` itself carries no position),
`contains-call` (the matched call on `present`; bare `#f` on absent, preserving its
dual-use as a `functions-matching` predicate), and `receiver-parameter-asymmetry`
(the single receiver read on `candidate`; all non-candidate verdicts stay bare).
Verdicts with no resolvable position stay bare symbols
(`'unordered`/`'missing`/`'malformed-ssa`, unlocated `unguarded`).

### Context Accessors

Available in `custom` lambdas:

| Accessor | Description |
|----------|-------------|
| `(make-context target)` | Build a lazy-loading context for a package pattern |
| `(ctx-pkgs ctx)` | Lazy-loaded type-checked packages |
| `(ctx-ssa ctx)` | Lazy-loaded SSA functions |
| `(ctx-callgraph ctx)` | Lazy-loaded call graph |
| `(ctx-field-index ctx)` | Lazy-loaded field access index |
| `(ctx-session ctx)` | The GoSession backing the context |
| `(ctx-find-ssa-func ctx pkg-path name)` | Look up SSA function by package + name |

### Utility Functions

Re-exported for use in `custom` lambdas: `nf`, `tag?`, `walk`, `filter-map`,
`flat-map`, `member?`, `unique` (from `(wile goast utils)`), plus
`string-contains` (index or `#f`) and `string-contains?` (predicate).

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

### Return Shape

`run-beliefs` returns a flat list of self-describing alists:

```scheme
;; Per-site belief
((name . "lock-unlock") (type . per-site) (status . strong)
 (pattern . paired-defer) (ratio . 9/10) (total . 10)
 (adherence . ("pkg.Foo" "pkg.Bar" ...))
 (deviations . (("pkg.Baz" . unpaired) ...))
 (findings . (#<finding> ...))   ;; one located finding per site
 (min-adherence . 0.9) (min-sites . 5)
 (sites-expr . (sites (functions-matching (contains-call "Lock"))))
 (expect-expr . (expect (paired-with "Lock" "Unlock"))))

;; Aggregate belief
((name . "pkg-cohesion") (type . aggregate) (status . ok)
 (sites-expr . (sites (all-functions-in)))
 (analyze-expr . (analyze (single-cluster 'idf-threshold 0.36)))
 (verdict . SPLIT) (confidence . HIGH) ...)
```

Status values: `strong`, `weak`, `no-sites`, `error` (per-site); `ok`, `error` (aggregate).

The `findings` field is *additive* (it sits beside the unchanged `adherence`/`deviations`):
a list of `finding` objects from `(wile goast provenance)`, one per site, each carrying
`value` (the category), `where` (`"file:line:col"` or `#f`), `why` (structured
`(reason-tag . data-alist)`, projected by `render-why`), and `score` (number or `#f`).
The category alone still drives voting; evidence rides alongside it.

### Emit Mode

`emit-beliefs` takes `run-beliefs` output and produces Scheme source code —
`define-belief` forms for strong per-site beliefs, `define-aggregate-belief` forms for
ok aggregates. Closes the discover → review → commit → enforce lifecycle.

```scheme
(define emitted (emit-beliefs (run-beliefs "my/package/...")))
(display emitted)  ;; => (define-belief "lock-unlock" ...)
```

| Export | Description |
|--------|-------------|
| `emit-beliefs` | Format strong/ok belief results as Scheme source code |

### Suppression

Committed beliefs live in `.scm` files; re-running discovery should not resurface a
belief already committed. Matching is structural (`equal?` on captured S-expressions);
names, thresholds, and ratios are ignored.

```scheme
(define results
  (with-belief-scope
    (lambda ()
      ;; ...discovery beliefs...
      (run-beliefs "my/pkg/..."))))
(define committed (load-committed-beliefs "beliefs/"))
(display (emit-beliefs (suppress-known results committed)))
```

| Export | Description |
|--------|-------------|
| `with-belief-scope` | Save/restore `*beliefs*` + `*aggregate-beliefs*` around a thunk via `dynamic-wind`. |
| `load-committed-beliefs` | Load `.scm` beliefs from a directory or single file into an isolated scope; return `(per-site-snapshot . aggregate-snapshot)` pair. Per-file `guard`: skip bad files with stderr warning. |
| `load-beliefs!` | Load `.scm` beliefs from a directory or single file into the **current** scope (activating them for `run-beliefs`), returning the count loaded. Wrap in `with-belief-scope` to confine. |
| `suppress-known` | Structural filter: drop results whose `sites-expr`/`expect-expr` (per-site) or `sites-expr`/`analyze-expr` (aggregate) match any committed tuple. |
| `current-beliefs` | Live snapshot of `*beliefs*`, symmetric to `aggregate-beliefs`. Necessary because user-code `*beliefs*` reads return a stale snapshot under Wile's import semantics. |

### Aggregate Beliefs

Aggregate beliefs evaluate whole-package properties instead of per-site patterns.

```scheme
(define-aggregate-belief "package-cohesion"
  (sites (all-functions-in))
  (analyze (single-cluster 'idf-threshold 0.36)))
```

| Analyzer | Description |
|----------|-------------|
| `(single-cluster . opts)` | Package cohesion via `recommend-split`. Exported by `(wile goast split)`, not by the belief library |
| `(aggregate-custom (lambda (sites ctx) ...))` | Escape hatch: returns a result alist |

---

## Shared Utilities -- `(wile goast utils)`

**Implementation:** Pure Scheme library (embedded in binary)

Traversal utilities for the tagged-alist node format shared by all layers.

| Function | Description |
|----------|-------------|
| `(nf node 'key)` | Get field value by key, or `#f` if absent |
| `(tag? node 'tag)` | Test whether node has a given tag |
| `(walk val visitor)` | Depth-first walk; collect non-`#f` visitor results |
| `(filter pred lst)` | Keep elements satisfying `pred` |
| `(filter-map f lst)` | Map, keeping only non-`#f` results |
| `(flat-map f lst)` | Map (f returns list), concatenate results |
| `(member? x lst)` | Membership test using `equal?` |
| `(unique lst)` | Remove duplicates, preserving order |
| `(has-char? s c)` | Does string `s` contain character `c`? |
| `(string-contains s sub)` | SRFI-13, re-exported: match index or `#f` |
| `(string-contains? s sub)` | Predicate variant: `#t` / `#f` |
| `(string-join lst delim)` | SRFI-13, re-exported |
| `(ordered-pairs lst)` | All unordered pairs from a list (each pair once) |
| `(take lst n)` | First n elements |
| `(drop lst n)` | Drop first n elements |
| `(opt-ref opts key default)` | Look up a keyword option in a `'key value ...` list |
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
| `(ssa-rule-idempotence)` | `x & x -> x`, `x \| x -> x` (integer types only) |
| `(ssa-rule-absorption)` | `x & (x \| y) -> x`, `x \| (x & y) -> x` (integer types only) |
| `(ssa-rule-associativity)` | Right-associate chained operations for canonical form |
| `(ssa-rule-set rule ...)` | Compose rules: first non-`#f` wins |
| `ssa-theory` | Named theory for `discover-equivalences` (all SSA axioms) |
| `ssa-binop-protocol` | Term protocol for SSA binop nodes |

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
| `(classify-ast-diff tag field str-a str-b path)` | AST classifier: `'type` / `'identifier` / `'literal` / `'structural` |
| `(classify-ssa-diff tag field str-a str-b path)` | SSA classifier: `'type` / `'register` / `'structural` |
| `(diff-result-similarity r)` | Extract similarity (0.0--1.0) from diff result |
| `(diff-result-shared r)` | Shared node count |
| `(diff-result-diff-count r)` | Differing node count |
| `(diff-result-diffs r)` | List of `(category path val-a val-b)` entries |
| `(find-root-substitutions pairs)` | Minimal substitution set from `(val-a . val-b)` diff pairs |
| `(collapse-diffs diffs roots)` | Drop diffs derivable from the root substitutions |
| `(score-diffs shared diff-count diffs)` | Compute effective similarity with substitution collapsing |
| `(unifiable? result threshold)` | Verdict: `#t` when effective similarity >= threshold and all remaining diffs are type/register |
| `(ssa-equivalent? node-a node-b)` | Algebraic equivalence via `discover-equivalences`: checks if two SSA nodes share a normal form under any sub-theory |

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

- [LIBRARIES.md](LIBRARIES.md) -- Higher-level analysis libraries (dataflow,
  abstract domains, FCA family, package splitting, deduplication, path algebra,
  provenance) and the source map.
- [MCP.md](MCP.md) -- MCP server: transports, engine model, pipeline tools, prompts.
- [GO-STATIC-ANALYSIS.md](GO-STATIC-ANALYSIS.md) -- Usage guide with
  architecture overview and cross-layer examples.
- [AST-NODES.md](AST-NODES.md) -- Field reference for all 50+ AST node tags.
- [EXAMPLES.md](EXAMPLES.md) -- Annotated walkthroughs of example scripts.
