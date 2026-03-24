# Shared Session Design

## Summary

Introduce a first-class `GoSession` object that holds loaded Go packages and
lazily-built analysis state (SSA, callgraph). All package-loading primitives
accept either a pattern string (load fresh, as today) or a GoSession (reuse
loaded state). A new `go-load` primitive creates sessions, and `go-list-deps`
provides lightweight dependency discovery before loading.

## Motivation

### Redundant loading

Every package-loading primitive independently calls `packages.Load`, type-checks,
and (for SSA-based layers) builds SSA. A single belief run that uses AST + SSA +
CFG + callgraph does up to 5 `packages.Load` calls and 4 SSA builds for the same
package. The Scheme-level caching in the belief DSL prevents calling the same
primitive twice, but between different primitives on the same package, Go reloads
and retypes everything.

### Snapshot consistency

Today, separate calls to `go-ssa-build` and `go-cfg` each call `packages.Load`
independently. If the source changes between calls, the two results are
inconsistent — SSA from version A, CFG from version B. A session provides a
single point of acquisition: all primitives operating on the same session see
the same code, regardless of when they are called.

### API composability

Ad-hoc scripts (outside the belief DSL) have no way to share loaded state across
layers. A script combining typecheck + SSA + CFG queries on the same package gets
3 redundant loads with no way to avoid it. Sessions make the natural usage pattern
also the efficient one.

## Design

### GoSession struct

Lives in `goast/` (the base package all sub-extensions depend on).

```go
// goast/session.go
type GoSession struct {
    patterns []string
    pkgs     []*packages.Package
    fset     *token.FileSet
    lintMode bool  // true if loaded with LoadAllSyntax

    // Lazy SSA — built on first demand
    ssaOnce sync.Once
    prog    *ssa.Program
    ssaPkgs []*ssa.Package

    // Lazy all-packages SSA build (for callgraph)
    allPkgsOnce sync.Once

    // Callgraph cache — per algorithm
    cgMu    sync.Mutex
    cgCache map[string]*callgraph.Graph
}

func (s *GoSession) SchemeString() string {
    return fmt.Sprintf("#<go-session %q %d-pkgs>", s.patterns, len(s.pkgs))
}
func (s *GoSession) IsVoid() bool         { return s == nil }
func (s *GoSession) EqualTo(v Value) bool { return s == v }
```

SSA is always built with `SanityCheckFunctions | InstantiateGenerics`. One SSA
program, one set of flags — `InstantiateGenerics` is needed for callgraph and
harmless for other layers.

### Lazy SSA building

```go
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

func (s *GoSession) SSAAllPackages() *ssa.Program {
    prog, _ := s.SSA()
    s.allPkgsOnce.Do(func() {
        for _, pkg := range prog.AllPackages() {
            pkg.Build()
        }
    })
    return prog
}
```

`SSA()` builds only the requested packages. `SSAAllPackages()` builds all
transitively loaded packages — needed by callgraph algorithms. `Build()` is
idempotent, so already-built packages are a no-op.

### New primitives

#### `go-load`

```scheme
(go-load pattern ... . options) → GoSession
```

Creates a session by calling `packages.Load` once with all patterns as roots.

Default load mode: `NeedName | NeedFiles | NeedSyntax | NeedTypes |
NeedTypesInfo | NeedImports | NeedDeps`.

Options:
- `'lint` — upgrades to `LoadAllSyntax` for `go-analyze` support.

Multi-pattern: all patterns are loaded into a single session. The loaded
packages are the roots; transitive imports are loaded because Go's type
checker requires them. Primitives return results for root packages only.

Fails fast on load errors — no partially-loaded sessions.

```scheme
(define s (go-load "my/pkg/a" "my/pkg/b"))
(define s (go-load "my/pkg/..." 'lint))
```

#### `go-list-deps`

```scheme
(go-list-deps pattern ...) → list of import path strings
```

Lightweight dependency discovery. Uses `packages.Load` with `NeedName |
NeedImports` only — no type checking, no syntax loading. Returns the
transitive closure of import paths for all given patterns.

Use before `go-load` to inspect scope:

```scheme
(go-list-deps "my/pkg/a" "my/pkg/b")
;; → ("my/pkg/a" "my/pkg/b" "fmt" "sync" ...)
```

#### `go-session?`

```scheme
(go-session? v) → #t | #f
```

Type predicate.

### Modified primitives (dual-accept)

Each package-loading primitive gains a type switch at the top:

```go
func PrimGoSSABuild(mc *machine.MachineContext) error {
    arg := mc.Arg(0)
    switch v := arg.(type) {
    case *GoSession:
        return ssaBuildFromSession(mc, v)
    case *values.String:
        return ssaBuildFromPattern(mc, v)
    default:
        return werr.WrapForeignErrorf(werr.ErrNotAString,
            "go-ssa-build: expected string or go-session, got %T", arg)
    }
}
```

The `FromSession` path uses the session's lazy builders. The `FromPattern`
path is the existing code, unchanged.

Affected primitives:

| Primitive | Session argument position |
|-----------|--------------------------|
| `go-typecheck-package` | arg 0 (replaces pattern) |
| `go-ssa-build` | arg 0 (replaces pattern) |
| `go-ssa-field-index` | arg 0 (replaces pattern) |
| `go-cfg` | arg 0 (replaces pattern) |
| `go-callgraph` | arg 0 (replaces pattern) |
| `go-analyze` | arg 0 (replaces pattern) |
| `go-interface-implementors` | arg 1 (replaces package-pattern) |

### Unchanged primitives

No session involvement — these don't load packages:

- `go-parse-file`, `go-parse-string`, `go-parse-expr`
- `go-format`, `go-node-type`
- `go-cfg-dominators`, `go-cfg-dominates?`, `go-cfg-paths`
- `go-callgraph-callers`, `go-callgraph-callees`, `go-callgraph-reachable`
- `go-analyze-list`

## Belief DSL integration

`make-context` creates a session and passes it to all primitives. The
Scheme-side lazy caching stays — it caches s-expression results to avoid
re-mapping. The Go-side session avoids redundant loading and building.

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

(define (ctx-session ctx) (ctx-ref ctx 'session))

(define (ctx-pkgs ctx)
  (or (ctx-ref ctx 'pkgs)
      (let ((pkgs (go-typecheck-package (ctx-session ctx))))
        (ctx-set! ctx 'pkgs pkgs)
        pkgs)))
```

Two layers of caching, each with a clear purpose:
- Go side (GoSession): caches loaded packages, SSA program — avoids redundant
  `packages.Load` and SSA builds
- Scheme side (ctx): caches s-expression results — avoids redundant mapping

Checkers that call Go primitives directly (e.g., `ordered` calls `go-cfg`)
extract the session from ctx:

```scheme
(cfg (and pkg-path (go-cfg (ctx-session ctx) fname)))
```

## Edge cases

**Snapshot consistency.** A session captures source at load time. All
primitives on the same session see the same code. This is a design goal —
prevents inconsistent cross-layer results from concurrent source changes.

**Load failures.** `go-load` fails fast. If `packages.Load` returns errors,
the primitive returns a Scheme error immediately. No partially-loaded sessions.

**Mixed patterns.** A session is bound to its root patterns. Primitives
operate on whatever packages the session loaded. To add packages, create a
new session with all needed patterns.

**Lint mode mismatch.** If a session was loaded without `'lint` and passed to
`go-analyze`, the analyzer falls back to loading fresh internally (same as
today). No error — just no session benefit for that call.

**Transitive loading.** `go-load` loads the requested packages plus all
transitive imports as a single atomic unit. Transitive deps are required by
Go's type checker. `go-list-deps` provides visibility into the transitive
closure before loading.

**Memory.** Sessions are garbage collected when no Scheme references remain.
No explicit close or dispose needed — matches how Channel, Thread, and other
opaque values work in Wile.

## Wile core dependency

GoSession wraps a Go struct as a Scheme value. This requires wile to support
opaque foreign values — a generic `OpaqueValue` type in `wile/values/`:

- `SchemeString()` → `#<tag:id>` display
- `IsVoid()`, `EqualTo()` (identity-based)
- Type predicate: `(opaque? v)`, tag accessor: `(opaque-tag v)`

~80 lines + tests. This work is blocked on in-progress wile changes.

Note: GoSession implements `values.Value` directly (not via `OpaqueValue`),
since it has session-specific methods (`SSA()`, `SSAAllPackages()`, etc.)
that Go code calls. The `OpaqueValue` facility is the prerequisite that
establishes the pattern and infrastructure for opaque values in wile.

## Implementation order

Dependency chain:

1. **A1: OpaqueValue in wile** (blocked — wile changes in progress)
2. **A2: GoSession + go-load + go-list-deps + go-session?** (depends on A1)
3. **A3: Refactor primitives to dual-accept** (depends on A2)
4. **A4: Update belief DSL + docs** (depends on A3)

See `TODO.md` for the complete task list with both tracks.

## Composable usage example

```scheme
;; Discover scope
(go-list-deps "my/pkg/a" "my/pkg/b")
;; → ("my/pkg/a" "my/pkg/b" "fmt" "sync" ...)

;; Load once, query many — all layers see the same source snapshot
(define s (go-load "my/pkg/a" "my/pkg/b"))
(define pkgs (go-typecheck-package s))
(define ssa  (go-ssa-build s))
(define cfg  (go-cfg s "MyFunc"))
(define cg   (go-callgraph s 'cha))

;; Old style still works — loads fresh each time
(go-ssa-build "my/pkg/a")
```
