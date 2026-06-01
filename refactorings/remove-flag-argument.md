# Remove flag argument

**Name:** remove flag argument / split-by-flag (`before → after`). The inverse is
**unify by flag** (merge two functions behind a boolean). This is the
**caller-side dual of inverse if-conversion** (`pull.md`): `pull.md` hoists a
predicate *inside* a function body; this hoists a *boolean parameter* out to the
call site by splitting the function in two.

**Precondition.** A function takes a **boolean (or enum) parameter that selects
between two behaviors**, so every call site reads `f(x, true)` / `f(x, false)`
with no clue what the flag means.

**Transform (`before → after`), steps:**

1. **Split the function** along the flag into two named functions, each
   specialized to one value — constant-folding the flag away inside, exactly like
   `pull.md`'s per-branch specialization.
2. **Replace each call** with the function whose name states the intent.

**What it optimizes / sacrifices.** Call sites self-document (`bookPremium` vs
`book(c, true)`), and each function is simpler. Sacrifices a single entry point:
if a caller's flag is computed at run time, it must now choose between the two
functions (an `if` at the call site) — sometimes worse than passing the bool.

**Detector status:** could-build — signature analysis for boolean params whose
only use is a top-level branch; pairs with the `pull.md` predicate-hoist detector.

---

Before — `premium` selects behavior:

```go
func priorityFee() int { return 50 }

func book(customer string, premium bool) int {
	base := 100
	if premium {
		return base + priorityFee()
	}
	return base
}
```

After — one function per intent:

```go
func priorityFee() int { return 50 }

func bookStandard(customer string) int { return 100 }
func bookPremium(customer string) int  { return 100 + priorityFee() }
```
