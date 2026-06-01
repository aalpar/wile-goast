# Guard-clause flattening

**Name:** guard-clause flattening / replace-nested-conditional-with-guard-clauses
(the `before → after` direction). The inverse is **single-exit structuring** —
folding early returns back into one nested `if/else` with a single return.

**Precondition.** Error/edge handling is expressed as **nested** conditionals,
pushing the happy path to the deepest indentation level. Each failure case is a
separate nesting level rather than an early exit.

**Transform (`before → after`), steps:**

1. **Invert each outer guard** and `return` early on the failure case.
2. **Unindent** the remaining body one level per guard removed.
3. The happy path ends up flat at the bottom.

**What it optimizes / sacrifices.** Reduces nesting depth and co-locates each
precondition with its failure response; the happy path reads top-to-bottom.
Trades a single exit point for many — a cost only if your style or tooling
demands one return.

**Prior art in this repo:** `go-cfg-to-structured` (`goast/prim_restructure.go`)
performs the *opposite* normalization — guard-if-return → nested if/else (Case 1)
and loop-return rewriting (Case 2) — to reach single-exit form. This refactoring
is that pass run backward.

---

Supporting declarations (shared by both versions):

```go
import "errors"

type T struct{ ok bool }

var errNil = errors.New("nil")
var errNotOK = errors.New("not ok")

func doWork(v *T) {}
```

Before — happy path buried two levels deep:

```go
func process(v *T) error {
	if v != nil {
		if v.ok {
			doWork(v)
			return nil
		}
		return errNotOK
	}
	return errNil
}
```

After — guards exit early, happy path flat:

```go
func process(v *T) error {
	if v == nil {
		return errNil
	}
	if !v.ok {
		return errNotOK
	}
	doWork(v)
	return nil
}
```
