// Package dispatch_synthetic reproduces a PHANTOM dispatch site: a
// compiler-generated forwarding function (ssa.Function.Synthetic != "")
// whose single invoke instruction has no source position -- it does not
// exist as a call site in source at all.
package dispatch_synthetic

type Ifc interface{ M() int }

type Impl struct{}

func (Impl) M() int { return 1 }

// BoundSite takes a METHOD VALUE (not an immediate call): the SSA builder
// synthesizes a "bound" forwarding function whose body is exactly one
// interface invoke with no source position.
func BoundSite() int {
	var x Ifc = Impl{}
	f := x.M
	return f()
}
