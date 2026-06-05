# Accept smallest interface

**Name:** accept smallest interface (`before → after`). The inverse — widening a
parameter to a concrete type or a fatter interface — is the over-specification
this reverses. The Go proverb is the rule: *accept interfaces, return structs.*

**Precondition.** A function parameter has a type whose method set is far larger
than the methods the function actually calls on it. The SSA call structure makes
the gap visible: it records the receiver and method of every dynamic call, so the
*used* method set is computable directly. Here `consume` takes `*os.File` but
invokes only `.Read`.

**Transform (`before → after`), steps:**

1. **Compute the invoked method set** of the parameter from the SSA `recv`/`method`
   fields of the calls it participates in (`.Read` only, here).
2. **Find or define the smallest interface** that covers exactly that set — for a
   single `Read(...) (int, error)`, the standard `io.Reader`.
3. **Replace the parameter type** with that interface and update the body to call
   through it. Callers that passed the concrete type still satisfy it.

**What it optimizes / sacrifices.** It widens what callers may pass (any
`io.Reader`, not just files), makes the function trivially testable with a fake,
and documents its real dependency in the signature. It sacrifices direct access to
the concrete type's other methods — if the function later needs `.Seek`, the
interface must grow or revert. Narrow to what is used *today*, widen deliberately
when a new need appears.

**Detector status:** could-build (Tier A) — wire a query over the SSA call
structure (`recv`/`method` for interface calls, `func`/`args[0]` for static method
calls; `goastssa/mapper.go`): the method set invoked on a parameter value.
*Not* `go-func-refs`, which emits a flat per-function ref set and never binds a
method to the receiver value it was called on — that binding is SSA's.

---

Before — `consume` takes a concrete `*os.File` but calls only `.Read`:

```go
import "os"

func consume(f *os.File) ([]byte, error) {
	buf := make([]byte, 4)
	_, err := f.Read(buf)
	return buf, err
}
```

After — narrowed to `io.Reader`, the smallest interface covering the call:

```go
import "io"

func consume(r io.Reader) ([]byte, error) {
	buf := make([]byte, 4)
	_, err := r.Read(buf)
	return buf, err
}
```
