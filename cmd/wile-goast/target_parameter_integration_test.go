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

func TestCurrentGoTargetDefault(t *testing.T) {
	goast.ResetTargetState()
	t.Setenv("WILE_GOAST_TARGET", "")

	engine := buildEngine(context.Background())
	defer func() { _ = engine.Close() }()

	result := testutil.RunScheme(t, engine, `(current-go-target)`)
	s, ok := result.Internal().(*values.String)
	qt.Assert(t, ok, qt.IsTrue, qt.Commentf("result is %T, want *values.String", result.Internal()))
	qt.Assert(t, s.Value, qt.Equals, "./...")
}

func TestCurrentGoTargetEnvVar(t *testing.T) {
	goast.ResetTargetState()
	t.Setenv("WILE_GOAST_TARGET", "github.com/example/foo/...")

	engine := buildEngine(context.Background())
	defer func() { _ = engine.Close() }()

	result := testutil.RunScheme(t, engine, `(current-go-target)`)
	s, ok := result.Internal().(*values.String)
	qt.Assert(t, ok, qt.IsTrue, qt.Commentf("result is %T, want *values.String", result.Internal()))
	qt.Assert(t, s.Value, qt.Equals, "github.com/example/foo/...")
}

func TestCurrentGoTargetParameterize(t *testing.T) {
	goast.ResetTargetState()
	t.Setenv("WILE_GOAST_TARGET", "")

	engine := buildEngine(context.Background())
	defer func() { _ = engine.Close() }()

	result := testutil.RunScheme(t, engine,
		`(parameterize ((current-go-target "./sub/..."))
		   (current-go-target))`)
	s, ok := result.Internal().(*values.String)
	qt.Assert(t, ok, qt.IsTrue, qt.Commentf("result is %T, want *values.String", result.Internal()))
	qt.Assert(t, s.Value, qt.Equals, "./sub/...")
}

func TestGoSSABuildUsesCurrentGoTarget(t *testing.T) {
	goast.ResetTargetState()
	t.Setenv("WILE_GOAST_TARGET", "")

	ctx := context.Background()
	engine := buildEngine(ctx)
	defer func() { _ = engine.Close() }()

	const pkg = "github.com/aalpar/wile-goast/goast"

	// Load a session first
	testutil.RunScheme(t, engine, `(define s (go-load "`+pkg+`"))`)

	// No arg — should use (current-go-target) which defaults to "./..."
	result := testutil.RunScheme(t, engine,
		`(parameterize ((current-go-target "`+pkg+`"))
		   (length (go-ssa-build)))`)
	n, ok := result.Internal().(*values.Integer)
	qt.Assert(t, ok, qt.IsTrue, qt.Commentf("result is %T, want *values.Integer", result.Internal()))
	qt.Assert(t, n.Value > 0, qt.IsTrue)
}

func TestGoSSABuildExplicitArgStillWorks(t *testing.T) {
	goast.ResetTargetState()
	t.Setenv("WILE_GOAST_TARGET", "")

	ctx := context.Background()
	engine := buildEngine(ctx)
	defer func() { _ = engine.Close() }()

	const pkg = "github.com/aalpar/wile-goast/goast"

	result := testutil.RunScheme(t, engine,
		`(length (go-ssa-build "`+pkg+`"))`)
	n, ok := result.Internal().(*values.Integer)
	qt.Assert(t, ok, qt.IsTrue, qt.Commentf("result is %T, want *values.Integer", result.Internal()))
	qt.Assert(t, n.Value > 0, qt.IsTrue)
}

func TestGoSSAFieldIndexUsesCurrentGoTarget(t *testing.T) {
	goast.ResetTargetState()
	t.Setenv("WILE_GOAST_TARGET", "")

	engine := buildEngine(context.Background())
	defer func() { _ = engine.Close() }()

	const pkg = "github.com/aalpar/wile-goast/goast"

	// No arg — should use (current-go-target)
	result := testutil.RunScheme(t, engine,
		`(parameterize ((current-go-target "`+pkg+`"))
		   (list? (go-ssa-field-index)))`)
	qt.Assert(t, result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoFuncRefsUsesCurrentGoTarget(t *testing.T) {
	goast.ResetTargetState()
	t.Setenv("WILE_GOAST_TARGET", "")

	engine := buildEngine(context.Background())
	defer func() { _ = engine.Close() }()

	const pkg = "github.com/aalpar/wile-goast/goast"

	result := testutil.RunScheme(t, engine,
		`(parameterize ((current-go-target "`+pkg+`"))
		   (list? (go-func-refs)))`)
	qt.Assert(t, result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoInterfaceImplementorsUsesCurrentGoTarget(t *testing.T) {
	goast.ResetTargetState()
	t.Setenv("WILE_GOAST_TARGET", "")

	engine := buildEngine(context.Background())
	defer func() { _ = engine.Close() }()

	// testdata/iface has `type Store interface` — pick that so the
	// call returns real data rather than a "not found" error.
	const pkg = "github.com/aalpar/wile-goast/goast/testdata/iface"

	// Only iface-name required; target defaults via parameter.
	result := testutil.RunScheme(t, engine,
		`(parameterize ((current-go-target "`+pkg+`"))
		   (pair? (go-interface-implementors "Store")))`)
	qt.Assert(t, result.Internal(), qt.Equals, values.TrueValue)
}
