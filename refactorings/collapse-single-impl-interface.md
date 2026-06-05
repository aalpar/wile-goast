# Collapse single-implementation interface

**Name:** collapse single-implementation interface (`before → after`). Read
backward it is **extract-interface** (`extract-interface.md`): introducing an
interface over a concrete type to admit alternatives. This file is the inverse —
removing an interface that never earned its abstraction — completing the pair.

**Precondition.** An interface has exactly one implementing type in the program,
no test substitutes a mock for it, and no plausible second implementation is on
the horizon. It is *speculative generality*: an abstraction boundary with nothing
on the other side. Here `Store` is implemented only by `memStore`, and `lookup`
gains nothing from the indirection.

**Transform (`before → after`), steps:**

1. **Count implementors.** `go-interface-implementors` returns exactly one, and a
   search for mock/fake/stub types finds none — confirm the interface is unused as
   a seam.
2. **Replace the interface type with the concrete type** at every use
   (`lookup(s Store, …)` → `lookup(s *memStore, …)`).
3. **Delete the interface declaration.**

**What it optimizes / sacrifices.** It removes a layer of indirection and a type
the reader must cross-reference to find the real behavior; the call becomes a
direct, devirtualized dispatch. It sacrifices the seam — if a second
implementation or a test double is later needed, re-extract the interface (the
inverse direction). Collapse only when the single implementation is the *settled*
design, not a first-of-many.

**Detector status:** existing — `go-interface-implementors` (`(wile goast)`); an
implementor count of 1 with no mock need is the speculative-generality signal. The
"no mock need" half is the human judgment the count cannot supply.

---

Before — `Store` has one implementor (`memStore`) and no test double:

```go
type Store interface {
	Get(k string) string
}

type memStore struct {
	m map[string]string
}

func (s *memStore) Get(k string) string { return s.m[k] }

func lookup(s Store, k string) string {
	return s.Get(k)
}
```

After — interface removed; the concrete type is used directly:

```go
type memStore struct {
	m map[string]string
}

func (s *memStore) Get(k string) string { return s.m[k] }

func lookup(s *memStore, k string) string {
	return s.Get(k)
}
```
