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

// TestReachesCall exercises the reaches-call transitive-reachability checker
// against the reachability fixture: Direct/OneHop/TwoHop all reach Target
// through the call graph (0, 1, and 2 hops), while Bad reaches only helper and
// is the sole deviation. This is the checker behind staff-sweep #1 Tier-2's
// continuation-capture-marks-shared belief.
func TestReachesCall(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/reachability"))

		;; Sites: every function whose body calls one of the on-path names or the
		;; off-path helper — i.e. Direct, OneHop, TwoHop (reach Target) and Bad
		;; (does not). Target itself is excluded (it has no outgoing call).
		(define sites
		  ((functions-matching
		     (any-of (contains-call "Target")
		             (contains-call "Direct")
		             (contains-call "OneHop")
		             (contains-call "helper"))) ctx))

		(define checker (reaches-call "Target"))
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

	t.Run("direct call reaches", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".Direct")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "reaches")
	})

	t.Run("one hop reaches", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".OneHop")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "reaches")
	})

	t.Run("two hops reaches", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".TwoHop")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "reaches")
	})

	t.Run("off-path caller is unreached", func(t *testing.T) {
		v := eval(t, engine, `(verdict-for ".Bad")`)
		qt.New(t).Assert(v.SchemeString(), qt.Equals, "unreached")
	})

	t.Run("exactly one deviation", func(t *testing.T) {
		devs := eval(t, engine, `
			(length (filter-map
			          (lambda (p) (and (not (eq? (cdr p) 'reaches)) p))
			          classified))
		`)
		qt.New(t).Assert(devs.SchemeString(), qt.Equals, "1")
	})
}
