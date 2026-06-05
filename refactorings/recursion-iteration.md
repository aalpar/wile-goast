# Recursion ⇄ iteration

**Name:** recursion ⇄ iteration. The two directions are duals run forward or
backward: `recursion → iteration` turns a tail/accumulator-recursive function
into a loop; `iteration → recursion` is the reverse, occasionally clearer for
inherently tree-shaped work. This file documents the recursion→iteration
direction as primary, completing the pair with `collapse-mutual-recursion.md`
(which removes a recursion *cycle* but leaves single-function recursion alone).

**Precondition.** A function calls itself in **tail position** — the recursive
call's result is returned directly, with no pending work after it — and threads
its progress through an **accumulator** parameter. This is the *convertible
shape*; non-tail recursion (where the call's result is combined with more
computation) needs an explicit stack instead and is out of scope here. Here `sum`
tail-calls itself, passing `acc + items[0]`.

**Transform (`before → after`), steps:**

1. **Confirm the convertible shape.** The function must be in a non-trivial SCC
   of the call graph (it recurses), *and* the recursive call must be in tail
   position with an accumulator — an AST/SSA shape check, not just "does it
   recurse."
2. **Turn the base case into the loop guard.** `if len(items) == 0 { return acc }`
   becomes `for len(items) > 0 { … }`.
3. **Turn the recursive arguments into loop-body assignments.** Each argument the
   call advanced (`acc+items[0]`, `items[1:]`) becomes a mutation of the
   corresponding variable inside the loop; `return acc` follows the loop.

**What it optimizes / sacrifices.** Go does not guarantee tail-call elimination,
so the iterative form removes unbounded stack growth (no overflow on large
inputs) and the per-call frame overhead. It sacrifices the recursive form's
direct correspondence to an inductive definition — for small, provably-bounded
inputs the recursion may read more clearly, which is why the reverse direction
exists.

**Detector status:** could-build (Tier B) — `path-node-in-cycle?`
(`path-algebra.scm`) detects *that* a function recurses, but recognizing the
*convertible shape* (tail position + accumulator) is a new AST+SSA classifier
with no existing primitive to query. This is the lone Tier-B entry in Pass 1: the
recursion signal exists; the shape recognizer must be built.

---

Before — `sum` is tail-recursive with an `acc` accumulator:

```go
func sum(items []int, acc int) int {
	if len(items) == 0 {
		return acc
	}
	return sum(items[1:], acc+items[0])
}
```

After — base case became the loop guard, recursive arguments became loop-body
assignments; `sum` leaves the SCC:

```go
func sum(items []int, acc int) int {
	for len(items) > 0 {
		acc += items[0]
		items = items[1:]
	}
	return acc
}
```
