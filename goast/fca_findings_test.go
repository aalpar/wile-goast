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
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestFieldIndexToPositions(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/falseboundary"
	eval(t, engine, `
		(import (wile goast fca))
		(define s (go-load "`+pkg+`"))
		(define idx (go-ssa-field-index s))
		(define pos-index (field-index->positions idx))
	`)

	t.Run("UpdateBoth resolves to a source position", func(t *testing.T) {
		out := eval(t, engine, `(hashtable-ref pos-index "`+pkg+`.UpdateBoth" #f)`).SchemeString()
		c.Assert(out, qt.Not(qt.Equals), "#f")
		c.Assert(strings.Contains(out, "falseboundary.go:"), qt.IsTrue, qt.Commentf("%s", out))
	})

	t.Run("absent function resolves to #f", func(t *testing.T) {
		out := eval(t, engine, `(hashtable-ref pos-index "does.not.Exist" #f)`).SchemeString()
		c.Assert(out, qt.Equals, "#f")
	})
}

func TestBoundaryFindings(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/falseboundary"
	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast provenance))
		(define s (go-load "`+pkg+`"))
		(define idx (go-ssa-field-index s))
		(define pos-index (field-index->positions idx))
		(define ctx (field-index->context idx 'write-only))
		(define lat (concept-lattice ctx))
		(define xb  (cross-boundary-concepts lat))
		(define bf  (boundary-findings xb pos-index))
		(define entry
		  (let loop ((rs bf))
		    (if (null? rs) #f
		      (let ((types (cdr (assoc 'types (car rs)))))
		        (if (and (member "Cache" types) (member "Index" types))
		          (car rs) (loop (cdr rs)))))))
		(define entry-findings (cdr (assoc 'findings entry)))
	`)

	t.Run("entry was found", func(t *testing.T) {
		out := eval(t, engine, `(if entry #t #f)`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("findings length equals extent-size", func(t *testing.T) {
		out := eval(t, engine, `
			(= (length entry-findings) (cdr (assoc 'extent-size entry)))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("every member is located at falseboundary.go", func(t *testing.T) {
		// Each finding's where is a string; assert via the rendered category so
		// we do not depend on a Scheme substring builtin.
		out := eval(t, engine, `(render-category "false-boundary" entry-findings)`).SchemeString()
		c.Assert(strings.Count(out, "falseboundary.go:"), qt.Equals, 3, qt.Commentf("%s", out))
	})

	t.Run("render-category produces the editor-walk header", func(t *testing.T) {
		out := eval(t, engine, `(render-category "false-boundary" entry-findings)`).SchemeString()
		c.Assert(strings.Contains(out, "false-boundary (3)"), qt.IsTrue, qt.Commentf("%s", out))
	})

	t.Run("why carries the shared intent (fields + types)", func(t *testing.T) {
		out := eval(t, engine, `
			(render-why (finding-why (car entry-findings)))
		`).SchemeString()
		c.Assert(strings.Contains(out, "cross-boundary"), qt.IsTrue, qt.Commentf("%s", out))
		c.Assert(strings.Contains(out, "Cache"), qt.IsTrue, qt.Commentf("%s", out))
		c.Assert(strings.Contains(out, "Index"), qt.IsTrue, qt.Commentf("%s", out))
	})

	t.Run("why is structured: a script can filter on participating types", func(t *testing.T) {
		// The why is (cross-boundary (fields . ...) (types . (...))); a downstream
		// script reads the 'types entry directly, not the rendered string.
		out := eval(t, engine, `
			(let* ((why (finding-why (car entry-findings)))
			       (data (cdr why))
			       (types (cdr (assoc 'types data))))
			  (and (member "Cache" types) (member "Index" types) #t))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})
}
