# Go Static Analysis

Analyze Go code structure using wile-goast's eval tool. Determine the right
analysis layer and compose Scheme expressions to answer structural questions.

## Your Question

{{question}}

## Target Package

{{package}}

## Instructions

### Step 1: Determine the analysis layer

Based on the question, select the appropriate layer:

| Question type | Layer | Import |
|--------------|-------|--------|
| Function structure, AST shape, parsing | AST | `(wile goast)` |
| Data flow, field stores, value tracking | SSA | `(wile goast ssa)` |
| Statement ordering, path enumeration | CFG | `(wile goast cfg)` |
| Who calls what, reachability | Call Graph | `(wile goast callgraph)` |
| Known anti-patterns, standard checks | Lint | `(wile goast lint)` |
| Statistical consistency patterns | Belief DSL | `(wile goast belief)` |

If the question spans multiple layers, compose them in a single expression.

### Step 2: Compose and run the analysis

Use the `eval` tool with a Scheme expression. All analysis runs inside the
wile-goast process — do not read Go source files to answer structural questions.

### Step 3: Interpret and report results

- Translate s-expression output into human-readable findings
- Reference specific file:line locations when position data is available
- Highlight actionable items vs. informational findings
- Always show the Scheme expression you ran (for reproducibility)

## Primitive Reference

### AST — `(import (wile goast))`
- `(go-parse-file path . options)` — parse .go file to s-expression AST
- `(go-parse-string source . options)` — parse Go source string
- `(go-parse-expr source)` — parse single expression
- `(go-format ast)` — convert s-expression AST back to Go source
- `(go-node-type ast)` — return tag symbol of an AST node
- `(go-typecheck-package pattern . options)` — load with type annotations

Options: `'positions` (include file:line:col), `'comments` (include doc/comments)

### SSA — `(import (wile goast ssa))`
- `(go-ssa-build pattern)` — build SSA for a package
- `(go-ssa-field-index ssa-pkg)` — field access index for all functions

### CFG — `(import (wile goast cfg))`
- `(go-cfg pattern func-name)` — build CFG for a named function
- `(go-cfg-dominators cfg)` — build dominator tree
- `(go-cfg-dominates? dom-tree block-a block-b)` — test dominance
- `(go-cfg-paths cfg from to)` — enumerate simple paths between blocks

### Call Graph — `(import (wile goast callgraph))`
- `(go-callgraph pattern . algorithm)` — build call graph (static, cha, rta)
- `(go-callgraph-callers cg func-name)` — incoming edges
- `(go-callgraph-callees cg func-name)` — outgoing edges
- `(go-callgraph-reachable cg func-name)` — transitive reachability

### Lint — `(import (wile goast lint))`
- `(go-analyze pattern analyzer-names...)` — run analysis passes
- `(go-analyze-list)` — list available analyzer names

### Belief DSL — `(import (wile goast belief))`

```scheme
(define-belief "name"
  (sites <selector>)
  (expect <checker>)
  (threshold <ratio> <min-count>))
(run-beliefs "package/pattern/...")
```

Site selectors: `functions-matching`, `callers-of`, `methods-of`, `sites-from`
Predicates: `has-params`, `has-receiver`, `name-matches`, `contains-call`,
  `stores-to-fields`, `all-of`, `any-of`, `none-of`
Property checkers: `contains-call`, `paired-with`, `ordered`, `co-mutated`,
  `checked-before-use`, `custom`

### Utilities — `(import (wile goast utils))`

```scheme
(nf node 'key)           ; field access -> value or #f
(tag? node 'func-decl)   ; tag predicate -> #t or #f
(walk val visitor)        ; depth-first walk, collects non-#f results
(filter-map f lst)        ; map keeping non-#f results
(flat-map f lst)          ; map then concatenate
```

## AST Representation

Go AST nodes are tagged alists: `(tag (key . val) ...)`.
Access fields: `(assoc 'key (cdr node))` or use `nf` from utils.

Key field type rules:
- `name`, `sel`, `label`, `value` (on lit) -> string
- `type` (on composite-lit, func-decl, etc.) -> node or `#f`
- `decls`, `list`, `elts`, `args` -> list of nodes
- `inferred-type`, `obj-pkg` -> string (only with `go-typecheck-package`)

## Common Patterns

Walk all nodes in a package:
```scheme
(import (wile goast utils))
(let ((pkgs (go-typecheck-package "my/package")))
  (for-each
    (lambda (pkg)
      (for-each
        (lambda (file)
          (walk file
            (lambda (node)
              (when (eq? (car node) 'composite-lit)
                (display (nf (nf node 'type) 'name))
                (newline))
              #f)))
        (cdr (assoc 'files (cdr pkg)))))
    pkgs))
```

Find callers of a function:
```scheme
(let ((cg (go-callgraph "my/package" 'cha)))
  (go-callgraph-callers cg "FuncName"))
```

Check dominance between statements:
```scheme
(let* ((cfg (go-cfg "my/package" "FuncName"))
       (dom (go-cfg-dominators cfg)))
  (go-cfg-dominates? dom 0 3))
```
