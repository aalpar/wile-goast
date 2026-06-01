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

// Task 1: the findings channel is additive — every site yields a finding,
// voting/adherence/deviations are unchanged, and (with ordered still bare in
// this task) findings are unlocated.
func TestBeliefFindingsChannel(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast provenance))
		(reset-beliefs!)
		(define-belief "ordering"
		  (sites (functions-matching
		           (all-of (contains-call "Validate") (contains-call "Process"))))
		  (expect (ordered "Validate" "Process"))
		  (threshold 0.5 1))
		(define results
		  (run-beliefs "github.com/aalpar/wile-goast/examples/goast-query/testdata/ordering"))
		(define r (car results))
		(define findings (cdr (assoc 'findings r)))
	`)

	t.Run("findings field present, one per site", func(t *testing.T) {
		got := eval(t, engine, `(= (length findings) (cdr (assoc 'total r)))`)
		qt.New(t).Assert(got.SchemeString(), qt.Equals, "#t")
	})

	t.Run("each finding value is its category", func(t *testing.T) {
		got := eval(t, engine, `
			(let loop ((fs findings))
			  (cond ((null? fs) #t)
			        ((memq (finding-value (car fs)) '(a-dominates-b b-dominates-a))
			         (loop (cdr fs)))
			        (else #f)))
		`)
		qt.New(t).Assert(got.SchemeString(), qt.Equals, "#t")
	})

	t.Run("voting fields unchanged", func(t *testing.T) {
		got := eval(t, engine, `
			(= (+ (length (cdr (assoc 'adherence r)))
			      (length (cdr (assoc 'deviations r))))
			   5)
		`)
		qt.New(t).Assert(got.SchemeString(), qt.Equals, "#t")
	})
}
