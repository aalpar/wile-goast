# Analysis Libraries

Higher-level Scheme analysis libraries layered above the core primitives. For the
core primitive reference (AST, SSA, CFG, call graph, lint) and the belief DSL,
SSA-normalization, and unification libraries, see [PRIMITIVES.md](PRIMITIVES.md).
All libraries here are pure Scheme, embedded in the binary.

## Dataflow Analysis — `(wile goast dataflow)`

Two facilities: def-use reachability (`defuse-reachable?`) and a general worklist-based dataflow analysis framework (`run-analysis`). Both operate on SSA block graphs from `go-ssa-build`.

| Export | Description |
|--------|-------------|
| `run-analysis` | Worklist-based forward/backward analysis: `(run-analysis direction lattice transfer ssa-fn protocol [initial-state] ['check-monotone])`. Re-exported from `(wile algebra dataflow)`. |
| `analysis-in` | Query in-state at block: `(analysis-in result block-idx)`. Re-exported. |
| `analysis-out` | Query out-state at block: `(analysis-out result block-idx)`. Re-exported. |
| `analysis-states` | Full result alist: `((idx in out) ...)`. Re-exported. |
| `ssa-cfg-protocol` | CFG-protocol adapter bridging SSA-function shape to the generic MFP solver. Pass as `protocol` argument to `run-analysis`. |
| `block-instrs` | Extract instruction list from SSA block |
| `defuse-reachable?` | Bounded def-use chain reachability via product lattice fixpoint |
| `make-reachability-transfer` | Product-lattice transfer closure used by `defuse-reachable?` |
| `boolean-lattice` | `{#f, #t}` lattice (utility) |
| `ssa-all-instrs` | Flatten all instructions from SSA function |
| `ssa-instruction-names` | All named values in SSA function |

## Abstract Domains — `(wile goast domains)`

Pre-built abstract domains that plug into C2's `run-analysis`. Each domain is a factory function that constructs a lattice and transfer function, calls `run-analysis`, and returns the result alist.

| Export | Description |
|--------|-------------|
| `go-concrete-eval` | Evaluate Go SSA integer opcodes on Scheme integers |
| `make-reaching-definitions` | Forward reaching definitions (powerset lattice) |
| `make-liveness` | Backward liveness analysis (powerset lattice) |
| `make-constant-propagation` | Forward constant propagation (flat + map-lattice) |
| `sign-lattice` | Construct the 5-element sign lattice: {bot, neg, zero, pos, top}. Re-exported from `(wile algebra)`. |
| `make-sign-analysis` | Forward sign analysis with transfer tables |
| `make-interval-analysis` | Forward interval analysis with per-block widening. Interval lattice supplied by `(wile algebra interval)`. |

## False Boundary Detection — `(wile goast fca)`

Formal Concept Analysis (Ganter & Wille, 1999) applied to Go struct field access patterns. Discovers natural field groupings from SSA data, then compares against actual struct boundaries. Mismatches are false boundary candidates — boundaries whose removal enables unification or simplifies state.

| Export | Description |
|--------|-------------|
| `make-context` | Build formal context from objects, attributes, incidence function |
| `context-from-alist` | Convenience: context from `((obj attr ...) ...)` entries |
| `context-objects` | Extract object set from context |
| `context-attributes` | Extract attribute set from context |
| `field-index->context` | Convert `go-ssa-field-index` output to formal context (modes: `'write-only`, `'read-write`, `'type-only`) |
| `field-index->positions` | Build a name→source hashtable from a field index. The Go↔source join that keeps `(wile algebra fca)` position-agnostic: the algebra's extent is opaque object names; positions live on the Go side (`ssa-field-summary.pos`) and are re-attached here by name. |
| `intent` | Galois connection: objects → shared attributes |
| `extent` | Galois connection: attributes → objects having all |
| `concept-lattice` | Compute all formal concepts via NextClosure (Ganter 1984) |
| `concept-extent` | Extract extent (object set) from concept |
| `concept-intent` | Extract intent (attribute set) from concept |
| `cross-boundary-concepts` | Filter concepts spanning multiple struct types (opts: `'min-extent`, `'min-intent`, `'min-types`) |
| `boundary-report` | Structured alist report for cross-boundary concepts |
| `boundary-findings` | Finding-shaped sibling of `boundary-report`: each extent member becomes a located `finding` (`value` = qualified func name, `where` = source position via a `field-index->positions` index, `why` = the shared intent as `(cross-boundary (fields . …) (types . …))`, `score` = `#f`). `boundary-report` is left unchanged so the `find_false_boundaries` MCP marshaller is unaffected. |

## FCA Algebraic Annotation — `(wile goast fca-algebra)`

Bridges FCA concept lattices with `(wile algebra lattice)` and `(wile algebra closure)` from wile's algebra library. The Galois closure operator `intent ∘ extent` is formalized via `make-closure-operator` on the attribute powerset lattice. Constructs an algebraic lattice from FCA concepts (with join/meet via closure application), and annotates boundary reports with lattice-theoretic relationships.

| Export | Description |
|--------|-------------|
| `concept-lattice->algebra-lattice` | Construct `(wile algebra lattice)` from FCA context + concepts |
| `concept-relationship` | Classify pair: `subconcept` / `superconcept` / `equal` / `incomparable` |
| `annotated-boundary-report` | Extend boundary report with `subconcept-of`, `superconcept-of`, `incomparable-with` |

## Function Boundary Recommendations — `(wile goast fca-recommend)`

Analyzes FCA concept lattices to produce ranked split/merge/extract recommendations for function boundaries. SSA data flow filtering distinguishes intentional coordination from accidental aggregation. Pareto dominance ranking with separate frontiers per type.

| Export | Description |
|--------|-------------|
| `dominates?` | Pareto dominance: X >= Y on all factors, > on at least one |
| `pareto-frontier` | Compute Pareto frontier and dominated groups |
| `concept-signature` | Map function name to its concepts in the lattice |
| `incomparable-pairs` | Find incomparable concept pairs in a signature |
| `split-candidates` | Functions serving incomparable state clusters |
| `merge-candidates` | Functions maintaining shared state separately |
| `extract-candidates` | Sub-operations shared by more callers than the full op |
| `boundary-recommendations` | Top-level: three Pareto frontiers (split/merge/extract) |
| `recommendation-functions` | The function names a candidate concerns, by type (split→`function`, merge→`functions`, extract→`broad-extent`) |
| `locate-recommendations` | Attach located findings to split/merge/extract candidates via `field-index->positions`; `why` = `(recommendation (type . T))` |
| `string-suffix?` | Test if a string ends with a given suffix |

## Boolean Simplification — `(wile goast boolean-simplify)`

Boolean normalization for Go AST conditions and belief selector predicates. Uses `(wile algebra symbolic)` recursive normalizer with a Boolean algebra theory (absorption, involution, idempotence, commutativity).

| Export | Description |
|--------|-------------|
| `boolean-normalize` | Normalize boolean S-expression; returns `(values normal-form trace)` |
| `boolean-equivalent?` | Check if two terms normalize to the same form |
| `selector->symbolic` | Project belief selector combinators (`all-of`→`and`, `any-of`→`or`, `none-of`→`not`) |
| `ast-condition->symbolic` | Project Go AST conditions (`&&`→`and`, `||`→`or`, `!`→`not`, comparisons→opaque atoms) |

Note: Go's `&&`/`||` become control flow in SSA, so `ast-condition->symbolic` works at the AST level (from `go-parse-expr`/`go-parse-file`), not SSA.

## Package Splitting — `(wile goast split)`

Import signature analysis for Go package decomposition. Discovers natural package boundaries using IDF-weighted FCA on per-function dependency profiles.

| Export | Description |
|--------|-------------|
| `import-signatures` | Extract per-function package dependency sets from `go-func-refs` output |
| `compute-idf` | IDF weights for dependency informativeness |
| `filter-noise` | Remove ubiquitous (low-IDF) dependencies |
| `build-package-context` | FCA context at package granularity |
| `refine-by-api-surface` | FCA context at (package, object) granularity |
| `find-split` | Min-cut two-way partition via concept lattice |
| `verify-acyclic` | Check proposed split for Go import cycles |
| `recommend-split` | Top-level: IDF + FCA + min-cut + cycle check + confidence |
| `recommend-split-findings` | Located findings for a `recommend-split` report: group-a/group-b members as findings (`why` = `(split-group (side . a\|b))`), positioned via `func-ref.pos` |

## Deduplication — `(wile goast dup-detect)`

The FCA audit trace for deduplication — the exact twin of `(wile goast fca)`'s
`boundary-findings`, on a `function × external-ref` concept lattice instead of
`function × field`. Functions sharing a maximal informative reference set (an FCA
concept with extent ≥ 2) are duplicate candidates; each extent member becomes a
located `finding` whose `why` is the shared ref intent. Composes the `split.scm`
clustering chain (objects are function names) with `fca` + `provenance`. Default
output is the audit trace; structural scoring (`ast-diff`/`ssa-diff`), the
benefit/equivalence measures, and the opt-in `candidate->verdict` are slice 5b;
the LLM judge is deferred. `go-func-refs` now carries an optional `pos`
(`"file:line:col"`, present when the function position is valid) — the name→source
data this module joins on.

| Export | Description |
|--------|-------------|
| `function-ref-context` | `function × external-ref` FCA context, IDF-filtered (reuses the `split.scm` chain at function granularity) |
| `duplicate-candidate-concepts` | Concepts with extent ≥ 2 and a non-empty intent — by FCA closure, duplicate-candidate clusters |
| `func-refs->positions` | Name→source hashtable from `go-func-refs` output (the `field-index->positions` twin; exact-match keys) |
| `dup-candidate-findings` | Per candidate concept, each extent member → a located `finding`; `why` = `(duplicate-candidate (refs . intent))`, `score` = `#f`. The `boundary-findings` twin |
| `find-duplicate-candidates` | Top-level (5a): `go-func-refs` → IDF-filtered context → concept lattice → candidate concepts → located findings |
| `score-candidate-pair` | (5b) Benefit measures (`benefit`, `type-params`, `value-params`, `similarity`) + `equiv-tier` for a candidate pair, joined to AST/SSA via `short-name`. Tier: `proven` (SSA-canonical `unifiable?`) / `structural` (AST `unifiable?`) / `divergent`. Returns `#f` when names don't resolve. Takes a **pre-canonicalized** SSA index (`build-func-canon-index`), not a raw one: canonicalization is hoisted to once-per-function (a function in a concept of extent _k_ was previously re-canonicalized _k−1_ times) and guarded — a function whose dominator tree fails to cover all blocks is omitted from the index and falls back to the AST tier instead of aborting the sweep |
| `build-func-canon-index` | (5b) `short-name` → *canonicalized* SSA func, computed once per function with `go-ssa-canonicalize` failures guarded. The pre-canonicalized counterpart to the raw `build-func-ssa-index`; consumed by `score-candidate-pair`. Eliminates the per-pair canonicalization redundancy and the crash on functions with unreachable blocks |
| `scored-candidates` / `find-scored-candidates` | (5b) Each within-cluster pair → a scored candidate: two located findings (`score` = effective similarity), `why` = `(unify-candidate (peer . other) (measures . M))`. `find-scored-candidates` is the top-level (clusters → scored pairs) |
| `candidate->verdict` | (5b) **Opt-in** projection of a candidate's `equiv-tier` to `duplicate`/`likely-duplicate`/`distinct` — the categorical analog of `finding->scalar`; the measure surface is the default, the verdict is requested, never imposed |
| `cand-new-edges` | (5c) `\|callers(a) ∪ callers(b)\|` — the in-degree the merged function would carry (coupling concentrated; merging retargets edges, doesn't add them) |
| `cand-creates-cycle?` | (5c) `a reaches b ∨ b reaches a` (BFS over `go-callgraph-callees`) — merging a call-path pair collapses it into a self-cycle. Merge-side analog of `verify-acyclic` |
| `cand-locality` | (5c) Ledger fact (never a verdict): `scope` (`same-pkg`/`shared-callers`/`disjoint`) + `dep-overlap` (Jaccard of external ref sets) |
| `build-func-ref-index` | (5c) name → `func-ref` entry, for `cand-locality`'s pkg + ref-set lookup |
| `find-candidates-with-cost` | (5c) Top-level full ledger: 5b scoring + the cost measures, findings embedding the full set |

Cost axes plug into the same `pareto-frontier`; negate lower-is-better axes
(e.g. `new-edges`) since dominance treats higher as better. (Note: there is no
`go-callgraph-reachable` primitive — reachability is computed by BFS over
`go-callgraph-callees`.)

Rank the measure surface with `pareto-frontier`/`dominates?` from
`(wile goast fca-recommend)` — the one documented combinator (no dedup-specific
ranking). Each item is `(id numeric-factors-alist)`; project to numeric measures
(e.g. `benefit`, `similarity`) since dominance ignores no key. The cost-half
measures (`cand-new-edges`, `cand-creates-cycle?`, `cand-locality`) are slice 5c.
`short-name` reconciliation collides only when two methods share a short name
across receiver types (rare; index keeps the last). The LLM judge is deferred.

## Path Algebra — `(wile goast path-algebra)`

Semiring-parameterized path computation over call graphs. Lazy single-source Bellman-Ford with per-source caching. SCC side-query API exposes mutual-recursion clusters; fast-path introspection reports when the bigint-counting kernel is active.

| Export | Description |
|--------|-------------|
| `make-path-analysis` | Construct path analysis from semiring, call graph, and optional edge-weight function |
| `path-analysis?` | Type predicate |
| `path-query` | Query semiring value between source and target (lazy, cached) |
| `path-query-all` | Return distance alist for all reachable nodes from source |
| `path-analysis-sccs` | Force SCC decomposition; returns a `<graph-scc>` record |
| `path-node-in-cycle?` | True iff function lies in a non-trivial SCC (i.e., is recursive or mutually recursive). Raises if name is not in the call graph. |
| `path-cyclic-nodes` | List of function names that lie in non-trivial SCCs |
| `path-analysis-fast-path?` | True iff bigint-counting kernel is attached |
| `path-analysis-fast-path-kind` | Symbol naming the fast-path strategy (`'bigint-counting`) or `#f` |

## Provenance — `(wile goast provenance)`

Resolve SSA instructions to source positions. The SSA mapper injects
`(pos . "file:line:col")` per instruction under `go-ssa-build`'s `'positions`
option; the belief context now builds with it on. These accessors surface that
position so analyses stop discarding the provenance they already compute. It
also defines the auditable *finding* — a value (category or measure) paired with
`where`/`why`/`score` — and its human rendering; `why` is structured
(`(reason-tag . data-alist)`) so downstream Scheme can filter/aggregate on it.
First primitives of the auditable-categorization facility
(`plans/2026-06-01-auditable-categorization-design.md`).

The provenance audit is now closed across the suite: every opinion-producer emits
located findings — the belief checkers (all six), FCA false-boundary
(`boundary-findings`), deduplication (`dup-detect`), the unification measure
surface (`dup-detect`'s scored candidates), and the split/boundary
*recommendations* (`recommend-split-findings`, `locate-recommendations`). Lint
(`go-analyze`) carries positions natively. A result you cannot point at and
justify is an incomplete result.

| Export | Description |
|--------|-------------|
| `ssa-instr-pos` | Source position `"file:line:col"` of an SSA instruction node, or `#f` |
| `ssa-call-position` | Position of the first call to a named function in a block, or `#f` |
| `ssa-first-pos` | Position of the first instruction in an SSA function matching a predicate (and carrying a position), or `#f` — lets a checker locate a store/guard it identified |
| `ssa-func-call-position` | Position of the first call to a named function anywhere in an SSA function — the function-level companion to `ssa-call-position` |
| `make-finding` | Construct an auditable finding `(value, where, why, score)` |
| `finding-value` / `finding-where` / `finding-why` / `finding-score` | Finding accessors |
| `render-why` | Project a structured reason `(reason-tag . data-alist)` to a human string |
| `render-finding` | One-line audit string `"where — why [score]"` |
| `render-category` | Editor-walkable report for a category: a `LABEL (N)` header then one indented `render-finding` line per member. Generic over any finding list (belief adherence/deviations, an FCA concept's extent, …). The literal "show me every X and a one-line why." |

## Interface Dispatch — `(wile goast dispatch)`

Located, justified findings for interface call sites. Introduces no new analysis:
it folds facts VTA, CHA, and SSA positions already compute into site-unit
`(wile goast provenance)` findings (`value` = class, `where` = call position,
`why` = `(dispatch ...)`, `score` = `#f` — no natural confidence exists here, so
none is fabricated). The unit is the call site, not the raw edge: grouping VTA's
invoke edges by `caller@pos` turns "27 rows" into one 27-way finding, which is
what makes `class` and `n` well-defined in the first place.

- **`class` is a pure function of `n`** (the VTA candidate count at the site):
  `0` -> `none` (no concrete type flows here *within scope*), `1` -> `must` (VTA's
  sound set is a singleton, so the true callee set is a subset of it: if the call
  executes, it calls that function), `>1` -> `may`. No judgment enters.
- **`must` is must-*within-scope*.** On an exported interface in a library, an
  external caller can inject a type VTA never saw. Every finding carries `scope`
  (the pattern analyzed) and `iface-exported` so the consumer can see the limit of
  the claim, not just the claim.
- **`dispatch-candidates` is `#f` when elided, never `'()`.** An empty list would
  let a 27-way site read as "no candidates" — the silent false negative,
  reintroduced through the encoding. `dispatch-n` is always the true candidate
  count regardless of `k`, so `default-dispatch-k` (8) controls *detail*, never
  *sites* or *truth*: every site is always returned, and `n` never shrinks.
- **A witness may have no position.** `ssa.MakeInterface` carries a valid `Pos()`
  only for an *explicit* conversion (`I(T{})`); the three implicit forms (var
  decl, call arg, assignment) are nearly all of real Go and yield `NoPos`. `func`
  (the enclosing SSA function) is always present; `pos` degrades to `#f` rather
  than a guess — a missing witness, never a wrong one.
- **A witness carries `iface`, the interface it actually entered**, which may
  differ from the site's own `iface`. The witness index is keyed on concrete type
  alone, so a witness list for type `T` can include a conversion of `T` into a
  *different* interface than the one this site dispatches on. Witnesses are
  labelled with their true `iface` rather than filtered by string equality
  against the site's `iface`: that filter would delete legitimate witnesses under
  interface embedding (`K` embeds `I`: the only `MakeInterface` for a `K` value
  records `type = K`, while the call site narrows to `I`).
- **`dispatch-sites` sorts by site-key before returning**, so output order is
  stable across calls. CHA/VTA traverse Go maps internally, so raw discovery
  order is not reproducible call to call, even at identical `k`.
- **`'precise` cannot help with interface questions.** `goastcg/precise.go:66-68`
  declines any site where `Common().IsInvoke()` and falls through to CHA's edge
  unrefined — while the mode is named `'precise`. On an interface corpus,
  `'precise` returns exactly CHA's over-approximate bound.

| Export | Description |
|--------|-------------|
| `dispatch-sites` | `(dispatch-sites pattern [k])` — entry point. One finding per interface call site in `pattern`. Folds VTA (candidates), CHA (`narrowed-from`), and SSA positions (witnesses). `k` defaults to `default-dispatch-k`; every site is always returned regardless of `k` |
| `dispatch-class` | `(dispatch-class f)` — `none` / `must` / `may`, a pure function of `n`. Alias for `finding-value` |
| `dispatch-n` | `(dispatch-n f)` — VTA's true candidate count at this site, independent of `k`/`detail` |
| `dispatch-iface` | `(dispatch-iface f)` — the interface type dispatched on |
| `dispatch-method` | `(dispatch-method f)` — the interface method invoked |
| `dispatch-narrowed-from` | `(dispatch-narrowed-from f)` — CHA's candidate count at the same site; the gap to `n` is VTA's evidence of work done |
| `dispatch-candidates` | `(dispatch-candidates f)` — list of `candidate` alists (`callee`, `concrete`, `witness`) when `n <= k`; `#f` — never `'()` — when elided |
| `dispatch-detail` | `(dispatch-detail f)` — `'full` or `'elided` |
| `default-dispatch-k` | Default `k` (8) used by `dispatch-sites` when omitted |

## Source Map

| File | Purpose |
|------|---------|
| `cmd/wile-goast/mcp.go` | MCP server: eval tool handler, prompt handlers |
| `cmd/wile-goast/prompts/*.md` | MCP prompt content (embedded) |
| `goast/mapper.go` | Go AST to s-expression conversion |
| `goast/unmapper.go` | S-expression to Go AST conversion (dispatch) |
| `goast/unmapper_{decl,stmt,expr,types,comments}.go` | Unmapper by AST category |
| `goast/helpers.go` | Shared utilities (list building, field extraction) |
| `goast/prim_goast.go` | Primitive implementations |
| `goast/register.go` | Extension registration |
| `goast{ssa,cfg,cg,lint}/mapper.go` | IR-specific s-expression mappers |
| `goast{ssa,cfg,cg,lint}/register.go` | Sub-extension registration |
| `lib/wile/goast/belief.scm` | Belief DSL implementation (embedded in binary) |
| `lib/wile/goast/dataflow.scm` | Def-use reachability + worklist dataflow analysis framework (embedded in binary) |
| `lib/wile/goast/utils.scm` | Shared traversal utilities (`nf`, `walk`, `tag?`) and tree rewriters (`ast-transform`, `ast-splice`) |
| `lib/wile/goast/ssa-normalize.scm` | SSA algebraic normalization rules (embedded in binary) |
| `lib/wile/goast/unify.scm` | AST/SSA diff engine with pluggable classifiers (embedded in binary) |
| `lib/wile/goast/fca.scm` | Formal Concept Analysis: false boundary detection via concept lattices (embedded in binary) |
| `lib/wile/goast/fca-algebra.scm` | FCA algebraic annotation: concept lattice as `(wile algebra lattice)`, relationship classification (embedded in binary) |
| `lib/wile/goast/fca-recommend.scm` | Function boundary recommendations: split/merge/extract via FCA + SSA cross-flow (embedded in binary) |
| `lib/wile/goast/split.scm` | Package splitting analysis: IDF-weighted FCA on import signatures (embedded in binary) |
| `goast/prim_funcrefs.go` | Per-function external reference extraction (`go-func-refs`) |
| `lib/wile/goast/boolean-simplify.scm` | Boolean normalization for Go AST conditions and belief selectors via `(wile algebra symbolic)` (embedded in binary) |
| `lib/wile/goast/path-algebra.scm` | Semiring path algebra: Bellman-Ford over call graphs (embedded in binary) |
| `lib/wile/goast/domains.scm` | Pre-built abstract domains: reaching defs, liveness, constant prop, sign, interval (embedded in binary) |
| `lib/wile/goast/provenance.scm` | Provenance: resolve SSA instructions to source positions (`ssa-instr-pos`, `ssa-call-position`); first primitive of the auditable-finding facility (embedded in binary) |
| `lib/wile/goast/dispatch.scm` | Interface dispatch as located, justified findings: site-unit grouping over VTA/CHA/SSA (embedded in binary) |
| `goast/prim_restructure.go` | Block restructuring: goto elimination, loop return rewriting, guard folding (`go-cfg-to-structured`) |
| `goastssa/prim_canonicalize.go` | SSA function canonicalization (`go-ssa-canonicalize`) |
