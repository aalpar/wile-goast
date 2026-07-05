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

// Package callsite is a fixture for the single-call-site checker. It reproduces
// the "single apply seam vs two hand-written arms" shape behind wile finding
// #9's belief B3: a capture primitive should apply its callback through ONE
// call site, with mode expressed as a target selection, not two duplicated
// ApplyCallable arms. The checker counts SSA call instructions to the op:
//
//   - SingleSeam  one apply(), mode is a target select -> 'single   (the B3
//     conforming shape after the dual-mode restructure)
//   - TwoArms     apply() duplicated across two branches -> 'multiple (the
//     pre-restructure hazard: two arms that can silently diverge)
//   - NoApply     no apply() at all                      -> 'missing
package callsite

// sink is a package-level effect so the fixture calls are not optimized away.
var sink int

type ctx struct{ id int }

// apply is the callback-application op the checker counts.
func (c *ctx) apply(v int) {
	sink += c.id + v
}

func newSub() *ctx {
	sink++
	return &ctx{sink}
}

// SingleSeam applies through ONE call site; the branch only selects the target
// context (ambient vs a fresh sub), not the apply itself. This is the B3
// conforming shape: single-call-site reports 'single.
func SingleSeam(rootless bool, v int) {
	target := &ctx{0}
	if rootless {
		target = newSub()
	}
	target.apply(v)
}

// TwoArms duplicates the apply across both branches — the pre-#9 hazard where
// two hand-written arms can drift apart. single-call-site reports 'multiple.
func TwoArms(rootless bool, v int) {
	if !rootless {
		c := &ctx{0}
		c.apply(v)
		return
	}
	sub := newSub()
	sub.apply(v)
}

// NoApply never applies the callback: single-call-site reports 'missing.
func NoApply(v int) {
	sink += v
}
