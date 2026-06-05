# Prune provably dead branch

**Name:** prune provably dead branch (`before → after`). No inverse — adding a
branch that can never execute is dead code, not a refactoring.

**Precondition.** A branch condition is decided by the *abstract value* of its
operands on every path that reaches it, even though no literal constant appears
in the source. Abstract interpretation — sign or interval analysis — proves the
condition constant. Here `n := len(items)` has sign `≥ 0` at the test, so
`n < 0` is unsatisfiable. This is strictly stronger than constant-folding:
constant-folding sees no literal `n`, so it cannot fire.

**Transform (`before → after`), steps:**

1. **Run the abstract domain** (`make-sign-analysis` or `make-interval-analysis`)
   to the branch's block and read the lattice value of the condition's operands.
2. **Decide the branch.** If the lattice value makes the condition always-false,
   the then-branch is dead; always-true, the else-branch is dead.
3. **Delete the dead arm** and unwrap the surviving one. With the guard gone, any
   variable that existed only to hold the tested value (here `n`) folds into its
   single use.

**What it optimizes / sacrifices.** It removes a branch the CPU never takes and
the reader must still reason about, and often collapses the scaffolding around it.
It sacrifices a defensive check — if the precondition that makes the branch dead
is ever weakened (e.g. the value later comes from a source that *can* be
negative), the now-deleted guard would have caught it. Prune only when the
abstract proof holds for the value's whole provenance, not just locally.

**Detector status:** could-build (Tier A) — wire a query over
`make-sign-analysis` / `make-interval-analysis` (`domains.scm`) plus the CFG: a
branch whose condition is determined by the block's lattice value. The fixpoint
solver already runs; the un-built piece is the small predicate "this lattice
value decides this branch."

---

Before — `n` is `len(items)`, provably `≥ 0`, so the `n < 0` arm is unreachable:

```go
func count(items []string) string {
	n := len(items)
	if n < 0 {
		return "impossible"
	}
	return describe(n)
}

func describe(n int) string { return "" }
```

After — dead branch pruned; `n` folds into its single use:

```go
func count(items []string) string {
	return describe(len(items))
}
```
