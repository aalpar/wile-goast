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

func TestSplit_ImportSignatures(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(import (wile goast utils))

		(define refs (go-func-refs
		  "github.com/aalpar/wile-goast/goast/testdata/iface"))
		(define sigs (import-signatures refs))
	`)

	c := qt.New(t)

	t.Run("returns alist", func(t *testing.T) {
		result := eval(t, engine, `(pair? (car sigs))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("car is function name", func(t *testing.T) {
		result := eval(t, engine, `(string? (caar sigs))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("cdr is list of package paths", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((first-sig (car sigs)))
			  (or (null? (cdr first-sig))
			      (string? (cadr first-sig))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestSplit_ComputeIDF(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))

		;; 3 functions: io appears in all 3 (IDF=0), fmt in 1 (IDF=log(3))
		(define sigs '(("F1" "io" "fmt")
		               ("F2" "io" "strings")
		               ("F3" "io")))
		(define idf (compute-idf sigs))
	`)

	c := qt.New(t)

	t.Run("io has IDF 0", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc "io" idf))`)
		c.Assert(result.SchemeString(), qt.Equals, "0.0")
	})

	t.Run("fmt has positive IDF", func(t *testing.T) {
		result := eval(t, engine, `(> (cdr (assoc "fmt" idf)) 0)`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestSplit_FilterNoise(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))

		(define sigs '(("F1" "io" "fmt")
		               ("F2" "io" "strings")
		               ("F3" "io")))
		(define idf (compute-idf sigs))
		(define filtered (filter-noise sigs idf))
	`)

	c := qt.New(t)

	t.Run("io removed (IDF=0 < threshold)", func(t *testing.T) {
		result := eval(t, engine, `
			(member "io" (cdr (assoc "F1" filtered)))`)
		c.Assert(result.SchemeString(), qt.Equals, "#f")
	})

	t.Run("fmt preserved (IDF > threshold)", func(t *testing.T) {
		result := eval(t, engine, `
			(not (equal? #f (member "fmt" (cdr (assoc "F1" filtered)))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestSplit_BuildPackageContext(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(import (wile goast fca))

		(define filtered '(("F1" "fmt" "strings")
		                    ("F2" "fmt")
		                    ("F3" "strings")))
		(define ctx (build-package-context filtered))
	`)

	c := qt.New(t)

	t.Run("3 objects", func(t *testing.T) {
		result := eval(t, engine, `(length (context-objects ctx))`)
		c.Assert(result.SchemeString(), qt.Equals, "3")
	})

	t.Run("2 attributes", func(t *testing.T) {
		result := eval(t, engine, `(length (context-attributes ctx))`)
		c.Assert(result.SchemeString(), qt.Equals, "2")
	})

	t.Run("functions with no deps excluded", func(t *testing.T) {
		result := eval(t, engine, `
			(define filtered2 '(("F1" "fmt") ("F2")))
			(define ctx2 (build-package-context filtered2))
			(length (context-objects ctx2))`)
		c.Assert(result.SchemeString(), qt.Equals, "1")
	})
}

func TestSplit_RefineByAPISurface(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(import (wile goast fca))

		;; Simulate go-func-refs output
		(define raw-refs
		  '((func-ref (name . "F1")
		              (pkg . "test/pkg")
		              (refs . ((ref (pkg . "io") (objects . ("Reader" "Writer")))
		                       (ref (pkg . "fmt") (objects . ("Println"))))))
		    (func-ref (name . "F2")
		              (pkg . "test/pkg")
		              (refs . ((ref (pkg . "io") (objects . ("Closer")))
		                       (ref (pkg . "fmt") (objects . ("Sprintf"))))))))
		;; Only io is high-IDF
		(define filtered '(("F1" "io") ("F2" "io")))
		(define ctx (refine-by-api-surface raw-refs filtered))
	`)

	c := qt.New(t)

	t.Run("attributes are pkg:object pairs", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((attrs (context-attributes ctx)))
			  (and (member "io:Reader" attrs)
			       (member "io:Closer" attrs)))`)
		c.Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})

	t.Run("F1 and F2 have different attributes", func(t *testing.T) {
		result := eval(t, engine, `
			(equal? (intent ctx '("F1"))
			        (intent ctx '("F2")))`)
		c.Assert(result.SchemeString(), qt.Equals, "#f")
	})
}
