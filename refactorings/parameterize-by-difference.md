# Parameterize by difference

**Name:** parameterize function / form template method (`before → after`). The
inverse is **specialization** (monomorphizing a general function back into
fixed-value variants for clarity or speed).

**Precondition.** Several functions perform the **same action sequence** and
differ only in **constant data** ("same verbs, not same nouns"). The shape is
identical; only a value varies.

**Transform (`before → after`), steps:**

1. **Diff the clones** and collapse the differences — confirm they reduce to one
   varying operand.
2. **Introduce a parameter** for that operand and keep one body.
3. **Rewrite each original** as a call passing its constant.

**What it optimizes / sacrifices.** Collapses N near-clones to one
implementation. Sacrifices nothing structurally; if the thin wrappers add no
value, delete them and call the parameterized form directly.

**Prior art in this repo:** `score-diffs` with substitution collapsing
(`lib/wile/goast/unify.scm`) is precisely the test for "differ only in
substitutable data" — the signal that a parameter, not a new function, is wanted.

---

Before — identical shape, differing only in the multiplier:

```go
func priceSmall(n int) int { return n * 5 }
func priceLarge(n int) int { return n * 9 }
```

After — the multiplier becomes a parameter:

```go
func priceSmall(n int) int { return price(n, 5) }
func priceLarge(n int) int { return price(n, 9) }

func price(n, unit int) int { return n * unit }
```
