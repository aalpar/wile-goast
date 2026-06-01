# Tail merging (cross-jumping)

**Name:** tail merging, a.k.a. cross-jumping (the `before → after` direction).
The inverse is **tail duplication** — exactly the move `pull.md`'s inverse
if-conversion uses in step 2. Whether to merge or duplicate is the central
trade-off: merging minimizes code size, duplication enables per-path
specialization.

**Precondition.** Two (or more) branches of a conditional **end in identical
statement sequences**. The shared suffix is written once per branch.

**Transform (`before → after`), steps:**

1. **Find the longest common suffix** of the branch bodies.
2. **Sink it below the conditional** so it runs once after the branches rejoin.
3. Leave only the branch-specific heads inside the arms.

**What it optimizes / sacrifices.** Eliminates duplicated code (smaller binary,
single point of edit for the shared tail). Sacrifices per-branch specialization
and can lengthen the live range of values that feed the merged tail.

**Prior art in this repo:** `go-cfg` exposes the block structure; `ast-diff` /
`unifiable?` (`lib/wile/goast/unify.scm`) detect that two blocks' trailing
statements are structurally identical and therefore mergeable.

---

Supporting declarations (shared by both versions):

```go
func prepA()  {}
func prepB()  {}
func commit() {}
func notify() {}
```

Before — `commit(); notify()` duplicated in both arms:

```go
func handle(x bool) {
	if x {
		prepA()
		commit()
		notify()
	} else {
		prepB()
		commit()
		notify()
	}
}
```

After — shared tail sunk below the branch:

```go
func handle(x bool) {
	if x {
		prepA()
	} else {
		prepB()
	}
	commit()
	notify()
}
```
