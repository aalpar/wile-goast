# Move field

**Name:** move field (`before → after`). The inverse is *move field* in the other
direction — the transform is symmetric, run toward whichever struct the field
actually co-varies with. There is no separate name for the reverse.

**Precondition.** A field declared on struct `A` is, in practice, always read or
written together with the fields of a *different* struct `B`, and never with
`A`'s own fields. In Formal Concept Analysis terms, the field belongs to a
concept whose extent spans `B`'s operations, not `A`'s — a cross-boundary
concept. Here `Account.Currency` is only ever used alongside `Money.Amount`.

**Transform (`before → after`), steps:**

1. **Find the cross-boundary concept.** The field co-varies with another
   struct's field set instead of its declaring struct's — the FCA signal.
2. **Move the field** to the struct it co-varies with (`Currency`: `Account` →
   `Money`).
3. **Rewrite accessors and signatures.** Call sites that passed both structs to
   reach the field now pass only the struct that owns it; `render(a, m)` becomes
   `render(m)`.

**What it optimizes / sacrifices.** It puts state where it is used, shrinking the
declaring struct and often dropping a parameter from the functions that needed
both structs only to assemble the field with its real collaborators. It
sacrifices nothing when the co-variation is total; if the field is *occasionally*
used with `A`, moving it trades one set of cross-references for another, and the
FCA extent sizes are the tiebreaker.

**Detector status:** existing — `cross-boundary-concepts` (`fca.scm`). A formal
concept whose extent crosses struct types is a move-field (or merge-struct)
candidate; `boundary-findings` emits each extent member as a located finding
whose `why` is the shared field intent.

---

Before — `Currency` lives on `Account` but only ever pairs with `Money.Amount`:

```go
type Account struct {
	ID       int
	Currency string
}

type Money struct {
	Amount int
}

func render(a Account, m Money) string {
	return format(m.Amount, a.Currency)
}

func format(amount int, currency string) string { return currency }
```

After — `Currency` moved to `Money`; `Account` shrinks and `render` drops a
parameter:

```go
type Account struct {
	ID int
}

type Money struct {
	Amount   int
	Currency string
}

func render(m Money) string {
	return format(m.Amount, m.Currency)
}
```
