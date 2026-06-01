# Replace conditional with polymorphism

**Name:** replace conditional with polymorphism (`before → after`). The inverse
is **inline polymorphism into a switch** — collapsing an interface back into a
type-tag switch, occasionally done to keep a small closed set in one place.

**Precondition.** A function branches on a **type tag** (a `kind` string/enum
field, or a type switch), and the same tag is switched on in more than one place.
The set of cases is open and likely to grow.

**Transform (`before → after`), steps:**

1. **Define an interface** with one method per operation the switch performs.
2. **Make each case a type** implementing that method with its branch body.
3. **Replace the switch** with a call to the interface method; dispatch is now
   the method table.

**What it optimizes / sacrifices.** Adding a case becomes adding a type, not
editing every switch (open/closed principle). Sacrifices locality: cases that
were one readable switch now scatter across types — worse when the set is small
and fixed (prefer a dispatch table, or the switch itself).

**Detector status:** could-build — AST type-switch / tag-field detection plus
`go-interface-implementors` to enumerate the case set; no packaged detector yet.

---

Before — behavior selected by a `kind` tag:

```go
type shape struct {
	kind    string
	w, h, r int
}

func area(s shape) float64 {
	switch s.kind {
	case "rect":
		return float64(s.w * s.h)
	case "circle":
		return 3.14159 * float64(s.r*s.r)
	}
	return 0
}
```

After — each case is a type; dispatch is the method table:

```go
type shape interface{ area() float64 }

type rect struct{ w, h int }
type circle struct{ r int }

func (r rect) area() float64   { return float64(r.w * r.h) }
func (c circle) area() float64 { return 3.14159 * float64(c.r*c.r) }
```

Sibling: `replace-switch-with-dispatch-table` is the lighter-weight move when the
cases are single one-line actions over a closed set rather than types with
several operations.
