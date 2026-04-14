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
	"os"
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
		wile.WithSourceFS(wile.StdLibFS),
		wile.WithSourceFS(os.DirFS("../cmd/wile-goast")),
		wile.WithLibraryPaths("lib"),
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
	// by package path + Form 3 name. Should return the SSA function.
	result := eval(t, engine, `
		(import (wile goast belief))

		(let ((ctx (make-context "github.com/aalpar/wile-goast/goast")))
		  ;; Trigger SSA build
		  (ctx-ssa ctx)
		  ;; Look up PrimGoParseFile by Form 3 name
		  (let ((fn (ctx-find-ssa-func ctx
		              "github.com/aalpar/wile-goast/goast"
		              "github.com/aalpar/wile-goast/goast.PrimGoParseFile")))
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
		// Form 3 names already contain the receiver type, e.g.
		// "(*pkg.MemoryStore).Get", "(pkg.SimpleStore).Get".
		result := eval(t, engine, `
			(import (wile goast belief))

			(let* ((ctx (make-context "github.com/aalpar/wile-goast/goast/testdata/iface"))
			       (selector (interface-methods "Store" "Get"))
			       (sites (selector ctx))
			       (names (map (lambda (s) (nf s 'name)) sites)))
			  names)
		`)
		s := result.SchemeString()
		c.Assert(s, qt.Matches, `.*MemoryStore\)\.Get.*`)
		c.Assert(s, qt.Matches, `.*SimpleStore\)\.Get.*`)
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
		qt.New(t).Assert(devName.SchemeString(), qt.Equals, `"github.com/aalpar/wile-goast/examples/goast-query/testdata/ordering.PipelineReversed"`)
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
		qt.New(t).Assert(devName.SchemeString(), qt.Equals, `"github.com/aalpar/wile-goast/examples/goast-query/testdata/handling.CallerBad"`)
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
		qt.New(t).Assert(devName.SchemeString(), qt.Equals, `"github.com/aalpar/wile-goast/examples/goast-query/testdata/checking.HandleUnsafe"`)
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
			  (lambda (p) (and (equal? (car p) "github.com/aalpar/wile-goast/examples/goast-query/testdata/sameblock.FooFirst") p))
			  classified)))
		`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "a-dominates-b")
	})

	t.Run("BarFirst is b-dominates-a", func(t *testing.T) {
		result := eval(t, engine, `
			(cdr (car (filter-map
			  (lambda (p) (and (equal? (car p) "github.com/aalpar/wile-goast/examples/goast-query/testdata/sameblock.BarFirst") p))
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
		qt.New(t).Assert(devName.SchemeString(), qt.Equals, `"(*github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing.Service).ReadUnsafe"`)
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
		qt.New(t).Assert(devName.SchemeString(), qt.Equals, `"(*github.com/aalpar/wile-goast/examples/goast-query/testdata/comutation.Config).SetServer"`)
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

func TestAllFunctionsIn(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast utils))

		(define ctx (make-context
			"github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing"))
		(define selector (all-functions-in))
		(define sites (selector ctx))
	`)

	c := qt.New(t)

	t.Run("returns non-empty list", func(t *testing.T) {
		result := eval(t, engine, `(> (length sites) 0)`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("each site is a func-decl", func(t *testing.T) {
		result := eval(t, engine, `(tag? (car sites) 'func-decl)`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("each site has pkg-path", func(t *testing.T) {
		result := eval(t, engine, `(nf (car sites) 'pkg-path)`)
		c.Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
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
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking.HandleSafeA"))
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
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking.HandleSafeA"))
		(define safe-result
		  (defuse-reachable? ssa-safe (list "err")
		    (lambda (i) (tag? i 'ssa-if)) 4))

		(define ssa-unsafe (ctx-find-ssa-func ctx
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking.HandleUnsafe"))
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
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking.HandleSafeA"))
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
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking.HandleUnsafe"))

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
		        ((equal? (nf (car fs) 'name) "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking.HandleSafeA") (car fs))
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
		        ((equal? (nf (car fs) 'name) "(*github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing.Service).UpdateSafe") (car fs))
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
		        ((equal? (nf (car fs) 'name) "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking.HandleUnsafe") (car fs))
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
		        ((equal? (nf (car fs) 'name) "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking.HandleSafeA") (car fs))
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
		        ((equal? (nf (car fs) 'name) "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking.HandleSafeA") (car fs))
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

// ═══════════════════════════════════════════════════════════════
// (wile goast domains) — Pre-built abstract domains
// ═══════════════════════════════════════════════════════════════

func TestDomainsImport(t *testing.T) {
	engine := newBeliefEngine(t)
	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `(import (wile goast domains))`)
}

func TestDomainsConcreteEval(t *testing.T) {
	engine := newBeliefEngine(t)
	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `(import (wile goast domains))`)

	t.Run("add", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'add 2 3)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "5")
	})

	t.Run("sub", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'sub 10 4)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "6")
	})

	t.Run("mul", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'mul 3 7)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "21")
	})

	t.Run("div", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'div 10 3)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "3")
	})

	t.Run("div-by-zero", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'div 10 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "unknown")
	})

	t.Run("rem", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'rem 10 3)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("unknown-opcode", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'shl 1 2)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "unknown")
	})
}

func TestDomainsReachingDefinitions(t *testing.T) {
	engine := newBeliefEngine(t)
	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile goast domains))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "../examples/goast-query/testdata/arithmetic"))
		(define (ends-with? s suffix)
			  (let ((slen (string-length s)) (plen (string-length suffix)))
			    (and (>= slen plen)
			         (string=? (substring s (- slen plen) slen) suffix))))
			(define fn-branch (let loop ((fs ssa))
			  (cond ((null? fs) #f)
			        ((ends-with? (nf (car fs) 'name) ".Branch") (car fs))
			        (else (loop (cdr fs))))))

		(define rd-result (make-reaching-definitions fn-branch))
	`)

	t.Run("returns non-empty result", func(t *testing.T) {
		result := eval(t, engine, `(> (length rd-result) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("entry block out has definitions from block 0", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((out0 (analysis-out rd-result 0)))
			  (and (pair? out0) (> (length out0) 0)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("successor inherits predecessor definitions", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((out0 (analysis-out rd-result 0))
			      (in1  (analysis-in rd-result 1)))
			  ;; Every name in out0 should appear in in1
			  (let check ((ns out0))
			    (cond ((null? ns) #t)
			          ((member (car ns) in1) (check (cdr ns)))
			          (else #f))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestDomainsLiveness(t *testing.T) {
	engine := newBeliefEngine(t)
	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile goast domains))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "../examples/goast-query/testdata/arithmetic"))
		(define (ends-with? s suffix)
			  (let ((slen (string-length s)) (plen (string-length suffix)))
			    (and (>= slen plen)
			         (string=? (substring s (- slen plen) slen) suffix))))
			(define fn-branch (let loop ((fs ssa))
			  (cond ((null? fs) #f)
			        ((ends-with? (nf (car fs) 'name) ".Branch") (car fs))
			        (else (loop (cdr fs))))))

		(define live-result (make-liveness fn-branch))
	`)

	t.Run("returns non-empty result", func(t *testing.T) {
		result := eval(t, engine, `(> (length live-result) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("exit blocks have empty out-state", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((blocks (nf fn-branch 'blocks))
			       (exits (filter-map
			                (lambda (b)
			                  (let ((s (nf b 'succs)))
			                    (and (or (not s) (null? s)) b)))
			                blocks)))
			  (let check ((es exits))
			    (cond ((null? es) #t)
			          ((null? (analysis-out live-result (nf (car es) 'index)))
			           (check (cdr es)))
			          (else #f))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("all blocks have states", func(t *testing.T) {
		// Branch has 3 blocks; each should have an entry in the result
		result := eval(t, engine, `
			(= (length live-result)
			   (length (nf fn-branch 'blocks)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestDomainsConstantPropagation(t *testing.T) {
	engine := newBeliefEngine(t)
	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast domains))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "../examples/goast-query/testdata/arithmetic"))
		(define (ends-with? s suffix)
			  (let ((slen (string-length s)) (plen (string-length suffix)))
			    (and (>= slen plen)
			         (string=? (substring s (- slen plen) slen) suffix))))
		(define fn-add (let loop ((fs ssa))
			  (cond ((null? fs) #f)
			        ((ends-with? (nf (car fs) 'name) ".Add") (car fs))
			        (else (loop (cdr fs))))))

		(define cp-result (make-constant-propagation fn-add))
	`)

	t.Run("returns non-empty result", func(t *testing.T) {
		result := eval(t, engine, `(> (length cp-result) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("parameter-derived binop is non-constant", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((out0 (analysis-out cp-result 0))
			       (t0-val (assoc "t0" out0)))
			  (and t0-val (eq? (cdr t0-val) 'flat-top)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestDomainsConstantPropagationBranch(t *testing.T) {
	engine := newBeliefEngine(t)
	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast domains))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "../examples/goast-query/testdata/arithmetic"))
		(define (ends-with? s suffix)
			  (let ((slen (string-length s)) (plen (string-length suffix)))
			    (and (>= slen plen)
			         (string=? (substring s (- slen plen) slen) suffix))))
			(define fn-branch (let loop ((fs ssa))
			  (cond ((null? fs) #f)
			        ((ends-with? (nf (car fs) 'name) ".Branch") (car fs))
			        (else (loop (cdr fs))))))

		(define cp-branch (make-constant-propagation fn-branch))
	`)

	t.Run("analysis completes on branching function", func(t *testing.T) {
		result := eval(t, engine, `(> (length cp-branch) 1)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("all states have alist structure", func(t *testing.T) {
		result := eval(t, engine, `
			(let check ((states (analysis-states cp-branch)))
			  (cond ((null? states) #t)
			        ((not (pair? (cadr (car states)))) #f)
			        (else (check (cdr states)))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestDomainsSignLattice(t *testing.T) {
	engine := newBeliefEngine(t)
	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast domains))
	`)

	t.Run("lattice validates", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sl (sign-lattice)))
			  (validate-lattice sl '(neg zero pos)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("join of incomparable is top", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sl (sign-lattice)))
			  (lattice-join sl 'neg 'pos))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "flat-top")
	})

	t.Run("meet of incomparable is bottom", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sl (sign-lattice)))
			  (lattice-meet sl 'neg 'pos))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "flat-bottom")
	})
}

func TestDomainsSignAnalysis(t *testing.T) {
	engine := newBeliefEngine(t)
	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast domains))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "../examples/goast-query/testdata/arithmetic"))
		(define (ends-with? s suffix)
			  (let ((slen (string-length s)) (plen (string-length suffix)))
			    (and (>= slen plen)
			         (string=? (substring s (- slen plen) slen) suffix))))
		(define fn-add (let loop ((fs ssa))
			  (cond ((null? fs) #f)
			        ((ends-with? (nf (car fs) 'name) ".Add") (car fs))
			        (else (loop (cdr fs))))))

		(define sign-result (make-sign-analysis fn-add))
	`)

	t.Run("analysis completes", func(t *testing.T) {
		result := eval(t, engine, `(> (length sign-result) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("parameter-derived binop is top", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((out0 (analysis-out sign-result 0))
			       (t0-val (assoc "t0" out0)))
			  (and t0-val (eq? (cdr t0-val) 'flat-top)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestDomainsIntervalLattice(t *testing.T) {
	engine := newBeliefEngine(t)
	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast domains))

		(define il (interval-lattice))
	`)

	t.Run("lattice validates on sample intervals", func(t *testing.T) {
		result := eval(t, engine, `
			(validate-lattice il
			  (list '(1 . 5) '(3 . 10) '(-2 . 2) '(0 . 0)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("join widens to encompass both", func(t *testing.T) {
		result := eval(t, engine, `
			(lattice-join il '(1 . 5) '(3 . 10))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(1 . 10)")
	})

	t.Run("meet narrows to intersection", func(t *testing.T) {
		result := eval(t, engine, `
			(lattice-meet il '(1 . 5) '(3 . 10))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(3 . 5)")
	})

	t.Run("empty meet is bottom", func(t *testing.T) {
		result := eval(t, engine, `
			(lattice-meet il '(1 . 3) '(5 . 10))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "interval-bot")
	})

	t.Run("bottom is join identity", func(t *testing.T) {
		result := eval(t, engine, `
			(lattice-join il (lattice-bottom il) '(2 . 5))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(2 . 5)")
	})

	t.Run("leq checks containment", func(t *testing.T) {
		result := eval(t, engine, `
			(and (lattice-leq? il '(2 . 5) '(1 . 10))
			     (not (lattice-leq? il '(1 . 10) '(2 . 5))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestDomainsIntervalAnalysis(t *testing.T) {
	engine := newBeliefEngine(t)
	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast domains))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "../examples/goast-query/testdata/arithmetic"))
		(define (ends-with? s suffix)
			  (let ((slen (string-length s)) (plen (string-length suffix)))
			    (and (>= slen plen)
			         (string=? (substring s (- slen plen) slen) suffix))))
		(define fn-add (let loop ((fs ssa))
			  (cond ((null? fs) #f)
			        ((ends-with? (nf (car fs) 'name) ".Add") (car fs))
			        (else (loop (cdr fs))))))
		(define fn-loop (let loop ((fs ssa))
			  (cond ((null? fs) #f)
			        ((ends-with? (nf (car fs) 'name) ".LoopSum") (car fs))
			        (else (loop (cdr fs))))))
	`)

	t.Run("analysis completes on Add", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((r (make-interval-analysis fn-add)))
			  (> (length r) 0))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("analysis terminates on LoopSum with widening", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((r (make-interval-analysis fn-loop)))
			  (> (length r) 0))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("custom widening threshold", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((r (make-interval-analysis fn-loop 2)))
			  (> (length r) 0))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestAggregateBeliefEvaluation(t *testing.T) {
	engine := newBeliefEngine(t)

	result := eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast utils))
		(reset-beliefs!)

		;; Register an aggregate belief with a custom analyzer
		;; that returns a fixed verdict.
		(define-aggregate-belief "test-cohesion"
			(sites (functions-matching (name-matches "Lock")))
			(analyze (custom (lambda (sites ctx)
				(list (cons 'type 'aggregate)
				      (cons 'verdict 'TEST-OK)
				      (cons 'functions (length sites)))))))

		(let ((out (open-output-string)))
		  (parameterize ((current-output-port out))
			(run-beliefs "github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing"))
		  (get-output-string out))
	`)

	c := qt.New(t)
	output := result.SchemeString()

	t.Run("aggregate belief header printed", func(t *testing.T) {
		c.Assert(output, qt.Matches, `.*Aggregate Belief: test-cohesion.*`)
	})

	t.Run("aggregate verdict printed", func(t *testing.T) {
		c.Assert(output, qt.Matches, `.*TEST-OK.*`)
	})

	t.Run("summary includes aggregate count", func(t *testing.T) {
		c.Assert(output, qt.Matches, `.*Aggregate beliefs:.*1.*`)
	})
}

func TestAggregateBeliefRegistration(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))
		(reset-beliefs!)
	`)

	c := qt.New(t)

	t.Run("register aggregate belief", func(t *testing.T) {
		eval(t, engine, `
			(define-aggregate-belief "test-agg"
				(sites (functions-matching (name-matches "Foo")))
				(analyze (custom (lambda (sites ctx) '((verdict . TEST))))))
		`)
		result := eval(t, engine, `(length (aggregate-beliefs))`)
		c.Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("reset clears aggregate beliefs", func(t *testing.T) {
		eval(t, engine, `(reset-beliefs!)`)
		result := eval(t, engine, `(length (aggregate-beliefs))`)
		c.Assert(result.SchemeString(), qt.Equals, "0")
	})
}
