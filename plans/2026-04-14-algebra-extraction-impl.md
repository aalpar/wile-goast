# Algebra Extraction: wile-goast → wile

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract four general-purpose algebra modules from wile-goast into wile's `(wile algebra)` library, then rewire wile-goast to import them.

**Architecture:** Each module follows the same pattern: create `.sld` + `.scm` in wile's `stdlib/lib/wile/algebra/`, add test file in wile's `test/wile/`, update the umbrella `algebra.sld`, then rewire wile-goast's imports. Local development uses `go.work` so wile changes are immediately visible to wile-goast.

**Tech Stack:** Scheme (R7RS), wile algebra conventions (`define-record-type`, `(chibi test)` framework, structured docstrings)

**Repos:**
- wile: `/Users/aalpar/projects/wile-workspace/wile`
- wile-goast: `/Users/aalpar/projects/wile-workspace/wile-goast`

**Conventions (wile):**
- `.sld` imports `(scheme base)` plus algebra deps, has `(description ...)`, ends with `(include "file.scm")`
- `.scm` uses `;;;` header, `define-record-type` with `<angle-brackets>`, internal constructor `*` suffix
- Docstrings: Description, Parameters, Returns, Category: `algebra`, Examples, See also
- Tests: `(chibi test)` with `test-begin`/`test-group`/`test`/`test-end`/`test-exit`
- Test files auto-discovered by `test/run-all.sh` (any `*-test.scm`)
- No `filter-map` in `(scheme base)` — define needed utilities locally in `.scm`

---

## Phase 1: Formal Concept Analysis

### Task 1: Create `(wile algebra fca)` — sorted sets + context + Galois operators + NextClosure

**Files:**
- Create: `stdlib/lib/wile/algebra/fca.sld` (in wile)
- Create: `stdlib/lib/wile/algebra/fca.scm` (in wile)

**What moves from wile-goast `fca.scm`:**
- Sorted string set operations: `sort-strings`, `set-add`, `set-intersect`, `set-member?`, `set-union`, `set-subset?`, `set-before` (lines 24-80)
- Context construction: `make-context`, `context-from-alist`, `context-objects`, `context-attributes` (lines 82-139)
- Galois operators: `intent`, `extent` (lines 141-167)
- Concept lattice: `concept-extent`, `concept-intent`, `fca-close`, `next-closure`, `concept-lattice` (lines 169-213)

**What moves from wile-goast `fca-algebra.scm`:**
- `concept-lattice->algebra-lattice` (lines 38-70) — bridges FCA → `(wile algebra lattice)`
- `concept-relationship` (lines 76-85) — subconcept/superconcept/equal/incomparable

**Key changes from goast version:**
1. Replace tagged-alist context with `define-record-type <fca-context>`
2. Replace `nf` calls with record accessors
3. Replace `member?` (from utils) with local `(define (member? x lst) (and (member x lst) #t))`
4. Replace `filter-map` with local definition
5. Replace `unique` with local definition (uses sorted sets)
6. `make-context` builds record instead of tagged alist
7. `context-from-alist` — keep as convenience constructor
8. `intent`/`extent` access hashtables via record accessors instead of `nf`

**`.sld` exports:**

```scheme
(define-library (wile algebra fca)
  (description "Formal Concept Analysis: contexts, Galois connections, concept lattices (NextClosure, Ganter 1984).")
  (export
    ;; Context
    make-context context-from-alist fca-context?
    context-objects context-attributes
    ;; Galois operators
    intent extent
    ;; Concept lattice
    concept-lattice concept-extent concept-intent
    ;; Algebra bridge
    concept-lattice->algebra-lattice concept-relationship
    ;; Sorted string sets (used by downstream)
    set-add set-intersect set-union set-subset? set-member? set-before
    sort-strings)
  (import (scheme base)
          (wile algebra lattice)
          (wile algebra closure))
  (include "fca.scm"))
```

**`.scm` structure:**

```scheme
;;; (wile algebra fca) — Formal Concept Analysis
;;;
;;; Discovers formal concepts (closed object-attribute pairs) from a
;;; binary relation via the NextClosure algorithm (Ganter, 1984).
;;; Concept lattices are the mathematical dual of Galois connections
;;; applied to finite contexts.

;;; ── Local utilities ──────────────────────────────────────

(define (filter-map f lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (let ((v (f (car xs))))
        (loop (cdr xs) (if v (cons v acc) acc))))))

(define (member? x lst)
  (and (member x lst) #t))

;;; ── Sorted string sets ───────────────────────────────────
;; [set-add, set-intersect, set-union, set-subset?, set-member?, set-before, sort-strings]
;; Identical to goast version — pure string-comparison operations

;;; ── Context ──────────────────────────────────────────────

(define-record-type <fca-context>
  (make-fca-context objects attributes obj->attrs attr->objs)
  fca-context?
  (objects    context-objects)
  (attributes context-attributes)
  (obj->attrs fca-context-obj->attrs)
  (attr->objs fca-context-attr->objs))

(define (make-context objects attributes incidence)
  "Build an FCA context from objects, attributes, and an incidence function.
   ...Category: algebra..."
  ;; Same logic as goast version but constructs record type
  ...)

(define (context-from-alist entries)
  "Build an FCA context from an association list.
   Each entry is (object attr1 attr2 ...).
   ...Category: algebra..."
  ...)

;;; ── Galois operators ─────────────────────────────────────
;; [intent, extent — same logic, use record accessors]

;;; ── Concept lattice (NextClosure) ────────────────────────
;; [concept-extent, concept-intent, fca-close, next-closure, concept-lattice]

;;; ── Algebra bridge ───────────────────────────────────────
;; [find-concept-by-intent, concept-lattice->algebra-lattice, concept-relationship]
;; From goast's fca-algebra.scm. Uses (wile algebra lattice) and (wile algebra closure).
```

**Step 1: Write the test file**

Create `test/wile/algebra-fca-test.scm` in wile:

```scheme
;;; algebra-fca-test.scm — Formal Concept Analysis tests

(import (scheme base)
        (chibi test)
        (wile algebra fca)
        (wile algebra lattice))

(test-begin "fca")

(test-group "sorted-sets"
  (test '("a" "b" "c") (sort-strings '("c" "a" "b")))
  (test '("a" "b" "c") (sort-strings '("a" "b" "b" "c")))
  (test '("a" "b") (set-intersect '("a" "b" "c") '("a" "b" "d")))
  (test '("a" "b" "c" "d") (set-union '("a" "c") '("b" "d")))
  (test #t (set-subset? '("a" "b") '("a" "b" "c")))
  (test #f (set-subset? '("a" "d") '("a" "b" "c")))
  (test #t (set-member? "b" '("a" "b" "c")))
  (test #f (set-member? "d" '("a" "b" "c"))))

(test-group "context"
  (let ((ctx (context-from-alist
               '(("f1" "A" "B") ("f2" "A" "C") ("f3" "B" "C")))))
    (test #t (fca-context? ctx))
    (test '("f1" "f2" "f3") (context-objects ctx))
    (test '("A" "B" "C") (context-attributes ctx))))

(test-group "galois-operators"
  (let ((ctx (context-from-alist
               '(("f1" "A" "B") ("f2" "A" "C") ("f3" "B" "C")))))
    ;; intent of {f1, f2} = attributes shared by both = {A}
    (test '("A") (intent ctx '("f1" "f2")))
    ;; extent of {A} = objects having A = {f1, f2}
    (test '("f1" "f2") (extent ctx '("A")))
    ;; vacuous: empty object-set → all attributes
    (test '("A" "B" "C") (intent ctx '()))
    ;; vacuous: empty attribute-set → all objects
    (test '("f1" "f2" "f3") (extent ctx '()))))

(test-group "concept-lattice"
  (let* ((ctx (context-from-alist
                '(("f1" "A" "B") ("f2" "A" "C") ("f3" "B" "C"))))
         (lattice (concept-lattice ctx)))
    ;; lattice is non-empty
    (test #t (> (length lattice) 0))
    ;; each concept is (extent . intent) pair
    (test #t (pair? (car lattice)))
    ;; top concept: all objects, shared attributes
    (let ((top (car (filter (lambda (c) (= (length (concept-extent c)) 3)) lattice))))
      (test '("f1" "f2" "f3") (concept-extent top))
      (test '() (concept-intent top)))))

(test-group "algebra-bridge"
  (let* ((ctx (context-from-alist
                '(("f1" "A" "B") ("f2" "A" "C") ("f3" "B" "C"))))
         (concepts (concept-lattice ctx))
         (alg (concept-lattice->algebra-lattice ctx concepts)))
    ;; bridge produces a valid lattice
    (test #t (and (lattice-top alg) #t))
    (test #t (and (lattice-bottom alg) #t))))

(test-group "concept-relationship"
  (let* ((ctx (context-from-alist
                '(("f1" "A" "B") ("f2" "A" "C") ("f3" "B" "C"))))
         (concepts (concept-lattice ctx)))
    ;; equal to itself
    (test 'equal (concept-relationship (car concepts) (car concepts)))
    ;; check that incomparable relationships exist in this lattice
    (let ((rels (map (lambda (c) (concept-relationship (car concepts) c)) concepts)))
      (test #t (> (length rels) 0)))))

(test-end)
(test-exit)
```

**Step 2: Run test — should fail (module doesn't exist)**

Run: `cd /Users/aalpar/projects/wile-workspace/wile && ./dist/wile test/wile/algebra-fca-test.scm`
Expected: Error — library `(wile algebra fca)` not found.

**Step 3: Implement `fca.sld` + `fca.scm`**

Create both files as described above. Port the code from goast's `fca.scm` (lines 21-213) and `fca-algebra.scm` (lines 22-85), making the changes listed above (record type, local utilities, docstrings with `Category: algebra`).

**Step 4: Run test — should pass**

Run: `cd /Users/aalpar/projects/wile-workspace/wile && ./dist/wile test/wile/algebra-fca-test.scm`
Expected: All tests pass.

**Step 5: Commit in wile**

```
feat(algebra): add formal concept analysis module

(wile algebra fca) — NextClosure algorithm (Ganter 1984),
Galois connection operators (intent/extent), and concept
lattice construction with algebra bridge.
```

---

### Task 2: Update wile's umbrella `algebra.sld`

**Files:**
- Modify: `stdlib/lib/wile/algebra.sld` (in wile)

**Step 1: Add FCA imports and exports**

Add to the import list:
```scheme
(wile algebra fca)
```

Add to the export list (with comment section):
```scheme
;; Formal Concept Analysis
make-context context-from-alist fca-context?
context-objects context-attributes
intent extent
concept-lattice concept-extent concept-intent
concept-lattice->algebra-lattice concept-relationship
set-add set-intersect set-union set-subset? set-member? set-before
sort-strings
```

**Step 2: Run full algebra test suite**

Run: `cd /Users/aalpar/projects/wile-workspace/wile && make test-scheme`
Expected: All tests pass (including new FCA tests).

**Step 3: Commit in wile**

```
chore(algebra): add fca to umbrella re-exports
```

---

### Task 3: Rewire wile-goast FCA imports

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca.sld` (wile-goast)
- Modify: `cmd/wile-goast/lib/wile/goast/fca.scm` (wile-goast)
- Modify: `cmd/wile-goast/lib/wile/goast/fca-algebra.sld` (wile-goast)
- Modify: `cmd/wile-goast/lib/wile/goast/fca-algebra.scm` (wile-goast)
- Modify: `cmd/wile-goast/lib/wile/goast/fca-recommend.sld` (wile-goast)

**Step 1: Update `fca.sld`**

Add `(wile algebra fca)` to imports. Remove from the export list everything that now comes from `(wile algebra fca)` — keep only Go-specific exports (`field-index->context`, `propagate-field-writes`, `cross-boundary-concepts`, `boundary-report`). Re-export `(wile algebra fca)` names that downstream goast modules need (set ops, concept-extent/intent, intent/extent, context accessors, concept-lattice).

**Step 2: Update `fca.scm`**

Delete the code that moved to wile:
- Sorted string set operations (lines 24-80)
- Context construction (lines 82-139) — `make-context`, `context-from-alist`, accessors
- Galois operators (lines 141-167) — `intent`, `extent`
- Concept lattice (lines 169-213) — `concept-extent`, `concept-intent`, `fca-close`, `next-closure`, `concept-lattice`

Keep:
- SSA bridge: `field-access-attr`, `attr-type-count`, `field-index->context`
- Call graph propagation: `cg-adjacency`, `cg-topo-sort`, `propagate-field-writes`, etc.
- Cross-boundary: `cross-boundary-concepts`, `boundary-report`, `group-fields-by-struct`, `attr-struct-name`

These now use imported names from `(wile algebra fca)` for set ops, intent, extent, concept accessors, etc.

**Step 3: Update `fca-algebra.sld`**

Add `(wile algebra fca)` to imports (for `concept-relationship`, `concept-lattice->algebra-lattice`). Remove these from the export list — they're now re-exported from the wile import. Keep only `annotated-boundary-report` and `concept-summary` as goast-specific exports.

**Step 4: Update `fca-algebra.scm`**

Delete: `find-concept-by-intent`, `concept-lattice->algebra-lattice`, `concept-relationship` — now from `(wile algebra fca)`.

Keep: `concept-summary`, `annotated-boundary-report` — these use dot-notation to extract Go type names from attributes, which is domain-specific.

**Step 5: Update `fca-recommend.sld`**

Add `(wile algebra fca)` to imports if it now needs set operations or concept accessors that `(wile goast fca)` no longer re-exports. Alternatively, if `(wile goast fca)` re-exports them, no change needed.

**Step 6: Run wile-goast tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "FCA|Fca|fca" -v -count=1`
Expected: All FCA tests pass (existing integration tests validate the rewiring).

**Step 7: Run full CI**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: All pass.

**Step 8: Commit in wile-goast**

```
refactor(fca): import core FCA from (wile algebra fca)

Context, Galois operators, concept lattice, and algebra bridge
now come from wile's algebra library. Go-specific bridges
(field-index->context, propagate-field-writes, cross-boundary
detection) remain in (wile goast fca).
```

---

## Phase 2: Pareto Dominance

### Task 4: Create `(wile algebra pareto)` in wile

**Files:**
- Create: `stdlib/lib/wile/algebra/pareto.sld` (in wile)
- Create: `stdlib/lib/wile/algebra/pareto.scm` (in wile)
- Create: `test/wile/algebra-pareto-test.scm` (in wile)

**What moves from wile-goast `fca-recommend.scm`:**
- `factor-leq?`, `factor-less?` (lines 36-42) — mixed boolean/numeric comparison
- `dominates?` (lines 47-56) — Pareto dominance check
- `pareto-frontier` (lines 62-93) — frontier + dominated groups

**`.sld`:**

```scheme
(define-library (wile algebra pareto)
  (description "Pareto dominance and multi-objective frontier computation.")
  (export dominates? pareto-frontier
          factor-leq? factor-less?)
  (import (scheme base))
  (include "pareto.scm"))
```

No algebra dependencies — these are self-contained partial-order operations on factor alists.

**Key changes from goast version:**
- Remove local `filter` definition (define locally or use `(scheme base)` facilities)
- Add structured docstrings with `Category: algebra`
- The `member?` call in `pareto-frontier` line 80 needs a local definition or replacement

**Step 1: Write test file**

```scheme
;;; algebra-pareto-test.scm — Pareto dominance tests

(import (scheme base) (chibi test) (wile algebra pareto))

(test-begin "pareto")

(test-group "factor-comparison"
  (test #t (factor-leq? #f #t))
  (test #t (factor-leq? #f #f))
  (test #f (factor-leq? #t #f))
  (test #t (factor-leq? 3 5))
  (test #t (factor-leq? 5 5))
  (test #f (factor-leq? 7 5)))

(test-group "dominance"
  ;; X dominates Y: >= on all, > on at least one
  (test #t (dominates?
             '((a . 5) (b . 3))
             '((a . 4) (b . 2))))
  ;; equal — no strict improvement
  (test #f (dominates?
             '((a . 5) (b . 3))
             '((a . 5) (b . 3))))
  ;; incomparable — each wins on one factor
  (test #f (dominates?
             '((a . 5) (b . 2))
             '((a . 4) (b . 3)))))

(test-group "frontier"
  (let ((result (pareto-frontier
                  '(("x" ((a . 5) (b . 3)))
                    ("y" ((a . 4) (b . 2)))
                    ("z" ((a . 3) (b . 4))))
                  '(a b))))
    ;; x and z are on frontier (incomparable), y is dominated by x
    (test 2 (length (cdr (assoc 'frontier result))))
    (test #t (and (member "x" (cdr (assoc 'frontier result))) #t))
    (test #t (and (member "z" (cdr (assoc 'frontier result))) #t))))

(test-end)
(test-exit)
```

**Step 2: Run test — fail, Step 3: Implement, Step 4: Run test — pass**

Run: `cd /Users/aalpar/projects/wile-workspace/wile && ./dist/wile test/wile/algebra-pareto-test.scm`

**Step 5: Update `algebra.sld` umbrella**

Add `(wile algebra pareto)` import and exports.

**Step 6: Commit in wile**

```
feat(algebra): add pareto dominance module
```

---

### Task 5: Rewire wile-goast Pareto imports

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca-recommend.sld` (wile-goast)
- Modify: `cmd/wile-goast/lib/wile/goast/fca-recommend.scm` (wile-goast)

**Step 1: Update `fca-recommend.sld`**

Add `(wile algebra pareto)` to imports. Remove `dominates?`, `pareto-frontier`, `factor-leq?`, `factor-less?` from exports (if exported) — or just delete them from the implementation.

**Step 2: Update `fca-recommend.scm`**

Delete: `factor-leq?`, `factor-less?`, `dominates?`, `pareto-frontier` and the local `filter` utility.
Keep: all candidate detection functions (`split-candidates`, `merge-candidates`, `extract-candidates`, `boundary-recommendations`), SSA helpers, lattice analysis utilities.

**Step 3: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "Recommend|recommend" -v -count=1`
Expected: All pass.

**Step 4: Full CI**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`

**Step 5: Commit in wile-goast**

```
refactor(fca-recommend): import pareto from (wile algebra pareto)
```

---

## Phase 3: Interval Arithmetic

### Task 6: Create `(wile algebra interval)` in wile

**Files:**
- Create: `stdlib/lib/wile/algebra/interval.sld` (in wile)
- Create: `stdlib/lib/wile/algebra/interval.scm` (in wile)
- Create: `test/wile/algebra-interval-test.scm` (in wile)

**What moves from wile-goast `domains.scm`:**
- Infinity-aware comparison: `inf<=`, `inf<`, `inf-min`, `inf-max` (lines ~268-281)
- Infinity-aware arithmetic: `inf+`, `inf-`, `inf*` (lines ~283-314)
- Interval lattice: `interval-lattice` (lines 316-341)
- Interval arithmetic: `interval-add`, `interval-sub`, `interval-mul` (lines 344-359)

**`.sld`:**

```scheme
(define-library (wile algebra interval)
  (description "Interval arithmetic with infinity-aware operations and interval lattice.")
  (export interval-lattice
          interval-add interval-sub interval-mul
          inf<= inf< inf-min inf-max inf+ inf- inf*)
  (import (scheme base)
          (wile algebra lattice))
  (include "interval.scm"))
```

**Key changes from goast version:**
- Add structured docstrings with `Category: algebra`
- Self-contained — no goast dependencies

**Test file should cover:**
- Infinity comparisons (`pos-inf`, `neg-inf` with finite values)
- Infinity arithmetic (edge cases: `0 * pos-inf = 0`, `pos-inf + neg-inf = pos-inf`)
- Lattice properties: join widens, meet narrows, empty meet = `interval-bot`, bottom is join identity
- Interval arithmetic: add, sub, mul on concrete intervals
- Lattice validation via `validate-lattice` from `(wile algebra lattice)`

**Step 1: Write test, Step 2: Fail, Step 3: Implement, Step 4: Pass**

Run: `cd /Users/aalpar/projects/wile-workspace/wile && ./dist/wile test/wile/algebra-interval-test.scm`

**Step 5: Update `algebra.sld`, Step 6: Commit in wile**

```
feat(algebra): add interval arithmetic module
```

---

### Task 7: Rewire wile-goast interval imports

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/domains.sld` (wile-goast)
- Modify: `cmd/wile-goast/lib/wile/goast/domains.scm` (wile-goast)

**Step 1: Update `domains.sld`**

Add `(wile algebra interval)` to imports. Remove interval exports if any.

**Step 2: Update `domains.scm`**

Delete: `inf<=`, `inf<`, `inf-min`, `inf-max`, `inf+`, `inf-`, `inf*`, `interval-lattice`, `interval-add`, `interval-sub`, `interval-mul`.
Keep: `make-interval-analysis` (uses interval-lattice but adds Go SSA-specific transfer function and widening).

**Step 3: Run tests + CI**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "Interval|interval" -v -count=1`
Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`

**Step 4: Commit in wile-goast**

```
refactor(domains): import interval from (wile algebra interval)
```

---

## Phase 4: Semiring Graph Algorithms

### Task 8: Create `(wile algebra graph)` in wile

**Files:**
- Create: `stdlib/lib/wile/algebra/graph.sld` (in wile)
- Create: `stdlib/lib/wile/algebra/graph.scm` (in wile)
- Create: `test/wile/algebra-graph-test.scm` (in wile)

**What moves from wile-goast `path-algebra.scm`:**
The core Bellman-Ford algorithm, abstracted from call-graph specifics. The goast version builds adjacency from CG nodes; the wile version takes an adjacency alist directly.

**`.sld`:**

```scheme
(define-library (wile algebra graph)
  (description "Semiring-parameterized graph algorithms: shortest path, reachability, path counting.")
  (export make-graph-analysis graph-analysis?
          graph-query graph-query-all)
  (import (scheme base)
          (wile algebra semiring))
  (include "graph.scm"))
```

**Key abstraction:**
- `make-graph-analysis` takes `(semiring adjacency edge-weight)` where `adjacency` is `((node . ((neighbor . edge-data) ...)) ...)` — a generic adjacency alist
- `edge-weight` is `(lambda (edge-data) -> semiring-value)` or `#f` for unit weights
- `graph-query` and `graph-query-all` — same as goast's `path-query`/`path-query-all`
- Internal: `compute-single-source` (Bellman-Ford worklist), cache layer

**Key changes from goast version:**
- Replace `build-adjacency` (CG-specific) with generic adjacency input
- Replace `nf` with alist accessors or record type
- Remove CG-specific naming (`pa-` prefix → `ga-` prefix or similar)
- Use `define-record-type <graph-analysis>` following wile conventions
- Add structured docstrings

**Test file should cover:**
- Boolean semiring: reachability on a small graph
- Tropical semiring: shortest path (hop count)
- Caching: second query returns same result
- Unreachable node returns semiring-zero
- Custom edge weights

**Step 1: Write test, Step 2: Fail, Step 3: Implement, Step 4: Pass**

Run: `cd /Users/aalpar/projects/wile-workspace/wile && ./dist/wile test/wile/algebra-graph-test.scm`

**Step 5: Update `algebra.sld`, Step 6: Commit in wile**

```
feat(algebra): add semiring graph algorithms module
```

---

### Task 9: Rewire wile-goast path-algebra imports

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/path-algebra.sld` (wile-goast)
- Modify: `cmd/wile-goast/lib/wile/goast/path-algebra.scm` (wile-goast)

**Step 1: Update `path-algebra.sld`**

Add `(wile algebra graph)` to imports. The goast module becomes a thin wrapper: `build-adjacency` converts CG to generic adjacency, then delegates to `(wile algebra graph)`.

**Step 2: Update `path-algebra.scm`**

Keep: `build-adjacency` (CG → adjacency alist conversion), `make-path-analysis` (wraps `make-graph-analysis` with CG-specific adjacency building).
Delete: `compute-single-source`, `get-or-compute`, the cache layer, the local `filter` — all now in `(wile algebra graph)`.
Rewrite: `path-query` and `path-query-all` to delegate to `graph-query`/`graph-query-all`.

**Step 3: Run tests + CI**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "Path|path" -v -count=1`
Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`

**Step 4: Commit in wile-goast**

```
refactor(path-algebra): import graph algorithms from (wile algebra graph)
```

---

## Phase 5: Finalize

### Task 10: Final CI on both repos

**Step 1: Run wile full test suite**

Run: `cd /Users/aalpar/projects/wile-workspace/wile && make test-scheme`
Expected: All pass including 4 new algebra test files.

**Step 2: Run wile-goast full CI**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: All pass, coverage >= 80%.

**Step 3: Update CLAUDE.md in wile-goast**

Remove the detailed descriptions of FCA internals, interval arithmetic, Pareto, and semiring graph from wile-goast's CLAUDE.md. Replace with notes that these now come from `(wile algebra)`. Keep documentation for the Go-specific bridges that remain.

**Step 4: Commit documentation update in wile-goast**

```
docs: note algebra extractions to wile
```
