# Move function across package

**Name:** move function (or method) across package (`before → after`). The inverse
is the symmetric move back; "across package" is the boundary that distinguishes
this from the intra-package `extract-function` / `inline-function`.

**Precondition.** A function lives in package A but **operates entirely on package
B's type** — it reaches across the boundary for all its data and gives package A
no reason to own it. This is feature envy at package scale.

**Transform (`before → after`), steps:**

1. **Locate the data the function actually touches** — here, only `order.Order`.
2. **Move it** to that package, typically as a method on the type.
3. **Update callers** and drop the now-unneeded import.

**What it optimizes / sacrifices.** Behavior lives with the data it operates on;
package A sheds a dependency on B. Sacrifices nothing if A truly didn't need the
function; if A and other packages both call it, weigh whether the move merely
relocates the coupling rather than removing it.

**Detector status:** existing (partial) — `recommend_split` and `go-func-refs`
surface functions whose dependency profile points entirely at another package
(the signal to move them); the move itself is manual.

---

Before — `report.Total` operates only on `order.Order`:

```go
// package report
import "example/order"

func Total(o order.Order) int {
	sum := 0
	for _, line := range o.Lines {
		sum += line
	}
	return sum
}
```

After — behavior moved to the type it belongs to:

```go
// package order
type Order struct {
	Lines []int
}

func (o Order) Total() int {
	sum := 0
	for _, line := range o.Lines {
		sum += line
	}
	return sum
}
```
