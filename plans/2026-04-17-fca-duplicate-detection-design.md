# FCA-Clustered Duplicate Function Detection — Design

> **Status:** Design draft (2026-04-17). Implementation plan forthcoming.
> **Spec source:** Conversation 2026-04-17 reviewing the
> `superpowers-lab:finding-duplicate-functions` skill.
> **Scope:** Scheme procedures only. Composes existing `(wile goast unify)`,
> `(wile goast fca)`, `(wile goast ssa-normalize)`, `(wile goast split)`, and
> `go-func-refs`. No new Go primitives required.

## Goal

Detect duplicate-intent functions in a Go codebase by composing structural,
algebraic, and reference-profile signals. Reserve LLM judgment for the
*residual* cases where structure has run out — not as the primary clustering or
similarity engine.

## Non-Goals

- **No new Go primitives.** All required mappers (`go-typecheck-package`,
  `go-func-refs`, `go-callgraph-callers`, `go-ssa-build`) already exist.
- **No new abstract domains or lattice machinery.** FCA, IDF weighting,
  algebraic SSA normalization, and AST/SSA diffing all ship in v0.5.50.
- **No language extension.** Go-only. Generalization to other languages is a
  separate question that depends on porting the AST/SSA mappers.
- **No automated consolidation.** The pipeline reports candidates and verifies
  substitutability; the actual rewrite remains a human (or downstream LLM) act.
- **No replacement for `(wile goast fca-recommend)`.** That tool answers
  "are these operations correctly *grouped*?" using struct-field attributes.
  This tool answers "are these operations *redundantly implemented*?" using
  external-reference attributes. Same lattice machinery, different question.

## Motivation

The `superpowers-lab:finding-duplicate-functions` skill (TS/JS, regex
extraction, two-stage LLM categorization + comparison) is what you build when
you have *no structural tooling*. Every step where the LLM is asked to
"compare these N functions" is doing work the LLM is bad at — re-parsing
source text, holding a similarity rubric across hundreds of pairs — precisely
because the input is unstructured strings.

Once a real compiler frontend is available (which `wile-goast` exposes for Go),
the LLM stops being a parser/clusterer/similarity engine and becomes a
**judge of last resort**, used only on the residual cases where structure runs
out.

## What's New vs. Prior Art

| Component | Status | This plan reuses or adds |
|-----------|--------|--------------------------|
| `go-func-refs` | Exists (v0.5.50) | Reuses for per-function reference profile |
| `(wile goast split)` IDF weighting | Exists | Reuses on functions, not packages |
| `(wile goast fca)` concept lattice | Exists | Reuses; new context shape (function × ref) |
| `(wile goast unify)` AST/SSA diff | Exists | Reuses pairwise within FCA clusters |
| `(wile goast ssa-normalize)` algebraic equivalence | Exists | Reuses for `ssa-equivalent?` triage |
| `(wile goast fca-recommend)` function-level FCA | Exists | Different attributes, different goal — peer module |
| **`(wile goast dup-detect)`** | **New** | Pipeline composition: FCA → unify → triage → verify |
| `examples/goast-query/unify-detect-pkg.scm` | Exists (725 LOC) | Within-package; this generalizes via FCA prefilter |

The novelty is **composition**, not new machinery. Specifically: applying the
existing FCA lattice machinery to a *function × external-reference* context to
prefilter candidates, then running the existing structural similarity scorer
within each concept's extent, then routing only the structurally-distant
in-cluster pairs to an LLM judge.

## Architecture

```
┌─ Phase 1: Parse ─────────────────────────────────────────┐
│  go-typecheck-package, go-ssa-build, go-func-refs        │
│  → per-function: (name, signature, ast, ssa, ref-set)    │
└──────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─ Phase 2: Cluster (FCA on references) ───────────────────┐
│  context: objects=functions, attributes=refs             │
│  IDF-weight refs (suppress fmt, errors, builtin types)   │
│  build concept lattice                                   │
│  → clusters: each concept (F, R) with |F| > 1            │
│    is by-construction a duplicate-candidate cluster      │
└──────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─ Phase 3: Score (structural, within cluster) ────────────┐
│  for each concept (F, R), |F| > 1:                       │
│    pairwise ast-diff, ssa-diff, score-diffs              │
│    pairwise ssa-equivalent? (algebraic)                  │
│  → similarity matrix per cluster                         │
└──────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─ Phase 4: Triage ────────────────────────────────────────┐
│  bucket-A: unifiable? = #t        STRUCTURAL DUPLICATE   │
│  bucket-B: ssa-equivalent? = #t   ALGEBRAIC DUPLICATE    │
│  bucket-C: high ref-overlap +     SEMANTIC CANDIDATE     │
│            low structural sim     (needs LLM judgment)   │
│  → only bucket-C escalates                               │
└──────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─ Phase 5: LLM judge (bucket-C only) ─────────────────────┐
│  inputs: normalized SSA + diff result + ref overlap      │
│  not raw source text                                     │
│  → verdict: duplicate / distinct / specialization        │
└──────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─ Phase 6: Verify substitutability ───────────────────────┐
│  for each CONSOLIDATE candidate (a, b):                  │
│    callers-a = go-callgraph-callers(a)                   │
│    callers-b = go-callgraph-callers(b)                   │
│    check signature compatibility for survivor pick       │
│  optionally: emit a define-belief asserting the merge,   │
│  so regression is detectable on future runs              │
└──────────────────────────────────────────────────────────┘
```

## Component Design

### Phase 1: Parse

No new code. Reuse a `GoSession` from `go-load` so all phases share the same
loaded packages snapshot.

```scheme
(define s (go-load target))
(define funcs (go-func-refs s))    ;; per-function reference profile
(define ssa   (go-ssa-build s))    ;; SSA functions
;; AST per function comes from typecheck if needed for ast-diff
```

### Phase 2: Cluster (FCA on references)

**New module:** `cmd/wile-goast/lib/wile/goast/dup-detect.scm`.

```scheme
;; Build the formal context: function -> set of qualified external refs
(define (function-ref-context func-refs)
  (context-from-alist
    (map (lambda (entry)
           (cons (func-ref-name entry)
                 (func-ref-uses entry)))
         func-refs)))

;; Suppress noise refs by IDF (reuse from (wile goast split))
(define (filter-noise-refs ctx threshold)
  ;; refs that appear in > (threshold * |objects|) functions are noise
  ;; (fmt.Sprintf, errors.New, builtin types, etc.)
  ...)

;; A concept whose extent has >1 function is a duplicate-candidate cluster.
(define (duplicate-candidate-concepts lattice min-extent)
  (filter (lambda (c) (>= (length (concept-extent c)) min-extent))
          lattice))
```

**Why FCA and not k-means / hierarchical clustering:** FCA produces a *lattice*
of concepts, not a flat partition. Each concept is a closed pair `(F, R)`
where every function in F uses every ref in R *and* every other function with
ref-set ⊇ R is in F. This closure property is exactly the duplicate
hypothesis: same maximal ref-set ⇒ same operation (modulo intentional
polymorphism). Flat clusterings have no such guarantee — they impose
boundaries that may not exist in the data.

**Why IDF weighting:** Without it, every Go function clusters around `fmt`,
`errors`, and the standard library. IDF (already in `(wile goast split)`)
suppresses ubiquitous attributes so the lattice reflects *informative*
co-dependence.

### Phase 3: Score (structural within cluster)

```scheme
(define (cluster-similarity-matrix concept ssa-funcs ast-funcs threshold)
  (let* ((funcs (concept-extent concept))
         (pairs (all-pairs funcs)))
    (map (lambda (pair)
           (let ((a (car pair)) (b (cadr pair)))
             (list a b
                   (ssa-equivalent?  (lookup ssa-funcs a) (lookup ssa-funcs b))
                   (unifiable?       (ast-diff (lookup ast-funcs a)
                                               (lookup ast-funcs b))
                                     threshold)
                   (diff-result-similarity
                     (ssa-diff (lookup ssa-funcs a) (lookup ssa-funcs b))))))
         pairs)))
```

All three predicates already exist in `(wile goast unify)` and
`(wile goast ssa-normalize)`. The only new code is the matrix-builder.

### Phase 4: Triage

```scheme
;; bucket-A: structural duplicate (no LLM needed)
;; bucket-B: algebraic duplicate (no LLM needed)
;; bucket-C: ref-clustered but structurally distant (LLM needed)
(define (triage similarity-row)
  (cond ((cadr  similarity-row) 'algebraic)         ;; ssa-equivalent?
        ((caddr similarity-row) 'structural)        ;; unifiable?
        ((> (cadddr similarity-row) min-overlap-similarity)
         'mixed)                                    ;; some structure
        (else 'semantic-candidate)))                ;; needs LLM
```

Triage is pure data. No LLM calls in this phase.

### Phase 5: LLM judge (bucket-C only)

Out of scope for the Scheme module. The output of Phase 4 is a JSON-shaped
report listing `semantic-candidate` pairs with their normalized SSA, diff
results, and ref overlap. A separate prompt (analogous to the existing
`goast-refactor` MCP prompt) feeds these to the LLM.

The key efficiency gain: **the LLM sees normalized SSA, not raw source text**,
and only the pairs where structural signals were inconclusive. For a 5K
function codebase, this is typically <5% of candidate pairs.

### Phase 6: Verify substitutability

```scheme
(define (verify-consolidate-candidate a b)
  (let ((callers-a (go-callgraph-callers cg a))
        (callers-b (go-callgraph-callers cg b))
        (sig-a (function-signature a))
        (sig-b (function-signature b)))
    (and (signatures-compatible? sig-a sig-b)
         (cons (pick-survivor a b callers-a callers-b)
               (append callers-a callers-b)))))
```

Optionally: emit a `define-belief` asserting that the consolidated cluster
should remain consolidated, so a future re-introduction of the duplicate is
caught by `run-beliefs`.

## Trade-offs and Open Questions

### What FCA-on-references catches and misses

| Pattern | Caught by reference-FCA? |
|---------|--------------------------|
| Same intent, same impl, same deps | YES (extent ≥ 2 in concept) |
| Same intent, similar impl, similar deps | YES (extent ≥ 2 in concept; structural diff confirms) |
| Same intent, different impl, *different* deps (e.g., regex vs manual parser) | NO — landed in different concepts |
| Different intent, same deps (e.g., two unrelated `fmt.Sprintf` callers) | YES, but bucket-A/B/C will eject — the structural diff will be low and the LLM will reject |

**The "same intent, different deps" miss is real.** Reference-profile
clustering biases toward similar implementation. To catch this case, a
*second* clustering signal is needed.

**Candidate second signal:** signature-shape grouping (already used in
`unify-detect-pkg.scm`). Two clustering dimensions, two duplicate-candidate
sets:

| Signal | Catches | Misses |
|--------|---------|--------|
| Reference-FCA | Similar dependencies → likely similar impl, possibly redundant | Different deps |
| Signature-shape | Same input/output types → likely same operation | Different signatures wrapping same intent |

Their **intersection** is the highest-confidence duplicate set. Their
**symmetric difference** is the genuinely hard case where the LLM judge has
the most to add.

**Open question:** should Phase 2 produce a *single* concept lattice
(reference-FCA only) or a *union of two prefilters* (reference-FCA ∪
signature-shape)? Recommendation: start with reference-FCA alone, measure
recall against a hand-labeled corpus, then decide whether to add the second
prefilter. Don't over-build.

### Concept relationships as quality signals

`(wile goast fca-algebra)` provides `concept-relationship` returning
`subconcept` / `superconcept` / `equal` / `incomparable`. Applied to the
duplicate-candidate concepts:

- **equal** concepts → exact duplicates of intent (highest confidence)
- **subconcept** of another → specialization (possibly intentional;
  caller may need the stricter contract)
- **superconcept** of another → generalization candidate (the survivor in
  CONSOLIDATE)
- **incomparable** → independent duplicate clusters (no merge between them)

This information is free — it falls out of the lattice the analysis already
constructs. The current pure-LLM approach has no equivalent.

### Cluster distance via path-algebra

Optional Phase 7 (deferred): apply `(wile goast path-algebra)` to compose
inter-cluster distances. Three weighted edge-functions (Jaccard on refs,
diff-score, equivalence boolean) feed three semirings (min-plus for
shortest similarity, max-min for bottleneck, Boolean for reachability).
Useful for ranking which cluster pairs to investigate first when there are
many. Out of scope for the initial implementation.

### Cost expectations (rough)

For a 5K-function codebase:

| Phase | Cost |
|-------|------|
| 1. Parse | one `go-load` + one `go-func-refs` + one `go-ssa-build` |
| 2. FCA cluster | NextClosure: O(\|L\| × \|F\| × \|R\|), typically seconds |
| 3. Score | pairwise within clusters; clusters are small (typically <20 funcs each) |
| 4. Triage | O(pairs), trivial |
| 5. LLM | only on bucket-C pairs, typically <5% of candidate pairs |
| 6. Verify | O(consolidate-candidates × callers) |

Phases 1-4 and 6 are pure Scheme. Phase 5 is the only LLM call, and its
prompt size is bounded by normalized SSA per pair (much smaller than raw
source text per pair).

## Files

| File | Purpose | Status |
|------|---------|--------|
| `cmd/wile-goast/lib/wile/goast/dup-detect.scm` | New: pipeline orchestration | Create |
| `cmd/wile-goast/lib/wile/goast/dup-detect.sld` | New: library definition | Create |
| `examples/goast-query/dup-detect-pkg.scm` | New: end-to-end example | Create |
| `cmd/wile-goast/prompts/goast-dup-detect.md` | New: MCP prompt for Phase 5 LLM judge | Create |
| `cmd/wile-goast/lib/wile/goast/unify.scm` | Reuse `unifiable?`, `ast-diff`, `ssa-diff`, `score-diffs`, `ssa-equivalent?` | Read-only |
| `cmd/wile-goast/lib/wile/goast/fca.scm` | Reuse `make-context`, `concept-lattice`, `concept-extent`, `concept-intent` | Read-only |
| `cmd/wile-goast/lib/wile/goast/fca-algebra.scm` | Reuse `concept-relationship` | Read-only |
| `cmd/wile-goast/lib/wile/goast/split.scm` | Reuse IDF weighting (`compute-idf`, `filter-noise`) | Read-only |
| `goast/prim_funcrefs.go` | Reuse `go-func-refs` | Read-only |

## Validation Plan

1. **Self-analysis on `wile-goast`.** Run on the project itself (≈300
   functions). Hand-validate the highest-confidence concepts. Compare against
   `unify-detect-pkg.scm` output.
2. **Synthetic corpus.** Build a small Go module with known duplicates: same
   impl, same algebra (different ops), same intent different deps.
   Measure recall and precision per bucket.
3. **Comparison against pure-LLM skill.** On the same Go corpus (translated
   from the TS skill's stated targets — utilities, validation, error
   formatting), measure: (a) candidate-set overlap, (b) confidence agreement
   on overlapping pairs, (c) cost (tokens spent).

## Status of Adjacent Plans

- `2026-04-12-ssa-equivalence-v2-design.md` — `ssa-equivalent?` and
  `discover-equivalences` shipped. Phase 4 bucket-B uses these directly.
- `2026-04-13-package-splitting-impl.md` — `(wile goast split)` IDF weighting
  shipped. Phase 2 reuses `compute-idf` and `filter-noise`.
- `2026-04-10-function-boundary-recommendations-impl.md` — `(wile goast
  fca-recommend)` shipped function-level FCA at the struct-field attribute
  granularity. This plan adds a peer module at the external-reference
  attribute granularity.

## Next Step

Implementation plan as `2026-04-17-fca-duplicate-detection-impl.md` once
this design is approved. Estimated 8-12 TDD tasks, primarily Scheme.
