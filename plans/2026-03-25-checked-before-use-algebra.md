# Migrate checked-before-use to (wile algebra)

**Status:** COMPLETED (2026-03-25). `(wile goast dataflow)` library created, `checked-before-use` migrated to algebraic fixpoint.

## Summary

Replace the hand-rolled Kleene iteration in `checked-before-use` (belief.scm:684-736)
with `fixpoint` over a product lattice from `(wile algebra)`. Extract reusable
def-use reachability primitives into a new `(wile goast dataflow)` library.

## Design decisions

- **Product lattice** for early exit: state is `(name-set, guard-found?)`.
  When `guard-found?` becomes `#t`, transfer returns unchanged → fixpoint
  converges immediately. Scales as hop limit and instruction count grow.
- **New library** `(wile goast dataflow)` — houses reusable pieces, belief.scm
  imports and uses them. Foundation for C2 dataflow framework.
- **Parameterized predicate** — `make-reachability-transfer` takes a `found?`
  predicate, not hardcoded to `ssa-if`. Future checkers reuse the same
  machinery with different predicates.

## Lattice structure

| Component | Lattice | Bottom | Top | Join |
|-----------|---------|--------|-----|------|
| Name set | `powerset-lattice` over instruction names | `()` | all names | set union |
| Guard flag | Boolean lattice | `#f` | `#t` | `or` |

Combined via `product-lattice`. Universe extracted per SSA function.

Transfer function (one pass over instructions):
1. If `guard-found?` is `#t`, return state unchanged (early exit)
2. Find instructions whose operands intersect current name set
3. Collect output names; for `ssa-store`, track operands instead
4. Check if any reached instruction satisfies `found?` predicate
5. Return `(names-join, guard-found? or found-hit?)`

## File layout

```
cmd/wile-goast/lib/wile/goast/dataflow.sld   — library definition
cmd/wile-goast/lib/wile/goast/dataflow.scm   — implementation
```

### Exports

```scheme
(define-library (wile goast dataflow)
  (export
    boolean-lattice
    ssa-instruction-names
    make-reachability-transfer
    defuse-reachable?)
  (import (scheme base)
          (wile algebra)
          (wile goast utils)))
```

### API

```scheme
(boolean-lattice)
;; → lattice of {#f, #t} with or/and/implies

(ssa-instruction-names ssa-fn)
;; → list of all instruction output names + store operands
;; Universe for powerset-lattice construction.

(make-reachability-transfer all-instrs found? universe)
;; → (lambda (state) → state)
;; state = (name-set guard-flag) as product-lattice element.
;; Early exit: if guard-flag is #t, returns state unchanged.

(defuse-reachable? ssa-fn start-names found? fuel)
;; → #t if found? predicate reached, #f if not (or fuel exhausted).
;; Builds lattice, transfer, runs fixpoint.
```

### belief.scm changes

`checked-before-use` becomes ~10 lines:

```scheme
(define (checked-before-use value-pattern)
  (define fuel 4)
  (lambda (site ctx)
    (let* ((ssa-fn (ctx-find-ssa-func ...)))
      (cond
        ((not ssa-fn) 'missing)
        ((defuse-reachable? ssa-fn (list value-pattern)
                            (lambda (i) (tag? i 'ssa-if)) fuel)
         'guarded)
        (else 'unguarded)))))
```

## Behavioral preservation

The migration must produce identical results for all existing belief tests
(category 4 tests in `examples/goast-query/testdata/`). The algorithm is the
same — bounded monotone iteration on (P(Names), ⊆) — just expressed through
the algebra library instead of hand-rolled.

Key equivalences:
- `(append tracked new-names)` → `(lattice-join names-lat tracked new-names)`
- `(null? new-names)` convergence check → `lattice-equal?` in `fixpoint`
- `(> depth max-depth)` → `fixpoint` fuel parameter
- Early `'guarded` exit → product lattice convergence when boolean component hits `#t`
