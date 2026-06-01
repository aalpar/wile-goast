# Extract function

**Name:** extract function (`before → after`). The inverse is **inline function**
(see `inline-function.md`). Use extraction when the same block recurs; inlining
when an abstraction earns nothing.

**Precondition.** Two or more sites contain a **structurally identical** block of
statements (a clone), differing in nothing or only in the values they operate on.

**Transform (`before → after`), steps:**

1. **Confirm the clones are unifiable** — identical action sequence, differences
   limited to substitutable operands.
2. **Lift the block** into a new function parameterized by the differing
   operands.
3. **Replace each clone** with a call.

**What it optimizes / sacrifices.** One definition to maintain instead of N
copies; a bug fixed once. The cost is a layer of indirection and a name to read —
worth it only when the clone is real, not coincidental.

**Prior art in this repo:** `ast-diff` and `unifiable?`
(`lib/wile/goast/unify.scm`) find the clones and verify the only remaining
differences are substitutable; this is the project's core unification move.

---

Before — `saveUser` and `saveOrder` share an identical body:

```go
type T struct{}

func validate(e *T)  {}
func normalize(e *T) {}
func persist(e *T)   {}

func saveUser(u *T) {
	validate(u)
	normalize(u)
	persist(u)
}

func saveOrder(o *T) {
	validate(o)
	normalize(o)
	persist(o)
}
```

After — body extracted to `save`:

```go
type T struct{}

func validate(e *T)  {}
func normalize(e *T) {}
func persist(e *T)   {}

func saveUser(u *T)  { save(u) }
func saveOrder(o *T) { save(o) }

func save(e *T) {
	validate(e)
	normalize(e)
	persist(e)
}
```
