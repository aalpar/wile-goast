# Axis-B Analyzer Implementation Design (Phase 3.B)

**Status**: Design. Not yet implemented.
**Parent designs (in wile repo)**:
- `wile/plans/2026-04-19-primitive-annotation-audit.md` — parent audit plan (axes A / B / C).
- `wile/plans/2026-04-19-axis-b-analyzer-design.md` — Phase 3 overall design (buckets, pipeline, output formats).
**Upstream artifact**: `wile/plans/axis-b-manifest.scm` — Phase 3.A committed on `feat/axis-b-manifest` in wile repo, 475 primitives enumerated with Go function names and source locations (428 resolved + 47 binding-only). Ready to consume.
**Scope of this document**: Phase 3.B — the SSA narrowing primitive + supporting settings infrastructure + the axis-b analyzer script, all in wile-goast.

---

## 1. Framing

Phase 3.A shipped the manifest — a committed S-expression list of every wile primitive with its declared `ReturnType`, Go function name, and source file:line. Phase 3.B consumes that manifest and produces the structured raw data (§7.3 of the parent Phase 3 design) that Phase 3.C will fold into the bucketed inventory (§7.1) and annotation-bug sidecar (§7.4).

The core operation: for each primitive's Go function, enumerate every reachable result-writing sink call, narrow each sink's value argument back through SSA to its concrete-type sources, union per primitive, classify into a bucket.

Phase 3.B ships three coordinated deliverables:

1. **Target setting** — a wile-goast parameter + env var so scripts stop hardcoding Go package paths. General-purpose infrastructure, not axis-b-specific.
2. **`go-ssa-narrow` primitive** — new wile-goast primitive that backward-walks an SSA value's def-use chain and returns a type set + tagged confidence. Iteratively deepenable per the kill-criteria gate.
3. **axis-b analyzer script** — `cmd/wile-goast/scripts/wile-axis-b.scm`, consumes (1) + (2) + wile's manifest to produce raw output and markdown inventory.

---

## 2. Non-goals

- Not shipping Phase 3.C (inventory landing in wile repo) or Phase 3.D (annotation-bug sweep) — separate plans.
- Not extending wile's `TypeConstraint` vocabulary. The §6 decision of the parent design stands: widenings are `TypeAny`. This phase only produces evidence.
- Not building a full static-analysis framework. `go-ssa-narrow` handles the specific SSA constructs wile's Impl functions use; exotic cases (reflect dispatch, interface-forwarding variadics, escape-through-channels) widen with `confidence=widened` and `reason` tags for later targeting.
- Not migrating existing scripts. `unify-detect.scm`, `goast-query.scm`, and `belief-example.scm` keep working unchanged after PR-1 lands; migrating them to drop hardcoded targets is a separate follow-up PR.
- Not adding CLI `--arg` support to wile-goast. Script inputs use environment variables for overrides and sensible defaults otherwise.
- Not running the analyzer under `make test` in either repo. It runs on demand per the parent design.

---

## 3. Success criteria

1. **Target setting**: every wile-goast primitive that accepts a pattern string (`go-ssa-build`, `go-callgraph`, `go-typecheck-package`, `go-parse-file`, `go-load`) reads `(current-go-target)` when no explicit arg is supplied. Initial value is `WILE_GOAST_TARGET` env var or `"./..."` fallback.
2. **Narrowing primitive**: `go-ssa-narrow` returns a three-field record (`types`, `confidence`, `reasons`) for any SSA value reference. Three confidence states: `narrow`, `widened`, `no-paths`. Reasons list is non-empty only for `widened`.
3. **Smoke test**: the five-primitive table (see §8) produces exact expected output. No flake.
4. **Reproducibility**: the axis-b script emits byte-identical output across runs on an unchanged wile tree.
5. **Kill-criterion gate**: on the first real run over wile's 428 non-binding-only primitives, **<30%** land in Helper-widened (confidence `widened`). If more, PR-3 does not land — PR-2' extends narrowing first.

---

## 4. Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│ wile-goast repo                                                 │
│                                                                 │
│  ┌────────────────────────┐   ┌──────────────────────────────┐  │
│  │ PR-1: Target setting   │   │ PR-2: Narrowing primitive    │  │
│  │                        │   │                              │  │
│  │ make-parameter         │   │ go-ssa-narrow                │  │
│  │ + WILE_GOAST_TARGET    │   │ (Go impl in goastssa/)       │  │
│  │                        │   │ SSA def-use walker           │  │
│  │ goastssa / goastcg /   │   │ MVP coverage boundary (§6)   │  │
│  │ goast primitives       │   │                              │  │
│  │ default-to-parameter   │   │                              │  │
│  └────────┬───────────────┘   └──────────────┬───────────────┘  │
│           │                                  │                  │
│           └──────────────┬───────────────────┘                  │
│                          │                                      │
│                          ▼                                      │
│            ┌────────────────────────────────┐                   │
│            │ PR-3: axis-b script            │                   │
│            │ cmd/wile-goast/scripts/        │                   │
│            │   wile-axis-b.scm              │                   │
│            │                                │                   │
│            │ Loads wile's manifest          │                   │
│            │ Uses target setting (PR-1)     │                   │
│            │ Calls go-ssa-narrow (PR-2)     │                   │
│            │ Emits raw + markdown           │                   │
│            └────────────────────────────────┘                   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**PR sequencing.** PR-1 and PR-2 are independent; either order or parallel. PR-3 depends on both being merged and a rebuilt wile-goast binary being installed locally.

**A follow-up PR-4** migrates existing scripts to drop hardcoded targets. Held separate from PR-1 so infrastructure lands reviewably.

---

## 5. PR-1 — target setting

### 5.1 The parameter

New file: `cmd/wile-goast/lib/wile/goast/defaults.scm`.

```scheme
(define current-go-target
  (make-parameter
    (let ((env (get-environment-variable "WILE_GOAST_TARGET")))
      (if (and env (not (string=? env "")))
          env
          "./..."))))
```

The env var is consulted once, at the parameter's construction time, to set the initial value. No separate "override after the fact" step — that avoids a footgun where R7RS parameters don't universally support positional-arg mutation (only `parameterize` is standard for override). Runtime consumers use `parameterize` to scope overrides.

Scripts read via `(current-go-target)` and override via `parameterize`. Fluid-binding semantics — a nested `parameterize` restores on exit, so a script can analyze multiple targets in sequence without manual state management.

### 5.2 Primitive consumption

Five Go-side primitives become optional-arg: if no arg, read `(current-go-target)` from the engine and use that; otherwise use the explicit arg (existing behavior preserved).

| Primitive | File | Notes |
|---|---|---|
| `go-ssa-build` | `goastssa/` | Already variadic for options; add pattern-from-parameter fallback |
| `go-callgraph` | `goastcg/` | Same treatment |
| `go-typecheck-package` | `goast/` | Same |
| `go-parse-file` | `goast/` | Same (if applicable — parse-file takes a path, not a pattern; verify during implementation) |
| `go-load` | `goast/` | Session constructor; pattern fallback |

A shared Go helper — `resolveTarget(args, engine)` — handles the check:
```go
func resolveTarget(args []values.Value, eng *wile.Engine) (string, error) {
    if len(args) > 0 {
        return coerceStringArg(args[0])
    }
    val, err := eng.InvokeParameter("current-go-target")
    if err != nil {
        return "", err
    }
    return coerceString(val)
}
```

### 5.3 The top-level-function-default rule

This setting is the canonical exception to the codebase's general "never default on nil/zero" rule. The rule exists to prevent silent fallbacks inside deep-call stacks where a missing value signals a bug. `current-go-target` is a *top-level session root* — analogous to a primordial-thread-like object at the top of the heap. Top-level entry points are the sanctioned places where a zero-value default is part of the interface, not a bug.

This rule is documented here explicitly so future readers who see the `"./..."` default don't mistakenly "fix" it.

### 5.4 PR-1 tests

Go-side unit tests for each affected primitive:
- Explicit arg → primitive uses it, parameter unchanged.
- No arg + parameter at default → uses `"./..."`.
- No arg + parameter overridden via `parameterize` → uses the override.
- `WILE_GOAST_TARGET` set at engine construction → parameter initialized to that value.

Scheme integration test in `test/`:
```scheme
(parameterize ((current-go-target "github.com/aalpar/wile-goast/..."))
  (let ((result (go-ssa-build)))
    (assert-not-nil result)))
```

### 5.5 Default when both env and arg are absent

`"./..."` — the current wile-goast default. Matches existing behavior exactly. No migration hazard.

---

## 6. PR-2 — `go-ssa-narrow` primitive

### 6.1 Signature

```scheme
(go-ssa-narrow ssa-value)
  ; => (narrow-result
  ;      (types (string ...))       ; fully-qualified Go type names
  ;      (confidence narrow|widened|no-paths)
  ;      (reasons (symbol ...)))    ; empty unless confidence=widened
```

The input `ssa-value` is any SSA value reference exposed by existing wile-goast primitives: instruction result, parameter, global, constant, or `Extract` from a tuple.

### 6.2 Algorithm

Backward walk over the def-use chain of `ssa-value`, dispatching on the producing instruction kind.

| Source | Action | Confidence contribution |
|---|---|---|
| `Alloc`, `New`, `&T{...}`, composite literal of concrete type `T` | Record `"*T"` | `narrow` |
| Call whose return type is a concrete (non-interface) type | Record return type | `narrow` |
| Call whose return type is an interface, callee is a concrete function (static dispatch) | Recurse into callee's return paths; union | (from callee) |
| Call whose return type is an interface, callee is an interface method (dynamic dispatch, e.g., `x.Foo()` where `x: SomeInterface`) and the receiver type couldn't be narrowed to a concrete type | Record no type; add reason `interface-method-dispatch` | `widened` |
| `Phi` | Recurse on each operand; union | (from operands) |
| Type assertion `x.(T)` | Record `T` | `narrow` |
| `Extract` from `(T, ok)` tuple return | Recurse on tuple at the correct index | (from recursion) |
| Load from global or struct field of interface type | Record no type; add reason `global-load` or `field-load` | `widened` |
| Parameter of enclosing function | Record no type; add reason `parameter` | `widened` |
| `nil` constant | Record no type; add reason `nil-constant` | `widened` |
| Cycle in inter-procedural walk (visited-set hit) | Record no type for this path; add reason `cycle` | `widened` |
| No def found / unreachable code | No type | `no-paths` |

**Overall confidence rule**: if any operand-path widened, overall is `widened` (reasons unioned). If every path resolved to a concrete type, `narrow`. If no path produced any type at all, `no-paths`.

**Cycle detection**: visited-set keyed by `(ssa.Function pointer, ssa.Value pointer)`. Pointer identity is stable across the analysis.

### 6.3 MVP coverage (PR-2)

Handled in the first ship:
- `Alloc`, `New`, composite literals of concrete type
- Direct-return calls (interface and concrete)
- Inter-procedural recursion with cycle break
- `Phi` union over operands
- Type assertion
- `Extract` from `(T, ok)` tuple

Widens with reason tag (no analysis):
- Parameters → reason `parameter`
- Global / field loads of interface type → reason `global-load` or `field-load`
- `nil` constants → reason `nil-constant`
- Cycle → reason `cycle`
- Interface-method dispatch with unresolvable receiver → reason `interface-method-dispatch`

Surfaces as `no-paths`:
- No sink-reachable path found from the starting value (detectable gap — see §8 smoke cases).

**Deferred to PR-2' iterations, if Helper-widened count demands them:**
- Call-graph-context parameter narrowing (look up all callers of a function and narrow the parameter by the union of arg types at call sites).
- Slice / map element type reasoning.
- Type-switch arm narrowing.
- Reflect-based value production.

### 6.4 Type classification layer

`go-ssa-narrow` returns Go type strings (e.g., `"*github.com/aalpar/wile/values.Integer"`). Mapping those to wile's `values.ValueType.Name()` strings (e.g., `"integer"`) is axis-b-specific and lives in the axis-b script — not in this primitive. Keeps `go-ssa-narrow` domain-neutral.

### 6.5 PR-2 tests

Go fixtures in `goastssa/testdata/narrow_*.go`, one file per case, each with a single well-named function whose return shape is known by construction.

| Fixture | Expected types | Expected confidence |
|---|---|---|
| `narrow_direct.go` — returns `*Foo{}` | `{*Foo}` | `narrow` |
| `narrow_phi.go` — if-else returning `*Foo` vs `*Bar` | `{*Foo, *Bar}` | `narrow` |
| `narrow_helper.go` — calls `helper()` which returns `*Foo` | `{*Foo}` | `narrow` |
| `narrow_helper_phi.go` — helper has its own phi-union | union of helper's types | `narrow` |
| `narrow_cycle.go` — A calls B, B calls A | whatever types are reached before cycle-break | `widened` with reason=`cycle` |
| `narrow_assertion.go` — `v.(*Foo)` | `{*Foo}` | `narrow` |
| `narrow_parameter.go` — returns a parameter of interface type | `{}` | `widened` with reason=`parameter` |
| `narrow_global.go` — returns a global `interface{}` value | `{}` | `widened` with reason=`global-load` |
| `narrow_tuple.go` — `x, _ := f()` where f returns `(*Foo, bool)` | `{*Foo}` | `narrow` |

Scheme integration test per fixture: `(go-ssa-narrow <value-from-fixture>)` — assert exact tuple match.

---

## 7. PR-3 — axis-b analyzer script

### 7.1 Script location and invocation

New file: `cmd/wile-goast/scripts/wile-axis-b.scm`.

Invocation: `wile-goast --run wile-axis-b`.

### 7.2 Inputs

- **Manifest**: `WILE_AXIS_B_MANIFEST` env var, defaulting to `<workspace>/wile/plans/axis-b-manifest.scm` where `<workspace>` is discovered by walking up from the wile-goast repo to the `go.work` file.
- **Target pattern**: implicit via `current-go-target`, set by the script to `"github.com/aalpar/wile/..."` via `parameterize`.

### 7.3 Outputs

- **Raw data**: `WILE_AXIS_B_RAW_OUTPUT` env var, defaulting to `<wile-repo>/plans/axis-b-raw.scm`.
- **Markdown inventory**: `WILE_AXIS_B_INVENTORY` env var, defaulting to `<wile-repo>/plans/2026-04-19-axis-b-inventory.md`.
- **Stdout**: summary — count per bucket, reason-tag tally for widened entries, per-primitive list of `no-paths` cases.

### 7.4 Script flow

```scheme
;; cmd/wile-goast/scripts/wile-axis-b.scm (pseudocode)

(define sink-functions
  ;; Discovered at PR-3 implementation time by reading wile's machine/ code.
  ;; Listed here as a fixed set keyed on fully-qualified Go method names.
  ;; If this list grows beyond ~15 entries during research, the §9 kill
  ;; criterion triggers.
  '(...))

(define (analyze-primitive entry ssa-pkgs)
  (let* ((fn (resolve-function entry ssa-pkgs))
         (sink-calls (find-sink-calls fn sink-functions))
         (narrows (map (lambda (call)
                         (go-ssa-narrow (sink-call-value-arg call)))
                       sink-calls))
         (type-union (union-types (map narrow-types narrows)))
         (conf-union (merge-confidences narrows)))
    (classify-bucket entry type-union conf-union)))

(define (main)
  (parameterize ((current-go-target "github.com/aalpar/wile/..."))
    (let* ((entries (read-manifest (manifest-path)))
           (ssa (go-ssa-build)))
      (for-each (lambda (e)
                  (write-raw-entry (analyze-primitive e ssa)))
                entries)
      (write-markdown-inventory ...))))
```

Key Scheme-layer responsibilities — deliberately small:
- Parse the manifest S-expression.
- Enumerate sink calls per function (via SSA instruction walk).
- Invoke `go-ssa-narrow` per sink's value-arg.
- Union types + merge confidences + merge reason tags.
- Map Go type strings → `values.ValueType.Name()` strings (hardcoded table, ~27 entries).
- Bucket per §5 of the parent Phase 3 design.
- Emit raw S-expression + markdown.

All real analysis work is in `go-ssa-narrow`.

### 7.5 Sink enumeration — first step of PR-3 implementation

A one-shot research task before any script code. Read `machine/machine_context.go` (and any related files) and enumerate every function/method that writes to result state — `SetValue`, `PushValue`, plus any tail-call-style continuation handover that assigns a value. Output is:

- A Scheme constant list (the `sink-functions` above) in `cmd/wile-goast/scripts/wile-axis-b.scm`.
- A commit-level note documenting what was read and what's covered.

If the list exceeds ~15 distinct names, §9 kill criterion triggers — stop and revisit the approach.

### 7.6 Raw output format

Matches parent Phase 3 design §6.4 / §7.3:

```scheme
(primitive
  (name "car")
  (impl
    (go-function "github.com/aalpar/wile/registry/core.PrimCar")
    (go-source "registry/core/prim_pairs.go:37"))
  (declared-return-type "any")
  (narrowed-return-types ("*values.Pair" "*values.EmptyList"))
  (confidence narrow)
  (reasons ())
  (bucket Narrow-union))
```

One entry per primitive, committed to `plans/axis-b-raw.scm` in the wile repo.

### 7.7 Markdown inventory format

Matches parent Phase 3 design §7.1 — one section per bucket (Single, Maybe(T), Narrow-union, Broad-union, Polymorphic, Helper-widened, Side-effecting). Each bucket section has a compact table (primitive / narrowed types / declared / impl) and a one-paragraph interpretation.

Final section: "Type-system recommendations" — three to five distilled bullets for Extension Contracts Phase 2+.

### 7.8 Smoke test

Go test at `goastssa/axis_b_smoke_test.go` (or similar) invoking the analyzer script against a five-entry fixture-manifest and asserting exact output tuples.

| Primitive | Expected narrowed types (Go) | Expected confidence | Expected reasons |
|---|---|---|---|
| `cons` | `{*values.Pair}` | `narrow` | — |
| `null?` | `{*values.Boolean}` | `narrow` | — |
| `length` | `{*values.Integer}` | `narrow` | — |
| `car` | `{}` (reads `pair.Car` — a `values.Value` interface field) | `widened` | `field-load` |
| `+` | `{*values.Integer, *values.BigFloat, ...}` — ≥ 2 numeric concretes | `narrow` | — |

The smoke test uses a hand-written fixture-manifest with exactly these five entries. It bypasses the real wile manifest so it can assert exact output. Failure of any row fails the test with a diff — this is the canonical regression guard against any future drift in either the primitive or the script.

**The expected values in the table are informed predictions at design time.** During PR-2/PR-3 implementation, the first successful run against these five primitives is used to lock in the *actual* expected values. If `+`'s narrowing produces `widened` rather than `narrow` (because numeric dispatch crosses an unresolvable boundary), the test table is updated to reflect reality and a reason tag is captured. The smoke test's purpose is regression prevention, not design verification — the design-time predictions are rough guides, not specifications.

---

## 8. File structure

### 8.1 wile-goast repo — new files

| File | Purpose |
|---|---|
| `cmd/wile-goast/lib/wile/goast/defaults.scm` | `current-go-target` parameter + env init |
| `goastssa/narrow.go` | `go-ssa-narrow` Go implementation (SSA walker, inter-procedural recursion, cycle detection) |
| `goastssa/testdata/narrow_*.go` | Fixture files (9 per §6.5) |
| `goastssa/narrow_test.go` | Go-side unit tests |
| `test/narrow_integration_test.go` | Scheme-level integration tests |
| `cmd/wile-goast/scripts/wile-axis-b.scm` | The analyzer script |
| `goastssa/axis_b_smoke_test.go` | End-to-end smoke test for the script |

### 8.2 wile-goast repo — modified files

| File | Change |
|---|---|
| `goastssa/ssa.go` (or wherever `go-ssa-build` lives) | Pattern-arg-from-parameter fallback |
| `goastcg/cg.go` (or wherever `go-callgraph` lives) | Same |
| `goast/typecheck.go` | Same |
| `goast/parse.go` | Same (if applicable) |
| `goast/session.go` | Same for `go-load` |

### 8.3 wile repo — produced by running PR-3

| File | Contents |
|---|---|
| `plans/axis-b-raw.scm` | Raw per-primitive data from the analyzer |
| `plans/2026-04-19-axis-b-inventory.md` | Bucketed markdown inventory |

These land in the wile repo via Phase 3.C's PR — separate from PR-3.

---

## 9. Kill criteria

Re-stated at PR boundary for visibility.

**At PR-3 first-run time:**

- If the sink-function list discovered in §7.5 exceeds **~15 distinct names**, stop and revisit. The "small fixed set" assumption (parent design §6.3) is broken; consider whether sinks should be discovered programmatically rather than enumerated.
- If **any primitives** land in `confidence=no-paths`, investigate the sink enumeration first. A primitive with zero sink paths almost always means a sink was missed.
- If **>30%** of non-binding-only primitives (more than 128 of 428) land in Helper-widened, stop before landing the inventory. Analyze the reason-tag distribution and open PR-2' to extend narrowing for the dominant tag:
  - If `parameter` dominates → implement call-graph-context parameter narrowing
  - If `field-load` dominates → implement field-type tracking for specific known-struct types (expected heavy hitter: `values.Pair.Car` / `Cdr`, `values.Vector` elements)
  - If `global-load` dominates → implement global-value constant folding
  - If `cycle` dominates → examine the cycles and consider whether a bound is warranted
  - If `interface-method-dispatch` dominates → implement receiver-type-aware method resolution (once receiver is narrowed to concrete types, find the specific method implementations and union their returns)

Each of these triggers a specific extension to PR-2; the kill criterion is the feedback loop.

---

## 10. Deferred / out-of-scope

- Phase 3.C (inventory landing in wile repo) and Phase 3.D (annotation-bug sweep) — separate plans.
- `TypeConstraint` vocabulary extensions (`TypeMaybe`, `TypeUnion`, parametric types) — gated on this phase's inventory findings.
- `ParamTypes` analysis — different value-flow direction.
- Migrating `unify-detect.scm`, `goast-query.scm`, etc. to drop hardcoded targets (follow-up PR-4).
- Adding CLI `--arg key=value` support to wile-goast (out of scope; env vars suffice for this phase).
- Multi-target parameter (list of patterns). Current `current-go-target` holds a single string.
