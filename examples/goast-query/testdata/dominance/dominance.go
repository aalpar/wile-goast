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

// Package dominance is a fixture for the dominates-call checker. Each function
// calls apply() in one or both arms of a branch; capture() sits in a position
// that dominates all, some, or none of those apply() call sites. The
// PartialDominance case is the one dominates-call catches but `ordered` (which
// compares only the first call block of each op) can miss: capture dominates the
// first apply but not the second.
package dominance

// sink is a package-level effect so the fixture calls are not optimized away.
var sink int

func capture() { sink++ }

func apply() { sink += 2 }

// DominatesAll: capture precedes the branch, so it dominates BOTH arms' apply.
// This is the correct "capture dominates every mode arm" shape.
func DominatesAll(c bool) {
	capture()
	if c {
		apply()
	} else {
		apply()
	}
}

// PartialDominance: capture is inside one arm only, so it dominates that arm's
// apply but NOT the other arm's. The regression shape (capture moved into a
// single mode arm) — dominates-call reports 'partial where `ordered` can pass.
func PartialDominance(c bool) {
	if c {
		capture()
		apply()
	} else {
		apply()
	}
}

// NoDominance: capture runs after apply on every path, so it dominates no apply
// call site.
func NoDominance(c bool) {
	if c {
		apply()
	} else {
		apply()
	}
	capture()
}

// MissingCapture: no capture call at all — the OP-A site is absent.
func MissingCapture(c bool) {
	if c {
		apply()
	} else {
		apply()
	}
}
