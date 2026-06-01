# Inline function

**Name:** inline function (`before → after`). The inverse is **extract function**
(see `extract-function.md`). Inline when an abstraction has one caller and earns
nothing; extract when a block recurs.

**Precondition.** A function is **called from exactly one site** (or is a trivial
pass-through) and its name adds no explanatory value the call site lacks.

**Transform (`before → after`), steps:**

1. **Confirm the callee has a single caller** (or that inlining every caller is
   intended).
2. **Substitute the body** at the call site, renaming parameters to arguments.
3. **Delete** the now-unreferenced function.

**What it optimizes / sacrifices.** Removes an indirection and a name to chase.
Sacrifices a reuse point — inline only when you're confident the abstraction
isn't earning its keep; over-inlining produces long, repetitive functions.

**Prior art in this repo:** `go-callgraph-callers` (`(wile goast callgraph)`)
reports the incoming edges; a single caller is the precondition signal. The
inverse direction is `ast-diff`-driven extraction.

---

Before — `double` has one caller and adds nothing:

```go
func handle(n int) int {
	return double(n) + 1
}

func double(n int) int {
	return n * 2
}
```

After — inlined:

```go
func handle(n int) int {
	return n*2 + 1
}
```
