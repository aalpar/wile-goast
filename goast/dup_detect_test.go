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
		(define canon-index (build-func-canon-index (go-ssa-build s)))
		(define m (score-candidate-pair "SumSlice" "TotalSlice" ast-index canon-index 0.6))
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
		out := eval(t, engine, `(score-candidate-pair "Nope" "Nada" ast-index canon-index 0.6)`).SchemeString()
		c.Assert(out, qt.Equals, "#f")
	})
}

func TestFindScoredCandidates(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(import (wile goast provenance))
		(define cands (find-scored-candidates "`+pkg+`"))
		(define clone
		  (let loop ((cs cands))
		    (if (null? cs) #f
		      (let ((p (cdr (assoc 'pair (car cs)))))
		        (if (and (member "SumSlice" p) (member "TotalSlice" p))
		          (car cs) (loop (cdr cs)))))))
	`)

	t.Run("clone pair is discovered and scored", func(t *testing.T) {
		out := eval(t, engine, `
			(and clone
			     (memq (cdr (assoc 'equiv-tier (cdr (assoc 'measures clone))))
			           '(proven structural))
			     #t)
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("two located findings, score = similarity", func(t *testing.T) {
		shape := eval(t, engine, `
			(let ((fs (cdr (assoc 'findings clone))))
			  (and (= (length fs) 2)
			       (string? (finding-where (car fs)))
			       (number? (finding-score (car fs)))))
		`).SchemeString()
		c.Assert(shape, qt.Equals, "#t")
		render := eval(t, engine, `(render-finding (car (cdr (assoc 'findings clone))))`).SchemeString()
		c.Assert(strings.Contains(render, "clones.go"), qt.IsTrue, qt.Commentf("%s", render))
	})

	t.Run("why renders the peer and is structured for filtering", func(t *testing.T) {
		render := eval(t, engine, `
			(render-why (finding-why (car (cdr (assoc 'findings clone)))))
		`).SchemeString()
		c.Assert(strings.Contains(render, "unify-candidate"), qt.IsTrue, qt.Commentf("%s", render))
		peer := eval(t, engine, `
			(let* ((f (car (cdr (assoc 'findings clone))))
			       (why (finding-why f)))
			  (cdr (assoc 'peer (cdr why))))
		`).SchemeString()
		c.Assert(strings.Contains(peer, "Slice"), qt.IsTrue, qt.Commentf("%s", peer))
	})
}

func TestCandidateToVerdict(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast dup-detect))`)

	t.Run("tier maps to verdict", func(t *testing.T) {
		out := eval(t, engine, `
			(list
			  (candidate->verdict (list (cons 'measures (list (cons 'equiv-tier 'proven)))))
			  (candidate->verdict (list (cons 'measures (list (cons 'equiv-tier 'structural)))))
			  (candidate->verdict (list (cons 'measures (list (cons 'equiv-tier 'divergent))))))
		`).SchemeString()
		c.Assert(out, qt.Equals, "(duplicate likely-duplicate distinct)")
	})
}

func TestParetoOverCandidates(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"
	// The documented combinator: rank scored candidates on a user-chosen numeric
	// measure vector via the existing pareto-frontier (no new ranking code). Each
	// item is (id factors-alist); factors must be numeric (drop the equiv-tier
	// symbol), else dominance ordering breaks.
	out := eval(t, engine, `
		(import (wile goast dup-detect))
		(import (wile goast fca-recommend))
		(define cands (find-scored-candidates "`+pkg+`"))
		(define items
		  (map (lambda (c)
		         (let ((m (cdr (assoc 'measures c))))
		           (list (cdr (assoc 'pair c))
		                 (list (cons 'benefit (cdr (assoc 'benefit m)))
		                       (cons 'similarity (cdr (assoc 'similarity m)))))))
		       cands))
		(define fr (pareto-frontier items '(benefit similarity)))
		(and (>= (length cands) 1)
		     (>= (length (cdr (assoc 'frontier fr))) 1)
		     #t)
	`).SchemeString()
	c.Assert(out, qt.Equals, "#t")
}

func TestCostEdgesAndCycle(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/transcall"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(define cg (go-callgraph "`+pkg+`" 'static))
	`)

	t.Run("Initialize reaches SetupConfig -> merging creates a cycle", func(t *testing.T) {
		out := eval(t, engine, `(cand-creates-cycle? "Initialize" "SetupConfig" cg)`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("two leaves on no shared path -> no cycle", func(t *testing.T) {
		out := eval(t, engine, `(cand-creates-cycle? "SetupConfig" "SetupLogger" cg)`).SchemeString()
		c.Assert(out, qt.Equals, "#f")
	})

	t.Run("new-edges = union of caller sets", func(t *testing.T) {
		out := eval(t, engine, `(cand-new-edges "SetupConfig" "SetupLogger" cg)`).SchemeString()
		c.Assert(out, qt.Equals, "1")
	})
}

func TestCostLocality(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/transcall"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(define cg (go-callgraph "`+pkg+`" 'static))
		(define fr-index (build-func-ref-index (go-func-refs "`+pkg+`")))
		(define loc (cand-locality "SetupConfig" "SetupLogger" fr-index cg))
	`)

	t.Run("same package -> same-pkg scope", func(t *testing.T) {
		out := eval(t, engine, `(cdr (assoc 'scope loc))`).SchemeString()
		c.Assert(out, qt.Equals, "same-pkg")
	})

	t.Run("dep-overlap is a number in [0,1]", func(t *testing.T) {
		out := eval(t, engine, `
			(let ((d (cdr (assoc 'dep-overlap loc))))
			  (and (number? d) (>= d 0) (<= d 1)))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})
}

func TestFindCandidatesWithCost(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(import (wile goast fca-recommend))
		(define cands (find-candidates-with-cost "`+pkg+`"))
		(define clone
		  (let loop ((cs cands))
		    (if (null? cs) #f
		      (let ((p (cdr (assoc 'pair (car cs)))))
		        (if (and (member "SumSlice" p) (member "TotalSlice" p))
		          (car cs) (loop (cdr cs)))))))
	`)

	t.Run("clone candidate carries the full benefit+cost ledger", func(t *testing.T) {
		out := eval(t, engine, `
			(let ((m (cdr (assoc 'measures clone))))
			  (and (assoc 'benefit m) (assoc 'equiv-tier m)
			       (assoc 'new-edges m) (assoc 'creates-cycle? m)
			       (assoc 'locality m)
			       (number? (cdr (assoc 'new-edges m)))
			       #t))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("pareto over benefit + neg-edges", func(t *testing.T) {
		out := eval(t, engine, `
			(let ((items (map (lambda (c)
			                    (let ((m (cdr (assoc 'measures c))))
			                      (list (cdr (assoc 'pair c))
			                            (list (cons 'benefit (cdr (assoc 'benefit m)))
			                                  (cons 'neg-edges (- (cdr (assoc 'new-edges m))))))))
			                  cands)))
			  (>= (length (cdr (assoc 'frontier (pareto-frontier items '(benefit neg-edges))))) 1))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})
}

// TestBuildFuncCanonIndex locks the two reasons canonicalization is hoisted out
// of score-candidate-pair into a single guarded pass (findings #1 + #2):
//   - it is keyed by short-name like build-func-ssa-index (its raw counterpart);
//   - a function whose SSA fails go-ssa-canonicalize is omitted, not raised, so
//     a single bad function cannot abort an O(n^2) scoring sweep.
func TestBuildFuncCanonIndex(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"
	eval(t, engine, `
		(import (wile goast dup-detect))
		(define s (go-load "`+pkg+`"))
		(define raw   (build-func-ssa-index   (go-ssa-build s)))
		(define canon (build-func-canon-index (go-ssa-build s)))
	`)

	t.Run("raw and canon indexes are both keyed by short-name", func(t *testing.T) {
		out := eval(t, engine, `
			(and (hashtable? raw) (hashtable? canon)
			     (pair? (hashtable-ref raw "SumSlice" #f))
			     (pair? (hashtable-ref canon "SumSlice" #f)))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("canonicalization failure is guarded, not raised", func(t *testing.T) {
		// A malformed ssa-func node that go-ssa-canonicalize rejects (empty blocks).
		// build-func-canon-index must skip it and still return an index, rather
		// than letting the error propagate and abort the whole pass.
		out := eval(t, engine, `
			(let ((idx (build-func-canon-index
			             (list '(ssa-func (name . "Bad") (blocks . ()))))))
			  (and (hashtable? idx)
			       (not (hashtable-ref idx "Bad" #f))))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})
}

// The split of find-scored-candidates into discovery + score-candidate-pairs
// is behaviour-preserving: driving the lower entry point with the full pair
// list must reproduce the top-level result exactly. This is the regression
// guard for the restructure that the scope/symbol push-down rides on.
func TestScoreCandidatePairs_MatchesTopLevel(t *testing.T) {
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"

	eval(t, engine, `
		(import (wile goast dup-detect))
		(import (wile goast fca))
		(import (wile goast utils))
		(define sess (go-load "`+pkg+`"))
		(define refs (go-func-refs sess))
		(define cands (duplicate-candidate-concepts
		                (concept-lattice (function-ref-context refs))))
		(define prs (candidate-pairs cands))
		(define via-top (find-scored-candidates sess 0.6))
		(define via-pairs (score-candidate-pairs sess refs prs 0.6))
		(define (ids cs)
		  (map (lambda (c) (cdr (assq 'pair c))) cs))
		(define (tiers cs)
		  (map (lambda (c) (cdr (assq 'equiv-tier (cdr (assq 'measures c))))) cs))
	`)

	c := qt.New(t)
	c.Assert(eval(t, engine, `(> (length via-top) 0)`).SchemeString(), qt.Equals, "#t")
	c.Assert(eval(t, engine, `(= (length via-top) (length via-pairs))`).SchemeString(),
		qt.Equals, "#t")
	c.Assert(eval(t, engine, `(equal? (ids via-top) (ids via-pairs))`).SchemeString(),
		qt.Equals, "#t")
	c.Assert(eval(t, engine, `(equal? (tiers via-top) (tiers via-pairs))`).SchemeString(),
		qt.Equals, "#t")
}

// Scoring a PRUNED pair list yields exactly the corresponding subset: the
// same pairs score identically whether or not their neighbours were scored.
// That is the soundness claim the push-down rests on.
func TestScoreCandidatePairs_PrunedIsSubset(t *testing.T) {
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"

	eval(t, engine, `
		(import (wile goast dup-detect))
		(import (wile goast fca))
		(import (wile goast utils))
		(define sess2 (go-load "`+pkg+`"))
		(define refs2 (go-func-refs sess2))
		(define cands2 (duplicate-candidate-concepts
		                 (concept-lattice (function-ref-context refs2))))
		(define prs2 (candidate-pairs cands2))
		(define (touches-sum? pr)
		  (or (glob-match? "*SumSlice*" (car pr) #\.)
		      (glob-match? "*SumSlice*" (cdr pr) #\.)))
		(define pruned (filter touches-sum? prs2))
		(define full-run (score-candidate-pairs sess2 refs2 prs2 0.6))
		(define pruned-run (score-candidate-pairs sess2 refs2 pruned 0.6))
		(define (find-pair cs a b)
		  (let loop ((xs cs))
		    (cond ((null? xs) #f)
		          ((equal? (cdr (assq 'pair (car xs))) (list a b)) (car xs))
		          (else (loop (cdr xs))))))
	`)

	c := qt.New(t)
	c.Assert(eval(t, engine, `(< (length pruned) (length prs2))`).SchemeString(),
		qt.Equals, "#t")
	c.Assert(eval(t, engine, `(<= (length pruned-run) (length full-run))`).SchemeString(),
		qt.Equals, "#t")
	// The clone pair scores identically in both runs.
	c.Assert(eval(t, engine, `
		(let ((a (find-pair full-run "SumSlice" "TotalSlice"))
		      (b (find-pair pruned-run "SumSlice" "TotalSlice")))
		  (and a b (equal? (cdr (assq 'measures a)) (cdr (assq 'measures b)))))
	`).SchemeString(), qt.Equals, "#t")
}
