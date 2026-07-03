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

// TestDominatesCall exercises the dominates-call checker against the dominance
// fixture. capture()/apply() sit so that capture dominates all, some, or none
// of the apply() call sites:
//   - DominatesAll     capture precedes the branch -> 'dominates-all
//   - PartialDominance capture in one arm only      -> 'partial   (the case
//     `ordered` can miss: it checks only the first apply block)
//   - NoDominance      capture after apply          -> 'none
//   - MissingCapture   no capture at all            -> 'missing
//
// This is the checker behind wile finding #9's callcc-capture-dominates-modes
// belief. It generalizes `ordered` to require dominance over EVERY call site of
// the target, not just the first -- necessary because PrimCallCC calls the
// callback (ApplyCallable) in both mode arms.
func TestDominatesCall(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/dominance"))

		;; Sites: every function that calls apply (all four fixture functions).
		;; capture/apply themselves are leaves and excluded.
		(define sites ((functions-matching (contains-call "apply")) ctx))

		(define checker (dominates-call "capture" "apply"))
		(define classified
		  (map (lambda (site) (cons (nf site 'name) (checker site ctx))) sites))

		(define (verdict-for short)
		  (let loop ((cs classified))
		    (cond ((null? cs) #f)
		          ((string-contains (car (car cs)) short) (cdr (car cs)))
		          (else (loop (cdr cs))))))
	`)

	t.Run("4 sites found", func(t *testing.T) {
		total := eval(t, engine, `(length sites)`)
		qt.New(t).Assert(total.SchemeString(), qt.Equals, "4")
	})

	t.Run("capture before branch dominates both arms", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".DominatesAll")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "dominates-all")
	})

	t.Run("capture in one arm dominates only that arm", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".PartialDominance")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "partial")
	})

	t.Run("capture after apply dominates nothing", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".NoDominance")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "none")
	})

	t.Run("no capture is missing", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".MissingCapture")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "missing")
	})

	t.Run("only DominatesAll fully adheres", func(t *testing.T) {
		devs := eval(t, engine, `
			(length (filter-map
			          (lambda (p) (and (not (eq? (cdr p) 'dominates-all)) p))
			          classified))
		`)
		qt.New(t).Assert(devs.SchemeString(), qt.Equals, "3")
	})
}
