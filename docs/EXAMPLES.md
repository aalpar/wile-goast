# Example Scripts

Annotated walkthroughs of the example scripts in `examples/goast-query/`.
Each script demonstrates a different analysis technique using the goast
primitives.

Primitives referenced below are documented in
[PRIMITIVES.md](PRIMITIVES.md).

---

## 1. goast-query.scm — Basic AST Queries

**File:** `examples/goast-query/goast-query.scm`

Three progressively richer queries on Go source:

1. Parse a Go source string and extract all function names.
2. Classify functions as exported vs. unexported (uppercase first letter).
3. Type-check a real on-disk package and find functions that return `error`.

### Layers Used

- **AST** (`go-parse-string`, `go-typecheck-package`)

### Key Techniques

**Tagged-alist access.** Every Go AST node is `(tag (key . val) ...)`. The
script defines `node-field` to extract values by key:

```scheme
(define (node-field node key)
  (let ((e (assoc key (cdr node))))
    (if e (cdr e) #f)))
```

**Tag dispatch.** `(eq? (car decl) 'func-decl)` filters declarations by
node type. This is the fundamental pattern across all scripts.

**Type traversal.** `returns-error?` walks the result field list of a
function's type node, checking for `(ident (name . "error"))`. Demonstrates
navigating nested AST: `func-decl -> type -> results -> field -> type -> ident -> name`.

### How to Run

```bash
wile-goast --run goast-query
```

---

## 2. unify-detect.scm — AST Diff Engine

**File:** `examples/goast-query/unify-detect.scm`

Given two Go functions as inline source strings, computes a recursive AST
diff, classifies each difference, and scores the pair for unification fitness.

### Layers Used

- **AST** (`go-parse-string`) — parsing only, no type-checking

### Key Techniques

**Recursive s-expression diff.** The core `ast-diff` function handles four
shapes:

1. Two tagged nodes with the same tag: compare fields pairwise
2. Two tagged nodes with different tags: structural divergence
3. Two lists: element-wise positional comparison
4. Two atoms: if equal, shared; if different, classify the leaf diff

The result is `(shared-count diff-count diffs)` where each diff is
`(category path val-a val-b)`.

**Path-based classification.** Diffs are classified by position in the AST:

| Category | Weight | What it means |
|----------|--------|---------------|
| `identifier` | 0 | Free rename, no parameter needed |
| `type-name` | 1 | One type parameter covers all |
| `literal-value` | 1 | One value parameter per distinct literal |
| `operator` | 2 | Callback or flag parameter |
| `structural` | 100 | Effectively rejects the pair |

### How to Run

```bash
wile-goast --run unify-detect
```

---

## 3. unify-detect-pkg.scm — Module-Wide Unification Detection

**File:** `examples/goast-query/unify-detect-pkg.scm`

The production unification detector. Loads all packages in a Go module,
extracts functions with 3+ statements, groups by signature shape, compares
all cross-package pairs, and reports candidates with substitution collapsing.

### Layers Used

- **AST** (`go-typecheck-package`) — type-checked ASTs with `inferred-type`
  and `obj-pkg` annotations

### Key Techniques

**Signature-shape pre-filtering.** Functions grouped by `(param-count . result-count)`.
Only cross-package pairs within the same group are compared, reducing the
comparison space from O(n^2) to O(sum of k_i^2).

**Substitution collapsing.** Type annotations propagate root substitutions
into every sub-expression. A single root change like `int64` to `uint64`
generates dozens of `inferred-type` diffs. The collapsing algorithm:

1. Sort type-name diff pairs by string length (shortest first)
2. If applying known roots to val-a yields val-b, the pair is derived
3. Reclassify derived diffs as weight-0

This recovers 25+ percentage points of similarity that raw comparison misses.

**Effective similarity.** After collapsing, derived diffs count as shared
nodes. A pair with 73% raw similarity may reach 98% effective similarity
when type propagation is accounted for.

### How to Run

```bash
cd /path/to/go/module
wile-goast --run unify-detect-pkg
```

The script targets `./...` (all packages in the current module).

---

## 4. belief-example.scm — Belief DSL Smoke Test

**File:** `examples/goast-query/belief-example.scm`

A minimal example of the belief DSL. Defines one belief against the
wile-goast codebase itself: functions matching "Prim" should have a body.

```scheme
(import (wile goast belief))

(define-belief "prim-functions-have-body"
  (sites (functions-matching (name-matches "Prim")))
  (expect (custom (lambda (site ctx)
    (if (nf site 'body) 'has-body 'no-body))))
  (threshold 0.90 3))

(run-beliefs "github.com/aalpar/wile-goast/goast")
```

### How to Run

```bash
wile-goast --run belief-example
```

All `Prim*` functions have bodies, so 100% adherence.

---

## Summary

| Script | Layers | What it demonstrates |
|--------|--------|---------------------|
| `goast-query.scm` | AST | Tagged-alist access, type traversal |
| `unify-detect.scm` | AST | Recursive tree diff, path classification, scoring |
| `unify-detect-pkg.scm` | AST (typed) | Substitution collapsing, module-wide comparison |
| `belief-example.scm` | AST | Declarative belief DSL |

### Utility Functions

All scripts share a common vocabulary extracted into `(wile goast utils)`:

| Function | Purpose |
|----------|---------|
| `nf` | Get field value from a tagged alist node |
| `tag?` | Test node tag |
| `walk` | Depth-first traversal, collect non-`#f` results |
| `filter-map` | Map keeping non-`#f` results |
| `flat-map` | Map + concatenate |
| `member?` | List membership via `equal?` |
| `unique` | Deduplicate preserving order |

The s-expression representation is uniform across all IR layers. The same
traversal patterns work on AST nodes, SSA instructions, CFG blocks, and
call graph edges.
