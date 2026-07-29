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
| `BELIEF-DSL.md` | Belief DSL: suppression shipped | Suppression shipped (emit done) |
| `2026-04-17-belief-suppression-design.md` | Belief suppression: `with-belief-scope`, `load-committed-beliefs`, `suppress-known` | Shipped — see `2026-04-17-belief-suppression-impl.md` |
| `2026-04-17-belief-suppression-impl.md` | Implementation plan: 9 tasks, TDD, 13 integration tests | Shipped 2026-04-23 |
| `2026-04-17-fca-duplicate-detection-design.md` | FCA-clustered duplicate function detection: reference-FCA + structural diff + algebraic equivalence + LLM residue | Design draft, impl pending |
| `2026-04-19-llm-concept-filter-design.md` | LLM as precision-oriented semantic filter on FCA concepts (cross-cutting, annotate-only default) | Design draft, impl pending |
| `2026-04-19-mcp-tool-surface-design.md` | Post-filter MCP tool surface: pipeline-shaped tools returning structured reports instead of `eval`-driven orchestration | Phase 1 shipped — see `2026-04-19-mcp-tool-surface-impl.md`. Phases 2-4 pending |
| `2026-04-19-mcp-tool-surface-impl.md` | Phase 1 impl: five pipeline tools (check_beliefs, discover_beliefs, recommend_split, recommend_boundaries, find_false_boundaries), Wile→JSON marshaller, envelope, `load-beliefs!` | Shipped 2026-05-30 |
| `2026-04-20-receiver-parameter-asymmetry-design.md` | Detect methods whose receiver contributes a single contextual read while a parameter drives the logic (Connascence of Meaning hidden by receiver syntax). Belief-based L1 rule + semantic joint-use predicate (L2). Surfaced by wile G.1 mid-parse-EOF fix. | Shipped 2026-06-05 — `receiver-parameter-asymmetry` checker; impl `2026-04-20-receiver-parameter-asymmetry-impl.md` |
| `2026-05-31-objective-measure-subjective-norm-design.md` | General architecture: two operators (C subjective categorize / M objective measure); LLM holds C, wile-goast holds M; freeze C→M via the discover→commit lifecycle. Names the pattern that `llm-concept-filter` (M→C, live) and `receiver-parameter-asymmetry` (C→M, frozen) both instantiate. | Design note, no impl (architecture statement) |
| `2026-07-06-find-duplicates-tool-design.md` | `find_duplicates` command-level MCP tool: surface the shipped `(wile goast dup-detect)` scan as tool #6; measure surface default, opt-in verdict; two acceptance gates (product correctness / A/B wedge value) | Design complete — see `2026-07-06-find-duplicates-tool-impl.md` |
| `2026-07-06-find-duplicates-tool-impl.md` | Impl: 6 TDD tasks — clean-candidate projection in `pipeline-find-duplicates`, Go handler + registration, verdict discipline, true-neg/determinism, empty+error paths (reuses `dupcluster` fixture), MCP.md row | Ready to implement |
| `2026-07-12-interface-dispatch-findings-design.md` | Interface dispatch as located, justified findings: site-unit findings, `class = f(n)`, witnesses, the detail-not-sites knob | Design complete |
| `2026-07-12-interface-dispatch-findings-impl.md` | Impl: 7 tasks — `concrete` on ssa-make-interface, `iface`/`method`/`recv` on cg-edge, golden fixture, `(wile goast dispatch)`, witnesses, K-invariant | Shipped 2026-07-12 (12 commits, `5662f82..7515585`). Gate reproduces the design census on real source sites: 137 sites / 97 `must` vs 137 / 96. **`must` is sound only modulo reflection+unsafe** — a false `must` was reproduced and the finding now carries a `reflection-in-scope` defeater. Follow-ups in [`TODO.md`](../TODO.md) → *Interface Dispatch*. |
| `2026-07-14-andersen-pointsto-design.md` | `(wile goast andersen)`: inclusion-based, field-sensitive points-to, scoped to real Go. Six constraint forms extracted in Go; the solve is `lfp(F)` over a product lattice `P(Object) × P(Defeater)` in the algebra layer. Interface dispatch is a `load` with a method table, so **the call graph falls out of the same fixpoint**. Lifts `goastcg/precise.go:66`. Validated against the vendored Andersen oracle (`goastcg/testdata/oracle/`, 126/126, `ssa.InstantiateGenerics` mandatory). Barred from cycle elimination / HVN by design, so the result speaks to the algebraic bet. | Design complete, impl pending. Serves notebook `2026-07-14-pre-composition/E26`; prerequisite is `2026-07-14-algebraic-bet/C3` (verify the operator laws first). |
| `2026-07-14-cflreach-demand-pointsto-design.md` | E35 probe design note: does demand-driven CFL-reachability sidestep the errorString flood? v1 engine + validation (126/126, median 11-node slice, flood-sized error tail). References section (10 verified CFL-reachability / demand-points-to citations). Superseded on the v2 mechanism by the E36 spec below. | Validated (E35); v2 section superseded by `2026-07-14-e36-rsm-pointsto-design.md` |
| `2026-07-14-e36-rsm-pointsto-design.md` | E36 final RSM design: demand-driven field-sensitive context-insensitive points-to as a surgical diff from `cfl.go`. Winner = boomerang (forward object-tracking: `demandContents` + `storesWhereValue` field-push + containment-tracking + load-out-gate fix), grafting magicsets' finite-Herbrand termination and tabling's semi-naive `delta`/`doneComplex` reuse. Fixes v1's uniform flood (`markActive`-on-relay, no whole-cone `activate`) and the 7/513 multi-hop-heap gaps; signature-filtered caller discovery (`icallRecvsBySig`) closes the demand-leak. `pt == batch` preserved by construction. Phase 2: k-bounded call-string context + heap cloning (SPDS rejected: E31 pre-explodes the field axis). Produced + adversarially verified by two independent multi-agent design runs that cross-validate the mechanism. | Design complete, impl pending. Target `notebooks/2026-07-14-pre-composition.d/E36-rsm/rsm.go`. Gates: EQUIV 126/126 + soundfix, DISPATCH 513/513, footprint ≈ slice, demand-leak corpus. |
| `2026-07-14-e36-rsm-pointsto-impl.md` | Impl plan for the E36 design: 7 tasks (0 scaffold `rsm.go` from `cfl.go` + snapshot soundfix; 1 semi-naive `delta`/`doneComplex` engine; 2 forward object-tracking writer-finding closes 7/513 + footprint collapse; 3 signature-filtered caller discovery closes demand-leak; 4 cyclic-heap EQUIV fixtures + push tripwire; 5 ≥3-hop deep-chain fixture; 6 full-target footprint run + notebook E36). Each semantic task independently gated on EQUIV 126/126. Harness verified: module `csize`, `GOWORK=off`, GNU-sed portability fixed, v1 baseline reproduced (EQUIV/DISPATCH PASS on corpora). | Implemented (notebook E36). 3 of 4 gates green: EQUIV 126/126+soundfix, DISPATCH 513/513+126/126, demand-leak. The **footprint-collapse gate is UNACHIEVED (not refuted)** — corrected in E37: engine footprint stays ~22790 vs optimal-slice 6 on soundfix (~3800× OVER-ACTIVATION), because the engine backward-activates all type-compatible candidate feeders but emits only resolved feeders (answer exact, footprint broad); this is NOT genuine dependency. The context-insensitive tightening lever E35 named (Sridharan-Bodik goal-directed refinement) is unimplemented, not falsified. Only the god-object-reaching minority may be genuinely context-bound. A **single-seed PROBE gate** was added (the co-seeded EQUIV/DISPATCH hid 3 latent under-approximations: gated-rule re-entrancy, signature-key name+alias, fnAddrTaken); PROBE 0 on all corpora+fixtures, 4/263 soundfix = irreducible Phase-2 merged-object residual. Both open residuals are Phase-2 (context-string + heap cloning). Target `notebooks/2026-07-14-pre-composition.d/E36-rsm/`. Follow-ups E37-E39 (footprint correction; both refinement directions refuted) + Phase 2 kickoff in the row below. |
| `2026-07-28-goast-result-view-design.md` | Result views for `find_duplicates` + `find_false_boundaries`: `scope`/`symbol` globs, `detail=summary\|full` row width, `limit`/`offset` with `truncated` provenance, via a shared `apply-view` combinator + per-tool keep/rank/project hooks. Two measured failure modes: 20404 pairs scored for 68 rows vs 5 rows that are 90% lattice context. **Filters push down only where sound** — `find_duplicates` prunes the flat pair list (after concept formation, genuine subset, and taken *before* the `max-candidate-pairs` guard so a narrow query on a wide target is answered rather than refused); `find_false_boundaries` post-filters, because restricting the FCA objects recomputes the lattice and reports 0 concepts where 5 exist. | Shipped — see `2026-07-28-goast-result-view-impl.md` |
| `2026-07-28-goast-result-view-impl.md` | Impl: 6 tasks — `glob-match?`/`position-file` in utils; the `apply-view` combinator with a tail-safe `view-sort` (the builtin `sort` dies past 20000 rows on Wile's 10000-frame limit); `find_false_boundaries` post-filter + `detail`; the `dup-detect` pair-list refactor; `find_duplicates` push-down with the guard read after the prune; docs + probe prompt. Corrects three design defects: the `*map*` regression pin does not discriminate (uses `*restructure*`), `find_false_boundaries` needed a `field-index->positions` join for `scope`, and `project` becomes `render (row detail)` so summary can skip the annotation pass. Both tools go to envelope version 2. | Shipped 2026-07-29 (commits `1616a52`..`a054bb8`). All 6 tasks reviewed clean. |
| `2026-07-15-e36-phase2-context-pointsto-plan.md` | E36 **Phase 2** kickoff + full resume point (self-contained after a context clear). Context sensitivity + heap cloning to lower the runtime god-graph footprint floor and close the 4/263 single-seed residual. **Task 0 = the flood-taxonomy experiment (GATE): soundfix's god object is the `runtime.mheap` GLOBAL singleton (uncloneable), not the errorString multi-alloc merge Phase 2 is designed for — so Task 0 characterizes which god objects are cloneable and picks/builds a validatable target (real errorString-heavy or synthetic) BEFORE any object-model code.** Then k-bounded call-string + SCC-collapse, `obj(key,t)`→`obj(key,t,ctx)`, query-scoped context via `wire`/`fireCall`, and the `⊔_c pts_cs == pts_ci` soundness gate (replaces `== batch`). Refinement line CLOSED (E38/E39): both forward-refinement directions refuted, backward engine is the tightest context-insensitive option, only context lowers the floor. | Kickoff. Start at Task 0. Phase 1 shipped (HEAD `b42e5d5`). |

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
