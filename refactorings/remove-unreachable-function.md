# Remove unreachable function

**Name:** remove unreachable function (`before → after`). No meaningful inverse —
deliberately adding code no caller reaches is dead weight, not a refactoring.

**Precondition.** A function (and any private cluster only it calls) has no path
from the program's *root set*. The root set is the judgment that makes this
analysis honest: in Go it is `main`, every exported identifier, every `TestXxx`,
plus anything reached only by reflection or `//go:linkname`. A function outside
the reachable closure of those roots is dead. Here `orphan`/`alsoOrphan` form a
call island with no edge from the only root, `Run`.

**Transform (`before → after`), steps:**

1. **Choose the root set.** This is the irreducible human input — too narrow and
   you delete live code; too broad and nothing is ever unreachable. Exported
   API, `main`, tests, and reflection entry points are the defensible default.
2. **Compute reachability** from the roots over the call graph; the dead set is
   `all − reachable(roots)`.
3. **Delete the dead set,** innermost first, so each deletion does not transiently
   appear to make its now-orphaned callees reachable.

**What it optimizes / sacrifices.** It removes maintenance burden and reader
confusion — code that looks live but is not. The sacrifice is entirely in the
root-set judgment: reflection and `//go:linkname` create edges the static call
graph cannot see, so an over-confident root set can delete code that *is*
reached dynamically. The tool reports the candidate set; confirming each is
genuinely dead is the review gate.

**Detector status:** existing (partial) — call-graph reachability via
`path-query-all` (`(wile goast path-algebra)`); unreachable is the complement of
the reachable closure from the chosen roots. The reachability computation
exists; the root-set selection (main + exported + tests + reflection) is the
configuration the tool cannot infer.

---

Before — `Run` is the only root; `orphan`/`alsoOrphan` are an unreachable island
(legal Go — unused unexported funcs compile):

```go
func Run() {
	helper()
}

func helper() {
	used()
}

func used() {}

func orphan() {
	alsoOrphan()
}

func alsoOrphan() {}
```

After — the island removed; only the closure reachable from `Run` remains:

```go
func Run() {
	helper()
}

func helper() {
	used()
}

func used() {}
```
