# SSA Field Index Primitive

**Status**: Planned
**Foundation**: [BELIEF-DSL.md](BELIEF-DSL.md) — belief checkers that consume SSA field access data
**Dependencies**: `goastssa` package, existing SSA build infrastructure

## Problem

SSA-based belief predicates (`stores-to-fields`, `co-mutated`) are the performance bottleneck in belief evaluation. For the kubelet (`k8s.io/kubernetes/pkg/kubelet/...`), evaluating a single `stores-to-fields` belief takes 4 minutes — compared to 4 seconds for package loading and SSA construction.

The bottleneck is in Scheme-side tree walking. `stores-to-fields` calls `collect-field-addrs` and `collect-stores` on each of 3766 SSA functions. Each call runs `walk` — a recursive traversal of the SSA instruction tree represented as nested s-expression alists. Two walks per function, 3766 functions, interpreted Scheme recursion: 4 minutes.

The irony: only ~5 of 3766 functions actually store to the target fields. But the predicate must walk every function to discover this, because the SSA data is a tree that must be traversed to extract instruction-level facts.

### Attempted Approaches

**SRFI-18 parallelism**: Wile supports SRFI-18 threads backed by goroutines. Splitting the `filter-map` across 8 threads produces data races in wile's evaluator — concurrent Scheme evaluation on independent data crashes with pair corruption. A global mutex around each predicate call works (serialized evaluation) but doesn't improve the fundamental 4-minute traversal time.

**Mutex-serialized threading**: Correct results in 6.4 seconds — a 40x improvement from Go-side preloading benefits, but the Scheme traversal is still the bottleneck (just amortized by goroutine scheduling).

### Root Cause

The performance gap exists because Go SSA instructions are flat arrays of typed structs (`[]ssa.Instruction`), but the s-expression representation converts them into deeply nested tagged alists. Iterating a flat array in Go takes microseconds. Recursively walking the same data as s-expressions in interpreted Scheme takes minutes.

## Design

### Approach: `go-ssa-field-index` Primitive

A new Go primitive in `goastssa` that iterates SSA instructions natively and returns a pre-correlated field access index. Scheme filters the flat index instead of walking trees.

The division: Go traverses (tight loop over `[]ssa.Instruction`), Scheme filters (fast list operations on flat results).

### Primitive Signature

```scheme
(go-ssa-field-index pattern)
;; pattern: go-list-compatible package pattern
;; Returns: list of ssa-field-summary nodes
```

### Return Shape

One entry per function that accesses at least one struct field. Functions with no field accesses are omitted (3761 of 3766 kubelet functions filtered out at the Go level):

```scheme
((ssa-field-summary
   (func   . "convertToKubeContainerStatus")
   (pkg    . "k8s.io/kubernetes/pkg/kubelet/kuberuntime")
   (fields . ((struct     . "Status")
              (struct-pkg . "k8s.io/kubernetes/pkg/kubelet/container")
              (field      . "Reason")
              (recv       . "t0")
              (mode       . write))
             ((struct     . "Status")
              (struct-pkg . "k8s.io/kubernetes/pkg/kubelet/container")
              (field      . "Message")
              (recv       . "t0")
              (mode       . write))
             ...))
 ...)
```

Fields per access entry:

| Field | Type | Source | Description |
|-------|------|--------|-------------|
| `struct` | string | Go type system | Short struct type name |
| `struct-pkg` | string | Go type system | Import path of package defining the struct |
| `field` | string | Go type system | Field name |
| `recv` | string | SSA register | Receiver register name (for grouping by instance) |
| `mode` | symbol | FieldAddr+Store correlation | `read` or `write` |

### Key Design Decision: Struct Type From Go Types, Not AST

The current Scheme code runs `struct-field-names` to walk the AST and find `type Status struct { ... }` — a separate tree traversal just to discover the struct's field set for receiver disambiguation.

The Go primitive eliminates this entirely. `ssa.FieldAddr` already knows the struct type via Go's type system: `typesDeref(v.X.Type()).(*types.Named)` gives the type name and its defining package. No AST walk needed.

`struct-field-names` is retained as a utility — it serves AST-level queries independent of SSA. But `stores-to-fields` and `co-mutated` no longer call it.

### Go Implementation

Lives in `goastssa/prim_ssa.go` alongside `PrimGoSSABuild`. Reuses existing helpers: `typesDeref`, `fieldNameAt`, `valName`.

Algorithm (one pass per function):

```
for each function in SSA program:
    fieldAddrs = {}   // register name → (struct, struct-pkg, field, recv)
    storeTargets = {} // set of addr register names
    directReads = []  // Field instruction reads

    for each block in function:
        for each instruction in block:
            switch type:
                FieldAddr: fieldAddrs[v.Name()] = extract(v)
                Store:     storeTargets.add(v.Addr.Name())
                Field:     directReads.append(extract(v))

    // Correlate
    accesses = []
    for name, info in fieldAddrs:
        mode = "write" if name in storeTargets else "read"
        accesses.append(info + mode)
    accesses.append(directReads with mode="read")

    if len(accesses) > 0:
        emit ssa-field-summary(func, pkg, accesses)
```

Struct type extraction from `ssa.FieldAddr`:

```go
structType := typesDeref(v.X.Type())
named, ok := structType.(*types.Named)
if ok {
    structName = named.Obj().Name()       // "Status"
    structPkg  = named.Obj().Pkg().Path() // "k8s.io/.../container"
}
```

These helpers already exist in the mapper (`typesDeref` at `mapper.go:328`, `fieldNameAt` at `mapper.go:337`).

### Belief DSL Integration

The context gains a new lazy-loaded slot:

```scheme
(define (make-context target)
  (list (cons 'target target)
        (cons 'pkgs #f)
        (cons 'ssa #f)
        (cons 'ssa-index #f)
        (cons 'field-index #f)    ;; new
        (cons 'callgraph #f)
        (cons 'results '())))

(define (ctx-field-index ctx)
  (or (ctx-ref ctx 'field-index)
      (let ((idx (go-ssa-field-index (ctx-target ctx))))
        (ctx-set! ctx 'field-index idx)
        idx)))
```

`stores-to-fields` becomes a flat filter:

```scheme
(define (stores-to-fields struct-name . field-names)
  (lambda (fn ctx)
    (let* ((fname (nf fn 'name))
           (pkg-path (nf fn 'pkg-path))
           (summary (find-summary (ctx-field-index ctx) pkg-path fname)))
      (and summary
           (let ((writes (writes-for-struct summary struct-name)))
             (all-present? field-names writes))))))
```

`co-mutated` simplifies similarly:

```scheme
(define (co-mutated . field-names)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (summary (find-summary (ctx-field-index ctx) pkg-path fname)))
      (if (not summary) 'partial
        (let ((writes (writes-for-struct summary #f)))
          (if (all-present? field-names writes)
            'co-mutated
            'partial))))))
```

Helper functions (`find-summary`, `writes-for-struct`, `all-present?`) are flat list operations — `assoc`, `filter-map`, `member?` on short pre-correlated lists.

### What Changes

**New Go code:**
- `goastssa/prim_ssa.go` — `PrimGoSSAFieldIndex` (~80 lines)
- `goastssa/register.go` — register the new primitive
- `goastssa/prim_ssa_test.go` — test

**Modified Scheme:**
- `belief.scm` — add `ctx-field-index`, rewrite `stores-to-fields` and `co-mutated` to filter the index, add `find-summary`/`writes-for-struct`/`all-present?` helpers
- `belief.sld` — export `ctx-field-index`

**Unchanged:**
- `go-ssa-build` — still returns full SSA trees for instruction-level consumers
- `struct-field-names` — retained as AST-level utility
- `checked-before-use` — still uses SSA tree (operand tracking, not field access)
- `contains-call`, `paired-with`, `ordered` — AST/CFG-based, unaffected
- All existing scripts using `go-ssa-build` directly

**Dead after migration (remove):**
- `collect-field-addrs` / `collect-stores` / `stored-fields-in-func` — replaced by index lookup
- `stores-to-fields` mutex cache — no longer needed
- `parallel-filter-map` — no longer needed (the hot loop is in Go)

### Performance

| Phase | Before | After |
|-------|--------|-------|
| Package loading | 1.5s | 1.5s |
| SSA build | 2.8s | 2.8s |
| Field index build | — | ~0.1s |
| `stores-to-fields` predicate | 4m 12s | ~0.01s |
| **Total** | **~4m 17s** | **~4.5s** |

### Consumers

One primitive, six consumers:

| Consumer | Operation on Index |
|----------|-------------------|
| `stores-to-fields` | Filter for functions with writes to target fields |
| `co-mutated` | Check if writes cover all target fields |
| dead-field-detect.scm | Union reads/writes across all functions |
| state-trace-full.scm | Group by receiver, check mutation independence |
| consistency-comutation.scm | Same as stores-to-fields + co-mutated |
| (future) parameter essentiality | Check whether parameter fields are read |

## Non-Goals

- Replacing `go-ssa-build` — the full SSA tree remains for `checked-before-use` and custom analysis
- Index caching across `run-beliefs` calls — per-call context is sufficient
- Migrating `checked-before-use` — separate concern (operand tracking, not field access)
- Query language for SSA patterns — the flat index covers all known field-access use cases
- SRFI-18 parallel evaluation — blocked by wile evaluator thread-safety; filed separately

## Testing

1. **Primitive unit test** — build field index for `goastssa` test package, verify known functions appear with correct struct/field/mode entries.
2. **Round-trip test** — compare `go-ssa-field-index` results against `go-ssa-build` + Scheme-side `collect-field-addrs`/`collect-stores` for the same package. Results must match.
3. **Belief integration** — run `stores-to-fields`/`co-mutated` beliefs against `goast` package using the index path, verify identical results to the old tree-walking path.
4. **Kubelet validation** — run `k8s-kubelet-comutation.scm` with the new primitive, verify same 4/5 co-mutated result with `convertToKubeContainerStatus` deviation.
