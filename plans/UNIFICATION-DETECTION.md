# Procedure Unification Detection — Remaining Work

**Current state**: AST-level prototype validated on crdt (4 zero-cost candidates found). Substitution collapsing implemented and load-bearing. Cross-package comparison working. SSA equivalence pass completed (v0.5.1): `go-ssa-canonicalize`, `(wile goast ssa-normalize)` (migrated to `(wile algebra symbolic)` named theories), `(wile goast unify)` diff engine with `ssa-equivalent?`.

**Reference**: `examples/goast-query/unify-detect-pkg.scm`

## SSA-Level Equivalence Pass (v2 — complete)

Axiom declarations migrated to `(wile algebra symbolic)` named-axiom/theory format. New exports: `ssa-theory`, `ssa-binop-protocol` (from ssa-normalize), `ssa-equivalent?` (from unify).

`ssa-equivalent?` uses `discover-equivalences` to check if two SSA nodes share a normal form under any sub-theory. Accepts custom theories for domain-specific equivalences (e.g., min/max absorption). Register name differences are tolerated via `ssa-diff` classification.

**Validation answer**: Standard algebraic normalization adds nothing over Go's SSA builder (v1 finding confirmed). The v2 infrastructure enables domain-specific theories that detect equivalences Go doesn't know about. The pipeline is ready; the theories are the next step.

**Key question** (unchanged): Does SSA normalization actually collapse enough to be useful, or do type differences still dominate?

## SSA Equivalence — Validation Results

Implemented in v0.5.1. Three components: Go primitive `go-ssa-canonicalize` (block ordering + register renaming), Scheme libraries `(wile goast ssa-normalize)` and `(wile goast unify)`.

Validation script: `examples/goast-query/ssa-unify-detect.scm`

```
Test case                       AST      SSA-canon  SSA-norm   Verdict
pncounter/gcounter Increment    0.6604   0.9082     0.9082     divergent
identity AddZero/JustReturn     0.8750   0.7500     0.7500     divergent
```

**Findings:**

1. **SSA canonicalization adds substantial value** — 0.66 → 0.91 for the primary test case. Block reordering and register alpha-renaming eliminate noise that dominates AST-level comparison.
2. **Scheme algebraic normalization adds no further improvement** — Go's SSA builder already performs constant folding and strength reduction. The Scheme rules (`x + 0 → x`, commutative sort) fire on the s-expression representation but the Go compiler has already eliminated these patterns.
3. **The normalization library remains useful** as extensible infrastructure for future rules that address patterns the Go compiler doesn't optimize (e.g., domain-specific identities).

## Other Future Enhancements

- **CFG isomorphism** — detect functions with identical branching structure but different computations. Combined with AST diff, distinguishes "same algorithm, different types" from "different algorithm, same types."
- **Sub-tree matching** — detect duplicated code *fragments* within different functions, not just whole-function similarity. Requires sliding-window or suffix-tree approaches on s-expressions.
- **Call graph context** — use `go-callgraph` to find functions that call the same dependencies as a pre-filter narrowing candidates to similar "purpose signatures."

## Known Limitations (inform v2 design)

- Positional list diff only — inserting a single statement shifts all subsequent pairs. Full tree edit distance needed for insertion/deletion similarity.
- No call-site cost measurement (how many callers change?)
- No interface compliance check (unifying may break interface contracts)
- Single-codebase validation only. Type-substitution dominance in findings may shrink as generics adoption grows.
