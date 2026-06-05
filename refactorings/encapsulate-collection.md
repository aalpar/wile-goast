# Encapsulate collection

**Name:** encapsulate collection (`before → after`). No useful inverse — handing
out a live reference to internal state is the leak this closes, not a refactoring
worth a name.

**Precondition.** A method returns a field of slice or map type directly. Because
slices and maps are reference types, the caller shares the backing array/buckets
and can mutate the owner's state without going through any method — the
encapsulation the struct appears to have is fictional. Here `Roster.Names()`
returns `r.names`, so a caller can rewrite the roster's contents in place.

**Transform (`before → after`), steps:**

1. **Find the leak.** A getter whose return value is, in SSA, the field load of a
   slice/map-typed field with no intervening copy.
2. **Return a defensive copy** (or an explicitly read-only view) instead of the
   field itself — `make` + `copy` for a slice, a clone for a map.
3. **Route all mutation through methods** (`Add`, `Remove`), so the struct owns
   every write to its collection.

**What it optimizes / sacrifices.** It restores the invariant that a struct
controls its own state, eliminating a class of spooky-action-at-a-distance bugs.
It sacrifices allocation-free reads: every call now copies. For large collections
read on a hot path, prefer a read-only view (an iterator or an index-accessor)
over a full copy, or document that the returned slice must not be retained or
mutated.

**Detector status:** could-build (Tier A) — wire a query over SSA: a getter whose
returned value is a direct load of a slice- or map-typed field, with no copy
between the load and the return. The field-type and return-value facts are already
in the SSA; the un-built piece is the "no intervening copy" predicate.

---

Before — `Names` hands out the internal slice; callers can mutate `Roster`:

```go
type Roster struct {
	names []string
}

func (r *Roster) Names() []string {
	return r.names
}

func (r *Roster) Add(name string) {
	r.names = append(r.names, name)
}
```

After — `Names` returns a copy; writes flow only through `Add`:

```go
type Roster struct {
	names []string
}

func (r *Roster) Names() []string {
	out := make([]string, len(r.names))
	copy(out, r.names)
	return out
}

func (r *Roster) Add(name string) {
	r.names = append(r.names, name)
}
```
