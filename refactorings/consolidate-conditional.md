# Consolidate conditional expression

**Name:** consolidate conditional expression (`before → after`). The inverse —
splitting one boolean into a sequence of guards — is occasionally done for
step-through debugging, but has no standard name.

**Precondition.** Several conditionals **yield the same result** and have no
intervening side effects; they are an OR (or AND) of predicates spelled out
longhand.

**Transform (`before → after`), steps:**

1. **Confirm each guard returns the same value** and the guards are
   side-effect-free.
2. **Combine the predicates** with the matching boolean connective and return
   once.

**What it optimizes / sacrifices.** Replaces a staircase of `if`s with one
expression that names the combined condition. Sacrifices per-branch
instrumentation points; if you need to log *which* predicate fired, keep them
split.

**Prior art in this repo:** `go-cfg` paths show the guards converge on one
result; `boolean-normalize` (`lib/wile/goast/boolean-simplify.scm`) canonicalizes
the combined predicate.

---

Supporting declarations (shared by both versions):

```go
type T struct {
	banned  bool
	expired bool
	active  bool
}
```

Before — three guards, same result:

```go
func disabled(u *T) bool {
	if u.banned {
		return true
	}
	if u.expired {
		return true
	}
	if !u.active {
		return true
	}
	return false
}
```

After — one combined predicate:

```go
func disabled(u *T) bool {
	return u.banned || u.expired || !u.active
}
```
