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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aalpar/wile/pkg/security"
	"github.com/aalpar/wile/pkg/values"
	"github.com/aalpar/wile/pkg/values/valuestest"
	"github.com/aalpar/wile/pkg/wile"

	extgoast "github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile-goast/testutil"

	qt "github.com/frankban/quicktest"
)

// writeTempModule creates a throwaway Go module in dir: a go.mod with no
// require block, plus one small .go file with a struct and a function. It
// has no dependencies so `go list` resolves it offline.
func writeTempModule(t *testing.T, dir string) {
	t.Helper()
	c := qt.New(t)
	err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/tmpmod\n\ngo 1.21\n"), 0o644)
	c.Assert(err, qt.IsNil)
	src := `package tmpmod

type Widget struct {
	Name string
}

func NewWidget(name string) Widget {
	return Widget{Name: name}
}
`
	err = os.WriteFile(filepath.Join(dir, "widget.go"), []byte(src), 0o644)
	c.Assert(err, qt.IsNil)
}

func TestGoLoad_DirOption_LoadsExternalModule(t *testing.T) {
	dir := t.TempDir()
	writeTempModule(t, dir)

	engine := newEngine(t)
	result := testutil.RunScheme(t, engine,
		`(go-load "./..." 'dir "`+dir+`")`)
	s, ok := extgoast.UnwrapSession(result.Internal())
	qt.New(t).Assert(ok, qt.IsTrue)
	qt.New(t).Assert(len(s.Packages()) > 0, qt.IsTrue)
	qt.New(t).Assert(s.Packages()[0].PkgPath, qt.Equals, "example.com/tmpmod")
}

// TestGoLoad_DirOption_AbsentDoesNotResolveTempModule is the negative half of
// the happy path: the same "./..." pattern without 'dir resolves against this
// repo's goast package (the test process's CWD), not the temp module.
func TestGoLoad_DirOption_AbsentDoesNotResolveTempModule(t *testing.T) {
	engine := newEngine(t)
	result := testutil.RunScheme(t, engine, `(go-load "./...")`)
	s, ok := extgoast.UnwrapSession(result.Internal())
	qt.New(t).Assert(ok, qt.IsTrue)
	for _, pkg := range s.Packages() {
		qt.New(t).Assert(pkg.PkgPath, qt.Not(qt.Equals), "example.com/tmpmod")
	}
}

func TestGoLoad_DirOption_NonexistentPath(t *testing.T) {
	engine := newEngine(t)
	_, err := engine.EvalMultiple(context.Background(),
		`(go-load "./..." 'dir "/no/such/path/for/go-load-dir-test")`)
	qt.New(t).Assert(err, qt.IsNotNil)
	qt.New(t).Assert(strings.Contains(err.Error(), "dir"), qt.IsTrue,
		qt.Commentf("error should name the dir option: %v", err))
}

func TestGoLoad_DirOption_PathIsFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir.txt")
	c := qt.New(t)
	c.Assert(os.WriteFile(file, []byte("x"), 0o644), qt.IsNil)

	engine := newEngine(t)
	_, err := engine.EvalMultiple(context.Background(),
		`(go-load "./..." 'dir "`+file+`")`)
	c.Assert(err, qt.IsNotNil)
	c.Assert(strings.Contains(err.Error(), "not a directory"), qt.IsTrue,
		qt.Commentf("error should say not a directory: %v", err))
}

func TestGoLoad_DirOption_MissingValue(t *testing.T) {
	engine := newEngine(t)
	testutil.RunSchemeExpectError(t, engine, `(go-load "./..." 'dir)`)
}

func TestGoLoad_DirOption_NonStringValue(t *testing.T) {
	engine := newEngine(t)
	testutil.RunSchemeExpectError(t, engine, `(go-load "./..." 'dir 42)`)
}

func TestGoLoad_DirOption_CombinedWithLintAndMultiplePatterns(t *testing.T) {
	dir := t.TempDir()
	writeTempModule(t, dir)

	engine := newEngine(t)
	result := testutil.RunScheme(t, engine,
		`(go-load "./..." "./..." 'dir "`+dir+`" 'lint)`)
	s, ok := extgoast.UnwrapSession(result.Internal())
	qt.New(t).Assert(ok, qt.IsTrue)
	qt.New(t).Assert(s.IsLintMode(), qt.IsTrue)
	qt.New(t).Assert(len(s.Patterns()), qt.Equals, 2)
	qt.New(t).Assert(len(s.Packages()) > 0, qt.IsTrue)
	qt.New(t).Assert(s.Packages()[0].PkgPath, qt.Equals, "example.com/tmpmod")
}

// TestGoLoad_DirOption_DeniedByFileAuthorizer pins the security gate added
// alongside 'dir: an authorizer that permits the ResourceProcess/ActionLoad
// check but denies ResourceFile/ActionRead on the dir target must block the
// load, mirroring PrimGoParseFile's file-scoped check.
func TestGoLoad_DirOption_DeniedByFileAuthorizer(t *testing.T) {
	dir := t.TempDir()
	writeTempModule(t, dir)

	auth := security.AuthorizerFunc(func(req security.AccessRequest) error {
		if req.Resource == security.ResourceFile && req.Target == dir {
			return security.ErrAccessDenied
		}
		return nil
	})
	engine, err := wile.NewEngine(context.Background(),
		wile.WithExtension(extgoast.Extension),
		wile.WithAuthorizer(auth),
	)
	qt.New(t).Assert(err, qt.IsNil)

	_, evalErr := engine.EvalMultiple(context.Background(),
		`(go-load "./..." 'dir "`+dir+`")`)
	qt.New(t).Assert(evalErr, qt.IsNotNil)
	qt.New(t).Assert(errors.Is(evalErr, security.ErrAccessDenied), qt.IsTrue)
}

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
