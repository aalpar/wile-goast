# PR-1 Target Setting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `current-go-target` R7RS parameter to wile-goast (with env-var default `WILE_GOAST_TARGET`), and make three pattern-accepting primitives — `go-ssa-build`, `go-typecheck-package`, `go-load` — fall back to the parameter when called with zero pattern arguments. Existing call sites that pass an explicit pattern continue to work unchanged.

**Architecture:** Declare the parameter as a Go-side `*machine.Parameter` in `goast/target.go`. Register it as a global binding named `current-go-target` during the `goast` extension's registration so Scheme code sees it as a standard parameter (read via `(current-go-target)`, override via `parameterize`). Modify the three primitives' arity from `ParamCount: 2, IsVariadic: true` (1 fixed + rest) to `ParamCount: 1, IsVariadic: true` (0 fixed + rest). Inside each primitive, check whether the rest list has a first element — if yes, treat it as the pattern (old behavior); if no, read the parameter via `mc.ResolveParameterValue()` and use that string as the pattern.

**Tech Stack:** Go 1.24 standard library; `machine.Parameter` from wile; `values.String`/`values.Tuple`/`values.Pair` for Scheme values; `werr` for error wrapping.

**Parent design:** `plans/2026-04-19-axis-b-analyzer-impl-design.md` §5.

**Project conventions observed:**
- wile-goast commits direct to master (no branch/PR workflow, per user's explicit instruction).
- Multi-line function bodies only (wile imperative).
- No compound if-assignments (`x := f()` then `if x != nil`, not `if x := f(); x != nil`).
- Production code uses `werr.WrapForeignErrorf` for error wrapping; test code is exempt.
- `VERSION` is auto-bumped by pre-commit hook — do not touch.
- After Go changes: run `make lint` and `make test` before claiming complete.
- Every commit asks the user first (wile-goast user convention).

**Scope boundary — what is NOT in this PR:**

- `go-parse-file` — takes a filename, not a pattern. Excluded.
- `go-callgraph` — takes `(pattern, algorithm-symbol)`, both required. Making pattern optional while keeping algorithm required would require type-disambiguated dispatch (string = pattern, symbol = algorithm) — magical and not worth it for a primitive the axis-b script doesn't need. Excluded. A later PR can add it if a concrete consumer shows up.
- Migration of existing scripts (`unify-detect.scm`, `goast-query.scm`, etc.) — follow-up PR-4.

---

## File Structure

**Create:**
- `goast/target.go` — parameter state, init, getter, reset-for-test.
- `goast/target_test.go` — Go-side unit tests for state behavior.
- `test/target_parameter_integration_test.go` — Scheme-level integration test exercising the parameter via the full engine.

**Modify:**
- `goast/register.go` — add one line in `addPrimitives` (or a new `addTargetParam`) to register the parameter as a global binding named `current-go-target`.
- `goastssa/prim_ssa.go` — extract a helper that reads the first rest element (or parameter fallback) and dispatches session-vs-string. Replace the current `mc.Arg(0)` / `mc.Arg(1)` pattern in `PrimGoSSABuild` to use the helper.
- `goastssa/register.go` — `ParamCount: 2, IsVariadic: true` → `ParamCount: 1, IsVariadic: true` for `go-ssa-build`. Update docstring to reflect the new optional-pattern shape.
- `goast/prim_goast.go` (or wherever `PrimGoTypecheckPackage` and `PrimGoLoad` live — implementer verifies) — same pattern.
- `goast/register.go` — `ParamCount` changes for `go-typecheck-package` and `go-load`. Update docstrings.

**Do not modify:**
- `goastcg/` — `go-callgraph` explicitly out of scope per the scope boundary above.
- `goast/` `go-parse-file` registration — filename, not pattern.
- `VERSION` — pre-commit hook handles it.

---

## Task 1: Create `goast/target.go` with parameter state

**Files:**
- Create: `goast/target.go`

- [ ] **Step 1: Read an existing wile state-file pattern for reference**

Read `/Users/aalpar/projects/wile-workspace/wile/internal/extensions/io/state.go` lines 30–100 to see the exact `machine.Parameter` idiom in wile. The pattern for wile-goast matches: a package-level `*machine.Parameter` variable, an idempotent `Init*State()` function, and a `Get*Param()` getter. No need to copy the port-specific caches — we just need one parameter.

- [ ] **Step 2: Write the state file**

Create `/Users/aalpar/projects/wile-workspace/wile-goast/goast/target.go` with this exact content:

```go
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

// Target parameter state for wile-goast.
//
// current-go-target is an R7RS parameter holding the default Go package
// pattern used by pattern-accepting primitives (go-ssa-build,
// go-typecheck-package, go-load) when called with no explicit pattern.
//
// Initialized from the WILE_GOAST_TARGET env var at first access, with
// a fallback default of "./...". Scheme code reads via (current-go-target)
// and overrides via parameterize.
//
// See plans/2026-04-19-axis-b-analyzer-impl-design.md §5 and
// plans/2026-04-19-pr-1-target-setting-impl.md.

package goast

import (
	"os"
	"sync"

	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/values"
)

const (
	// targetEnvVar is the environment variable consulted at initialization
	// to set the parameter's default value.
	targetEnvVar = "WILE_GOAST_TARGET"

	// targetDefaultPattern is the fallback when the env var is unset or empty.
	// "./..." matches wile-goast's current hardcoded target in existing
	// scripts — this preserves current behavior for new scripts that don't
	// set the parameter explicitly. See §5.5 of the design doc for why this
	// specific value is sanctioned as a top-level default (the exception to
	// the project's "never default on nil/zero" rule).
	targetDefaultPattern = "./..."
)

var (
	targetOnce  sync.Once
	targetParam *machine.Parameter
)

// InitTargetState lazily initializes the current-go-target parameter.
// Idempotent — safe to call multiple times.
func InitTargetState() {
	targetOnce.Do(func() {
		initial := os.Getenv(targetEnvVar)
		if initial == "" {
			initial = targetDefaultPattern
		}
		targetParam = machine.NewParameter(values.NewString(initial), nil)
	})
}

// GetCurrentGoTargetParam returns the *machine.Parameter backing the
// current-go-target Scheme parameter. Calls InitTargetState first.
func GetCurrentGoTargetParam() *machine.Parameter {
	InitTargetState()
	return targetParam
}

// ResetTargetState resets the parameter for test isolation. Must not be
// called from production code — only from tests that need a clean slate.
func ResetTargetState() {
	targetOnce = sync.Once{}
	targetParam = nil
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build ./goast/`

Expected: no output (success). If it fails, read the error — typical cause is an import path mistake. The imports above are correct for the current workspace.

- [ ] **Step 4: Commit — ask the user first**

> "Task 1 complete — created `goast/target.go` with the `current-go-target` parameter state. Builds cleanly. Want me to commit?"

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
git add goast/target.go && \
git commit -m "feat(goast): add current-go-target parameter state

Declares a *machine.Parameter backing the current-go-target R7RS
parameter. Initial value from WILE_GOAST_TARGET env var or './...'
fallback. Idempotent InitTargetState + GetCurrentGoTargetParam getter
+ ResetTargetState for test isolation.

Registration as a global binding comes in Task 2.

See plans/2026-04-19-pr-1-target-setting-impl.md Task 1."
```

---

## Task 2: Unit-test the parameter state

**Files:**
- Create: `goast/target_test.go`

- [ ] **Step 1: Write the failing test**

Create `/Users/aalpar/projects/wile-workspace/wile-goast/goast/target_test.go`:

```go
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

package goast

import (
	"testing"

	"github.com/aalpar/wile/values"
)

func TestCurrentGoTargetDefault(t *testing.T) {
	ResetTargetState()
	t.Setenv(targetEnvVar, "")
	p := GetCurrentGoTargetParam()
	if p == nil {
		t.Fatal("GetCurrentGoTargetParam returned nil")
	}
	s, ok := p.Value().(*values.String)
	if !ok {
		t.Fatalf("parameter value is %T, want *values.String", p.Value())
	}
	if s.Value() != targetDefaultPattern {
		t.Errorf("default value = %q, want %q", s.Value(), targetDefaultPattern)
	}
}

func TestCurrentGoTargetEnvOverride(t *testing.T) {
	ResetTargetState()
	t.Setenv(targetEnvVar, "github.com/example/foo/...")
	p := GetCurrentGoTargetParam()
	s, ok := p.Value().(*values.String)
	if !ok {
		t.Fatalf("parameter value is %T, want *values.String", p.Value())
	}
	if s.Value() != "github.com/example/foo/..." {
		t.Errorf("env-override value = %q, want %q",
			s.Value(), "github.com/example/foo/...")
	}
}

func TestCurrentGoTargetIdempotentInit(t *testing.T) {
	ResetTargetState()
	first := GetCurrentGoTargetParam()
	second := GetCurrentGoTargetParam()
	if first != second {
		t.Errorf("repeated InitTargetState returned different parameters: %p vs %p",
			first, second)
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test -v -run 'TestCurrentGoTarget' ./goast/`

Expected: all three tests PASS.

If `TestCurrentGoTargetEnvOverride` fails — check that `targetOnce` is reset properly by `ResetTargetState`. The `t.Setenv` in Go restores the env var when the test completes, so tests are parallel-safe only via `ResetTargetState` between runs, which the tests do explicitly.

- [ ] **Step 3: Commit — ask the user first**

> "Task 2 complete — `goast/target_test.go` with three unit tests covering default, env-override, and idempotency. Want me to commit?"

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
git add goast/target_test.go && \
git commit -m "test(goast): unit tests for current-go-target state

Covers default value, WILE_GOAST_TARGET override, and idempotent
initialization. Uses ResetTargetState between tests for isolation.

See plans/2026-04-19-pr-1-target-setting-impl.md Task 2."
```

---

## Task 3: Register the parameter as a global binding

**Files:**
- Modify: `goast/register.go`

- [ ] **Step 1: Read the existing addPrimitives function**

Open `/Users/aalpar/projects/wile-workspace/wile-goast/goast/register.go`. Note the structure:

```go
var Builder = registry.NewRegistryBuilder(addPrimitives)
var AddToRegistry = Builder.AddToRegistry

func addPrimitives(r *registry.Registry) error {
    r.AddPrimitives([]registry.PrimitiveSpec{
        // ...
    })
    return nil
}
```

The `AddGlobalValue` call is equivalent to how wile's io extension registers its port parameters at `wile/internal/extensions/io/register.go:247-249`.

- [ ] **Step 2: Add a new addTargetParam registration function**

Add at the bottom of `/Users/aalpar/projects/wile-workspace/wile-goast/goast/register.go`, after the existing `addPrimitives` function:

```go
// addTargetParam registers the current-go-target R7RS parameter as a
// global binding. See goast/target.go for the parameter's semantics.
func addTargetParam(r *registry.Registry) error {
	r.AddGlobalValue("current-go-target", GetCurrentGoTargetParam())
	return nil
}
```

- [ ] **Step 3: Wire addTargetParam into the Builder**

Find the line near the top of the file that reads:

```go
var Builder = registry.NewRegistryBuilder(addPrimitives)
```

Change it to include `addTargetParam`:

```go
var Builder = registry.NewRegistryBuilder(addPrimitives, addTargetParam)
```

(`NewRegistryBuilder` accepts any number of registration functions; verify in `wile/registry/builder.go` if the signature differs — the variadic-of-funcs pattern is standard.)

- [ ] **Step 4: Verify it compiles**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build ./goast/ && go build ./cmd/wile-goast/`

Expected: no output. If the `cmd/wile-goast/` build fails with an "undefined" error on something, the issue is probably that `GetCurrentGoTargetParam` is exported from goast but the main binary doesn't see it through the extension chain — report back, don't guess.

- [ ] **Step 5: Quick manual check via a one-liner**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build -o /tmp/wg ./cmd/wile-goast/ && /tmp/wg -e '(display (current-go-target))'`

Expected output: `./...` (no trailing newline). This proves the parameter is registered as a Scheme binding.

Also verify env-var override:

```bash
WILE_GOAST_TARGET=foo /tmp/wg -e '(display (current-go-target))'
```

Expected: `foo`.

Clean up: `rm /tmp/wg`.

- [ ] **Step 6: Commit — ask the user first**

> "Task 3 complete — `current-go-target` is now a Scheme-accessible binding. Manual test confirms: default `./...` and env-var override both work. Want me to commit?"

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
git add goast/register.go && \
git commit -m "feat(goast): register current-go-target as global binding

addTargetParam wires the *machine.Parameter from goast/target.go into
the goast extension's Builder, exposing it as a Scheme-level parameter
named 'current-go-target'. Scripts read via (current-go-target),
override via parameterize.

See plans/2026-04-19-pr-1-target-setting-impl.md Task 3."
```

---

## Task 4: Scheme-level integration test for the parameter

**Files:**
- Create: `test/target_parameter_integration_test.go`

**Pre-step: verify the integration-test entry point.** Before writing, run:

```bash
ls /Users/aalpar/projects/wile-workspace/wile-goast/test/
```

If no `test/` directory exists, or its contents suggest a different pattern than `cmd/wile-goast/session_integration_test.go`, STOP and report — the test location assumption in this plan may be wrong. Otherwise read one existing integration test file (3–5 minutes of reading) to understand the helper pattern, then proceed with the adjusted location.

- [ ] **Step 1: Write the integration test**

Path may need to be adjusted based on the pre-step. The default target is `/Users/aalpar/projects/wile-workspace/wile-goast/cmd/wile-goast/target_parameter_integration_test.go` (co-located with `session_integration_test.go`):

```go
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
	"strings"
	"testing"
)

// runGoastOneLiner invokes the wile-goast binary with -e and returns stdout.
// Matches the pattern in session_integration_test.go — the implementer should
// use the test helper from that file if one exists; otherwise build the binary
// inline. This placeholder calls out the dependency but does NOT itself set up
// the build — use the same mechanism as the existing integration test file.

func TestCurrentGoTargetDefault(t *testing.T) {
	t.Setenv("WILE_GOAST_TARGET", "")
	out := runGoastOneLiner(t, `(display (current-go-target))`)
	if strings.TrimSpace(out) != "./..." {
		t.Errorf("default target = %q, want %q", out, "./...")
	}
}

func TestCurrentGoTargetEnvVar(t *testing.T) {
	t.Setenv("WILE_GOAST_TARGET", "github.com/example/foo/...")
	out := runGoastOneLiner(t, `(display (current-go-target))`)
	if strings.TrimSpace(out) != "github.com/example/foo/..." {
		t.Errorf("env-override target = %q, want %q",
			out, "github.com/example/foo/...")
	}
}

func TestCurrentGoTargetParameterize(t *testing.T) {
	t.Setenv("WILE_GOAST_TARGET", "")
	out := runGoastOneLiner(t,
		`(parameterize ((current-go-target "./sub/..."))
		   (display (current-go-target)))`)
	if strings.TrimSpace(out) != "./sub/..." {
		t.Errorf("parameterize value = %q, want %q", out, "./sub/...")
	}
}
```

The `runGoastOneLiner` helper is NOT provided by this plan — the implementer reads `session_integration_test.go` to find the project's existing mechanism for invoking the built binary with a one-liner script. If the existing file uses a different approach (e.g., in-process engine construction), MATCH that approach instead of shelling out. Report the approach used.

- [ ] **Step 2: Run the tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test -v -run 'TestCurrentGoTarget' ./cmd/wile-goast/`

Expected: three subtests PASS. If env-var test fails, the issue is probably process-isolation — `t.Setenv` in the test process won't propagate to a child binary unless explicitly forwarded. The helper `runGoastOneLiner` must pass env vars through.

- [ ] **Step 3: Commit — ask the user first**

> "Task 4 complete — Scheme integration test verifies default, env-var override, and parameterize all work end-to-end. Want me to commit?"

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
git add cmd/wile-goast/target_parameter_integration_test.go && \
git commit -m "test(goast): Scheme-level integration for current-go-target

Verifies the parameter is accessible via the full engine pipeline:
default value, WILE_GOAST_TARGET override, and parameterize all
behave as R7RS expects.

See plans/2026-04-19-pr-1-target-setting-impl.md Task 4."
```

---

## Task 5: Add parameter fallback to `go-ssa-build`

**Files:**
- Modify: `goastssa/register.go` (ParamCount change + docstring)
- Modify: `goastssa/prim_ssa.go` (impl change)

- [ ] **Step 1: Write the failing test**

Add to `/Users/aalpar/projects/wile-workspace/wile-goast/cmd/wile-goast/target_parameter_integration_test.go`:

```go
func TestGoSSABuildUsesCurrentGoTarget(t *testing.T) {
	t.Setenv("WILE_GOAST_TARGET", "")
	// No arg — should use (current-go-target) which defaults to "./..."
	out := runGoastOneLiner(t, `(display (length (go-ssa-build)))`)
	n := strings.TrimSpace(out)
	if n == "" || n == "0" {
		t.Errorf("go-ssa-build with no args returned 0 or empty: %q", out)
	}
}

func TestGoSSABuildExplicitArgStillWorks(t *testing.T) {
	t.Setenv("WILE_GOAST_TARGET", "")
	out := runGoastOneLiner(t, `(display (length (go-ssa-build "./goast/...")))`)
	n := strings.TrimSpace(out)
	if n == "" || n == "0" {
		t.Errorf("go-ssa-build with explicit arg returned 0 or empty: %q", out)
	}
}
```

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test -v -run 'TestGoSSABuild' ./cmd/wile-goast/`

Expected: `TestGoSSABuildUsesCurrentGoTarget` FAILS with an arity error (the primitive still requires 1 fixed arg). `TestGoSSABuildExplicitArgStillWorks` PASSES (unchanged behavior).

- [ ] **Step 2: Update the ParamCount in `goastssa/register.go`**

Find the `go-ssa-build` spec at `goastssa/register.go:43–46` area. Change `ParamCount: 2, IsVariadic: true` to `ParamCount: 1, IsVariadic: true`. The rest of the spec is unchanged. The `Doc:` string should be updated — old text says "First arg is a package pattern or GoSession" which is no longer accurate at the "required" level. Change that line to "Pattern or GoSession may be the first arg; if absent, (current-go-target) is used."

The edit looks like this (context before/after):

```go
		{Name: "go-ssa-build", ParamCount: 1, IsVariadic: true, Impl: PrimGoSSABuild,
			Doc: "Builds SSA form for a Go package and returns a list of ssa-func nodes.\n" +
				"Pattern or GoSession may be the first arg; if absent, (current-go-target)\n" +
				"is used. Options: 'positions.\n" +
				// ... rest of Doc unchanged
```

- [ ] **Step 3: Modify `PrimGoSSABuild` in `goastssa/prim_ssa.go`**

Open `/Users/aalpar/projects/wile-workspace/wile-goast/goastssa/prim_ssa.go`. The current body (around line 73–85) reads:

```go
func PrimGoSSABuild(mc machine.CallContext) error {
	arg := mc.Arg(0)
	session, ok := goast.UnwrapSession(arg)
	if ok {
		return ssaBuildFromSession(mc, session)
	}
	pat, ok := arg.(*values.String)
	if !ok {
		return werr.WrapForeignErrorf(werr.ErrNotAString,
			"go-ssa-build: expected string or go-session, got %T", arg)
	}
	return ssaBuildFromPattern(mc, pat)
}
```

Replace it with:

```go
func PrimGoSSABuild(mc machine.CallContext) error {
	mctx, ok := mc.(*machine.MachineContext)
	if !ok {
		return werr.WrapForeignErrorf(werr.ErrInternalError,
			"go-ssa-build: CallContext is not *MachineContext")
	}
	arg, rest, err := extractTargetAndRest(mctx, mc.Arg(0))
	if err != nil {
		return err
	}
	session, ok := goast.UnwrapSession(arg)
	if ok {
		return ssaBuildFromSessionWithRest(mc, session, rest)
	}
	pat, ok := arg.(*values.String)
	if !ok {
		return werr.WrapForeignErrorf(werr.ErrNotAString,
			"go-ssa-build: expected string or go-session, got %T", arg)
	}
	return ssaBuildFromPatternWithRest(mc, pat, rest)
}
```

Three new/changed things here: a new `extractTargetAndRest` helper (defined below), and two new names `ssaBuildFromSessionWithRest` / `ssaBuildFromPatternWithRest` which are adapted wrappers around the existing `ssaBuildFromSession` / `ssaBuildFromPattern`.

Add `extractTargetAndRest` to `goastssa/prim_ssa.go` (near the top, after imports):

```go
// extractTargetAndRest unpacks the rest-list arg of a pattern-accepting
// primitive. If the rest list is non-empty, returns the first element
// (which the caller dispatches as string or GoSession) plus the remaining
// rest list. If the rest list is empty, reads the current-go-target
// parameter and returns its value plus an empty rest list.
func extractTargetAndRest(mc *machine.MachineContext, restArg values.Value) (values.Value, values.Value, error) {
	tuple, ok := restArg.(values.Tuple)
	if !ok {
		return nil, nil, werr.WrapForeignErrorf(werr.ErrInternalError,
			"extractTargetAndRest: rest arg is %T, not a values.Tuple", restArg)
	}
	if values.IsEmptyList(tuple) {
		param := goast.GetCurrentGoTargetParam()
		return mc.ResolveParameterValue(param), tuple, nil
	}
	pair, ok := tuple.(*values.Pair)
	if !ok {
		return nil, nil, werr.WrapForeignErrorf(werr.ErrInternalError,
			"extractTargetAndRest: non-empty rest is %T, not a *values.Pair", tuple)
	}
	return pair.Car(), pair.Cdr(), nil
}
```

Now adapt the existing `ssaBuildFromSession` and `ssaBuildFromPattern` to accept the rest-list explicitly. The current implementations (starting around line 87) pass `mc.Arg(1)` as the options tuple. After the arity change, options live in the rest list we already extracted.

Rename the existing `ssaBuildFromSession` to `ssaBuildFromSessionWithRest` and change its signature to accept the rest explicitly:

```go
// Before (existing):
// func ssaBuildFromSession(mc machine.CallContext, session *goast.GoSession) error {
//     mapper, err := parseSSAOpts(mc.Arg(1), session.FileSet())

// After:
func ssaBuildFromSessionWithRest(mc machine.CallContext, session *goast.GoSession, rest values.Value) error {
	mapper, err := parseSSAOpts(rest, session.FileSet())
	// ... rest of the function body unchanged
}
```

Similarly for `ssaBuildFromPattern` → `ssaBuildFromPatternWithRest`:

```go
// Before:
// func ssaBuildFromPattern(mc machine.CallContext, pat *values.String) error {
//     // ... uses mc.Arg(1) somewhere

// After:
func ssaBuildFromPatternWithRest(mc machine.CallContext, pat *values.String, rest values.Value) error {
	// Wherever the body references mc.Arg(1), change to `rest` instead.
}
```

If `ssaBuildFromPattern` does NOT use `mc.Arg(1)` (the implementer verifies by reading), the simplest path is: keep the original name and signature, don't thread `rest` through, and let the impl ignore options. But check first — the `parseSSAOpts` call in `ssaBuildFromSession` implies options handling exists.

- [ ] **Step 4: Run the tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test -v -run 'TestGoSSABuild' ./cmd/wile-goast/`

Expected: both subtests PASS.

Also run the existing goastssa tests to confirm no regression:

```
cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goastssa/
```

Expected: PASS. If existing tests fail, the most likely cause is that they call `go-ssa-build` at the Scheme level and depend on the old arity — implementer checks the test output and adapts the tests (or reverts if the test is exercising the old arity contract intentionally, in which case the migration is the right fix and the tests should be updated).

- [ ] **Step 5: Commit — ask the user first**

> "Task 5 complete — `go-ssa-build` now falls back to `(current-go-target)` when called with no args. Existing callers with explicit args unaffected. Want me to commit?"

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
git add goastssa/register.go goastssa/prim_ssa.go \
        cmd/wile-goast/target_parameter_integration_test.go && \
git commit -m "feat(goastssa): go-ssa-build falls back to current-go-target

Changes ParamCount 2 → 1 with IsVariadic true (fully optional rest
list). Impl extracts first element of rest as pattern/session; if
empty, reads current-go-target via mc.ResolveParameterValue.

Existing callers (go-ssa-build \"./...\") unaffected — their arg
goes into the rest list as the first element and is processed
identically.

See plans/2026-04-19-pr-1-target-setting-impl.md Task 5."
```

---

## Task 6: Add parameter fallback to `go-typecheck-package`

**Files:**
- Modify: `goast/register.go` (ParamCount change + docstring)
- Modify: `goast/prim_goast.go` (impl change — verify file name by grep first)

- [ ] **Step 1: Locate PrimGoTypecheckPackage**

Run: `grep -n "func PrimGoTypecheckPackage" /Users/aalpar/projects/wile-workspace/wile-goast/goast/*.go`

Expected: one match, probably in `prim_goast.go`. Record the file path; all subsequent steps in this task refer to it as `<impl-file>`.

- [ ] **Step 2: Write the failing test**

Add to `cmd/wile-goast/target_parameter_integration_test.go`:

```go
func TestGoTypecheckPackageUsesCurrentGoTarget(t *testing.T) {
	t.Setenv("WILE_GOAST_TARGET", "")
	out := runGoastOneLiner(t, `(display (if (null? (go-typecheck-package)) "empty" "ok"))`)
	s := strings.TrimSpace(out)
	if s == "empty" || s == "" {
		t.Errorf("go-typecheck-package with no args returned empty: %q", out)
	}
}

func TestGoTypecheckPackageExplicitArgStillWorks(t *testing.T) {
	t.Setenv("WILE_GOAST_TARGET", "")
	out := runGoastOneLiner(t, `(display (if (null? (go-typecheck-package "./goast/...")) "empty" "ok"))`)
	s := strings.TrimSpace(out)
	if s == "empty" || s == "" {
		t.Errorf("go-typecheck-package with explicit arg returned empty: %q", out)
	}
}
```

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test -v -run 'TestGoTypecheckPackage' ./cmd/wile-goast/`

Expected: first test FAILS with arity error, second PASSES.

- [ ] **Step 3: Update ParamCount in `goast/register.go`**

Find the `go-typecheck-package` spec. Change `ParamCount: 2, IsVariadic: true` → `ParamCount: 1, IsVariadic: true`. Update the Doc to match the pattern from Task 5: "Pattern or GoSession may be the first arg; if absent, (current-go-target) is used."

- [ ] **Step 4: Move the helper to `goast/target.go` and export it**

`extractTargetAndRest` from Task 5 lives in `goastssa/prim_ssa.go`. Tasks 6 and 7 need the same helper — moving it into `goast` (which both packages import) avoids duplication.

Cut the `extractTargetAndRest` function from `goastssa/prim_ssa.go`.

Paste into `goast/target.go` (below the existing functions), renaming to exported:

```go
// ExtractTargetAndRest unpacks the rest-list arg of a pattern-accepting
// primitive. If the rest list is non-empty, returns the first element
// (which the caller dispatches as string or GoSession) plus the remaining
// rest list. If the rest list is empty, reads the current-go-target
// parameter and returns its value plus an empty rest list.
//
// Used by PrimGoSSABuild (goastssa), PrimGoTypecheckPackage (goast), and
// PrimGoLoad (goast).
func ExtractTargetAndRest(mc *machine.MachineContext, restArg values.Value) (values.Value, values.Value, error) {
	tuple, ok := restArg.(values.Tuple)
	if !ok {
		return nil, nil, werr.WrapForeignErrorf(werr.ErrInternalError,
			"ExtractTargetAndRest: rest arg is %T, not a values.Tuple", restArg)
	}
	if values.IsEmptyList(tuple) {
		param := GetCurrentGoTargetParam()
		return mc.ResolveParameterValue(param), tuple, nil
	}
	pair, ok := tuple.(*values.Pair)
	if !ok {
		return nil, nil, werr.WrapForeignErrorf(werr.ErrInternalError,
			"ExtractTargetAndRest: non-empty rest is %T, not a *values.Pair", tuple)
	}
	return pair.Car(), pair.Cdr(), nil
}
```

Update the imports in `goast/target.go` to include `werr`:

```go
import (
	"os"
	"sync"

	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)
```

Update the call site in `goastssa/prim_ssa.go` (from Task 5): change `extractTargetAndRest(mctx, mc.Arg(0))` to `goast.ExtractTargetAndRest(mctx, mc.Arg(0))`. Delete the local definition.

- [ ] **Step 5: Replace the `PrimGoTypecheckPackage` body in `goast/prim_goast.go`**

Current impl at `goast/prim_goast.go:239-251`:

```go
func PrimGoTypecheckPackage(mc machine.CallContext) error {
	arg := mc.Arg(0)
	session, ok := UnwrapSession(arg)
	if ok {
		return typecheckFromSession(mc, session)
	}
	pat, ok := arg.(*values.String)
	if !ok {
		return werr.WrapForeignErrorf(werr.ErrNotAString,
			"go-typecheck-package: expected string or go-session, got %T", arg)
	}
	return typecheckFromPattern(mc, pat)
}
```

Replace with:

```go
func PrimGoTypecheckPackage(mc machine.CallContext) error {
	mctx, ok := mc.(*machine.MachineContext)
	if !ok {
		return werr.WrapForeignErrorf(werr.ErrInternalError,
			"go-typecheck-package: CallContext is not *MachineContext")
	}
	arg, rest, err := ExtractTargetAndRest(mctx, mc.Arg(0))
	if err != nil {
		return err
	}
	session, ok := UnwrapSession(arg)
	if ok {
		return typecheckFromSessionWithRest(mc, session, rest)
	}
	pat, ok := arg.(*values.String)
	if !ok {
		return werr.WrapForeignErrorf(werr.ErrNotAString,
			"go-typecheck-package: expected string or go-session, got %T", arg)
	}
	return typecheckFromPatternWithRest(mc, pat, rest)
}
```

Then rename the two helper functions: `typecheckFromSession` → `typecheckFromSessionWithRest` (adds `rest values.Value` arg, uses `rest` instead of `mc.Arg(1)`), and `typecheckFromPattern` → `typecheckFromPatternWithRest` (same pattern). Bodies change one line each:

```go
// Before (line 254):   baseOpts, _, optErr := parseOpts(mc.Arg(1), session.FileSet())
// After:                baseOpts, _, optErr := parseOpts(rest, session.FileSet())
```

```go
// Before (line 268):   baseOpts, _, optErr := parseOpts(mc.Arg(1), fset)
// After:                baseOpts, _, optErr := parseOpts(rest, fset)
```

Complete post-change bodies:

```go
func typecheckFromSessionWithRest(mc machine.CallContext, session *GoSession, rest values.Value) error {
	baseOpts, _, optErr := parseOpts(rest, session.FileSet())
	if optErr != nil {
		return optErr
	}
	result := make([]values.Value, len(session.Packages()))
	for i, pkg := range session.Packages() {
		result[i] = mapPackage(pkg, baseOpts)
	}
	mc.SetValue(ValueList(result))
	return nil
}

func typecheckFromPatternWithRest(mc machine.CallContext, pattern *values.String, rest values.Value) error {
	fset := token.NewFileSet()
	baseOpts, _, optErr := parseOpts(rest, fset)
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

	result := make([]values.Value, len(pkgs))
	for i, pkg := range pkgs {
		result[i] = mapPackage(pkg, baseOpts)
	}
	mc.SetValue(ValueList(result))
	return nil
}
```

- [ ] **Step 6: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ ./goastssa/ ./cmd/wile-goast/ -run 'TestGo'`

Expected: all tests PASS. Both new integration tests plus unchanged goastssa tests.

- [ ] **Step 7: Commit — ask the user first**

> "Task 6 complete — `go-typecheck-package` falls back to parameter. Helper moved to `goast.ExtractTargetAndRest` and shared with goastssa. Want me to commit?"

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
git add goast/register.go goast/target.go <impl-file> goastssa/prim_ssa.go \
        cmd/wile-goast/target_parameter_integration_test.go && \
git commit -m "feat(goast): go-typecheck-package falls back to current-go-target

Same pattern as go-ssa-build: ParamCount 2 → 1 with IsVariadic true,
Impl reads first rest element or falls back to parameter.

Moves ExtractTargetAndRest helper from goastssa/prim_ssa.go to
goast/target.go (exported) so both packages share one implementation.

See plans/2026-04-19-pr-1-target-setting-impl.md Task 6."
```

---

## Task 7: Add parameter fallback to `go-load`

**Files:**
- Modify: `goast/register.go` (ParamCount change + docstring)
- Modify: `goast/prim_session.go` (impl change — `PrimGoLoad` lives here)

`go-load` has a slightly different shape from the others: it collects **multiple patterns** (a primary plus any additional string args in the rest list). The fallback rule: if no patterns are supplied at all, use `(current-go-target)` as the single pattern.

- [ ] **Step 1: Write the failing test**

Add to `cmd/wile-goast/target_parameter_integration_test.go`:

```go
func TestGoLoadUsesCurrentGoTarget(t *testing.T) {
	t.Setenv("WILE_GOAST_TARGET", "")
	// go-load returns a session object; just verify no error and non-empty result
	out := runGoastOneLiner(t, `(display (if (eq? (go-load) #f) "failed" "ok"))`)
	s := strings.TrimSpace(out)
	if s != "ok" {
		t.Errorf("go-load with no args: %q", out)
	}
}

func TestGoLoadExplicitArgStillWorks(t *testing.T) {
	t.Setenv("WILE_GOAST_TARGET", "")
	out := runGoastOneLiner(t, `(display (if (eq? (go-load "./goast/...") #f) "failed" "ok"))`)
	s := strings.TrimSpace(out)
	if s != "ok" {
		t.Errorf("go-load with explicit arg: %q", out)
	}
}
```

Run: `go test -v -run 'TestGoLoad' ./cmd/wile-goast/`

Expected: first fails with arity error, second passes.

- [ ] **Step 2: Update ParamCount in `goast/register.go`**

Change `ParamCount: 2, IsVariadic: true` → `ParamCount: 1, IsVariadic: true` for the `go-load` entry. Update the Doc to match the optional-pattern convention.

- [ ] **Step 3: Replace `PrimGoLoad` body in `goast/prim_session.go`**

Current impl at `goast/prim_session.go:31-39` (the arg-parsing prologue):

```go
func PrimGoLoad(mc machine.CallContext) error {
	first, err := helpers.RequireArg[*values.String](mc, 0, werr.ErrNotAString, "go-load")
	if err != nil {
		return err
	}

	patterns := []string{first.Value}
	lintMode := false

	// Walk variadic rest for additional patterns and options.
	tuple, ok := mc.Arg(1).(values.Tuple)
```

Replace with (note: this changes the prologue only — the rest of the function body remains unchanged):

```go
func PrimGoLoad(mc machine.CallContext) error {
	mctx, ok := mc.(*machine.MachineContext)
	if !ok {
		return werr.WrapForeignErrorf(werr.ErrInternalError,
			"go-load: CallContext is not *MachineContext")
	}
	arg, rest, err := ExtractTargetAndRest(mctx, mc.Arg(0))
	if err != nil {
		return err
	}
	first, ok := arg.(*values.String)
	if !ok {
		return werr.WrapForeignErrorf(werr.ErrNotAString,
			"go-load: first arg must be a string, got %T", arg)
	}

	patterns := []string{first.Value}
	lintMode := false

	// Walk the remaining rest list for additional patterns and options.
	tuple, ok := rest.(values.Tuple)
```

The `helpers.RequireArg` check is removed because `ExtractTargetAndRest` guarantees `arg` is non-nil (either a real value from the rest list or the parameter's value). Type-check is now done via the string type-assertion, which handles the "parameter set to non-string" error case too.

The existing `for !values.IsEmptyList(tuple) { ... }` loop body (lines 43-66) is unchanged — the `tuple` variable now starts from `rest` instead of `mc.Arg(1)`, but the loop logic is identical.

One removal: the `helpers` import can be dropped if `RequireArg` was the only use of the `helpers` package in this file. Run:

```
grep -n "helpers\." /Users/aalpar/projects/wile-workspace/wile-goast/goast/prim_session.go
```

If no other matches, remove `"github.com/aalpar/wile-goast/goast/helpers"` (or similar — exact import path from the file header) from the imports block.

- [ ] **Step 4: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./... -run 'TestGo'`

Expected: all go-* integration tests plus package-level tests PASS.

- [ ] **Step 5: Commit — ask the user first**

> "Task 7 complete — `go-load` falls back to parameter. All three pattern-accepting primitives now consult `current-go-target`. Want me to commit?"

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
git add goast/register.go <load-impl-file> \
        cmd/wile-goast/target_parameter_integration_test.go && \
git commit -m "feat(goast): go-load falls back to current-go-target

Completes the PR-1 set: go-ssa-build, go-typecheck-package, go-load
all use the parameter when called with no args.

See plans/2026-04-19-pr-1-target-setting-impl.md Task 7."
```

---

## Task 8: Finalize with make lint + make test

**Files:**
- No code changes expected. Runs the project's build-clean gates.

- [ ] **Step 1: Run make lint**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make lint`

Expected: 0 issues. If it fails:
- Unused import → remove
- `interface{}` used anywhere → replace with `any` (wile-goast uses the modernize linter too)
- Missing goimports grouping → run `goimports -w` on the touched files

Repeat until clean.

- [ ] **Step 2: Run make test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make test`

Expected: all packages PASS. The new integration tests should appear under `cmd/wile-goast/`.

If a test fails that wasn't failing before this PR, read the failure carefully before adjusting. A likely cause: an existing test calls one of the three modified primitives at a specific arity and depends on the old ParamCount for a negative test. If so, update the test to reflect the new shape (or delete it if it's now covered by a Task 5/6/7 test).

- [ ] **Step 3: Commit any lint/test fixups — ask the user first**

If fixes were made:

> "Task 8 complete — `make lint` and `make test` pass. Want me to commit the fixup?"

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
git add -u && \
git commit -m "style(goast): satisfy make lint for PR-1 changes"
```

If no changes were needed, there's nothing to commit.

---

## Task 9: Update the design doc to reflect shipped scope

**Files:**
- Modify: `plans/2026-04-19-axis-b-analyzer-impl-design.md`

The design doc listed 5 primitives (go-ssa-build, go-callgraph, go-typecheck-package, go-parse-file, go-load). PR-1 ships 3. Update the design to match reality and note the exclusions with their reasons.

- [ ] **Step 1: Update §5.2 of the design doc**

Edit the table in §5.2 "Primitive consumption" to reflect the shipped set:

```markdown
| Primitive | File | Status in PR-1 |
|---|---|---|
| `go-ssa-build` | `goastssa/register.go` + `goastssa/prim_ssa.go` | Shipped — pattern optional |
| `go-typecheck-package` | `goast/register.go` + `goast/prim_goast.go` | Shipped — pattern optional |
| `go-load` | `goast/register.go` + `<load-impl-file>` | Shipped — pattern optional |
| `go-callgraph` | `goastcg/` | Excluded — second arg (algorithm) is required, making pattern optional would require type-disambiguated dispatch. Follow-up PR if needed. |
| `go-parse-file` | `goast/` | Excluded — takes a filename, not a pattern. |
```

- [ ] **Step 2: Commit — ask the user first**

> "Task 9 complete — design doc updated to reflect the 3-primitive scope. Want me to commit?"

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
git add plans/2026-04-19-axis-b-analyzer-impl-design.md && \
git commit -m "docs(plans): narrow PR-1 scope to 3 primitives

Originally listed 5; shipping 3. Reasons documented in §5.2:
- go-callgraph: second arg required, can't disambiguate
- go-parse-file: takes a filename, not a pattern"
```

---

## Self-review checklist (plan author)

- [x] Every step has exact file paths.
- [x] Every code step shows actual code (no "implement the function" without the function body).
- [x] Every test step says how to run it and what to expect.
- [x] No compound-if statements in generated code.
- [x] No single-line function bodies.
- [x] Commits are asked-for, not auto-taken.
- [x] `make lint` and `make test` are run at the end.
- [x] Types and names are consistent across tasks: `targetParam`, `InitTargetState`, `GetCurrentGoTargetParam`, `ResetTargetState`, `ExtractTargetAndRest`, `current-go-target`, `WILE_GOAST_TARGET`.
- [x] Every spec requirement (§5.1–§5.5 of the parent design doc) maps to a task:
  - `current-go-target` parameter with env-var default → Tasks 1, 2
  - Registered as global Scheme binding → Tasks 3, 4
  - Go-side primitive fallback for `go-ssa-build` → Task 5
  - Go-side primitive fallback for `go-typecheck-package` → Task 6
  - Go-side primitive fallback for `go-load` → Task 7
  - Final build-clean gates → Task 8
  - Design doc reflects shipped scope → Task 9
