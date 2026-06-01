# Decompose conditional

**Name:** decompose conditional (`before → after`). The inverse is **inline
condition** — substituting the predicate's body back at its single use.

**Precondition.** A conditional's **test is complex** enough that its *meaning*
is not obvious from its form — a multi-operator boolean whose intent deserves a
name.

**Transform (`before → after`), steps:**

1. **Name the predicate** by extracting the test into a well-named function.
2. **Replace the inline test** with a call to it.

**What it optimizes / sacrifices.** Readability: the call names *why*, the
function holds *how*. Unlike most entries here this does not remove computation —
it trades an anonymous expression for an indirection, paying a name's worth of
overhead for clarity.

**Prior art in this repo:** `ast-condition->symbolic`
(`lib/wile/goast/boolean-simplify.scm`) projects an AST condition to a symbolic
term — the handle for deciding when a test is complex enough to deserve naming.

---

Before — the surge rule is inline and unnamed:

```go
func fee(temp, qty int, holiday bool) int {
	if temp < 0 && qty > 100 || holiday {
		return qty * 2
	}
	return qty
}
```

After — the rule is named:

```go
func fee(temp, qty int, holiday bool) int {
	if surge(temp, qty, holiday) {
		return qty * 2
	}
	return qty
}

func surge(temp, qty int, holiday bool) bool {
	return temp < 0 && qty > 100 || holiday
}
```
