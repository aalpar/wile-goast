# False Boundary Detection via Formal Concept Analysis

Discover boundaries (struct, interface, function, package) that prevent
unification, simplify state, or improve design coherence. The core insight:
instead of checking consistency *within* assumed-correct boundaries (the belief
DSL's role), discover what the *natural* boundaries are from access patterns,
then compare against actual code structure. Mismatches are false boundary
candidates.

**Status:** Design approved. Not yet implemented.

## Motivation

Existing wile-goast tools validate boundaries:
- **Unification detector** — finds similar functions across existing type/package lines
- **Belief DSL** — checks consistency patterns within established boundaries

Neither asks the prior question: *are these the right boundaries?* A false
boundary is one whose removal enables unification, simplifies state, or
improves coherence. Examples:

- Two structs whose fields are always modified in concert → fields should
  be colocated
- One struct's field always checked before operating on another's → implicit
  coupling that should be explicit
- Two functions whose tails/heads form a coherent unit → boundary drawn in
  the wrong place
- Interface implementations that restate the same logic with type substitution
  → the interface forces unnecessary duplication

The tool surfaces coupling signals with evidence. The user decides whether
a boundary removal is worthwhile.

## Foundations

**Formal Concept Analysis** (Wille, 1982; Ganter & Wille, 1999). Given a set
of objects, a set of attributes, and an incidence relation ("object has
attribute"), FCA constructs the unique *concept lattice* — the set of all
maximal groups where every object in the group shares every attribute in the
group.

A **formal context** is a triple (G, M, I):
- G = objects (functions)
- M = attributes (struct fields, qualified as `Type.Field`)
- I ⊆ G × M = incidence ("function f accesses field a")

A **concept** is a pair (A, B) where:
- A ⊆ G (the *extent* — which objects)
- B ⊆ M (the *intent* — which attributes)
- A = {g ∈ G | ∀m ∈ B: (g,m) ∈ I} (all objects having every attribute in B)
- B = {m ∈ M | ∀g ∈ A: (g,m) ∈ I} (all attributes shared by every object in A)

The **derivation operators** form a Galois connection:
- intent(S) = attributes shared by all objects in S
- extent(T) = objects that have all attributes in T
- extent ∘ intent is a closure operator on objects
- intent ∘ extent is a closure operator on attributes

A concept is a fixpoint of this closure. The set of all concepts ordered by
extent inclusion forms a complete lattice.

**Key property:** FCA discovers groupings from the data with no prior
assumptions about where boundaries should be. The discovered decomposition
is then compared against actual code boundaries.

**Related work:**
- Modular decomposition (graph theory) — modules are vertex sets
  indistinguishable from outside. Splitting a module is a false boundary.
- Reflexion models (Murphy, Notkin, Sullivan 1995) — compare intended
  architecture against actual dependencies. Divergences = false boundaries.
- Hypergraph partitioning (VLSI) — minimize edge cuts across partitions.
  High-cut partitions = false boundaries.
- Parnas (1972) — a boundary is justified iff it hides a design decision
  that could change.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Discovery engine | Formal Concept Analysis | Discovers natural groupings without assuming existing boundaries are correct |
| Lattice algorithm | NextClosure (Ganter 1984) | Generates concepts in lexicographic order, no redundancy, O(&#124;G&#124;·&#124;M&#124;·&#124;L&#124;) |
| Object granularity | Functions (summary-level) | `go-ssa-field-index` is function-level; if too coarse, decompose functions first and re-run |
| Attribute encoding | Mode parameter (`'read-write`, `'write-only`, `'type-only`) | Different encodings reveal different coupling; user selects per analysis |
| New Go primitives | None | `go-ssa-field-index` provides everything needed for Phase 1 |
| Scoring model | None — report evidence, user decides | Tool detects coupling, not correctness. Same philosophy as belief DSL. |
| Galois connection | Use `(wile algebra lattice)` | intent/extent are a Galois connection; existing algebra infrastructure applies |
| Phase 1 scope | Struct boundaries only | Highest priority, most concrete signals, validates concept before expanding |

## Phase Plan

| Phase | Boundary | Objects | Attributes | Status |
|-------|----------|---------|------------|--------|
| 1 | Struct | Functions | Struct fields accessed | This design |
| 2 | Interface | Implementations | Method bodies (via unification similarity) | Future |
| 3 | Function | Code blocks within functions | Variables/fields referenced | Future |
| 4 | Subsequent | Call-site pairs (f then g) | Fields written by f's tail ∩ read by g's head | Future |
| 5 | Package | Packages | Dependencies imported | Future (low priority — mostly covered by structural analysis) |

All phases reuse the same `concept-lattice` with different context construction.

## API

### Context Construction

```scheme
(make-context objects attributes incidence)
;; objects: list of symbols/strings
;; attributes: list of symbols/strings
;; incidence: (lambda (object attribute) -> boolean)
;; Returns: context

(context-from-alist '((func-a Type.X Type.Y) (func-b Type.Y Type.Z) ...))
;; Convenience: each entry is (object attr ...).
;; Objects and attributes derived from entries.
;; Returns: context

(field-index->context field-index mode)
;; field-index: result of (go-ssa-field-index session)
;; mode: 'read-write | 'write-only | 'type-only
;;   'read-write  — "Type.Field:r" / "Type.Field:w" (distinguishes access mode)
;;   'write-only  — "Type.Field" for writes only (pure co-mutation)
;;   'type-only   — "Type" only, no field (coarse type-level coupling)
;; Returns: context
```

### Derivation Operators

```scheme
(intent context object-set)
;; Returns: set of attributes shared by all objects in object-set

(extent context attribute-set)
;; Returns: set of objects that have all attributes in attribute-set
```

### Concept Lattice

```scheme
(concept-lattice context)
;; Returns: list of (extent . intent) pairs ordered by extent inclusion
;; Algorithm: NextClosure (Ganter 1984)

(concept-extent concept)   ;; car
(concept-intent concept)   ;; cdr
```

### Boundary Discovery

```scheme
(cross-boundary-concepts lattice)
;; Filter: concepts whose intent spans multiple struct types.
;; Returns: list of concepts with cross-boundary intents

(boundary-report concepts)
;; Format concepts as a displayable/structured report.
;; Each entry: struct types involved, fields, function evidence, extent coverage.
;; Returns: list of alists (structured, not printed — MCP-compatible)
```

### Filtering Parameters

```scheme
(cross-boundary-concepts lattice
  'min-extent 3          ;; concept must be backed by >= 3 functions
  'min-intent 2          ;; concept must span >= 2 fields
  'min-types 2)          ;; concept must span >= 2 struct types (default)
```

## NextClosure Algorithm

Generates all concepts in lexicographic order without redundancy.

**Lectic ordering:** Attribute sets are ordered lexicographically by their
characteristic vector. For attributes {a₁, a₂, ..., aₙ} with a fixed
ordering, set A < set B iff the smallest attribute where they differ is
in B but not A.

**Closure operator:** For attribute set B, the closure is
intent(extent(B)) — "close" B by finding all objects that have every
attribute in B, then finding all attributes shared by those objects.

**NextClosure iteration:**

```
1. Start with closure of ∅ (the bottom concept's intent)
2. To find the next intent after B:
   a. For i = |M| down to 1:
      - Let B_i = (B ∩ {a₁,...,a_{i-1}}) ∪ {aᵢ}
      - Let C = closure(B_i)
      - If C is lectically greater than B and agrees with B_i
        on {a₁,...,aᵢ}: return C
   b. If no next found: done
3. For each intent B, the extent is extent(B)
```

This produces exactly the set of all concepts, each generated once. No
redundancy, no pruning needed.

**Complexity:** O(|G| · |M| · |L|) where |L| is the number of concepts.
For typical Go codebases (hundreds of functions, tens of struct fields),
this is fast.

## Boundary Comparison

For each concept (A, B) in the lattice:

1. **Group B by struct type:** Parse `"Type.Field"` attributes into
   `{Type → [Field ...]}` buckets.
2. **Count types:** If only one struct type, skip — no boundary crossing.
3. **Compute evidence:** A is the set of functions that access *all* fields
   in B. These functions treat the cross-boundary fields as a unit.
4. **Compute coverage:** For each struct type T in the concept, count how
   many functions in the *entire* codebase access any field of T. The ratio
   |A| / |total T-accessors| shows how dominant the coupling is.

**Contrast with single-struct concepts:** If the same fields appear in a
strictly larger concept that's entirely within one struct, the cross-boundary
signal is weaker — the coupling is a subset of a natural single-struct
cluster. Report but flag as "subset of single-struct concept."

## Output Format

Structured alist, compatible with MCP `eval` tool:

```scheme
((boundary
   (types ("Cache" "Index"))
   (fields (("Cache" "Entries" write) ("Cache" "TTL" write)
            ("Index" "Keys" write)))
   (functions ("pkg.UpdateBoth" "pkg.Invalidate" "pkg.Rebuild" "pkg.Sync"))
   (extent-size 4)
   (coverage (("Cache" 4 6) ("Index" 4 5)))  ;; (type coupled-fns total-fns)
   (notes ())))
```

No scoring, no recommendations. Evidence only.

## File Layout

New files:

- `cmd/wile-goast/lib/wile/goast/fca.scm` — implementation
- `cmd/wile-goast/lib/wile/goast/fca.sld` — library definition

Testdata:

- `examples/goast-query/testdata/falseboundary/` — synthetic Go package
  with two structs whose fields are always co-modified

No changes to existing files except adding `fca.sld` to the embed directive
in `cmd/wile-goast/main.go`.

## Exports

```scheme
(define-library (wile goast fca)
  (import (scheme base) (wile algebra lattice) (wile goast utils))
  (export
    ;; Context construction
    make-context
    context-from-alist
    field-index->context

    ;; Derivation operators
    intent
    extent

    ;; Concept lattice
    concept-lattice
    concept-extent
    concept-intent

    ;; Boundary discovery
    cross-boundary-concepts
    boundary-report)
  (include "fca.scm"))
```

## Dependencies

- `(wile algebra lattice)` — set operations (powerset membership, intersection).
  The derivation operators are a Galois connection in the existing sense.
- `(wile goast utils)` — `nf`, `tag?` for field-index traversal
- `go-ssa-field-index` — provides the raw field access data (existing primitive)
- `go-load` — session sharing for package loading (existing primitive)

No new Go primitives. No new dependencies.

## Testing Strategy

1. **Unit tests for FCA core:** Small hand-constructed contexts (5-10 objects,
   5-10 attributes). Verify concept lattice matches known results. These can
   run without Go packages — pure Scheme.

2. **Synthetic Go testdata:** Package with two structs `Cache` and `Index`
   where 4 functions always modify fields from both. Run full pipeline:
   `go-load` → `go-ssa-field-index` → `field-index->context` →
   `concept-lattice` → `cross-boundary-concepts`. Verify the cross-boundary
   concept is discovered.

3. **Negative case:** Package with two structs that are legitimately separate
   (no co-access). Verify no false positives.

## Relationship to Other Tracks

- **Belief DSL (C6):** Confirmed false boundaries can graduate to beliefs.
  "These fields should always be co-mutated" becomes a `define-belief` that
  enforces the discovered coupling. FCA discovers; beliefs enforce.
- **Unification detector:** Interface boundary detection (Phase 2) combines
  FCA with the existing `ast-diff`/`ssa-diff` — FCA finds implementations
  that cluster together, unification scoring confirms they're type-substitution
  clones.
- **Dataflow (C2):** Phase 4 (subsequent detection) may use dataflow analysis
  to track field state across function call boundaries.
- **C4 (path algebra on call graphs):** Package boundary detection (Phase 5)
  could use call graph reachability as an additional signal alongside FCA.
