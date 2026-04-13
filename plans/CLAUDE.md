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

## Plan Files

| File | Contents | Status |
|------|----------|--------|
| `BELIEF-DSL.md` | Belief DSL design: graduation model, bootstrapping, discovery emit mode, suppression | Graduation/emit open |
| `CONSISTENCY-DEVIATION.md` | Five belief categories: validation results, bug fixes, known limitations | Complete |
| `UNIFICATION-DETECTION.md` | AST/SSA equivalence detection for procedure unification | SSA equivalence v2 complete |
| `2026-03-23-interface-behavioral-consistency.md` | Cross-implementation behavior consistency checking for Go interfaces | Implemented |
| `2026-03-23-mcp-server-design.md` | MCP server: eval tool, prompts, stdio transport | Complete |
| `2026-03-24-shared-session-design.md` | GoSession for reusing loaded packages across primitives | Complete (v0.5.0) |
| `2026-03-24-ssa-equivalence-design.md` | SSA-level comparison pass for unification detection | Complete (v0.5.1) |
| `2026-03-24-transformation-primitives-design.md` | AST transformation primitives: `ast-transform`, `ast-splice`, `cfg-to-structured` | Complete |
| `2026-03-25-algebra-rewrite-design.md` | General-purpose term rewriting for algebraic axioms | Complete |
| `2026-03-25-b3-c2-c6-design.md` | B3 restructuring + C2-C6 algebra framework roadmap | B3 complete; C2-C6 design only |
| `2026-03-25-checked-before-use-algebra.md` | Migration of checked-before-use to algebraic fixpoint | Complete |
| `2026-03-26-c2-dataflow-design.md` | Worklist-based dataflow analysis framework for SSA block graphs | Design approved |
| `2026-03-26-c2-dataflow-impl.md` | C2 dataflow analysis implementation plan | Complete |
| `2026-03-26-c3-domains-design.md` | Pre-built abstract domains: reaching defs, liveness, constant prop, sign, interval | Complete |
| `2026-03-26-c3-domains-impl.md` | C3 abstract domains implementation plan | Complete |
| `2026-04-06-structured-docstrings-design.md` | Structured docstrings for Go primitives and Scheme procedures | Complete |
| `2026-04-06-structured-docstrings-impl.md` | Implementation plan: 93 docstrings across 10 files in 11 tasks + 21 post-plan docstrings (fca.scm, path-algebra.scm) | Complete |
| `2026-04-08-false-boundary-detection-design.md` | FCA-based false boundary detection: discover natural struct groupings from field access patterns | Complete |
| `2026-04-08-false-boundary-detection-impl.md` | Implementation plan: 9 tasks, FCA core + bridge + boundary detection + integration test | Complete |
| `2026-04-09-fca-findings-goast.md` | FCA self-analysis findings: 7 cross-boundary concepts, AccessRequest×Config extraction (9 sites), makeVarDeclInt inline | Complete |
| `2026-04-09-function-name-forms.md` | Five function name forms across layers, mismatch bugs, reduction plan (target: 2 forms + pkg metadata) | Complete |
| `2026-04-10-function-boundary-recommendations-design.md` | Function boundary recommendations: FCA lattice + SSA cross-flow, Pareto ranking, separate split/merge/extract frontiers | Approved |
| `2026-04-10-function-boundary-recommendations-impl.md` | Implementation plan: 12 tasks, Pareto + split/merge/extract + SSA filter + integration | Open |
| `2026-04-10-symbolic-algebra-integration.md` | Phase 3 of wile symbolic algebra: Go boolean simplification, belief equivalence, FCA lattice annotation | Complete |
| `2026-04-12-path-algebra-design.md` | C4: Semiring path algebra on call graphs — lazy single-source Bellman-Ford with `(wile algebra semiring)` | Complete |
| `2026-04-12-path-algebra-impl.md` | C4 implementation: 7 tasks, TDD, synthetic + real CG tests | Complete |
| `2026-04-12-fca-closure-unification-design.md` | Replace hand-rolled Galois closure in fca-algebra with `(wile algebra closure)` | Complete |
| `2026-04-12-ssa-equivalence-v2-design.md` | SSA equivalence v2: migrate to symbolic theories, wire discover-equivalences into unify | Complete |
| `2026-04-12-package-splitting-design.md` | Package splitting via import signature analysis: FCA + IDF weighting + API surface refinement + min-cut + cycle verification | Proposed |

## Documentation (outside plans/)

| File | Purpose |
|------|---------|
| `docs/PRIMITIVES.md` | Complete primitive reference for all layers |
| `docs/AST-NODES.md` | AST node field reference (types, optionality for all tags) |
| `docs/EXAMPLES.md` | Annotated walkthroughs of example scripts |
| `docs/GO-STATIC-ANALYSIS.md` | Usage guide with architecture overview and cross-layer examples |
| `BIBLIOGRAPHY.md` | Academic references: SSA, dominators, call graphs, consistency deviation |
