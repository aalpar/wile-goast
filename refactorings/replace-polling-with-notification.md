# Replace polling with notification

**Name:** replace polling with notification (`before → after`). The inverse —
polling instead of blocking — is a regression except in narrow cases (e.g. a
frame loop that must run regardless of events).

**Precondition.** A goroutine **spins in a loop checking a condition** with a
sleep between checks, burning CPU and adding latency between the event and its
observation.

**Transform (`before → after`), steps:**

1. **Replace the polled flag** with a channel (or `sync.Cond`).
2. **Block** on a receive instead of looping.
3. The producer **signals** — sends, or closes the channel — when the condition
   becomes true.

**What it optimizes / sacrifices.** Zero CPU while waiting and immediate wakeup —
no sleep-interval latency. Sacrifices the polling loop's forgiving nature: the
producer and consumer must now agree on a signaling protocol (a `close` for
one-shot readiness, buffered sends for counts), and a missed signal can hang the
receiver.

**Detector status:** out-of-scope — recognizing a poll loop and synthesizing the
signaling protocol needs a concurrency model the tool doesn't have. **`go build`
/ `go vet` prove well-formedness; correct signaling (who closes `ready`) is a
human concern.**

---

Before — busy-poll an atomic flag:

```go
import (
	"sync/atomic"
	"time"
)

func waitReady(ready *int32) {
	for atomic.LoadInt32(ready) == 0 {
		time.Sleep(time.Millisecond)
	}
}
```

After — block until signaled:

```go
func waitReady(ready <-chan struct{}) {
	<-ready
}
```
