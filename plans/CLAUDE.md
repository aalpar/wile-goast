# plans/ -- Plan File Conventions

**Plans go in `plans/`.** Do not create plan files in any other location.

**Plan file naming**: Use `UPPERCASE-WITH-HYPHENS.md` (e.g., `BELIEF-DSL.md`) or date-prefixed `YYYY-MM-DD-description.md` for time-stamped designs. Paired design + implementation plans use the same date prefix with `-design.md` / `-impl.md` suffixes.

## Before Starting Work

**ALWAYS check existing project artifacts before planning or proposing solutions:**

1. **Check `plans/` directory** -- Read relevant plan files to understand existing design decisions, phase status, and what's already been explored
2. **Check `TODO.md`** -- Verify the task isn't already completed or documented as deferred
3. **Check existing patterns** -- Search the codebase for prior art before proposing new designs

**Do not:**
- Create new plan files without reading existing ones in `plans/`
- Propose architectural approaches without checking how similar problems are already solved
- Start implementation without verifying assumptions against actual code

## Cross-project coordination

Work that spans wile and wile-goast is tracked in a workspace-level roadmap:
**[`../../wile/plans/WORKSPACE-ROADMAP.md`](../../wile/plans/WORKSPACE-ROADMAP.md)** (in the sibling `wile` repo).

Consult it before proposing or committing to cross-project work so sequencing
and dependency direction are explicit. When wile-side deliverables ship for
consumers in wile-goast (algebra extensions, coverage enhancements, etc.),
the workspace roadmap carries both the wile-side plan link and the
wile-goast-side consumer/follow-up status.

## Active Plan Files

| File | Contents | Status |
|------|----------|--------|
| `BELIEF-DSL.md` | Belief DSL: suppression | Suppression open (emit done) |
| `2026-04-17-belief-suppression-design.md` | Belief suppression: `with-belief-scope`, `load-committed-beliefs`, `suppress-known` | Design approved, impl pending |
| `2026-04-17-fca-duplicate-detection-design.md` | FCA-clustered duplicate function detection: reference-FCA + structural diff + algebraic equivalence + LLM residue | Design draft, impl pending |
| `2026-04-19-llm-concept-filter-design.md` | LLM as precision-oriented semantic filter on FCA concepts (cross-cutting, annotate-only default) | Design draft, impl pending |
| `2026-04-19-mcp-tool-surface-design.md` | Post-filter MCP tool surface: pipeline-shaped tools returning structured reports instead of `eval`-driven orchestration | Design draft, impl pending (Phase 1 next) |
| `2026-04-20-receiver-parameter-asymmetry-design.md` | Detect methods whose receiver contributes a single contextual read while a parameter drives the logic (Connascence of Meaning hidden by receiver syntax). Belief-based L1 rule + semantic joint-use predicate (L2). Surfaced by wile G.1 mid-parse-EOF fix. | Design draft, impl pending |

## Completed Plans (moved to `memory/`)

| File | Contents |
|------|----------|
| `CONSISTENCY-DEVIATION.md` | Five belief categories: validation results, bug fixes, known limitations |
| `UNIFICATION-DETECTION.md` | AST/SSA equivalence detection for procedure unification |
| `2026-03-23-interface-behavioral-consistency.md` | Cross-implementation behavior consistency checking for Go interfaces |
| `2026-03-23-mcp-server-design.md` | MCP server: eval tool, prompts, stdio transport |
| `2026-03-24-shared-session-design.md` | GoSession for reusing loaded packages across primitives |
| `2026-03-24-ssa-equivalence-design.md` | SSA-level comparison pass for unification detection |
| `2026-03-24-transformation-primitives-design.md` | AST transformation primitives: `ast-transform`, `ast-splice`, `cfg-to-structured` |
| `2026-03-25-algebra-rewrite-design.md` | General-purpose term rewriting for algebraic axioms |
| `2026-03-25-checked-before-use-algebra.md` | Migration of checked-before-use to algebraic fixpoint |
| `2026-03-26-c2-dataflow-design.md` | Worklist-based dataflow analysis framework for SSA block graphs |
| `2026-03-26-c2-dataflow-impl.md` | C2 dataflow analysis implementation plan |
| `2026-03-26-c3-domains-design.md` | Pre-built abstract domains: reaching defs, liveness, constant prop, sign, interval |
| `2026-03-26-c3-domains-impl.md` | C3 abstract domains implementation plan |
| `2026-04-06-structured-docstrings-design.md` | Structured docstrings for Go primitives and Scheme procedures |
| `2026-04-06-structured-docstrings-impl.md` | 93 docstrings across 10 files in 11 tasks + 21 post-plan docstrings |
| `2026-04-08-false-boundary-detection-design.md` | FCA-based false boundary detection via concept lattices |
| `2026-04-08-false-boundary-detection-impl.md` | FCA implementation: 9 tasks, NextClosure + boundary detection |
| `2026-04-09-fca-findings-goast.md` | FCA self-analysis findings: 7 cross-boundary concepts |
| `2026-04-09-function-name-forms.md` | Five function name forms, reduction to 2 forms + pkg metadata |
| `2026-04-09-readme-rewrite-design.md` | README rewrite: question-led approach |
| `2026-04-09-readme-rewrite-impl.md` | README rewrite implementation: 6 tasks |
| `2026-04-10-function-boundary-recommendations-design.md` | Function boundary recommendations: FCA + SSA + Pareto |
| `2026-04-10-function-boundary-recommendations-impl.md` | 12 tasks: Pareto + split/merge/extract + SSA filter |
| `2026-04-10-symbolic-algebra-integration.md` | Boolean simplification, belief equivalence, FCA lattice annotation |
| `2026-04-12-path-algebra-design.md` | C4: Semiring path algebra — lazy Bellman-Ford |
| `2026-04-12-path-algebra-impl.md` | C4 implementation: 7 tasks, TDD |
| `2026-04-12-fca-closure-unification-design.md` | Replace hand-rolled Galois closure with `(wile algebra closure)` |
| `2026-04-12-ssa-equivalence-v2-design.md` | SSA equivalence v2: symbolic theories + discover-equivalences |
| `2026-04-12-package-splitting-design.md` | Package splitting: FCA + IDF + min-cut + cycle verification |
| `2026-04-13-package-splitting-impl.md` | go-func-refs + (wile goast split): 12 tasks, TDD |
| `2026-04-13-split-belief-planner-design.md` | Aggregate beliefs + interactive MCP split planner |
| `2026-04-13-split-belief-planner-impl.md` | Aggregate beliefs + goast-split prompt: 13 tasks, TDD |
| `2026-04-14-run-beliefs-return-value-design.md` | run-beliefs structured return value design |
| `2026-04-14-run-beliefs-return-value-impl.md` | run-beliefs return value: 7 tasks, TDD |
| `2026-04-14-algebra-extraction-impl.md` | Extract FCA/Pareto/interval/graph from wile-goast → wile |

## Documentation (outside plans/)

| File | Purpose |
|------|---------|
| `docs/PRIMITIVES.md` | Complete primitive reference for all layers |
| `docs/AST-NODES.md` | AST node field reference (types, optionality for all tags) |
| `docs/EXAMPLES.md` | Annotated walkthroughs of example scripts |
| `docs/GO-STATIC-ANALYSIS.md` | Usage guide with architecture overview and cross-layer examples |
| `BIBLIOGRAPHY.md` | Academic references: SSA, dominators, call graphs, consistency deviation |
