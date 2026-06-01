# Narrow lock scope

**Name:** narrow lock scope (`before → after`). The inverse is **widen lock
scope** — extending a critical section, occasionally needed to make a multi-step
update atomic.

**Precondition.** A lock is **held across work that does not touch the shared
state** it protects (I/O, logging, pure computation), serializing operations that
needn't be.

**Transform (`before → after`), steps:**

1. **Identify the minimal span** that actually reads or writes shared state.
2. **Release the lock** immediately after that span — replacing a
   function-scoped `defer Unlock` with an explicit `Unlock` where safe.
3. **Move lock-independent work** outside the critical section.

**What it optimizes / sacrifices.** Shortens contention, raising concurrency.
Sacrifices the simplicity and panic-safety of `defer Unlock`: an explicit unlock
must be reasoned about on every path, and over-narrowing can split what needed to
be one atomic section — a correctness bug, not a style nit.

**Detector status:** out-of-scope (partial) — the lock/unlock *pairing* belief
exists (Engler-style deviation), but proving which statements touch the guarded
state needs lock-span + alias analysis the tool doesn't have. **`go build` /
`go vet` confirm the snippet is well-formed Go; they do not prove the narrowed
section is still atomic — that is a human judgment.**

---

Before — lock held over `logAccess`, which is independent of the shared map:

```go
import "sync"

type cache struct {
	mu sync.Mutex
	m  map[string]int
}

func (c *cache) get(k string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.m[k]
	logAccess(k) // holds the lock for no reason
	return v
}
```

After — critical section covers only the shared read:

```go
import "sync"

type cache struct {
	mu sync.Mutex
	m  map[string]int
}

func (c *cache) get(k string) int {
	c.mu.Lock()
	v := c.m[k]
	c.mu.Unlock()
	logAccess(k) // outside the lock
	return v
}
```
