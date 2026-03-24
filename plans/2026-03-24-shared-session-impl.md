# Shared Session API Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Introduce `GoSession` — a first-class opaque value holding loaded Go packages with lazy SSA/callgraph building — so all analysis primitives can share loaded state.

**Architecture:** `GoSession` lives in `goast/` (base package). New primitives `go-load`, `go-list-deps`, `go-session?` create and inspect sessions. All 7 package-loading primitives gain a type switch accepting either a string (load fresh) or a GoSession (reuse). Belief DSL updated to create a session in `make-context`.

**Tech Stack:** Go (`golang.org/x/tools/go/packages`, `go/ssa`, `go/ssa/ssautil`), Scheme (belief DSL), wile extension API.

**Prerequisite:** ~~`OpaqueValue` type in wile core (blocked).~~ Resolved: wile v1.9.1 ships `values.Opaque` interface + `values.OpaqueValue` struct.

---

## Amendments (2026-03-24 — post-verification)

Corrections found by verifying plan assumptions against the current codebase.

### A0: New Task — shared test utility (before Task 1)

Create `testutil/testutil.go` with `RunScheme` and `RunSchemeExpectError`.
All test code across all 5 packages uses these instead of per-package
helpers. Engine builders remain per-package (different extension combos).

### A1: Test helper rename

Plan uses `runScheme`. Actual codebase helper is named differently.
**Decision:** use `testutil.RunScheme` (exported from shared package).
Plan's `runScheme(t, engine, ...)` → `testutil.RunScheme(t, engine, ...)`

### A2: Sub-extension tests need `goast.Extension`

Dual-accept tests (Tasks 6-10) call `(go-load ...)` which lives in
`goast.Extension`. Each sub-extension test file needs a `newSessionEngine(t)`
that loads both `goast.Extension` and its own extension.

### A3: `go-ssa-field-index` is non-variadic

`ParamCount: 1` — no rest args. Type switch on `mc.Arg(0)` must come
BEFORE `helpers.RequireArg`, not after.

### A4: SSA flag consistency

Add `ssa.InstantiateGenerics` to the string paths of `go-ssa-build` and
`go-ssa-field-index` to match the session path and `go-callgraph`. This
is a separate commit before the dual-accept refactors.

### A5: GoSession implements `values.Opaque`

Add `func (s *GoSession) OpaqueTag() string { return "go-session" }` so
`(opaque? session)` returns `#t` and `(opaque-tag session)` returns
`'go-session`.

### A6: Use `strings.Join` not custom `joinErrors`

Match existing codebase pattern in `prim_goast.go`.

---

---

### Task 1: GoSession struct

**Files:**
- Create: `goast/session.go`
- Test: `goast/session_test.go`

**Step 1: Write the test**

```go
// goast/session_test.go
package goast_test

import (
	"testing"

	"github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile/values"

	qt "github.com/frankban/quicktest"
)

func TestGoSession_SchemeString(t *testing.T) {
	c := qt.New(t)
	s := goast.NewGoSession([]string{"my/pkg"}, nil, nil, false)
	c.Assert(s.SchemeString(), qt.Matches, `#<go-session.*my/pkg.*>`)
}

func TestGoSession_IsVoid(t *testing.T) {
	c := qt.New(t)
	s := goast.NewGoSession([]string{"my/pkg"}, nil, nil, false)
	c.Assert(s.IsVoid(), qt.IsFalse)
	var nilSession *goast.GoSession
	c.Assert(nilSession.IsVoid(), qt.IsTrue)
}

func TestGoSession_EqualTo(t *testing.T) {
	c := qt.New(t)
	s1 := goast.NewGoSession([]string{"my/pkg"}, nil, nil, false)
	s2 := goast.NewGoSession([]string{"my/pkg"}, nil, nil, false)
	c.Assert(s1.EqualTo(s1), qt.IsTrue)
	c.Assert(s1.EqualTo(s2), qt.IsFalse) // identity, not structural
}

func TestGoSession_ImplementsValue(t *testing.T) {
	var _ values.Value = (*goast.GoSession)(nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run TestGoSession -v`
Expected: FAIL — `GoSession` not defined.

**Step 3: Write the struct**

```go
// goast/session.go
package goast

import (
	"fmt"
	"go/token"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/aalpar/wile/values"
)

// GoSession holds loaded Go packages and lazily-built analysis state.
// All package-loading primitives accept a GoSession to reuse loaded state
// instead of calling packages.Load independently.
type GoSession struct {
	patterns []string
	pkgs     []*packages.Package
	fset     *token.FileSet
	lintMode bool

	ssaOnce     sync.Once
	prog        *ssa.Program
	ssaPkgs     []*ssa.Package
	allPkgsOnce sync.Once

	// Generic cache for sub-extension use (callgraph, etc.).
	cacheMu sync.Mutex
	cache   map[string]any
}

// NewGoSession creates a GoSession from already-loaded packages.
func NewGoSession(patterns []string, pkgs []*packages.Package, fset *token.FileSet, lintMode bool) *GoSession {
	return &GoSession{
		patterns: patterns,
		pkgs:     pkgs,
		fset:     fset,
		lintMode: lintMode,
		cache:    make(map[string]any),
	}
}

func (s *GoSession) SchemeString() string {
	return fmt.Sprintf("#<go-session %q %d-pkgs>",
		strings.Join(s.patterns, " "), len(s.pkgs))
}

func (s *GoSession) IsVoid() bool {
	return s == nil
}

func (s *GoSession) EqualTo(v values.Value) bool {
	return s == v
}

// Packages returns the loaded packages.
func (s *GoSession) Packages() []*packages.Package { return s.pkgs }

// FileSet returns the shared token.FileSet.
func (s *GoSession) FileSet() *token.FileSet { return s.fset }

// IsLintMode returns true if loaded with LoadAllSyntax.
func (s *GoSession) IsLintMode() bool { return s.lintMode }

// Patterns returns the root patterns used to load this session.
func (s *GoSession) Patterns() []string { return s.patterns }

// SSA lazily builds SSA for the requested packages.
func (s *GoSession) SSA() (*ssa.Program, []*ssa.Package) {
	s.ssaOnce.Do(func() {
		s.prog, s.ssaPkgs = ssautil.Packages(s.pkgs,
			ssa.SanityCheckFunctions|ssa.InstantiateGenerics)
		for _, pkg := range s.ssaPkgs {
			if pkg != nil {
				pkg.Build()
			}
		}
	})
	return s.prog, s.ssaPkgs
}

// SSAAllPackages lazily builds SSA for all transitively loaded packages.
// Required by callgraph algorithms that need cross-package edges.
func (s *GoSession) SSAAllPackages() *ssa.Program {
	prog, _ := s.SSA()
	s.allPkgsOnce.Do(func() {
		for _, pkg := range prog.AllPackages() {
			pkg.Build()
		}
	})
	return prog
}

// CachedValue retrieves a cached value by key. Sub-extensions use this
// to cache derived data (e.g., callgraphs) without GoSession importing
// their packages.
func (s *GoSession) CachedValue(key string) (any, bool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	v, ok := s.cache[key]
	return v, ok
}

// SetCachedValue stores a value in the session cache.
func (s *GoSession) SetCachedValue(key string, v any) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cache[key] = v
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./goast/ -run TestGoSession -v`
Expected: PASS

**Step 5: Commit**

```
git add goast/session.go goast/session_test.go
git commit -m "feat: add GoSession struct with lazy SSA building"
```

---

### Task 2: `go-load` and `go-session?` primitives

**Files:**
- Create: `goast/prim_session.go`
- Create: `goast/prim_session_test.go`
- Modify: `goast/register.go`

**Step 1: Write the test**

```go
// goast/prim_session_test.go
package goast_test

import (
	"testing"

	extgoast "github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile/values"

	qt "github.com/frankban/quicktest"
)

func TestGoLoad_ReturnsSession(t *testing.T) {
	engine := newEngine(t)
	result := runScheme(t, engine,
		`(go-load "github.com/aalpar/wile-goast/goast")`)
	_, ok := result.Internal().(*extgoast.GoSession)
	qt.New(t).Assert(ok, qt.IsTrue)
}

func TestGoLoad_MultiplePatterns(t *testing.T) {
	engine := newEngine(t)
	result := runScheme(t, engine,
		`(go-load "github.com/aalpar/wile-goast/goast" "github.com/aalpar/wile-goast/goastssa")`)
	s, ok := result.Internal().(*extgoast.GoSession)
	qt.New(t).Assert(ok, qt.IsTrue)
	qt.New(t).Assert(len(s.Patterns()), qt.Equals, 2)
}

func TestGoLoad_SessionPredicate(t *testing.T) {
	engine := newEngine(t)
	runScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)

	result := runScheme(t, engine, `(go-session? s)`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)

	result = runScheme(t, engine, `(go-session? "not a session")`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.FalseValue)
}

func TestGoLoad_LintOption(t *testing.T) {
	engine := newEngine(t)
	result := runScheme(t, engine,
		`(go-load "github.com/aalpar/wile-goast/goast" 'lint)`)
	s, ok := result.Internal().(*extgoast.GoSession)
	qt.New(t).Assert(ok, qt.IsTrue)
	qt.New(t).Assert(s.IsLintMode(), qt.IsTrue)
}

func TestGoLoad_Errors(t *testing.T) {
	engine := newEngine(t)
	tcs := []struct {
		name string
		code string
	}{
		{name: "nonexistent package", code: `(go-load "github.com/does/not/exist-xyz")`},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			evalExpectError(t, engine, tc.code)
		})
	}
}
```

Note: use `runScheme` as the test helper name if `eval` triggers security hooks.
If the existing test files already use a helper named `eval`, keep consistency
and rename in this file to match. The existing tests in `goast/prim_goast_test.go`
define `eval` at line 42 — use that same function. If the test file is in the
same package (`goast_test`), it has access to `eval` from the other test file.

**Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run "TestGoLoad|TestGoSession_Pred" -v`
Expected: FAIL — `PrimGoLoad` not defined.

**Step 3: Write the primitive**

```go
// goast/prim_session.go
package goast

import (
	"go/token"
	"sort"

	"golang.org/x/tools/go/packages"

	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/security"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

var errGoLoadError = werr.NewStaticError("go load error")

// PrimGoLoad implements (go-load pattern ... . options).
// Loads Go packages and returns a GoSession for reuse across primitives.
func PrimGoLoad(mc *machine.MachineContext) error {
	err := security.CheckWithAuthorizer(mc.Authorizer(), security.AccessRequest{
		Resource: security.ResourceProcess,
		Action:   security.ActionLoad,
		Target:   "go",
	})
	if err != nil {
		return err
	}

	// Collect pattern strings and option symbols from variadic args.
	var patterns []string
	lintMode := false

	// The primitive is variadic: arg(0) is the first pattern,
	// arg(1) is the rest-args pair list.
	first, err := helpers.RequireArg[*values.String](mc, 0, werr.ErrNotAString, "go-load")
	if err != nil {
		// Check if first arg is a symbol (option with no patterns).
		return werr.WrapForeignErrorf(errGoLoadError,
			"go-load: first argument must be a package pattern string")
	}
	patterns = append(patterns, first.Value)

	// Walk variadic rest.
	rest := mc.Arg(1)
	tuple, ok := rest.(values.Tuple)
	if ok {
		for !values.IsEmptyList(tuple) {
			pair, ok := tuple.(*values.Pair)
			if !ok {
				break
			}
			switch v := pair.Car().(type) {
			case *values.String:
				patterns = append(patterns, v.Value)
			case *values.Symbol:
				if v.Key == "lint" {
					lintMode = true
				} else {
					return werr.WrapForeignErrorf(errGoLoadError,
						"go-load: unknown option '%s'; valid options: lint", v.Key)
				}
			}
			tuple, ok = pair.Cdr().(values.Tuple)
			if !ok {
				break
			}
		}
	}

	fset := token.NewFileSet()
	mode := packages.NeedName |
		packages.NeedFiles |
		packages.NeedSyntax |
		packages.NeedTypes |
		packages.NeedTypesInfo |
		packages.NeedImports |
		packages.NeedDeps
	if lintMode {
		mode = packages.LoadAllSyntax
	}

	cfg := &packages.Config{
		Mode:    mode,
		Context: mc.Context(),
		Fset:    fset,
	}

	pkgs, loadErr := packages.Load(cfg, patterns...)
	if loadErr != nil {
		return werr.WrapForeignErrorf(errGoLoadError,
			"go-load: %s", loadErr)
	}

	var errs []string
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			errs = append(errs, e.Error())
		}
	}
	if len(errs) > 0 {
		return werr.WrapForeignErrorf(errGoLoadError,
			"go-load: %s", joinErrors(errs))
	}

	mc.SetValue(NewGoSession(patterns, pkgs, fset, lintMode))
	return nil
}

// PrimGoSessionP implements (go-session? v).
func PrimGoSessionP(mc *machine.MachineContext) error {
	_, ok := mc.Arg(0).(*GoSession)
	if ok {
		mc.SetValue(values.TrueValue)
	} else {
		mc.SetValue(values.FalseValue)
	}
	return nil
}

func joinErrors(errs []string) string {
	if len(errs) == 1 {
		return errs[0]
	}
	result := errs[0]
	for _, e := range errs[1:] {
		result += "; " + e
	}
	return result
}
```

Note: import `"github.com/aalpar/wile/registry/helpers"` for `helpers.RequireArg`.
Check the existing import pattern in `goast/prim_goast.go` for the correct alias.

**Step 4: Register the new primitives**

Add to `goast/register.go` inside `addPrimitives`, before the closing `})`:

```go
{Name: "go-load", ParamCount: 2, IsVariadic: true, Impl: PrimGoLoad,
	Doc:        "Loads Go packages and returns a GoSession for reuse across analysis primitives.",
	ParamNames: []string{"pattern", "rest"}, Category: "goast"},
{Name: "go-session?", ParamCount: 1, Impl: PrimGoSessionP,
	Doc:        "Returns #t if the argument is a GoSession.",
	ParamNames: []string{"v"}, Category: "goast"},
```

**Step 5: Run tests**

Run: `go test ./goast/ -run "TestGoLoad" -v`
Expected: PASS

**Step 6: Commit**

```
git add goast/prim_session.go goast/prim_session_test.go goast/register.go
git commit -m "feat: add go-load and go-session? primitives"
```

---

### Task 3: `go-list-deps` primitive

**Files:**
- Modify: `goast/prim_session.go`
- Modify: `goast/prim_session_test.go`
- Modify: `goast/register.go`

**Step 1: Write the test**

Add to `goast/prim_session_test.go`:

```go
func TestGoListDeps_ReturnsImportPaths(t *testing.T) {
	engine := newEngine(t)
	result := runScheme(t, engine,
		`(pair? (go-list-deps "github.com/aalpar/wile-goast/goast"))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoListDeps_IncludesStdlib(t *testing.T) {
	engine := newEngine(t)
	// goast/ imports "go/ast", so "go/ast" should appear in deps.
	result := runScheme(t, engine, `
		(let loop ((deps (go-list-deps "github.com/aalpar/wile-goast/goast")))
			(cond ((null? deps) #f)
				  ((equal? (car deps) "go/ast") #t)
				  (else (loop (cdr deps)))))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoListDeps_MultiplePatterns(t *testing.T) {
	engine := newEngine(t)
	result := runScheme(t, engine,
		`(pair? (go-list-deps "github.com/aalpar/wile-goast/goast"
		                      "github.com/aalpar/wile-goast/goastssa"))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run TestGoListDeps -v`
Expected: FAIL

**Step 3: Write the primitive**

Add to `goast/prim_session.go`:

```go
// PrimGoListDeps implements (go-list-deps pattern ...).
// Lightweight dependency discovery — returns the transitive closure of
// import paths without type checking or syntax loading.
func PrimGoListDeps(mc *machine.MachineContext) error {
	err := security.CheckWithAuthorizer(mc.Authorizer(), security.AccessRequest{
		Resource: security.ResourceProcess,
		Action:   security.ActionLoad,
		Target:   "go",
	})
	if err != nil {
		return err
	}

	first, err := helpers.RequireArg[*values.String](mc, 0, werr.ErrNotAString, "go-list-deps")
	if err != nil {
		return err
	}
	patterns := []string{first.Value}

	// Collect additional patterns from variadic rest.
	rest := mc.Arg(1)
	tuple, ok := rest.(values.Tuple)
	if ok {
		for !values.IsEmptyList(tuple) {
			pair, ok := tuple.(*values.Pair)
			if !ok {
				break
			}
			sv, ok := pair.Car().(*values.String)
			if ok {
				patterns = append(patterns, sv.Value)
			}
			tuple, ok = pair.Cdr().(values.Tuple)
			if !ok {
				break
			}
		}
	}

	cfg := &packages.Config{
		Mode:    packages.NeedName | packages.NeedImports,
		Context: mc.Context(),
	}

	pkgs, loadErr := packages.Load(cfg, patterns...)
	if loadErr != nil {
		return werr.WrapForeignErrorf(errGoLoadError,
			"go-list-deps: %s", loadErr)
	}

	// BFS to collect transitive import paths.
	seen := make(map[string]bool)
	queue := append([]*packages.Package{}, pkgs...)
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		if seen[pkg.PkgPath] {
			continue
		}
		seen[pkg.PkgPath] = true
		for _, imp := range pkg.Imports {
			if !seen[imp.PkgPath] {
				queue = append(queue, imp)
			}
		}
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	result := make([]values.Value, len(paths))
	for i, p := range paths {
		result[i] = Str(p)
	}
	mc.SetValue(ValueList(result))
	return nil
}
```

**Step 4: Register**

Add to `goast/register.go`:

```go
{Name: "go-list-deps", ParamCount: 2, IsVariadic: true, Impl: PrimGoListDeps,
	Doc:        "Returns the transitive closure of import paths for the given package patterns.",
	ParamNames: []string{"pattern", "rest"}, Category: "goast"},
```

**Step 5: Run tests**

Run: `go test ./goast/ -run TestGoListDeps -v`
Expected: PASS

**Step 6: Commit**

```
git add goast/prim_session.go goast/prim_session_test.go goast/register.go
git commit -m "feat: add go-list-deps primitive for dependency discovery"
```

---

### Task 4: Dual-accept `go-typecheck-package`

This is the template for all dual-accept refactors (Tasks 5-10).

**Files:**
- Modify: `goast/prim_goast.go`
- Modify: `goast/prim_session_test.go`

**Step 1: Write the test**

```go
func TestGoTypecheckPackage_WithSession(t *testing.T) {
	engine := newEngine(t)
	runScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)
	result := runScheme(t, engine, `(pair? (go-typecheck-package s))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoTypecheckPackage_SessionMatchesString(t *testing.T) {
	engine := newEngine(t)
	runScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)
	fromSession := runScheme(t, engine, `
		(let ((pkgs (go-typecheck-package s)))
			(cdr (assoc 'name (cdr (car pkgs)))))`)
	fromString := runScheme(t, engine, `
		(let ((pkgs (go-typecheck-package "github.com/aalpar/wile-goast/goast")))
			(cdr (assoc 'name (cdr (car pkgs)))))`)
	qt.New(t).Assert(fromSession.Internal(), qt.DeepEquals, fromString.Internal())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run TestGoTypecheckPackage_With -v`
Expected: FAIL

**Step 3: Refactor `PrimGoTypecheckPackage`**

Replace the function in `goast/prim_goast.go`:

```go
func PrimGoTypecheckPackage(mc *machine.MachineContext) error {
	arg := mc.Arg(0)
	switch v := arg.(type) {
	case *GoSession:
		return typecheckFromSession(mc, v)
	case *values.String:
		return typecheckFromPattern(mc, v)
	default:
		return werr.WrapForeignErrorf(werr.ErrNotAString,
			"go-typecheck-package: expected string or go-session, got %T", arg)
	}
}

func typecheckFromSession(mc *machine.MachineContext, session *GoSession) error {
	baseOpts, _ := parseOpts(mc.Arg(1), session.FileSet())
	result := make([]values.Value, len(session.Packages()))
	for i, pkg := range session.Packages() {
		result[i] = mapPackage(pkg, baseOpts)
	}
	mc.SetValue(ValueList(result))
	return nil
}
```

Move the existing body (lines 228-283) into `typecheckFromPattern`, unchanged
except for the function signature: `func typecheckFromPattern(mc *machine.MachineContext, pattern *values.String) error`.

**Step 4: Run all typecheck tests**

Run: `go test ./goast/ -run TestGoTypecheckPackage -v`
Expected: PASS (both new session tests and existing string tests).

**Step 5: Commit**

```
git add goast/prim_goast.go goast/prim_session_test.go
git commit -m "feat: go-typecheck-package accepts GoSession"
```

---

### Task 5: Dual-accept `go-interface-implementors`

**Files:** `goast/prim_goast.go`, `goast/prim_session_test.go`

Note: session is at **arg 1** (not arg 0). The interface name is arg 0.

**Test:**

```go
func TestGoInterfaceImplementors_WithSession(t *testing.T) {
	engine := newEngine(t)
	runScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)
	result := runScheme(t, engine,
		`(go-node-type (go-interface-implementors "Value" s))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.NewSymbol("interface-info"))
}
```

Extract the interface-search and implementor-scan logic into a shared function
`findImplementors(pkgs []*packages.Package, ifaceName string) (values.Value, error)`
that both `implementorsFromSession` and `implementorsFromPattern` call.

**Commit:** `feat: go-interface-implementors accepts GoSession`

---

### Task 6: Dual-accept `go-ssa-build`

**Files:** `goastssa/prim_ssa.go`, `goastssa/prim_ssa_test.go`

**Test:**

```go
func TestGoSSABuild_WithSession(t *testing.T) {
	engine := newEngine(t)
	runScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)
	result := runScheme(t, engine, `(pair? (go-ssa-build s))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}
```

Session path calls `session.SSA()` → gets `(prog, ssaPkgs)` → runs the same
member/method iteration and mapper logic as existing code. Move existing body
to `ssaBuildFromPattern`.

**Commit:** `feat: go-ssa-build accepts GoSession`

---

### Task 7: Dual-accept `go-ssa-field-index`

**Files:** `goastssa/prim_ssa.go`, `goastssa/prim_ssa_test.go`

**Test:**

```go
func TestGoSSAFieldIndex_WithSession(t *testing.T) {
	engine := newEngine(t)
	runScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)
	result := runScheme(t, engine, `(list? (go-ssa-field-index s))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}
```

Session path calls `session.SSA()` → iterates `ssaPkgs` → calls
`collectFieldSummaries` (existing function, unchanged).

**Commit:** `feat: go-ssa-field-index accepts GoSession`

---

### Task 8: Dual-accept `go-cfg`

**Files:** `goastcfg/prim_cfg.go`, `goastcfg/prim_cfg_test.go`

**Test:**

```go
func TestGoCFG_WithSession(t *testing.T) {
	engine := newEngine(t)
	runScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)
	result := runScheme(t, engine,
		`(pair? (go-cfg s "PrimGoParseExpr"))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}
```

Session path calls `session.SSA()`, then `findFunction(prog, ssaPkg, funcName)`
(existing function in `prim_cfg.go:152`), then `mapper.mapFunction(fn)`.

**Commit:** `feat: go-cfg accepts GoSession`

---

### Task 9: Dual-accept `go-callgraph`

**Files:** `goastcg/prim_callgraph.go`, `goastcg/prim_callgraph_test.go`

**Test:**

```go
func TestGoCallgraph_WithSession(t *testing.T) {
	engine := newEngine(t)
	runScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)
	result := runScheme(t, engine,
		`(pair? (go-callgraph s 'static))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoCallgraph_SessionCachesPerAlgorithm(t *testing.T) {
	engine := newEngine(t)
	runScheme(t, engine, `(define s (go-load "github.com/aalpar/wile-goast/goast"))`)
	// Two calls with same algorithm should not error.
	runScheme(t, engine, `(go-callgraph s 'static)`)
	runScheme(t, engine, `(go-callgraph s 'static)`)
	// Different algorithm also works.
	runScheme(t, engine, `(go-callgraph s 'cha)`)
}
```

Session path: calls `session.SSAAllPackages()` (builds all packages for
cross-package edges). Uses `session.CachedValue("callgraph:" + algo)` to
cache per algorithm. Calls `dispatchCallgraph(prog, algo)` (existing function
at `prim_callgraph.go:129`) and `session.SetCachedValue(...)`.

**Commit:** `feat: go-callgraph accepts GoSession`

---

### Task 10: Dual-accept `go-analyze`

**Files:** `goastlint/prim_lint.go`, `goastlint/prim_lint_test.go`

**Test:**

```go
func TestGoAnalyze_WithLintSession(t *testing.T) {
	engine := newEngine(t)
	runScheme(t, engine,
		`(define s (go-load "github.com/aalpar/wile-goast/goast" 'lint))`)
	result := runScheme(t, engine,
		`(list? (go-analyze s "assign"))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestGoAnalyze_WithNonLintSession_FallsBack(t *testing.T) {
	engine := newEngine(t)
	runScheme(t, engine,
		`(define s (go-load "github.com/aalpar/wile-goast/goast"))`)
	result := runScheme(t, engine,
		`(list? (go-analyze s "assign"))`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}
```

Session path: if `session.IsLintMode()`, uses `session.Packages()` directly
with `checker.Analyze`. If not lint mode, falls back to `analyzeFromPattern`
(loads fresh).

**Commit:** `feat: go-analyze accepts GoSession with lint fallback`

---

### Task 11: Update belief DSL

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm`

**Step 1: Modify `make-context` (line 57-65)**

```scheme
(define (make-context target)
  (let ((session (go-load target)))
    (list (cons 'target target)
          (cons 'session session)
          (cons 'pkgs #f)
          (cons 'ssa #f)
          (cons 'ssa-index #f)
          (cons 'field-index #f)
          (cons 'callgraph #f)
          (cons 'interface-cache '())
          (cons 'results '()))))
```

**Step 2: Add accessor (after `ctx-target`)**

```scheme
(define (ctx-session ctx) (ctx-ref ctx 'session))
```

**Step 3: Update `ctx-pkgs` (line 75-79)**

```scheme
(define (ctx-pkgs ctx)
  (or (ctx-ref ctx 'pkgs)
      (let ((pkgs (go-typecheck-package (ctx-session ctx))))
        (ctx-set! ctx 'pkgs pkgs)
        pkgs)))
```

**Step 4: Update `ctx-ssa` (line 81-85)**

```scheme
(define (ctx-ssa ctx)
  (or (ctx-ref ctx 'ssa)
      (let ((ssa (go-ssa-build (ctx-session ctx))))
        (ctx-set! ctx 'ssa ssa)
        ssa)))
```

**Step 5: Update `ctx-callgraph` (line 87-91)**

```scheme
(define (ctx-callgraph ctx)
  (or (ctx-ref ctx 'callgraph)
      (let ((cg (go-callgraph (ctx-session ctx) 'static)))
        (ctx-set! ctx 'callgraph cg)
        cg)))
```

**Step 6: Update `ctx-field-index` (line 93-97)**

```scheme
(define (ctx-field-index ctx)
  (or (ctx-ref ctx 'field-index)
      (let ((idx (go-ssa-field-index (ctx-session ctx))))
        (ctx-set! ctx 'field-index idx)
        idx)))
```

**Step 7: Update `ctx-interface-info` (line 100-107)**

```scheme
(define (ctx-interface-info ctx iface-name)
  (let* ((cache (ctx-ref ctx 'interface-cache))
         (session (ctx-session ctx))
         (key (cons iface-name session))
         (cached (assoc key cache)))
    (if cached (cdr cached)
      (let ((info (go-interface-implementors iface-name session)))
        (ctx-set! ctx 'interface-cache (cons (cons key info) cache))
        info))))
```

**Step 8: Update `ordered` checker (line 549)**

Replace:
```scheme
(cfg (and pkg-path (go-cfg pkg-path fname)))
```
With:
```scheme
(cfg (and pkg-path (go-cfg (ctx-session ctx) fname)))
```

**Step 9: Test**

Build and run an existing belief script:

```bash
make build
./dist/darwin/arm64/wile-goast '(begin
  (import (wile goast belief))
  (define-belief "test"
    (sites (functions-matching (contains-call "Lock")))
    (expect (paired-with "Lock" "Unlock"))
    (threshold 0.90 5))
  (run-beliefs "github.com/aalpar/wile-goast/goast"))'
```

Expected: runs without error.

**Step 10: Commit**

```
git add cmd/wile-goast/lib/wile/goast/belief.scm
git commit -m "feat: belief DSL uses GoSession for shared package loading"
```

---

### Task 12: Update documentation

**Files:**
- Modify: `docs/PRIMITIVES.md`
- Modify: `CLAUDE.md`

**Step 1:** Add `go-load`, `go-list-deps`, `go-session?` to the primitives
table in `CLAUDE.md` under `goast`.

**Step 2:** Add a "Session Management" section to `docs/PRIMITIVES.md` with
full signatures, options, multi-pattern support, and examples.

**Step 3:** For each dual-accept primitive entry, add a note:

> The first argument may be a package pattern string or a GoSession from
> `go-load`. When given a GoSession, reuses the session's loaded state.

**Step 4: Commit**

```
git add docs/PRIMITIVES.md CLAUDE.md
git commit -m "docs: add session primitives and dual-accept signatures"
```

---

### Task 13: Integration test — full session pipeline

**Files:**
- Create: `goast/prim_session_integration_test.go`

This test verifies that a single session feeds all layers without error.
It needs all extensions loaded. Place in `goast/` and load all extensions
in `newEngine`, or place in `cmd/wile-goast/` where all are composed.

```go
func TestSession_FullPipeline(t *testing.T) {
	engine := newAllExtensionsEngine(t)
	pkg := "github.com/aalpar/wile-goast/goast"

	runScheme(t, engine, `(define s (go-load "`+pkg+`"))`)

	t.Run("typecheck", func(t *testing.T) {
		result := runScheme(t, engine, `(pair? (go-typecheck-package s))`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("ssa-build", func(t *testing.T) {
		result := runScheme(t, engine, `(pair? (go-ssa-build s))`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("cfg", func(t *testing.T) {
		result := runScheme(t, engine,
			`(pair? (go-cfg s "PrimGoParseExpr"))`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("callgraph", func(t *testing.T) {
		result := runScheme(t, engine,
			`(pair? (go-callgraph s 'static))`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("session-predicate", func(t *testing.T) {
		result := runScheme(t, engine, `(go-session? s)`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})
}
```

**Commit:** `test: add session full-pipeline integration test`
