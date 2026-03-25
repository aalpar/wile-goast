# SSA Equivalence Pass — Design

**Status**: Approved
**Date**: 2026-03-24
**Answers**: `plans/UNIFICATION-DETECTION.md` open question — "Does SSA normalization collapse enough to be useful?"

## Goal

Add an SSA-level comparison pass to the unification detection pipeline. The pass runs as a refinement on AST candidates, producing a "unifiable" verdict when all remaining differences are type substitutions. This turns a similarity suggestion into a proof.

## Pipeline

```
go-ssa-build → filter (AST candidates) → go-ssa-canonicalize → ssa-normalize → ssa-diff → score → verdict
```

The first two steps exist (`unify-detect-pkg.scm`). This design adds the last four.

## 1. Go Primitive: `go-ssa-canonicalize`

### Signature

```scheme
(go-ssa-canonicalize ssa-func-sexp)  →  ssa-func-sexp
```

Takes a single SSA function s-expression (from `go-ssa-build`) and returns a new s-expression with canonical block order and alpha-renamed registers.

### Block canonicalization

1. Parse the `blocks` list from the SSA function s-expression.
2. Build the dominator tree from `idom` fields (already present in mapper output).
3. Pre-order DFS of the dominator tree produces a canonical block sequence.
4. Reindex blocks: first in dominator order becomes 0, second becomes 1, etc.
5. Update all cross-references: `preds`, `succs`, `idom`, phi edges, jump/if targets.

### Register alpha-renaming

1. Walk blocks in canonical order, instructions in sequence.
2. Maintain a counter and a map: `old-name → new-name`.
3. Parameters get `p0`, `p1`, ... (order preserved from signature).
4. First new definition gets `r0`, next `r1`, etc.
5. Replace all register references (operand fields, phi edges, etc.).

### Implementation

`goastssa/prim_canonicalize.go` — operates on `values.Value` s-expressions, not raw Go SSA. Reads the alist structure, reindexes, builds a new s-expression.

### What it does NOT do

- No algebraic simplification (Scheme layer).
- No type normalization (existing substitution collapsing handles this).
- No cross-function comparison (diff engine).

## 2. Scheme Library: `(wile goast ssa-normalize)`

### Location

`cmd/wile-goast/lib/wile/goast/ssa-normalize.sld` + `ssa-normalize.scm`, embedded alongside belief and utils libraries.

### API

```scheme
(import (wile goast ssa-normalize))

;; Apply all default rules
(ssa-normalize func-sexp)  →  func-sexp

;; Apply a custom rule set
(ssa-normalize func-sexp my-rules)  →  func-sexp

;; Build a rule set from individual rules
(ssa-rule-set rule ...)  →  rule-set

;; Rule constructors
(ssa-rule-identity)       ;; x + 0 → x, x * 1 → x, x | 0 → x, x & ~0 → x
(ssa-rule-commutative)    ;; sort operands of +, *, &, |, ^ by register name
(ssa-rule-annihilation)   ;; x * 0 → 0, x & 0 → 0
(ssa-rule-double-neg)     ;; !!x → x, --x → x
```

### Rule protocol

Each rule is `(lambda (node) ...)` compatible with `ast-transform`. Returns a replacement node or `#f` to keep the original. `ssa-rule-set` composes rules — tries each in order, first non-`#f` wins.

### Commutative sort

Normalizes operand order for commutative SSA ops (`+`, `*`, `&`, `|`, `^`, `==`, `!=`). After alpha-renaming, `min(x,y)` goes to the `x` field, `max(x,y)` to `y`. This eliminates ordering differences from source-level variable introduction order.

### Identity elimination

Recognizes SSA constant names with type suffixes (`"0:int"`, `"0:uint64"`) via a `constant-zero?` predicate. Replaces `(ssa-binop (op . +) (x . reg) (y . "0:int"))` with a reference to `reg`.

### Extensibility

Users add rules the same way:

```scheme
(define my-rules
  (ssa-rule-set
    (ssa-rule-identity)
    (ssa-rule-commutative)
    (lambda (node)
      ;; custom: normalize nil checks to canonical form
      ...)))
```

### What it does NOT do

- No dead code elimination (would change block structure).
- No cross-block optimizations.

## 3. SSA Diff Integration and Scoring

### Diff engine

Reuse the existing `ast-diff` / `fields-diff` / `list-diff` recursive comparator from `unify-detect-pkg.scm`. SSA s-expressions are tagged alists — same format, different tags.

### SSA classification

| SSA field | Category | Weight |
|-----------|----------|--------|
| `name` (register) | `register` | 0 (identical after alpha-rename) |
| `type` | `type-name` | 1 |
| `op` | `operator` | 2 |
| `func` (call target) | `call-target` | 3 |
| `field` | `field-name` | 1 |
| `index`, `preds`, `succs` | `structural` | 0 (identical after canonicalization) |

New category `normalized-away` (weight 0) for instructions eliminated by algebraic rules where one function has the instruction and the other doesn't.

### Substitution collapsing

Works as-is. SSA type strings (`"int64"`, `"*Counter"`, `"map[Dot]GValue"`) get the same root-substitution treatment as AST type strings.

### Scoring

```
effective_similarity = (shared + derived + normalized) / total
```

### Verdict

A candidate is **unifiable** when:
- SSA effective similarity >= threshold (e.g., 0.95)
- All remaining diffs are `type-name` or `derived-type`
- Zero `operator`, `structural`, or `call-target` diffs

Otherwise: **similar but not unifiable**, with differing positions flagged.

### Shared library

Extract the diff engine and scoring from `unify-detect-pkg.scm` into `(wile goast unify)` so both AST-only and AST+SSA scripts can use it.

## 4. Validation

### Test case 1: pncounter.Increment vs gcounter.Increment (positive)

Already embedded in `unify-detect.scm`. Known AST result: ~0.92 similarity, 2 type params.

**Success**: SSA similarity > AST similarity. Verdict: unifiable with 2 type params.

### Test case 2: Semantically different functions (negative)

Two functions with same signature shape but different loop mechanics (range vs index iteration).

**Success**: SSA similarity <= AST similarity, or verdict "not unifiable" with operator/structural diffs.

### Test case 3: Algebraic equivalence (synthetic)

One function has `x + 0`, the other just uses `x`. AST sees structural difference. SSA normalization collapses it.

**Success**: SSA detects equivalence that AST cannot.

### Output format

```
  Pair                          AST sim  SSA sim  Verdict
  pncounter.Inc / gcounter.Inc  0.92     0.97     unifiable (2 type params)
  rangeLoop / indexLoop         0.85     0.72     not unifiable (operator diffs)
  withIdentity / withoutId      0.78     1.00     unifiable (0 params)
```

### Answering the open question

If SSA similarity is consistently higher than AST similarity for true clones and the verdict correctly classifies: SSA normalization adds value. If type differences still dominate (SSA sim ~ AST sim): the answer is "not enough" and further investment goes toward dataflow fingerprinting (Approach B) or acceptance that AST-level is sufficient.

## Deliverables

| Artifact | Location |
|----------|----------|
| Go primitive | `goastssa/prim_canonicalize.go` |
| Scheme normalize lib | `cmd/wile-goast/lib/wile/goast/ssa-normalize.{sld,scm}` |
| Scheme unify lib | `cmd/wile-goast/lib/wile/goast/unify.{sld,scm}` |
| Validation script | `examples/goast-query/ssa-unify-detect.scm` |
| Tests | `goastssa/prim_canonicalize_test.go` |

## Out of scope

- Sub-tree matching (fragment detection within functions)
- CFG isomorphism as a standalone tool
- Call graph context pre-filtering
- Belief DSL integration
- `--emit` mode for the unification detector
