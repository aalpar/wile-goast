# FCA Closure Unification

Replace hand-rolled Galois closure in `fca-algebra.scm` with `(wile algebra closure)`.

**Status:** Approved

**Depends on:** `(wile algebra closure)` (wile v1.13+), `(wile goast fca)` (existing)

---

## What changes

`concept-lattice->algebra-lattice` in `cmd/wile-goast/lib/wile/goast/fca-algebra.scm`:

1. **Build closure operator** from the FCA context: `cl(A) = intent(extent(A))` on attribute sets, using `make-closure-operator` from `(wile algebra closure)`. The underlying lattice is a powerset lattice on attributes from the context.

2. **Replace join/meet lambdas** — currently hand-chain `extent` → `intent`. After: `(closure-close cl (set-intersect I1 I2))` for join, `(closure-close cl (set-union I1 I2))` for meet.

3. **Replace top/bottom scan** — currently O(n) scan by extent size. After: `(closure-close cl '())` yields top intent (attributes shared by all objects), `(closure-close cl all-attrs)` yields bottom intent (attributes of objects having all attributes). Look up concepts by intent.

## What stays

- `concept-relationship` — pure intent subset comparison, no algebra overlap
- `annotated-boundary-report` — report formatting, uses `concept-relationship`
- `find-concept-by-intent` — still needed to map closed intents back to concepts
- `concept-summary` — string formatting

## Import changes

```scheme
;; Before
(import (wile goast utils)
        (wile goast fca)
        (wile algebra lattice))

;; After
(import (wile goast utils)
        (wile goast fca)
        (wile algebra lattice)
        (wile algebra closure))
```

## Testing

Existing tests in `goast/fca_algebra_test.go` (5 test functions) must pass unchanged — this is a pure refactoring.
