# Constant folding & propagation

**Name:** constant folding (evaluate constant expressions) plus constant
propagation (substitute known values), `before → after`. There is no useful
inverse — generalizing a constant to a parameter is a *different* refactoring
(parameterize-by-difference), not the undo of this one.

**Precondition.** A value is **statically known**: a literal, or a variable
assigned only a literal and never reassigned before use.

**Transform (`before → after`), steps:**

1. **Propagate** each known value forward to its uses.
2. **Fold** any expression whose operands are now all constant into a single
   literal.

**What it optimizes / sacrifices.** Eliminates run-time work and dead locals,
and exposes further simplification. The risk is *meaning*: a folded magic number
can be less self-documenting than the expression it replaced — keep a named
constant if the structure carried intent.

**Prior art in this repo:** `make-constant-propagation`
(`lib/wile/goast/domains.scm`) is the forward analysis; `go-concrete-eval`
evaluates the integer opcodes used to fold.

---

Before — values known, product computed at run time:

```go
func windowBytes() int {
	width := 1024
	scale := 8
	return width * scale
}
```

After — propagated and folded:

```go
func windowBytes() int {
	return 8192
}
```
