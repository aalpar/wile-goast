# Loop fusion

**Name:** loop fusion / loop jamming (`before → after`). The inverse is **split
loop** (see `split-loop.md`). The pair trades passes against separability.

**Precondition.** Two loops **iterate the same range** with no dependency forcing
them apart — the second does not need the first to have finished the whole range,
only the current element.

**Transform (`before → after`), steps:**

1. **Confirm the bodies are independent per index** (no loop-carried dependency
   from one loop into the other).
2. **Merge** them into a single loop, composing the per-element work.

**What it optimizes / sacrifices.** One pass instead of two — better cache
behavior and fewer bounds checks. Sacrifices the separability that split-loop
buys: the two concerns are now entangled in one body and must be edited together.

**Detector status:** could-build — SSA/CFG: adjacent loops over the same iteration
space with no cross-loop dependency.

---

Before — two passes over `out`:

```go
func transform(xs []int) []int {
	out := make([]int, len(xs))
	for i, x := range xs {
		out[i] = x * 2
	}
	for i := range out {
		out[i]++
	}
	return out
}
```

After — fused into one pass:

```go
func transform(xs []int) []int {
	out := make([]int, len(xs))
	for i, x := range xs {
		out[i] = x*2 + 1
	}
	return out
}
```
