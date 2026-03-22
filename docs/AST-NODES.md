# AST Node Reference

Field types for every node tag produced by the goast mapper. Generated from
`goast/mapper.go`.

## Field Type Legend

| Notation | Meaning |
|----------|---------|
| `string` | Scheme string |
| `symbol` | Scheme symbol |
| `bool` | `#t` or `#f` |
| `node` | A tagged alist (another AST node) |
| `node?` | A tagged alist or `#f` (nil in Go AST) |
| `list` | A Scheme list of nodes |
| `list?` | A Scheme list or `#f` |
| `string?` | A string or `#f` |

### Optional Fields

Fields marked with `†` appear only with `'positions` option.
Fields marked with `‡` appear only with `'comments` option.
Fields marked with `§` appear only with type-checked packages (`go-typecheck-package`).

---

## Top-Level

### `file`
| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Package name |
| `decls` | list | Declarations (func-decl, gen-decl, bad-decl) |
| `comments` ‡ | list? | All comment groups in the file |

---

## Declarations

### `func-decl`
| Field | Type | Description |
|-------|------|-------------|
| `doc` ‡ | list? | Doc comment strings |
| `name` | string | Function name |
| `recv` | list? | Receiver field list (methods) or `#f` (functions) |
| `type` | node (func-type) | Function signature |
| `body` | node? (block) | Function body or `#f` (external) |

### `gen-decl`
| Field | Type | Description |
|-------|------|-------------|
| `doc` ‡ | list? | Doc comment strings |
| `tok` | symbol | `import`, `const`, `type`, or `var` |
| `specs` | list | Spec nodes (import-spec, value-spec, type-spec) |

### `bad-decl`
| Field | Type | Description |
|-------|------|-------------|
| `pos` † | string | Position "file:line:col" |
| `end` † | string | End position |

---

## Specs

### `import-spec`
| Field | Type | Description |
|-------|------|-------------|
| `doc` ‡ | list? | Doc comment strings |
| `name` | string? | Local name or `#f` (default) |
| `path` | node (lit) | Import path literal |
| `comment` ‡ | list? | Line comment |

### `value-spec`
| Field | Type | Description |
|-------|------|-------------|
| `doc` ‡ | list? | Doc comment strings |
| `names` | list | Name strings |
| `type` | node? | Type expression or `#f` |
| `values` | list | Value expressions |
| `comment` ‡ | list? | Line comment |

### `type-spec`
| Field | Type | Description |
|-------|------|-------------|
| `doc` ‡ | list? | Doc comment strings |
| `name` | string | Type name |
| `type` | node | Type expression |
| `comment` ‡ | list? | Line comment |

---

## Statements

### `block`
| Field | Type | Description |
|-------|------|-------------|
| `list` | list | Statement nodes |

### `return-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `results` | list | Result expressions |

### `expr-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `x` | node | Expression |

### `assign-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `lhs` | list | Left-hand side expressions |
| `tok` | symbol | `=`, `:=`, `+=`, etc. |
| `rhs` | list | Right-hand side expressions |

### `if-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `init` | node? | Init statement or `#f` |
| `cond` | node | Condition expression |
| `body` | node (block) | Then block |
| `else` | node? | Else block/if-stmt or `#f` |

### `for-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `init` | node? | Init statement or `#f` |
| `cond` | node? | Condition expression or `#f` |
| `post` | node? | Post statement or `#f` |
| `body` | node (block) | Loop body |

### `range-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `key` | node? | Key variable or `#f` |
| `value` | node? | Value variable or `#f` |
| `tok` | symbol | `=` or `:=` |
| `x` | node | Range expression |
| `body` | node (block) | Loop body |

### `branch-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `tok` | symbol | `break`, `continue`, `goto`, `fallthrough` |
| `label` | string? | Label name or `#f` |

### `decl-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `decl` | node (gen-decl) | Declaration |

### `inc-dec-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `x` | node | Operand expression |
| `tok` | symbol | `++` or `--` |

### `go-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `call` | node (call-expr) | Call expression |

### `defer-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `call` | node (call-expr) | Call expression |

### `send-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `chan` | node | Channel expression |
| `value` | node | Value expression |

### `labeled-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `label` | string | Label name |
| `stmt` | node | Labeled statement |

### `switch-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `init` | node? | Init statement or `#f` |
| `tag` | node? | Tag expression or `#f` |
| `body` | node (block) | Block of case-clauses |

### `type-switch-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `init` | node? | Init statement or `#f` |
| `assign` | node | Type switch guard (assign-stmt or expr-stmt) |
| `body` | node (block) | Block of case-clauses |

### `case-clause`
| Field | Type | Description |
|-------|------|-------------|
| `list` | list? | Case expressions or `#f` (default) |
| `body` | list | Statement nodes |

### `select-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `body` | node (block) | Block of comm-clauses |

### `comm-clause`
| Field | Type | Description |
|-------|------|-------------|
| `comm` | node? | Send/receive statement or `#f` (default) |
| `body` | list | Statement nodes |

### `bad-stmt`
| Field | Type | Description |
|-------|------|-------------|
| `pos` † | string | Position "file:line:col" |
| `end` † | string | End position |

---

## Expressions

### `ident`
| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Identifier name |
| `inferred-type` § | string | Go type string |
| `obj-pkg` § | string | Package path of resolved object |

### `lit`
| Field | Type | Description |
|-------|------|-------------|
| `kind` | symbol | `INT`, `FLOAT`, `IMAG`, `CHAR`, `STRING` |
| `value` | string | Literal text (e.g. `"42"`, `"\"hello\""`) |
| `inferred-type` § | string | Go type string |

### `binary-expr`
| Field | Type | Description |
|-------|------|-------------|
| `op` | symbol | `+`, `-`, `*`, `/`, `==`, `!=`, `<`, `>`, `&&`, `\|\|`, etc. |
| `x` | node | Left operand |
| `y` | node | Right operand |
| `inferred-type` § | string | Result type |

### `unary-expr`
| Field | Type | Description |
|-------|------|-------------|
| `op` | symbol | `&`, `*`, `!`, `-`, `+`, `^`, `<-` |
| `x` | node | Operand |
| `inferred-type` § | string | Result type |

### `call-expr`
| Field | Type | Description |
|-------|------|-------------|
| `fun` | node | Function expression |
| `args` | list | Argument expressions |
| `inferred-type` § | string | Return type |

### `selector-expr`
| Field | Type | Description |
|-------|------|-------------|
| `x` | node | Receiver expression |
| `sel` | string | Selected field/method name |
| `inferred-type` § | string | Result type |

### `index-expr`
| Field | Type | Description |
|-------|------|-------------|
| `x` | node | Indexed expression |
| `index` | node | Index expression |
| `inferred-type` § | string | Element type |

### `index-list-expr`
| Field | Type | Description |
|-------|------|-------------|
| `x` | node | Indexed expression |
| `indices` | list | Index expressions |
| `inferred-type` § | string | Result type |

### `star-expr`
| Field | Type | Description |
|-------|------|-------------|
| `x` | node | Operand |
| `inferred-type` § | string | Result type |

### `paren-expr`
| Field | Type | Description |
|-------|------|-------------|
| `x` | node | Inner expression |
| `inferred-type` § | string | Result type |

### `composite-lit`
| Field | Type | Description |
|-------|------|-------------|
| `type` | node? | Type expression (ident, selector-expr, array-type, map-type, etc.) or `#f` |
| `elts` | list | Element expressions (kv-expr for keyed, or bare expressions) |
| `inferred-type` § | string | Go type string |

### `kv-expr`
| Field | Type | Description |
|-------|------|-------------|
| `key` | node | Key expression |
| `value` | node | Value expression |
| `inferred-type` § | string | Result type |

### `func-lit`
| Field | Type | Description |
|-------|------|-------------|
| `type` | node (func-type) | Function signature |
| `body` | node (block) | Function body |
| `inferred-type` § | string | Function type |

### `type-assert-expr`
| Field | Type | Description |
|-------|------|-------------|
| `x` | node | Expression being asserted |
| `type` | node? | Asserted type or `#f` (type switch `x.(type)`) |
| `inferred-type` § | string | Result type |

### `slice-expr`
| Field | Type | Description |
|-------|------|-------------|
| `x` | node | Sliced expression |
| `low` | node? | Low bound or `#f` |
| `high` | node? | High bound or `#f` |
| `max` | node? | Max capacity or `#f` |
| `slice3` | bool | Three-index slice? |
| `inferred-type` § | string | Result type |

### `ellipsis`
| Field | Type | Description |
|-------|------|-------------|
| `elt` | node? | Element type or `#f` |
| `inferred-type` § | string | Result type |

### `bad-expr`
| Field | Type | Description |
|-------|------|-------------|
| `pos` † | string | Position "file:line:col" |
| `end` † | string | End position |

---

## Type Expressions

### `func-type`
| Field | Type | Description |
|-------|------|-------------|
| `params` | list? | Parameter fields or `#f` |
| `results` | list? | Result fields or `#f` |

### `array-type`
| Field | Type | Description |
|-------|------|-------------|
| `len` | node? | Length expression or `#f` (slice) |
| `elt` | node | Element type |

### `map-type`
| Field | Type | Description |
|-------|------|-------------|
| `key` | node | Key type |
| `value` | node | Value type |

### `struct-type`
| Field | Type | Description |
|-------|------|-------------|
| `fields` | list? | Field list or `#f` |

### `interface-type`
| Field | Type | Description |
|-------|------|-------------|
| `methods` | list? | Method list or `#f` |

### `chan-type`
| Field | Type | Description |
|-------|------|-------------|
| `dir` | symbol | `send`, `recv`, or `both` |
| `value` | node | Element type |

### `field`
| Field | Type | Description |
|-------|------|-------------|
| `doc` ‡ | list? | Doc comment strings |
| `names` | list | Name strings (empty for embedded fields) |
| `type` | node | Type expression |
| `tag` | node? (lit) | Struct tag literal (only when present) |
| `comment` ‡ | list? | Line comment |

---

## Utilities — `(wile goast utils)`

### `(nf node key)` → value or `#f`
Field accessor. `(nf node 'name)` returns the value of the `name` field.

### `(tag? node tag)` → `#t` or `#f`
Tag predicate. `(tag? node 'func-decl)` tests if node is a func-decl.

### `(walk val visitor)` → list
Depth-first walk over AST. Calls `(visitor node)` on each tagged-alist node.
Collects non-`#f` return values. **Note: first argument is the AST value,
second is the visitor function.**

### `(filter-map f lst)` → list
Map keeping only non-`#f` results.

### `(flat-map f lst)` → list
Map then concatenate.
