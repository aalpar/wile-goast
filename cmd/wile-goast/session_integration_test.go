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

	"github.com/aalpar/wile-goast/testutil"
	"github.com/aalpar/wile/values"

	qt "github.com/frankban/quicktest"
)

func TestSession_FullPipeline(t *testing.T) {
	ctx := context.Background()
	engine := buildEngine(ctx)
	defer func() { _ = engine.Close() }()

	const pkg = "github.com/aalpar/wile-goast/goast"

	testutil.RunScheme(t, engine, `(define s (go-load "`+pkg+`"))`)

	t.Run("typecheck", func(t *testing.T) {
		result := testutil.RunScheme(t, engine, `(pair? (go-typecheck-package s))`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("ssa-build", func(t *testing.T) {
		result := testutil.RunScheme(t, engine, `(pair? (go-ssa-build s))`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("cfg", func(t *testing.T) {
		result := testutil.RunScheme(t, engine,
			`(pair? (go-cfg s "PrimGoParseExpr"))`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("callgraph", func(t *testing.T) {
		result := testutil.RunScheme(t, engine,
			`(pair? (go-callgraph s 'static))`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("field-index", func(t *testing.T) {
		result := testutil.RunScheme(t, engine,
			`(list? (go-ssa-field-index s))`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("session-predicate", func(t *testing.T) {
		result := testutil.RunScheme(t, engine, `(go-session? s)`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("opaque-tag", func(t *testing.T) {
		result := testutil.RunScheme(t, engine, `(opaque-tag s)`)
		qt.New(t).Assert(result.Internal(), qt.DeepEquals, values.NewSymbol("go-session"))
	})
}
