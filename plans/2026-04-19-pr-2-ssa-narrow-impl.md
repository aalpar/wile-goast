# PR-2 `go-ssa-narrow` Primitive Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `go-ssa-narrow` — a new wile-goast primitive that, given a reference to an SSA function and a value name within it, backward-walks the def-use chain to produce the set of concrete Go types that can flow into that value, plus confidence and reasons.

**Architecture:** A new opaque wrapper `SSAFunctionRef` (values.OpaqueValue) embeds `*ssa.Function` inside the `(ssa-func ...)` tagged alist emitted by `go-ssa-build`. `go-ssa-narrow` takes `(ssa-func-alist, value-name)`, unwraps the ref, finds the named value, and runs a flow-sensitive def-use walker with inter-procedural recursion, cycle detection, and reason tags for widened paths.

**Tech Stack:** `golang.org/x/tools/go/ssa` (already vendored by wile-goast); wile `*machine.Parameter` pattern (PR-1); `goast.Node` + `goast.Field` + `goast.Str` + `goast.Sym` for result construction.

**Parent design:** `plans/2026-04-19-axis-b-analyzer-impl-design.md` §6.

**Project conventions observed:**
- wile-goast is commit-direct-to-master (no branch workflow).
- Multi-line function bodies only. No compound if-assignments.
- Production errors wrap with `werr.WrapForeignErrorf(sentinel, ...)`.
- Tests use quicktest (`qt`).
- `make lint` + `make test` must pass before PR is complete.
- Every commit asks the user first.
- User WIP files (`BIBLIOGRAPHY.md`, `docs/THESIS.md`, `plans/CLAUDE.md`, `plans/2026-04-19-llm-concept-filter-design.md`) are not touched.

---

## File Structure

**Create:**

- `goastssa/narrow.go` — the narrowing algorithm (`narrow(ssa.Value) *narrowResult`, `narrowWalk`, `visitedSet`, reason tags).
- `goastssa/prim_narrow.go` — the Scheme-level primitive `PrimGoSSANarrow` + `buildNarrowResult` helper.
- `goastssa/narrow_test.go` — Go-side unit tests using fixture files.
- `goastssa/testdata/narrow/` — directory of fixture Go files (one per narrowing case).
- `cmd/wile-goast/narrow_integration_test.go` — Scheme-level end-to-end tests.

**Modify:**

- `goastssa/ref.go` (new file — lives alongside mapper/prim_ssa) — adds `SSAFunctionRef` opaque type + wrap/unwrap helpers + `findValueByName`.
- `goastssa/mapper.go` — `mapFunction` adds one field `(ref <opaque>)` embedding `SSAFunctionRef`.
- `goastssa/register.go` — register `go-ssa-narrow`.

**Do not modify:**

- Existing tests (they ignore the new `ref` field — tagged alists are extensible).
- Anything outside `goastssa/` and `cmd/wile-goast/`.

---

## Task 1: `SSAFunctionRef` opaque type

**Files:**
- Create: `goastssa/ref.go`

- [ ] **Step 1: Write the failing unit test**

Create `/Users/aalpar/projects/wile-workspace/wile-goast/goastssa/ref_test.go`:

```go
// Copyright 2026 Aaron Alpar
//
// Licensed under the Apache License, Version 2.0 (the "License");
// ... (standard header — copy from existing file in the same dir)

package goastssa

import (
	"testing"

	"golang.org/x/tools/go/ssa"

	qt "github.com/frankban/quicktest"
)

func TestWrapUnwrapSSAFunctionRef(t *testing.T) {
	c := qt.New(t)
	fn := &ssa.Function{} // a zero-value pointer is enough for wrap/unwrap identity
	wrapped := WrapSSAFunctionRef(fn)
	c.Assert(wrapped, qt.IsNotNil)

	got, ok := UnwrapSSAFunctionRef(wrapped)
	c.Assert(ok, qt.IsTrue)
	c.Assert(got, qt.Equals, fn)
}

func TestUnwrapSSAFunctionRefWrongTag(t *testing.T) {
	c := qt.New(t)
	_, ok := UnwrapSSAFunctionRef(nil)
	c.Assert(ok, qt.IsFalse)
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test -run TestWrapUnwrapSSAFunctionRef ./goastssa/`

Expected: compile error (undefined WrapSSAFunctionRef / UnwrapSSAFunctionRef).

- [ ] **Step 3: Implement**

Create `/Users/aalpar/projects/wile-workspace/wile-goast/goastssa/ref.go`:

```go
// Copyright 2026 Aaron Alpar
// ... (standard header)

// SSAFunctionRef opaque wrapper for *ssa.Function.
//
// Embedded inside (ssa-func ...) tagged alists by the mapper, so Scheme code
// can hand an SSA function back to Go-side primitives (go-ssa-narrow) without
// name-lookup overhead or ambiguity.

package goastssa

import (
	"golang.org/x/tools/go/ssa"

	"github.com/aalpar/wile/values"
)

const ssaFunctionRefTag = "ssa-function-ref"

// WrapSSAFunctionRef wraps an *ssa.Function as an OpaqueValue for Scheme.
func WrapSSAFunctionRef(fn *ssa.Function) *values.OpaqueValue {
	return values.NewOpaqueValue(ssaFunctionRefTag, fn)
}

// UnwrapSSAFunctionRef extracts an *ssa.Function from a values.Value.
// Returns nil, false if v is not an ssa-function-ref OpaqueValue.
func UnwrapSSAFunctionRef(v values.Value) (*ssa.Function, bool) {
	o, ok := v.(*values.OpaqueValue)
	if !ok || o.OpaqueTag() != ssaFunctionRefTag {
		return nil, false
	}
	fn, ok := o.Unwrap().(*ssa.Function)
	return fn, ok
}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test -run TestWrapUnwrapSSAFunctionRef ./goastssa/`

Expected: PASS.

- [ ] **Step 5: Commit — ask the user first**

> "Task 1 complete — SSAFunctionRef opaque wrapper + tests. Want me to commit?"

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
git add -f goastssa/ref.go goastssa/ref_test.go && \
git commit -m "feat(goastssa): add SSAFunctionRef opaque wrapper

Opaque values.OpaqueValue wrapper for *ssa.Function, tagged with
'ssa-function-ref'. Embedded in (ssa-func ...) alists by the mapper
(next task) so Scheme code can pass SSA functions to Go primitives
without name-lookup.

See plans/2026-04-19-pr-2-ssa-narrow-impl.md Task 1."
```

---

## Task 2: Embed the ref in `(ssa-func ...)` alists

**Files:**
- Modify: `goastssa/mapper.go` (add one field at `mapFunction`'s end)

- [ ] **Step 1: Add a failing test**

Append to `goastssa/mapper_test.go` (read existing file first to match conventions):

```go
func TestMapFunctionIncludesRefField(t *testing.T) {
	c := qt.New(t)
	// Build a minimal SSA package with one function.
	dir := t.TempDir()
	writeTestPackage(t, dir, `
package testpkg
func Foo() int { return 42 }
`)
	fn := loadSSAFunction(t, dir, "Foo")

	m := &ssaMapper{fset: token.NewFileSet()}
	node := m.mapFunction(fn)

	refField := findField(node, "ref")
	c.Assert(refField, qt.IsNotNil)
	got, ok := UnwrapSSAFunctionRef(refField)
	c.Assert(ok, qt.IsTrue)
	c.Assert(got, qt.Equals, fn)
}
```

The helpers `writeTestPackage`, `loadSSAFunction`, `findField` are common to existing mapper tests — check they exist in `mapper_test.go`. If `findField` doesn't exist, write it inline:

```go
// findField looks up a field by key in a tagged node's value list.
// Returns nil if not found.
func findField(node values.Value, key string) values.Value {
	// A tagged alist is (tag (field k1 v1) (field k2 v2) ...)
	// Iterate and match on the 'field' keyword.
	// This is fiddly; if a similar helper already exists in goast utils,
	// use it instead. If not, here's an inline version:
	p, ok := node.(*values.Pair)
	if !ok {
		return nil
	}
	// Skip the tag (car), walk the cdr.
	tail, ok := p.Cdr().(values.Tuple)
	if !ok {
		return nil
	}
	for !values.IsEmptyList(tail) {
		pr, ok := tail.(*values.Pair)
		if !ok {
			break
		}
		fp, ok := pr.Car().(*values.Pair)
		if ok {
			// (field "key" value)
			sym, sok := fp.Car().(*values.Symbol)
			if sok && sym.Key == "field" {
				next, ok := fp.Cdr().(*values.Pair)
				if ok {
					k, ok := next.Car().(*values.String)
					if ok && k.Value == key {
						vnext, ok := next.Cdr().(*values.Pair)
						if ok {
							return vnext.Car()
						}
					}
				}
			}
		}
		tail, ok = pr.Cdr().(values.Tuple)
		if !ok {
			break
		}
	}
	return nil
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test -run TestMapFunctionIncludesRefField ./goastssa/`

Expected: FAIL (ref field not found).

- [ ] **Step 3: Add the field to `mapFunction`**

Read `goastssa/mapper.go` around lines 55–65 (the fields-assembly block for `mapFunction`). Before the existing `if fn.Pkg != nil { ... }` block, append a `ref` field:

```go
fields := []values.Value{
	goast.Field("name", goast.Str(fn.String())),
	goast.Field("signature", goast.Str(fn.Signature.String())),
	goast.Field("params", goast.ValueList(params)),
	goast.Field("free-vars", goast.ValueList(freeVars)),
	goast.Field("blocks", goast.ValueList(blocks)),
	goast.Field("ref", WrapSSAFunctionRef(fn)),  // NEW
}
if fn.Pkg != nil {
	fields = append(fields, goast.Field("pkg", goast.Str(fn.Pkg.Pkg.Path())))
}
return goast.Node("ssa-func", fields...)
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goastssa/`

Expected: PASS (new test green; existing mapper tests still green — they treat the alist as extensible).

If any existing test FAILS because it does exact-equality comparison on the (ssa-func ...) output, that's a legitimate breakage — the test was asserting a closed schema. Report and stop; we'd need to update the test to ignore the new field, or mark the plan a design-level break.

- [ ] **Step 5: Commit — ask the user first**

> "Task 2 complete — `(ssa-func ...)` alists now embed an opaque ref. Existing tests still green. Want me to commit?"

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast && \
git add -f goastssa/mapper.go goastssa/mapper_test.go && \
git commit -m "feat(goastssa): embed SSAFunctionRef in (ssa-func ...) alists

mapFunction appends a 'ref' field wrapping *ssa.Function as
OpaqueValue. Scheme code passes it to go-ssa-narrow (next tasks)
without needing to re-resolve the function by name.

Existing tests unaffected — tagged alists are extensible; consumers
access fields by key via findField/nf/etc.

See plans/2026-04-19-pr-2-ssa-narrow-impl.md Task 2."
```

---

## Task 3: `findValueByName` helper

**Files:**
- Modify: `goastssa/ref.go` (add helper + tests in ref_test.go)

- [ ] **Step 1: Write failing test**

Append to `goastssa/ref_test.go`:

```go
func TestFindValueByName(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	writeTestPackage(t, dir, `
package testpkg
func Foo() int {
	x := 42
	return x
}
`)
	fn := loadSSAFunction(t, dir, "Foo")

	// SSA will have a constant value named "t0" or "x" — we test that lookup works by
	// iterating the function's blocks and picking one real value name.
	var sampleName string
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			if v, ok := instr.(ssa.Value); ok && v.Name() != "" {
				sampleName = v.Name()
				break
			}
		}
		if sampleName != "" {
			break
		}
	}
	c.Assert(sampleName, qt.Not(qt.Equals), "")

	v, ok := findValueByName(fn, sampleName)
	c.Assert(ok, qt.IsTrue)
	c.Assert(v.Name(), qt.Equals, sampleName)
}

func TestFindValueByNameMissing(t *testing.T) {
	c := qt.New(t)
	fn := &ssa.Function{}
	_, ok := findValueByName(fn, "nonexistent")
	c.Assert(ok, qt.IsFalse)
}
```

- [ ] **Step 2: Run and confirm failure** (`undefined: findValueByName`)

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test -run TestFindValueByName ./goastssa/`

Expected: compile error.

- [ ] **Step 3: Implement**

Append to `goastssa/ref.go`:

```go
// findValueByName walks fn's blocks and returns the first ssa.Value whose
// Name() matches. Parameters are checked first (they aren't in blocks).
// Returns nil, false if no match.
func findValueByName(fn *ssa.Function, name string) (ssa.Value, bool) {
	if fn == nil {
		return nil, false
	}
	for _, p := range fn.Params {
		if p.Name() == name {
			return p, true
		}
	}
	for _, fv := range fn.FreeVars {
		if fv.Name() == name {
			return fv, true
		}
	}
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			v, ok := instr.(ssa.Value)
			if !ok {
				continue
			}
			if v.Name() == name {
				return v, true
			}
		}
	}
	return nil, false
}
```

- [ ] **Step 4: Run tests**

Expected: PASS for both.

- [ ] **Step 5: Commit — ask user**

Commit message: "feat(goastssa): add findValueByName helper for SSA value lookup".

---

## Task 4: `go-ssa-narrow` primitive skeleton

**Files:**
- Create: `goastssa/prim_narrow.go`
- Modify: `goastssa/register.go` (add registration)

- [ ] **Step 1: Write failing integration test**

Create `/Users/aalpar/projects/wile-workspace/wile-goast/cmd/wile-goast/narrow_integration_test.go`:

```go
// Copyright 2026 Aaron Alpar
// ... (standard header)

package main

import (
	"context"
	"testing"

	"github.com/aalpar/wile-goast/testutil"

	qt "github.com/frankban/quicktest"
)

func TestGoSSANarrowPrimitiveRegistered(t *testing.T) {
	c := qt.New(t)
	engine := buildEngine(context.Background())
	defer func() { _ = engine.Close() }()

	// Just verify the primitive is callable and returns a narrow-result
	// tagged alist. At this stage, behavior is a stub.
	result := testutil.RunScheme(t, engine,
		`(define s (go-load "./goastssa/..."))
		 (define funcs (go-ssa-build s))
		 (define f (car funcs))
		 (define name (car (cdr (assoc 'params (cdr f)))))
		 (go-ssa-narrow f "nonexistent")`)

	// Stub always returns a well-shaped narrow-result; refine in later tasks.
	c.Assert(result, qt.IsNotNil)
}
```

This test is loose on purpose — Task 4 just wires the primitive. Later tasks assert narrowed types.

- [ ] **Step 2: Create `prim_narrow.go`**

```go
// Copyright 2026 Aaron Alpar
// ... (standard header)

// go-ssa-narrow primitive: flow-sensitive SSA narrowing. See
// plans/2026-04-19-axis-b-analyzer-impl-design.md §6.

package goastssa

import (
	"github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

var errSSANarrowError = werr.NewStaticError("ssa narrow error")

// PrimGoSSANarrow implements (go-ssa-narrow ssa-func value-name).
// Returns (narrow-result (types (...)) (confidence narrow|widened|no-paths) (reasons (...))).
func PrimGoSSANarrow(mc machine.CallContext) error {
	// Arg 0: (ssa-func ...) tagged alist — extract the ref field.
	funcArg := mc.Arg(0)
	ref := findFieldInNode(funcArg, "ref")
	if ref == nil {
		return werr.WrapForeignErrorf(errSSANarrowError,
			"go-ssa-narrow: first arg is not an ssa-func alist (no 'ref' field)")
	}
	fn, ok := UnwrapSSAFunctionRef(ref)
	if !ok {
		return werr.WrapForeignErrorf(errSSANarrowError,
			"go-ssa-narrow: ref field is not an ssa-function-ref")
	}

	// Arg 1: value name (string).
	nameArg, ok := mc.Arg(1).(*values.String)
	if !ok {
		return werr.WrapForeignErrorf(werr.ErrNotAString,
			"go-ssa-narrow: second arg must be a string, got %T", mc.Arg(1))
	}

	v, ok := findValueByName(fn, nameArg.Value)
	if !ok {
		// No such value in the function — surface as confidence=no-paths.
		mc.SetValue(buildNarrowResult(nil, "no-paths", []string{"value-not-found"}))
		return nil
	}

	// Stub: return a well-shaped result with no information, confidence=no-paths.
	// Task 5+ replaces this with the real narrowing algorithm.
	_ = v
	mc.SetValue(buildNarrowResult(nil, "no-paths", nil))
	return nil
}

// buildNarrowResult constructs the Scheme-visible narrow-result alist.
func buildNarrowResult(typeNames []string, confidence string, reasons []string) values.Value {
	types := make([]values.Value, len(typeNames))
	for i, t := range typeNames {
		types[i] = goast.Str(t)
	}
	reasonVals := make([]values.Value, len(reasons))
	for i, r := range reasons {
		reasonVals[i] = goast.Sym(r)
	}
	return goast.Node("narrow-result",
		goast.Field("types", goast.ValueList(types)),
		goast.Field("confidence", goast.Sym(confidence)),
		goast.Field("reasons", goast.ValueList(reasonVals)),
	)
}

// findFieldInNode is a local wrapper around the mapper's findField helper
// (if exported); otherwise inline the same logic. TODO during implementation:
// consolidate with whatever is exported from goast/ utils.
func findFieldInNode(node values.Value, key string) values.Value {
	// Use the same idiom as ref_test.go findField helper.
	// Inlined here; if goast exports a similar accessor, prefer that.
	// ... (body identical to findField in Task 2's test helper)
}
```

**If `goast.Sym` doesn't exist**, check whether the idiom is `values.NewSymbol("name")` — match whatever the mapper uses (grep for `goast\.Sym` or `NewSymbol` in `goastssa/mapper.go`).

**Implementation note:** `findFieldInNode` is duplicated across ref_test.go and prim_narrow.go. Factor it out into `goastssa/ref.go` as `FindFieldInNode` (exported since it may be reused). Update Task 2's test helper to call this exported function.

- [ ] **Step 3: Register the primitive**

In `goastssa/register.go`, in `addPrimitives`, after the `go-ssa-build` entry (around lines 45-65), add:

```go
{Name: "go-ssa-narrow", ParamCount: 2, Impl: PrimGoSSANarrow,
	Doc: "Narrows an SSA value to its set of concrete producing types.\n" +
		"First arg: an (ssa-func ...) alist from go-ssa-build.\n" +
		"Second arg: a value name (string) — matches ssa.Value.Name().\n" +
		"Returns (narrow-result (types (...)) (confidence narrow|widened|no-paths) (reasons (symbol ...))).\n" +
		"Confidence 'widened' means at least one path hit an untyped boundary; reasons\n" +
		"enumerates which: parameter, global-load, field-load, nil-constant, cycle,\n" +
		"interface-method-dispatch.\n",
	ParamNames: []string{"ssa-func", "value-name"},
	Category:   "goast-ssa",
	ReturnType: values.TypeAny,
},
```

`ParamCount: 2, IsVariadic: false` — two required args, no options.

- [ ] **Step 4: Run the integration test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test -run TestGoSSANarrowPrimitive ./cmd/wile-goast/`

Expected: PASS (stub returns a well-shaped result).

- [ ] **Step 5: Commit — ask user**

Commit message: "feat(goastssa): add go-ssa-narrow primitive skeleton (stub implementation)".

---

## Task 5: Handle `Alloc` and composite-literal cases

**Files:**
- Create: `goastssa/narrow.go`
- Create: `goastssa/testdata/narrow/narrow_direct.go`
- Modify: `goastssa/prim_narrow.go` (dispatch to narrow logic)
- Modify: `goastssa/narrow_test.go` (new file for fixture-based tests)

Full algorithm goes in `goastssa/narrow.go`; this task implements only the Alloc + composite-literal case.

- [ ] **Step 1: Write the fixture**

Create `/Users/aalpar/projects/wile-workspace/wile-goast/goastssa/testdata/narrow/narrow_direct.go`:

```go
package narrow

// Foo is the canonical direct-concrete-return case. The body allocates
// a *Bar and returns it — the narrower should resolve the return value
// to exactly {"*.../narrow.Bar"}.
type Bar struct{ N int }

func Foo() *Bar {
	return &Bar{N: 42}
}
```

Plus a go.mod stub — or use the pattern from existing goastssa tests (look at `mapper_test.go`'s `writeTestPackage` helper; the fixture may be inlined there rather than a separate file). Decide after reading mapper_test.go.

- [ ] **Step 2: Write the failing test**

Create `/Users/aalpar/projects/wile-workspace/wile-goast/goastssa/narrow_test.go`:

```go
// Copyright 2026 Aaron Alpar
// ... (standard header)

package goastssa

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestNarrowDirectAlloc(t *testing.T) {
	c := qt.New(t)
	fn := loadFixtureFunction(t, "narrow_direct", "Foo")

	// Find the return instruction's return value.
	retVal := firstReturnValue(t, fn)

	result := narrow(fn, retVal)
	c.Assert(result.Confidence, qt.Equals, "narrow")
	c.Assert(result.Types, qt.DeepEquals, []string{"*github.com/aalpar/wile-goast/goastssa/testdata/narrow.Bar"})
	c.Assert(result.Reasons, qt.HasLen, 0)
}
```

`loadFixtureFunction` and `firstReturnValue` are new helpers — add to narrow_test.go. The fixture path/module setup mirrors existing mapper tests.

- [ ] **Step 3: Run and confirm failure** (undefined `narrow`).

- [ ] **Step 4: Implement narrow algorithm + Alloc case**

Create `/Users/aalpar/projects/wile-workspace/wile-goast/goastssa/narrow.go`:

```go
// Copyright 2026 Aaron Alpar
// ... (standard header)

// Flow-sensitive SSA narrowing: backward-walk the def-use chain of an
// SSA value to determine the set of concrete types that can flow into it.
//
// MVP coverage (this task and subsequent): Alloc, Call (concrete return),
// Call (interface return, inter-procedural), Phi, TypeAssert, Extract,
// cycle detection. Parameters, global loads, field loads, nil constants,
// and interface-method-dispatch widen with reason tags.
//
// See plans/2026-04-19-axis-b-analyzer-impl-design.md §6.

package goastssa

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// narrowResult is the Go-side narrowing output. Converted to Scheme via buildNarrowResult.
type narrowResult struct {
	Types      []string // fully-qualified Go type strings
	Confidence string   // "narrow" | "widened" | "no-paths"
	Reasons    []string // reason tags, empty unless widened
}

// narrow is the public entry point. Wraps narrowWalk with a fresh visited set.
func narrow(fn *ssa.Function, v ssa.Value) *narrowResult {
	visited := make(map[ssa.Value]bool)
	return narrowWalk(fn, v, visited)
}

// narrowWalk performs the backward SSA traversal.
func narrowWalk(fn *ssa.Function, v ssa.Value, visited map[ssa.Value]bool) *narrowResult {
	if visited[v] {
		return &narrowResult{Confidence: "widened", Reasons: []string{"cycle"}}
	}
	visited[v] = true

	switch x := v.(type) {
	case *ssa.Alloc:
		// Alloc produces *T; record it.
		return &narrowResult{
			Types:      []string{types.TypeString(x.Type(), nil)},
			Confidence: "narrow",
		}
	default:
		// Unhandled — widen with parameter as fallback reason for now.
		// Subsequent tasks add Phi, Call, TypeAssert, Extract, interface-method.
		return &narrowResult{Confidence: "widened", Reasons: []string{"parameter"}}
	}
}
```

- [ ] **Step 5: Wire narrow into PrimGoSSANarrow**

Update `goastssa/prim_narrow.go`'s stub return to call `narrow`:

```go
// ... after findValueByName:
result := narrow(fn, v)
mc.SetValue(buildNarrowResult(result.Types, result.Confidence, result.Reasons))
return nil
```

- [ ] **Step 6: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test -run TestNarrowDirectAlloc ./goastssa/`

Expected: PASS.

- [ ] **Step 7: Commit — ask user**

Commit: "feat(goastssa): implement go-ssa-narrow for Alloc case".

---

## Task 6: Handle `Call` with concrete return type

Add a case for `*ssa.Call` where the callee returns a concrete (non-interface) type. Fixture: `narrow_call_concrete.go`:

```go
package narrow

type Baz struct{ X int }

func makeBaz() Baz { return Baz{X: 1} }

func CallerConcrete() Baz {
	return makeBaz()
}
```

Test: `TestNarrowCallConcreteReturn` — narrow the return of `CallerConcrete`. Expected types: `{"github.com/.../narrow.Baz"}`, confidence narrow.

Implementation: in `narrow.go`, add a `case *ssa.Call` branch:

```go
case *ssa.Call:
	// Inspect the call's return type.
	retType := x.Type()
	if isInterface(retType) {
		// Handled by Task 7 — for now widen.
		return &narrowResult{Confidence: "widened", Reasons: []string{"interface-method-dispatch"}}
	}
	return &narrowResult{
		Types:      []string{types.TypeString(retType, nil)},
		Confidence: "narrow",
	}

// Helper:
func isInterface(t types.Type) bool {
	_, ok := t.Underlying().(*types.Interface)
	return ok
}
```

- [ ] Steps: write fixture + test → fail → implement case → pass → commit.

---

## Task 7: Handle interface-return `Call` via inter-procedural recursion

Fixture `narrow_call_interface.go`:

```go
package narrow

type I interface{ V() int }
type A struct{}
func (A) V() int { return 1 }

func makeI() I { return A{} }
func CallerInterface() I { return makeI() }
```

Test: narrow `CallerInterface`'s return. Expected: `{"github.com/.../narrow.A"}` (recursing into `makeI`), confidence narrow.

Implementation: extend the `*ssa.Call` case:

```go
case *ssa.Call:
	retType := x.Type()
	if !isInterface(retType) {
		return &narrowResult{
			Types:      []string{types.TypeString(retType, nil)},
			Confidence: "narrow",
		}
	}
	// Interface return — recurse into callee's return paths.
	callee := x.Call.StaticCallee()
	if callee == nil {
		return &narrowResult{Confidence: "widened", Reasons: []string{"interface-method-dispatch"}}
	}
	return narrowFunctionReturns(callee, visited)
```

Add helper `narrowFunctionReturns` that iterates the function's blocks, collects each `*ssa.Return`'s Results[0], narrows each, unions:

```go
// narrowFunctionReturns unions the narrowings of each *ssa.Return instruction
// in fn. Reuses the caller's visited set to detect cross-function cycles.
func narrowFunctionReturns(fn *ssa.Function, visited map[ssa.Value]bool) *narrowResult {
	union := &narrowResult{Confidence: "narrow"}
	found := false
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			ret, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}
			if len(ret.Results) == 0 {
				continue
			}
			inner := narrowWalk(fn, ret.Results[0], visited)
			unionInto(union, inner)
			found = true
		}
	}
	if !found {
		return &narrowResult{Confidence: "no-paths"}
	}
	return union
}

// unionInto merges b into a (in place). Confidence degrades: narrow ∪ widened = widened;
// anything ∪ no-paths leaves the more-informative side.
func unionInto(a, b *narrowResult) {
	for _, t := range b.Types {
		if !contains(a.Types, t) {
			a.Types = append(a.Types, t)
		}
	}
	switch {
	case b.Confidence == "widened":
		a.Confidence = "widened"
		for _, r := range b.Reasons {
			if !contains(a.Reasons, r) {
				a.Reasons = append(a.Reasons, r)
			}
		}
	case b.Confidence == "no-paths" && a.Confidence == "narrow":
		// a stays narrow; no-paths adds no info.
	}
}

func contains(ss []string, t string) bool {
	for _, s := range ss {
		if s == t {
			return true
		}
	}
	return false
}
```

- [ ] Steps: write fixture + test → fail → implement → pass → commit.

---

## Task 8: Handle `Phi`

Fixture `narrow_phi.go`:

```go
package narrow

type X struct{}
type Y struct{}

func PhiCase(b bool) interface{} {
	var r interface{}
	if b {
		r = &X{}
	} else {
		r = &Y{}
	}
	return r
}
```

Test: narrow `PhiCase`'s return. Expected types: `{"*...X", "*...Y"}`, confidence narrow.

Implementation: add `case *ssa.Phi` to `narrowWalk`:

```go
case *ssa.Phi:
	union := &narrowResult{Confidence: "narrow"}
	for _, edge := range x.Edges {
		inner := narrowWalk(fn, edge, visited)
		unionInto(union, inner)
	}
	if len(union.Types) == 0 && union.Confidence == "narrow" {
		return &narrowResult{Confidence: "no-paths"}
	}
	return union
```

- [ ] Steps: fixture + test → fail → implement → pass → commit.

---

## Task 9: Handle `TypeAssert`

Fixture `narrow_assertion.go`:

```go
package narrow

func AssertCase(i interface{}) *int {
	return i.(*int)
}
```

Test: narrow `AssertCase`'s return. Expected types: `{"*int"}`, confidence narrow.

Implementation:

```go
case *ssa.TypeAssert:
	return &narrowResult{
		Types:      []string{types.TypeString(x.AssertedType, nil)},
		Confidence: "narrow",
	}
```

Note: `TypeAssert.AssertedType` is a `types.Type`. For `x.(*Foo)` it's `*Foo`.

- [ ] Steps: fixture + test → fail → implement → pass → commit.

---

## Task 10: Handle `Extract` from tuple

Fixture `narrow_tuple.go`:

```go
package narrow

type T struct{ V int }

func pair() (*T, bool) { return &T{V: 1}, true }

func ExtractCase() *T {
	t, _ := pair()
	return t
}
```

Test: narrow `ExtractCase`'s return. Expected: `{"*...T"}`, confidence narrow.

Implementation: the `*ssa.Extract` case unwraps the tuple position:

```go
case *ssa.Extract:
	// Extract is always from a Call with tuple return. Recurse into the
	// callee's return paths, but take only the Index-th result each time.
	call, ok := x.Tuple.(*ssa.Call)
	if !ok {
		return &narrowResult{Confidence: "widened", Reasons: []string{"unknown-tuple-source"}}
	}
	callee := call.Call.StaticCallee()
	if callee == nil {
		return &narrowResult{Confidence: "widened", Reasons: []string{"interface-method-dispatch"}}
	}
	return narrowFunctionReturnsAt(callee, x.Index, visited)
```

And `narrowFunctionReturnsAt` variant that takes an index:

```go
func narrowFunctionReturnsAt(fn *ssa.Function, idx int, visited map[ssa.Value]bool) *narrowResult {
	union := &narrowResult{Confidence: "narrow"}
	found := false
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			ret, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}
			if idx >= len(ret.Results) {
				continue
			}
			inner := narrowWalk(fn, ret.Results[idx], visited)
			unionInto(union, inner)
			found = true
		}
	}
	if !found {
		return &narrowResult{Confidence: "no-paths"}
	}
	return union
}
```

- [ ] Steps: fixture + test → fail → implement → pass → commit.

---

## Task 11: Handle parameter and field/global load cases with reason tags

Fixtures:

`narrow_parameter.go`:
```go
package narrow
func ParamCase(i interface{}) interface{} { return i }
```

`narrow_field.go`:
```go
package narrow
type Box struct{ V interface{} }
func FieldCase(b *Box) interface{} { return b.V }
```

Tests: narrow the return of each. Expected:
- `ParamCase`: confidence `widened`, reasons `{"parameter"}`, types `{}`.
- `FieldCase`: confidence `widened`, reasons `{"field-load"}`, types `{}`.

Implementation:

```go
case *ssa.Parameter:
	return &narrowResult{Confidence: "widened", Reasons: []string{"parameter"}}

case *ssa.FreeVar:
	return &narrowResult{Confidence: "widened", Reasons: []string{"free-var"}}

case *ssa.UnOp:
	// Dereference of a pointer; recurse on x.X to narrow the source.
	// For simplicity in this MVP, widen with field-load if x.X is a FieldAddr.
	if _, isField := x.X.(*ssa.FieldAddr); isField {
		return &narrowResult{Confidence: "widened", Reasons: []string{"field-load"}}
	}
	return narrowWalk(fn, x.X, visited)

case *ssa.FieldAddr:
	// FieldAddr produces *T from a struct field. Widened because the field's
	// contents at runtime are unknown statically (unless we track stores — deferred).
	return &narrowResult{Confidence: "widened", Reasons: []string{"field-load"}}

case *ssa.Global:
	return &narrowResult{Confidence: "widened", Reasons: []string{"global-load"}}

case *ssa.Const:
	if x.Value == nil {
		return &narrowResult{Confidence: "widened", Reasons: []string{"nil-constant"}}
	}
	return &narrowResult{
		Types:      []string{types.TypeString(x.Type(), nil)},
		Confidence: "narrow",
	}
```

- [ ] Steps: fixtures + tests → fail → implement each case → pass → commit.

May split into multiple commits (one per case) if TDD granularity demands; batching is fine if the cases are small and the fixtures are separate.

---

## Task 12: Cycle detection end-to-end test

Fixture `narrow_cycle.go`:

```go
package narrow

func a(b bool) interface{} {
	if b {
		return c()
	}
	return &struct{}{}
}

func c() interface{} {
	return a(true)
}
```

Test: `narrow` on `a`'s return. Expected: confidence `widened`, reasons includes `"cycle"`, types at least includes `{"*struct{}"}`.

The visited-set mechanism is already in place; this test is a coverage verification. If the test fails, debug the visited-set passing between `narrowFunctionReturns` and `narrowWalk`.

- [ ] Steps: fixture + test → run → commit (no implementation if cycle detection already works; investigate if not).

---

## Task 13: Scheme-level integration tests

**Files:**
- Modify: `cmd/wile-goast/narrow_integration_test.go`

Add end-to-end tests invoking `go-ssa-narrow` via Scheme against the fixture package. Load the fixture via `go-load`, walk `(go-ssa-build ...)` to find a function by name, then call `go-ssa-narrow`.

```go
func TestGoSSANarrowEndToEnd(t *testing.T) {
	c := qt.New(t)
	engine := buildEngine(context.Background())
	defer func() { _ = engine.Close() }()

	result := testutil.RunScheme(t, engine, `
(define s (go-load "./goastssa/testdata/narrow"))
(define funcs (go-ssa-build s))
(define (find-func name funcs)
  (cond ((null? funcs) #f)
        ((string-contains? (nf (car funcs) 'name) name) (car funcs))
        (else (find-func name (cdr funcs)))))
(define foo-func (find-func "Foo" funcs))
(go-ssa-narrow foo-func "t0")
`)

	// Assert the result is a narrow-result alist. Details vary by SSA's
	// chosen value name for the alloc — "t0" may or may not match; refine
	// using the fixture's known structure.
	c.Assert(result, qt.IsNotNil)
}
```

This test is best-effort — SSA value names aren't stable across Go versions. A more robust test uses `go-ssa-build`'s output to find a specific instruction and extract its name programmatically, then passes it to `go-ssa-narrow`.

If the test is flaky, the cleanest fix is to use `go-ssa-build` to walk the function's blocks in Scheme, find a known-shape instruction (e.g., the ssa-return), and extract its operand name — then narrow that.

- [ ] Steps: write test → run → adjust value-name strategy as needed → commit.

---

## Task 14: Run `make lint` + `make test`

- [ ] Run both; fix any lint issues; re-commit if needed.

---

## Task 15: Update design doc §6 to reflect shipped MVP coverage

- [ ] Mark shipped cases (Alloc, Call-concrete, Call-interface, Phi, TypeAssert, Extract, cycle, parameter, field-load, global-load, nil-constant) as Shipped.
- [ ] List deferred cases: call-graph-context parameter narrowing, slice/map element reasoning, type-switch arm narrowing, reflect-based values.
- [ ] Commit with message "docs(plans): reflect PR-2 shipped narrowing coverage in design §6".

---

## Self-review checklist

- [x] Every step has exact file paths.
- [x] Every task has concrete code for the SSA case it implements.
- [x] Test fixtures are specified with full Go source.
- [x] Type/function names consistent across tasks: `narrow`, `narrowWalk`, `narrowResult`, `narrowFunctionReturns`, `narrowFunctionReturnsAt`, `unionInto`, `isInterface`, `findValueByName`, `WrapSSAFunctionRef`, `UnwrapSSAFunctionRef`, `PrimGoSSANarrow`, `buildNarrowResult`, `errSSANarrowError`, `ssaFunctionRefTag`.
- [x] Every design-spec reason tag (parameter, global-load, field-load, nil-constant, cycle, interface-method-dispatch) lands in a specific case in Tasks 6-11.
- [x] `make lint` + `make test` gate is Task 14.
- [x] Design doc update is Task 15.

**Scope note on test fixtures:** The `goastssa/testdata/narrow/` directory is a real Go package that wile-goast's test code loads via `packages.Load` + `ssautil.Packages`. It must be a valid Go module. Exact setup — whether each fixture is its own `.go` file in a single module, or each fixture is its own module — is determined by reading existing test code in `goastssa/mapper_test.go` Task 5's Step 1. Match the existing pattern.
