# Common subexpression elimination

**Name:** common subexpression elimination (CSE) (the `before → after`
direction). The inverse is **rematerialization** — recomputing a value instead
of holding it, to cut register/memory pressure.

**Precondition.** The **same expression** is computed more than once along a
path, and its operands are unchanged between computations (the value is
available).

**Transform (`before → after`), steps:**

1. **Identify equivalent expressions** with matching operands.
2. **Compute once**, bind to a name, and reference the name at each later site.

**What it optimizes / sacrifices.** Removes redundant computation. Sacrifices a
little: the bound value extends a live range, and CSE across large distances can
raise register pressure (the rematerialization inverse exists for exactly that).

**Prior art in this repo:** `ssa-equivalent?` and `discover-equivalences`
(`lib/wile/goast/unify.scm`) decide whether two SSA expressions share a normal
form; availability is a `make-reaching-definitions` query
(`lib/wile/goast/domains.scm`).

---

Before — `w*h` computed twice:

```go
func area(w, h int) int {
	return w*h + w*h/2
}
```

After — computed once, reused:

```go
func area(w, h int) int {
	wh := w * h
	return wh + wh/2
}
```
