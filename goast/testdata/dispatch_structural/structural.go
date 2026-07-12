// Package dispatch_structural reproduces type-exported?'s blind spot: an
// ANONYMOUS/STRUCTURAL interface arrives as a TYPE LITERAL, not a qualified
// type name. Any package anywhere can structurally satisfy it, so `must` here
// is MORE scope-limited than a named exported interface, not less -- the
// opposite of what a naive #f ("not exported") would suggest.
package dispatch_structural

type Closer struct{}

func (Closer) Close() error { return nil }

// UseAnon dispatches on an ANONYMOUS interface literal, not a named type.
func UseAnon(c interface{ Close() error }) {
	c.Close()
}

func CallAnon() {
	UseAnon(Closer{})
}
