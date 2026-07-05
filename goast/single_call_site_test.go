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

// TestSingleCallSite exercises the single-call-site checker against the callsite
// fixture. The checker counts SSA call instructions to an op and reports whether
// the site function applies it through exactly ONE seam ('single), duplicates it
// across arms ('multiple), or never calls it ('missing). It is the structural
// acceptance test behind wile finding #9's belief B3
// (callcc-mode-selection-single-seam): after the PrimCallCC / cwcc dual-mode
// restructure, each capture primitive applies its callback through one seam, so
// a future re-split into two arms is caught as 'multiple.
func TestSingleCallSite(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/callsite"))

		(define sites ((all-functions-in) ctx))
		(define checker (single-call-site "apply"))
		(define classified
		  (map (lambda (site) (cons (nf site 'name) (checker site ctx))) sites))

		(define (verdict-for short)
		  (let loop ((cs classified))
		    (cond ((null? cs) #f)
		          ((string-contains (car (car cs)) short) (cdr (car cs)))
		          (else (loop (cdr cs))))))
	`)

	t.Run("one apply seam is single", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".SingleSeam")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "single")
	})

	t.Run("two hand-written arms are multiple", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".TwoArms")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "multiple")
	})

	t.Run("no apply call is missing", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".NoApply")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "missing")
	})

	t.Run("only SingleSeam fully adheres", func(t *testing.T) {
		// Deviation count, mirroring flows_to_all_test.go / dominates_call_test.go:
		// of the 5 functions in the fixture (apply, newSub, SingleSeam, TwoArms,
		// NoApply), only SingleSeam is 'single, so 4 deviate.
		devs := eval(t, engine, `
			(length (filter-map
			          (lambda (p) (and (not (eq? (cdr p) 'single)) p))
			          classified))`)
		qt.New(t).Assert(devs.SchemeString(), qt.Equals, "4")
	})
}
