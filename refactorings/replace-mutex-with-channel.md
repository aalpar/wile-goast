# Replace mutex with channel

**Name:** replace mutex with channel (`before → after`) — "share memory by
communicating." The inverse, **replace channel with mutex**, is equally valid and
often *simpler* for plain shared state; this pair is a genuine trade-off, not a
one-way improvement.

**Precondition.** Shared state is guarded by a mutex, and you want a single owner
that serializes access through messages — useful when access is naturally
request/response, or the state must coordinate with other events via `select`.

**Transform (`before → after`), steps:**

1. **Give the state to one goroutine** that owns it exclusively.
2. **Replace direct field access** with messages on channels (a request channel,
   a reply channel).
3. **Serialize** reads and writes in that goroutine's `select` loop.

**What it optimizes / sacrifices.** No locks; the state has one owner and can
coordinate with timeouts/cancellation via `select`. Sacrifices simplicity and
speed for the plain case — a `sync.Mutex` around a counter is shorter, faster, and
clearer than a goroutine-plus-channels machine. Pick by access pattern, not
fashion.

**Detector status:** out-of-scope — choosing between the two needs happens-before
reasoning the tool lacks. **`go build` / `go vet` prove well-formedness, not
deadlock or race freedom; the owner goroutine below also runs forever (a leak) —
illustrative, not production-ready.**

---

Before — mutex-guarded counter:

```go
import "sync"

type counter struct {
	mu sync.Mutex
	n  int
}

func (c *counter) inc() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}

func (c *counter) get() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}
```

After — state owned by one goroutine, accessed by messages:

```go
type counter struct {
	bump chan struct{}
	read chan int
}

func newCounter() *counter {
	c := &counter{bump: make(chan struct{}), read: make(chan int)}
	go func() {
		n := 0
		for {
			select {
			case <-c.bump:
				n++
			case c.read <- n:
			}
		}
	}()
	return c
}

func (c *counter) inc()     { c.bump <- struct{}{} }
func (c *counter) get() int { return <-c.read }
```
