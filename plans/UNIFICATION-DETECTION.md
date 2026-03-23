# Procedure Unification Detection — Remaining Work

**Current state**: AST-level prototype validated on crdt (4 zero-cost candidates found). Substitution collapsing implemented and load-bearing. Cross-package comparison working.

**Reference**: `examples/goast-query/unify-detect-pkg.scm`

## SSA-Level Equivalence Pass (v2 — unbuilt)

For functions that pass the AST filter, compare SSA representations to detect operator-level equivalence. Go's SSA builder already normalizes operand order, folds constants, and applies strength reductions — yielding commutativity, identity elimination, and similar algebraic properties without custom rewrite rules. The comparison leverages what the compiler knows for free rather than reimplementing algebraic laws in Scheme.

**Key question**: Does SSA normalization actually collapse enough to be useful, or do type differences still dominate?

## Other Future Enhancements

- **CFG isomorphism** — detect functions with identical branching structure but different computations. Combined with AST diff, distinguishes "same algorithm, different types" from "different algorithm, same types."
- **Sub-tree matching** — detect duplicated code *fragments* within different functions, not just whole-function similarity. Requires sliding-window or suffix-tree approaches on s-expressions.
- **Call graph context** — use `go-callgraph` to find functions that call the same dependencies as a pre-filter narrowing candidates to similar "purpose signatures."

## Known Limitations (inform v2 design)

- Positional list diff only — inserting a single statement shifts all subsequent pairs. Full tree edit distance needed for insertion/deletion similarity.
- No call-site cost measurement (how many callers change?)
- No interface compliance check (unifying may break interface contracts)
- Single-codebase validation only. Type-substitution dominance in findings may shrink as generics adoption grows.
