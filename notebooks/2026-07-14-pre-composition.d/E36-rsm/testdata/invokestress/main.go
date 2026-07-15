// invokestress — a stress corpus for receiver-TYPE-scoped invoke discovery.
//
// Three interfaces (Handler, Filter, Stage) share the method name "Process" but
// have DISJOINT method sets (OnStart / OnError / Reset), so each concrete type
// implements exactly one of them. The naive method-name-only caller discovery
// would, when backfilling a shared-name method's receiver param on a single
// seed, activate all three interfaces' Process() invoke sites program-wide.
// Receiver-type-scoped discovery keeps only the site(s) whose interface the
// receiver type actually implements.
//
// Each driver dispatches Process through a different interface, then invokes
// Emit() on the returned payload's tag. Each path resolves to a DISTINCT Tagger
// concrete type (Alpha/Beta/Gamma), so:
//   - if the narrowing DROPS a real caller, the Emit receiver under-approximates
//     on a single seed (PROBE fails);
//   - if it wires the WRONG interface's callers, DISPATCH over-approximates.
// Passing DISPATCH exact + PROBE 0 proves the narrowing keeps every real caller.
package main

// Sink keeps the dispatch results live without importing fmt (which would drag
// in os/runtime and their context-collapsed heap floods, unrelated to what this
// corpus exercises).
var Sink int

type Handler interface {
	Process() Payload
	OnStart()
}
type Filter interface {
	Process() Payload
	OnError()
}
type Stage interface {
	Process() Payload
	Reset()
}

type Payload struct{ tag Tagger }

type Tagger interface{ Emit() int }

type AlphaTag struct{}

func (AlphaTag) Emit() int { return 1 }

type BetaTag struct{}

func (BetaTag) Emit() int { return 2 }

type GammaTag struct{}

func (GammaTag) Emit() int { return 3 }

// HImpl implements Handler ONLY.
type HImpl struct{ p Payload }

func (h *HImpl) Process() Payload { return h.p }
func (h *HImpl) OnStart()         {}

// FImpl implements Filter ONLY.
type FImpl struct{ p Payload }

func (f *FImpl) Process() Payload { return f.p }
func (f *FImpl) OnError()         {}

// SImpl implements Stage ONLY.
type SImpl struct{ p Payload }

func (s *SImpl) Process() Payload { return s.p }
func (s *SImpl) Reset()           {}

// Each driver: invoke Process through one interface, then invoke Emit on the
// returned payload's tag. The Emit receiver's single-seed pts requires
// backfilling the shared-name Process receiver via invoke discovery.
func driveHandler(h Handler) int {
	res := h.Process()
	return res.tag.Emit()
}
func driveFilter(f Filter) int {
	res := f.Process()
	return res.tag.Emit()
}
func driveStage(s Stage) int {
	res := s.Process()
	return res.tag.Emit()
}

func main() {
	h := &HImpl{p: Payload{tag: AlphaTag{}}}
	f := &FImpl{p: Payload{tag: BetaTag{}}}
	s := &SImpl{p: Payload{tag: GammaTag{}}}
	Sink = driveHandler(h) + driveFilter(f) + driveStage(s)
}
