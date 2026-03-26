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
	"context"
	"testing"

	"github.com/aalpar/wile"
	"github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile-goast/goastcfg"
	"github.com/aalpar/wile-goast/goastcg"
	"github.com/aalpar/wile-goast/goastlint"
	"github.com/aalpar/wile-goast/goastssa"

	qt "github.com/frankban/quicktest"
)

// newBeliefEngine creates a Wile engine with all goast extensions and
// library support loaded. The library path points to the embedded lib/
// directory under cmd/wile-goast/ so that (import (wile goast belief)) resolves.
func newBeliefEngine(t *testing.T) *wile.Engine {
	t.Helper()
	engine, err := wile.NewEngine(context.Background(),
		wile.WithSafeExtensions(),
		wile.WithLibraryPaths("../cmd/wile-goast/lib", "../../wile/stdlib/lib"),
		wile.WithExtension(goast.Extension),
		wile.WithExtension(goastssa.Extension),
		wile.WithExtension(goastcg.Extension),
		wile.WithExtension(goastcfg.Extension),
		wile.WithExtension(goastlint.Extension),
	)
	qt.New(t).Assert(err, qt.IsNil)
	return engine
}


func TestBeliefImport(t *testing.T) {
	engine := newBeliefEngine(t)

	// Importing the belief library should succeed without error.
	eval(t, engine, `(import (wile goast belief))`)
}

func TestBeliefSiteAnnotation(t *testing.T) {
	engine := newBeliefEngine(t)

	// After importing the belief library, load a package and extract
	// func-decls via all-func-decls. Each site should have a pkg-path field.
	result := eval(t, engine, `
		(import (wile goast belief))

		(let* ((pkgs (go-typecheck-package "github.com/aalpar/wile-goast/goast"))
		       (funcs (all-func-decls pkgs)))
		  ;; Check that the first func-decl has a pkg-path field
		  (and (pair? funcs)
		       (nf (car funcs) 'pkg-path)))
	`)
	c := qt.New(t)
	c.Assert(result, qt.Not(qt.Equals), nil)
	// Result should be the package path string
	c.Assert(result.SchemeString(), qt.Matches, `.*wile-goast/goast.*`)
}

func TestBeliefSSALookup(t *testing.T) {
	engine := newBeliefEngine(t)

	// Build SSA for the goast package. Look up a known function
	// by package path + short name. Should return the SSA function.
	result := eval(t, engine, `
		(import (wile goast belief))

		(let ((ctx (make-context "github.com/aalpar/wile-goast/goast")))
		  ;; Trigger SSA build
		  (ctx-ssa ctx)
		  ;; Look up PrimGoParseFile by package path + short name
		  (let ((fn (ctx-find-ssa-func ctx
		              "github.com/aalpar/wile-goast/goast"
		              "PrimGoParseFile")))
		    (and fn (nf fn 'name))))
	`)
	c := qt.New(t)
	c.Assert(result, qt.Not(qt.Equals), nil)
	c.Assert(result.SchemeString(), qt.Matches, `.*PrimGoParseFile.*`)
}

func TestBeliefMultiPackage(t *testing.T) {
	engine := newBeliefEngine(t)

	// Use functions-matching with name-matches to find Prim* functions
	// across all goast packages. Count distinct pkg-path values.
	result := eval(t, engine, `
		(import (wile goast belief))

		(let* ((ctx (make-context "github.com/aalpar/wile-goast/..."))
		       (selector (functions-matching (name-matches "Prim")))
		       (funcs (selector ctx))
		       (pkg-paths (filter-map
		                    (lambda (fn) (nf fn 'pkg-path))
		                    funcs))
		       (unique-pkgs (unique pkg-paths)))
		  (length unique-pkgs))
	`)
	c := qt.New(t)
	c.Assert(result, qt.Not(qt.Equals), nil)
	// Prim* functions exist in goast, goastssa, goastcfg, goastcg, goastlint
	c.Assert(result.SchemeString(), qt.Not(qt.Equals), "0")
	c.Assert(result.SchemeString(), qt.Not(qt.Equals), "1")
}

func TestBeliefDefineAndRun(t *testing.T) {
	engine := newBeliefEngine(t)

	// Define a belief that checks whether functions matching "Prim" have a body,
	// then run it against the goast package itself.
	eval(t, engine, `
		(import (wile goast belief))

		(define-belief "prim-functions-have-body"
		  (sites (functions-matching (name-matches "Prim")))
		  (expect (custom (lambda (site ctx)
		    (if (nf site 'body) 'has-body 'no-body))))
		  (threshold 0.90 3))

		(run-beliefs "github.com/aalpar/wile-goast/goast")
	`)
}

func TestUtilsTakeDrop(t *testing.T) {
	engine := newBeliefEngine(t)

	t.Run("take", func(t *testing.T) {
		result := eval(t, engine, `
			(import (wile goast utils))
			(take '(a b c d e) 3)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(a b c)")
	})

	t.Run("take zero", func(t *testing.T) {
		result := eval(t, engine, `
			(import (wile goast utils))
			(take '(a b c) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "()")
	})

	t.Run("drop", func(t *testing.T) {
		result := eval(t, engine, `
			(import (wile goast utils))
			(drop '(a b c d e) 2)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(c d e)")
	})

	t.Run("drop all", func(t *testing.T) {
		result := eval(t, engine, `
			(import (wile goast utils))
			(drop '(a b c) 3)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "()")
	})
}

func TestAstTransform(t *testing.T) {
	engine := newBeliefEngine(t)

	t.Run("replace matching node", func(t *testing.T) {
		result := eval(t, engine, `
			(import (wile goast utils))
			(let* ((node (go-parse-expr "x + 1"))
			       (transformed (ast-transform node
			         (lambda (n)
			           (and (tag? n 'ident)
			                (equal? (nf n 'name) "x")
			                (list 'ident (cons 'name "y")))))))
			  (go-format transformed))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `"y + 1"`)
	})

	t.Run("no match returns unchanged", func(t *testing.T) {
		result := eval(t, engine, `
			(import (wile goast utils))
			(let* ((node (go-parse-expr "x + 1"))
			       (transformed (ast-transform node
			         (lambda (n) #f))))
			  (go-format transformed))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `"x + 1"`)
	})

	t.Run("no recursion into replacement", func(t *testing.T) {
		result := eval(t, engine, `
			(import (wile goast utils))
			(let* ((node (go-parse-expr "x"))
			       (transformed (ast-transform node
			         (lambda (n)
			           (and (tag? n 'ident)
			                (equal? (nf n 'name) "x")
			                (go-parse-expr "x + 1"))))))
			  (go-format transformed))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `"x + 1"`)
	})
}

func TestAstSplice(t *testing.T) {
	engine := newBeliefEngine(t)

	t.Run("splice replaces element with multiple", func(t *testing.T) {
		result := eval(t, engine, `
			(import (wile goast utils))
			(ast-splice '(a b c)
			  (lambda (x) (and (eq? x 'b) '(x y z))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(a x y z c)")
	})

	t.Run("splice no match keeps original", func(t *testing.T) {
		result := eval(t, engine, `
			(import (wile goast utils))
			(ast-splice '(a b c) (lambda (x) #f))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(a b c)")
	})

	t.Run("splice with empty list deletes", func(t *testing.T) {
		result := eval(t, engine, `
			(import (wile goast utils))
			(ast-splice '(a b c)
			  (lambda (x) (and (eq? x 'b) '())))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(a c)")
	})
}

func TestBeliefImplementorsOf(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// implementors-of should return func-decls whose receiver type implements Store.
	// MemoryStore and SimpleStore each have 3 methods = 6 func-decls total.
	result := eval(t, engine, `
		(import (wile goast belief))

		(let* ((ctx (make-context "github.com/aalpar/wile-goast/goast/testdata/iface"))
		       (selector (implementors-of "Store"))
		       (sites (selector ctx)))
		  (length sites))
	`)
	c.Assert(result.SchemeString(), qt.Equals, "6")
}

func TestBeliefInterfaceMethods(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	t.Run("all methods across implementors", func(t *testing.T) {
		// 3 methods x 2 implementors = 6 func-decls
		result := eval(t, engine, `
			(import (wile goast belief))

			(let* ((ctx (make-context "github.com/aalpar/wile-goast/goast/testdata/iface"))
			       (selector (interface-methods "Store"))
			       (sites (selector ctx)))
			  (length sites))
		`)
		c.Assert(result.SchemeString(), qt.Equals, "6")
	})

	t.Run("single method across implementors", func(t *testing.T) {
		// "Get" on MemoryStore + SimpleStore = 2 func-decls
		result := eval(t, engine, `
			(import (wile goast belief))

			(let* ((ctx (make-context "github.com/aalpar/wile-goast/goast/testdata/iface"))
			       (selector (interface-methods "Store" "Get"))
			       (sites (selector ctx)))
			  (length sites))
		`)
		c.Assert(result.SchemeString(), qt.Equals, "2")
	})

	t.Run("impl-type annotation present", func(t *testing.T) {
		// Each returned func-decl should have an impl-type field.
		result := eval(t, engine, `
			(import (wile goast belief))

			(let* ((ctx (make-context "github.com/aalpar/wile-goast/goast/testdata/iface"))
			       (selector (interface-methods "Store" "Get"))
			       (sites (selector ctx)))
			  (and (pair? sites) (nf (car sites) 'impl-type)))
		`)
		c.Assert(result, qt.Not(qt.Equals), nil)
		c.Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})

	t.Run("display name is type-qualified", func(t *testing.T) {
		// site-display-name should produce "TypeName.MethodName" for interface-methods sites.
		result := eval(t, engine, `
			(import (wile goast belief))

			(let* ((ctx (make-context "github.com/aalpar/wile-goast/goast/testdata/iface"))
			       (selector (interface-methods "Store" "Get"))
			       (sites (selector ctx))
			       (names (map (lambda (s)
			                     (let ((impl-type (nf s 'impl-type))
			                           (name (nf s 'name)))
			                       (string-append impl-type "." name)))
			                   sites)))
			  names)
		`)
		s := result.SchemeString()
		c.Assert(s, qt.Matches, `.*MemoryStore\.Get.*`)
		c.Assert(s, qt.Matches, `.*SimpleStore\.Get.*`)
	})
}

func TestBeliefCategory4_Ordering(t *testing.T) {
	engine := newBeliefEngine(t)

	// Use the public API: get sites via functions-matching, classify each
	// with ordered checker, then inspect the results directly.
	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/ordering"))

		;; Get all functions that call both Validate and Process
		(define sites ((functions-matching
		                 (all-of (contains-call "Validate") (contains-call "Process")))
		               ctx))

		;; Classify each site with the ordered checker
		(define checker (ordered "Validate" "Process"))
		(define classified
		  (map (lambda (site) (cons (nf site 'name) (checker site ctx)))
		       sites))
	`)

	t.Run("5 sites found", func(t *testing.T) {
		total := eval(t, engine, `(length sites)`)
		qt.New(t).Assert(total.SchemeString(), qt.Equals, "5")
	})

	t.Run("majority is a-dominates-b", func(t *testing.T) {
		// Count a-dominates-b results — should be 4 of 5
		count := eval(t, engine, `
			(length (filter-map (lambda (p) (and (eq? (cdr p) 'a-dominates-b) p)) classified))
		`)
		qt.New(t).Assert(count.SchemeString(), qt.Equals, "4")
	})

	t.Run("1 deviation", func(t *testing.T) {
		// Non-a-dominates-b entries are deviations
		devs := eval(t, engine, `
			(length (filter-map (lambda (p) (and (not (eq? (cdr p) 'a-dominates-b)) p)) classified))
		`)
		qt.New(t).Assert(devs.SchemeString(), qt.Equals, "1")
	})

	t.Run("deviation is PipelineReversed", func(t *testing.T) {
		devName := eval(t, engine, `
			(let ((devs (filter-map (lambda (p) (and (not (eq? (cdr p) 'a-dominates-b)) p)) classified)))
			  (car (car devs)))
		`)
		qt.New(t).Assert(devName.SchemeString(), qt.Equals, `"PipelineReversed"`)
	})
}

func TestBeliefCategory3_Handling(t *testing.T) {
	engine := newBeliefEngine(t)

	// Use the public API: get sites via callers-of, classify each
	// with contains-call checker, then inspect the results directly.
	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/handling"))

		;; Get all callers of DoWork
		(define sites ((callers-of "DoWork") ctx))

		;; Classify each site with the contains-call checker
		(define checker (contains-call "Errorf"))
		(define classified
		  (map (lambda (site) (cons (nf site 'name) (checker site ctx)))
		       sites))
	`)

	t.Run("5 sites found", func(t *testing.T) {
		total := eval(t, engine, `(length sites)`)
		qt.New(t).Assert(total.SchemeString(), qt.Equals, "5")
	})

	t.Run("4 callers wrap errors", func(t *testing.T) {
		// contains-call returns #t/#f when used directly (not through runner)
		count := eval(t, engine, `
			(length (filter-map (lambda (p) (and (cdr p) p)) classified))
		`)
		qt.New(t).Assert(count.SchemeString(), qt.Equals, "4")
	})

	t.Run("1 deviation", func(t *testing.T) {
		devs := eval(t, engine, `
			(length (filter-map (lambda (p) (and (not (cdr p)) p)) classified))
		`)
		qt.New(t).Assert(devs.SchemeString(), qt.Equals, "1")
	})

	t.Run("deviation is CallerBad", func(t *testing.T) {
		devName := eval(t, engine, `
			(let ((devs (filter-map (lambda (p) (and (not (cdr p)) p)) classified)))
			  (car (car devs)))
		`)
		qt.New(t).Assert(devName.SchemeString(), qt.Equals, `"CallerBad"`)
	})
}

func TestBeliefCategory2_Check(t *testing.T) {
	engine := newBeliefEngine(t)

	// Use the public API: get sites via functions-matching, classify each
	// with checked-before-use, then inspect the results directly.
	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"))

		;; Get all functions that take an error parameter
		(define sites ((functions-matching (has-params "error")) ctx))

		;; Classify each site with the checked-before-use checker
		(define checker (checked-before-use "err"))
		(define classified
		  (map (lambda (site) (cons (nf site 'name) (checker site ctx)))
		       sites))
	`)

	t.Run("5 sites found", func(t *testing.T) {
		total := eval(t, engine, `(length sites)`)
		qt.New(t).Assert(total.SchemeString(), qt.Equals, "5")
	})

	t.Run("4 guarded", func(t *testing.T) {
		count := eval(t, engine, `
			(length (filter-map (lambda (p) (and (eq? (cdr p) 'guarded) p)) classified))
		`)
		qt.New(t).Assert(count.SchemeString(), qt.Equals, "4")
	})

	t.Run("1 unguarded", func(t *testing.T) {
		devs := eval(t, engine, `
			(length (filter-map (lambda (p) (and (eq? (cdr p) 'unguarded) p)) classified))
		`)
		qt.New(t).Assert(devs.SchemeString(), qt.Equals, "1")
	})

	t.Run("deviation is HandleUnsafe", func(t *testing.T) {
		devName := eval(t, engine, `
			(let ((devs (filter-map (lambda (p) (and (eq? (cdr p) 'unguarded) p)) classified)))
			  (car (car devs)))
		`)
		qt.New(t).Assert(devName.SchemeString(), qt.Equals, `"HandleUnsafe"`)
	})
}

func TestBeliefCategory4_SameBlockOrdering(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/sameblock"))

		(define checker (ordered "Foo" "Bar"))
		(define sites ((functions-matching
		                 (all-of (contains-call "Foo") (contains-call "Bar")))
		               ctx))
		(define classified
		  (map (lambda (site) (cons (nf site 'name) (checker site ctx)))
		       sites))
	`)

	t.Run("FooFirst is a-dominates-b", func(t *testing.T) {
		result := eval(t, engine, `
			(cdr (car (filter-map
			  (lambda (p) (and (equal? (car p) "FooFirst") p))
			  classified)))
		`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "a-dominates-b")
	})

	t.Run("BarFirst is b-dominates-a", func(t *testing.T) {
		result := eval(t, engine, `
			(cdr (car (filter-map
			  (lambda (p) (and (equal? (car p) "BarFirst") p))
			  classified)))
		`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "b-dominates-a")
	})
}

func TestBeliefCategory2_FieldGuard(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/fieldguard"))

		(define checker (checked-before-use "r"))
		(define sites ((functions-matching (has-params "Request")) ctx))
		(define classified
		  (map (lambda (site) (cons (nf site 'name) (checker site ctx)))
		       sites))
	`)

	t.Run("5 sites found", func(t *testing.T) {
		total := eval(t, engine, `(length sites)`)
		qt.New(t).Assert(total.SchemeString(), qt.Equals, "5")
	})

	t.Run("4 guarded", func(t *testing.T) {
		count := eval(t, engine, `
			(length (filter-map (lambda (p) (and (eq? (cdr p) 'guarded) p)) classified))
		`)
		qt.New(t).Assert(count.SchemeString(), qt.Equals, "4")
	})

	t.Run("1 unguarded", func(t *testing.T) {
		count := eval(t, engine, `
			(length (filter-map (lambda (p) (and (eq? (cdr p) 'unguarded) p)) classified))
		`)
		qt.New(t).Assert(count.SchemeString(), qt.Equals, "1")
	})
}

func TestBeliefCategory1_Pairing(t *testing.T) {
	engine := newBeliefEngine(t)

	// Use the public API: get sites via functions-matching, classify each
	// with paired-with, then inspect the results directly.
	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing"))

		;; Get all functions that call Lock
		(define sites ((functions-matching (contains-call "Lock")) ctx))

		;; Classify each site with the paired-with checker
		(define checker (paired-with "Lock" "Unlock"))
		(define classified
		  (map (lambda (site) (cons (nf site 'name) (checker site ctx)))
		       sites))
	`)

	t.Run("5 sites found", func(t *testing.T) {
		total := eval(t, engine, `(length sites)`)
		qt.New(t).Assert(total.SchemeString(), qt.Equals, "5")
	})

	t.Run("majority is paired-defer", func(t *testing.T) {
		// Count paired-defer results — should be 4 of 5
		count := eval(t, engine, `
			(length (filter-map (lambda (p) (and (eq? (cdr p) 'paired-defer) p)) classified))
		`)
		qt.New(t).Assert(count.SchemeString(), qt.Equals, "4")
	})

	t.Run("1 deviation", func(t *testing.T) {
		// Non-paired-defer entries are deviations
		devs := eval(t, engine, `
			(length (filter-map (lambda (p) (and (not (eq? (cdr p) 'paired-defer)) p)) classified))
		`)
		qt.New(t).Assert(devs.SchemeString(), qt.Equals, "1")
	})

	t.Run("deviation is ReadUnsafe", func(t *testing.T) {
		devName := eval(t, engine, `
			(let ((devs (filter-map (lambda (p) (and (not (eq? (cdr p) 'paired-defer)) p)) classified)))
			  (car (car devs)))
		`)
		qt.New(t).Assert(devName.SchemeString(), qt.Equals, `"ReadUnsafe"`)
	})
}

func TestBeliefCategory5_CoMutation(t *testing.T) {
	engine := newBeliefEngine(t)

	// Use the public API: get sites via functions-matching + stores-to-fields,
	// classify each with co-mutated, then inspect results.
	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/comutation"))

		;; Get all methods that store to at least Host and Port on Config
		(define sites ((functions-matching
		                 (stores-to-fields "Config" "Host" "Port"))
		               ctx))

		;; Classify each site: do they also write Timeout?
		(define checker (co-mutated "Host" "Port" "Timeout"))
		(define classified
		  (map (lambda (site) (cons (nf site 'name) (checker site ctx)))
		       sites))
	`)

	t.Run("5 sites found", func(t *testing.T) {
		total := eval(t, engine, `(length sites)`)
		qt.New(t).Assert(total.SchemeString(), qt.Equals, "5")
	})

	t.Run("4 co-mutated", func(t *testing.T) {
		count := eval(t, engine, `
			(length (filter-map (lambda (p) (and (eq? (cdr p) 'co-mutated) p)) classified))
		`)
		qt.New(t).Assert(count.SchemeString(), qt.Equals, "4")
	})

	t.Run("1 partial", func(t *testing.T) {
		count := eval(t, engine, `
			(length (filter-map (lambda (p) (and (eq? (cdr p) 'partial) p)) classified))
		`)
		qt.New(t).Assert(count.SchemeString(), qt.Equals, "1")
	})

	t.Run("deviation is SetServer", func(t *testing.T) {
		devName := eval(t, engine, `
			(let ((devs (filter-map (lambda (p) (and (eq? (cdr p) 'partial) p)) classified)))
			  (car (car devs)))
		`)
		qt.New(t).Assert(devName.SchemeString(), qt.Equals, `"SetServer"`)
	})
}

func TestBeliefStoresToFields(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/comutation"))
	`)

	t.Run("all 5 write Host+Port", func(t *testing.T) {
		// All 5 methods write at least Host and Port
		count := eval(t, engine, `
			(length ((functions-matching
			           (stores-to-fields "Config" "Host" "Port"))
			         ctx))
		`)
		qt.New(t).Assert(count.SchemeString(), qt.Equals, "5")
	})

	t.Run("only 4 write all three", func(t *testing.T) {
		// Only 4 methods also write Timeout
		count := eval(t, engine, `
			(length ((functions-matching
			           (stores-to-fields "Config" "Host" "Port" "Timeout"))
			         ctx))
		`)
		qt.New(t).Assert(count.SchemeString(), qt.Equals, "4")
	})
}

func TestBeliefSitesFrom(t *testing.T) {
	engine := newBeliefEngine(t)

	// Define two beliefs: the second bootstraps from the first's deviations.
	// Capture run-beliefs output to verify the second belief sees the right sites.
	result := eval(t, engine, `
		(import (wile goast belief))

		(reset-beliefs!)

		(define-belief "pairing"
		  (sites (functions-matching (contains-call "Lock")))
		  (expect (paired-with "Lock" "Unlock"))
		  (threshold 0.60 3))

		(define-belief "followup"
		  (sites (sites-from "pairing" 'which 'deviation))
		  (expect (custom (lambda (site ctx) 'found)))
		  (threshold 0.50 1))

		(let ((out (open-output-string)))
		  (parameterize ((current-output-port out))
		    (run-beliefs
		      "github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing"))
		  (get-output-string out))
	`)

	c := qt.New(t)
	output := result.SchemeString()

	t.Run("pairing belief reported", func(t *testing.T) {
		c.Assert(output, qt.Matches, `.*pairing.*`)
	})

	t.Run("followup belief sees 1 site", func(t *testing.T) {
		// The followup belief should see exactly 1 site (the deviation from pairing)
		c.Assert(output, qt.Matches, `.*followup.*1/1 sites.*`)
	})
}

func TestBeliefRunBeliefsOutput(t *testing.T) {
	engine := newBeliefEngine(t)

	// Run a single belief and verify the output contains expected structure:
	// header, belief name, pattern, deviation, summary.
	result := eval(t, engine, `
		(import (wile goast belief))

		(reset-beliefs!)

		(define-belief "ordering-test"
		  (sites (functions-matching
		           (all-of (contains-call "Validate") (contains-call "Process"))))
		  (expect (ordered "Validate" "Process"))
		  (threshold 0.60 3))

		(let ((out (open-output-string)))
		  (parameterize ((current-output-port out))
		    (run-beliefs
		      "github.com/aalpar/wile-goast/examples/goast-query/testdata/ordering"))
		  (get-output-string out))
	`)

	c := qt.New(t)
	output := result.SchemeString()

	t.Run("has header", func(t *testing.T) {
		c.Assert(output, qt.Matches, `.*Consistency Analysis.*`)
	})

	t.Run("has belief name", func(t *testing.T) {
		c.Assert(output, qt.Matches, `.*ordering-test.*`)
	})

	t.Run("has pattern with counts", func(t *testing.T) {
		c.Assert(output, qt.Matches, `.*a-dominates-b.*4/5 sites.*`)
	})

	t.Run("has deviation", func(t *testing.T) {
		c.Assert(output, qt.Matches, `.*DEVIATION.*PipelineReversed.*`)
	})

	t.Run("has summary", func(t *testing.T) {
		c.Assert(output, qt.Matches, `.*Beliefs evaluated:.*1.*`)
		c.Assert(output, qt.Matches, `.*Strong beliefs:.*1.*`)
		c.Assert(output, qt.Matches, `.*Deviations found:.*1.*`)
	})
}

// ── Dataflow library tests ──────────────────────────────

func TestDataflowBooleanLattice(t *testing.T) {
	engine := newBeliefEngine(t)

	t.Run("join is or", func(t *testing.T) {
		result := eval(t, engine, `
			(import (wile algebra))
			(import (wile goast dataflow))
			(let ((L (boolean-lattice)))
			  (list (lattice-join L #f #f)
			        (lattice-join L #f #t)
			        (lattice-join L #t #f)
			        (lattice-join L #t #t)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(#f #t #t #t)")
	})

	t.Run("bottom is false", func(t *testing.T) {
		result := eval(t, engine, `
			(import (wile algebra))
			(import (wile goast dataflow))
			(lattice-bottom (boolean-lattice))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#f")
	})
}

func TestDataflowSSANames(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast dataflow))
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"))
		(define ssa-fn (ctx-find-ssa-func ctx
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"
		  "HandleSafeA"))
		(define instrs (ssa-all-instrs ssa-fn))
		(define names (ssa-instruction-names ssa-fn))
	`)

	t.Run("instrs is non-empty", func(t *testing.T) {
		result := eval(t, engine, `(> (length instrs) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("names is non-empty", func(t *testing.T) {
		result := eval(t, engine, `(> (length names) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("names are strings", func(t *testing.T) {
		result := eval(t, engine, `(string? (car names))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestDataflowDefuseReachable(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast dataflow))
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"))

		(define ssa-safe (ctx-find-ssa-func ctx
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"
		  "HandleSafeA"))
		(define safe-result
		  (defuse-reachable? ssa-safe (list "err")
		    (lambda (i) (tag? i 'ssa-if)) 4))

		(define ssa-unsafe (ctx-find-ssa-func ctx
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"
		  "HandleUnsafe"))
		(define unsafe-result
		  (defuse-reachable? ssa-unsafe (list "err")
		    (lambda (i) (tag? i 'ssa-if)) 4))
	`)

	t.Run("safe function is guarded", func(t *testing.T) {
		result := eval(t, engine, `safe-result`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("unsafe function is unguarded", func(t *testing.T) {
		result := eval(t, engine, `unsafe-result`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#f")
	})
}

func TestDataflowBlockInstrs(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast dataflow))
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"))
		(define ssa-fn (ctx-find-ssa-func ctx
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"
		  "HandleSafeA"))
		(define blocks (nf ssa-fn 'blocks))
		(define b0 (car blocks))
	`)

	t.Run("block 0 has 2 instructions", func(t *testing.T) {
		result := eval(t, engine, `(number->string (length (block-instrs b0)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `"2"`)
	})

	t.Run("missing block returns empty list", func(t *testing.T) {
		result := eval(t, engine, `(block-instrs '(ssa-block (index . 99)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "()")
	})
}

func TestDataflowRunAnalysisForwardSingleBlock(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast dataflow))
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"))
		(define fn (ctx-find-ssa-func ctx
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"
		  "HandleUnsafe"))

		;; Reaching names: powerset lattice over instruction names in function
		(define universe (ssa-instruction-names fn))
		(define lat (powerset-lattice universe))

		;; Transfer: union in-state with names defined in block
		(define (transfer blk in-val)
		  (let ((names (filter-map
		                 (lambda (i) (nf i 'name))
		                 (block-instrs blk))))
		    (lattice-join lat in-val names)))

		(define result (run-analysis 'forward lat transfer fn))
	`)

	t.Run("entry in-state is bottom", func(t *testing.T) {
		result := eval(t, engine, `(analysis-in result 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "()")
	})

	t.Run("entry out-state has defined names", func(t *testing.T) {
		result := eval(t, engine, `(> (length (analysis-out result 0)) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("analysis-states returns alist of length 1", func(t *testing.T) {
		result := eval(t, engine, `(number->string (length (analysis-states result)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, `"1"`)
	})
}

func TestDataflowRunAnalysisForwardBranching(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast dataflow))
		(import (wile goast utils))

		(define ssa (go-ssa-build
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"))
		(define fn (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "HandleSafeA") (car fs))
		        (else (loop (cdr fs))))))

		(define universe (ssa-instruction-names fn))
		(define lat (powerset-lattice universe))
		(define (transfer block state)
		  (let ((names (filter-map (lambda (i) (nf i 'name)) (block-instrs block))))
		    (lattice-join lat state names)))

		(define result (run-analysis 'forward lat transfer fn))
	`)

	t.Run("block 0 out has t0", func(t *testing.T) {
		result := eval(t, engine, `(and (member "t0" (analysis-out result 0)) #t)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("block 1 in has t0 from predecessor", func(t *testing.T) {
		result := eval(t, engine, `(and (member "t0" (analysis-in result 1)) #t)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("block 1 out has names from both blocks", func(t *testing.T) {
		result := eval(t, engine, `
			(and (member "t0" (analysis-out result 1))
			     (member "t1" (analysis-out result 1))
			     #t)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("block 2 in has t0 only", func(t *testing.T) {
		result := eval(t, engine, `
			(and (member "t0" (analysis-in result 2))
			     (= (length (analysis-in result 2)) 1))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestDataflowRunAnalysisForwardJoin(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast dataflow))
		(import (wile goast utils))

		(define ssa (go-ssa-build
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing"))
		(define fn (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "UpdateSafe") (car fs))
		        (else (loop (cdr fs))))))

		(define universe (ssa-instruction-names fn))
		(define lat (powerset-lattice universe))
		(define (transfer block state)
		  (let ((names (filter-map (lambda (i) (nf i 'name)) (block-instrs block))))
		    (lattice-join lat state names)))

		(define result (run-analysis 'forward lat transfer fn))
	`)

	t.Run("join block in includes names from both predecessors", func(t *testing.T) {
		// Block 3 has preds {0, 2}. Its in-state = union of out(0) and out(2).
		// Names from block 2 should appear in block 3's in even though direct
		// path 0->3 skips block 2.
		result := eval(t, engine, `
			(let ((b2-names (filter-map (lambda (i) (nf i 'name))
			                  (block-instrs (caddr (nf fn 'blocks)))))
			      (b3-in (analysis-in result 3)))
			  (let check ((ns b2-names))
			    (cond ((null? ns) #t)
			          ((member (car ns) b3-in) (check (cdr ns)))
			          (else #f))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestDataflowRunAnalysisInitialState(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast dataflow))
		(import (wile goast utils))

		(define ssa (go-ssa-build
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"))
		(define fn (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "HandleUnsafe") (car fs))
		        (else (loop (cdr fs))))))

		(define seeded-universe (cons "SEED" (ssa-instruction-names fn)))
		(define seeded-lat (powerset-lattice seeded-universe))
		(define (seeded-transfer block state)
		  (let ((names (filter-map (lambda (i) (nf i 'name)) (block-instrs block))))
		    (lattice-join seeded-lat state names)))

		(define result-seeded (run-analysis 'forward seeded-lat seeded-transfer fn
		                        (list "SEED")))
	`)

	t.Run("custom initial state propagates to in", func(t *testing.T) {
		result := eval(t, engine, `(and (member "SEED" (analysis-in result-seeded 0)) #t)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("custom initial state reaches output", func(t *testing.T) {
		result := eval(t, engine, `(and (member "SEED" (analysis-out result-seeded 0)) #t)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestDataflowRunAnalysisBackward(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast dataflow))
		(import (wile goast utils))

		(define ssa (go-ssa-build
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"))
		(define fn (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "HandleSafeA") (car fs))
		        (else (loop (cdr fs))))))

		(define universe (ssa-instruction-names fn))
		(define lat (powerset-lattice universe))
		(define (transfer block state)
		  (let ((ops (flat-map
		               (lambda (i) (or (nf i 'operands) '()))
		               (block-instrs block))))
		    (let ((relevant (filter-map (lambda (o) (and (member o universe) o)) ops)))
		      (lattice-join lat state relevant))))

		(define result (run-analysis 'backward lat transfer fn))
	`)

	t.Run("exit blocks in-state is bottom", func(t *testing.T) {
		result := eval(t, engine, `(analysis-in result 1)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "()")
	})

	t.Run("entry block accumulates from successors", func(t *testing.T) {
		result := eval(t, engine, `(> (length (analysis-out result 0)) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("backward propagates usage toward entry", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((b0-out (analysis-out result 0))
			      (b1-in  (analysis-in result 1))
			      (b2-in  (analysis-in result 2)))
			  (>= (length b0-out)
			      (max (length b1-in) (length b2-in))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestDataflowMonotonicityViolation(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast dataflow))
		(import (wile goast utils))

		(define ssa (go-ssa-build
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"))
		(define fn (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "HandleSafeA") (car fs))
		        (else (loop (cdr fs))))))

		(define universe (ssa-instruction-names fn))
		(define lat (powerset-lattice universe))

		(define call-count 0)
		(define (bad-transfer block state)
		  (set! call-count (+ call-count 1))
		  (if (> call-count 1) '() state))
	`)

	t.Run("no flag means no check", func(t *testing.T) {
		eval(t, engine, `
			(set! call-count 0)
			(run-analysis 'forward lat bad-transfer fn)`)
		// No error = pass
	})

	t.Run("check-monotone catches violation", func(t *testing.T) {
		evalExpectError(t, engine, `
			(set! call-count 0)
			(run-analysis 'forward lat bad-transfer fn
			  (lattice-bottom lat) 'check-monotone)`)
	})
}
