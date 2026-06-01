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
)

func TestProvenanceInstrPos(t *testing.T) {
	engine := newBeliefEngine(t)

	result := eval(t, engine, `
		(import (wile goast provenance))
		(and (equal? (ssa-instr-pos '(ssa-call (pos . "foo.go:10:3") (func . "Lock")))
		             "foo.go:10:3")
		     (eq? (ssa-instr-pos '(ssa-call (func . "Lock"))) #f))
	`)
	qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
}

func TestProvenanceCallPosition(t *testing.T) {
	engine := newBeliefEngine(t)

	// ssa-call-position finds the first matching call's position, #f otherwise.
	// A literal block with one non-call instr and one positioned call.
	result := eval(t, engine, `
		(import (wile goast provenance))
		(let ((block '(ssa-block (index . 0)
		                (instrs (ssa-binop (name . "t0"))
		                        (ssa-call (pos . "bar.go:7:5") (func . "Unlock"))))))
		  (and (equal? (ssa-call-position block "Unlock") "bar.go:7:5")
		       (eq? (ssa-call-position block "Nope") #f)))
	`)
	qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
}
