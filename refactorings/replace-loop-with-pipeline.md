# Replace loop with pipeline

**Name:** replace loop with pipeline (`before → after`) — express an imperative
accumulation as filter/map stages. The inverse is **replace pipeline with loop**,
often the right call in Go where an explicit loop is idiomatic and
allocation-free.

**Precondition.** A loop **builds a collection by filtering and transforming**
another, with the control structure (`append`, `continue`) obscuring the data
transformation underneath.

**Transform (`before → after`), steps:**

1. **Name each stage** — filter (which elements survive), map (how each
   transforms).
2. **Compose** the stages, replacing the loop with the composition.

**What it optimizes / sacrifices.** The *what* (filter long names, uppercase
them) reads directly, free of append/continue bookkeeping. Sacrifices real
efficiency: each stage allocates an intermediate slice where the loop allocated
once, and Go's standard library has no lazy iterators before `iter` — so the
pipeline is eager. In hot paths the loop wins; this is a readability trade, not a
speed one.

**Detector status:** could-build — AST: a range loop whose body is an
append-guarded-by-predicate over a transform of the element. The allocation-minded
inverse moves (preallocate a slice's capacity, keep values on the stack via
`reduce-escape`) are **out-of-scope** — they need escape analysis the tool
doesn't have.

---

Before — filter-and-transform tangled with append/continue:

```go
import "strings"

func longNames(users []string) []string {
	var out []string
	for _, u := range users {
		if len(u) > 3 {
			out = append(out, strings.ToUpper(u))
		}
	}
	return out
}
```

After — stages named and composed:

```go
import "strings"

func Filter[T any](xs []T, keep func(T) bool) []T {
	var out []T
	for _, x := range xs {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out
}

func Map[T, U any](xs []T, f func(T) U) []U {
	out := make([]U, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

func longNames(users []string) []string {
	return Map(Filter(users, func(u string) bool { return len(u) > 3 }), strings.ToUpper)
}
```
