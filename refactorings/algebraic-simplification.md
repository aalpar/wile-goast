# Algebraic simplification

**Name:** algebraic simplification / strength reduction (`before → after`).
There is no inverse worth naming — complicating an expression is obfuscation.

**Precondition.** An expression contains an **algebraic identity, annihilator,
or idempotent** sub-term: `x*1`, `x+0`, `x*0`, `x&x`, `x|x`, `x&(x|y)`, etc.
Identity/annihilation rules are sound only on integer types (IEEE-754 `NaN`/`±0`
break them on floats).

**Transform (`before → after`), steps:**

1. **Match** each sub-term against the rule set (identity, annihilation,
   idempotence, absorption).
2. **Rewrite** to the canonical reduced form and re-fold the surrounding
   expression.

**What it optimizes / sacrifices.** Fewer operations and a canonical form that
makes later passes (CSE, equivalence checks) more effective. Sacrifices nothing
on integers within the rules' type scope; misapplied to floats it is unsound.

**Prior art in this repo:** `ssa-normalize.scm` implements exactly these rules —
`ssa-rule-identity`, `ssa-rule-annihilation`, `ssa-rule-idempotence`,
`ssa-rule-absorption` — all integer-type scoped for this reason.

---

Before — `x*1`, `y*0`, `x&x` left in place:

```go
func reduce(x, y int) int {
	return x*1 + y*0 + (x & x)
}
```

After — identities applied (`x*1→x`, `y*0→0`, `x&x→x`, then `x+x→2*x`):

```go
func reduce(x, y int) int {
	return 2 * x
}
```
