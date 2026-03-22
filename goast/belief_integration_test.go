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
		wile.WithLibraryPaths("../cmd/wile-goast/lib"),
		wile.WithExtension(goast.Extension),
		wile.WithExtension(goastssa.Extension),
		wile.WithExtension(goastcg.Extension),
		wile.WithExtension(goastcfg.Extension),
		wile.WithExtension(goastlint.Extension),
	)
	qt.New(t).Assert(err, qt.IsNil)
	return engine
}

// evalMultiple runs Scheme code containing multiple expressions (e.g.
// import + define) and returns the last result.
func evalMultiple(t *testing.T, engine *wile.Engine, code string) wile.Value {
	t.Helper()
	result, err := engine.EvalMultiple(context.Background(), code)
	qt.New(t).Assert(err, qt.IsNil)
	return result
}

func TestBeliefImport(t *testing.T) {
	engine := newBeliefEngine(t)

	// Importing the belief library should succeed without error.
	evalMultiple(t, engine, `(import (wile goast belief))`)
}

func TestBeliefSiteAnnotation(t *testing.T) {
	engine := newBeliefEngine(t)

	// After importing the belief library, load a package and extract
	// func-decls via all-func-decls. Each site should have a pkg-path field.
	result := evalMultiple(t, engine, `
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
	result := evalMultiple(t, engine, `
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
	result := evalMultiple(t, engine, `
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
	evalMultiple(t, engine, `
		(import (wile goast belief))

		(define-belief "prim-functions-have-body"
		  (sites (functions-matching (name-matches "Prim")))
		  (expect (custom (lambda (site ctx)
		    (if (nf site 'body) 'has-body 'no-body))))
		  (threshold 0.90 3))

		(run-beliefs "github.com/aalpar/wile-goast/goast")
	`)
}
