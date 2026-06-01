# Collapse mutual recursion

**Name:** collapse mutual recursion (`before → after`) — merge a strongly
connected component of the call graph into a single self-recursive function.
No standard inverse; splitting one function into a mutually recursive pair is
rarely desirable.

**Precondition.** Two or more functions form a **non-trivial strongly connected
component** (they call each other) and differ only in a small amount of state
that can be carried as a parameter.

**Transform (`before → after`), steps:**

1. **Identify the SCC** in the call graph.
2. **Introduce a parameter** capturing what distinguishes the members
   (here, parity).
3. **Merge the bodies** into one self-recursive function and rewrite the original
   names as thin calls into it.

**What it optimizes / sacrifices.** Collapses a cycle to a single recursion,
removing the cross-function indirection and making the shared structure explicit.
Sacrifices the individually-named entry points unless you keep wrappers (shown).

**Prior art in this repo:** `path-analysis-sccs` and `path-node-in-cycle?`
(`lib/wile/goast/path-algebra.scm`) detect the non-trivial SCC — the structural
signature that a set of functions is mutually recursive and a collapse candidate.

---

Before — `isEven` and `isOdd` are mutually recursive:

```go
func isEven(n int) bool {
	if n == 0 {
		return true
	}
	return isOdd(n - 1)
}

func isOdd(n int) bool {
	if n == 0 {
		return false
	}
	return isEven(n - 1)
}
```

After — collapsed to one self-recursive function with a parity parameter:

```go
func parity(n int, even bool) bool {
	if n == 0 {
		return even
	}
	return parity(n-1, !even)
}

func isEven(n int) bool { return parity(n, true) }
func isOdd(n int) bool  { return parity(n, false) }
```
