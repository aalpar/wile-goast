# Replace primitive with value object

**Name:** replace primitive with value object / whole-value (`before → after`).
The inverse is **inline value object** — dropping the wrapper when a bare
primitive carries no invariant worth enforcing.

**Precondition.** A primitive (`string`, `int`, ...) carries **meaning and an
invariant** (an email contains `@`; a port is 0–65535) but the invariant is
re-checked ad hoc at each use site, or not at all.

**Transform (`before → after`), steps:**

1. **Wrap the primitive** in a named type.
2. **Move validation into a constructor** that is the only way to build a valid
   value.
3. **Pass the type** instead of the primitive; downstream code trusts the
   invariant by construction.

**What it optimizes / sacrifices.** The invariant is enforced once, at the
boundary, instead of defensively everywhere; the type documents intent.
Sacrifices a constructor and conversions at the edges where raw primitives enter.

**Detector status:** could-build — `types.Info.Uses` / `go-func-refs` to find a
primitive threaded through many signatures with repeated validation (the
"primitive obsession" smell). Folds in **replace-type-code-with-subtype** — an
`int`/`string` tag promoted to a distinct type is the same move.

---

Before — `email` is a bare string, validated inline:

```go
import (
	"errors"
	"strings"
)

func sendWelcome(email string) error {
	if !strings.Contains(email, "@") {
		return errors.New("invalid email")
	}
	return nil
}
```

After — `Email` enforces its invariant by construction:

```go
import (
	"errors"
	"strings"
)

type Email struct{ addr string }

func NewEmail(s string) (Email, error) {
	if !strings.Contains(s, "@") {
		return Email{}, errors.New("invalid email")
	}
	return Email{s}, nil
}

func (e Email) String() string { return e.addr }

func sendWelcome(e Email) error {
	return nil // e is valid by construction
}
```
