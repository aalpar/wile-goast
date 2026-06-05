# Replace mutex counter with atomic

**Name:** replace mutex counter with atomic (`before → after`). The inverse —
widening an atomic back to a mutex — is correct once the protected region grows
past a single word and must be updated together with other state; this transform
applies only while the mutex guards exactly one scalar.

**Precondition.** A struct's only shared mutable state is a single integer (or
pointer/bool) guarded by a mutex used for nothing else: every critical section is
one read, one write, or one increment of that one word. The mutex is then pure
overhead — `sync/atomic` provides the same guarantee lock-free. Here `Counter`'s
`mu` protects only `n`.

**Transform (`before → after`), steps:**

1. **Confirm the lock guards exactly one word** — no critical section reads or
   writes a *second* field, and no invariant ties the counter to other state. (This
   is the step the tool cannot fully verify; see the caveat.)
2. **Replace the field with its atomic type** (`int64` + `sync.Mutex` →
   `atomic.Int64`) and drop the mutex.
3. **Rewrite each critical section as the matching atomic op:** `Lock; n++; Unlock`
   → `n.Add(1)`; `Lock; return n; Unlock` → `return n.Load()`.

**What it optimizes / sacrifices.** It removes lock contention and the mutex field,
making increments and reads lock-free and faster under concurrency. It sacrifices
compound atomicity: a mutex can guard *several* operations as one critical section,
an atomic cannot. The moment a second piece of state must change together with the
counter, the atomic is wrong and the mutex must return.

**Detector status:** out-of-scope — deciding that a lock guards *only* this one
word, with no cross-field invariant, needs happens-before / lock-coverage
reasoning the tool lacks (the same limit as `replace-mutex-with-channel`). **`go
build` / `go vet` prove the result is well-formed Go, not that the conversion
preserves the program's concurrency invariants.**

---

Before — a mutex guarding a single counter word:

```go
import "sync"

type Counter struct {
	mu sync.Mutex
	n  int64
}

func (c *Counter) Inc() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}

func (c *Counter) Load() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}
```

After — the word becomes an `atomic.Int64`; the mutex disappears:

```go
import "sync/atomic"

type Counter struct {
	n atomic.Int64
}

func (c *Counter) Inc() {
	c.n.Add(1)
}

func (c *Counter) Load() int64 {
	return c.n.Load()
}
```
