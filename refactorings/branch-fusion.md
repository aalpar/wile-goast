# Branch fusion

**Name:** branch fusion / consolidate-duplicate-conditional-fragments (the
`before → after` direction). The inverse is **branch splitting**. This is the
adjacent-guard special case of `pull.md`'s predicate hoist: two `if x` guards on
the *same* value with nothing forcing them apart.

**Precondition.** Two consecutive guards test the **same predicate**, with no
intervening code that depends on the first guard's effects (or none at all).

**Transform (`before → after`), steps:**

1. **Prove the two predicates are equal** (syntactically identical, or
   equivalent under boolean normalization).
2. **Merge the bodies** into one guard, preserving statement order.

**What it optimizes / sacrifices.** One predicate test instead of two, and one
block to read. Sacrifices nothing when the guards are truly adjacent; if code
sat between them it must first be proven movable.

**Prior art in this repo:** `go-cfg-to-structured` guard folding
(`goast/prim_restructure.go`); predicate equality can be discharged with
`boolean-equivalent?` (`lib/wile/goast/boolean-simplify.scm`).

---

Before — same predicate guarded twice:

```go
func apply(x bool, n int) int {
	if x {
		n += 10
	}
	if x {
		n -= 3
	}
	return n
}
```

After — guards fused:

```go
func apply(x bool, n int) int {
	if x {
		n += 10
		n -= 3
	}
	return n
}
```
