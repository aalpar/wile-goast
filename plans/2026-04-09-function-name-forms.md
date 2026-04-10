# Function Name Forms in wile-goast

**Goal:** Document every function name form, where each is produced and consumed, identify which can be eliminated, and converge on a design where Form 1 never escapes into cross-referencing contexts.

**Status:** Findings documented. Reduction plan finalized.

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

**Source:** `ssa.Function.RelString(pkg)` where `pkg` is the function's own package. Can also be constructed from AST: `f.Recv` (receiver type) + `f.Name.Name`.

**Used by:** Not currently used in wile-goast.

**Properties:** Unique within a package. Shorter than Form 3. Distinguishes methods on different receiver types (unlike Form 1). Not unique across packages. Proper suffix of Form 3.

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

## Root Cause

Every site that produces a Form 1 name has a caller with enough context to produce a more qualified form. But the functions don't require that context as a parameter, so they default to the weakest form. The result: Form 1 names escape into fields that get cross-referenced, and normalizers proliferate downstream to compensate.

| Production site | Context available at caller | Could produce |
|----------------|---------------------------|---------------|
| AST mapper (`mapFuncDecl`) | `mapperOpts` has `typeInfo`; caller has `pkgPath` | Form 3 (typed) or Form 4 (untyped) |
| SSA mapper (`mapFunction`) | Has `ssa.Function` directly | Form 3 via `fn.String()` |
| SSA field index (`buildFuncSummary`) | Has `ssa.Function` directly | Form 3 — **already fixed** |
| CFG lookup (`findFunction`) | Has `ssa.Function` for comparison | Form 3 via `fn.String()` |
| Belief DSL (`all-func-decls`) | Has `pkg-path` from package traversal | Form 3 (if receiver type qualified) |

No site is forced to produce Form 1 because of missing context. The context exists — it's just not required as a parameter.

## Reduction Plan

**Design principle:** Require context at production sites so Form 1 never escapes into cross-referencing fields. Form 1 is a valid local representation (e.g. inside `findFunction` for SSA package lookup), but it should never be stored in a node field that downstream layers will match against.

**Target state:** Two forms in exposed node fields:
- Form 3 (SSA qualified) — all fields that carry function identity: `func-decl.name` (typed), `ssa-function.name`, `ssa-field-summary.func`, `cg-node.name`, `cg-edge.caller/callee`
- Form 1 (short name) — only accepted as user-facing input (convenience); primitives resolve it internally

Form 2 eliminated (already done). Form 4 is no longer needed as a separate target — typed ASTs can produce Form 3 directly, and untyped ASTs don't participate in cross-referencing. Form 5 stays as package metadata.

### Step 1: Thread `pkgPath` through `mapperOpts`

Add `pkgPath string` to `mapperOpts`. Every caller that maps typed ASTs already has it:
- `mapPackage` in the typecheck pipeline → `pkg.PkgPath`
- `PrimGoLoad` → from the loaded `packages.Package`

Untyped AST callers (`go-parse-file`, `go-parse-string`) pass `""` — these ASTs aren't cross-referenced.

**File:** `goast/mapper.go:28-33`

```go
type mapperOpts struct {
    fset      *token.FileSet
    positions bool
    comments  bool
    typeInfo  *types.Info
    pkgPath   string      // empty for untyped ASTs
}
```

### Step 2: `mapFuncDecl` produces Form 3 (typed) or Form 1 (untyped)

When `typeInfo` and `pkgPath` are available, build the fully qualified name. The receiver type can be resolved via `typeInfo.TypeOf(recv)` to get the package-qualified type name, matching what `ssa.Function.String()` produces.

When `typeInfo` is nil (untyped AST), keep Form 1. These ASTs are for formatting/display, not cross-referencing.

**File:** `goast/mapper.go:206-218`

```go
func mapFuncDecl(f *ast.FuncDecl, opts *mapperOpts) values.Value {
    funcName := f.Name.Name
    if opts.pkgPath != "" {
        if f.Recv != nil && len(f.Recv.List) > 0 {
            recvType := qualifiedRecvType(f.Recv.List[0].Type, opts)
            funcName = "(" + recvType + ")." + f.Name.Name
        } else {
            funcName = opts.pkgPath + "." + f.Name.Name
        }
    }
    // ...
    Field("name", Str(funcName)),
}
```

`qualifiedRecvType` uses `opts.typeInfo` to resolve the receiver's type to its full import path. For `*raft` in package `go.etcd.io/etcd/raft/v3`, this produces `*go.etcd.io/etcd/raft/v3.raft`, matching the SSA convention.

```go
func qualifiedRecvType(expr ast.Expr, opts *mapperOpts) string {
    if opts.typeInfo == nil {
        return types.ExprString(expr)  // fallback: unqualified
    }
    tv, ok := opts.typeInfo.Types[expr]
    if !ok {
        return types.ExprString(expr)
    }
    return types.TypeString(tv.Type, nil)  // nil qualifier → full path
}
```

**The form tracks information fidelity:** typed ASTs produce Form 3 (globally unique). Untyped ASTs produce Form 1 (honest about what they don't know). No normalizers needed downstream because the form is determined at production time based on available context.

### Step 3: SSA mapper uses Form 3

Change `ssa-function.name` from `fn.Name()` to `fn.String()`. The `ssa.Function` is right there — no context threading needed.

**File:** `goastssa/mapper.go:56`

```go
// Before:
goast.Field("name", goast.Str(fn.Name())),

// After:
goast.Field("name", goast.Str(fn.String())),
```

Now `ssa-function.name` matches `cg-node.name` — direct equality, no normalization.

### Step 4: CFG primitive accepts both forms

`go-cfg` keeps accepting Form 1 as a convenience. Internally, `findFunction` first tries exact match via `fn.String()` (Form 3), then falls back to `fn.Name()` match (Form 1). No change to user-facing API, but Form 3 input works too.

**File:** `goastcfg/prim_cfg.go:150-172`

```go
func findFunction(prog *ssa.Program, ssaPkg *ssa.Package, name string) *ssa.Function {
    // Fast path: try Form 3 exact match on all functions
    if strings.Contains(name, ".") || strings.Contains(name, "(") {
        for _, mem := range ssaPkg.Members {
            if fn, ok := mem.(*ssa.Function); ok && fn.String() == name {
                return fn
            }
        }
        return nil
    }
    // Form 1: existing short-name lookup
    fn := ssaPkg.Func(name)
    if fn != nil {
        return fn
    }
    // ... method search by fn.Name() ...
}
```

### Step 5: Eliminate belief DSL normalizers

With `func-decl.name` carrying Form 3 (for typed ASTs) and `ssa-field-summary.func` already carrying Form 3:

- **`find-field-summary`**: `(equal? (nf entry 'func) (nf site 'name))` — exact match. Delete `ssa-name-matches?`.
- **`cg-resolve-name`**: `(nf site 'name)` is already Form 3. Use it directly with `go-callgraph-callers`. Delete `cg-resolve-name`.
- **`ssa-short-name`**: Keep for display/logging only. Remove from matching paths.
- **`build-ssa-index`**: Index by Form 3 directly. No extraction step needed.

**Files:** `cmd/wile-goast/lib/wile/goast/belief.scm`

### Step 6: Document the convention

Add to `docs/PRIMITIVES.md`:

> **Function names in node fields:** All `name` fields on typed AST, SSA, call graph, and field index nodes carry the SSA-qualified name (e.g. `"(*go.etcd.io/etcd/raft/v3.raft).Step"` for methods, `"go.etcd.io/etcd/raft/v3.stepLeader"` for top-level functions). Untyped AST nodes (`go-parse-file`, `go-parse-string`) carry the short Go name.
>
> **User-facing arguments:** Primitives that accept function names (`go-cfg`, `go-callgraph-callers`, etc.) accept both short names (`"Step"`) and SSA-qualified names. Short names are resolved within the loaded package; SSA-qualified names match exactly.

## Blast Radius

### What breaks

1. **Scheme scripts comparing `func-decl.name` against short strings.** Example: `(equal? (nf fn 'name) "Step")` on a typed AST will fail — now it's `"(*go.etcd.io/etcd/raft/v3.raft).Step"`. Fix: use `name-matches` predicate or match the full name.

2. **Belief DSL `name-matches` predicate.** Currently does substring match on Form 1. Needs to handle Form 3 names — either match against the method suffix or use exact match.

3. **Any test asserting `func-decl.name` is a short string** for typed ASTs. Must be updated to expect Form 3.

### What doesn't break

- Untyped ASTs (`go-parse-file`, `go-parse-string`) — still Form 1, no change.
- `go-format` (AST → Go source) — reads `f.Name.Name` from the AST node, not the mapped `name` field.
- Call graph, field index, FCA — already use Form 3.
- `go-cfg` — accepts both forms after Step 4.

## Trade-offs

**Why not Form 4 at the AST layer instead of Form 3?** If `typeInfo` is available, we can produce Form 3 directly. Form 4 would be the right choice only if we didn't have `typeInfo` but did have the receiver. Since every typed AST has both `typeInfo` and `pkgPath`, Form 3 is achievable and eliminates all normalization — not just within-package normalization. Form 4 would still require a package-prefix step to match Form 3.

**Why keep Form 1 as user-facing input?** Ergonomics. `(go-cfg session "Step")` is what a human types. The primitive resolves it internally. Form 1 is valid as a *query* — it's invalid as a *stored identity* that other layers will match against.

**Why keep Form 5 (pkg)?** Filtering. When analyzing multiple packages, `pkg` enables `(filter (lambda (s) (equal? (nf s 'pkg) "my/pkg")) summaries)` without parsing Form 3.
