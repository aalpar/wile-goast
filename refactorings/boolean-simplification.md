# Boolean simplification

**Name:** boolean simplification (`before → after`). The inverse is **expansion**
(rewriting to a fuller normal form such as DNF/CNF) — useful for analysis, not
for reading.

**Precondition.** A boolean expression is **reducible** under the laws of Boolean
algebra: absorption, complement, idempotence, involution, De Morgan.

**Transform (`before → after`), steps:**

1. **Normalize** the expression with the Boolean-algebra theory.
2. **Replace** the original with its normal form when the form is strictly
   simpler.

**What it optimizes / sacrifices.** Fewer operators and operands, and a canonical
form for equivalence checks. The risk is intent: `(a && b) || (a && !b)` may have
documented the two cases on purpose — collapsing to `a` is correct but discards
that the author was reasoning about `b`.

**Prior art in this repo:** `boolean-normalize` and `boolean-equivalent?`
(`lib/wile/goast/boolean-simplify.scm`) implement exactly this, over a Boolean
algebra theory built on `(wile algebra symbolic)`.

---

Before — `b` cancels out:

```go
func keep(a, b bool) bool {
	return (a && b) || (a && !b)
}
```

After — normalized:

```go
func keep(a, b bool) bool {
	return a
}
```
