# Break import cycle

**Name:** break import cycle (`before → after`). No meaningful inverse — *adding*
a cycle is never desirable, and in Go it is not even possible: the compiler
rejects import cycles outright.

**Precondition.** Two packages **import each other** (directly or transitively).
Unlike every other entry in this catalog, the `before` does **not compile** — Go
enforces acyclic imports — so this refactoring is forced, not discretionary.

**Transform (`before → after`), steps:**

1. **Find the back-edge** — the import that closes the cycle.
2. **Invert or relocate the dependency:** depend on an interface defined on the
   lower layer, replace a typed back-reference with an ID/handle, or move the
   shared type to a third package both can import.
3. The dependency graph becomes one-directional.

**What it optimizes / sacrifices.** Makes the code compile and clarifies which
package is the lower layer. Sacrifices a direct typed reference across the
boundary — here `[]order.Order` becomes `[]int` of IDs, trading a pointer-chase
for a lookup.

**Detector status:** existing — the Go compiler rejects an existing cycle, and
`verify-acyclic` / `go-list-deps` (split.scm) check whether a *proposed* package
split would introduce one. The tool's role is preventing cycles in refactors, not
finding extant ones (the compiler already does that).

---

Before — `user` and `order` import each other (**does not compile** — Go rejects
the cycle):

```go
// package user
type User struct {
	History []order.Order // user → order
}

// package order
type Order struct {
	Buyer *user.User // order → user   ← cycle
}
```

After — `user` holds IDs; the dependency points one way (`order → user`):

```go
// package user — imports nothing from order
type User struct {
	OrderIDs []int
}

// package order — imports user only
import "example/user"

type Order struct {
	ID    int
	Buyer *user.User
}
```
