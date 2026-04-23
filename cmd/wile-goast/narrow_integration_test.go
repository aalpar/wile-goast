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

package main

import (
	"context"
	"testing"

	"github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile-goast/testutil"
	"github.com/aalpar/wile/values"

	qt "github.com/frankban/quicktest"
)

// TestGoSSANarrowValueNotFoundErrors verifies that an unknown value name
// raises an error rather than synthesizing a no-paths verdict. A caller
// who mistypes a value name gets told immediately; they don't silently
// receive a plausible 'no producing paths' claim.
func TestGoSSANarrowValueNotFoundErrors(t *testing.T) {
	goast.ResetTargetState()
	t.Setenv("WILE_GOAST_TARGET", "")

	ctx := context.Background()
	engine := buildEngine(ctx)
	defer func() { _ = engine.Close() }()

	const pkg = "github.com/aalpar/wile-goast/goast"

	_, err := engine.EvalMultiple(ctx,
		`(parameterize ((current-go-target "`+pkg+`"))
		   (let* ((funcs (go-ssa-build))
		          (f (car funcs)))
		     (go-ssa-narrow f "nonexistent")))`)
	qt.Assert(t, err, qt.IsNotNil)
	qt.Assert(t, err.Error(), qt.Contains, "no value named")
}

// TestGoSSANarrowParameterWidens exercises the end-to-end Scheme binding
// on a function whose first parameter is interface-typed: the runtime
// value depends on call-site context, so narrowing must widen with the
// "parameter" reason. This mirrors goastssa.TestNarrowParameterWidens at
// the Scheme-binding layer.
//
// The fixture scans the goast package for the first function whose first
// parameter's type string contains "values.Value" — an interface from
// github.com/aalpar/wile. Matching by type-string keeps the test decoupled
// from SSA-member-list ordering without sacrificing outcome determinism.
func TestGoSSANarrowParameterWidens(t *testing.T) {
	goast.ResetTargetState()
	t.Setenv("WILE_GOAST_TARGET", "")

	ctx := context.Background()
	engine := buildEngine(ctx)
	defer func() { _ = engine.Close() }()

	const pkg = "github.com/aalpar/wile-goast/goast"

	result := testutil.RunScheme(t, engine,
		`(import (wile goast utils))
		 (parameterize ((current-go-target "`+pkg+`"))
		   (let loop ((funcs (go-ssa-build)))
		     (cond ((null? funcs) #f)
		           (else
		            (let* ((f (car funcs))
		                   (params (nf f 'params)))
		              (cond ((or (not params) (null? params))
		                     (loop (cdr funcs)))
		                    ((string-contains (nf (car params) 'type) "values.Value")
		                     (go-ssa-narrow f (nf (car params) 'name)))
		                    (else (loop (cdr funcs))))))))) `)

	pair, ok := result.Internal().(*values.Pair)
	qt.Assert(t, ok, qt.IsTrue,
		qt.Commentf("result is %T, want *values.Pair (no interface-typed parameter found?)", result.Internal()))

	tag, ok := pair.Car().(*values.Symbol)
	qt.Assert(t, ok, qt.IsTrue)
	qt.Assert(t, tag.Key, qt.Equals, "narrow-result")

	conf, ok := goast.GetField(pair.Cdr(), "confidence")
	qt.Assert(t, ok, qt.IsTrue)
	confSym, ok := conf.(*values.Symbol)
	qt.Assert(t, ok, qt.IsTrue)
	qt.Assert(t, confSym.Key, qt.Equals, "widened",
		qt.Commentf("interface-typed parameter must widen, got confidence=%q", confSym.Key))

	reasons, ok := goast.GetField(pair.Cdr(), "reasons")
	qt.Assert(t, ok, qt.IsTrue)
	reasonsPair, ok := reasons.(*values.Pair)
	qt.Assert(t, ok, qt.IsTrue,
		qt.Commentf("reasons is %T, want *values.Pair", reasons))
	firstReason, ok := reasonsPair.Car().(*values.Symbol)
	qt.Assert(t, ok, qt.IsTrue)
	qt.Assert(t, firstReason.Key, qt.Equals, "parameter",
		qt.Commentf("expected first reason=parameter for interface-typed param, got %q", firstReason.Key))
}

// TestGoSSANarrowConcreteAlloc exercises the narrow-confidence path: find
// the first ssa-alloc in any function and narrow it. An Alloc always
// produces a concrete pointer type, so confidence must be 'narrow' and
// types must have exactly one entry.
func TestGoSSANarrowConcreteAlloc(t *testing.T) {
	goast.ResetTargetState()
	t.Setenv("WILE_GOAST_TARGET", "")

	ctx := context.Background()
	engine := buildEngine(ctx)
	defer func() { _ = engine.Close() }()

	const pkg = "github.com/aalpar/wile-goast/goast"

	result := testutil.RunScheme(t, engine,
		`(import (wile goast utils))
		 (define (find-alloc f)
		   (let block-loop ((bs (nf f 'blocks)))
		     (cond ((or (not bs) (null? bs)) #f)
		           (else
		            (let instr-loop ((is (nf (car bs) 'instrs)))
		              (cond ((or (not is) (null? is))
		                     (block-loop (cdr bs)))
		                    ((tag? (car is) 'ssa-alloc)
		                     (nf (car is) 'name))
		                    (else (instr-loop (cdr is)))))))))
		 (parameterize ((current-go-target "`+pkg+`"))
		   (let loop ((funcs (go-ssa-build)))
		     (cond ((null? funcs) #f)
		           (else
		            (let* ((f (car funcs))
		                   (alloc-name (find-alloc f)))
		              (if alloc-name
		                  (go-ssa-narrow f alloc-name)
		                  (loop (cdr funcs))))))))`)

	// Result must be a narrow-result alist with confidence='narrow and at
	// least one type. The type string is Go-internal (*pkg.Type) — we don't
	// pin it exactly, only that the algorithm recovered a concrete type.
	pair, ok := result.Internal().(*values.Pair)
	qt.Assert(t, ok, qt.IsTrue,
		qt.Commentf("result is %T, want *values.Pair", result.Internal()))

	conf, ok := goast.GetField(pair.Cdr(), "confidence")
	qt.Assert(t, ok, qt.IsTrue)
	confSym, ok := conf.(*values.Symbol)
	qt.Assert(t, ok, qt.IsTrue)
	qt.Assert(t, confSym.Key, qt.Equals, "narrow",
		qt.Commentf("expected confidence=narrow for ssa-alloc operand"))

	types, ok := goast.GetField(pair.Cdr(), "types")
	qt.Assert(t, ok, qt.IsTrue)
	typesPair, ok := types.(*values.Pair)
	qt.Assert(t, ok, qt.IsTrue,
		qt.Commentf("types is %T, want *values.Pair (non-empty list)", types))
	// First entry is a Scheme string naming the Go type.
	firstType, ok := typesPair.Car().(*values.String)
	qt.Assert(t, ok, qt.IsTrue)
	qt.Assert(t, firstType.Value != "", qt.IsTrue,
		qt.Commentf("type name should be non-empty, got %q", firstType.Value))
}
