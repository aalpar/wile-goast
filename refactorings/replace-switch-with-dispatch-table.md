# Replace switch with dispatch table

**Name:** replace switch with dispatch table (`before → after`). The inverse is
**inline the table back into a switch**, sometimes preferred when cases need
fallthrough or must close over the enclosing function's locals.

**Precondition.** A `switch` maps a **key to an action** with no inter-case
fallthrough and no shared mutable state — each case is independent and
self-contained.

**Transform (`before → after`), steps:**

1. **Build a map** from key to the action (a function value).
2. **Replace the switch** with a lookup; handle the miss explicitly.

**What it optimizes / sacrifices.** The dispatch becomes data: register a new
case by adding a map entry, and the case set is enumerable/inspectable at run
time. Sacrifices the switch's `fallthrough` and its free access to the enclosing
locals; closures recover the latter at a readability cost.

**Detector status:** could-build — AST (a `switch` whose cases are independent
return/assign actions over a single scrutinee).

---

Before — switch dispatch:

```go
func dispatch(cmd string, n int) int {
	switch cmd {
	case "inc":
		return n + 1
	case "dec":
		return n - 1
	case "double":
		return n * 2
	}
	return n
}
```

After — table dispatch:

```go
var ops = map[string]func(int) int{
	"inc":    func(n int) int { return n + 1 },
	"dec":    func(n int) int { return n - 1 },
	"double": func(n int) int { return n * 2 },
}

func dispatch(cmd string, n int) int {
	if op, ok := ops[cmd]; ok {
		return op(n)
	}
	return n
}
```

Sibling: `replace-conditional-with-polymorphism` when the cases are types
carrying several operations, not single one-line actions.
