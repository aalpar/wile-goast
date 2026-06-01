# Confine shared state to a goroutine

**Name:** confine shared state to a goroutine (`before → after`). The inverse is
**share via lock** — exposing the state behind a mutex again, sometimes simpler
when many readers need synchronous access.

**Precondition.** State is **mutated from multiple goroutines under a lock**, but
the mutation is fire-and-forget (no caller needs the result synchronously). The
lock exists only because the state is shared.

**Transform (`before → after`), steps:**

1. **Make the state local** to a single owner goroutine.
2. **Feed it through a channel** — senders hand off values; the owner is the only
   writer.
3. The lock disappears because the state is no longer shared.

**What it optimizes / sacrifices.** Eliminates the lock and the data race it
guarded against, by construction: one writer, no sharing. Sacrifices synchronous
access (callers can't read the state back without an added request path) and adds
a goroutine and channel to reason about and shut down.

**Detector status:** out-of-scope — needs escape/ownership analysis to prove the
state stops being shared. **`go build` / `go vet` confirm well-formedness only;
channel lifecycle (who closes `in`, when the owner exits) is the reviewer's
responsibility.**

---

Before — shared slice mutated under a lock:

```go
import "sync"

type collector struct {
	mu   sync.Mutex
	data []int
}

func (c *collector) add(x int) {
	c.mu.Lock()
	c.data = append(c.data, x)
	c.mu.Unlock()
}
```

After — one owner goroutine, no lock:

```go
type collector struct {
	in chan int
}

func newCollector() *collector {
	c := &collector{in: make(chan int)}
	go func() {
		var data []int
		for x := range c.in {
			data = append(data, x)
		}
	}()
	return c
}

func (c *collector) add(x int) { c.in <- x }
```
