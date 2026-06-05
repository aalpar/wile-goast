# Deduplicate with type parameters

**Name:** deduplicate with type parameters (`before → after`). The inverse —
*monomorphizing* a generic into per-type copies — is occasionally done for
readability or to escape a constraint the type system cannot express, but as a
default it is the duplication this transform removes. This is the type-level twin
of `parameterize-by-difference.md`: that one abstracts over a differing *value*,
this one abstracts over a differing *type*.

**Precondition.** Two or more functions are structurally identical and differ
*only* in the types of their operands and results — an AST diff whose every
remaining difference is classified "type", nothing else. The shared body must
already be type-generic in spirit: every operation it performs (`>`, `+`, …) must
be available on all the types via a single constraint. Here `maxInt` and
`maxFloat` differ only in `int` vs `float64`, and `>` works on both.

**Transform (`before → after`), steps:**

1. **Confirm the diff is type-only.** Run `ast-diff` / `unifiable?`; if any
   non-type difference remains (different operators, different control flow), this
   is parameterize-by-difference or a true distinct function, not this.
2. **Choose the constraint.** The set of operations the body uses dictates it —
   `>` on both operands ⇒ `cmp.Ordered`.
3. **Replace the clones with one generic function,** lifting the differing type
   to a type parameter; delete the monomorphic copies and update call sites
   (Go infers the type argument at most calls).

**What it optimizes / sacrifices.** It collapses N copies into one definition, so
a fix or extension is written once. It sacrifices the zero-cost obviousness of a
concrete signature, and Go's generics carry some compile-time and (occasionally)
runtime cost; for exactly two small clones the win is marginal, for many it is
decisive. Apply when the clone count or the body's size makes the duplication a
real maintenance hazard.

**Detector status:** could-build (Tier A) — `ast-diff` (`(wile goast unify)`)
already separates type differences from structural ones; the un-built piece is
the predicate "this diff is *only* type differences", which reads the existing
classification rather than computing anything new.

---

Before — `maxInt` and `maxFloat` are identical but for the operand type:

```go
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
```

After — one generic function constrained by `cmp.Ordered`:

```go
import "cmp"

func maxOf[T cmp.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}
```
