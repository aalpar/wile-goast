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

| Primitive | File | Status in PR-1 |
|---|---|---|
| `go-ssa-build` | `goastssa/register.go` + `goastssa/prim_ssa.go` | Shipped — pattern optional |
| `go-typecheck-package` | `goast/register.go` + `goast/prim_goast.go` | Shipped — pattern optional |
| `go-load` | `goast/register.go` + `goast/prim_session.go` | Shipped — pattern optional |
| `go-callgraph` | `goastcg/` | Excluded — second arg (algorithm) is required. Making pattern optional would require type-disambiguated dispatch (string vs symbol). Not needed for axis-b; follow-up PR if a consumer appears. |
| `go-parse-file` | `goast/` | Excluded — takes a filename, not a Go package pattern. Not applicable. |

The shared helper lives in `goast/target.go` as exported `ExtractTargetAndRest(mc *machine.MachineContext, restArg values.Value) (values.Value, values.Value, error)`. `goastssa/prim_ssa.go` imports `goast` and calls `goast.ExtractTargetAndRest`. Local sentinel `errExtractTargetError` lives alongside the helper.

Each modified primitive's Go-side logic is: cast `CallContext` to `*MachineContext`, call `ExtractTargetAndRest` to unpack the rest list (with parameter fallback), then dispatch to existing session vs string helpers which were renamed `*WithRest` and accept the rest-list directly instead of reading `mc.Arg(1)`.

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

### 6.3 MVP coverage (PR-2) — **shipped**

PR-2 landed in commits `f8d9c6e` (SSAFunctionRef), `3b389ea` (mapper ref field), `3664cfe` (findValueByName — commit-history collision with parallel goastlint work, see commit note), `06513dc` (primitive skeleton), `c14617a` (narrowing algorithm), `3fe3424` (Scheme integration tests).

**Shipped — `narrow` confidence (concrete type recovered):**
- `Alloc`, `MakeClosure`, `MakeMap`, `MakeSlice`, `MakeChan` — direct type
- `MakeInterface` — recurses on wrapped value
- `ChangeType`, `ChangeInterface`, `Convert`, `MultiConvert`, `SliceToArrayPointer` — recurses on operand
- `TypeAssert` — records `AssertedType`
- `Phi` — unions over edges
- `Extract` — records tuple-at-index element type, falls back to tuple-walk for interface elements
- `Call` with concrete return type — direct record
- `Call` with interface return, static callee — inter-procedural recursion over callee's `Return` instructions
- `BinOp`, `UnOp` (non-interface), `Field`, `FieldAddr`, `Index`, `IndexAddr`, `Lookup` (non-interface), `Slice`, `Range`, `Next`, typed `Const`, `Function` — direct type
- Cycle detection via `(*ssa.Function, ssa.Value)` visited set keyed by pointer identity

**Shipped — `widened` confidence with reason tag:**
- `Parameter` → `parameter`
- `FreeVar` → `free-var`
- `Global` → `global-load`
- `UnOp`/`Field`/`FieldAddr`/`Lookup` with interface result → `field-load`
- `Const` where `IsNil()` → `nil-constant`
- `Call` with `IsInvoke()` → `interface-method-dispatch`
- `Builtin` / non-static callee → `unresolvable-callee`
- Unrecognized instruction → `unhandled`
- Cycle (revisit of value in same walk) → `cycle`

**Shipped — `no-paths`:**
- Value name not found in function → `value-not-found`
- Empty callee return set (function with no `Return` instructions reachable)
- Merge of only `no-paths` results

`mergeResults` computes the union of types + reasons with deterministic ordering (insertion-sort for reproducibility). Overall confidence is `widened` if any input widened, `no-paths` if all inputs are no-paths, else `narrow`.

**Deferred to follow-up iterations if the `widened`-with-`parameter`/`field-load` rate on the 428-prim run exceeds the §8 kill threshold (>30% Helper-widened):**
- Call-graph-context parameter narrowing (look up callers, narrow parameter by union of call-site arg types).
- Store-site tracking for fields/globals (walk Stores-to-field, narrow to union of stored values).
- Slice / map element type reasoning via `MakeSlice.Elem` + `Store`/`Index` pairs.
- Type-switch arm narrowing (each arm establishes the type within its block).
- Reflect-based value production (`reflect.New`, `reflect.Zero`).

### 6.4 Type classification layer

`go-ssa-narrow` returns Go type strings (e.g., `"*github.com/aalpar/wile/values.Integer"`). Mapping those to wile's `values.ValueType.Name()` strings (e.g., `"integer"`) is axis-b-specific and lives in the axis-b script — not in this primitive. Keeps `go-ssa-narrow` domain-neutral.

### 6.5 PR-2 tests — **shipped**

Rather than per-fixture Go files under `testdata/`, PR-2 used inline Go source via the existing `buildSSAFromSource` helper (`goastssa/mapper_test.go`). This matches the pattern already used by every other mapper test and avoids a `testdata/narrow/` go.mod. The tests live in `goastssa/narrow_test.go`.

Go-side coverage (all passing):

| Test | Case | Expected |
|---|---|---|
| `TestNarrowDirectAlloc` | `return &Bar{}` | `{*testpkg.Bar}`, `narrow` |
| `TestNarrowBinOpReturn` | `return a + b` (int) | `{int}`, `narrow` |
| `TestNarrowPhi` | if/else assign to `v`, return v | `{*testpkg.Bar, *testpkg.Baz}`, `narrow` |
| `TestNarrowTypeAssert` | `return x.(*Bar)` | `{*testpkg.Bar}`, `narrow` |
| `TestNarrowExtractTuple` | `v, _ := m["k"]` | `{int}`, `narrow` |
| `TestNarrowInterProceduralStaticCall` | `return helper()` where helper returns `*Bar` | `{*testpkg.Bar}`, `narrow` |
| `TestNarrowParameterWidens` | `return x` (parameter) | `widened`, `{parameter}` |
| `TestNarrowInvokeWidens` | `s.String()` on interface | `widened`, `{interface-method-dispatch}` |
| `TestNarrowGlobalLoadWidens` | `return G` (global interface) | `widened` |
| `TestNarrowCycleDetected` | Mutually recursive A↔B | `widened` (cycle) or `narrow` if inlined |
| `TestNarrowMergeResultsEmpty` | Merge no inputs | `no-paths` |
| `TestNarrowMergeResultsWidenedWins` | Merge narrow + widened | `widened`, preserved reason |
| `TestNarrowMergeResultsAllNoPaths` | Merge no-paths inputs | `no-paths` |
| `TestNarrowMergeResultsDeduplicatesTypes` | Union type sets | sorted, deduplicated |

Scheme-level integration in `cmd/wile-goast/narrow_integration_test.go`:

| Test | Strategy | Assertion |
|---|---|---|
| `TestGoSSANarrowPrimitiveRegistered` | Call with nonexistent value name | `narrow-result` alist, confidence `no-paths` |
| `TestGoSSANarrowParameterWidens` | Narrow first function's first parameter in `wile-goast/goast` | confidence `widened`, reason `parameter` |
| `TestGoSSANarrowConcreteAlloc` | Walk blocks to find first `ssa-alloc`, narrow it | confidence `narrow`, non-empty type list |

The Scheme tests avoid hardcoded SSA value names (which drift across toolchain versions) by walking SSA in Scheme with `(nf)` / `(tag?)` to find the value dynamically.

---

## 7. PR-3 — axis-b analyzer script — **shipped, then moved to wile**

PR-3 initially landed in wile-goast (commits `293e6d5` through `c21dc6e`, 8 commits). **The script was subsequently moved to the wile repo** because its content is wile-specific (sink methods on wile's `CallContext`, Go→wile value-type mapping, declared-return-type comparison against wile's `TypeConstraint` vocabulary), and keeping it in wile-goast inverted the "generic tool, specific users" architecture: wile-goast is a general Go-static-analysis tool, wile is the specific consumer.

### 7.1 Script location and invocation

**Current location (after move):** `wile/audit/wile-axis-b.scm` in the wile repo.

**Invocation (from wile repo root):** `wile-goast -f audit/wile-axis-b.scm`.

This uses the already-supported `-f <path>` CLI flag rather than `--run` (which requires `go:embed`-ed scripts). The script consumes the generic `go-ssa-build` / `go-ssa-narrow` primitives that remain in wile-goast.

### 7.2 Why the script moved

The script hardcodes four wile-specific concerns:

1. Sink method names — `SetValue` / `SetValues` on wile's `CallContext` interface.
2. Go→wile type mapping — 30+ entries mapping `*values.Integer` → `"integer"` etc.
3. Bucket semantics — tuned for wile's `TypeConstraint` vocabulary decision.
4. Default output paths — point into the wile repo's `plans/` directory.

None of these are meaningful to other wile-goast users. Keeping the script in wile-goast made wile-goast know wile's internal structure; moving it inverts that. wile-goast now contains only generic SSA-analysis primitives (plus their unit tests and a few demo scripts); wile owns the wile-specific analyzer that invokes those primitives.

The wile-goast smoke test `goastssa/axis_b_smoke_test.go` was deleted along with the script and moved to `wile/audit_axis_b_test.go`.

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

### 7.5 Sink enumeration — **complete**

Research done during PR-3 planning. The `CallContext` interface (`wile/machine/call_context.go`) exposes exactly two result-writing methods: `SetValue(v)` and `SetValues(vs...)`. Primitives call them either via the interface (invoke-mode, most common) or on `*MachineContext` directly (static call, when the primitive type-asserts for full VM access). Four SSA-visible names total, well under the 15-name kill-criterion threshold.

```scheme
(define sink-method-names '("SetValue" "SetValues"))
```

Matched against both invoke-mode `method` field and static-call `func` field in `ssa-call` nodes.

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

### 7.8 Smoke test — **shipped**

`goastssa/axis_b_smoke_test.go` runs the script via `go run ./cmd/wile-goast --run wile-axis-b` against a 4-entry fixture manifest (`goastssa/testdata/axis-b-fixture-manifest.scm`): `cons`, `null?`, `length`, `car`. The test asserts each primitive appears in the generated raw output and that the stdout summary reports 4 primitives. It does NOT pin exact bucket assignments — those are subject to PR-2' narrowing improvements; the test is a regression guard against "script crashes / output format drifts", not a pin against specific buckets.

Actual bucket assignments from the first full run against `github.com/aalpar/wile/...`:

| Primitive | Narrowed (wile types) | Confidence | Bucket |
|---|---|---|---|
| `cons` | `{pair}` | narrow | Single |
| `null?` | `{boolean}` | narrow | Single |
| `length` | `{integer}` | narrow | Single |
| `car` | `{}` | widened | Helper-widened (reason: `interface-method-dispatch`) |

`car` widens because `pair.Car()` is an interface method call in Go (the `Tuple` interface) — not a field access. Fixing this would require interface-method dispatch resolution in PR-2' (narrow the receiver to a concrete type, then union over concrete method implementations).

### 7.9 First-run findings (historical — triggered kill criterion)

Full run against `wile/...` (3168 SSA functions indexed, all 475 manifest entries analyzed):

| Bucket | Count | % of resolved |
|---|---|---|
| Single | 154 | 44% |
| Maybe | 12 | 3% |
| Narrow-union | 6 | 2% |
| Broad-union | 2 | <1% |
| Helper-widened | 125 | **36%** |
| Side-effecting | 52 | 15% |
| Unresolved | 124 | (n/a — 46 binding-only + 57 init-closure + 21 other) |

**Kill criterion triggered**: Helper-widened at 36% exceeded the 30% threshold from §9. The inventory did NOT land in the wile repo (Phase 3.C) until PR-2' extended narrowing.

Reason-tag distribution for widened primitives (109 total):

| Reason | Count | Implied PR-2' work |
|---|---|---|
| `pointer-load` | 109 | **Store-site tracking for locals**: trace `*Alloc` through `Store`/`UnOp` to recover the stored value type. Dominant hitter. |
| `interface-method-dispatch` | 18 | Receiver-type-aware method resolution: narrow the receiver, union over concrete method implementations. |
| `nil-constant` | 8 | Context-aware handling: nil as error signal vs nil as Value. |
| `cycle` | 7 | Review cycle detection — expected small. |
| `interface-result` | 5 | Investigate — possibly generics or embedded interfaces. |
| `parameter` | 3 | Call-graph-context parameter narrowing. |

The `pointer-load` dominance (>75% of widened reasons) pointed clearly at the highest-value PR-2' extension: any primitive writing a local `var result values.Value` and setting from conditional branches hits this pattern, and the fix is tracking Alloc→Store→UnOp triples.

Unresolved breakdown:
- 46 binding-only primitives (expected — Impl is nil, go-function is empty string)
- 57 init-closure primitives (e.g., `init.MakeTypePredicate.func36`) — `runtime.FuncForPC` reports a closure name that doesn't round-trip through `ssa.Function.String()`. The manifest generator (PR Phase 3.A) has a gap here: SSA doesn't expose these closures as top-level functions.
- 21 other — mix of missing-package primitives and suspected minor matching bugs worth investigating before the inventory lands.

### 7.10 PR-2' progression and kill-criterion clearance

Two narrowing extensions shipped after the first run:

1. **f6bb925** — `narrowFromAllocStores`: walks `alloc.Parent()` blocks for Store instructions whose Addr is a local Alloc; recovers stored-value types. Also split the opaque `pointer-load` reason into five specific tags (`global-load`, `field-deref-load`, `slice-deref-load`, residual `pointer-load`, Alloc-narrowing path).

   Effect on the bucket distribution was *diagnostic more than corrective*: Helper-widened stayed at 125 (34.7% of resolved), but the reason-tag split revealed that 105 of the 109 "pointer-load" instances were actually **global-load** — loading package-level singletons like `values.Void`. Alloc was not the bottleneck; globals were.

2. **c7729bd** — `narrowFromGlobalInit`: walks `g.Pkg.Func("init")` for Store instructions whose Addr is the global, unions narrowed types. Strategy is init-only — runtime reassignments from non-init functions are NOT tracked. SSA lowers package-level var declarations (`var G Iface = &Concrete{}`) into init-function Stores, so this naturally handles the dominant Go idiom without AST-level work.

   Widening reasons introduced: `global-no-pkg`, `global-no-init`, `global-no-stores`.

Second-run results (same `wile/...` target, 3179 SSA functions indexed, 500 primitives analyzed — 475 → 500 reflects wile's v1.x growth between runs):

| Bucket | Before (first run) | After (intermediate, f6bb925) | After (final, c7729bd) |
|---|---|---|---|
| Single | 154 | 162 | **215** (+61 vs first) |
| Maybe | 12 | 12 | **32** (+20) |
| Narrow-union | 6 | 7 | **22** (+16) |
| Broad-union | 2 | 2 | 3 |
| **Helper-widened** | **125 (36%)** | **125 (34.7%)** | **36 (10.0%)** |
| Side-effecting | 52 | 52 | 52 |
| Unresolved | 124 | 140 | 140 |

**Kill criterion cleared**: 36 / 360 = **10.0%** Helper-widened, well under the 30% threshold. PR-3 inventory is unblocked for Phase 3.C landing in the wile repo.

Final reason-tag distribution (36 widened primitives, 54 reason tags):

| Reason | Count | Note |
|---|---|---|
| `interface-method-dispatch` | 18 | Largest remaining bucket; addressing would require receiver-type narrowing + method-set resolution. |
| `field-deref-load` | 10 | Struct-field interface loads — analogous to globals but with per-instance stores. |
| `nil-constant` | 8 | Expected; nil as error signal vs nil as Value is context-dependent. |
| `cycle` | 7 | Expected small; cycle detection working as designed. |
| `interface-result` | 5 | Possibly generics or embedded interfaces. |
| `parameter` | 3 | Call-graph-context parameter narrowing would address. |
| `slice-deref-load` | 1 | Rare. |
| `narrow-error` | 1 | Internal error — worth investigating before Phase 3.C. |
| `unresolvable-callee` | 1 | Builtin/closure dispatch. |

Zero regressions: no primitive that previously narrowed moved into widened. The 105 recovered global-load primitives redistributed as Single: +53, Maybe: +20, Narrow-union: +15 vs. the intermediate baseline — consistent with the Go idiom "package-scoped singleton returned from primitive" being either exactly-one-concrete-type (Single) or interface-global-holding-2-to-3-types (Maybe / Narrow-union).

### 7.11 PR-2''' — invoke dispatch and concrete-parameter narrowing

Third narrowing extension shipped 2026-04-20 (commit `fd609ce`). Three coordinated changes in `goastssa/narrow.go`:

1. **`narrowFromInvokeDispatch`**: enumerate every concrete type in the program implementing the receiver's interface, resolve the specific method on each (value and pointer receiver forms), and union over each implementation's return-path narrowings. Uses `prog.AllPackages()` + `types.Implements` + `prog.MethodSets.MethodSet` + `prog.MethodValue`. Synthetic wrapper functions skipped.

2. **Concrete-return short-circuit** in `narrowFromCall`: calls whose declared return type is already concrete (static or invoke) narrow from type alone — the callee internals cannot refine what's already tight. Short-circuits the (potentially expensive) all-implementors enumeration for invokes returning concrete types.

3. **Concrete-parameter narrowing** in `narrowWalk`'s `*ssa.Parameter` case: parameters with concrete types narrow from type alone; only interface-typed parameters widen with `"parameter"`. The dominant beneficiary is method receivers — `func (c *Cat) Get() { return c }` now narrows to `*Cat` even though `c` is a Parameter.

Third-run results against `wile/...` (same 3180 SSA functions, 500 primitives):

| Bucket | Before PR-2''' | After PR-2''' | Δ |
|---|---|---|---|
| Single | 215 | 217 | +2 |
| Maybe | 32 | 32 | 0 |
| Narrow-union | 22 | 26 | +4 |
| Broad-union | 3 | 3 | 0 |
| **Helper-widened** | **36 (10.0%)** | **30 (8.3%)** | **-6** |
| Side-effecting | 52 | 52 | 0 |
| Unresolved | 140 | 140 | 0 |

Reason-tag shifts (30 widened primitives, 55 reason tags total):

| Reason | Before | After | Note |
|---|---|---|---|
| `interface-method-dispatch` | 18 | **0** | Bucket eliminated. |
| `field-deref-load` | 10 | 15 | +5 — previously masked by dispatch widening on the same primitive. |
| `slice-deref-load` | 1 | 8 | +7 — same masking effect. |
| `cycle` | 7 | 8 | +1 — one new cycle in the invoke recursion path. |
| `callee-no-returns` | 0 | 7 | NEW — invoke dispatch reaching methods whose bodies panic / never return. |
| `nil-constant` | 8 | 8 | Unchanged. |
| `interface-result` | 5 | 5 | Unchanged. |
| `parameter` | 3 | 3 | Unchanged (all remaining parameter reasons are interface-typed). |
| `narrow-error` | 1 | 1 | Unchanged. |
| `unresolvable-callee` | 1 | 0 | Cleared (builtin/closure cases now resolve). |

**Onion-peeling observation**: removing `interface-method-dispatch` surfaced `field-deref-load` and `slice-deref-load` reasons that were previously masked when those primitives had dispatch *also* in their reason list. The Helper-widened count dropped less than the dispatch-reason count because 12 primitives had multi-reason widenings. This is diagnostic for the next PR-2'''' target: `field-deref-load` (15) and `slice-deref-load` (8) together cover 23 reasons across the remaining 30 widened primitives. Their narrowing would use a `narrowFromFieldStores` / `narrowFromSliceStores` pattern similar to `narrowFromGlobalInit`, but store sites can come from any function in the program rather than the single Init, making it more expensive.

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
