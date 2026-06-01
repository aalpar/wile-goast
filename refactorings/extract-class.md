# Extract class (split struct)

**Name:** extract class / split struct (`before → after`). The inverse is
**inline class** (merge two types whose fields are always used together).

**Precondition.** A struct's fields form **disjoint clusters** — subsets that are
read and written together, with little or no method touching fields from more
than one cluster. The struct boundary groups fields that don't actually co-vary.

**Transform (`before → after`), steps:**

1. **Find the clusters** — fields that co-occur in the same methods.
2. **Extract each cluster** into its own type.
3. **Embed or compose** the new types in the original and move each method to the
   type that owns its fields.

**What it optimizes / sacrifices.** Each new type has a tighter invariant and
methods stop reaching across unrelated state. Sacrifices flatness: field access
gains a level of qualification (`wd.bounds.w` vs `wd.w`).

**Prior art in this repo:** `find_false_boundaries` and `fca-recommend`
(`lib/wile/goast/fca.scm`, `fca-recommend.scm`) discover field clusters via
Formal Concept Analysis on SSA field-access data — a struct whose fields split
into disjoint concepts is the signal for this split.

---

Before — geometry and color fields share one struct:

```go
type widget struct {
	x, y, w, h int
	r, g, b    uint8
}

func (wd *widget) area() int   { return wd.w * wd.h }
func (wd *widget) gray() uint8 { return (wd.r + wd.g + wd.b) / 3 }
```

After — disjoint clusters extracted:

```go
type rect struct{ x, y, w, h int }
type color struct{ r, g, b uint8 }

type widget struct {
	bounds rect
	fill   color
}

func (r rect) area() int    { return r.w * r.h }
func (c color) gray() uint8 { return (c.r + c.g + c.b) / 3 }
```
