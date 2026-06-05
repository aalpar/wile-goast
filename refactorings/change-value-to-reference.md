# Change value to reference (receiver consistency)

**Name:** change value to reference (`before → after`), in the method-receiver
sense: switching a value receiver to a pointer receiver so mutations take effect,
and making a type's receiver kinds consistent. The inverse — value receivers for
small immutable types — is the correct default *when nothing mutates*; this
transform applies when something does.

**Precondition.** A method with a *value* receiver assigns to a receiver field —
the assignment mutates a copy and is silently lost — or a single type carries a
mix of value and pointer receivers, which is a latent bug (the value-receiver
methods see stale copies, and the type's method set depends on whether you hold a
value or a pointer). Here `Counter.Inc` has a value receiver and discards its
increment, while `Counter.Value` uses a pointer receiver.

**Transform (`before → after`), steps:**

1. **Detect the mismatch.** AST gives each method's receiver kind; SSA shows
   whether the method stores to a receiver field. A value receiver that stores is
   the bug; mixed kinds on one type is the inconsistency.
2. **Make the mutating methods pointer-receiver,** so the store lands on the
   caller's value.
3. **Make the whole type's receivers consistent** (Go style: if any method needs a
   pointer receiver, give them all pointer receivers).

**What it optimizes / sacrifices.** It fixes lost-update bugs and gives the type a
single, predictable method set. It sacrifices value semantics — a `*Counter` is
shared, not copied on assignment, so callers must be aware of aliasing, and the
zero value behaves differently. Switch to pointer receivers when mutation or
consistency demands it, not reflexively; genuinely immutable value types should
stay value-receiver.

**Detector status:** could-build (Tier A) — wire a query over AST receiver kinds
plus SSA field stores: a value receiver that stores to a receiver field, or a type
whose methods mix receiver kinds. Both inputs (receiver kind, field-store target)
already exist; the un-built piece is the conjunction.

---

Before — `Inc` mutates a copy (increment lost); receiver kinds are mixed:

```go
type Counter struct {
	n int
}

func (c Counter) Inc() {
	c.n++
}

func (c *Counter) Value() int {
	return c.n
}
```

After — consistent pointer receivers; `Inc` mutates the real `Counter`:

```go
type Counter struct {
	n int
}

func (c *Counter) Inc() {
	c.n++
}

func (c *Counter) Value() int {
	return c.n
}
```
