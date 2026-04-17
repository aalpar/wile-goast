# Package Splitting via Import Signature Analysis

Analyze a Go package's functions by their external dependency profiles to discover
natural package boundaries. The tool answers: **"If this package were split into
two packages, where is the cut that minimizes cross-boundary coupling?"**

**Status:** Design proposed.

## Origin

The methodology was discovered empirically during a refactoring session on the
Wile Scheme interpreter's `machine/` package (102 files, ~20K LOC). The session
followed this sequence:

1. **Import signature extraction.** For each function in `machine/`, compute the
   set of external packages it imports. Group functions by identical import
   signatures.

2. **Noise filtering.** Observe that some packages (e.g., `values/`) appear in
   nearly every function — they carry no discriminating information. The
   meaningful dependencies are the ones that appear in *some* functions but not
   others.

3. **Semantic sub-clustering.** Functions sharing the same high-signal dependency
   may use different *slices* of that dependency's API. In the `machine/` case,
   some functions use `environment.EnvironmentFrame` as live mutable state
   (frame lifecycle), while others use `environment.Binding` and
   `environment.LocalIndex` as read-only compiled metadata (compiled indexing).
   Same import, different API surface, different semantic group.

4. **Min-cut identification.** Between the two groups, count the functions that
   bridge both — that's the coupling cost of the split. In the `machine/` case:
   3 resolve functions + 2 operation Apply methods.

5. **Feasibility verification.** Check that the proposed split doesn't create
   Go import cycles.

The result: 4 files (~730 LOC) moved from `machine/` to `machine/compilation/`,
with a well-defined interface boundary (3 accessor methods on MachineContext).

## Theoretical Foundations

### Import Signatures as a Formal Context

The analysis is an instance of **Formal Concept Analysis** (Ganter & Wille, 1999)
applied to package structure, following Lindig & Snelting (1997):

- **Objects (G):** Functions and methods in the target package.
- **Attributes (M):** External packages referenced.
- **Incidence (I):** Function f references package p iff f's body (or signature)
  contains an identifier resolved by `go/types` to package p.

Each **concept** in the resulting lattice is a maximal set of functions sharing a
maximal set of external dependencies. The lattice reveals which function groups
are dependency-equivalent and where the boundaries naturally fall.

### IDF Weighting: Filtering Noise Dependencies

Not all dependencies are equally informative. A dependency that appears in every
function (e.g., `values/` in a Scheme interpreter) has zero discriminating power —
it cannot distinguish one group from another.

This is the **TF-IDF insight** from information retrieval (Salton & Buckley, 1988):

```
IDF(pkg) = log(N / df(pkg))

where:
  N     = total number of functions in the package
  df(p) = number of functions that reference package p
```

High-IDF packages (referenced by few functions) are the structural dependencies
that define real clusters. Low-IDF packages (referenced by most functions) are
noise.

**Threshold heuristic:** A package referenced by >70% of functions has IDF < 0.36
and should be excluded from clustering. This threshold is configurable.

### API Surface Refinement

Two functions may import the same package but use disjoint API surfaces. This
sub-clustering adds a second dimension to the formal context:

- **Refined attributes (M'):** (package, object-name) pairs — e.g.,
  `(environment, EnvironmentFrame)` vs `(environment, LocalIndex)`.

The refined lattice has more concepts and finer distinctions. In practice, this
turns a single cluster (all environment-dependent functions) into two or more
sub-clusters (frame lifecycle vs compiled indexing).

### Properties That Enable a Clean Split

A package split is feasible when four conditions hold:

**P1: Bimodal dependency profile.** The functions partition into two groups
with distinct high-IDF dependency sets. In lattice terms: the lattice has two
or more incomparable concepts with large extents (many functions) and disjoint
intents (different dependency sets). If every function depends on everything,
there's no split.

**P2: Low cross-boundary coupling.** The number of functions that participate
in both groups is small relative to the total. This is the **min-cut** of the
bipartite graph between the two clusters. A good split has a cut ratio
(cut edges / total edges) below ~15%.

**P3: Semantic coherence.** The two clusters correspond to recognizable concerns
— not random partitions. This is verified by the API surface analysis: each
cluster uses a coherent slice of the shared dependency's API. Frame lifecycle
functions call constructors and mutators; compiled indexing functions call
accessors and encoders.

**P4: Acyclic dependency graph.** The proposed packages must not form an import
cycle. In Go, this means: if package A imports package B, then B cannot import A.
Given a split of package P into P1 and P2:
- P1 → P2 is allowed (or P2 → P1, but not both)
- Existing packages that import P must be checked: do they need types from
  both P1 and P2?

**P5: Interface bridgeability.** The cross-boundary functions (the min-cut) can
be bridged by a small interface or accessor set. If the cut requires exposing
large amounts of private state, the split creates a false abstraction.

### What the Split Encodes

A package split is not just a code organization choice. It encodes three
properties about the codebase:

1. **Dependency isolation.** Each package depends on a subset of the original's
   dependencies. Consumers that need only one concern can import only one
   package.

2. **Change independence.** Changes to one concern (e.g., the expansion
   operations) don't require recompilation of the other (e.g., the VM runtime),
   provided the interface boundary is stable.

3. **Semantic legibility.** The package name communicates what the code does.
   A monolithic `machine/` says "VM stuff." Split into `machine/` + `compilation/`,
   each name carries meaning.

The split does NOT encode:
- Performance characteristics (that's the compiler's job)
- Deployment units (that's module boundaries, not packages)
- Team ownership (that's a social concern, not a structural one)

## Relationship to Existing wile-goast Analysis

This analysis occupies the **package level** in a hierarchy of boundary questions:

| Level | Question | Existing tool |
|-------|----------|---------------|
| **Struct** | Are these fields correctly grouped? | `(wile goast fca)` — Phase 1, false boundary detection |
| **Function** | Are these operations correctly grouped? | Function boundary recommendations (designed, not yet implemented) |
| **Package** | Are these functions correctly grouped? | **This tool** (proposed) |

All three levels use the same Galois connection. The objects and attributes change;
the lattice interpretation changes; the FCA core is shared.

| Level | Objects (G) | Attributes (M) | Incidence (I) |
|-------|-------------|-----------------|---------------|
| Struct | Functions | Struct fields | f accesses field |
| Function | Statements | Field accesses | stmt reads/writes field |
| Package | Functions | External packages (weighted) | f references pkg |

The package level adds IDF weighting (absent from struct/function levels because
fields don't have a "noise" problem — every field is meaningful). It also adds
the acyclicity constraint (Go-specific).

## Implementation

### Phase 1: New Go Primitive — `go-func-refs`

The critical missing piece. For each function/method in the target package,
return the set of external (package-path, object-name) pairs it references.

**Inputs:** Package pattern or GoSession.

**Output:** List of alists:

```scheme
((func-ref
   (name . "MachineContext.Run")
   (pkg  . "github.com/aalpar/wile/machine")
   (refs . ((ref (pkg . "github.com/aalpar/wile/environment")
                 (objects . ("EnvironmentFrame" "NewLocalEnvironment"
                             "NewEnvironmentFrameWithParent")))
            (ref (pkg . "github.com/aalpar/wile/werr")
                 (objects . ("WrapForeignErrorf" "ErrNotANumber"))))))
 ...)
```

**Implementation approach:**

```go
func goFuncRefs(cc machine.CallContext) error {
    // 1. Load packages via GoSession (reuse existing session infrastructure)
    // 2. For each *ast.FuncDecl in each package:
    //    a. Walk body with ast.Inspect
    //    b. For each *ast.Ident, look up in types.Info.Uses
    //    c. If the defining object's package != the function's package,
    //       record (pkg-path, object-name)
    //    d. Group by package path
    // 3. Return as tagged alists
}
```

Key details:
- Use `types.Info.Uses` (not `types.Info.Defs`) to find identifiers *used* in
  each function, resolved to their defining package.
- Include method receivers: `(*T).Method` uses both the receiver type's package
  and any packages referenced in the body.
- Handle embedded fields: `env.EnvironmentFrame` where `env` is a parameter
  resolved to `*environment.EnvironmentFrame`.
- Handle type assertions: `v.(*environment.Binding)` references `environment`.
- Exclude the function's own package from the reference set.

Estimated: ~80-100 lines of Go in `goast/register.go`.

### Phase 2: Scheme Analysis Library — `(wile goast split)`

A new Scheme library built on `go-func-refs` and the existing `(wile goast fca)`
module.

#### Core functions

```scheme
;; Compute per-function import signatures from go-func-refs output.
;; Returns: alist mapping function name → set of external package paths.
(define (import-signatures func-refs) ...)

;; Compute IDF weights for each package.
;; Returns: alist mapping package path → IDF score.
(define (compute-idf signatures) ...)

;; Filter signatures to high-IDF packages only.
;; threshold: minimum IDF score (default: 0.36, i.e., < 70% of functions).
(define (filter-noise signatures idf-weights threshold) ...)

;; Build the FCA formal context from filtered signatures.
;; Objects = function names, Attributes = high-IDF packages.
;; Returns: formal context suitable for (fca-lattice).
(define (build-package-context filtered-signatures) ...)

;; Refine by API surface: replace package-level attributes with
;; (package, object-name) pairs. Uses go-func-refs detail.
(define (refine-by-api-surface func-refs filtered-signatures) ...)

;; Find the balanced min-cut: partition functions into two groups
;; minimizing cross-group references.
;; Returns: ((group-a . (func ...)) (group-b . (func ...))
;;           (cut . ((func . reason) ...)))
(define (find-split context lattice) ...)

;; Verify that a proposed split doesn't create import cycles.
;; Takes the two groups and the current package's import graph.
;; Returns: #t if acyclic, or a list of cycle paths if not.
(define (verify-acyclic group-a group-b dep-graph) ...)
```

#### Top-level entry point

```scheme
;; Analyze a package and recommend a split.
;; Returns a report with: groups, cut, feasibility, confidence.
(define (recommend-split target-package . options) ...)
```

Estimated: ~250-300 lines of Scheme.

### Phase 3: Integration with Belief DSL

A new belief type for package cohesion:

```scheme
(define-belief "package-cohesion"
  (sites (all-functions-in "github.com/aalpar/wile/machine"))
  (expect (single-cluster #:idf-threshold 0.36))
  (threshold 0.80 10))
```

This belief fires when >20% of functions would naturally belong in a different
cluster — signaling that the package has grown past its natural boundary.

### Phase 4: Interactive Split Planner

A prompt-driven workflow exposed via the MCP server:

```
wile-goast> (recommend-split "./machine")

Package: github.com/aalpar/wile/machine (102 files, 467 functions)

High-IDF dependencies:
  environment  (IDF 1.23, 21 functions)
  security     (IDF 2.89, 3 functions)
  syntax       (IDF 1.67, 12 functions)

Recommended split on "environment" dependency:
  Group A (Frame Lifecycle):      11 files, 48 functions
  Group B (Compiled Indexing):     9 files, 31 functions
  Bridge (both groups):            1 file,   3 functions

Cut ratio: 3/82 = 3.7%  (excellent: < 15%)
Acyclic: yes (Group B → Group A is safe)

Confidence: HIGH
  P1 bimodal: ✓ (two incomparable concepts with |extent| > 10)
  P2 low cut: ✓ (3.7%)
  P3 coherent: ✓ (Group A uses constructors/mutators, Group B uses accessors)
  P4 acyclic: ✓
  P5 bridgeable: ✓ (3 accessor methods needed)
```

## Complexity Estimate

| Component | LOC | Language |
|-----------|-----|----------|
| `go-func-refs` primitive | ~100 | Go |
| `(wile goast split)` library | ~250 | Scheme |
| IDF weighting | ~30 | Scheme |
| FCA context construction | ~40 | Scheme (reuses `(wile goast fca)`) |
| Min-cut analysis | ~50 | Scheme |
| Cycle verification | ~30 | Scheme |
| Report formatting | ~50 | Scheme |
| Belief integration | ~30 | Scheme |
| Tests | ~150 | Scheme + Go |
| **Total** | **~730** | |

## Open Questions

1. **Multi-way splits.** The current design finds a two-way split. Some packages
   may benefit from three-way or N-way splits. The FCA lattice naturally supports
   this (N incomparable concepts = N-way split), but the interpretation and
   cycle-checking become more complex. Start with two-way; generalize if needed.

2. **Transitive closure.** Should the analysis consider transitive dependencies
   (packages imported by the imported packages)? Current design: direct
   references only. Transitive closure might reveal hidden coupling but adds
   noise.

3. **Weighted min-cut.** Should some cross-boundary references count more than
   others? A function that bridges both groups via a type assertion (tight
   coupling) is harder to split than one that just passes a value through.
   The SSA cross-flow analysis from function boundary recommendations could
   be reused here.

4. **Incremental analysis.** Can the tool detect when a previously clean split
   has degraded? The belief DSL integration (Phase 3) partially addresses this
   by flagging when cohesion drops below threshold.

## References

See `BIBLIOGRAPHY.md` section 9 (Software Modularization and Package
Decomposition) for the foundational papers:
- Mancoridis et al. (1998) — Bunch clustering tool
- Mitchell & Mancoridis (2006) — Journal version with evaluation
- Baldwin & Clark (2000) — Dependency Structure Matrix
- Martin (2003) — Ca/Ce coupling metrics
- Lindig & Snelting (1997) — FCA applied to software modularization

See `BIBLIOGRAPHY.md` section 8 for FCA foundations:
- Ganter & Wille (1999) — Formal Concept Analysis
- Parnas (1972) — Module decomposition criteria
