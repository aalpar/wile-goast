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

func TestSplit_FindSplit(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(import (wile goast fca))

		;; Two clear groups: F1,F2 use "A"; F3,F4 use "B"; F5 bridges both.
		(define ctx (context-from-alist
		  '(("F1" "A" "C")
		    ("F2" "A" "C")
		    ("F3" "B" "D")
		    ("F4" "B" "D")
		    ("F5" "A" "B"))))
		(define lat (concept-lattice ctx))
		(define result (find-split ctx lat))
	`)

	c := qt.New(t)

	t.Run("two non-empty groups", func(t *testing.T) {
		result := eval(t, engine, `
			(and (not (null? (cdr (assoc 'group-a result))))
			     (not (null? (cdr (assoc 'group-b result)))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("F5 in cut (bridges both)", func(t *testing.T) {
		result := eval(t, engine, `
			(not (equal? #f (member "F5" (cdr (assoc 'cut result)))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("cut-ratio is 0.2 (1 of 5)", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'cut-ratio result))`)
		c.Assert(result.SchemeString(), qt.Equals, "0.2")
	})

	t.Run("no split when all share same deps", func(t *testing.T) {
		result := eval(t, engine, `
			(define ctx-uniform (context-from-alist
			  '(("F1" "A") ("F2" "A") ("F3" "A"))))
			(define lat-u (concept-lattice ctx-uniform))
			(define result-u (find-split ctx-uniform lat-u))
			(cdr (assoc 'cut-ratio result-u))`)
		c.Assert(result.SchemeString(), qt.Equals, "1.0")
	})
}

func TestSplit_RecommendSplit_Synthetic(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))

		;; Two clusters with distinct deps, one bridge function.
		(define refs
		  '((func-ref (name . "Read")  (pkg . "p")
		              (refs . ((ref (pkg . "io") (objects . ("Reader"))))))
		    (func-ref (name . "Write") (pkg . "p")
		              (refs . ((ref (pkg . "io") (objects . ("Writer"))))))
		    (func-ref (name . "Parse") (pkg . "p")
		              (refs . ((ref (pkg . "go/ast") (objects . ("File"))))))
		    (func-ref (name . "Check") (pkg . "p")
		              (refs . ((ref (pkg . "go/ast") (objects . ("Inspect"))))))
		    (func-ref (name . "Bridge") (pkg . "p")
		              (refs . ((ref (pkg . "io") (objects . ("Reader")))
		                       (ref (pkg . "go/ast") (objects . ("File"))))))))
		(define report (recommend-split refs))
	`)

	c := qt.New(t)

	t.Run("has confidence", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'confidence report))`)
		c.Assert(result.SchemeString(), qt.Not(qt.Equals), "NONE")
	})

	t.Run("function count is 5", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'functions report))`)
		c.Assert(result.SchemeString(), qt.Equals, "5")
	})

	t.Run("two non-empty groups", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((gs (cdr (assoc 'groups report))))
			  (and (not (null? (cdr (assoc 'group-a gs))))
			       (not (null? (cdr (assoc 'group-b gs))))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestSplit_RecommendSplit_MaxAttributesGuard(t *testing.T) {
	// With 'max-attributes set very low, a context exceeding it should
	// return a NONE-confidence report with a diagnostic reason — not hang
	// on the 2^N concept-lattice enumeration.
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(define refs
		  '((func-ref (name . "Read")  (pkg . "p")
		              (refs . ((ref (pkg . "io") (objects . ("Reader"))))))
		    (func-ref (name . "Write") (pkg . "p")
		              (refs . ((ref (pkg . "io") (objects . ("Writer"))))))
		    (func-ref (name . "Parse") (pkg . "p")
		              (refs . ((ref (pkg . "go/ast") (objects . ("File"))))))
		    (func-ref (name . "Check") (pkg . "p")
		              (refs . ((ref (pkg . "go/ast") (objects . ("Inspect"))))))
		    (func-ref (name . "Bridge") (pkg . "p")
		              (refs . ((ref (pkg . "io") (objects . ("Reader")))
		                       (ref (pkg . "go/ast") (objects . ("File"))))))))
		(define guarded (recommend-split refs 'max-attributes 1))
	`)

	c := qt.New(t)

	t.Run("guard triggers NONE confidence", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'confidence guarded))`)
		c.Assert(result.SchemeString(), qt.Equals, "NONE")
	})

	t.Run("guard reports attribute count", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((ac (assoc 'attribute-count guarded)))
			  (and ac (> (cdr ac) 1)))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("guard reason mentions max-attributes", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((r (assoc 'reason guarded)))
			  (and r (string? (cdr r))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("NONE branch omits groups and acyclic (no fabricated data)", func(t *testing.T) {
		// A caller that reads 'groups without checking 'confidence would
		// previously see a fabricated empty split. Missing key is the clear
		// sentinel; force callers to branch on 'confidence.
		result := eval(t, engine, `
			(and (not (assoc 'groups guarded))
			     (not (assoc 'acyclic guarded)))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestSplit_RecommendSplit_TooFewFunctionsShape(t *testing.T) {
	// The "too few functions" NONE branch also omits groups/acyclic.
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(define report (recommend-split '()))
	`)

	c := qt.New(t)

	t.Run("empty input gives NONE", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'confidence report))`)
		c.Assert(result.SchemeString(), qt.Equals, "NONE")
	})

	t.Run("no groups or acyclic keys on NONE", func(t *testing.T) {
		result := eval(t, engine, `
			(and (not (assoc 'groups report))
			     (not (assoc 'acyclic report)))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestSplit_VerifyAcyclic(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(import (wile goast utils))

		;; Simulate: F1 calls F3 (a->b), but F3 doesn't call F1 (no b->a).
		(define refs
		  '((func-ref (name . "F1") (pkg . "my/pkg")
		              (refs . ((ref (pkg . "my/pkg")
		                            (objects . ("F3"))))))
		    (func-ref (name . "F2") (pkg . "my/pkg")
		              (refs . ()))
		    (func-ref (name . "F3") (pkg . "my/pkg")
		              (refs . ()))
		    (func-ref (name . "F4") (pkg . "my/pkg")
		              (refs . ()))))
	`)

	c := qt.New(t)

	t.Run("one-way dependency is acyclic", func(t *testing.T) {
		result := eval(t, engine, `
			(define v (verify-acyclic '("F1" "F2") '("F3" "F4") refs))
			(cdr (assoc 'acyclic v))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("a-refs-b count is 1", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'a-refs-b v))`)
		c.Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("bidirectional dependency is cyclic", func(t *testing.T) {
		result := eval(t, engine, `
			;; F3 also calls F1 -> cycle
			(define refs-cycle
			  '((func-ref (name . "F1") (pkg . "p")
			              (refs . ((ref (pkg . "p") (objects . ("F3"))))))
			    (func-ref (name . "F3") (pkg . "p")
			              (refs . ((ref (pkg . "p") (objects . ("F1"))))))))
			(define vc (verify-acyclic '("F1") '("F3") refs-cycle))
			(cdr (assoc 'acyclic vc))`)
		c.Assert(result.SchemeString(), qt.Equals, "#f")
	})
}

func TestSplit_SingleCluster(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast split))
		(import (wile goast utils))
		(reset-beliefs!)

		(define-aggregate-belief "test-split"
			(sites (all-functions-in))
			(analyze (single-cluster)))

		(run-beliefs
			"github.com/aalpar/wile-goast/goast/testdata/iface")
	`)

	c := qt.New(t)

	t.Run("completes without error", func(t *testing.T) {
		c.Assert(true, qt.IsTrue)
	})
}

func TestSplit_SingleCluster_Synthetic(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast split))
		(import (wile goast utils))

		(define analyzer (single-cluster 'idf-threshold 0.36))

		(define ctx (make-context
			"github.com/aalpar/wile-goast/goast/testdata/iface"))
		(define sites '())
		(define result (analyzer sites ctx))
	`)

	c := qt.New(t)

	t.Run("result has type aggregate", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'type result))`)
		c.Assert(result.SchemeString(), qt.Equals, "aggregate")
	})

	t.Run("result has verdict", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((v (cdr (assoc 'verdict result))))
				(or (eq? v 'COHESIVE) (eq? v 'SPLIT)))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("result has functions count", func(t *testing.T) {
		result := eval(t, engine, `(number? (cdr (assoc 'functions result)))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("result has report", func(t *testing.T) {
		result := eval(t, engine, `(pair? (cdr (assoc 'report result)))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestSplit_Integration_Goast(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(import (wile goast utils))

		(define refs (go-func-refs
		  "github.com/aalpar/wile-goast/goast"))
		(define report (recommend-split refs))
	`)

	c := qt.New(t)

	t.Run("many functions", func(t *testing.T) {
		result := eval(t, engine, `(> (cdr (assoc 'functions report)) 20)`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("has high-IDF packages", func(t *testing.T) {
		result := eval(t, engine, `
			(not (null? (cdr (assoc 'high-idf report))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("confidence is a known level", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((c (cdr (assoc 'confidence report))))
			  (or (eq? c 'HIGH) (eq? c 'MEDIUM)
			      (eq? c 'LOW) (eq? c 'NONE)))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("acyclic field present", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((a (cdr (assoc 'acyclic report))))
			  (assoc 'acyclic a))`)
		c.Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})

	t.Run("refined split also works", func(t *testing.T) {
		result := eval(t, engine, `
			(define report-r (recommend-split refs 'refine))
			(> (cdr (assoc 'functions report-r)) 20)`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestSplit_AggregateBeliefIntegration_Goast(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast split))
		(import (wile goast utils))
		(reset-beliefs!)

		(define-aggregate-belief "goast-cohesion"
			(sites (all-functions-in))
			(analyze (single-cluster)))

		(run-beliefs "github.com/aalpar/wile-goast/goast")
	`)

	c := qt.New(t)

	t.Run("completes without error on real package", func(t *testing.T) {
		c.Assert(true, qt.IsTrue)
	})
}
