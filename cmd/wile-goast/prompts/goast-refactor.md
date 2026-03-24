# Analysis-Backed Refactoring

Use wile-goast to find unification candidates (duplicate/near-duplicate functions)
and verify refactoring correctness via structural analysis.

## Goal

{{goal}}

## Target Package

{{package}}

## Phase 1: Detect unification candidates

Use the eval tool to run the built-in unification detection:

```scheme
;; For package-wide scan:
(begin
  (import (wile goast) (wile goast utils))
  ;; Load and run unify-detect-pkg logic
  )
```

Or for specific function pairs:
1. Parse both functions to s-expression ASTs
2. Run recursive AST diff
3. Score similarity (shared nodes, diff categories, weighted cost)
4. Identify parameterizable differences (type params, value params)

**Interpreting scores:**
- `similarity > 0.80` with `weighted-cost < 10` — strong unification candidate
- `param-count <= 3` — feasible to parameterize
- `structural` diffs — different control flow, unlikely to unify
- `identifier` diffs — free renames, don't count against unification
- `type-name` diffs — become type parameters or interface constraints
- `literal-value` diffs — become value parameters

## Phase 2: Plan the unification

If unification is feasible:
1. Identify the unified function signature (original + type/value params)
2. Use call graph to find ALL call sites of both functions:
   ```scheme
   (let ((cg (go-callgraph "package" 'cha)))
     (append (go-callgraph-callers cg "FuncA")
             (go-callgraph-callers cg "FuncB")))
   ```
3. Determine if unification reduces total complexity (fewer lines, fewer
   concepts) or merely compresses code at the cost of indirection

## Phase 3: Verify after refactoring

After the refactoring is applied:
1. Run call graph analysis to confirm all call sites reference the unified
   function
2. Run beliefs (if `.goast-beliefs/` exists) to confirm no consistency patterns
   were broken
3. Run lint passes on changed packages:
   ```scheme
   (go-analyze "package/..." "nilness" "unusedresult" "shadow")
   ```

## Rules

- Always show the diff scores and your interpretation before suggesting a merge
- Don't suggest unification when the weighted cost is high (structural diffs)
- Don't suggest unification when it would add more parameters than it removes
  lines — that's compression, not simplification
- Verify by substitution: can every call site use the unified function?
