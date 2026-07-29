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

// These tests drive apply-view with SYNTHETIC rows -- ((score . N) (name . S))
// -- plus the three hooks it takes, so they exercise the combinator itself and
// not either tool's analysis.

func TestApplyView_RankAndTieBreak(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(import (wile goast utils))
		(define rows
		  '(((score . 3) (name . "c"))
		    ((score . 9) (name . "b"))
		    ((score . 9) (name . "a"))
		    ((score . 1) (name . "d"))))
		(define (rk r) (list (cdr (assq 'score r)) (cdr (assq 'name r))))
		(define (rn r detail) (if (eq? detail 'full) r (list (assq 'name r))))
		(define (names shown) (map (lambda (r) (cdr (assq 'name r))) shown))
	`)

	// score descending; equal scores broken by name ascending.
	got := eval(t, engine, `
		(call-with-values
		  (lambda () (apply-view rows '() #f rk rn))
		  (lambda (shown prov) (names shown)))`)
	qt.Assert(t, got.SchemeString(), qt.Equals, `("a" "b" "c" "d")`)
}

func TestApplyView_ProvenanceAndTruncation(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(import (wile goast utils))
		(define rows
		  '(((score . 3) (name . "c"))
		    ((score . 9) (name . "b"))
		    ((score . 9) (name . "a"))
		    ((score . 1) (name . "d"))))
		(define (rk r) (list (cdr (assq 'score r)) (cdr (assq 'name r))))
		(define (rn r detail) (if (eq? detail 'full) r (list (assq 'name r))))
		(define (prov-of opts)
		  (call-with-values
		    (lambda () (apply-view rows opts #f rk rn))
		    (lambda (shown prov) prov)))
		(define (get opts k) (cdr (assq k (prov-of opts))))
	`)

	// Complete result: truncated #f, counts agree, defaults echoed.
	for expr, want := range map[string]string{
		`(get '() 'total-count)`:    "4",
		`(get '() 'returned-count)`: "4",
		`(get '() 'truncated)`:      "#f",
		// This is the one site that pins the default's actual value; every
		// other assertion of the limit reads it off the envelope instead.
		`(get '() 'limit)`:                             "100",
		`(get '() 'offset)`:                            "0",
		`(get '() 'detail)`:                            "summary",
		`(get '((limit . 2)) 'returned-count)`:         "2",
		`(get '((limit . 2)) 'total-count)`:            "4",
		`(get '((limit . 2)) 'truncated)`:              "#t",
		`(get '((limit . 2) (offset . 2)) 'truncated)`: "#f",
		`(get '((limit . 0)) 'returned-count)`:         "0",
		`(get '((limit . 0)) 'truncated)`:              "#t",
		`(get '((offset . 99)) 'returned-count)`:       "0",
		`(get '((offset . 99)) 'truncated)`:            "#f",
	} {
		got := eval(t, engine, expr)
		qt.Assert(t, got.SchemeString(), qt.Equals, want, qt.Commentf("%s", expr))
	}

	// scope/symbol are echoed only when set.
	qt.Assert(t, eval(t, engine, `(assq 'scope (prov-of '()))`).SchemeString(),
		qt.Equals, "#f")
	qt.Assert(t, eval(t, engine, `(cdr (assq 'scope (prov-of '((scope . "x/*")))))`).SchemeString(),
		qt.Equals, `"x/*"`)
	qt.Assert(t, eval(t, engine, `(cdr (assq 'symbol (prov-of '((symbol . "Foo")))))`).SchemeString(),
		qt.Equals, `"Foo"`)
}

// Paging must partition: page 1 and page 2 are disjoint and together equal an
// unpaged call, in the same order. This is what makes offset paging usable.
func TestApplyView_PagesPartition(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(import (wile goast utils))
		(define rows
		  '(((score . 3) (name . "c"))
		    ((score . 9) (name . "b"))
		    ((score . 9) (name . "a"))
		    ((score . 1) (name . "d"))))
		(define (rk r) (list (cdr (assq 'score r)) (cdr (assq 'name r))))
		(define (rn r detail) r)
		(define (page opts)
		  (call-with-values
		    (lambda () (apply-view rows opts #f rk rn))
		    (lambda (shown prov) (map (lambda (r) (cdr (assq 'name r))) shown))))
	`)
	all := eval(t, engine, `(page '())`).SchemeString()
	p1 := eval(t, engine, `(page '((limit . 2) (offset . 0)))`).SchemeString()
	p2 := eval(t, engine, `(page '((limit . 2) (offset . 2)))`).SchemeString()
	qt.Assert(t, all, qt.Equals, `("a" "b" "c" "d")`)
	qt.Assert(t, p1, qt.Equals, `("a" "b")`)
	qt.Assert(t, p2, qt.Equals, `("c" "d")`)
}

// keep? filters BEFORE total-count, so truncated means the same thing whether
// the tool filtered here or upstream. #f admits everything.
func TestApplyView_KeepPredicate(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(import (wile goast utils))
		(define rows
		  '(((score . 3) (name . "c"))
		    ((score . 9) (name . "b"))
		    ((score . 9) (name . "a"))
		    ((score . 1) (name . "d"))))
		(define (rk r) (list (cdr (assq 'score r)) (cdr (assq 'name r))))
		(define (rn r detail) r)
		(define (hi? r) (> (cdr (assq 'score r)) 2))
		(define (tc keep) (call-with-values
		  (lambda () (apply-view rows '() keep rk rn))
		  (lambda (shown prov) (cdr (assq 'total-count prov)))))
	`)
	qt.Assert(t, eval(t, engine, `(tc hi?)`).SchemeString(), qt.Equals, "3")
	qt.Assert(t, eval(t, engine, `(tc #f)`).SchemeString(), qt.Equals, "4")
}

// render receives the detail level and runs only on the returned page.
func TestApplyView_RenderPerDetail(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(import (wile goast utils))
		(define rows '(((score . 9) (name . "a"))))
		(define (rk r) (list (cdr (assq 'score r)) (cdr (assq 'name r))))
		;; The render counter lives in a hashtable, not a top-level variable:
		;; Wile top-level bindings are immutable ("cannot mutate immutable
		;; binding"), so set! on one fails. Mutating the hashtable is fine.
		(define counter (make-hashtable))
		(define (bump!) (hashtable-set! counter 'n (+ 1 (hashtable-ref counter 'n 0))))
		(define (renders) (hashtable-ref counter 'n 0))
		(define (rn r detail)
		  (bump!)
		  (if (eq? detail 'full) r (list (assq 'name r))))
		(define (keys opts)
		  (call-with-values
		    (lambda () (apply-view rows opts #f rk rn))
		    (lambda (shown prov) (map car (car shown)))))
	`)
	qt.Assert(t, eval(t, engine, `(keys '())`).SchemeString(),
		qt.Equals, "(name)")
	qt.Assert(t, eval(t, engine, `(keys '((detail . full)))`).SchemeString(),
		qt.Equals, "(score name)")

	// Withheld rows are never rendered: 5 rows, limit 1 => exactly 1 more
	// render call than before, whatever the two calls above left behind.
	eval(t, engine, `
		(define before (renders))
		(define rows5
		  '(((score . 5) (name . "e")) ((score . 4) (name . "d"))
		    ((score . 3) (name . "c")) ((score . 2) (name . "b"))
		    ((score . 1) (name . "a"))))
		(call-with-values
		  (lambda () (apply-view rows5 '((limit . 1)) #f rk rn))
		  (lambda (shown prov) shown))`)
	qt.Assert(t, eval(t, engine, `(- (renders) before)`).SchemeString(),
		qt.Equals, "1")
}

// view-name-matcher is the shared scope/symbol core both tools' keep? hooks
// are built from. The subtle parts are that scope is applied to the FILE of
// the name's position, and that both globs set is a conjunction on the SAME
// name -- not "scope matched something and symbol matched something".
func TestViewNameMatcher(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(import (wile goast utils))
		(define px (make-hashtable))
		(hashtable-set! px "pkg.Alpha" "/repo/goast/a.go:10:1")
		(hashtable-set! px "pkg.Beta"  "/repo/goast/b.go:20:1")
		;; Gamma is deliberately unlocated: no entry in the index.
		(define (m opts name)
		  (let ((f (view-name-matcher opts px)))
		    (if f (f name) 'no-matcher)))
	`)

	for expr, want := range map[string]string{
		// Neither glob set => no matcher at all, so apply-view admits all.
		`(m '() "pkg.Alpha")`: "no-matcher",
		// symbol alone.
		`(m '((symbol . "Alpha")) "pkg.Alpha")`: "#t",
		`(m '((symbol . "Alpha")) "pkg.Beta")`:  "#f",
		// scope alone, matched against the file, not the position.
		`(m '((scope . "a.go")) "pkg.Alpha")`:       "#t",
		`(m '((scope . "goast/a.go")) "pkg.Alpha")`: "#t",
		`(m '((scope . "a.go")) "pkg.Beta")`:        "#f",
		// Conjunction on the SAME name: Alpha is in a.go, Beta is not.
		`(m '((scope . "a.go") (symbol . "Alpha")) "pkg.Alpha")`: "#t",
		`(m '((scope . "a.go") (symbol . "Beta")) "pkg.Beta")`:   "#f",
		`(m '((scope . "b.go") (symbol . "Beta")) "pkg.Beta")`:   "#t",
		// An unlocated name cannot satisfy a path glob, but a name glob
		// still works on it.
		`(m '((scope . "a.go")) "pkg.Gamma")`:   "#f",
		`(m '((symbol . "Gamma")) "pkg.Gamma")`: "#t",
	} {
		got := eval(t, engine, expr)
		qt.Assert(t, got.SchemeString(), qt.Equals, want, qt.Commentf("%s", expr))
	}
}

// view-sort must survive a list past Wile's 10000-frame call-depth limit;
// the builtin `sort' dies at 20000. find_duplicates' own guard admits 25000
// scored pairs, every one of which may unify, so this is reachable.
func TestViewSort_DeepList(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(define big
		  (let loop ((i 0) (acc '()))
		    (if (>= i 25000) acc (loop (+ i 1) (cons i acc)))))
		(define sorted (view-sort < big))
	`)
	qt.Assert(t, eval(t, engine, `(length sorted)`).SchemeString(), qt.Equals, "25000")
	qt.Assert(t, eval(t, engine, `(car sorted)`).SchemeString(), qt.Equals, "0")
	qt.Assert(t, eval(t, engine, `(list-ref sorted 24999)`).SchemeString(), qt.Equals, "24999")
}

// apply-view's page-and-render step must survive a limit past Wile's
// 10000-frame call-depth limit, the same cliff view-sort exists to avoid:
// `take' and `map' are both non-tail, and LIMIT has no ceiling of its own --
// find_duplicates' own max-candidate-pairs guard admits 25000 rows, so a
// caller asking for everything is a reachable case, not a hypothetical one.
func TestApplyView_DeepLimit(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(define rows
		  (let loop ((i 0) (acc '()))
		    (if (>= i 25000) acc (loop (+ i 1) (cons (list (cons 'n i)) acc)))))
		(define (rk r) (list (cdr (assq 'n r))))
		(define (rn r detail) r)
		(define-values (shown prov)
		  (apply-view rows '((limit . 25000)) #f rk rn))
	`)
	qt.Assert(t, eval(t, engine, `(length shown)`).SchemeString(), qt.Equals, "25000")
	qt.Assert(t, eval(t, engine, `(cdr (assq 'returned-count prov))`).SchemeString(), qt.Equals, "25000")
	qt.Assert(t, eval(t, engine, `(cdr (assq 'truncated prov))`).SchemeString(), qt.Equals, "#f")
}
