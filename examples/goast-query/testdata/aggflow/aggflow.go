// Copyright 2026 Aaron Alpar
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package aggflow is a fixture for the flows-to-all checker. It reproduces the
// value-through-aggregate shape that defeats a plain def-use walk: a captured
// value is boxed and passed as a VARIADIC argument, so in SSA it is stored into
// a backing array and the callee receives a SLICE of that array. The def-use
// chain from the captured value to the call therefore runs
//
//	capture() -> make-interface -> store(&arr[0], box) -> slice(arr) -> apply(slice)
//
// A generic def-use reachability under-approximates this: the store taints the
// element address (&arr[0]), but the slice reads the backing array (arr), and
// there is no alias edge connecting them. flows-to-all adds that edge (a store
// through an element/field address taints the aggregate), so it can see the
// captured value reach apply() in BOTH arms. This mirrors PrimCallCC, where the
// captured continuation `capt` is passed as `mc.ApplyCallable(mcls, capt)` — a
// variadic ...Value — in both mode arms (wile finding #9, belief B2).
package aggflow

// sink is a package-level effect so the fixture calls are not optimized away.
var sink int

type box struct{ v int }

func capture() *box {
	sink++
	return &box{sink}
}

// apply takes its arguments variadically over an interface type, so a call
// apply(b) with a concrete *box packs b via make-interface into a []any backing
// array and passes a slice — the exact aggregate shape flows-to-all must trace.
func apply(vs ...any) {
	for _, v := range vs {
		if bb, ok := v.(*box); ok {
			sink += bb.v
		}
	}
}

// FlowsAll: one capture flows to apply() in BOTH arms through the variadic
// packing. A generic def-use walk reports this as unreached (the aggregate gap);
// flows-to-all reports 'flows-all. This is the teeth for the aggregate-alias
// edge itself, and the conforming shape for B2.
func FlowsAll(c bool) {
	b := capture()
	if c {
		apply(b)
	} else {
		apply(b)
	}
}

// SplitCapture: each arm captures its OWN value, so no single capture reaches
// both apply() sites. This is the B2 violation (an arm re-captures instead of
// sharing the one continuation) — flows-to-all reports 'partial.
func SplitCapture(c bool) {
	if c {
		b := capture()
		apply(b)
	} else {
		b := capture()
		apply(b)
	}
}

// NoFlow: capture happens (side effect) but a DIFFERENT value is applied on
// every path, so the captured value reaches no apply() site — 'none.
func NoFlow(c bool) {
	_ = capture()
	other := &box{99}
	if c {
		apply(other)
	} else {
		apply(other)
	}
}

// MissingCapture: no capture() call at all — the source op is absent, 'missing.
func MissingCapture(c bool) {
	other := &box{1}
	apply(other)
}
