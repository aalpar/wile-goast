# Analysis Libraries

Higher-level Scheme analysis libraries layered above the core primitives. For the
core primitive reference (AST, SSA, CFG, call graph, lint) and the belief DSL,
SSA-normalization, and unification libraries, see [PRIMITIVES.md](PRIMITIVES.md).
All libraries here are pure Scheme, live under `lib/wile/goast/`, and are embedded
in the binary.

| Library | Covered in |
|---------|-----------|
| `(wile goast dataflow)` | [below](#dataflow-analysis--wile-goast-dataflow) |
| `(wile goast domains)` | [below](#abstract-domains--wile-goast-domains) |
| `(wile goast fca)` | [below](#false-boundary-detection--wile-goast-fca) |
| `(wile goast fca-algebra)` | [below](#fca-algebraic-annotation--wile-goast-fca-algebra) |
| `(wile goast fca-recommend)` | [below](#function-boundary-recommendations--wile-goast-fca-recommend) |
| `(wile goast boolean-simplify)` | [below](#boolean-simplification--wile-goast-boolean-simplify) |
| `(wile goast split)` | [below](#package-splitting--wile-goast-split) |
| `(wile goast dup-detect)` | [below](#deduplication--wile-goast-dup-detect) |
| `(wile goast path-algebra)` | [below](#path-algebra--wile-goast-path-algebra) |
| `(wile goast provenance)` | [below](#provenance--wile-goast-provenance) |
| `(wile goast dispatch)` | [below](#interface-dispatch--wile-goast-dispatch) |
| `(wile goast ifds)` | [below](#valid-path-reachability--wile-goast-ifds) |
| `(wile goast taint)` | [below](#taint-flows--wile-goast-taint) |
| `(wile goast pointsto)` | [below](#points-to-and-lock-escape--wile-goast-pointsto) |
| `(wile goast pipelines)` | [below](#mcp-pipelines--wile-goast-pipelines) |
| `(wile goast belief)` | [PRIMITIVES.md](PRIMITIVES.md) |
| `(wile goast utils)` | [PRIMITIVES.md](PRIMITIVES.md) |
| `(wile goast ssa-normalize)` | [PRIMITIVES.md](PRIMITIVES.md) |
| `(wile goast unify)` | [PRIMITIVES.md](PRIMITIVES.md) |

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
| `value-flow-reached` | `(value-flow-reached <ssa-fn> <seed-names>)` -> list of SSA value names reachable from the seeds via def-use *plus* aggregate aliasing (a store through an element/field address taints the backing aggregate). Sees value-through-variadic-slice flow that `defuse-reachable?` misses; backs the `flows-to-all` checker |
| `build-addr-aggregate-map` | `(build-addr-aggregate-map <instrs>)` -> alist `(addr-register . aggregate-register)` from `ssa-index-addr`/`ssa-field-addr`. The aggregate-alias edge `value-flow-reached` adds |
| `ssa-all-instrs` | Flatten all instructions from SSA function |
| `ssa-instruction-names` | All named values in SSA function |

The `{#f,#t}` guard lattice is no longer defined here; it lives in `(wile algebra)`
as `two-point-lattice`.

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

Re-exports the whole `(wile algebra fca)` surface (`make-context`,
`context-from-alist`, `fca-context?`, `context-objects`, `context-attributes`,
`intent`, `extent`, `concept-lattice`, `concept-extent`, `concept-intent`,
`concept-lattice->algebra-lattice`, `concept-relationship`, the set operations
`set-intersect` / `set-member?` / `set-add` / `set-before` / `set-union` /
`set-subset?`, and `sort-strings`) so a Go analysis needs one import, not two.
The rows below are the algebra procedures a Go analysis reaches for, plus the
locally-defined Go SSA/callgraph bridge.

| Export | Description |
|--------|-------------|
| `make-context` | Build formal context from objects, attributes, incidence function |
| `context-from-alist` | Convenience: context from `((obj attr ...) ...)` entries |
| `fca-context?` | Type predicate |
| `context-objects` | Extract object set from context |
| `context-attributes` | Extract attribute set from context |
| `field-index->context` | Convert `go-ssa-field-index` output to formal context (modes: `'write-only`, `'read-write`, `'type-only`) |
| `propagate-field-writes` | `(propagate-field-writes <field-index> <callgraph>)` -> a new field index whose write-mode accesses are transitively closed over call edges (a caller inherits its callees' writes). Callees-before-callers order; DFS back-edges skipped, so a recursive function contributes only its direct writes |
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
| `single-cluster` | `(single-cluster [opt]...)` -> an analyzer `(lambda (sites ctx) -> report)` for `define-aggregate-belief`. Self-contained: derives `go-func-refs` from the belief context's session and ignores `sites`. Options forward to `recommend-split` |

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
| `short-name` | Qualified name → its trailing segment. The join key between call-graph/SSA names and AST func-decl names |
| `all-pairs` | Unordered pairs of a list, as `(a . b)` conses |
| `build-func-ast-index` | `short-name` → AST func-decl, over parsed packages |
| `build-func-ssa-index` | `short-name` → *raw* SSA func. The un-canonicalized counterpart to `build-func-canon-index` |
| `pair-findings` | The two located findings for one scored pair (`score` = similarity, `why` = `(unify-candidate …)`); the shared constructor under `scored-candidates` |
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
(e.g. `new-edges`) since dominance treats higher as better. (Note: `cand-creates-cycle?`
keeps its own BFS over `go-callgraph-callees` rather than calling
`(wile goast path-algebra)`'s `go-callgraph-reachable`: candidates are `short-name`s
and callee names are qualified, so the traversal has to compare on `short-name`.)

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
| `go-callgraph-reachable` | `(go-callgraph-reachable <cg> <root>)` -> sorted names reachable from `root` (inclusive), `'()` when `root` is not a node. Boolean-semiring `path-query-all`; replaces the former Go BFS primitive |
| `go-callgraph-reaching` | `(go-callgraph-reaching <cg> <target>)` -> sorted transitive callers of `target` (inclusive), `'()` when absent. The caller-ward mirror: same query over the transposed graph (`edges-in`) |
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
- **`must` rests on VTA's soundness, and VTA's soundness is conditional.**
  `golang.org/x/tools/go/callgraph/vta/vta.go:74-75`: the call graph is sound
  "MODULO USE OF REFLECTION AND UNSAFE". A concrete type injected into an
  interface only through reflection — `reflect.New(t).Elem().Interface().(I)`,
  the reflective-registry idiom used by `encoding/json`, `database/sql`,
  protobuf, and apimachinery's `runtime.Scheme` — never appears in an
  `ssa.MakeInterface` instruction, so VTA cannot see it flow in and `must` **can
  be wrong** in a scope that uses reflect/unsafe. This cannot be computed away
  (VTA's own doc names it an inherent limit), so every finding instead discloses
  it: `dispatch-reflection-in-scope` is `#t` when the analyzed scope reaches
  `reflect`/`unsafe` anywhere, `#f` otherwise. **This is a DEFEATER PRESENCE
  flag, not a proof that any specific finding is wrong** — `#t` means the
  mechanism that can hide a type from VTA is reachable somewhere in scope, so
  `must` there needs independent verification before being trusted; it does not
  mean *this* `must` is incorrect, only that it cannot be trusted on VTA's word
  alone.
- **`must` is must-*within-scope*.** On an exported interface in a library, an
  external caller can inject a type VTA never saw. Every finding carries `scope`
  (the pattern analyzed) and `iface-exported` so the consumer can see the limit of
  the claim, not just the claim.
- **`iface-exported` is `'unnamed`, not a fabricated boolean, on a structural
  interface.** `type-exported?` (which computes it) originally assumed its input
  was always a *qualified type name* ("pkg.Type"); an anonymous interface
  (`interface{Close() error}`) arrives as a *type literal* instead, and reading
  a capital letter off it is not a coherent "is this exported" answer — a naive
  scan returned `#f` for `interface{Close() error}` (FALSE REASSURANCE: any
  package anywhere can structurally satisfy it, so `must` there is *more*
  scope-limited than an exported named interface, not less), and `#t` for
  `interface{Write(b *bytes.Buffer) error}` only by accident, off the `B` in
  `bytes.Buffer` inside a method signature. `dispatch-iface-exported` reports
  `'unnamed` for any type literal (detected structurally — the string contains
  `{` or `(` — not by a hardcoded name list), so a structural interface can
  never silently read as "not exported / safely in scope".
- **`dispatch-narrowed-from` is `#f` on a CHA key miss, never a fabricated `0`.**
  VTA's candidate set is a subset of CHA's (VTA only prunes, never invents), so
  a fabricated `0` could print the impossible `narrowed-from: 0, n: 5` — CHA
  finding *fewer* candidates than VTA. `#f` means "no CHA count is available for
  this site-key", not "CHA counted zero"; it is not clamped to `n` (that would
  hide a genuine key-mismatch between VTA's and CHA's site enumeration rather
  than surface it).
- **`dispatch-synthetic-caller` marks phantom sites.** A compiler-generated
  forwarding function (`ssa.Function.Synthetic != ""` — `$bound`/`$thunk`
  closures, interface method-set wrappers, promoted-embedding stubs) can be the
  *caller* of an invoke edge. Its single invoke has no source position because
  it is not a call site that exists in source at all; it is trivially `must`
  (one forwarding call, one target) and, left unmarked, inflates a `must`-rate
  census with sites that are not really there — on client-go, 61 of 62
  position-less findings are exactly this. Surfaced from `cg-edge`'s
  `caller-synthetic` (the raw `ssa.Function.Synthetic` string), which
  `goastcg/mapper.go`'s edge mapper carries whenever the edge's caller is
  synthetic.
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
| `dispatch-narrowed-from` | `(dispatch-narrowed-from f)` — CHA's candidate count at the same site, or `#f` on a CHA key miss (never a fabricated `0`); the gap to `n` is VTA's evidence of work done |
| `dispatch-candidates` | `(dispatch-candidates f)` — list of `candidate` alists (`callee`, `concrete`, `witness`) when `n <= k`; `#f` — never `'()` — when elided |
| `dispatch-detail` | `(dispatch-detail f)` — `'full` or `'elided` |
| `dispatch-iface-exported` | `(dispatch-iface-exported f)` — `#t`/`#f` for a qualified type name, or `'unnamed` for a type literal (a structural/anonymous interface) |
| `dispatch-reflection-in-scope` | `(dispatch-reflection-in-scope f)` — `#t` when the analyzed scope reaches `reflect`/`unsafe` anywhere. A DEFEATER PRESENCE flag: `#t` means `must` here needs independent verification, not that this finding is wrong |
| `dispatch-synthetic-caller` | `(dispatch-synthetic-caller f)` — `#t` when the site's caller is a compiler-generated forwarding function (no source position; a phantom site) |
| `default-dispatch-k` | Default `k` (8) used by `dispatch-sites` when omitted |

## Valid-Path Reachability — `(wile goast ifds)`

Realizable interprocedural path reachability over `(wile algebra cfl)`. The
Reps-Horwitz-Sagiv-Rosay valid-path grammar: every return matches its own call, so
descending into a callee and returning to an ancestor are both allowed, but
returning from a function nobody called is not. Transitive closure over-approximates
by admitting exactly those unbalanced paths; the CFL grammar is what excludes them.

Call sites are the bracket alphabet: each is an open label on the forward edge and
the matching close label on the return edge. Everything is a graph, so nothing here
is Go-specific; `(wile goast taint)` supplies the Go instance.

| Export | Description |
|--------|-------------|
| `make-valid-path-grammar` | `(make-valid-path-grammar <call-site-ids>)` -> `cfl-grammar` with start symbol `VP` over distinct call-site ids |
| `make-ifds-analysis` | `(make-ifds-analysis <nodes> <call-sites>)` -> solved `cfl-solution`. `call-sites` is a list of `(from-node id to-node)`; each contributes a forward open edge and a return close edge |
| `ifds-reachable?` | `(ifds-reachable? <analysis> <from> <to>)` -> boolean. Both nodes must be declared in `nodes` (the solver fails fast otherwise) |
| `ifds-open-label` / `ifds-close-label` | Call-site id -> the bracket symbols (`cfl-open-<id>` / `cfl-close-<id>`) |

## Taint Flows — `(wile goast taint)`

Interprocedural taint over a Go call graph, at function-summary granularity: nodes
are functions, each call edge `f -> g` is an open/close bracket pair, and a function
is taint-transparent unless it is a sanitizer. A flow is a source-to-sink pair joined
by a *valid* path, so a taint that returns to a caller that never called it is not
reported.

Granularity is the limitation: there is no intraprocedural def-use, so a function
that touches both a source and a sink is a flow whether or not the tainted value ever
reaches it. Over-approximate: false positives, not false negatives, modulo the call
graph's own soundness. Nodes with no `name` are dropped (anonymous/synthetic functions
cannot be graph ids).

| Export | Description |
|--------|-------------|
| `taint-flows` | `(taint-flows <cg> <sources> <sinks> [sanitizer])` -> list of `(source-name . sink-name)`. `sources`/`sinks`/`sanitizer` are predicates on a cg-node |
| `taint-from-names` | `(taint-from-names <names>)` -> predicate matching exact names |
| `taint-from-pattern` | `(taint-from-pattern <substr>)` -> predicate matching names containing `substr` |
| `taint-default-sources` / `taint-default-sinks` / `taint-default-sanitizers` | Starter Go security sets (request/env/stdin readers; exec/sql/file entry points; `strconv.Atoi`, `filepath.Clean`). Overridable, not authoritative |

## Points-to and Lock Escape — `(wile goast pointsto)`

Value-level (instance-sensitive) points-to: a forward powerset dataflow over
allocation sites, on the same `run-analysis` engine as `(wile goast domains)`. A
value's state is the set of abstract allocation sites it may reference. The `⊤`
sentinel `*pointsto-unknown*` means "provenance unresolved, assume anything" and is
the sound default that keeps the measure from ever reporting a false candidate.

The measure on top is `lock-escape-measure`: per locked allocation instance, how many
distinct goroutine roots can reach a lock on it. `escape = 1` marks a vestigial lock
(one root, no contention); `> 1` is genuine sharing. The tool never decides the
collapse.

**Status: the interprocedural edge is unimplemented.** `make-param-seed` seeds every
parameter with `⊤`. A method receiver *is* a parameter, so `c` in `c.mu.Lock()` is `⊤`
and the measure abstains on real code: locks land in `unresolved`, and any instance
that did resolve is reported `'unbounded` because an unresolved site could alias it.
The TODO in `make-param-seed`'s body is the analysis, not a finishing touch; it also
names the context-policy choice (k-CFA vs. context-insensitive) that fixes the
false-positive profile.

| Export | Description |
|--------|-------------|
| `pointsto-function` | `(pointsto-function <ssa-fn>)` -> `run-analysis` result alist `((idx in out) ...)`; each state is `((value-name . points-to-set) ...)` |
| `pointsto-at` | `(pointsto-at <result> <block-idx> <value-name>)` -> points-to set in that block's out-state, or `#f` |
| `pointsto-anywhere` | `(pointsto-anywhere <result> <value-name>)` -> union across all out-states. SSA is single-assignment, so this is the value's invariant provenance |
| `alloc-sites` | Allocation ids generated in a function, as `"<fn>::<reg>"`. Program-unique, which is what makes two same-typed allocations distinguishable |
| `*pointsto-unknown*` / `unknown-pointee?` | The `⊤` sentinel and its membership test |
| `make-param-seed` | `(make-param-seed <ssa-fn> <keys>)` -> entry in-state. Today: parameters ↦ `⊤`, others ↦ `∅` |
| `lock-call?` | True iff an instruction is a call/defer to a method named `Lock`. Not `RLock`; `Unlock` excluded (the acquisition site is where the receiver is named) |
| `lock-sites` | `(lock-sites <ssa-fn>)` -> `(fn-name . base-value-name)` per acquisition, base being the receiver pointer |
| `goroutine-roots` | `(goroutine-roots <program>)` -> functions launched with `go` anywhere in the program |
| `make-roots-of-fn` | `(make-roots-of-fn <program> <reachable-from?> [entry-roots])` -> `fn-name -> root ids`. The call graph is *injected* (`reachable-from?`, e.g. a closure over `go-callgraph-reachable`), not assumed. `entry-roots` defaults to `("main")` |
| `lock-escape-measure` | `(lock-escape-measure <program> <roots-of-fn>)` -> `((resolved . ((alloc-id . escape) ...)) (unresolved . ((fn-name . base) ...)))`. `escape` is a count or `'unbounded`. Two keys, so "no locks" and "all locks unresolved" cannot be confused for each other |

## MCP Pipelines — `(wile goast pipelines)`

The Scheme half of the MCP pipeline tools. Each procedure wraps an
already-implemented analysis and returns the flat envelope
`((version . <int>) (provenance . <alist>) (result . <alist-or-list>))`;
`cmd/wile-goast/mcp_tools.go` evaluates it and `cmd/wile-goast/marshal.go` marshals to
JSON (kebab-case keys here, snake_case at the JSON boundary). `version` is per-tool
and bumps only on a breaking result-shape change. No `tool` field: peer protocols do
not echo the call name in responses. See [MCP.md](MCP.md) for the tool-level contract.

| Export | Description |
|--------|-------------|
| `pipeline-envelope` | `(pipeline-envelope <version> <provenance> <result>)`: the shared constructor |
| `pipeline-check-beliefs` | `(pipeline-check-beliefs <target> <beliefs-path>)`. `with-belief-scope` confines the loaded beliefs to the call; provenance carries the belief count |
| `pipeline-discover-beliefs` | `(pipeline-discover-beliefs <target> <discovery-path> <committed-path>)`. Result is `emitted-source` (Scheme ready to commit) + `filtered-results`; provenance carries raw vs. filtered counts. `committed-path` `""` means no suppression |
| `pipeline-recommend-split` | `(pipeline-recommend-split <target> <opts>)`. `opts` alist keys: `idf-threshold`, `max-attributes`, `refine` |
| `pipeline-recommend-boundaries` | `(pipeline-recommend-boundaries <target> <mode>)`. `mode` defaults to `'write-only` |
| `pipeline-find-false-boundaries` | `(pipeline-find-false-boundaries <target> <opts>)`. `opts` keys: `mode`, `min-extent`, `min-intent`, `min-types` (the minima default to 2) |
| `pipeline-find-duplicates` | `(pipeline-find-duplicates <target> <opts>)`. `opts` keys: `threshold` (default 0.6) and `verdict` (default `#f`). `threshold` sets each pair's `equiv-tier`; it does **not** filter which pairs are returned. Projects `dup-detect`'s findings into marshaller-clean alists and truncates BigFloat scores to float64 |

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
| `cmd/wile-goast/mcp_tools.go` | MCP pipeline tool handlers (evaluate the `(wile goast pipelines)` procedures) |
| `cmd/wile-goast/marshal.go` | Scheme envelope to JSON marshaller |
| `lib/wile/goast/belief.scm` | Belief DSL implementation (embedded in binary) |
| `lib/wile/goast/belief-checkers.scm` | The belief DSL's property checkers (included by `belief.sld` alongside `belief.scm`) |
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
| `lib/wile/goast/dup-detect.scm` | Deduplication: FCA reference clustering + AST/SSA measure surface + cost half (embedded in binary) |
| `lib/wile/goast/ifds.scm` | Valid-path (realizable interprocedural path) reachability over `(wile algebra cfl)` (embedded in binary) |
| `lib/wile/goast/taint.scm` | Interprocedural taint flows over a Go call graph, on `ifds` (embedded in binary) |
| `lib/wile/goast/pointsto.scm` | Value-level points-to and the lock-escape measure (embedded in binary) |
| `lib/wile/goast/pipelines.scm` | The MCP pipeline procedures and their result envelope (embedded in binary) |
| `goast/prim_restructure.go` | Block restructuring: goto elimination, loop return rewriting, guard folding (`go-cfg-to-structured`) |
| `goastssa/prim_canonicalize.go` | SSA function canonicalization (`go-ssa-canonicalize`) |

Every `lib/wile/goast/<lib>.scm` has a `<lib>.sld` sibling carrying the R7RS
library definition; the `(export ...)` clause there is the authoritative surface.
