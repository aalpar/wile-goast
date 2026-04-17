# Function Boundary Recommendations via FCA + SSA

Analyze Go codebases to recommend function splits, merges, and extractions
based on how functions actually use state. Extends the existing `(wile goast fca)`
module with an interpretive layer that maps lattice structure to actionable
recommendations, filtered by SSA data flow analysis.

**Status:** Design approved. Implementation plan in `2026-04-10-function-boundary-recommendations-impl.md`.

## Motivation

Phase 1 of false boundary detection (`(wile goast fca)`, v0.5.4) answers:
"Are these fields correctly grouped into structs?" This answers the dual
question: **"Are these operations correctly grouped into functions?"**

The same concept lattice serves both questions via the Galois connection.
What changes is the interpretation: instead of looking for cross-boundary
concepts (struct boundaries), we look for lattice features that indicate
function boundaries drawn in the wrong place.

## Three Recommendation Types

### Split

A function participates in **incomparable concepts** in the lattice. Two
concepts C1, C2 are incomparable when I1 ⊄ I2 and I2 ⊄ I1. Each
incomparable pair defines a potential split plane.

**SSA filter:** Not all split candidates should be split. If data flows
from one cluster's fields to the other's (via SSA def-use chains), the
function is intentionally coordinating, not accidentally aggregating.
`defuse-reachable?` from cluster 1 field-addr registers to cluster 2
store instructions detects this.

- Data flow present: intentional coordination, suppress recommendation
- No data flow: accidental aggregation, recommend split

### Merge

Two concept pairs with **disjoint extents but overlapping intents**. Multiple
functions maintain the same state separately -- a coordination risk.

### Extract

A concept C_broad (many functions, small intent) above C_narrow (fewer
functions, larger intent) where |E_broad| > |E_narrow|. The broad concept's
intent is a coherent sub-operation shared by more callers than the full
operation. Extraction would reduce duplication.

## Ranking: Pareto Dominance

Recommendations are ranked by **Pareto dominance** with separate frontiers
per type. No arbitrary weight combination. A recommendation X dominates Y
iff X >= Y on every factor and X > Y on at least one. Incomparable
recommendations are honestly reported as incomparable.

**Why Pareto over weighted scoring:** Weighted scoring hides trade-offs
behind arbitrary coefficients. Pareto surfaces preserve them. The frontier
size is itself a meta-signal: small frontier = clear worst boundary; large
frontier = many independent problems.

**Why separate frontiers:** Split, merge, and extract answer different
questions. A split and a merge have different factor vectors. Comparing
them on a shared scale conflates "function does too many things" with
"functions duplicate effort."

## Factor Vectors

### Split Factors

| Factor | Type | Meaning |
|--------|------|---------|
| `incomparable-count` | int | Incomparable concept pairs for this function |
| `intent-disjointness` | ratio [0,1] | 1.0 when clusters share no fields |
| `no-cross-flow` | boolean | SSA confirms no data flow between clusters |
| `pattern-balance` | ratio [0,1] | min(\|E1\|,\|E2\|) / max(\|E1\|,\|E2\|) -- how established both halves are in the codebase |
| `stmt-count` | int | Function size (trivial splits not worth reporting) |

`pattern-balance` measures how well each half integrates into the rest of
the codebase after splitting. Symmetric means both halves land in established
access patterns. Asymmetric means one half becomes an orphan. It does NOT
measure importance, complexity, or processing volume.

### Merge Factors

| Factor | Type | Meaning |
|--------|------|---------|
| `intent-overlap` | ratio [0,1] | \|I1 ∩ I2\| / \|I1 ∪ I2\| |
| `write-overlap` | ratio [0,1] | Same restricted to write-mode fields |
| `extent-count` | int | Functions maintaining shared state separately |

### Extract Factors

| Factor | Type | Meaning |
|--------|------|---------|
| `extent-ratio` | ratio (1,inf) | \|E_broad\| / \|E_narrow\| -- callers benefiting |
| `intent-size` | int | Size of the extractable sub-operation |
| `sub-concept-depth` | int | Lattice depth -- deeper = more specific, cleaner |

## Pipeline

```
go-load -> go-ssa-build + go-ssa-field-index
              |                    |
              |              field-index->context
              |                    |
              |              concept-lattice
              |                    |
              |         +---------+----------+
              |         |         |          |
              |    split-     merge-    extract-
              |    candidates candidates candidates
              |         |
              +-- defuse-reachable? (cross-flow filter)
                        |         |          |
                   pareto-    pareto-    pareto-
                   frontier   frontier   frontier
                        |         |          |
                   boundary-recommendations
```

### SSA filter mechanics

For each split candidate function f with incomparable clusters I1, I2:

1. Get f's SSA function from `go-ssa-build`
2. Walk instructions to find `ssa-field-addr` nodes
3. Classify each field-addr by cluster using its `struct` + `field` attributes
4. Start names = cluster I1 field-addr register names
5. Found? = is this a `ssa-store` whose `addr` is in cluster I2's field-addr set?
6. `(defuse-reachable? ssa-fn start-names found? 10)` -> #t means cross-flow

**Prerequisite:** Add `struct` field to `ssa-field-addr` and `ssa-field` mapper
nodes so the Scheme-side filter can identify which struct a field access belongs
to. Currently these nodes have `field` (field name like "Timeout") but not the
struct type name. ~3 lines of Go.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Ranking model | Pareto dominance | Preserves trade-offs, no arbitrary weights |
| Frontier scope | Separate per type | Split/merge/extract are different questions |
| SSA filtering | Eager (all candidates) | Correctness: lazy misses promotions when dominators are filtered |
| Cross-flow detection | `defuse-reachable?` | Reuses existing dataflow; per-function, not whole-program |
| FCA mode for splits | `write-only` | Write clusters define the split planes |
| FCA mode for extracts | `read-write` | Shared reads are the extractable sub-operation |
| New Go code | Add `struct` to mapper | Enables Scheme-side cluster classification without fragile type inference |
| Library | `(wile goast fca-recommend)` | Separate from core FCA; imports fca + dataflow |

## Evidence Structure

Each recommendation carries its proof as an alist:

```scheme
;; Split recommendation
((type . split)
 (function . "pkg.ProcessRequest")
 (factors
   (incomparable-count . 1)
   (intent-disjointness . 1.0)
   (no-cross-flow . #t)
   (pattern-balance . 0.67)
   (stmt-count . 8))
 (clusters
   ((intent "Config.Timeout" "Config.MaxRetries")
    (extent "pkg.ProcessRequest" "pkg.ConfigOnly"))
   ((intent "Metrics.RequestCount" "Metrics.ErrorCount")
    (extent "pkg.ProcessRequest" "pkg.MetricsOnly")))
 (dominated-by . #f))

;; Merge recommendation
((type . merge)
 (functions "pkg.ResetSession" "pkg.ExpireSession")
 (factors
   (intent-overlap . 1.0)
   (write-overlap . 1.0)
   (extent-count . 2))
 (shared-intent "Session.Token" "Session.Expiry")
 (dominated-by . #f))

;; Extract recommendation
((type . extract)
 (sub-operation "Session.Token:r" "Session.Expiry:r")
 (factors
   (extent-ratio . 1.5)
   (intent-size . 2)
   (sub-concept-depth . 2))
 (broad-extent "pkg.ValidateSession" "pkg.HandleAuth" "pkg.HandleResponse")
 (narrow-extent "pkg.HandleAuth" "pkg.HandleResponse")
 (dominated-by . #f))
```

## Relationship to Existing Work

- **Phase 1 FCA** (`(wile goast fca)`) provides the lattice. This module adds interpretation.
- **Belief DSL** could enforce discovered recommendations: "define-belief that every
  split candidate with no cross-flow gets investigated."
- **Unification detector** could confirm extract candidates: if the shared sub-operation
  has similar implementations across functions, extraction is more clearly warranted.
- **Phase 2 FCA** (cross-function field flow, designed in false-boundary-detection-design.md)
  addresses a different boundary type (between functions in a call chain, not within a function).

## File Layout

New files:
- `cmd/wile-goast/lib/wile/goast/fca-recommend.sld`
- `cmd/wile-goast/lib/wile/goast/fca-recommend.scm`
- `examples/goast-query/testdata/funcboundary/funcboundary.go`

Modified files:
- `goastssa/mapper.go` -- add `struct` field to `ssa-field-addr` and `ssa-field`
- `goast/fca_recommend_test.go` -- new test file

## Testing Strategy

1. **Pareto machinery:** Pure Scheme, hand-constructed factor vectors. No Go packages.
2. **Lattice utilities:** Hand-constructed contexts (reuse patterns from fca_test.go).
3. **Candidate detection:** Contexts designed to produce known split/merge/extract patterns.
4. **Cross-flow filter:** Synthetic Go testdata with and without cross-cluster data flow.
5. **Integration:** Full pipeline on testdata package, verify recommendations match expectations.
