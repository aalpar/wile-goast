# Transformation Primitives Design

**Status:** COMPLETED. B1 (Scheme utils), B2a (Case 1), B2b (Case 2), B3 (improvements) all implemented.

## Summary

Add three primitives for AST transformation: `ast-transform` and `ast-splice`
as Scheme library functions in `(wile goast utils)`, and `go-cfg-to-structured`
as a Go primitive in `goast/` that restructures control flow into single-exit
form.

## Motivation

wile-goast's existing primitives are query-oriented: parse, type-check, walk,
collect. The belief DSL composes these for read-only analysis. But refactoring
operations — inlining, function extraction, code motion, goto elimination —
require **transformation**: producing new ASTs from existing ones.

An inlining exercise (`examples/goast-query/inline-expand.scm`) measured the
cost of transformation without dedicated primitives:

| Component | Lines | Issue |
|-----------|-------|-------|
| `ast-transform` (reimplemented) | 15 | Every transformation script pays this |
| `ast-splice` (manual take/drop) | 20 | Block surgery via index arithmetic |
| Control flow restructuring | 80 | Wrong result — early returns not handled |
| **Total** | **~150** | **Broken output** |

With the proposed primitives, the same inlining script becomes ~40 lines with
correct output. The token cost reduction is 3-4x, and the correctness gap
closes entirely.

### Design principle

Token count for a given analysis is the feedback signal for whether primitives
are at the right level. These three primitives target the specific token cost
spikes observed in the inlining exercise.

## Part 1: `ast-transform` (Scheme)

**Location:** `cmd/wile-goast/lib/wile/goast/utils.scm`

```scheme
(ast-transform node f) → new-node
```

Depth-first pre-order tree rewriter over tagged-alist s-expressions.

- `f` returns a node → replace (no recursion into replacement)
- `f` returns `#f` → keep original, recurse into children

Traversal follows the same three-way dispatch as `walk`:

1. Tagged alist `(tag (k . v) ...)` → call `f`, if `#f` recurse into field values
2. List of children → recurse into each element
3. Atom → pass through

### No recursion into replacements

If `f` returns a node, that node is final. This prevents infinite loops when a
transformer always matches. For recursive transformation, the user calls
`ast-transform` on the replacement explicitly:

```scheme
(ast-transform node
  (lambda (n)
    (let ((r (try-rewrite n)))
      (and r (ast-transform r f)))))  ;; explicit recursion
```

### Implementation

~15 lines. Mirrors the structure of `walk` but produces a new tree instead of
collecting results.

## Part 2: `ast-splice` (Scheme)

**Location:** `cmd/wile-goast/lib/wile/goast/utils.scm`

```scheme
(ast-splice lst f) → new-list
```

Flat-mapping rewriter for statement/declaration lists. `f` is called on each
element.

- `f` returns a list → splice those elements in place of the original
- `f` returns `#f` → keep the original element

```scheme
;; Expand one assign-stmt into three inlined statements
(ast-splice (nf block 'list)
  (lambda (stmt)
    (and (should-inline? stmt)
         (list new-stmt-1 new-stmt-2 new-stmt-3))))
```

### Relationship to ast-transform

`ast-transform` is 1:1 (one node in, one out). `ast-splice` is 1:N (one
element in, zero or more out). They are independent — `ast-splice` operates
on flat lists, not trees. A combined rewriter that does both can be built on
top by using `ast-splice` when processing list-valued fields inside
`ast-transform`.

### Implementation

~10 lines. Essentially `flat-map` with a fallback for non-matched elements.

## Part 3: `go-cfg-to-structured` (Go primitive)

**Location:** `goast/` package (base package — operates on s-expressions via
the existing bidirectional mapper, no SSA/callgraph dependency).

```scheme
(go-cfg-to-structured block-sexpr) → block-sexpr | #f
```

Takes a block s-expression. Returns a restructured block where all control
flow has a single exit point — no early returns. The last statement in the
returned block is the sole `return-stmt`. Returns `#f` if the block contains
control flow it cannot restructure.

### Why s-expression-based (not name-based)

Existing CFG primitives (`go-cfg`, `go-cfg-dominators`) take `(pkg-path
func-name)` and load the package from disk. But transformation pipelines
operate on in-memory s-expressions that may have already been modified. A
primitive that only works on on-disk code defeats the purpose of in-memory
transformation.

The Go implementation:

1. Unmaps the block s-expression to `ast.BlockStmt` (existing `unmapNode`)
2. Analyzes the statement list for early-return patterns
3. Restructures into single-exit form
4. Maps the result back to s-expression (existing mapper)

### Case 1: Linear with early returns

The common Go pattern — guard clauses followed by a main path.

```go
// Input                        // Output
if x < lo {                     if x < lo {
    return lo                       return lo
}                               } else if x > hi {
if x > hi {                         return hi
    return hi                   } else {
}                                   return x
return x                        }
```

Algorithm: right-fold over the statement list. When encountering
`if <cond> { ... return }` with no else branch, nest the accumulated remaining
statements as the else branch. Non-if statements and the final return
accumulate as "rest."

The result is a single if/else tree with `return` only at the leaves.

### Case 2: Early returns inside loops

```go
// Input                        // Output
for _, v := range items {       var _r0 T
    if v.bad {                  var _done bool
        return errBad           for _, v := range items {
    }                               if v.bad {
}                                       _r0 = errBad
return nil                              _done = true
                                        break
                                    }
                                }
                                if !_done {
                                    _r0 = nil
                                }
                                return _r0
```

When the primitive detects a `return` inside a `for`/`range` body:

- Introduces result variable (`_r0`) and done flag (`_done`)
- Rewrites `return X` as `_r0 = X; _done = true; break`
- After the loop, wraps remaining statements in `if !_done { ... }`
- Final statement is `return _r0`

Fresh names use `_r0`, `_r1`, ... prefix (unlikely to collide with user code).

### Failure mode

Returns `#f` when the block contains:

- `goto` statements or labeled branches to outer scopes
- Control flow the primitive cannot restructure

The caller checks and falls back:

```scheme
(let ((structured (go-cfg-to-structured body)))
  (or structured body))  ;; use original if restructuring fails
```

### Not handled (deferred — see TODO.md)

- `goto` / labeled branches (returns `#f`)
- `switch`/`select` with early returns inside cases
- Multiple return values (`_r0`, `_r1`, ... — straightforward extension)

## Composition: Inlining pipeline

With all three primitives, the full inlining workflow:

```scheme
;; Phase 1: inline single-expression callees (ast-transform)
(define phase1 (ast-transform target
  (lambda (n) (try-inline-expr n file))))

;; Phase 2: inline multi-statement callees (ast-splice + go-cfg-to-structured)
(define phase2-body
  (ast-splice (nf (nf phase1 'body) 'list)
    (lambda (stmt)
      (let* ((call (extract-assign-call stmt))
             (callee (and call (resolve-callee call file)))
             (target-var (and call (extract-lhs-name stmt))))
        (and callee
             (let* ((structured (go-cfg-to-structured (nf callee 'body)))
                    (substituted (subst-idents structured param-map))
                    (rewritten (ast-transform substituted
                                 (lambda (n)
                                   (and (tag? n 'return-stmt)
                                        (make-assign target-var
                                                     (nf n 'results)))))))
               (nf rewritten 'list)))))))
```

Helper functions (`extract-assign-call`, `resolve-callee`, `make-assign`,
`try-inline-expr`) are each ~5 lines, using existing `nf`, `tag?`, and the
new `ast-transform`.

### Token cost with new primitives

| Component | Lines |
|-----------|-------|
| Helpers (extract-call, resolve, make-assign) | 20 |
| Phase 1 (expression-level inline) | 5 |
| Phase 2 (statement-level inline) | 15 |
| **Total** | **~40** |

Down from ~150 lines (broken) to ~40 lines (correct). 3-4x compression.

## Other use cases

| Use case | Primitives used |
|----------|----------------|
| Goto elimination | `go-cfg-to-structured` + `go-format` |
| Function extraction | `walk` (find free vars) + `ast-splice` (cut) + `ast-transform` (build func-decl) |
| Code instrumentation | `ast-transform` (wrap calls in logging) |
| Desugar patterns | `ast-transform` (rewrite idioms) |
| Early-return cleanup | `go-cfg-to-structured` + `go-format` |

## Implementation notes

- `ast-transform` and `ast-splice` are pure Scheme — add to existing
  `utils.scm`, export from `utils.sld`. Also add `take` and `drop`.
- `go-cfg-to-structured` lives in `goast/` (not `goastcfg/`) because it
  operates on s-expressions via the mapper/unmapper, not on SSA CFG data.
  It may internally build a lightweight CFG from the Go AST for case 2
  analysis, but this is an implementation detail.
- The unmapper already handles all statement types needed for round-tripping.
  No new unmapper code expected for cases 1 and 2.
