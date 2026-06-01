# Inverse if-conversion

**Name:** inverse if-conversion (the `before → after` direction). Read the
other way — `after → before` — it is plain **if-conversion**: collapsing two
branches into predicated, guarded straight-line code. The two directions are a
single transform run forward or backward.

**Precondition.** One scalar predicate (`x`) is computed once and then re-tested
at multiple guard sites scattered through a function body, with unconditional
code wedged between the guards. Because every guarded statement keys off the
*same* value, the body has only two real execution paths, not 2ⁿ.

**Transform (`before → after`), three steps:**

1. **Hoist the predicate.** Replace the distributed `if x` guards with a single
   `if x { … } else { … }` decision at the top.
2. **Duplicate the body per path** (tail duplication). Each branch becomes a
   specialized function: `processTrue()` is the `x == true` path,
   `processFalse()` is the `x == false` path.
3. **Constant-fold each path.** With `x`'s value fixed, dead guarded assignments
   drop out and their zero values propagate into the surviving calls. In
   `processFalse()`, `a = first()` and `c = third(b)` are dead, so `a`/`c` stay
   at their zero value (`nil`) and the calls fold to `second(nil)` /
   `finish(nil)`.

**What it optimizes / sacrifices.** It removes the redundant re-check of `x` and
makes each path independently legible — at the cost of duplicating the shared
tail. It is *not* a line-count win (the `after` is longer); the payoff is one
decision instead of many, and per-path code with no live predicate.

**Prior art in this repo:** the guard-folding phase of `go-cfg-to-structured`
(`goast/prim_restructure.go`), the `state-trace` skill (distributed comparisons
collapsing to one scalar op), and per-path constant propagation
(`make-constant-propagation` in `lib/wile/goast/domains.scm`).

---

Supporting declarations (shared by both versions):

```go
type T struct{}

func cond() bool     { return true }
func first() *T      { return &T{} }
func second(p *T) *T { return p }
func third(p *T) *T  { return p }
func finish(p *T)    {}
```

Before — one predicate `x`, two guard sites, shared tail between them:

```go
func process() {
	var a *T
	x := cond()
	if x {
		a = first()
	}
	b := second(a)
	var c *T
	if x {
		c = third(b)
	}
	finish(c)
}
```

After — predicate hoisted to one decision, each path specialized and folded:

```go
func process() {
	if cond() {
		processTrue()
	} else {
		processFalse()
	}
}

func processTrue() {
	a := first()
	b := second(a)
	c := third(b)
	finish(c)
}

func processFalse() {
	second(nil) // result unused; kept for side effects
	finish(nil)
}
```

