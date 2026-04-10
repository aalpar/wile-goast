# Function Name Forms in wile-goast

**Goal:** Document every function name form, where each is produced and consumed, and identify which can be eliminated.

**Status:** Findings documented. Reduction plan proposed.

## The Five Forms

Go's analysis toolchain produces five distinct string representations of the same function. wile-goast uses all five across different layers, creating normalization bugs at every layer boundary.

### Form 1: Short name

```
stepLeader
UpdateBoth
```

**Source:** `ast.FuncDecl.Name.Name` (AST), `ssa.Function.Name()` (SSA)

**Used by:**
- AST mapper → `func-decl` node's `name` field (`goast/mapper.go:212`)
- SSA mapper → `ssa-function` node's `name` field (`goastssa/mapper.go:56`)
- CFG lookup → `findFunction` matches via `fn.Name()` (`goastcfg/prim_cfg.go:165`)
- CFG primitive → user-facing argument to `go-cfg` (`goastcfg/prim_cfg.go:178`)
- Belief DSL → site function `name` from AST func-decl (`belief.scm:512`)
- Belief DSL → `ssa-short-name` extracts this from Form 3 (`belief.scm:166`)

**Properties:** Not unique across receiver types. `(*raft).Step` and `(*RawNode).Step` both produce `"Step"`. No package qualification. Shortest form.

### Form 2: Package-qualified short name

```
go.etcd.io/etcd/raft/v3.stepLeader
```

**Source:** Manual concatenation of `pkg + "." + fn.Name()`.

**Used by:**
- Field index → `field-index->context` **previously** built this from `pkg` + `func` fields (`fca.scm:204`). **Fixed:** now uses Form 3 directly.

**Properties:** Unique for top-level functions. Still ambiguous for methods — `(*raft).Step` and `(*RawNode).Step` both become `raft/v3.Step`. This form exists nowhere in Go's toolchain; it was an accidental invention from concatenating two fields.

### Form 3: SSA qualified name

```
go.etcd.io/etcd/raft/v3.stepLeader           (top-level function)
(*go.etcd.io/etcd/raft/v3.raft).stepLeader    (pointer receiver method)
(go.etcd.io/etcd/raft/v3.Config).validate     (value receiver method)
```

**Source:** `ssa.Function.String()` — which calls `ssa.Function.RelString(nil)`.

**Used by:**
- Call graph mapper → `cg-node` `name` field, `cg-edge` `caller`/`callee` fields (`goastcg/mapper.go:65,81,84`)
- Call graph primitives → `go-callgraph-callers`, `go-callgraph-callees` user-facing argument (`goastcg/prim_callgraph.go:193,221`)
- Field index → `ssa-field-summary` `func` field (`goastssa/prim_ssa.go:315`). **Fixed:** was Form 1, now Form 3.
- FCA → `field-index->context` uses this directly as the object name (`fca.scm:202`). **Fixed:** was Form 2.
- Belief DSL → `build-ssa-index` indexes by this, then extracts Form 1 via `ssa-short-name` (`belief.scm:183-185`)
- Belief DSL → `cg-resolve-name` searches for this by suffix-matching Form 1 (`belief.scm:271`)
- Belief DSL → `find-field-summary` matches via `ssa-name-matches?` suffix check (`belief.scm:139`)

**Properties:** Globally unique. Includes receiver type. This is the canonical SSA name.

### Form 4: Package-relative SSA name

```
stepLeader                  (top-level function — same as Form 1)
(*raft).stepLeader           (method — includes receiver but not package)
```

**Source:** `ssa.Function.RelString(pkg)` where `pkg` is the function's own package.

**Used by:** Not currently used in wile-goast, but available via Go's API.

**Properties:** Unique within a package. Shorter than Form 3. Distinguishes methods on different receiver types (unlike Form 1). Not unique across packages.

### Form 5: Package path only

```
go.etcd.io/etcd/raft/v3
```

**Source:** `ssa.Function.Pkg.Pkg.Path()` or `ast` package path.

**Used by:**
- Every layer as the `pkg` or `pkg-path` field alongside a function name
- Field index → `ssa-field-summary` `pkg` field
- SSA mapper → `ssa-function` `pkg` field
- Call graph mapper → `cg-node` `pkg` field
- Belief DSL → site function `pkg-path` field from AST traversal

**Properties:** Not a function name — it's the package component, used together with Form 1 to reconstruct Form 2, or as a filter alongside Form 3.

## Where the Bug Was

```
Field index:  Form 1 (func) + Form 5 (pkg)  →  FCA built Form 2
Call graph:   Form 3 (name, caller, callee)

Form 2 ≠ Form 3 for methods.
```

`"go.etcd.io/etcd/raft/v3.stepLeader"` (Form 2) could never match `"(*go.etcd.io/etcd/raft/v3.raft).stepLeader"` (Form 3). The fix changed the field index to emit Form 3 directly.

## Current State (Post-Fix)

| Layer | Field | Form | Notes |
|-------|-------|------|-------|
| AST mapper | `func-decl.name` | 1 | Short name from Go source |
| SSA mapper | `ssa-function.name` | 1 | **Mismatch with CG** |
| SSA field index | `ssa-field-summary.func` | 3 | Fixed (was 1) |
| SSA field index | `ssa-field-summary.pkg` | 5 | Unchanged |
| CG mapper | `cg-node.name` | 3 | Canonical |
| CG mapper | `cg-edge.caller/callee` | 3 | Canonical |
| CG primitives | user argument | 3 | User must pass Form 3 |
| CFG primitive | user argument | 1 | User passes short name |
| Belief DSL sites | `name` | 1 | From AST func-decl |
| Belief DSL | `find-field-summary` | 1→3 | Suffix match bridge |
| Belief DSL | `cg-resolve-name` | 1→3 | Suffix match bridge |
| Belief DSL | `ssa-short-name` | 3→1 | Strips to short name |
| FCA | `field-index->context` | 3 | Fixed (was 2) |

## Remaining Inconsistencies

1. **SSA mapper uses Form 1, call graph uses Form 3.** The `ssa-function` node's `name` field is `fn.Name()` (Form 1), while `cg-node.name` is `fn.String()` (Form 3). A user querying SSA functions sees short names; querying the call graph sees qualified names. Cross-referencing requires `ssa-short-name`.

2. **CFG primitive accepts Form 1, call graph accepts Form 3.** `(go-cfg session "Step")` works; `(go-callgraph-callers cg "Step")` does not. The user must know which form each primitive expects.

3. **Belief DSL has three normalization functions.** `ssa-short-name` (3→1), `cg-resolve-name` (1→3), `ssa-name-matches?` (3≈1). These exist solely to bridge the form mismatch.

## Reduction Plan

**Target state:** Two forms — Form 1 (short name, user-facing) and Form 3 (SSA qualified, machine-facing). Eliminate Form 2 entirely. Form 4 is unused, keep it out. Form 5 stays as package metadata.

### Step 1: SSA mapper — add Form 3

Add an `ssa-name` field to `ssa-function` nodes with `fn.String()` (Form 3), alongside the existing `name` field (Form 1). The `name` field stays for readability; `ssa-name` is for cross-referencing.

**File:** `goastssa/mapper.go:56`

```go
// Current:
goast.Field("name", goast.Str(fn.Name())),

// Proposed:
goast.Field("name", goast.Str(fn.Name())),
goast.Field("ssa-name", goast.Str(fn.String())),
```

This lets users cross-reference SSA functions with call graph nodes without normalization.

### Step 2: CFG primitive — accept Form 3

`go-cfg` currently uses `findFunction` which matches by `fn.Name()`. Add a fast path: if the user's argument contains `"."` or `"("`, treat it as Form 3 and match via `fn.String()` instead.

**File:** `goastcfg/prim_cfg.go:150-172`

### Step 3: Eliminate belief DSL normalizers

Once SSA functions carry `ssa-name` (Form 3):
- `find-field-summary`: match by `(equal? (nf entry 'func) ssa-name)` directly — no suffix matching
- `cg-resolve-name`: look up `ssa-name` from the site's SSA function — no search
- `ssa-short-name`: still useful for display, but no longer needed for correctness

### Step 4: Document the convention

Add to `docs/PRIMITIVES.md`:

> **Function names:** Primitives that accept function names accept two forms: short name (`"Step"`) or SSA-qualified name (`"(*pkg.Type).Step"`). Short names are matched within the loaded package. SSA-qualified names are matched exactly. The call graph always uses SSA-qualified names. When cross-referencing between layers, use the `ssa-name` field.

## Trade-offs

**Why not just use Form 3 everywhere?** Form 3 is verbose for interactive use. `(go-cfg session "Step")` is much friendlier than `(go-cfg session "(*go.etcd.io/etcd/raft/v3.raft).Step")`. The user-facing API should accept Form 1 as a convenience.

**Why not use Form 4 (package-relative)?** It's a middle ground — `(*raft).Step` — but it's neither user-friendly (still has receiver syntax) nor machine-unique (not unique across packages). It doesn't eliminate any normalization.

**Why keep Form 5 (pkg) on field-summary?** The `pkg` field enables filtering by package without parsing Form 3. Useful for multi-package analysis where you want summaries from specific packages.
