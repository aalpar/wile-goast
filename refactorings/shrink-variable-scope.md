# Shrink variable scope

**Name:** shrink variable scope (`before → after`). The inverse — widening a
declaration to an enclosing scope — is occasionally forced (a variable that must
outlive a loop), but as a default it is the anti-pattern this transform reverses.

**Precondition.** A variable is declared in a scope strictly larger than its
*live range* — the span from its first real use to its last. Liveness analysis
exposes the gap: the declaration sits above code that does not yet need it, and
sometimes the entire live range is confined to a single inner block. Here `tmp`
is declared at function top but is live only on the `flag == true` path, so the
`compute(items)` call runs even when its result is discarded.

**Transform (`before → after`), steps:**

1. **Compute liveness** (`make-liveness`) and locate the variable's first use.
2. **Find the tightest scope** containing the whole live range — often an inner
   `if`/`for` block rather than the function body.
3. **Slide the declaration down** to that scope (combining declaration with first
   assignment via `:=`). Any initializing call moves with it, so the work happens
   only where the value is consumed.

**What it optimizes / sacrifices.** It narrows the variable's visibility (fewer
ways to misuse it later) and, when the live range is inside a conditional, makes
the initializing computation pay only on the path that uses it. It sacrifices
nothing semantically; the only caution is a computation with side effects that
other paths depended on running — there, moving it changes behavior, so the live
range alone is not sufficient license.

**Detector status:** could-build (Tier A) — wire a query over `make-liveness`
(`domains.scm`): a declaration site earlier than the start of the live range ⇒
slide the declaration to first use. The backward liveness fixpoint already runs;
the un-built piece is comparing the declaration position to the live-range start.

---

Before — `tmp` computed unconditionally, but live only when `flag` is true:

```go
func handle(items []int, flag bool) int {
	tmp := compute(items)
	if flag {
		return tmp + 1
	}
	return 0
}

func compute(items []int) int { return len(items) }
```

After — declaration slid into the only block where `tmp` is live; `compute`
runs only when its result is used:

```go
func handle(items []int, flag bool) int {
	if flag {
		tmp := compute(items)
		return tmp + 1
	}
	return 0
}
```
