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

// TestGoSSANarrowPrimitiveRegistered verifies the go-ssa-narrow primitive is
// callable and returns a well-shaped narrow-result alist. Semantic narrowing
// cases are covered by Go-side fixture tests in goastssa/narrow_test.go.
func TestGoSSANarrowPrimitiveRegistered(t *testing.T) {
	goast.ResetTargetState()
	t.Setenv("WILE_GOAST_TARGET", "")

	ctx := context.Background()
	engine := buildEngine(ctx)
	defer func() { _ = engine.Close() }()

	const pkg = "github.com/aalpar/wile-goast/goast"

	result := testutil.RunScheme(t, engine,
		`(parameterize ((current-go-target "`+pkg+`"))
		   (let* ((funcs (go-ssa-build))
		          (f (car funcs)))
		     (go-ssa-narrow f "nonexistent")))`)

	// narrow-result should be a tagged alist; the stub returns
	// (narrow-result (types ()) (confidence no-paths) (reasons (value-not-found))).
	pair, ok := result.Internal().(*values.Pair)
	qt.Assert(t, ok, qt.IsTrue, qt.Commentf("result is %T, want *values.Pair", result.Internal()))
	tag, ok := pair.Car().(*values.Symbol)
	qt.Assert(t, ok, qt.IsTrue)
	qt.Assert(t, tag.Key, qt.Equals, "narrow-result")

	conf, ok := goast.GetField(pair.Cdr(), "confidence")
	qt.Assert(t, ok, qt.IsTrue)
	confSym, ok := conf.(*values.Symbol)
	qt.Assert(t, ok, qt.IsTrue)
	qt.Assert(t, confSym.Key, qt.Equals, "no-paths")
}
