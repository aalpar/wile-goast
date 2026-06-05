# Convert method to function

**Name:** convert method to function (`before → after`). The inverse —
attaching a free function to a type as a method — is correct when the function
genuinely operates on that type's state; this transform applies in the opposite
case, where the receiver is a namespace, not the subject.

**Precondition.** A method reads exactly one receiver field, writes none, and
takes at least one parameter, and that single receiver read is used as *data*
combined with a parameter rather than as the *subject* of a delegated call. The
receiver is then a context holder, not the entity the operation acts on — the
"receiver as namespace" anti-pattern (Page-Jones's Connascence of Meaning: the
received field and the parameter must agree on a shared semantic, and the method
form hides that contract). Here `(*Parser).wrapMidParseEOF` reads only `p.cur`,
and `p.cur` must be the token at which the parameter `err` occurred.

**Transform (`before → after`), steps:**

1. **Confirm the shape.** One receiver field read, no receiver writes, ≥1
   parameter, the read combined with a parameter (not the receiver of a call).
2. **Lift the receiver field to a parameter.** `p.cur` becomes an explicit
   `tok Token` argument; drop the receiver.
3. **Rewrite call sites** to pass the field explicitly: `p.wrapMidParseEOF(p.err,
   "list")` becomes `wrapMidParseEOF(p.err, p.cur, "list")`.

**What it optimizes / sacrifices.** It moves a hidden correspondence from
invisible (you must read the body to learn `p.cur` and `err` must match) to
visible (every call supplies both, so a future desync forces a call-site audit).
The connascence level is unchanged — still Connascence of Meaning — but its
locality improves. It sacrifices the grouping the receiver provided: if the field
is genuinely shared state across several methods (a traversal's `visited` set,
say), the method form is the right home and converting would scatter the state.
Convert only when the receiver is incidental context, not shared state.

**Detector status:** could-build — the `receiver-parameter-asymmetry` checker in
`(wile goast belief)` classifies each method as `candidate` / `forwarder` /
`mutation` / `accessor` / `multi-read` / `unused-recv` / `interface-method`; the
`candidate` verdict is exactly this refactoring's precondition, emitted as a
located finding pointing at the single receiver read. The forwarder/candidate
split (delegation vs. subject-inversion) is decided by call-subject position in
SSA. The conversion itself is left to a human — the receiver may be intentional.

---

Supporting declarations (shared by both versions):

```go
type Token int

func newErr(tok Token, base error, form string) error {
	return fmt.Errorf("%w: unterminated %s at token %d", base, form, tok)
}

type Parser struct {
	cur Token
	err error
}
```

Before — `wrapMidParseEOF` is a method; `p.cur` is an implicit context read,
and its required correspondence with `err` is invisible at call sites:

```go
import (
	"errors"
	"fmt"
	"io"
)

func (p *Parser) wrapMidParseEOF(err error, form string) error {
	if errors.Is(err, io.EOF) {
		return newErr(p.cur, io.ErrUnexpectedEOF, form)
	}
	return err
}

func (p *Parser) use() error { return p.wrapMidParseEOF(p.err, "list") }
```

After — a free function; the token is an explicit parameter, so the `err`/`tok`
correspondence is visible in every call:

```go
func wrapMidParseEOF(err error, tok Token, form string) error {
	if errors.Is(err, io.EOF) {
		return newErr(tok, io.ErrUnexpectedEOF, form)
	}
	return err
}

func (p *Parser) use() error { return wrapMidParseEOF(p.err, p.cur, "list") }
```
