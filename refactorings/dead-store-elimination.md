# Dead-store elimination

**Name:** dead-store elimination (and the broader dead-code elimination),
`before → after`. No meaningful inverse — adding a never-read store is just
introducing dead code.

**Precondition.** A value is **assigned but not read** before being overwritten
or before the function returns — the store is *dead*. The computation feeding it
must be free of observable side effects.

**Transform (`before → after`), steps:**

1. **Run liveness** backward; a store whose variable is not live immediately
   after it is dead.
2. **Delete** the dead store (and any now-unreachable computation feeding only
   it).

**What it optimizes / sacrifices.** Removes wasted work and shrinks live ranges.
The one hazard is side effects: if the dead store's right-hand side does I/O or
mutates shared state, deleting it changes behavior — liveness alone is not a
license to drop it.

**Prior art in this repo:** `make-liveness` (`lib/wile/goast/domains.scm`) is the
backward analysis that proves a store dead.

---

Before — `y := 0` is overwritten before any read:

```go
func compute(x int) int {
	y := 0
	y = x + 1
	return y
}
```

After — dead store removed:

```go
func compute(x int) int {
	return x + 1
}
```
