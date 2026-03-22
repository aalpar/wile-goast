# Call-Set Clustering — Design

**Date:** 2026-03-22
**Status:** Draft
**Target:** `CompileTimeContinuation` in `github.com/aalpar/wile/machine`

## Problem

The call-convention mining script revealed that `CompileTimeContinuation` (95 calling methods) has no call convention above 60%. This means either the type is a grab-bag that should be split, the methods cluster into subgroups with internal conventions, or every method is unique and splitting won't help. We need data to distinguish these cases.

## Approach

Cluster methods by call-set similarity using pairwise Jaccard and greedy agglomerative clustering. If clusters emerge, they are candidate sub-types. If no clusters emerge, the type is genuinely heterogeneous.

## Algorithm

**Input:** A package pattern and a receiver type name.

**Step 1 — Call Extraction.** Parse the package, extract methods on the target type, build call sets. Reuse extraction logic from the mining script. Exclude methods with empty call sets.

**Step 2 — Pairwise Jaccard Matrix.** For each pair of methods (i, j):

```
J(i,j) = |callees_i ∩ callees_j| / |callees_i ∪ callees_j|
```

Store as `(method-a method-b similarity)` triples, sorted descending.

**Step 3 — Greedy Agglomerative Clustering.** Start with each method in its own singleton. Walk the similarity list top-down:
- Both unclustered: create a new cluster containing both.
- One in a cluster: add the other if its average similarity to existing members exceeds `min-cluster-similarity`.
- Both in different clusters: skip (don't merge clusters — keeps them tight).

**Step 4 — Cluster Characterization.** For each cluster, compute its core call set — callees that every member calls. Report cluster size, core calls, and members. Singletons go in an "unclustered" group.

## Output Format

```
══ CompileTimeContinuation: Method Clusters ══
  95 methods, 4522 pairs compared
  Clusters: 5 (67 methods), Unclustered: 28

── Top 10 Similarity Pairs ──
  compileIf <-> compileCond  0.85
  compileAnd <-> compileOr   0.82
  ...

── Cluster 1 (15 methods) ──
  Core calls: pushOperation, IsSyntaxEmptyList, SyntaxCar, SyntaxCdr
  Members:
    compileIf, compileCond, compileCase, ...

── Unclustered (28 methods) ──
    String, validate, Reset, ...
```

## Configuration

| Parameter | Default | Purpose |
|-----------|---------|---------|
| `target` | `"github.com/aalpar/wile/machine"` | Go package pattern |
| `target-type` | `"CompileTimeContinuation"` | Receiver type to cluster |
| `min-cluster-similarity` | `0.30` | Jaccard threshold for joining a cluster |

## Script Structure

Single file: `examples/goast-query/call-cluster.scm`

```
Configuration
Utilities — import (wile goast utils)
Call Extraction — receiver-type-name, callee-name, extract-callees
Jaccard Similarity — set-intersect, set-union, jaccard
Pairwise Matrix — all pairs, sorted by similarity desc
Greedy Clustering — assign methods to clusters
Report — top pairs, clusters with core calls, unclustered
```

## Scope

**In scope:**
- Single type clustering, configurable target
- Jaccard similarity, greedy agglomerative
- Core call characterization per cluster
- Top similarity pairs for sanity-checking

**Out of scope (v1):**
- Multi-type analysis (run it once per type)
- Dendrograms or tree visualization
- Optimal clustering (k-means, spectral)
- Automatic type-splitting recommendations

## Success Criteria

- At least 2 distinct clusters emerge from `CompileTimeContinuation`
- Clusters have non-trivial core call sets (≥2 shared callees)
- Unclustered methods are genuinely dissimilar
