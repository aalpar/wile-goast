# Loop unswitching

**Name:** loop unswitching (the `before → after` direction). The inverse —
sinking a loop-invariant branch back inside the loop — has no common name; it is
simply the undo. Closely related to inverse if-conversion (`pull.md`): same
predicate-hoisting move, but the duplicated body is a *loop* rather than a
straight-line tail.

**Precondition.** A loop contains a branch on a predicate that is **invariant**
across all iterations (`flag` never changes in the loop body). The branch is
re-evaluated every iteration even though its value is fixed.

**Transform (`before → after`), steps:**

1. **Hoist the invariant branch** out of the loop to a single `if/else`.
2. **Duplicate the loop** into each arm, specialized to one side of the branch.

**What it optimizes / sacrifices.** Removes `n` redundant predicate tests (one
per iteration) and lets each loop body be optimized independently (e.g.
vectorized). Costs code size: the loop is duplicated, and the bodies can drift
if later edited carelessly.

**Prior art in this repo:** loop and branch structure come from `go-cfg`; the
guard-folding phase of `go-cfg-to-structured` (`goast/prim_restructure.go`)
performs the same hoist-a-guard move at block granularity.

---

Supporting declarations (shared by both versions):

```go
func fast(i int) {}
func slow(i int) {}
```

Before — predicate re-tested every iteration:

```go
func run(n int, flag bool) {
	for i := 0; i < n; i++ {
		if flag {
			fast(i)
		} else {
			slow(i)
		}
	}
}
```

After — predicate tested once, loop specialized per arm:

```go
func run(n int, flag bool) {
	if flag {
		for i := 0; i < n; i++ {
			fast(i)
		}
	} else {
		for i := 0; i < n; i++ {
			slow(i)
		}
	}
}
```
