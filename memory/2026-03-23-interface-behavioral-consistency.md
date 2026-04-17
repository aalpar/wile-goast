# Interface Behavioral Consistency

**Date:** 2026-03-23
**Status:** Implemented

## Problem

Go interfaces define contracts structurally (method signatures), but not
behaviorally (what implementations should do). When multiple types implement the
same interface, behavioral inconsistencies — one implementation handles an edge
case, another doesn't — are latent bugs that surface only when swapping
implementations at runtime.

No existing Go tool checks this:
- **gopls** finds implementations but doesn't compare their behavior
- **golangci-lint** checks each function in isolation
- **errcheck/nilness** are intraprocedural

The question "do all implementations of this interface behave consistently?" requires
cross-layer analysis: types (interface satisfaction) + AST/SSA (body comparison)
+ statistical consistency (belief DSL).

## Scope

Project-specific interfaces where both the interface definition and its
implementors are in the same package subtree. Stdlib interfaces (`io.Reader`,
`error`) are out of scope — their implementors are scattered across the module
graph, requiring a different loading strategy.

## Design

### New Go Primitive: `go-interface-implementors`

Single new primitive in the `goast` package.

**Signature:**

```scheme
(go-interface-implementors <interface-name> <package-pattern>)
```

- `interface-name` — short name (`"Storage"`) or qualified (`"go.etcd.io/raft/v3.Storage"`)
- `package-pattern` — glob pattern, same syntax as `go-typecheck-package` and `run-beliefs`

**Return value:** Tagged alist:

```scheme
(interface-info
  (name . "Storage")
  (pkg . "go.etcd.io/raft/v3")
  (methods . ("Entries" "Term" "LastIndex" "FirstIndex" "Snapshot"))
  (implementors .
    (((type . "MemoryStorage") (pkg . "go.etcd.io/raft/v3"))
     ((type . "diskStorage")   (pkg . "go.etcd.io/raft/v3/internal")))))
```

**Disambiguation:** If multiple interfaces match the short name across loaded
packages, return an error listing the candidates with their package paths.

**Loading:** Accepts a package pattern string, loads via `packages.Load` with
`NeedTypes`. No `NeedSyntax` — the primitive returns type names and package
paths, not AST nodes. Pure `go/types` query.

**Interface satisfaction check:** Must check both `T` and `*T` against the
interface. A type with pointer receivers satisfies the interface only via `*T`.

**Implementation location:** `goast/prim_goast.go`, registered in
`goast/register.go`. No new sub-extension — interface satisfaction is a
`go/types` operation, not an additional analysis pass.

**Registration:**

```go
registry.NewPrimitive("go-interface-implementors", primInterfaceImplementors, 2, 2)
```

### Belief DSL Integration

Two new site selectors in `cmd/wile-goast/lib/wile/goast/belief.scm`.

#### `(implementors-of <interface-name>)`

Returns all func-decls whose receiver type implements the named interface.

```scheme
;; "Types implementing Storage should call Close"
(define-belief "storage-has-close"
  (sites (implementors-of "Storage"))
  (expect (contains-call "Close"))
  (threshold 0.60 3))
```

Mechanics: calls `go-interface-implementors`, gets implementor type names,
filters `all-func-decls` by receiver match (reusing existing `has-receiver`
substring logic).

#### `(interface-methods <interface-name> [method-name])`

Returns func-decls that implement interface methods, optionally narrowed to a
specific method.

```scheme
;; "Every Entries() implementation handles ErrCompacted"
(define-belief "entries-handles-compacted"
  (sites (interface-methods "Storage" "Entries"))
  (expect (contains-call "ErrCompacted" "ErrUnavailable"))
  (threshold 0.80 2))
```

Without the second argument: returns all interface methods across all
implementors (`MemoryStorage.Entries`, `diskStorage.Entries`,
`MemoryStorage.Term`, `diskStorage.Term`, etc.).

With the second argument: narrows to one method name — the primary form for
behavioral consistency analysis ("compare how different types implement the same
method").

#### Caching

The belief context gets a new lazy slot keyed by `(interface-name . pkg-pattern)`.
Multiple beliefs targeting the same interface share the cached result. Follows
the existing pattern for `field-index` and `callgraph` lazy slots.

#### Display Names

Deviation reports should be qualified by implementor type:
`MemoryStorage.Entries` not just `Entries`.

### Testing

Test file in `goast/` defining:
- A small interface with 2-3 methods
- Two types implementing it (one complete, one missing a behavior)
- One type not implementing it

Assert: the primitive returns the correct implementors and method list.
Assert: belief DSL correctly selects interface methods and reports deviations.

## Demo Script

Primary target: etcd's `raft.Storage` interface.

```scheme
;;; raft-storage-consistency.scm
;;;
;;; Do all implementations of raft.Storage handle edge cases consistently?
;;;
;;; Usage: wile-goast -f raft-storage-consistency.scm

(import (wile goast belief))

(define target "go.etcd.io/raft/v3/...")

;; Every Entries() implementation should guard against compacted log
(define-belief "entries-compaction-guard"
  (sites (interface-methods "Storage" "Entries"))
  (expect (contains-call "ErrCompacted" "ErrUnavailable"))
  (threshold 0.80 2))

;; Every Snapshot() should handle ErrSnapshotTemporarilyUnavailable
(define-belief "snapshot-temp-unavail"
  (sites (interface-methods "Storage" "Snapshot"))
  (expect (contains-call "ErrSnapshotTemporarilyUnavailable"))
  (threshold 0.60 2))

;; Every Term() implementation should handle out-of-range indices
(define-belief "term-bounds-check"
  (sites (interface-methods "Storage" "Term"))
  (expect (contains-call "ErrCompacted" "ErrUnavailable"))
  (threshold 0.80 2))

;; Broader: do Storage implementors consistently close/release resources?
(define-belief "storage-resource-cleanup"
  (sites (implementors-of "Storage"))
  (expect (contains-call "Close" "Release" "Unlock"))
  (threshold 0.50 3))

(run-beliefs target)
```

Alternative demo targets if etcd doesn't produce interesting findings:
- kubernetes `container.Runtime` interface
- hashicorp/consul `Backend` interface
- Any project with a Handler/Store/Provider pattern

## Summary

| Component | What | Where |
|-----------|------|-------|
| Primitive | `go-interface-implementors` | `goast/prim_goast.go` |
| Registration | 2 required args, no optionals | `goast/register.go` |
| Belief selector | `(implementors-of "I")` | `belief.scm` |
| Belief selector | `(interface-methods "I" [method])` | `belief.scm` |
| Caching | Lazy slot keyed by `(iface . pattern)` | `belief.scm` |
| Test | Interface + 2 implementors + 1 non-implementor | `goast/prim_goast_test.go` |
| Demo | `raft-storage-consistency.scm` | `examples/etcd/` |

## Open Questions

1. **Embedded interfaces.** If `Storage` embeds `io.Closer`, should the method
   list include `Close`? Probably yes — `iface.Method(i)` on a `types.Interface`
   already includes promoted methods. Verify during implementation.

2. **Generic interfaces.** Go 1.18+ interfaces with type constraints
   (`interface { ~int | ~string }`) are not method-set interfaces. The primitive
   should skip these or return an error. Check `iface.IsMethodSet()` (Go 1.23+)
   or fall back to `iface.NumMethods() > 0`.

3. **Threshold tuning.** For interfaces with only 2-3 implementors, the belief
   DSL's minimum site count may need to be 2 rather than the typical 3-5. The
   demo scripts use `2` for this reason.
