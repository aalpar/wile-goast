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

func TestFuncRefsToPositions(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(define refs (go-func-refs "`+pkg+`"))
		(define pos-index (func-refs->positions refs))
	`)

	t.Run("EncodeA resolves to dupcluster.go", func(t *testing.T) {
		out := eval(t, engine, `(hashtable-ref pos-index "EncodeA" #f)`).SchemeString()
		c.Assert(out, qt.Not(qt.Equals), "#f")
		c.Assert(strings.Contains(out, "dupcluster.go:"), qt.IsTrue, qt.Commentf("%s", out))
	})

	t.Run("absent name resolves to #f", func(t *testing.T) {
		out := eval(t, engine, `(hashtable-ref pos-index "Nope" #f)`).SchemeString()
		c.Assert(out, qt.Equals, "#f")
	})
}

func TestDuplicateCandidateConcepts(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(import (wile goast fca))
		(define refs (go-func-refs "`+pkg+`"))
		(define ctx (function-ref-context refs))
		(define lat (concept-lattice ctx))
		(define cands (duplicate-candidate-concepts lat))
	`)

	t.Run("at least the json and log clusters", func(t *testing.T) {
		out := eval(t, engine, `(>= (length cands) 2)`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("a concept shares encoding/json across 3 functions", func(t *testing.T) {
		out := eval(t, engine, `
			(let loop ((cs cands))
			  (if (null? cs) #f
			    (let ((int (concept-intent (car cs)))
			          (ext (concept-extent (car cs))))
			      (if (and (member "encoding/json" int) (= (length ext) 3))
			        #t (loop (cdr cs))))))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})
}

func TestDupCandidateFindings(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(import (wile goast provenance))
		(define bf (find-duplicate-candidates "`+pkg+`"))
		(define entry
		  (let loop ((es bf))
		    (if (null? es) #f
		      (let ((refs (cdr (assoc 'refs (car es)))))
		        (if (member "encoding/json" refs) (car es) (loop (cdr es)))))))
		(define entry-findings (cdr (assoc 'findings entry)))
	`)

	t.Run("json cluster has 3 findings matching extent-size", func(t *testing.T) {
		out := eval(t, engine, `
			(and entry
			     (= (length entry-findings) (cdr (assoc 'extent-size entry)))
			     (= (length entry-findings) 3))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("every member is located at dupcluster.go", func(t *testing.T) {
		out := eval(t, engine, `(render-category "duplicate" entry-findings)`).SchemeString()
		c.Assert(strings.Count(out, "dupcluster.go:"), qt.Equals, 3, qt.Commentf("%s", out))
		c.Assert(strings.Contains(out, "duplicate (3)"), qt.IsTrue, qt.Commentf("%s", out))
	})

	t.Run("why carries the shared ref intent", func(t *testing.T) {
		out := eval(t, engine, `(render-why (finding-why (car entry-findings)))`).SchemeString()
		c.Assert(strings.Contains(out, "duplicate-candidate"), qt.IsTrue, qt.Commentf("%s", out))
		c.Assert(strings.Contains(out, "encoding/json"), qt.IsTrue, qt.Commentf("%s", out))
	})

	t.Run("why is structured: a script can filter on shared packages", func(t *testing.T) {
		out := eval(t, engine, `
			(let* ((why (finding-why (car entry-findings)))
			       (refs (cdr (assoc 'refs (cdr why)))))
			  (and (member "encoding/json" refs) #t))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})
}

func TestScoreCandidatePair(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(define s (go-load "`+pkg+`"))
		(define ast-index (build-func-ast-index (go-typecheck-package s)))
		(define ssa-index (build-func-ssa-index (go-ssa-build s)))
		(define m (score-candidate-pair "SumSlice" "TotalSlice" ast-index ssa-index 0.6))
	`)

	t.Run("the clone pair scores a real tier and similarity", func(t *testing.T) {
		out := eval(t, engine, `
			(and m
			     (memq (cdr (assoc 'equiv-tier m)) '(proven structural))
			     (> (cdr (assoc 'similarity m)) 0.6)
			     (>= (cdr (assoc 'benefit m)) 1))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("unresolvable names score #f", func(t *testing.T) {
		out := eval(t, engine, `(score-candidate-pair "Nope" "Nada" ast-index ssa-index 0.6)`).SchemeString()
		c.Assert(out, qt.Equals, "#f")
	})
}
