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

package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/aalpar/wile/values"
)

// TestIFDS_ValidPathCanary builds a 3-call-site analysis:
//
//	a1 and a2 each call p with call-site id 1.
//	b2 calls p with call-site id 2.
//
// The grammar allows matched open-i/close-i transitions (a1->a2 via call-site
// 1: open-1 then close-1) but rejects mismatched ones (a1->b2: open-1 then
// close-2 — wrong-caller return, the key precision over boolean reachability).
func TestIFDS_ValidPathCanary(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast ifds))`)
	eval(t, engine, `
		(define nodes '("a1" "a2" "b2" "p"))
		(define call-sites (list (list "a1" 1 "p") (list "a2" 1 "p") (list "b2" 2 "p")))
		(define A (make-ifds-analysis nodes call-sites))`)

	// a1 -> a2: open-1 (a1 calls p) then close-1 (a2 is also a caller of p at
	// call-site 1), so the path open-1 close-1 is balanced — MUST be reachable.
	result := eval(t, engine, `(ifds-reachable? A "a1" "a2")`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// a1 -> b2: open-1 (a1 calls p) then close-2 (b2 uses call-site 2) —
	// mismatched brackets; the grammar rejects this — MUST NOT be reachable.
	result = eval(t, engine, `(ifds-reachable? A "a1" "b2")`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)
}
