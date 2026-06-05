# Normalize divergent sites

**Name:** normalize divergent sites (`before → after`). The inverse —
*introducing* a deviation — is never a deliberate refactoring; it is the drift
this transform reverses. This is the refactoring the belief DSL was built to
feed: Engler et al.'s premise is that *deviation from a statistical norm is a
likely bug*, and the cure is to rewrite the outlier to the dominant pattern.

**Precondition.** A population of structurally similar sites (functions,
methods, branches) overwhelmingly follows one pattern — the *dominant* form —
while a small minority deviates. The dominance must be strong enough that the
minority reads as oversight, not as a deliberate special case. Here three
functions acquire a file; two `defer f.Close()` and one closes manually, leaking
`f` on an early-return error path.

**Transform (`before → after`), steps:**

1. **Establish the norm.** Run the belief whose `adherence` set is the dominant
   pattern and whose `deviations` set is the outliers. The deviations list *is*
   the work list — each entry is a located finding (`where` + `why`).
2. **Confirm the deviation is accidental,** not a justified exception. This is
   the human judgment the tool refuses to make; the belief only proposes.
3. **Rewrite each deviant site to the dominant form.** Replace the manual
   `f.Close()` with `defer f.Close()` immediately after the successful acquire,
   so every return path — including the error path — releases the resource.

**What it optimizes / sacrifices.** It removes a class of latent bugs (here a
file-descriptor leak) and makes the population uniform, so a reader learns the
pattern once. It sacrifices nothing in the common case; the only risk is
normalizing a site whose deviation was intentional — which is exactly why
step 2 is a review gate, not an automatic edit.

**Detector status:** existing — `run-beliefs` (`(wile goast belief)`). The
`deviations` field of a per-site belief result is the site set directly; the
`paired-with` checker emits each as a located finding. No new detector is
needed — the pattern is the tool's output.

---

Supporting type and helper (shared by both versions):

```go
func validate(f *os.File) error { return nil }
```

Before — `readA`/`readB` defer the close; `readC` deviates with a manual close
that leaks `f` when `validate` fails:

```go
import "os"

func readA(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return validate(f)
}

func readB(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return validate(f)
}

// Deviant site: manual Close — leaks f on the validate-error path.
func readC(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := validate(f); err != nil {
		return err // f is never closed here
	}
	f.Close()
	return nil
}
```

After — `readC` normalized to the dominant `defer f.Close()`; the leak is gone:

```go
func readC(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return validate(f)
}
```
