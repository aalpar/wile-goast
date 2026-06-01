# Split loop

**Name:** split loop (`before → after`). The inverse is **loop fusion** (see
`loop-fusion.md`); the two form a dual pair, and which direction wins depends on
whether you're optimizing for separable concerns or for fewer passes.

**Precondition.** One loop body computes **two independent results** that share
nothing but the iteration — two concerns fused only because they happen to walk
the same collection.

**Transform (`before → after`), steps:**

1. **Identify the independent computations** in the body.
2. **Give each its own loop** over the same range.

**What it optimizes / sacrifices.** Each loop does one thing — independently
readable, testable, and parallelizable; later you can move or drop one without
touching the other. Sacrifices a pass over the data: two traversals instead of
one (usually negligible, occasionally not for huge inputs — then fuse).

**Detector status:** could-build — SSA/CFG: a loop whose body's def-use graph
partitions into disjoint components carrying separate results.

---

Before — one loop, two unrelated accumulations:

```go
func process(xs []int) (int, int) {
	total := 0
	count := 0
	for _, x := range xs {
		total += x
		if x < 0 {
			count++
		}
	}
	return total, count
}
```

After — one concern per loop:

```go
func process(xs []int) (int, int) {
	total := 0
	for _, x := range xs {
		total += x
	}
	count := 0
	for _, x := range xs {
		if x < 0 {
			count++
		}
	}
	return total, count
}
```
