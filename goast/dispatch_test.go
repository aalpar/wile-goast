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

	"github.com/aalpar/wile/pkg/values"
)

const dispatchPkg = `"github.com/aalpar/wile-goast/goast/testdata/dispatch"`

// TestDispatch_MustSite: one implementor flows to the site, so VTA's SOUND set is
// a singleton — the true callee set is a subset of {(OnlyImpl).S}. That is a
// genuine `must`, and it needs no analysis beyond counting.
func TestDispatch_MustSite(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast dispatch) (wile goast utils))`)
	eval(t, engine, `(define ds (dispatch-sites `+dispatchPkg+`))`)
	// The iface field is a FULL type string, e.g.
	// "github.com/aalpar/wile-goast/goast/testdata/dispatch.Single" — match on the
	// suffix, never on a guessed fully-qualified name.
	eval(t, engine, `
		(define must-site
		  (let loop ((l ds))
		    (cond ((null? l) #f)
		          ((string-contains? (or (dispatch-iface (car l)) "") "Single") (car l))
		          (else (loop (cdr l))))))`)

	c.Assert(eval(t, engine, `(if must-site #t #f)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(eq? (dispatch-class must-site) 'must)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(= (dispatch-n must-site) 1)`).Internal(), qt.Equals, values.TrueValue)
}

// TestDispatch_MaySite_PrunesTheDecoy: THE property the library exists for. Decoy
// implements Multi and is allocated, but never converted to Multi. CHA folds it in;
// VTA prunes it. n must be 3, and `Decoy` must not appear among the candidates.
// If this fails, the library is reporting a bound formatted as a fact.
func TestDispatch_MaySite_PrunesTheDecoy(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast dispatch) (wile goast utils))`)
	eval(t, engine, `(define ds (dispatch-sites `+dispatchPkg+`))`)
	eval(t, engine, `
		(define may-site
		  (let loop ((l ds))
		    (cond ((null? l) #f)
		          ((and (eq? (dispatch-class (car l)) 'may)
		                (= (dispatch-n (car l)) 3)) (car l))
		          (else (loop (cdr l))))))`)

	c.Assert(eval(t, engine, `(if may-site #t #f)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(= (dispatch-n may-site) 3)`).Internal(), qt.Equals, values.TrueValue)

	// narrowed-from records CHA's count at the same site: it must exceed n,
	// which is the evidence that VTA actually pruned something.
	c.Assert(eval(t, engine, `(> (dispatch-narrowed-from may-site) 3)`).Internal(),
		qt.Equals, values.TrueValue)

	// The decoy must be absent from the candidate set.
	c.Assert(eval(t, engine, `
		(let loop ((cs (dispatch-candidates may-site)))
		  (cond ((null? cs) #f)
		        ((string-contains? (nf (car cs) 'callee) "Decoy") #t)
		        (else (loop (cdr cs)))))`).Internal(), qt.Equals, values.FalseValue)
}

// TestDispatch_IsAFinding: a dispatch site IS a (wile goast provenance) finding, so
// render-finding and every other finding consumer work on it unchanged. `score` is
// #f: no natural confidence exists here and inventing one would be a fabrication.
func TestDispatch_IsAFinding(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast dispatch) (wile goast provenance))`)
	eval(t, engine, `(define d (car (dispatch-sites `+dispatchPkg+`)))`)

	c.Assert(eval(t, engine, `(symbol? (finding-value d))`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(eq? (finding-score d) #f)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(eq? (car (finding-why d)) 'dispatch)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(string? (render-finding d))`).Internal(), qt.Equals, values.TrueValue)
}
