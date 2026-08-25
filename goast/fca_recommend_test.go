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

func TestSplitCandidates_Lattice(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("ProcessRequest" "A.x" "A.y" "B.z" "B.w")
		    ("ConfigOnly" "A.x")
		    ("MetricsOnly" "B.z"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("ProcessRequest is a split candidate", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((splits (split-candidates lat #f)))
			  (and (pair? splits)
			       (let ((first (car splits)))
			         (string=? (cdr (assoc 'function first))
			                   "ProcessRequest"))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("ConfigOnly is not a split candidate", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((splits (split-candidates lat #f)))
			  (let loop ((ss splits))
			    (cond ((null? ss) #t)
			          ((string=? (cdr (assoc 'function (car ss)))
			                     "ConfigOnly") #f)
			          (else (loop (cdr ss))))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("split has intent-disjointness factor", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((splits (split-candidates lat #f))
			       (first (car splits))
			       (factors (cdr (assoc 'factors first))))
			  (assoc 'intent-disjointness factors))`)
		qt.New(t).Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})
}

func TestPareto_Dominates(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast fca-recommend))`)

	t.Run("strictly better on all factors", func(t *testing.T) {
		result := eval(t, engine, `
			(dominates?
			  '((a . 3) (b . 2) (c . #t))
			  '((a . 1) (b . 1) (c . #f)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("equal on all factors is not domination", func(t *testing.T) {
		result := eval(t, engine, `
			(dominates?
			  '((a . 3) (b . 2))
			  '((a . 3) (b . 2)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#f")
	})

	t.Run("better on one worse on another is not domination", func(t *testing.T) {
		result := eval(t, engine, `
			(dominates?
			  '((a . 3) (b . 1))
			  '((a . 1) (b . 3)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#f")
	})

	t.Run("boolean factor ordering", func(t *testing.T) {
		result := eval(t, engine, `
			(dominates?
			  '((a . 3) (b . #t))
			  '((a . 3) (b . #f)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestIntegration_FuncBoundary(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))
		(import (wile goast dataflow))

		(define s (go-load
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/funcboundary"))
		(define ssa-funcs (go-ssa-build s))
		(define idx (go-ssa-field-index s))

		;; Write-only mode for splits and merges
		(define ctx-w (field-index->context idx 'write-only))
		(define lat-w (concept-lattice ctx-w))

		;; Read-write mode for extracts
		(define ctx-rw (field-index->context idx 'read-write))
		(define lat-rw (concept-lattice ctx-rw))

		(define recs-w (boundary-recommendations lat-w ssa-funcs))
		(define recs-rw (boundary-recommendations lat-rw ssa-funcs))
	`)

	t.Run("ProcessRequest on split frontier with no-cross-flow", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((sf (cdr (assoc 'splits recs-w)))
			       (frontier-ids (cdr (assoc 'frontier sf))))
			  (let loop ((ids frontier-ids))
			    (cond ((null? ids) #f)
			          ((string-suffix? ".ProcessRequest" (car ids)) #t)
			          (else (loop (cdr ids))))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("merge candidates include session writers", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((mf (cdr (assoc 'merges recs-w)))
			       (frontier-ids (cdr (assoc 'frontier mf))))
			  (pair? frontier-ids))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("extract candidates found in read-write mode", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((ef (cdr (assoc 'extracts recs-rw)))
			       (frontier-ids (cdr (assoc 'frontier ef))))
			  (pair? frontier-ids))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestCrossFlowFilter(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))
		(import (wile goast dataflow))

		(define s (go-load
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/funcboundary"))
		(define ssa-funcs (go-ssa-build s))
		(define idx (go-ssa-field-index s))
		(define ctx (field-index->context idx 'write-only))
		(define lat (concept-lattice ctx))
		(define splits (split-candidates lat ssa-funcs))
	`)

	t.Run("ProcessRequest has no cross-flow", func(t *testing.T) {
		result := eval(t, engine, `
			(let loop ((ss splits))
			  (cond ((null? ss) 'not-found)
			        ((string-suffix? ".ProcessRequest"
			           (cdr (assoc 'function (car ss))))
			         (cdr (assoc 'no-cross-flow
			                (cdr (assoc 'factors (car ss))))))
			        (else (loop (cdr ss)))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("ProcessAndRecord has cross-flow", func(t *testing.T) {
		result := eval(t, engine, `
			(let loop ((ss splits))
			  (cond ((null? ss) 'not-found)
			        ((string-suffix? ".ProcessAndRecord"
			           (cdr (assoc 'function (car ss))))
			         (cdr (assoc 'no-cross-flow
			                (cdr (assoc 'factors (car ss))))))
			        (else (loop (cdr ss)))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#f")
	})
}

func TestMergeCandidates(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("ResetSession" "Session.Token" "Session.Expiry")
		    ("ExpireSession" "Session.Token" "Session.Expiry")
		    ("Other" "X.a"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("merge candidate found", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((merges (merge-candidates lat)))
			  (pair? merges))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("merge has intent-overlap factor", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((merges (merge-candidates lat))
			       (first (car merges))
			       (factors (cdr (assoc 'factors first))))
			  (cdr (assoc 'intent-overlap factors)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1.0")
	})

	t.Run("merge functions are ResetSession and ExpireSession", func(t *testing.T) {
		eval(t, engine, `(import (wile goast utils))`)
		result := eval(t, engine, `
			(let* ((merges (merge-candidates lat))
			       (first (car merges))
			       (fns (cdr (assoc 'functions first))))
			  (and (member? "ResetSession" fns)
			       (member? "ExpireSession" fns)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestBoundaryRecommendations(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("Split1" "A.x" "B.y")
		    ("A-only" "A.x")
		    ("B-only" "B.y")
		    ("Merge1" "C.a" "C.b")
		    ("Merge2" "C.a" "C.b"))))
		(define lat (concept-lattice ctx))
		(define recs (boundary-recommendations lat #f))
	`)

	t.Run("returns three frontiers", func(t *testing.T) {
		result := eval(t, engine, `
			(and (assoc 'splits recs)
			     (assoc 'merges recs)
			     (assoc 'extracts recs)
			     #t)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("split frontier non-empty", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sf (cdr (assoc 'splits recs))))
			  (pair? (cdr (assoc 'frontier sf))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("merge frontier non-empty", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((mf (cdr (assoc 'merges recs))))
			  (pair? (cdr (assoc 'frontier mf))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

// The three direction tables are data ABOUT the factor alists the candidate
// functions build, and nothing in the language ties them to those alists.
// factor-direction falls back to 'up on a key it cannot find, so a table that
// names a factor the producer does not emit does not fail — that axis just
// silently keeps the default. Every entry here happens to BE 'up today, which
// makes the failure completely invisible: the tables could be entirely wrong
// and every existing test would still pass.
//
// So pin the join instead of the values: each table's key set must equal the
// key set its candidate function emits, in both directions. A factor added to
// split-candidates without a direction fails this, and so does a direction for
// a factor that no longer exists.
func TestDirectionTablesMatchFrontierFactors(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))
		(import (wile goast utils))

		;; A lattice rich enough to yield all three candidate kinds.
		(define ctx (context-from-alist
		  '(("Split1" "A.x" "B.y")
		    ("A-only" "A.x")
		    ("B-only" "B.y")
		    ("Merge1" "C.a" "C.b")
		    ("Merge2" "C.a" "C.b")
		    ("Broad1" "D.p")
		    ("Broad2" "D.p")
		    ("Narrow1" "D.p" "D.q"))))
		(define lat (concept-lattice ctx))

		(define (keys-of alist) (map car alist))
		;; Containment only, and NOT length-guarded: a length check would make
		;; both directions fail together on any drift, so the assertion that
		;; fired would not tell you which way it drifted. Returns #t, or the
		;; first element of A missing from B -- the offending key itself.
		(define (covered-by? a b)
		  (let loop ((xs a))
		    (cond ((null? xs) #t)
		          ((member? (car xs) b) (loop (cdr xs)))
		          (else (car xs)))))
		;; CANDS is a candidate list; pull the factors off the first entry.
		(define (factor-keys cands)
		  (if (null? cands) '()
		    (keys-of (cdr (assoc 'factors (car cands))))))

		(define split-keys   (factor-keys (split-candidates lat #f)))
		(define merge-keys   (factor-keys (merge-candidates lat)))
		(define extract-keys (factor-keys (extract-candidates lat)))
	`)

	// If a fixture stops producing a candidate kind the agreement check below
	// passes vacuously, so assert the fixture first.
	t.Run("fixture yields all three candidate kinds", func(t *testing.T) {
		out := eval(t, engine, `
			(and (pair? split-keys) (pair? merge-keys) (pair? extract-keys) #t)
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	for _, tc := range []struct{ name, table, keys string }{
		{"split", "split-directions", "split-keys"},
		{"merge", "merge-directions", "merge-keys"},
		{"extract", "extract-directions", "extract-keys"},
	} {
		t.Run(tc.name+" directions match emitted factors", func(t *testing.T) {
			out := eval(t, engine, `(covered-by? (keys-of `+tc.table+`) `+tc.keys+`)`).SchemeString()
			c.Assert(out, qt.Equals, "#t",
				qt.Commentf("%s names a factor the producer does not emit: %s", tc.table, out))
			out = eval(t, engine, `(covered-by? `+tc.keys+` (keys-of `+tc.table+`))`).SchemeString()
			c.Assert(out, qt.Equals, "#t",
				qt.Commentf("factor emitted with no direction in %s: %s", tc.table, out))
		})
	}

	t.Run("every direction is up or down", func(t *testing.T) {
		out := eval(t, engine, `
			(let loop ((ds (append split-directions merge-directions extract-directions)))
			  (cond ((null? ds) #t)
			        ((memq (cdr (car ds)) '(up down)) (loop (cdr ds)))
			        (else (car (car ds)))))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})
}

func TestExtractCandidates(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))
		(import (wile goast utils))

		(define ctx (context-from-alist
		  '(("Validate" "S.Token:r" "S.Expiry:r")
		    ("HandleAuth" "S.Token:r" "S.Expiry:r" "Auth.User:w" "Auth.Level:w")
		    ("HandleResp" "S.Token:r" "S.Expiry:r" "Resp.Body:w" "Resp.Status:w"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("extract candidate found", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((extracts (extract-candidates lat)))
			  (pair? extracts))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("extent-ratio > 1", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((extracts (extract-candidates lat))
			       (first (car extracts))
			       (factors (cdr (assoc 'factors first))))
			  (> (cdr (assoc 'extent-ratio factors)) 1))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("sub-operation includes session reads", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((extracts (extract-candidates lat))
			       (first (car extracts))
			       (sub-op (cdr (assoc 'sub-operation first))))
			  (and (member? "S.Token:r" sub-op)
			       (member? "S.Expiry:r" sub-op)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestConceptSignature(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("F1" "A.x" "B.y")
		    ("F2" "A.x")
		    ("F3" "B.y"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("F1 in multiple concepts", func(t *testing.T) {
		result := eval(t, engine, `(length (concept-signature lat "F1"))`)
		qt.New(t).Assert(result.SchemeString(), qt.Not(qt.Equals), "0")
	})

	t.Run("F2 in at least one concept", func(t *testing.T) {
		result := eval(t, engine, `(>= (length (concept-signature lat "F2")) 1)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestIncomparablePairs(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("F1" "A.x" "B.y")
		    ("F2" "A.x")
		    ("F3" "B.y"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("F1 has incomparable pairs", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sig (concept-signature lat "F1")))
			  (length (incomparable-pairs sig)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("F2 has no incomparable pairs", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sig (concept-signature lat "F2")))
			  (length (incomparable-pairs sig)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "0")
	})
}

func TestPareto_Frontier(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast fca-recommend))`)

	t.Run("single item frontier", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((result (pareto-frontier
			                (list (list 'r1 '((a . 3) (b . 2))))
			                '(a b))))
			  (length (cdr (assoc 'frontier result))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("incomparable items both on frontier", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((result (pareto-frontier
			                (list (list 'r1 '((a . 3) (b . 1)))
			                      (list 'r2 '((a . 1) (b . 3))))
			                '(a b))))
			  (length (cdr (assoc 'frontier result))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "2")
	})

	t.Run("dominated item not on frontier", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((result (pareto-frontier
			                (list (list 'r1 '((a . 3) (b . 3)))
			                      (list 'r2 '((a . 1) (b . 1))))
			                '(a b))))
			  (list (length (cdr (assoc 'frontier result)))
			        (length (cdr (assoc 'dominated result)))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(1 1)")
	})

	t.Run("dominated grouped under dominator", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((result (pareto-frontier
			                (list (list 'r1 '((a . 3) (b . 3)))
			                      (list 'r2 '((a . 1) (b . 1))))
			                '(a b))))
			  (let ((dom-groups (cdr (assoc 'dominated result))))
			    (car (car dom-groups))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "r1")
	})
}
