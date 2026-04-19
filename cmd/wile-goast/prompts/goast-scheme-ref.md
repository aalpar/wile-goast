# Wile Scheme Reference for wile-goast

Quick reference for writing correct Scheme when using the wile-goast eval tool.
Load this prompt before writing non-trivial Scheme expressions.

## Base Environment — Available Primitives

### List operations
`cons`, `car`, `cdr`, `list`, `pair?`, `null?`, `length`, `append`, `reverse`,
`map`, `for-each`, `filter-map`, `assoc`, `member`, `fold`

**`fold` is SRFI-1 style:** `(fold f seed list)` where f is `(lambda (elem acc) ...)`.

### String operations
`string-append`, `string-length`, `string-ref`, `string-set!`, `substring`,
`string=?`, `string<?`, `string>?`, `string<=?`, `string>=?`,
`number->string`, `string->number`, `symbol->string`, `string->symbol`,
`string-copy`, `string->list`, `list->string`

### Hashtable operations
`make-hashtable`, `hashtable-set!`, `hashtable-ref`

**That's it.** No `hashtable->alist`, `hashtable-keys`, `hashtable-values`,
`hashtable-delete!`, `hashtable-contains?`, or `hashtable-size`.

### Numeric
`+`, `-`, `*`, `/`, `=`, `<`, `>`, `<=`, `>=`, `zero?`, `positive?`,
`negative?`, `abs`, `min`, `max`, `modulo`, `remainder`, `quotient`,
`floor`, `ceiling`, `truncate`, `round`, `exact->inexact`, `inexact->exact`

### Boolean / control
`if`, `cond`, `case`, `and`, `or`, `not`, `when`, `unless`,
`begin`, `let`, `let*`, `letrec`, `define`, `set!`, `values`,
`call-with-values`, `dynamic-wind`

### I/O
`display`, `newline`, `write`, `read`

### Predicates
`boolean?`, `number?`, `string?`, `symbol?`, `pair?`, `null?`,
`procedure?`, `vector?`, `char?`, `eq?`, `eqv?`, `equal?`

### Vectors
`make-vector`, `vector-ref`, `vector-set!`, `vector-length`,
`vector->list`, `list->vector`, `vector`

## NOT Available — Common Pitfalls

| You might write | Status | Workaround |
|-----------------|--------|------------|
| `filter` | **Missing** | `(filter-map (lambda (x) (and (pred x) x)) lst)` |
| `fold-left` | **Missing** | `fold` (SRFI-1) or named-let |
| `fold-right` | **Missing** | Named-let with accumulator |
| `list-sort` / `sort` | **Missing** | Manual sort or restructure to avoid |
| `string-contains` | **Missing** | Manual search via `string-ref` loop |
| `string-prefix?` / `string-suffix?` | **Missing** | `substring` + `string=?` |
| `hashtable->alist` | **Missing** | No workaround — track keys separately |
| `hashtable-keys` | **Missing** | No workaround — track keys separately |
| `hashtable-contains?` | **Missing** | `(hashtable-ref ht key #f)` and check |
| `hashtable-delete!` | **Missing** | No workaround |
| `hashtable-size` | **Missing** | No workaround — count externally |
| `string->list` on large strings | **Slow** | Character-at-a-time via `string-ref` |
| `(import (scheme base))` | **Unnecessary** | Bindings are built-in |

## Call Depth Limit

Wile has a **10,000 call depth limit**. This affects:

- **`map`** — not tail-recursive. Overflows on lists > ~8k elements.
- **`filter-map`** — same issue.
- **Recursive DFS** — overflows on large graphs.

**Workaround for large lists:** Use `for-each` with mutation + `reverse`:
```scheme
;; Safe map for large lists
(define (map* f lst)
  (let ((acc '()))
    (for-each (lambda (x) (set! acc (cons (f x) acc))) lst)
    (reverse acc)))

;; Safe filter-map for large lists
(define (filter-map* f lst)
  (let ((acc '()))
    (for-each (lambda (x) (let ((v (f x))) (if v (set! acc (cons v acc))))) lst)
    (reverse acc)))
```

## Type Conventions for goast Primitives

| Primitive | Argument types |
|-----------|---------------|
| `go-callgraph` | session-or-string, **symbol** (`'static`, `'cha`, `'rta`, `'vta`) |
| `go-ssa-build` | session-or-string |
| `go-cfg` | session-or-string, **string** (function name) |
| `go-analyze` | session-or-string, **list of strings** (analyzer names) |
| `go-parse-file` | **string** (file path) |
| `go-parse-string` | **string** (Go source code) |
| `field-index->context` | index, **symbol** (`'write-only`, `'read-write`, `'type-only`) |

All session-sharing primitives accept either a pattern string or a GoSession from `go-load`.

## Library Exports

### (wile goast utils)
`nf`, `tag?`, `walk`, `filter-map`, `flat-map`, `member?`, `unique`,
`has-char?`, `ordered-pairs`, `take`, `drop`, `ast-transform`, `ast-splice`

**`nf`** (node-field): `(nf node 'key)` — shorthand for `(cdr (assoc 'key (cdr node)))`.
Must `(import (wile goast utils))` to use it — other libraries import it internally
but do not re-export it.

### (wile goast fca)
`make-context`, `context-from-alist`, `context-objects`, `context-attributes`,
`field-index->context`, `intent`, `extent`, `concept-lattice`, `concept-extent`,
`concept-intent`, `cross-boundary-concepts`, `boundary-report`,
`propagate-field-writes`,
`set-intersect`, `set-member?`, `set-add`, `set-before`, `set-union`, `set-subset?`

### (wile goast belief)
`define-belief`, `run-beliefs`, `reset-beliefs!`, `*beliefs*`,
`make-context`, `ctx-pkgs`, `ctx-ssa`, `ctx-callgraph`, `ctx-find-ssa-func`, `ctx-field-index`,
`functions-matching`, `callers-of`, `methods-of`, `sites-from`, `all-func-decls`,
`implementors-of`, `interface-methods`,
`has-params`, `has-receiver`, `name-matches`, `contains-call`, `stores-to-fields`,
`all-of`, `any-of`, `none-of`,
`paired-with`, `ordered`, `co-mutated`, `checked-before-use`, `custom`

Also re-exports from utils: `nf`, `tag?`, `walk`, `filter-map`, `flat-map`, `member?`, `unique`

### (wile goast dataflow)
`boolean-lattice`, `ssa-all-instrs`, `ssa-instruction-names`,
`make-reachability-transfer`, `defuse-reachable?`,
`block-instrs`, `run-analysis`, `analysis-in`, `analysis-out`, `analysis-states`

### (wile goast unify)
`tree-diff`, `ast-diff`, `ssa-diff`,
`classify-ast-diff`, `classify-ssa-diff`,
`diff-result-similarity`, `diff-result-diffs`, `diff-result-shared`, `diff-result-diff-count`,
`score-diffs`, `find-root-substitutions`, `collapse-diffs`, `unifiable?`, `ssa-equivalent?`

### (wile goast ssa-normalize)
`ssa-normalize`, `ssa-rule-set`, `ssa-rule-identity`, `ssa-rule-commutative`,
`ssa-rule-annihilation`, `ssa-theory`, `ssa-binop-protocol`

### (wile goast fca-algebra)
`concept-lattice->algebra-lattice`, `annotated-boundary-report`, `concept-relationship`

### (wile goast boolean-simplify)
`boolean-normalize`, `boolean-equivalent?`, `selector->symbolic`, `ast-condition->symbolic`

### (wile goast path-algebra)
`make-path-analysis`, `path-analysis?`, `path-query`, `path-query-all`

### (wile goast domains)
`go-concrete-eval`,
`make-reaching-definitions`, `make-liveness`, `make-constant-propagation`,
`sign-lattice`, `make-sign-analysis`, `interval-lattice`, `make-interval-analysis`

## Wile-Specific Gotchas

1. **Named-let loop variable shadowing:** Don't reuse the same name in a `let*` binding
   inside a named-let body if you also `set!` it. Use distinct names.

2. **SSA names ≠ AST names:** SSA uses `(*Type).Method`, AST uses `Method`.
   The SSA index normalizes via `ssa-short-name` in the belief DSL.

3. **`nf` on missing keys:** Returns `#f`, not an error. Check for `#f` if the key
   might not exist.

4. **Empty field lists:** `(nf summary 'fields)` may return `()` (empty list) or a list
   of field-access nodes. Always guard: `(if (pair? fields) fields '())`.
