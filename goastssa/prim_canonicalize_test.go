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
	"testing"

	"github.com/aalpar/wile/values"

	qt "github.com/frankban/quicktest"
)

func TestGoSSACanonicalize_Errors(t *testing.T) {
	engine := newEngine(t)

	t.Run("wrong arg type", func(t *testing.T) {
		evalExpectError(t, engine, `(go-ssa-canonicalize 42)`)
	})

	t.Run("wrong tag", func(t *testing.T) {
		evalExpectError(t, engine, `(go-ssa-canonicalize '(ssa-block (index . 0)))`)
	})

	t.Run("missing required name field", func(t *testing.T) {
		evalExpectError(t, engine, `(go-ssa-canonicalize '(ssa-func (blocks . ())))`)
	})

	t.Run("non-integer block index", func(t *testing.T) {
		evalExpectError(t, engine, `
			(go-ssa-canonicalize
			  '(ssa-func
			     (name . "F") (signature . "func()") (pkg . "p")
			     (params . ()) (free-vars . ())
			     (blocks . ((ssa-block (index . "not-an-int") (preds . ()) (succs . ()) (instrs . ()))))))`)
	})

	t.Run("non-integer idom", func(t *testing.T) {
		evalExpectError(t, engine, `
			(go-ssa-canonicalize
			  '(ssa-func
			     (name . "F") (signature . "func()") (pkg . "p")
			     (params . ()) (free-vars . ())
			     (blocks . ((ssa-block (index . 0) (idom . "bad")
			                (preds . ()) (succs . ()) (instrs . ()))))))`)
	})

	t.Run("no entry block (all blocks have idom)", func(t *testing.T) {
		// Every block has an idom field, so entryIdx stays -1.
		evalExpectError(t, engine, `
			(go-ssa-canonicalize
			  '(ssa-func
			     (name . "F") (signature . "func()") (pkg . "p")
			     (params . ()) (free-vars . ())
			     (blocks . ((ssa-block (index . 0) (idom . 1)
			                 (preds . ()) (succs . ()) (instrs . ()))
			                (ssa-block (index . 1) (idom . 0)
			                 (preds . ()) (succs . ()) (instrs . ()))))))`)
	})

	t.Run("malformed params list (not a pair)", func(t *testing.T) {
		evalExpectError(t, engine, `
			(go-ssa-canonicalize
			  '(ssa-func
			     (name . "F") (signature . "func()") (pkg . "p")
			     (params . (42))
			     (free-vars . ())
			     (blocks . ())))`)
	})
}

func TestGoSSACanonicalize_SingleBlock(t *testing.T) {
	engine := newEngine(t)

	// Find a function with exactly one block (single-block functions exist).
	eval(t, engine, `(define funcs (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)
	eval(t, engine, `
		(define single-fn
			(let loop ((fs funcs))
				(if (null? fs) #f
					(let* ((fn (car fs))
						   (blocks (cdr (assoc 'blocks (cdr fn)))))
						(if (= (length blocks) 1) fn (loop (cdr fs)))))))`)

	// If no single-block function found, skip.
	found := eval(t, engine, `(pair? single-fn)`)
	if found.Internal() != values.TrueValue {
		t.Skip("no single-block function found in package")
	}

	result := eval(t, engine, `(eq? (car (go-ssa-canonicalize single-fn)) 'ssa-func)`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoSSACanonicalize_ReturnsSSAFunc(t *testing.T) {
	engine := newEngine(t)

	// Build SSA, grab one function, canonicalize it.
	eval(t, engine, `(define funcs (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)
	eval(t, engine, `(define fn (car funcs))`)
	result := eval(t, engine, `(eq? (car (go-ssa-canonicalize fn)) 'ssa-func)`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoSSACanonicalize_RegisterRenaming(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `(define funcs (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)
	eval(t, engine, `(define fn (car funcs))`)
	eval(t, engine, `(define canon (go-ssa-canonicalize fn))`)

	t.Run("params are p0 p1 etc", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((params (cdr (assoc 'params (cdr canon))))
				   (first (car params)))
				(equal? (cdr (assoc 'name (cdr first))) "p0"))`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("instruction names start with r", func(t *testing.T) {
		// Find the first named instruction in block 0.
		result := eval(t, engine, `
			(let* ((blocks (cdr (assoc 'blocks (cdr canon))))
				   (entry (car blocks))
				   (instrs (cdr (assoc 'instrs (cdr entry)))))
				(let loop ((is instrs))
					(if (null? is) #f
						(let ((name-pair (assoc 'name (cdr (car is)))))
							(if name-pair
								(string-ref (cdr name-pair) 0)
								(loop (cdr is)))))))`)
		// First character should be 'r'
		c, ok := result.Internal().(*values.Character)
		if ok {
			qt.New(t).Assert(c.Value, qt.Equals, 'r')
		} else {
			// If no named instruction in block 0, that's fine
			t.Skip("no named instruction in entry block")
		}
	})
}

func TestGoSSACanonicalize_BlockOrder(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `(define funcs (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)
	eval(t, engine, `
		(define multi-fn
			(let loop ((fs funcs))
				(if (null? fs) #f
					(let* ((fn (car fs))
						   (blocks (cdr (assoc 'blocks (cdr fn)))))
						(if (> (length blocks) 2) fn (loop (cdr fs)))))))`)
	eval(t, engine, `(define canon (go-ssa-canonicalize multi-fn))`)

	t.Run("block 0 is entry (no idom)", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((blocks (cdr (assoc 'blocks (cdr canon))))
				   (entry (car blocks)))
				(not (assoc 'idom (cdr entry))))`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("block indices are sequential", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((blocks (cdr (assoc 'blocks (cdr canon)))))
				(let loop ((bs blocks) (i 0))
					(if (null? bs) #t
						(let ((idx (cdr (assoc 'index (cdr (car bs))))))
							(if (= idx i)
								(loop (cdr bs) (+ i 1))
								#f)))))`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("idom references use new indices (parent < child)", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((blocks (cdr (assoc 'blocks (cdr canon)))))
				(let loop ((bs (cdr blocks)))
					(if (null? bs) #t
						(let* ((b (car bs))
							   (idx (cdr (assoc 'index (cdr b))))
							   (idom-pair (assoc 'idom (cdr b))))
							(if idom-pair
								(if (< (cdr idom-pair) idx)
									(loop (cdr bs))
									#f)
								(loop (cdr bs)))))))`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})
}
