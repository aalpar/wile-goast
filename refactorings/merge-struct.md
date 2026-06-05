# Merge struct (inline class)

**Name:** merge struct (`before → after`), the inline-class direction. Read
backward it is **extract-class** (`extract-class.md`): pulling a cohesive field
cluster out into its own type. The two are one transform run in opposite
directions; this file gives the inline direction its own entry, promoting the
"inline class" mention inside `extract-class.md` to a first-class refactoring.

**Precondition.** A struct `B` is embedded or held by exactly one struct `A`,
is never instantiated or passed independently, and its whole field set always
travels with `A`'s. In FCA terms it is `move-field`'s limiting case: a concept
whose extent covers *all* of `B`'s fields together with `A`'s, so the boundary
between them carries no information. Here `Address` is only ever reached through
`Customer`.

**Transform (`before → after`), steps:**

1. **Confirm `B` has no independent life** — no standalone construction, no
   function takes or returns a bare `B`, no other struct holds it. The same FCA
   report that drives extract-class, read inverse, supplies this.
2. **Hoist `B`'s fields into `A`,** preserving names (rename only on collision).
3. **Flatten the accessors:** `c.Addr.Street` becomes `c.Street`; delete `B`.

**What it optimizes / sacrifices.** It removes a layer of indirection and a type
that earned its keep only as a grouping label, shortening every access path. It
sacrifices the grouping itself — if `B` later needs independent reuse, or its
fields acquire an invariant of their own, you are back to extract-class. Inline
only when the sub-struct is a pure accident of past extraction, not a latent
abstraction.

**Detector status:** existing (partial) — the `cross-boundary-concepts` report
(`fca.scm`) read in the inverse direction: a concept whose extent spans one
struct's entire field set plus another's signals an inlineable boundary. Same
detector as extract-class and move-field; the direction is the human's call.

---

Before — `Address` exists only as a field group inside `Customer`:

```go
type Address struct {
	Street string
	City   string
}

type Customer struct {
	Name string
	Addr Address
}

func label(c Customer) string {
	return c.Name + " " + c.Addr.Street + " " + c.Addr.City
}
```

After — `Address` inlined into `Customer`; one struct, no indirection:

```go
type Customer struct {
	Name   string
	Street string
	City   string
}

func label(c Customer) string {
	return c.Name + " " + c.Street + " " + c.City
}
```
