# Encapsulate field

**Name:** encapsulate field (`before → after`). The inverse is **expose field** —
dropping accessors when a type is a plain data record with no invariant.

**Precondition.** A struct field is **exported** (mutable by any package) but the
type has an invariant, wants to control mutation, or needs a seam to change its
representation later without breaking callers.

**Transform (`before → after`), steps:**

1. **Unexport the field.**
2. **Add accessor/mutator methods** expressing the allowed operations.
3. **Route external access** through the methods.

**What it optimizes / sacrifices.** The type controls its own state, and the
representation can change behind the methods. Sacrifices the convenience of direct
field access and adds method surface — overkill for a pure data-transfer struct
with no invariant.

**Detector status:** existing (partial) — lint analyzers can flag exported
mutable fields; deciding *which* deserve encapsulation (those with an invariant)
is a judgment the tool informs but does not make.

---

Before — `Count` is exported and freely mutable:

```go
type Counter struct {
	Count int
}

func (c *Counter) Reset() { c.Count = 0 }
```

After — state is private, mutation is through methods:

```go
type Counter struct {
	count int
}

func (c *Counter) Count() int { return c.count }
func (c *Counter) Inc()       { c.count++ }
func (c *Counter) Reset()     { c.count = 0 }
```
