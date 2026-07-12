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

package goastcg_test

import (
	"context"
	"testing"

	"github.com/aalpar/wile/pkg/values"
	"github.com/aalpar/wile/pkg/wile"

	extgoast "github.com/aalpar/wile-goast/goast"
	extgoastcg "github.com/aalpar/wile-goast/goastcg"
	"github.com/aalpar/wile-goast/testutil"

	qt "github.com/frankban/quicktest"
)

func newEngine(t *testing.T) *wile.Engine {
	t.Helper()
	engine, err := wile.NewEngine(context.Background(),
		wile.WithExtension(extgoastcg.Extension),
	)
	qt.New(t).Assert(err, qt.IsNil)
	return engine
}

func eval(t *testing.T, engine *wile.Engine, code string) wile.Value {
	t.Helper()
	result, err := engine.EvalMultiple(context.Background(), code)
	qt.New(t).Assert(err, qt.IsNil)
	return result
}

func evalExpectError(t *testing.T, engine *wile.Engine, code string) {
	t.Helper()
	expr, err := engine.Parse(context.Background(), code)
	if err != nil {
		return
	}
	_, err = engine.Eval(context.Background(), expr)
	qt.New(t).Assert(err, qt.IsNotNil)
}

func TestExtensionLibraryName(t *testing.T) {
	type libraryNamer interface {
		LibraryName() []string
	}
	namer, ok := extgoastcg.Extension.(libraryNamer)
	qt.New(t).Assert(ok, qt.IsTrue)
	qt.New(t).Assert(namer.LibraryName(), qt.DeepEquals, []string{"wile", "goast", "callgraph"})
}

func TestGoCallgraph_Static(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	// Load a known package with static analysis.
	result := eval(t, engine,
		`(pair? (go-callgraph "github.com/aalpar/wile-goast/goast" 'static))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoCallgraph_CHA(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	result := eval(t, engine,
		`(pair? (go-callgraph "github.com/aalpar/wile-goast/goast" 'cha))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoCallgraph_Precise(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	result := eval(t, engine,
		`(pair? (go-callgraph "github.com/aalpar/wile-goast/goast" 'precise))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

// TestGoCallgraph_StringAlgorithm is the regression guard for the symbol-vs-string
// arg fix: a *string* algorithm is canonical and must work identically to the symbol
// form, aligning go-callgraph with go-callgraph-callers/-callees and go-cfg.
func TestGoCallgraph_StringAlgorithm(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	result := eval(t, engine,
		`(pair? (go-callgraph "github.com/aalpar/wile-goast/goast" "cha"))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func newSessionEngine(t *testing.T) *wile.Engine {
	t.Helper()
	engine, err := wile.NewEngine(context.Background(),
		wile.WithExtension(extgoast.Extension),
		wile.WithExtension(extgoastcg.Extension),
	)
	qt.New(t).Assert(err, qt.IsNil)
	return engine
}

func TestGoCallgraph_WithSession(t *testing.T) {
	engine := newSessionEngine(t)
	testutil.RunScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)
	result := testutil.RunScheme(t, engine,
		`(pair? (go-callgraph s 'static))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoCallgraph_Errors(t *testing.T) {
	engine := newEngine(t)

	tcs := []struct {
		name string
		code string
	}{
		{name: "wrong pattern type", code: `(go-callgraph 42 'static)`},
		{name: "wrong algorithm type", code: `(go-callgraph "pkg" 42)`},
		{name: "invalid algorithm string", code: `(go-callgraph "github.com/aalpar/wile-goast/goast" "unknown")`},
		{name: "invalid algorithm", code: `(go-callgraph "github.com/aalpar/wile-goast/goast" 'unknown)`},
		{name: "nonexistent package", code: `(go-callgraph "github.com/aalpar/wile/does-not-exist-xyz" 'static)`},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			evalExpectError(t, engine, tc.code)
		})
	}
}

// goastTestFunc is the fully-qualified SSA name for PrimGoParseExpr in the goast package.
// ssa.Function.String() returns the full module path, not the short package alias.
const goastTestFunc = "github.com/aalpar/wile-goast/goast.PrimGoParseExpr"

func TestGoCallgraphCallers(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	// Build a static callgraph for a package.
	eval(t, engine,
		`(define cg (go-callgraph "github.com/aalpar/wile-goast/goast" 'static))`)

	// Returns a list (may be empty for a function not called by other goast functions).
	result := eval(t, engine,
		`(list? (go-callgraph-callers cg "`+goastTestFunc+`"))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoCallgraphCallees(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	eval(t, engine,
		`(define cg (go-callgraph "github.com/aalpar/wile-goast/goast" 'static))`)

	// PrimGoCallgraph calls helpers and security — it should have outgoing edges.
	result := eval(t, engine,
		`(pair? (go-callgraph-callees cg "`+goastTestFunc+`"))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoCallgraphCallers_ShortName(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	eval(t, engine,
		`(define cg (go-callgraph "github.com/aalpar/wile-goast/goast" 'static))`)

	// Form 1 short name should resolve via suffix matching.
	result := eval(t, engine,
		`(list? (go-callgraph-callers cg "PrimGoParseExpr"))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoCallgraphCallees_ShortName(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	eval(t, engine,
		`(define cg (go-callgraph "github.com/aalpar/wile-goast/goast" 'static))`)

	result := eval(t, engine,
		`(pair? (go-callgraph-callees cg "PrimGoParseExpr"))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoCallgraphCallers_NotFound(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	eval(t, engine,
		`(define cg (go-callgraph "github.com/aalpar/wile-goast/goast" 'static))`)

	// Nonexistent function returns #f (not empty list).
	result := eval(t, engine,
		`(go-callgraph-callers cg "does.not.Exist")`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)
}

func TestIntegration_CallgraphQuery(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	// Build static callgraph for the goast extension.
	eval(t, engine,
		`(define cg (go-callgraph "github.com/aalpar/wile-goast/goast" 'static))`)

	// Define a helper to extract a named field from an alist node.
	eval(t, engine, `
		(define (nf node key)
			(let ((e (assoc key (cdr node))))
				(if e (cdr e) #f)))`)

	// Verify the graph has cg-node entries with expected structure.
	result := eval(t, engine, `
		(let ((first-node (car cg)))
			(and (eq? (car first-node) 'cg-node)
			     (string? (nf first-node 'name))
			     (integer? (nf first-node 'id))
			     (list? (nf first-node 'edges-in))
			     (list? (nf first-node 'edges-out))))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// Verify edges have expected structure.
	result = eval(t, engine, `
		(let* ((node (car cg))
		       (edges (nf node 'edges-out)))
			(if (null? edges)
				;; Skip if this node has no outgoing edges.
				#t
				(let ((edge (car edges)))
					(and (eq? (car edge) 'cg-edge)
					     (string? (nf edge 'description))))))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoCallgraph_RTA_NoMain(t *testing.T) {
	// RTA on a library package (no main) should error.
	engine := newEngine(t)
	evalExpectError(t, engine,
		`(go-callgraph "github.com/aalpar/wile-goast/goast" 'rta)`)
}

// TestCgEdge_InvokeFields: on an interface-dispatch edge, cg-edge names the
// interface, the method, and the concrete receiver. Without these a consumer
// cannot tell a CHA guess from a proven call — the two are byte-identical today.
func TestCgEdge_InvokeFields(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	// Any edge carrying `iface` must also carry `method` and `recv`.
	result := eval(t, engine, `
		(let ((cg (go-callgraph "github.com/aalpar/wile-goast/goast/testdata/dispatch" 'vta)))
		  (let nloop ((ns cg) (seen #f))
		    (if (null? ns)
		        seen
		        (let eloop ((es (cdr (assq 'edges-out (cdr (car ns))))) (s seen))
		          (if (null? es)
		              (nloop (cdr ns) s)
		              (let ((e (cdr (car es))))
		                (if (assq 'iface e)
		                    (eloop (cdr es)
		                           (and (assq 'method e) (assq 'recv e) #t))
		                    (eloop (cdr es) s))))))))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

// TestCgEdge_StaticCallHasNoIface: the fields appear ONLY on invoke sites. A static
// call has no interface, and inventing one would make the field useless as the
// dispatch-site predicate.
func TestCgEdge_StaticCallHasNoIface(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	result := eval(t, engine, `
		(let ((cg (go-callgraph "github.com/aalpar/wile-goast/goast/testdata/dispatch" 'vta)))
		  (let nloop ((ns cg) (bad #f))
		    (if (null? ns)
		        bad
		        (let eloop ((es (cdr (assq 'edges-out (cdr (car ns))))) (b bad))
		          (if (null? es)
		              (nloop (cdr ns) b)
		              (let* ((e (cdr (car es)))
		                     (d (cdr (assq 'description e))))
		                (eloop (cdr es)
		                       (or b (and (equal? d "static function call")
		                                  (assq 'iface e)
		                                  #t)))))))))`)
	c.Assert(result.Internal(), qt.Not(qt.Equals), values.TrueValue)
}
