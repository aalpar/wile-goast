# Replace sentinel-string with typed error

**Name:** replace sentinel-string comparison with a typed/sentinel error
(`before → after`). No useful inverse — matching on `err.Error()` strings is
fragile by nature.

**Precondition.** Error identity is conveyed by a **string compared at the call
site** (`err.Error() == "not found"`), so any reword of the message silently
breaks callers, and wrapped errors don't match at all.

**Transform (`before → after`), steps:**

1. **Declare a sentinel** `var ErrX = errors.New(...)` (or a typed error for
   structured detail).
2. **Return the sentinel** instead of an ad-hoc `errors.New`.
3. **Match with `errors.Is`** (sentinel) or `errors.As` (typed), which see
   through `%w` wrapping.

**What it optimizes / sacrifices.** Error identity is stable under rewording and
survives wrapping; callers depend on a value, not prose. Sacrifices nothing — this
is close to a strict improvement, and matches this project's error policy (public
APIs → `errors.As` typed; internal → `errors.Is` sentinels). The related
**consolidate-error-handling / wrap-at-boundary** move belongs here too: wrap with
`%w` and context at package boundaries rather than re-checking strings downstream.

**Detector status:** existing (partial) — `go vet`'s `errorsas` analyzer and
related lint passes flag misuse; finding `err.Error()` string comparisons is a
lint-level AST query.

---

Before — fragile string comparison:

```go
import "errors"

func find(id int) error {
	if id < 0 {
		return errors.New("not found")
	}
	return nil
}

func handle(id int) bool {
	err := find(id)
	return err != nil && err.Error() == "not found"
}
```

After — sentinel matched with `errors.Is`:

```go
import "errors"

var ErrNotFound = errors.New("not found")

func find(id int) error {
	if id < 0 {
		return ErrNotFound
	}
	return nil
}

func handle(id int) bool {
	return errors.Is(find(id), ErrNotFound)
}
```
