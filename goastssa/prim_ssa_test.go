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

package goastssa_test

import (
	"context"
	"testing"

	"github.com/aalpar/wile"
	extgoast "github.com/aalpar/wile-goast/goast"
	extgoastssa "github.com/aalpar/wile-goast/goastssa"
	"github.com/aalpar/wile-goast/testutil"
	"github.com/aalpar/wile/values"

	qt "github.com/frankban/quicktest"
)

func newEngine(t *testing.T) *wile.Engine {
	t.Helper()
	engine, err := wile.NewEngine(context.Background(),
		wile.WithExtension(extgoastssa.Extension),
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
	namer, ok := extgoastssa.Extension.(libraryNamer)
	qt.New(t).Assert(ok, qt.IsTrue)
	qt.New(t).Assert(namer.LibraryName(), qt.DeepEquals, []string{"wile", "goast", "ssa"})
}

func TestGoSSABuild_WithPositionsOption(t *testing.T) {
	engine := newEngine(t)
	// The positions option is accepted without error.
	result := eval(t, engine,
		`(pair? (go-ssa-build "github.com/aalpar/wile-goast/goast" 'positions))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoSSABuild_ReturnsListOfFunctions(t *testing.T) {
	engine := newEngine(t)

	// Load a known package, verify we get a list back.
	result := eval(t, engine,
		`(pair? (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoSSABuild_FunctionStructure(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	// Cache the SSA build result.
	eval(t, engine, `(define funcs (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)

	t.Run("function has name", func(t *testing.T) {
		// Find a known function: PrimGoParseExpr
		result := eval(t, engine, `
			(let loop ((fs funcs))
				(cond
					((null? fs) #f)
					((equal? (cdr (assoc 'name (cdr (car fs)))) "PrimGoParseExpr")
					 (car fs))
					(else (loop (cdr fs)))))`)
		// Should find the function (not #f).
		c.Assert(result.Internal(), qt.Not(qt.Equals), values.FalseValue)
	})

	t.Run("function has params", func(t *testing.T) {
		result2 := eval(t, engine, `
			(let ((fn (car funcs)))
				(assoc 'params (cdr fn)))`)
		c.Assert(result2.Internal(), qt.Not(qt.Equals), values.FalseValue)
	})

	t.Run("function has blocks", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((fn (car funcs)))
				(assoc 'blocks (cdr fn)))`)
		c.Assert(result.Internal(), qt.Not(qt.Equals), values.FalseValue)
	})

	t.Run("block has index", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((fn (car funcs))
				   (blocks (cdr (assoc 'blocks (cdr fn))))
				   (block (car blocks)))
				(cdr (assoc 'index (cdr block))))`)
		c.Assert(result.Internal(), qt.Equals, values.NewInteger(0))
	})
}

func TestIntegration_FieldStoreQuery(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	// Define helpers and build SSA for a package with known struct field accesses.
	eval(t, engine, `(define funcs (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)
	eval(t, engine, `
		(define (tag? node t)
			(and (pair? node) (eq? (car node) t)))`)
	eval(t, engine, `
		(define (walk-instrs fn pred)
			(let loop ((blocks (cdr (assoc 'blocks (cdr fn)))) (acc '()))
				(if (null? blocks) (reverse acc)
					(let ((instrs (cdr (assoc 'instrs (cdr (car blocks))))))
						(loop (cdr blocks)
							(let iloop ((is instrs) (a acc))
								(if (null? is) a
									(iloop (cdr is)
										(if (pred (car is))
											(cons (car is) a)
											a)))))))))`)

	// Query: do any functions contain ssa-field-addr instructions?
	// The goast package has struct field accesses (mapperOpts, etc.)
	// so we expect at least one.
	result := eval(t, engine, `
		(let loop ((fs funcs))
			(if (null? fs) #f
				(let ((addrs (walk-instrs (car fs) (lambda (i) (tag? i 'ssa-field-addr)))))
					(if (pair? addrs) #t (loop (cdr fs))))))`)

	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoSSABuild_BlockDominance(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	// Build SSA and find a function with multiple blocks (any function with control flow).
	eval(t, engine, `(define funcs (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)
	eval(t, engine, `
		(define multi-block-fn
			(let loop ((fs funcs))
				(if (null? fs) #f
					(let* ((fn (car fs))
						   (blocks (cdr (assoc 'blocks (cdr fn)))))
						(if (> (length blocks) 2) fn (loop (cdr fs)))))))`)

	t.Run("entry block has no idom", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((blocks (cdr (assoc 'blocks (cdr multi-block-fn))))
				   (entry (car blocks)))
				(assoc 'idom (cdr entry)))`)
		c.Assert(result.Internal(), qt.Equals, values.FalseValue)
	})

	t.Run("non-entry block has idom", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((blocks (cdr (assoc 'blocks (cdr multi-block-fn))))
				   (second (cadr blocks)))
				(pair? (assoc 'idom (cdr second))))`)
		c.Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("idom chain reaches entry", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((blocks (cdr (assoc 'blocks (cdr multi-block-fn))))
				   (last-block (car (reverse blocks)))
				   (start-idx (cdr (assoc 'index (cdr last-block)))))
				(let loop ((idx start-idx))
					(cond
						((= idx 0) #t)
						(else
							(let* ((blk (list-ref blocks idx))
								   (idom-pair (assoc 'idom (cdr blk))))
								(if idom-pair
									(loop (cdr idom-pair))
									#f))))))`)
		c.Assert(result.Internal(), qt.Equals, values.TrueValue)
	})
}

func TestGoSSAFieldIndex_ReturnsFieldSummaries(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	// go-ssa-field-index should return a list of ssa-field-summary nodes.
	result := eval(t, engine, `
		(let ((index (go-ssa-field-index "github.com/aalpar/wile-goast/goastssa")))
		  (and (pair? index)
		       (let ((first (car index)))
		         (and (pair? first)
		              (eq? (car first) 'ssa-field-summary)))))
	`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoSSAFieldIndex_Content(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	// Each entry should have func, pkg, and fields keys.
	result := eval(t, engine, `
		(let* ((index (go-ssa-field-index "github.com/aalpar/wile-goast/goastssa"))
		       (first (car index))
		       (fields (cdr first)))
		  (and (assoc 'func fields)
		       (assoc 'pkg fields)
		       (assoc 'fields fields)
		       #t))
	`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoSSAFieldIndex_AccessMode(t *testing.T) {
	c := qt.New(t)
	engine := newEngine(t)

	// Field access entries should have struct, field, recv, mode keys.
	// Mode should be read or write.
	result := eval(t, engine, `
		(let* ((index (go-ssa-field-index "github.com/aalpar/wile-goast/goastssa"))
		       (entry (let loop ((idx index))
		                (if (null? idx) #f
		                  (let ((fs (cdr (assoc 'fields (cdr (car idx))))))
		                    (if (pair? fs) (car idx) (loop (cdr idx)))))))
		       (access (car (cdr (assoc 'fields (cdr entry))))))
		  (and (assoc 'struct (cdr access))
		       (assoc 'field (cdr access))
		       (assoc 'recv (cdr access))
		       (assoc 'mode (cdr access))
		       (let ((m (cdr (assoc 'mode (cdr access)))))
		         (or (eq? m 'read) (eq? m 'write)))))
	`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoSSAFieldIndex_Errors(t *testing.T) {
	engine := newEngine(t)

	tcs := []struct {
		name string
		code string
	}{
		{name: "wrong arg type", code: `(go-ssa-field-index 42)`},
		{name: "nonexistent package", code: `(go-ssa-field-index "github.com/aalpar/wile/does-not-exist-xyz")`},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			evalExpectError(t, engine, tc.code)
		})
	}
}

func newSessionEngine(t *testing.T) *wile.Engine {
	t.Helper()
	engine, err := wile.NewEngine(context.Background(),
		wile.WithExtension(extgoast.Extension),
		wile.WithExtension(extgoastssa.Extension),
	)
	qt.New(t).Assert(err, qt.IsNil)
	return engine
}

func TestGoSSABuild_WithSession(t *testing.T) {
	engine := newSessionEngine(t)
	testutil.RunScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)
	result := testutil.RunScheme(t, engine, `(pair? (go-ssa-build s))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoSSAFieldIndex_WithSession(t *testing.T) {
	engine := newSessionEngine(t)
	testutil.RunScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goastssa"))`)
	result := testutil.RunScheme(t, engine, `(list? (go-ssa-field-index s))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoSSABuild_Errors(t *testing.T) {
	engine := newEngine(t)

	tcs := []struct {
		name string
		code string
	}{
		{name: "wrong arg type", code: `(go-ssa-build 42)`},
		{name: "nonexistent package", code: `(go-ssa-build "github.com/aalpar/wile/does-not-exist-xyz")`},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			evalExpectError(t, engine, tc.code)
		})
	}
}
