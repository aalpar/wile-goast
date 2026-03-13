# Example Walkthroughs

Annotated walkthroughs of the five example scripts in `examples/goast-query/`.
Each script demonstrates a different level of analysis complexity, from basic AST
queries to module-wide structural comparison with type substitution collapsing.

These scripts are the kind of analysis an AI agent could compose given the
primitive reference. The s-expression representation is uniform across all five
IR layers -- the same `assoc`, `map`, `walk`, and `filter-map` patterns work
everywhere.

Primitives referenced below are documented in
[`docs/GO-STATIC-ANALYSIS.md`](GO-STATIC-ANALYSIS.md). Design rationale for the
unification scripts is in [`plans/UNIFICATION-DETECTION.md`](../plans/UNIFICATION-DETECTION.md).

---

## 1. goast-query.scm -- Basic AST Query

**File:** `examples/goast-query/goast-query.scm`

### Purpose

Three progressively richer queries on Go source:

1. Parse a Go source string and extract all function names.
2. Classify functions as exported vs. unexported (uppercase first letter).
3. Type-check a real on-disk package and find functions that return `error`.

### Layers Used

- **AST** (`go-parse-string`, `go-typecheck-package`) -- queries 1-2 use parsing
  only; query 3 adds type-checking to resolve the `error` interface.

### Key Techniques

**Tagged-alist access.** Every Go AST node is `(tag (key . val) ...)`. The
script defines `node-field` (later scripts shorten this to `nf`) to extract
values by key:

```scheme
(define (node-field node key)
  (let ((e (assoc key (cdr node))))
    (if e (cdr e) #f)))
```

**Tag dispatch.** Checking `(eq? (car decl) 'func-decl)` filters declarations
by AST node type. This is the fundamental pattern: test the tag, then extract
fields.

**Collect-matching.** A `filter-map` over `(node-field file 'decls)` collects
function names from `func-decl` nodes. This pattern -- iterate declarations,
test a predicate, extract a value -- recurs in every later script.

**Type traversal for query 3.** `returns-error?` walks the result field list of
a function's type node, checking whether any result field has type
`(ident (name . "error"))`. This demonstrates navigating nested AST structures:
`func-decl -> type -> results -> field -> type -> ident -> name`.

### How to Run

```bash
./dist/wile-goast -f examples/goast-query/goast-query.scm
```

### Sample Output

```
--- 1. Function names from source string ---
  All:        (Add Sub helper Mul)

--- 2. Exported vs unexported ---
  Exported:   (Add Sub Mul)
  Unexported: (helper)

--- 3. Functions returning error (type-checked package) ---
  Error-returning: (ParseFile ParseString TypecheckPackage)
```

### Without wile-goast

Query 1-2 require writing a Go program that calls `go/parser.ParseFile`, walks
`*ast.File.Decls`, type-switches on `*ast.FuncDecl`, and extracts `Name.Name`.
Perhaps 40-60 lines of Go with imports, error handling, and flag parsing.

Query 3 requires `go/packages.Load` with `NeedTypes`, iterating
`pkg.TypesInfo.Defs`, and checking `*types.Signature.Results()`. The setup
boilerplate alone (load config, error handling, iteration) is typically 80+ lines.

The Scheme version is 117 lines total for all three queries, with zero
boilerplate.

---

## 2. state-trace-detect.scm -- Two-Pass AST Analysis

**File:** `examples/goast-query/state-trace-detect.scm`

### Purpose

Detects split-state patterns in Go code -- conceptually atomic values scattered
across multiple struct fields that are checked piecewise:

- **Pass 1: Boolean clusters.** Structs with 2+ boolean fields. These are
  candidates for replacement with an enum or state machine.
- **Pass 2: If-chain field sweeps.** Cascading `if/else if` chains that check
  multiple fields of the same receiver. These suggest the fields encode a
  single logical value.

### Layers Used

- **AST** (`go-typecheck-package`) -- both passes operate on the typed AST.
  No SSA, CFG, or call graph.

### Key Techniques

**Depth-first `walk`.** This script introduces the generic tree walker that all
later scripts reuse. It distinguishes three shapes in the s-expression tree:

```scheme
(define (walk val visitor)
  (cond
    ((not (pair? val)) '())                    ;; atom: skip
    ((symbol? (car val))                       ;; tagged node: visit + recurse
     (let ((here (visitor val))
           (children (flat-map
                       (lambda (kv)
                         (if (pair? kv) (walk (cdr kv) visitor) '()))
                       (cdr val))))
       (if here (cons here children) children)))
    ((pair? (car val))                         ;; list of nodes: recurse each
     (flat-map (lambda (child) (walk child visitor)) val))
    (else '())))
```

The visitor returns `#f` to skip or a value to collect. Non-`#f` results are
flattened into a list. This single function handles both struct-field scanning
(Pass 1) and if-chain detection (Pass 2).

**If-chain spine traversal.** `if-chain-conditions` follows the `else` field of
nested `if-stmt` nodes, collecting each condition. This recursive pattern:

```scheme
(define (if-chain-conditions node)
  (if (not (tag? node 'if-stmt)) '()
    (cons (nf node 'cond)
          (let ((el (nf node 'else)))
            (if (and el (tag? el 'if-stmt))
              (if-chain-conditions el)
              '())))))
```

**Nested walk for selector extraction.** `selectors-in` runs `walk` on each
condition expression, collecting `(receiver . field)` pairs from
`selector-expr` nodes. A walk inside a walk -- the outer walk finds if-chains,
the inner walk finds field accesses within conditions.

### How to Run

Edit the `target` variable at the top of the script, then:

```bash
./dist/wile-goast -f examples/goast-query/state-trace-detect.scm
```

### Without wile-goast

This requires a custom `go/analysis` pass. You would need to:

1. Register an `analysis.Analyzer` with `Requires: [inspect.Analyzer]`
2. Write an `ast.Inspect` visitor for `*ast.TypeSpec` with `*ast.StructType`
3. Write a separate visitor for `*ast.IfStmt` chains
4. Extract selector expressions via type-switching through `*ast.SelectorExpr`
5. Group by receiver, count, filter, report

The Scheme version is ~180 lines. The Go equivalent is typically 200-300 lines
plus test infrastructure and a `main` driver.

---

## 3. state-trace-full.scm -- Four-Layer Cross-Cutting Analysis

**File:** `examples/goast-query/state-trace-full.scm`

### Purpose

Extends `state-trace-detect.scm` with two additional passes that use SSA and
CFG layers:

| Pass | Layer | Question |
|------|-------|----------|
| 1 | AST | Which structs have 2+ boolean fields? |
| 2 | AST | Which if-chains check multiple fields of the same receiver? |
| 3 | SSA | Are those boolean fields mutated independently across functions? |
| 4 | CFG | Do reads of one field always dominate reads of the other? |

This is the script that no single existing Go tool can replicate -- it
cross-references three separate compiler intermediate representations in a
single analysis.

### Layers Used

- **AST** (`go-typecheck-package`) -- Passes 1 and 2.
- **SSA** (`go-ssa-build`) -- Pass 3.
- **CFG** (`go-cfg`, `go-cfg-dominators`, `go-cfg-dominates?`) -- Pass 4.

### Key Techniques

**SSA instruction matching.** Pass 3 reuses the same `walk` function on SSA
s-expressions (they use the same tagged-alist format). It collects
`ssa-field-addr` instructions (which name the field being addressed) and
`ssa-store` instructions (which name the target register), then correlates them:

```scheme
(define (stored-fields-in-func ssa-func cluster-fields)
  (let* ((field-addrs (collect-field-addrs ssa-func))
         (stores (collect-stores ssa-func))
         (store-addrs (map car stores))
         (stored (filter-map
                   (lambda (fa)
                     (let ((reg (car fa))
                           (field (cadr fa)))
                       (and (member? reg store-addrs)
                            (member? field cluster-fields)
                            field)))
                   field-addrs)))
    (unique stored)))
```

If a function stores to field A but not field B (both in the same boolean
cluster), that is evidence of independent mutation.

**CFG dominance queries.** Pass 4 finds SSA functions that access 2+ cluster
fields, builds each function's CFG, computes the dominator tree, and tests
whether one field's access block dominates the other's:

```scheme
(let* ((cfg (go-cfg target func-name))
       (dom (go-cfg-dominators cfg))
       ...)
  (go-cfg-dominates? dom (cdr fa) (cdr fb))
```

Results are classified as `dominates`, `dominated-by`, `same-block`, or
`no-dominance`. A `dominates` result proves a fixed priority ordering across
every execution path.

**Error guarding.** Pass 4 wraps CFG construction in `(guard (exn (#t #f)) ...)`
because not every SSA function name resolves to a CFG-buildable function (e.g.,
synthetic functions with `$` in the name are filtered out).

### How to Run

```bash
./dist/wile-goast -f examples/goast-query/state-trace-full.scm
```

### Sample Output

(From running against `github.com/aalpar/wile/machine`.)

```
══════════════════════════════════════════════════
  State-Trace: Cross-Layer Split State Detection
══════════════════════════════════════════════════

-- Pass 1: Boolean Clusters (AST) --
  struct ErrExceptionEscape: bool fields (Continuable Handled)
  struct NativeTemplate: bool fields (isVariadic noCopyApply)
  struct opcodeInfo: bool fields (writesValue isBranch)

-- Pass 2: If-Chain Field Sweeps (AST) --
  receiver mc: fields (multiValues singleValue) across 2-branch chain

-- Pass 3: Mutation Independence (SSA) --
  struct NativeTemplate:
    NewForeignClosure stores only: (isVariadic)
    bindRestParameter stores only: (isVariadic)
    NewNativeTemplate stores only: (isVariadic)
    computeNoCopyApply stores only: (noCopyApply)

-- Pass 4: Check Ordering (SSA + CFG) --
  struct NativeTemplate:
    func Copy:
      isVariadic [block 4] -> noCopyApply [block 4]: same-block
  struct ErrExceptionEscape:
    func goErrorToSchemeException:
      Continuable [block 2] -> Handled [block 2]: same-block

-- Summary --
  Boolean clusters:          3
  Field sweep chains:        2
  Independent mutation sites: 4
  Dominance orderings:       2
```

### What Each Layer Contributes

**AST alone** finds struct declarations and if-chain patterns but cannot
determine whether fields are mutated together or separately.

**SSA adds data flow**: it traces `ssa-field-addr` + `ssa-store` instructions
to show that `NativeTemplate.isVariadic` and `NativeTemplate.noCopyApply` are
always stored by different functions.

**CFG adds control flow ordering**: it proves that when both fields are accessed
in the same function, their reads share the same basic block or one dominates
the other.

### Without wile-goast

This analysis requires three separate Go programs or a single monolithic
`go/analysis` pass that:

1. Builds typed ASTs for struct and if-chain detection
2. Constructs SSA via `golang.org/x/tools/go/ssa` and walks basic blocks
   for `FieldAddr` + `Store` correlation
3. Builds CFGs via `golang.org/x/tools/go/cfg` and computes dominator trees

Each of these has its own setup ceremony (`ssa.NewProgram`, `cfg.New`,
dominator computation). The Go implementation would be 500-800 lines plus a
driver and test infrastructure.

The Scheme version is ~410 lines with all four passes, using the same `walk`
utility for AST and SSA traversal and three primitive calls for the CFG layer.

---

## 4. unify-detect.scm -- AST Diff Engine Prototype

**File:** `examples/goast-query/unify-detect.scm`

### Purpose

A prototype for procedure unification detection. Given two Go functions (as
inline source strings), it:

1. Parses both into s-expression ASTs
2. Computes a recursive structural diff
3. Classifies each difference (type name, identifier, literal, operator,
   structural)
4. Scores the pair for unification fitness

The test case compares `pncounter.Increment` vs. `gcounter.Increment` -- two
functions from the crdt project that share the find-own-dot/replace/build-delta
pattern but differ in value type (`int64` vs. `uint64`, `CounterValue` vs.
`GValue`).

### Layers Used

- **AST** (`go-parse-string`) -- parsing only, no type-checking. This is
  intentional: the prototype uses inline source strings and demonstrates that
  the diff engine works on untyped ASTs. The package-level version (script 5)
  adds `go-typecheck-package` for type annotations.

### Key Techniques

**Recursive s-expression diff.** The core of the script is `ast-diff`, a
recursive comparator that handles four shapes:

1. **Two tagged nodes with the same tag:** count a shared node, diff fields
   pairwise via `fields-diff`.
2. **Two tagged nodes with different tags:** structural divergence.
3. **Two lists:** element-wise positional comparison via `list-diff`.
4. **Two atoms:** if equal, shared; if different, classify the leaf diff.

The result is a triple `(shared-count diff-count diffs)` where each diff is
`(category path val-a val-b)`.

**Path-based classification.** String diffs are classified by their position in
the AST. The classifier examines the field path from root to the diff point:

- Field in `type-fields` (type, inferred-type, obj-pkg, signature) at the path
  leaf: `type-name`
- Field named `name` reached through a type-position ancestor: `type-name`
- Field in `identifier-fields` (name, sel, label): `identifier`
- Field named `value`: `literal-value`
- Field named `tok`: `operator`

This heuristic works because Go AST nodes encode positional semantics in their
field names.

**Weighted scoring.** Each diff category has a weight:

| Category | Weight | Meaning |
|----------|--------|---------|
| `identifier` | 0 | Free rename, no parameter needed |
| `type-name` | 1 | One type parameter covers all |
| `literal-value` | 1 | One value parameter per distinct literal |
| `operator` | 2 | Callback or flag parameter |
| `structural` | 100 | Effectively rejects the pair |

The score also counts distinct type parameter substitutions and value parameter
substitutions needed.

**Full-file comparison.** After the targeted `Increment` comparison, the script
also finds all function pairs that share a name across both files and diffs each
pair. This previews the exhaustive comparison that the package-level script
performs.

### How to Run

```bash
./dist/wile-goast -f examples/goast-query/unify-detect.scm
```

### Without wile-goast

Implementing this in Go requires:

1. Parsing both sources with `go/parser`
2. Writing a recursive `ast.Node` comparator with type-switch dispatch over
   every AST node type (~40 cases for declarations, statements, expressions,
   types)
3. Building a path tracker
4. Classifying diffs by node position
5. Scoring and reporting

The Go AST has ~70 node types. A generic tree diff must handle each one
explicitly because Go's `ast.Node` interface does not provide generic field
access -- you must type-switch to `*ast.FuncDecl`, `*ast.IfStmt`, etc. and
enumerate fields by hand.

The s-expression representation collapses all 70 node types into a single
shape: `(tag (key . val) ...)`. The diff engine handles this with one
recursive function. This is the core value proposition of wile-goast for
analysis scripts.

---

## 5. unify-detect-pkg.scm -- Module-Wide Unification with Substitution Collapsing

**File:** `examples/goast-query/unify-detect-pkg.scm`

### Purpose

The production version of the unification detector. It:

1. Loads all packages matching a Go `go list` pattern (default: `./...`)
   with full type annotations via `go-typecheck-package`
2. Extracts all functions with 3+ statements
3. Groups functions by signature shape (param-count, return-count) to avoid
   O(n^2) full comparison
4. Compares all cross-package function pairs within each shape group
5. Collapses derived type diffs into root substitutions
6. Reports candidates above a configurable similarity threshold (default: 60%)

### Layers Used

- **AST** (`go-typecheck-package`) -- type-checked ASTs with `inferred-type`
  and `obj-pkg` annotations on identifiers. These annotations are critical
  for distinguishing type-name diffs from identifier diffs and for enabling
  substitution collapsing.

### Key Techniques

**Signature-shape pre-filtering.** Functions are grouped by
`(param-count . result-count)`. Only cross-package pairs within the same shape
group are compared. This reduces the comparison space from O(n^2) to
O(sum of k_i^2) where k_i is the size of each shape group.

```scheme
(define (signature-shape func)
  (let* ((ftype (nf func 'type))
         (params (nf ftype 'params))
         (results (nf ftype 'results))
         (pc (if (and params (pair? params)) (length params) 0))
         (rc (if (and results (pair? results)) (length results) 0)))
    (cons pc rc)))
```

**Substitution collapsing.** This is the major innovation over `unify-detect.scm`.
Type annotations from `go-typecheck-package` propagate root type substitutions
into every sub-expression. A single root change like `CounterValue` to `GValue`
generates dozens of `inferred-type` diffs throughout the function body.

The collapsing algorithm:

1. Collect all `(val-a . val-b)` pairs from `type-name` diffs
2. Sort by string length (shortest first)
3. Iterate: if applying known root substitutions to `val-a` yields `val-b`,
   the pair is derived. Otherwise, it is a new root.
4. Reclassify derived diffs as `derived-type` (weight 0)

```scheme
(define (find-root-substitutions pairs)
  (let ((sorted (sort-by-length (unique pairs))))
    (let loop ((ps sorted) (roots '()))
      (if (null? ps) roots
        (let ((a (caar ps)) (b (cdar ps)))
          (if (derivable? a b roots)
            (loop (cdr ps) roots)
            (loop (cdr ps) (cons (cons a b) roots))))))))
```

**Effective similarity.** After collapsing, derived diffs are counted as shared
nodes, producing an "effective similarity" that reflects the true structural
overlap. For `pncounter.Increment` vs. `gcounter.Increment`: raw similarity is
73.6%, effective similarity is 97.9% (92 derived type diffs collapsed to 3 root
substitutions).

**Minimum body size.** Functions with fewer than 3 statements are skipped. Small
functions (getters, trivial constructors) produce false positives because
trivially similar code is not a meaningful unification target.

### How to Run

```bash
cd /path/to/go/module
wile-goast -f /path/to/unify-detect-pkg.scm
```

The script uses `./...` as the default target, loading all packages in the
current module.

### Validation Results

Run against the crdt module (17 packages, 132 functions with 3+ statements),
the script found exactly 4 zero-cost candidates out of 399 pairs above the
60% threshold:

| Eff. Sim. | Cost | Root Params | Pair |
|-----------|------|-------------|------|
| 99.4% | 0 | `ewflag`->`dwflag`, `EWFlag`->`DWFlag` | `ewflag.Enable` vs. `dwflag.Disable` |
| 99.3% | 0 | `ewflag`->`dwflag`, `EWFlag`->`DWFlag` | `ewflag.Disable` vs. `dwflag.Enable` |
| 97.9% | 0 | `pncounter`->`gcounter`, `CounterValue`->`GValue`, `int64`->`uint64` | `pncounter.Increment` vs. `gcounter.Increment` |
| 97.6% | 0 | (same 3 roots) | `pncounter.Value` vs. `gcounter.Value` |

The ewflag/dwflag duality was discovered mechanically -- it was previously
documented only in prose. The pncounter/gcounter pattern was already known
and cross-referenced via NOTE comments in the source.

The remaining 395 pairs all had weighted cost >= 100 (structural diffs,
missing elements, cross-domain mismatches). The weighted-cost filter cleanly
separates signal from noise.

### Without wile-goast

A Go implementation of this analysis would require:

1. `go/packages.Load` with `NeedTypes` + `NeedSyntax` for all packages
2. A recursive `ast.Node` comparator with type-switch dispatch over ~70 node
   types
3. Access to `types.Info` for type annotations on identifiers
4. A substitution-collapsing algorithm operating on Go type strings
5. Signature-shape grouping, cross-package enumeration, scoring, reporting

The s-expression representation eliminates the 70-case type switch entirely.
The substitution collapsing operates on plain strings because type annotations
are already flattened into the `inferred-type` field. The entire script is
~560 lines of Scheme.

The Go equivalent -- with proper error handling, test infrastructure, and the
recursive comparator -- would be 1000-1500 lines. More importantly, modifying
the diff engine (adding a new classification category, changing the scoring
weights, adjusting the collapsing heuristic) requires recompiling. The Scheme
version is edited and re-run in seconds.

---

## Progression Summary

| Script | Lines | Layers | Complexity | What it demonstrates |
|--------|-------|--------|-----------|---------------------|
| `goast-query.scm` | 117 | AST | Single-pass extraction | Tagged-alist access, `assoc`-based field lookup |
| `state-trace-detect.scm` | 182 | AST | Multi-pass pattern detection | Generic `walk`, nested walkers, if-chain spine traversal |
| `state-trace-full.scm` | 411 | AST + SSA + CFG | Cross-layer correlation | SSA instruction matching, CFG dominance queries, same `walk` across IRs |
| `unify-detect.scm` | 518 | AST | Recursive tree diff | s-expression structural comparison, path-based classification, weighted scoring |
| `unify-detect-pkg.scm` | 563 | AST (typed) | Module-wide analysis | Substitution collapsing, signature-shape grouping, threshold filtering |

Each script builds on patterns from the previous one. The utility functions
(`nf`, `tag?`, `walk`, `filter-map`, `flat-map`) are copy-pasted between
scripts -- they are small enough that a shared library is not necessary, and
having them inline makes each script self-contained.

The scripts are composable primitives for an AI agent: given the reference for
`go-typecheck-package`, `go-ssa-build`, `go-cfg`, and the tagged-alist node
format, an LLM can generate analysis scripts in the same style. The uniform
s-expression representation means one traversal pattern covers all five IR
layers.
