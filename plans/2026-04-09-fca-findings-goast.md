# FCA False Boundary Findings — wile-goast Self-Analysis

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Document false boundary candidates discovered by running the FCA pipeline on wile-goast itself, and implement the one high-value refactoring: extracting the hand-unrolled security-check + package-load pattern.

**Architecture:** Phase 1 FCA (`field-index->context` → `concept-lattice` → `cross-boundary-concepts`) run across all five goast packages. Seven cross-boundary concepts found. One is a clear extraction target (AccessRequest × Config, 9 functions). Two are genuine but low-priority (AST construction helpers). Four are semantic coupling inherent to the domain.

**Tech Stack:** Go, `go/packages`, `wile/security`, `wile/werr`

---

## Findings

### Analysis Parameters

```scheme
(go-load
  "github.com/aalpar/wile-goast/goast"
  "github.com/aalpar/wile-goast/goastssa"
  "github.com/aalpar/wile-goast/goastcfg"
  "github.com/aalpar/wile-goast/goastcg"
  "github.com/aalpar/wile-goast/goastlint")
```

- **Mode:** `'write-only` with `'cross-type-only` pre-filter
- **Input:** 268 functions, 30 cross-type functions after pre-filter
- **Lattice:** 32 concepts, 7 cross-boundary

### Concept 1 — AccessRequest × Config (9 functions) — **EXTRACT**

**Signal:** 9 functions across all 5 packages construct an identical `security.AccessRequest{ResourceProcess, ActionLoad, "go"}` then a `packages.Config{Mode: ..., Context: mc.Context()}`. 7 of the 9 also set `Config.Fset`.

The FCA naturally separated these into two nested concepts: the 9-function concept (base pattern) and a 7-function sub-concept (base + Fset). The two excluded from the inner concept (`PrimGoListDeps`, `implementorsFromPattern`) don't need a FileSet.

**Functions:**

| Function | Package | Sentinel | Checks pkg.Errors | Uses Fset |
|----------|---------|----------|--------------------|-----------|
| `PrimGoLoad` | goast | `errGoLoadError` | yes | yes |
| `PrimGoListDeps` | goast | `errGoLoadError` | no | no |
| `typecheckFromPattern` | goast | `errGoPackageLoadError` | yes | yes |
| `implementorsFromPattern` | goast | `errGoPackageLoadError` | yes | no |
| `ssaBuildFromPattern` | goastssa | `errSSABuildError` | yes | yes |
| `fieldIndexFromPattern` | goastssa | `errSSAFieldIndexError` | yes | yes |
| `cfgFromPattern` | goastcfg | `errCFGBuildError` | yes | yes |
| `callgraphFromPattern` | goastcg | `errCGBuildError` | yes | yes |
| `analyzeFromPattern` | goastlint | `errLintBuildError` | yes | yes |

**10th site excluded by FCA:** `PrimGoParseFile` in `prim_goast.go:81` uses `ResourceFile`/`ActionRead` — a genuinely different security check (file read, not process spawn). Correctly excluded.

**Variations across sites:**
- Error sentinels differ per sub-extension (each defines its own)
- 8 of 9 check `pkg.Errors`; `PrimGoListDeps` skips it (only needs `NeedName|NeedImports`)
- Error format strings vary: some use 1-arg (`"command: %s"`), most use 2-arg (`"command: %s: %s"`)
- All that check `pkg.Errors` join with `"; "` separator

**Verdict:** The irreducible core is: security check → config construction → `packages.Load` → error aggregation. The variations (sentinel, format string, fset) are parameterizable. Extract a helper in `goast/` that the sub-extensions can call.

### Concept 2 — DeclStmt × GenDecl × ValueSpec (2 functions) — **INLINE**

`makeVarDeclInt(name)` is literally `makeVarDeclTyped(name, ast.NewIdent("int"))`. One should call the other.

**Files:** `goast/prim_restructure.go:902-948`

### Concept 3 — BlockStmt × IfStmt (4 functions) — **SEMANTIC**

`makeCtlGuard`, `replaceReturnsInIf`, `restructureBackwardGotos`, `restructureForwardGotos` all construct if-statements with bodies. The coupling is inherent to AST construction — you can't build an `IfStmt` without a `BlockStmt` body. Not a refactoring target.

### Concept 4 — BlockStmt × ForStmt (2 functions) — **SEMANTIC**

Same as Concept 3 but for for-loops. Inherent coupling.

### Concept 5 — BlockStmt × FuncType × GenDecl (2 functions) — **SEMANTIC**

`assignStmtLeadingPos` and `attachDeclComments` both write position fields across AST node types. These are tree walkers that set `token.Pos` values. The coupling is the position-assignment traversal pattern, not a false boundary.

### Concept 6 — BasicLit × Ident (2 functions) — **SEMANTIC**

`assignExprLeadingPos` and `attachSpecComments` — same pattern as Concept 5 but for expression-level position fields.

### Concept 7 — AccessRequest × Config with Fset (7 functions) — **SUBSET OF CONCEPT 1**

Strict subset of Concept 1. The lattice hierarchy correctly shows this as a specialization.

---

## Implementation: Extract `loadPackagesChecked`

### Task 1: Define the helper signature

**Files:**
- Create: `goast/load.go`

**Step 1: Write the failing test**

Add to `goast/prim_goast_test.go` (or a new `goast/load_test.go`):

```go
func TestLoadPackagesChecked(t *testing.T) {
	c := qt.New(t)

	engine := newEngine(t)

	// A valid package should load without error.
	result := eval(t, engine, `
		(define s (go-load "github.com/aalpar/wile-goast/examples/goast-query/testdata/falseboundary"))
		(go-session? s)
	`)
	c.Assert(result.SchemeString(), qt.Equals, "#t")
}
```

This test already passes — it validates the baseline. The real test is that after refactoring, all existing tests still pass.

**Step 2: Create the helper**

In `goast/load.go`:

```go
package goast

import (
	"fmt"
	"strings"

	"go/token"

	"golang.org/x/tools/go/packages"

	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/security"
	"github.com/aalpar/wile/werr"
)

// LoadPackagesChecked performs the security check for Go process spawning,
// constructs a packages.Config, calls packages.Load, and aggregates any
// package-level errors. If fset is nil, Config.Fset is left unset.
//
// The caller provides its own error sentinel and command name for error
// wrapping so that each primitive's error type is preserved.
func LoadPackagesChecked(
	mc machine.CallContext,
	mode packages.LoadMode,
	fset *token.FileSet,
	sentinel error,
	command string,
	patterns ...string,
) ([]*packages.Package, error) {
	err := security.CheckWithAuthorizer(mc.Authorizer(), security.AccessRequest{
		Resource: security.ResourceProcess,
		Action:   security.ActionLoad,
		Target:   "go",
	})
	if err != nil {
		return nil, err
	}

	cfg := &packages.Config{
		Mode:    mode,
		Context: mc.Context(),
	}
	if fset != nil {
		cfg.Fset = fset
	}

	pkgs, loadErr := packages.Load(cfg, patterns...)
	if loadErr != nil {
		return nil, werr.WrapForeignErrorf(sentinel,
			"%s: %s", command, loadErr)
	}

	var errs []string
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			errs = append(errs, e.Error())
		}
	}
	if len(errs) > 0 {
		return nil, werr.WrapForeignErrorf(sentinel,
			"%s: %s", command, strings.Join(errs, "; "))
	}

	return pkgs, nil
}
```

**Step 3: Run all tests**

Run: `make test`
Expected: PASS (helper defined but not yet called)

**Step 4: Commit**

```
feat(goast): add LoadPackagesChecked helper

Extracted from 9 hand-unrolled instances of security check + packages.Load
across all five sub-extensions. Discovered via FCA false boundary analysis.
```

### Task 2: Migrate goast callers

**Files:**
- Modify: `goast/prim_goast.go` (typecheckFromPattern, implementorsFromPattern)
- Modify: `goast/prim_session.go` (PrimGoLoad, PrimGoListDeps)

**Step 1: Migrate `typecheckFromPattern`**

Replace lines 271-311 in `prim_goast.go`. Before:

```go
err := security.CheckWithAuthorizer(mc.Authorizer(), security.AccessRequest{...})
// ... 40 lines of config + load + error checking
```

After:

```go
fset := token.NewFileSet()
baseOpts, _, optErr := parseOpts(mc.Arg(1), fset)
if optErr != nil {
	return optErr
}

pkgs, err := LoadPackagesChecked(mc,
	packages.NeedName|packages.NeedFiles|packages.NeedSyntax|
		packages.NeedTypes|packages.NeedTypesInfo,
	fset, errGoPackageLoadError, "go-typecheck-package",
	pattern.Value)
if err != nil {
	return err
}
```

**Step 2: Migrate `implementorsFromPattern`**

Replace lines 348-376 in `prim_goast.go`. After:

```go
pkgs, err := LoadPackagesChecked(mc,
	packages.NeedName|packages.NeedTypes,
	nil, errGoPackageLoadError, "go-interface-implementors",
	pattern.Value)
if err != nil {
	return err
}
return findImplementors(mc, ifaceName, pkgs)
```

Note: `nil` for fset — this is one of the two sites that don't use a FileSet.

**Step 3: Migrate `PrimGoListDeps`**

This site is special: it does NOT check `pkg.Errors`. The helper always checks them. Two options:

- **Option A:** Accept the behavior change — `PrimGoListDeps` will now fail on packages with type errors. Since it only needs `NeedName|NeedImports`, type errors shouldn't surface. Low risk.
- **Option B:** Add a `skipPkgErrors bool` parameter to the helper.

Prefer Option A. If it breaks a test, revisit.

```go
pkgs, err := LoadPackagesChecked(mc,
	packages.NeedName|packages.NeedImports,
	nil, errGoLoadError, "go-list-deps",
	patterns...)
if err != nil {
	return err
}
```

**Step 4: Migrate `PrimGoLoad`**

Most complex site — has variadic pattern parsing, optional `'lint` flag, and constructs a `GoSession`. Only the security check + config + load + error check portion is extracted. The variadic parsing and session construction remain.

```go
// ... variadic parsing stays unchanged ...

fset := token.NewFileSet()
mode := packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo |
	packages.NeedImports | packages.NeedDeps
if lintMode {
	mode = packages.LoadAllSyntax
}

pkgs, err := LoadPackagesChecked(mc, mode, fset,
	errGoLoadError, "go-load", patterns...)
if err != nil {
	return err
}

// ... GoSession construction stays unchanged ...
```

**Step 5: Run tests**

Run: `make test`
Expected: PASS

**Step 6: Commit**

```
refactor(goast): migrate 4 callers to LoadPackagesChecked
```

### Task 3: Migrate goastssa callers

**Files:**
- Modify: `goastssa/prim_ssa.go` (ssaBuildFromPattern, fieldIndexFromPattern)

**Step 1: Migrate `ssaBuildFromPattern`**

```go
pkgs, err := goast.LoadPackagesChecked(mc,
	packages.NeedName|packages.NeedFiles|packages.NeedSyntax|
		packages.NeedTypes|packages.NeedTypesInfo|
		packages.NeedImports|packages.NeedDeps,
	fset, errSSABuildError, "go-ssa-build",
	pattern.Value)
if err != nil {
	return err
}
```

**Step 2: Migrate `fieldIndexFromPattern`**

```go
pkgs, err := goast.LoadPackagesChecked(mc,
	packages.NeedName|packages.NeedFiles|packages.NeedSyntax|
		packages.NeedTypes|packages.NeedTypesInfo|
		packages.NeedImports|packages.NeedDeps,
	fset, errSSAFieldIndexError, "go-ssa-field-index",
	pattern.Value)
if err != nil {
	return err
}
```

**Step 3: Remove unused `security` import from `goastssa/prim_ssa.go`**

**Step 4: Run tests**

Run: `make test`
Expected: PASS

**Step 5: Commit**

```
refactor(goastssa): migrate to goast.LoadPackagesChecked
```

### Task 4: Migrate goastcfg, goastcg, goastlint callers

**Files:**
- Modify: `goastcfg/prim_cfg.go` (cfgFromPattern)
- Modify: `goastcg/prim_callgraph.go` (callgraphFromPattern)
- Modify: `goastlint/prim_lint.go` (analyzeFromPattern)

Same pattern as Task 3. One function per file.

**Step 1: Migrate all three**

Each follows the same substitution. Example for `cfgFromPattern`:

```go
pkgs, err := goast.LoadPackagesChecked(mc,
	packages.NeedName|packages.NeedFiles|packages.NeedSyntax|
		packages.NeedTypes|packages.NeedTypesInfo|
		packages.NeedImports|packages.NeedDeps,
	fset, errCFGBuildError, "go-cfg",
	pattern.Value)
if err != nil {
	return err
}
```

**Step 2: Remove unused `security` imports from all three files**

**Step 3: Run tests**

Run: `make test`
Expected: PASS

**Step 4: Commit**

```
refactor(goast{cfg,cg,lint}): migrate to goast.LoadPackagesChecked
```

### Task 5: Inline makeVarDeclInt

**Files:**
- Modify: `goast/prim_restructure.go`

**Step 1: Find callers of makeVarDeclInt**

Run: `grep -n 'makeVarDeclInt' goast/prim_restructure.go`

**Step 2: Replace each call with `makeVarDeclTyped(name, ast.NewIdent("int"))`**

**Step 3: Delete `makeVarDeclInt` (lines 902-914)**

**Step 4: Run tests**

Run: `make test`
Expected: PASS

**Step 5: Commit**

```
refactor(goast): inline makeVarDeclInt into makeVarDeclTyped
```

### Task 6: Verify with FCA re-run

**Step 1: Rebuild**

Run: `go install ./cmd/wile-goast/`

**Step 2: Re-run FCA analysis**

```scheme
(import (wile goast fca))
(define s (go-load
  "github.com/aalpar/wile-goast/goast"
  "github.com/aalpar/wile-goast/goastssa"
  "github.com/aalpar/wile-goast/goastcfg"
  "github.com/aalpar/wile-goast/goastcg"
  "github.com/aalpar/wile-goast/goastlint"))
(define idx (go-ssa-field-index s))
(define ctx (field-index->context idx 'write-only 'cross-type-only))
(define lat (concept-lattice ctx))
(define xb (cross-boundary-concepts lat))
(display (length xb))
```

**Expected:** The AccessRequest × Config concepts should be gone (only `LoadPackagesChecked` touches both now). The DeclStmt × GenDecl × ValueSpec concept should also be gone (`makeVarDeclInt` inlined). Remaining concepts should be the 4 semantic ones (BlockStmt × IfStmt/ForStmt, position assignment).

**Step 3: Commit verification results as an update to this plan file**

```
docs(plans): update FCA findings with post-refactoring verification
```

### Task 7: Full CI

Run: `make ci`
Expected: PASS (lint + build + test + covercheck + verify-mod)
