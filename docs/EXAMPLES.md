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

## 5. inline-expand.scm — AST Transformation

**File:** `examples/goast-query/inline-expand.scm`

Stress-tests AST transformation by inlining function calls. Two phases:

1. **Expression-level inlining.** Single-return functions (e.g. `double(x) = x * 2`)
   are replaced at call sites with their body expression, substituting formals for actuals.
2. **Statement-level inlining.** Multi-statement functions (e.g. `clamp` with early returns)
   are spliced into the caller's block, rewriting `return` statements into assignments.

### Layers Used

- **AST** (`go-parse-string`, `go-format`) — parse, transform, and round-trip to Go source

### Key Techniques

**`ast-transform`** — depth-first pre-order tree rewriter. A function `f` returns a
replacement node or `#f` (keep original, recurse). This script originally implemented
`ast-transform` inline as a missing primitive; it is now provided by `(wile goast utils)`.

**`ast-splice`** — flat-map rewriter for lists. Used to splice multiple statements into a
block where a single statement was.

### How to Run

```bash
wile-goast -f examples/goast-query/inline-expand.scm
```

---

## 6. ssa-unify-detect.scm — SSA Equivalence Pass

**File:** `examples/goast-query/ssa-unify-detect.scm`

Runs the full unification pipeline on test case pairs, measuring similarity at
three stages to isolate each layer's contribution:

1. **AST diff** — raw structural comparison of type-checked ASTs
2. **SSA canonicalized** — after dominator-order blocks and alpha-renamed registers
3. **SSA canonicalized + normalized** — after algebraic normalization rules

### Layers Used

- **AST** (`go-typecheck-package`)
- **SSA** (`go-ssa-build`, `go-ssa-canonicalize`)
- **Unify** (`ast-diff`, `ssa-diff`, `unifiable?`)
- **SSA Normalize** (`ssa-normalize`)

### How to Run

```bash
cd /path/to/wile-goast
wile-goast -f examples/goast-query/ssa-unify-detect.scm
```

---

## 7. belief-validate-categories.scm — Belief Category Validation

**File:** `examples/goast-query/belief-validate-categories.scm`

Validates all four belief categories against synthetic testdata packages with
known deviations. Each category should detect exactly one deviation:

| Category | Belief | Checker | Testdata |
|----------|--------|---------|----------|
| 1. Pairing | lock-unlock | `paired-with` | `testdata/pairing/` |
| 2. Check | err-checked | `checked-before-use` | `testdata/checking/` |
| 3. Handling | dowork-wrap | `contains-call` via `callers-of` | `testdata/handling/` |
| 4. Ordering | validate-process | `ordered` | `testdata/ordering/` |

### Layers Used

- **Belief DSL** — all four checker types
- **AST**, **SSA**, **Call Graph** — loaded lazily by the DSL

### How to Run

```bash
wile-goast -f examples/goast-query/belief-validate-categories.scm
```

---

## etcd Examples

The `examples/etcd/` directory contains real-world belief scripts targeting
[etcd](https://github.com/etcd-io/etcd). Each demonstrates the belief DSL
against a large, production Go codebase.

| Script | Target | What it finds |
|--------|--------|---------------|
| `convention-mine.scm` | `etcd/server` | Statistical call-convention discovery: per receiver type, which callees appear in >= threshold% of methods |
| `lock-beliefs.scm` | `etcd/server` | Lock/Unlock pairing consistency |
| `etcd-beliefs.scm` | `etcd/server` | Storage layer co-mutation patterns |
| `mvcc-beliefs.scm` | `etcd/server` | MVCC package: locking, co-mutation in concurrent watcher/transaction code |
| `raft-dispatch-consistency.scm` | `etcd/server` | Inconsistent use of `raftRequest` vs. `raftRequestOnce` |
| `raft-check-beliefs.scm` | `etcd/raft` | Value-guarding beliefs in raft functions |
| `raft-error-handling.scm` | `etcd/raft` | Consistent error handling across callers of `raft.Step` |
| `raft-storage-consistency.scm` | `etcd/raft` | Behavioral consistency across `raft.Storage` implementations via `interface-methods` |

### How to Run

```bash
cd /path/to/etcd/server   # or etcd/raft for raft-* scripts
wile-goast -f /path/to/examples/etcd/<script>.scm
```

---

## Summary

| Script | Layers | What it demonstrates |
|--------|--------|---------------------|
| `goast-query.scm` | AST | Tagged-alist access, type traversal |
| `unify-detect.scm` | AST | Recursive tree diff, path classification, scoring |
| `unify-detect-pkg.scm` | AST (typed) | Substitution collapsing, module-wide comparison |
| `belief-example.scm` | AST | Declarative belief DSL |
| `inline-expand.scm` | AST | `ast-transform`, `ast-splice`, round-trip inlining |
| `ssa-unify-detect.scm` | AST + SSA + Unify | Three-stage similarity pipeline |
| `belief-validate-categories.scm` | Belief DSL | All four checker categories against synthetic testdata |
| etcd scripts (7) | Belief DSL | Real-world beliefs against etcd (pairing, co-mutation, ordering, conventions) |

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
| `has-char?` | String contains character? |
| `ordered-pairs` | All unordered pairs from a list |
| `take` / `drop` | List slicing |
| `ast-transform` | Depth-first pre-order tree rewriter |
| `ast-splice` | Flat-map rewriter for lists |

The s-expression representation is uniform across all IR layers. The same
traversal patterns work on AST nodes, SSA instructions, CFG blocks, and
call graph edges.
