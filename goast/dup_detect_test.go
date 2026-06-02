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
