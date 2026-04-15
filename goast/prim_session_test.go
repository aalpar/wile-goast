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
	"testing"

	extgoast "github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile-goast/testutil"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/values/valuestest"

	qt "github.com/frankban/quicktest"
)

func TestGoLoad_ReturnsSession(t *testing.T) {
	engine := newEngine(t)
	result := testutil.RunScheme(t, engine,
		`(go-load "github.com/aalpar/wile-goast/goast")`)
	_, ok := extgoast.UnwrapSession(result.Internal())
	qt.New(t).Assert(ok, qt.IsTrue)
}

func TestGoLoad_MultiplePatterns(t *testing.T) {
	engine := newEngine(t)
	result := testutil.RunScheme(t, engine,
		`(go-load "github.com/aalpar/wile-goast/goast" "github.com/aalpar/wile-goast/goastssa")`)
	s, ok := extgoast.UnwrapSession(result.Internal())
	qt.New(t).Assert(ok, qt.IsTrue)
	qt.New(t).Assert(len(s.Patterns()), qt.Equals, 2)
}

func TestGoLoad_LintOption(t *testing.T) {
	engine := newEngine(t)
	result := testutil.RunScheme(t, engine,
		`(go-load "github.com/aalpar/wile-goast/goast" 'lint)`)
	s, ok := extgoast.UnwrapSession(result.Internal())
	qt.New(t).Assert(ok, qt.IsTrue)
	qt.New(t).Assert(s.IsLintMode(), qt.IsTrue)
}

func TestGoLoad_SessionPredicate(t *testing.T) {
	engine := newEngine(t)
	testutil.RunScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)

	result := testutil.RunScheme(t, engine, `(go-session? s)`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)

	result = testutil.RunScheme(t, engine, `(go-session? "not a session")`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.FalseValue)
}

func TestGoLoad_OpaqueIntegration(t *testing.T) {
	engine := newEngine(t)
	testutil.RunScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)

	result := testutil.RunScheme(t, engine, `(opaque? s)`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)

	result = testutil.RunScheme(t, engine, `(opaque-tag s)`)
	qt.New(t).Assert(result.Internal(), qt.DeepEquals, values.NewSymbol("go-session"))
}

func TestGoLoad_Errors(t *testing.T) {
	engine := newEngine(t)
	tcs := []struct {
		name string
		code string
	}{
		{name: "nonexistent package", code: `(go-load "github.com/does/not/exist-xyz")`},
		{name: "wrong arg type", code: `(go-load 42)`},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			testutil.RunSchemeExpectError(t, engine, tc.code)
		})
	}
}

func TestGoTypecheckPackage_WithSession(t *testing.T) {
	engine := newEngine(t)
	testutil.RunScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)
	result := testutil.RunScheme(t, engine, `(pair? (go-typecheck-package s))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoTypecheckPackage_SessionMatchesString(t *testing.T) {
	engine := newEngine(t)
	testutil.RunScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)
	fromSession := testutil.RunScheme(t, engine, `
		(let ((pkgs (go-typecheck-package s)))
			(cdr (assoc 'name (cdr (car pkgs)))))`)
	fromString := testutil.RunScheme(t, engine, `
		(let ((pkgs (go-typecheck-package "github.com/aalpar/wile-goast/goast")))
			(cdr (assoc 'name (cdr (car pkgs)))))`)
	qt.New(t).Assert(fromSession.Internal(), valuestest.SchemeEquals, fromString.Internal())
}

func TestGoInterfaceImplementors_WithSession(t *testing.T) {
	engine := newEngine(t)
	testutil.RunScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast/testdata/iface"))`)
	result := testutil.RunScheme(t, engine,
		`(go-node-type (go-interface-implementors "Store" s))`)
	qt.New(t).Assert(result.Internal(), valuestest.SchemeEquals, values.NewSymbol("interface-info"))
}

func TestGoListDeps_ReturnsImportPaths(t *testing.T) {
	engine := newEngine(t)
	result := testutil.RunScheme(t, engine,
		`(pair? (go-list-deps "github.com/aalpar/wile-goast/goast"))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoListDeps_IncludesStdlib(t *testing.T) {
	engine := newEngine(t)
	// goast/ imports "go/ast", so "go/ast" should appear in deps.
	result := testutil.RunScheme(t, engine, `
		(let loop ((deps (go-list-deps "github.com/aalpar/wile-goast/goast")))
			(cond ((null? deps) #f)
				  ((equal? (car deps) "go/ast") #t)
				  (else (loop (cdr deps)))))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoListDeps_MultiplePatterns(t *testing.T) {
	engine := newEngine(t)
	result := testutil.RunScheme(t, engine,
		`(pair? (go-list-deps "github.com/aalpar/wile-goast/goast"
		                      "github.com/aalpar/wile-goast/goastssa"))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}
