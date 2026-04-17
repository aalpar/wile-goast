# Symbolic Algebra Integration — wile-goast

Phase 3 of the wile symbolic algebra plan (`wile/plans/2026-04-10-symbolic-algebra-design.md`).
Consumes `(wile algebra symbolic)` to add algebraic reasoning to three existing
wile-goast systems: Go boolean expression simplification, belief equivalence
detection, and FCA concept lattice annotation.

**Status:** Complete. All three tasks implemented (2026-04-10).
**Depends on:** wile `(wile algebra symbolic)`, `(wile algebra rewrite)` v2 (absorption + associativity axioms)
**Source:** Tasks 13-15 from `wile/plans/2026-04-10-symbolic-algebra-impl.md`

## Dependency: `(wile algebra symbolic)`

These tasks require the following from wile (not yet landed):

| Export | Library | Phase |
|--------|---------|-------|
| `boolean->theory` | `(wile algebra symbolic)` | 1 |
| `lattice->theory` | `(wile algebra symbolic)` | 1 |
| `make-recursive-normalizer` | `(wile algebra symbolic)` | 1 |
| `sexp-term-protocol` | `(wile algebra symbolic)` | 1 |
| `format-trace` | `(wile algebra symbolic)` | 1 |
| `make-named-axiom` | `(wile algebra symbolic)` | 1 |
| `make-theory` | `(wile algebra symbolic)` | 1 |
| `discover-equivalences` | `(wile algebra symbolic)` | 2 |
| `make-absorption-axiom` | `(wile algebra rewrite)` | 1 |
| `make-associativity-axiom` | `(wile algebra rewrite)` | 1 |

Once wile lands these, bump `go.mod` dependency and verify `StdLibFS` includes
the new `.sld`/`.scm` files.

---

## Task 1: Boolean expression term protocol for Go AST

**Goal:** Normalize Go boolean expressions extracted from SSA conditions. Detect
redundant conditions (e.g., `x != nil && (x != nil || y > 0)` simplifies to
`x != nil` via absorption).

**Motivation (from wile design doc):**

```
Go: x != nil && (x != nil || y > 0)
Symbolic: (and (not-nil x) (or (not-nil x) (gt y 0)))

Boolean normalization via boolean->theory:
  absorption (a ∧ (a ∨ b) = a): → (not-nil x)

Report: "redundant disjunction — absorption law"
```

**What exists:** SSA conditions appear as `ssa-if` nodes with boolean operands.
The SSA layer exposes `ssa-binop` (for `&&`, `||`) and `ssa-unop` (for `!`)
with operand names. The belief DSL's `checked-before-use` checker already
walks SSA conditions via `defuse-reachable?`.

**What's new:**

1. **Projection function:** `go-condition->sexp` — walks an SSA boolean
   expression tree and produces an S-expression term using symbolic operators
   (`and`, `or`, `not`, plus domain predicates like `not-nil`, `gt`, `eq`).

2. **Normalizer setup:** Construct a Boolean algebra theory from
   `boolean->theory`, build a `make-recursive-normalizer`, apply to the
   projected term.

3. **Integration point:** New Scheme library `(wile goast boolean-simplify)`
   that composes the projection with the normalizer. Single entry point:
   `(simplify-condition ssa-fn condition-register)` → `(values simplified-sexp trace)`.

**Files:**
- Create: `cmd/wile-goast/lib/wile/goast/boolean-simplify.sld`
- Create: `cmd/wile-goast/lib/wile/goast/boolean-simplify.scm`
- Create: `goast/boolean_simplify_test.go`
- Testdata: extend existing `checking` testdata or create new package with
  redundant boolean conditions

**Key design choices:**

- **Atom identity:** Two SSA expressions are "equal" for absorption purposes
  when they refer to the same SSA register name. This is conservative —
  aliased values that are semantically equal but have different register names
  won't be simplified. SSA's single-assignment property makes register name
  equality stronger than source-level variable equality.

- **Scope:** Single-function conditions only. Cross-function condition
  correlation (e.g., caller checks `x != nil`, callee checks again) requires
  interprocedural analysis and is deferred.

- **Output:** The normalized S-expression is the primary result. The trace
  (from `format-trace`) provides human-readable justification. Both are
  returned as values for MCP tool consumption.

**Testing strategy:**

- Synthetic Go functions with redundant conditions:
  - `if x != nil && (x != nil || y > 0)` → simplifies via absorption
  - `if !(!(x > 0))` → simplifies via involution
  - `if x != nil && x != nil` → simplifies via idempotence
- Negative case: `if x != nil && y > 0` → no simplification (irreducible)
- Verify trace contains correct rule names

---

## Task 2: Belief predicate symbolic representation

**Goal:** Detect equivalent beliefs. Two beliefs whose predicates normalize to
the same symbolic form under Boolean algebra laws are semantically equivalent,
even if written differently.

**Motivation (from wile design doc):**

```scheme
;; Belief A: "functions calling Lock also call Unlock"
;;   Symbolic: (and (calls "Lock") (calls "Unlock"))

;; Belief B: "functions calling Unlock also call Lock"
;;   Symbolic: (and (calls "Unlock") (calls "Lock"))

;; Boolean normalization: commutativity makes these identical.
```

**What exists:** The belief DSL (`(wile goast belief)`) defines beliefs with
site selectors and property checkers. Each belief produces verdicts (adherence /
violation) per site. There is no symbolic representation of what a belief
*means* algebraically.

**What's new:**

1. **Symbolic projection:** Each belief's checker maps to a symbolic term.
   - `(contains-call "Lock")` → `(calls "Lock")`
   - `(paired-with "Lock" "Unlock")` → `(and (calls "Lock") (calls "Unlock"))`
   - `(ordered "A" "B")` → `(ordered "A" "B")` (opaque — not boolean-reducible)
   - `(checked-before-use "x")` → `(guarded "x")` (opaque)
   - `(custom ...)` → `(custom ...)` (opaque, no normalization)

2. **Equivalence detection:** After constructing symbolic terms for all defined
   beliefs, normalize each under `boolean->theory`. Group by normal form.
   Beliefs in the same group are equivalent.

3. **Integration point:** New export from belief DSL or a companion library:
   `(belief-equivalences package-pattern)` → list of equivalence classes,
   each a list of belief names whose symbolic forms are identical after
   normalization.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm` (add symbolic projection)
- Modify: `cmd/wile-goast/lib/wile/goast/belief.sld` (export new procedure)
- Modify: `goast/belief_integration_test.go` (add equivalence detection test)

**Key design choices:**

- **Opaque predicates:** Checkers like `ordered` and `checked-before-use` have
  no Boolean decomposition — they're atomic propositions. Two beliefs are only
  equivalent if their opaque predicates match *and* their Boolean structure
  normalizes to the same form.

- **Checker → term mapping:** This is a pattern match on the checker type.
  Only boolean-combinable checkers (`contains-call`, `paired-with`, `co-mutated`)
  produce decomposable terms. The rest are atoms.

- **When to run:** `belief-equivalences` runs after `define-belief` calls but
  before `run-beliefs`. It's a static analysis of the belief definitions
  themselves, not of the target code.

**Testing strategy:**

- Define two beliefs with commuted predicates → detected as equivalent
- Define two beliefs with different predicates → not equivalent
- Define a belief with opaque predicates → stays atomic, no false equivalences
- Verify output groups belief names correctly

---

## Task 3: FCA concept lattice algebraic annotation

**Goal:** Annotate FCA `boundary-report` output with lattice-algebraic
relationships between concepts. Instead of just listing cross-boundary concepts,
explain *how* they relate in the concept lattice using lattice theory vocabulary.

**Motivation (from wile design doc):**

```scheme
;; C1: intent = {Cache.Entries, Cache.TTL}
;; C2: intent = {Cache.Entries, Cache.TTL, Index.Keys, Index.Version}
;;
;; C1's intent ⊆ C2's intent → C1 is a superconcept of C2
;; C2's intent = C1's intent ∧ {Index.Keys, Index.Version}
;;
;; Report: "The Cache+Index concept extends the Cache-only concept
;;          by adding Index fields — meet in the concept lattice"
```

**What exists:** `(wile goast fca)` computes concept lattices and produces
`boundary-report` output as structured alists. `(wile goast fca-recommend)`
(from the function boundary recommendations plan) adds split/merge/extract
recommendations with Pareto ranking.

**What's new:**

1. **Lattice theory projection:** Apply `lattice->theory` from
   `(wile algebra symbolic)` to the concept lattice's natural ordering.
   The concept lattice *is* a lattice — its join and meet are defined by
   extent union/intersection.

2. **Relationship annotation:** For each pair of concepts in a boundary
   report, describe their relationship:
   - **Subconcept:** C1 ≤ C2 (C1's extent ⊆ C2's extent)
   - **Meet:** C1 ∧ C2 = C3 (the greatest concept below both)
   - **Join:** C1 ∨ C2 = C4 (the least concept above both)
   - **Incomparable:** neither ≤ the other (already used by split detection)

3. **Integration:** Extend `boundary-report` (or a new `annotated-boundary-report`)
   to include `(lattice-relation ...)` entries alongside existing fields.

**Files:**
- Modify or extend: `cmd/wile-goast/lib/wile/goast/fca.scm`
- Modify or extend: `cmd/wile-goast/lib/wile/goast/fca.sld`
- Modify: `goast/fca_test.go` (annotation tests)

**Key design choices:**

- **Concept lattice as a wile lattice:** The concept lattice from `concept-lattice`
  is a list of `(extent . intent)` pairs. To use `lattice->theory`, we need to
  construct a `(wile algebra lattice)` instance from it. This requires defining
  join and meet operations on concepts. Join = concept whose extent is the union
  of extents (closed under the Galois connection). Meet = concept whose intent
  is the union of intents (closed).

- **Eagerness:** Computing all pairwise relationships is O(n^2) in the number
  of concepts. For typical Go packages (10-50 concepts), this is fine. For
  large lattices (100+), limit to cross-boundary concepts only.

- **Output format:** Extend the boundary-report alist:

  ```scheme
  ((types ("Cache" "Index"))
   (fields ...)
   (functions ...)
   (extent-size 3)
   (lattice-relations
     ((subconcept-of "Cache-only concept"
        "The Cache+Index concept specializes the Cache-only concept"))))
  ```

**Testing strategy:**

- Small hand-constructed lattice with known ordering → verify annotations match
- falseboundary testdata → verify cross-boundary concepts get correct
  subconcept/incomparable annotations
- Verify that `lattice->theory` can normalize symbolic lattice expressions
  (e.g., `(join C1 (meet C1 C2))` → `C1` via absorption in the concept lattice)

---

## Phasing within wile-goast

| Task | Blocked on | Effort | Priority |
|------|-----------|--------|----------|
| 1 — Boolean simplification | wile Phase 1 (`boolean->theory`, normalizer) | ~120 lines Scheme + 80 lines Go test | High — directly actionable on SSA conditions |
| 2 — Belief equivalence | wile Phase 1 + Task 1 (for term protocol patterns) | ~80 lines Scheme + 60 lines Go test | Medium — useful for large belief sets |
| 3 — FCA annotation | wile Phase 1 (`lattice->theory`) | ~100 lines Scheme + 60 lines Go test | Medium — enriches existing output |

Tasks 1 and 3 are independent. Task 2 depends on the term protocol patterns
established in Task 1 but could be done in parallel with a shared design.

## Relationship to Other Plans

- **wile `2026-04-10-symbolic-algebra-design.md`** — the parent design. This
  plan implements the wile-goast consumer integration described in its Phase 3.
- **`2026-04-08-false-boundary-detection-design.md`** — FCA design. Task 3
  adds algebraic annotation to FCA output.
- **`2026-04-10-function-boundary-recommendations-design.md`** — Task 3's
  annotations could enrich split/merge/extract recommendations with lattice
  relationship context.
- **`plans/CONSISTENCY-DEVIATION.md`** — Task 2's belief equivalence detection
  extends the belief validation story.
