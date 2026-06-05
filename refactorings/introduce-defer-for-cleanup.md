# Introduce defer for cleanup

**Name:** introduce defer for cleanup (`before → after`), the pair-acquire-release
direction. The inverse — replacing a `defer` with manual cleanup on each path — is
occasionally forced (cleanup that must happen *before* the function returns, or be
conditional), but as a default it is the fragility this transform removes. This is
the catalog's **lifecycle / time** axis, and the most direct consumer of the
belief DSL's flagship `paired-with` checker.

**Precondition.** A resource is acquired once (`Lock`, `Open`, `Add`) and released
by a manual call repeated on every return path. The repetition is the hazard: a
return added later silently skips the release. The `paired-with` checker detects
the acquire/release pairing and flags sites where the release is by hand rather
than deferred. Here `update` calls `mu.Unlock()` on two paths.

**Transform (`before → after`), steps:**

1. **Find the manual pairing.** `paired-with "Lock" "Unlock"` (or Open/Close,
   Add/Done) returns `paired-call` rather than `paired-defer` for the site.
2. **Place a single `defer release()`** immediately after the successful acquire.
3. **Delete every manual release.** Each return now runs the deferred release on
   the way out — including return paths added in the future.

**What it optimizes / sacrifices.** It guarantees the release on every path,
including error paths and ones not yet written, and collapses N release calls to
one. It sacrifices fine control over *when* the release runs: `defer` fires at
function exit, so if the lock must be held for less than the whole remaining body,
keep the release manual (or scope the locked region into its own function). It
also adds the small per-call `defer` overhead, rarely material.

**Detector status:** existing (partial) — the `paired-with` belief checker
(`(wile goast belief)`) detects the acquire/release pairing and distinguishes
`paired-defer` from manual `paired-call`. The checker emits the site; turning it
into the rewrite is the human's step.

---

Before — `Unlock` is repeated on each return path (a new early return forgets it):

```go
import (
	"errors"
	"sync"
)

var mu sync.Mutex
var errBad = errors.New("bad")

func update(ok bool) error {
	mu.Lock()
	if !ok {
		mu.Unlock()
		return errBad
	}
	mu.Unlock()
	return nil
}
```

After — one `defer mu.Unlock()` covers every path:

```go
func update(ok bool) error {
	mu.Lock()
	defer mu.Unlock()
	if !ok {
		return errBad
	}
	return nil
}
```
