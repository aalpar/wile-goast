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

// TestFlowsToAll exercises the flows-to-all checker against the aggflow fixture.
// The fixture passes a captured value to apply() VARIADICALLY, so in SSA the
// value is boxed, stored into a backing array, and handed over as a slice — the
// value-through-aggregate shape that a plain def-use walk cannot follow. The
// four functions sit so that capture()'s value flows to all, some, or none of
// the apply() call sites:
//   - FlowsAll       one capture reaches BOTH arms  -> 'flows-all
//   - SplitCapture   each arm re-captures its own    -> 'partial   (the B2
//     violation: no single capture reaches both callbacks)
//   - NoFlow         a different value is applied     -> 'none
//   - MissingCapture no capture at all               -> 'missing
//
// This is the checker behind wile finding #9's callcc-same-capture-to-both-arms
// belief (B2). It is the value-flow analog of dominates-call: dominates-call
// asks whether the capture DOMINATES every callback arm (control); flows-to-all
// asks whether the same captured VALUE reaches every callback arm (data). The
// aggregate-alias edge is what makes FlowsAll report 'flows-all rather than the
// 'none a generic defuse-reachable? returns.
func TestFlowsToAll(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/aggflow"))

		;; Sites: every function that calls apply (all four fixture functions).
		;; capture/apply themselves are leaves and excluded.
		(define sites ((functions-matching (contains-call "apply")) ctx))

		(define checker (flows-to-all "capture" "apply"))
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

	t.Run("one capture reaches both arms through variadic packing", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".FlowsAll")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "flows-all")
	})

	t.Run("re-capturing per arm is only partial", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".SplitCapture")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "partial")
	})

	t.Run("applying a different value reaches nothing", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".NoFlow")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "none")
	})

	t.Run("no capture is missing", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".MissingCapture")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "missing")
	})

	t.Run("only FlowsAll fully adheres", func(t *testing.T) {
		devs := eval(t, engine, `
			(length (filter-map
			          (lambda (p) (and (not (eq? (cdr p) 'flows-all)) p))
			          classified))
		`)
		qt.New(t).Assert(devs.SchemeString(), qt.Equals, "3")
	})
}
