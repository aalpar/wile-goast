# Re-roll loop

**Name:** re-roll loop / replace-hand-unrolled-loop-with-loop (`before → after`).
The inverse is **loop unrolling**, a performance transform a compiler applies —
hand-unrolling in source is the anti-pattern this undoes.

**Precondition.** A sequence of statements is a **hand-unrolled loop**: repeated
blocks differing only in a running index or offset.

**Transform (`before → after`), steps:**

1. **Detect the repetition** — adjacent statements identical up to a single
   incrementing operand.
2. **Re-roll** into a loop over the index range.

**What it optimizes / sacrifices.** Source shrinks from N copies to one body, and
the bound becomes data (change `len` once, not N edits). Sacrifices the marginal
speed of unrolling — which belongs to the compiler, not the source.

**Prior art in this repo:** `ast-diff` (`lib/wile/goast/unify.scm`) detects the
"repeated blocks differing only in data" pattern the project's refactoring rules
explicitly flag as a loop waiting to be found.

---

Before — four hand-written accumulations:

```go
func sum4(a [4]int) int {
	s := 0
	s += a[0]
	s += a[1]
	s += a[2]
	s += a[3]
	return s
}
```

After — one loop over the index:

```go
func sum4(a [4]int) int {
	s := 0
	for i := 0; i < len(a); i++ {
		s += a[i]
	}
	return s
}
```
