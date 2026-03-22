# Consistency-Based Deviation Detection

**Status**: Partially implemented (co-mutation validated; belief DSL implemented; categories 1-4 designed but unvalidated)
**Foundation**: [wile-goast](https://github.com/aalpar/wile-goast) — all five goast layers (see `plans/GO-STATIC-ANALYSIS.md`)
**Dependencies**: None beyond existing goast infrastructure
**Implementation**: Pure Scheme scripts using `(wile goast)`, `(wile goast ssa)`, `(wile goast callgraph)`, `(wile goast cfg)` primitives
**Prior art**: Engler et al., "Bugs as Deviant Behavior" (SOSP 2001)

## Problem

Codebases accumulate implicit conventions — patterns that most call sites follow but no specification enforces. When a site deviates from its codebase's own convention, the deviation is either a bug or an intentional exception. Mechanically surfacing deviations lets a human decide which.

The key insight (Engler et al.): **the code is its own specification.** A pattern followed in 98 of 100 sites is a strong convention. The 2 deviations deserve investigation — they may be bugs or intentional exceptions. The statistical signal comes from the codebase itself, requiring no annotations, no configuration, and no external spec.

## Fundamental Assumption

One assumption this approach cannot verify: **the majority pattern should generally represent the intended convention.** If the majority behavior contains the bug, this approach confirms the bug rather than catching it. Engler's method detects *inconsistency*, not *incorrectness*. A codebase where every caller mishandles errors consistently produces zero deviations.

The validation (§ Validation Results) shows this is a soft guideline, not a hard requirement: deviations are frequently *intentional* (focused setter functions, semantically distinct operations). The tool surfaces deviations; the human classifies them.

The dual of the unification detector's objective precondition. Unification requires agreement on the shared domain (a semantic property). Consistency detection requires that the majority behavior is the intended behavior (a social property). Both require human judgment at the boundary.

## Belief Model

A **belief** is a pattern extracted from code — not from a spec, but from statistical observation of what the code does.

```
belief = (pattern, sites, adherence_count, deviation_count)
```

- **pattern**: A structural or behavioral property (e.g., "error return is checked")
- **sites**: The set of code locations where the pattern could apply (e.g., all call sites of function F)
- **adherence_count**: Sites that follow the pattern
- **deviation_count**: Sites that do not

A belief is **strong** when `adherence_count / (adherence_count + deviation_count)` is high and the total site count large enough for statistical significance.

### Ranking

Rank beliefs by a z-statistic:

```
z = (adherence - n * p₀) / √(n * p₀ * (1 - p₀))
```

where `n` is total sites, `adherence` is sites following the pattern, and `p₀` is a baseline expectation (typically 0.5 — no prior assumption). A high z-score means the pattern is strongly established and deviations likely meaningful. A low z-score means the "pattern" may be coincidence.

Thresholds are tunable per belief category. Conservative default: report deviations only when adherence ≥ 90% and total sites ≥ 5.

**Note on ranking in practice:** The validation (§ Validation Results) used simple ratio thresholds (66%/3), not z-scores. The z-statistic and ratio threshold differ: the z-statistic accounts for sample size more rigorously. Future work should evaluate whether z-scores improve signal quality over raw ratios at varying corpus sizes.

**Independence assumption:** The z-statistic assumes each site independently follows or deviates. Sites within the same package, by the same author, or copied from the same template may correlate, inflating the z-score beyond what the evidence warrants.

## Layer Strategy

Each analysis layer answers a different kind of consistency question. Some questions need a single layer. The interesting ones require crossing layers — where wile-goast provides capability that no commonly available Go analysis tool matches. (Tools like CodeQL and Semgrep offer cross-layer queries for other ecosystems; the claim concerns Go-specific scriptability, not the general space of static analysis tools.)

### Single-Layer Roles

| Layer | Consistency question | Example |
|-------|---------------------|---------|
| AST | Same syntactic shape? | "Error checks use `if err != nil`, not `if err == nil`" |
| SSA | Same data flow? | "Parameter P is bounds-checked before use" |
| CFG | Same execution order? | "Lock acquisition dominates critical section" |
| Call Graph | Same inter-procedural context? | "Every caller of F also calls G" |
| Lint | Same analyzer profile? | "Which functions trigger `nilness` when peers don't?" |

### Cross-Layer Composition

Existing tools (`errcheck`, `nilness`, `go vet`) partially cover the single-layer patterns above. They do not cover the cross-layer patterns.

The composition pattern is uniform: **one layer enumerates sites, another characterizes behavior, a third verifies ordering or context.** Statistical comparison then operates on the characterized behaviors.

```
enumerate(layer₁) → characterize(layer₂) → verify(layer₃) → compare statistically
```

This matches the multi-pass structure of `state-trace-full.scm` and `unify-detect-pkg.scm` — enumerate candidates in one pass, refine with deeper analysis in subsequent passes. The infrastructure (`walk`, `nf`, `tag?`, `filter-map`, `flat-map`) transfers directly.

## Belief Categories

Five categories, ordered by implementation complexity. Each has a site enumeration strategy, a characterization method, and a deviation definition.

**Validation status:** Only category 5 (co-mutation) has empirical validation (§ Validation Results). Categories 1-4 are designed but untested. The belief DSL (§ `BELIEF-DSL.md`) implements verbs for all five categories, but categories 1-4 have not been exercised against real codebases.

**Completeness:** These five categories are not exhaustive. Belief types that fall outside them include: initialization-order beliefs (field X set before use), concurrency beliefs (goroutine spawning patterns), import-dependency beliefs (certain packages always imported together), and naming-convention beliefs. The `custom` escape hatch in the DSL covers these, but expressing them requires more effort.

**Granularity:** All five categories enumerate *functions* as sites. Some conventions operate at other scopes: statement, block, expression, file, or package level. A design choice, not an inherent limitation — but beliefs like "all test files import testify" or "all struct literals initialize field X" fall outside the current site model.

### 1. Pairing Beliefs

**Pattern:** "Operation A is always paired with operation B."

**Examples:**
- `mu.Lock()` paired with `mu.Unlock()` (or `defer mu.Unlock()`)
- `os.Open()` paired with `f.Close()` (or `defer f.Close()`)
- `ctx, cancel := context.WithCancel(...)` paired with `cancel()` (or `defer cancel()`)

**Site enumeration (AST):** All `func-decl` bodies containing a `call-expr` matching operation A.

**Characterization (AST + CFG):**
- Does the same function body contain a matching operation B?
- If B is deferred, is B's defer guaranteed to execute? (CFG: does B's block post-dominate A's block?)

**Deviation:** A site contains operation A but no matching operation B.

**Existing coverage:** `go vet` checks specific pairing patterns. The Engler approach generalizes: it discovers pairing beliefs from the code rather than checking a hardcoded list.

**Implementation complexity:** Low. AST-only for the basic version; CFG for the post-dominance verification.

### 2. Check Beliefs

**Pattern:** "Value V is checked for condition C before use."

**Examples:**
- Error return checked for `!= nil` before use
- Pointer checked for `!= nil` before dereference
- Slice length checked before index access
- Map lookup `ok` value checked before using result

**Site enumeration (SSA):** All instructions that produce a value of a checkable type (error, pointer, map-lookup tuple).

**Characterization (SSA + CFG):**
- Is there an `ssa-if` instruction whose operand is the produced value (or a comparison involving it)?
- Does the `ssa-if` block dominate the use site? (`go-cfg-dominates?`)

**Deviation:** A value is used without a dominating check, when peer values (same function, same type) are checked.

**Existing coverage:** `errcheck` covers error returns. `nilness` covers some nil dereferences. Neither covers bounds checks, map-lookup checks, or codebase-specific validation patterns. Neither compares *across call sites of the same function* — both check individual sites in isolation.

**Implementation complexity:** Medium. Requires SSA for value tracing, CFG for dominance.

### 3. Handling Beliefs

**Pattern:** "All callers of function F handle the result the same way."

Strictly more powerful than check beliefs. Check beliefs ask "is the error checked?" Handling beliefs ask "is the error *handled consistently*?"

**Examples:**
- 8 callers wrap the error with `fmt.Errorf("...: %w", err)` and return. 1 caller silently discards.
- 12 callers retry on `ErrTemporary`. 1 caller returns immediately.
- 6 callers log the error and continue. 1 caller panics.

**Site enumeration (Call Graph):** Use `go-callgraph-callers` to find all callers of F. Filter to functions with ≥ 5 call sites (statistical significance).

**Characterization (AST):** At each call site, classify the surrounding error-handling pattern:

```scheme
;; Classify the error-handling pattern at a call site.
;; Returns a symbol: 'wrap-return, 'raw-return, 'log-continue, 'ignore, 'other
(define (classify-error-handling call-site-context)
  (let* ((if-stmt (find-enclosing-if call-site-context))
         (body (and if-stmt (nf if-stmt 'body))))
    (cond
      ((not if-stmt) 'ignore)
      ((contains-wrap-return? body) 'wrap-return)
      ((contains-raw-return? body) 'raw-return)
      ((contains-log? body) 'log-continue)
      (else 'other))))
```

**Deviation:** A caller whose handling category differs from the majority category.

**Classification caveat:** The classifier forces each call site into a single category. In practice, a site may both wrap and log, or wrap with a different format string than its peers. Multi-label classification would increase precision but complicate statistical comparison. The current approach trades precision for simplicity — acceptable for deviation *detection* (flag for human review) but not for automated *correction*.

**Existing coverage:** No existing Go tool compares error handling *across callers of the same function*. This cross-layer pattern requires call graph for enumeration and AST for characterization.

**Implementation complexity:** Medium-high. Call graph for site enumeration, AST for pattern classification, heuristic rules for classifying handler patterns.

### 4. Ordering Beliefs

**Pattern:** "Operation A always precedes operation B."

**Examples:**
- Validation check before database write
- Authentication check before authorization check
- Resource acquisition before resource use
- Field initialization before method dispatch

**Site enumeration (AST or Call Graph):** All functions containing both operations A and B.

**Characterization (CFG):** Use `go-cfg-dominates?` to check whether A's block dominates B's block.

**Deviation:** A function where B is not dominated by A, when peer functions always have A dominating B.

**Connection to state-trace:** Pass 4 of `state-trace-full.scm` already performs dominance-based ordering analysis for boolean field accesses. The ordering belief generalizes this to arbitrary operation pairs.

**Implementation complexity:** Medium. CFG for dominance, but the difficult part is identifying which operation pairs to check.

### 5. Co-Mutation Beliefs

**Pattern:** "Fields X and Y of struct S are always modified together."

**Examples:**
- `offset` and `length` updated together in a buffer struct
- `valid` flag and `value` field updated together
- `lastModified` timestamp and data field updated together

**Site enumeration (SSA):** All functions that store to any field of struct S via `ssa-field-addr` + `ssa-store`.

**Characterization (SSA):** For each function, compute the set of fields stored. Two fields are co-mutated if they always appear in the same store-set.

**Deviation:** A function that stores to field X but not field Y, when all other functions store to both.

**Connection to state-trace:** Pass 3 of `state-trace-full.scm` detects independent mutation of boolean field clusters. The co-mutation belief generalizes from boolean fields to all fields, and from "evidence of split state" to "deviation from co-mutation convention."

**Existing coverage:** No existing Go tool tracks field co-mutation patterns across functions.

**Implementation complexity:** Low-medium. SSA only. The `stored-fields-in-func` helper from `state-trace-full.scm` transfers directly.

## Cross-Layer Patterns

These patterns require information from multiple layers that no single-layer tool provides. They are the primary motivation for this plan.

### Pattern A: Guard-at-the-Right-Level

**Layers:** Call Graph + AST + SSA

**Belief:** "If a precondition check exists for parameter P, it is in the first function that receives P from external input — not redundantly repeated in callees."

1. **Call graph:** Build the call chain from caller → callee → deep callee via `go-callgraph-callees`.
2. **SSA:** For each function in the chain, check whether the parameter is validated (appears as operand to `ssa-if` with a bounds/nil/error comparison).
3. **AST:** Identify the guard's structural pattern (what condition, what action on failure).

**Deviation types:**
- **Missing guard:** No function in the chain validates the parameter. All peer chains validate.
- **Redundant guard:** Both caller and callee validate the same parameter the same way. The callee's check is dead weight.
- **Misplaced guard:** The callee validates but the caller does not. The check is too deep — a different caller could bypass it.

**Relevance to parameter essentiality:** A parameter used only in a guard (single-read, guard-only), where the caller already performs the same guard, is redundant. Removing the callee's guard simplifies its signature and clarifies responsibility.

### Pattern B: Consistent Field Protocol

**Layers:** AST + SSA + CFG

**Belief:** "For struct S, all methods that access field X first establish condition C."

1. **AST:** Find all methods with receiver type S.
2. **SSA:** Find which methods access field X (`ssa-field-addr` with matching field name).
3. **CFG:** For each such method, check whether condition C's block dominates the access block.

**Deviation:** A method that accesses field X without condition C dominating, when peer methods always have C dominating.

A generalization of typestate analysis — inferred from the code, not annotated. If 9 of 10 methods check `s.initialized` before using `s.data`, the 10th is suspect.

### Pattern C: Error Handling Consistency Across Callers

**Layers:** Call Graph + AST

**Belief:** "All callers of function F handle the error return in the same structural pattern."

1. **Call graph:** `go-callgraph-callers` to enumerate call sites.
2. **AST:** Classify each site's error handling pattern (wrap-return, raw-return, log-continue, ignore, panic).

**Deviation:** A caller whose handling pattern differs from the majority.

Handling belief category (§3 above) implemented as a cross-layer script.

### Pattern D: Callee-Set Similarity

**Layers:** Call Graph + (AST for structural comparison)

**Belief:** "Functions that call the same downstream functions serve the same purpose."

1. **Call graph:** For each function, compute its callee set via `go-callgraph-callees`.
2. **Comparison:** Jaccard similarity between callee sets.
3. **AST:** For high-similarity pairs, run the unification detector's structural diff.

A pre-filter for unification detection — functions with similar callee sets are more likely to be unification candidates. Call graph structure narrows the O(n²) comparison space.

## Rule Architecture

### Phase 0: Data Collection

```scheme
;; Load all layers for the target module
(define target "./...")
(define pkgs (go-typecheck-package target))
(define ssa-funcs (go-ssa-build target))
(define cg (go-callgraph target 'vta))
```

All five layers operate on the same target. The uniform s-expression format lets the same traversal utilities work everywhere.

### Phase 1: Site Enumeration

Each belief category has its own enumeration strategy. The general pattern:

```scheme
;; Enumerate sites for a pairing belief.
;; Find all functions that call operation A.
;; Returns: ((func-name call-site-ast) ...)
(define (enumerate-sites-for-op pkgs op-selector op-method)
  (flat-map
    (lambda (pkg)
      (flat-map
        (lambda (file)
          (flat-map
            (lambda (decl)
              (and (tag? decl 'func-decl)
                   (let ((calls (walk (nf decl 'body)
                                  (lambda (node)
                                    (and (tag? node 'call-expr)
                                         (let ((fn (nf node 'fun)))
                                           (and (tag? fn 'selector-expr)
                                                (equal? (nf fn 'sel) op-method)
                                                node)))))))
                     (map (lambda (c) (list (nf decl 'name) c)) calls))))
            (cdr (assoc 'decls (cdr file)))))
        (nf pkg 'files)))
    pkgs))
```

For call-graph-based enumeration:

```scheme
;; Enumerate callers of a specific function.
;; Returns: ((caller-name edge) ...)
(define (enumerate-callers cg func-name)
  (let ((edges (go-callgraph-callers cg func-name)))
    (if edges
      (map (lambda (e) (list (nf e 'caller) e)) edges)
      '())))
```

### Phase 2: Behavior Characterization

Each site is characterized by a symbol or small data structure representing its behavior. Characterization depends on the belief category.

For check beliefs (SSA-based):

```scheme
;; Characterize how a function uses a parameter.
;; Returns: 'guarded, 'unguarded, or 'unused
(define (characterize-param-use ssa-func param-name)
  (let* ((blocks (nf ssa-func 'blocks))
         (all-instrs (flat-map
                       (lambda (b) (nf b 'instrs))
                       (if (pair? blocks) blocks '())))
         ;; Find instructions that reference this parameter
         (uses (filter
                 (lambda (instr)
                   (let ((ops (nf instr 'operands)))
                     (and (pair? ops) (member? param-name ops))))
                 all-instrs))
         ;; Check if any use is an ssa-if (guard)
         (has-guard (any (lambda (u) (tag? u 'ssa-if)) uses)))
    (cond
      ((null? uses) 'unused)
      (has-guard 'guarded)
      (else 'unguarded))))
```

For pairing beliefs (AST-based):

```scheme
;; Characterize whether a function body contains a matching
;; cleanup for an acquired resource.
;; Returns: 'paired-defer, 'paired-call, 'unpaired
(define (characterize-pairing func-body acquire-method release-method)
  (let ((has-defer-release
          (walk func-body
            (lambda (node)
              (and (tag? node 'defer-stmt)
                   (let ((call (nf node 'call)))
                     (and (tag? call 'call-expr)
                          (let ((fn (nf call 'fun)))
                            (and (tag? fn 'selector-expr)
                                 (equal? (nf fn 'sel) release-method)))))))))
        (has-call-release
          (walk func-body
            (lambda (node)
              (and (tag? node 'call-expr)
                   (let ((fn (nf node 'fun)))
                     (and (tag? fn 'selector-expr)
                          (equal? (nf fn 'sel) release-method))))))))
    (cond
      ((pair? has-defer-release) 'paired-defer)
      ((pair? has-call-release) 'paired-call)
      (else 'unpaired))))
```

### Phase 3: Statistical Comparison

```scheme
;; Group sites by their characterization, compute majority, report deviations.
(define (find-deviations sites characterizations)
  (let* ((pairs (map cons sites characterizations))
         ;; Count each category
         (counts (fold-categories pairs))
         (total (length pairs))
         ;; Find majority category
         (majority (max-by-count counts))
         (majority-cat (car majority))
         (majority-count (cdr majority))
         ;; Compute adherence ratio
         (ratio (/ majority-count total))
         ;; Deviations = sites not in majority
         (deviations (filter (lambda (p) (not (eq? (cdr p) majority-cat)))
                             pairs)))
    (list ratio total majority-cat deviations)))

;; Report only when belief is strong enough
(define min-adherence 0.90)
(define min-sites 5)

(define (report-if-significant result)
  (let ((ratio (car result))
        (total (cadr result))
        (majority (caddr result))
        (deviations (cadddr result)))
    (and (>= ratio min-adherence)
         (>= total min-sites)
         (pair? deviations)
         (begin
           (display "  Belief: ") (display majority)
           (display " (") (display (- total (length deviations)))
           (display "/") (display total) (display " sites)")
           (newline)
           (for-each
             (lambda (d)
               (display "    DEVIATION: ") (display (car d))
               (display " -> ") (display (cdr d))
               (newline))
             deviations)))))
```

### Phase 4: Reporting

Output matches the existing examples — structured text with pass labels and summaries:

```
══════════════════════════════════════════════════
  Consistency Check: Error Handling Across Callers
══════════════════════════════════════════════════

── Function: (*DB).Query ──
  Belief: wrap-return (11/12 callers)
    DEVIATION: handleLegacyRequest -> ignore

── Function: (*Client).Send ──
  Belief: raw-return (8/9 callers)
    DEVIATION: processBackground -> log-continue

── Summary ──
  Functions analyzed:   42
  Strong beliefs:       7
  Deviations found:     3
```

## Belief Bootstrapping

A strong belief's output has the same shape as a site enumeration input: `(pattern, sites, counts)`. Phase 3 output (adherence/deviation site lists) feeds Phase 1 input without changing the data model. The pipeline composes across discovery stages, though it is not algebraically closed — the final reporting step produces human-readable output, not belief forms.

### Forms

Three forms, ordered by signal strength:

**1. Vertical Refinement (Belief → Sites).** A discovered belief defines a new cohort for a different category:

```
co-mutation discovers {stepMode, stepFrame, stepFrameDepth}
    ↓
field group becomes an enumeration set
    ↓
ordering belief: "is stepMode always stored before stepFrame?"
    ↓
check belief: "is some condition checked before mutating this group?"
```

Co-mutation output (category 5) produces field groups. Ordering (category 4) consumes operation pairs. The adapter converts field names to store operations; the adherence sites become the enumeration.

**2. Horizontal Composition (Belief + Belief → Compound Belief).** Two beliefs sharing the same sites combine:

```
pairing belief: "Lock always paired with Unlock"
  +
ordering belief: "Lock always dominates field access X"
  =
compound belief: "Lock-Unlock pairs always protect access to field X"
```

The compound belief generates its own deviations: functions that access X without a Lock-Unlock guard.

**3. Deviation Clustering (Deviations → Explanatory Belief).** Deviation sites from one belief become candidates for new analysis. If 3 deviations from a "wrap-return" handling belief all occur in background goroutines, that is a sub-convention, not a bug. The weakest form: deviation sets are small by definition, so statistical power is low.

### Funnel Property

Each bootstrapping step strictly shrinks the site set. Co-mutation starts with all functions that store to any field of struct S. Strong beliefs narrow to functions that store a specific pair. Ordering narrows further to functions where the pair occupies different blocks. The chain terminates when the site set drops below `min-sites`.

This guarantees convergence. Bootstrapping should work best on large codebases with many instances of the same pattern, where each funnel step retains enough sites for statistical power — though empirical validation has not gone beyond the co-mutation → ordering chain prediction (§ Validation Prediction).

### Trade-offs

| Gain | Cost |
|------|------|
| Discovers beliefs no single category finds | False positive amplification — each step inherits prior errors |
| No pre-specification of cross-category patterns | Interpretability degrades with chain depth |
| Self-limiting via funnel property | Computational cost is multiplicative across steps |

### Concrete Adapter: Co-Mutation → Ordering

The co-mutation prototype's `pair-stats` returns:

```
(field-a field-b co-count a-only-count b-only-count co-funcs a-only-funcs b-only-funcs)
```

The `co-funcs` list — functions that store both fields — is exactly the site enumeration for an ordering belief. No new enumeration pass required.

#### Step 1: Extract ordering candidates from strong co-mutation beliefs

```scheme
(define (comutation->ordering-candidates all-pair-stats)
  (filter-map
    (lambda (stats)
      (let* ((field-a    (list-ref stats 0))
             (field-b    (list-ref stats 1))
             (co-count   (list-ref stats 2))
             (a-only     (list-ref stats 3))
             (b-only     (list-ref stats 4))
             (co-funcs   (list-ref stats 5))
             (total      (+ co-count a-only b-only)))
        ;; Double threshold: co-mutation must be strong AND
        ;; co-func set must be large enough for ordering signal.
        (and (>= total min-sites)
             (>= (/ co-count total) min-adherence)
             (>= (length co-funcs) min-sites)
             (list field-a field-b co-funcs))))
    all-pair-stats))
```

The double threshold is the funnel property in action: the co-mutation belief must be strong, and the surviving co-func set must be large enough for the ordering belief to retain statistical power.

#### Step 2: Locate field stores by SSA block

The co-mutation script uses a flat `walk` that destroys block structure. Ordering needs to know which block each store occupies. Since `ssa-field-addr` and `ssa-store` can reside in different blocks, a two-pass join is required — first collecting register-to-field mappings, then finding stores through those registers:

```scheme
;; Returns: ((field-name . block-index) ...)
(define (field-stores-by-block ssa-func target-fields)
  (let* ((blocks (nf ssa-func 'blocks)))
    (if (not (pair? blocks)) '()
      (let* (;; Pass A: register → field mapping (across all blocks)
             (reg->field
               (flat-map
                 (lambda (block)
                   (filter-map
                     (lambda (instr)
                       (and (tag? instr 'ssa-field-addr)
                            (member? (nf instr 'field) target-fields)
                            (cons (nf instr 'name) (nf instr 'field))))
                     (let ((instrs (nf block 'instrs)))
                       (if (pair? instrs) instrs '()))))
                 blocks))
             ;; Pass B: store → (field . block-index)
             (stores
               (flat-map
                 (lambda (block)
                   (let ((block-idx (nf block 'index)))
                     (filter-map
                       (lambda (instr)
                         (and (tag? instr 'ssa-store)
                              (let ((entry (assoc (nf instr 'addr) reg->field)))
                                (and entry (cons (cdr entry) block-idx)))))
                       (let ((instrs (nf block 'instrs)))
                         (if (pair? instrs) instrs '())))))
                 blocks)))
        stores))))
```

#### Step 3: Check dominance via idom chain

The `ssa-block` node exposes an `idom` field — the immediate dominator's block index. The entry block (index 0) omits `idom`; it is the dominator tree root. Check dominance by walking the `idom` chain from the target block toward the root:

```scheme
;; Does block-a dominate block-b?
;; Walk idom chain from b toward root; if we hit a, yes.
(define (dominates? blocks block-a block-b)
  (let loop ((idx block-b))
    (cond
      ((= idx block-a) #t)
      ((= idx 0) #f)
      (else
        (let ((blk (list-ref blocks idx)))
          (let ((idom-pair (assoc 'idom (cdr blk))))
            (if idom-pair (loop (cdr idom-pair)) #f)))))))
```

#### Step 4: Characterize ordering

```scheme
(define (characterize-ordering ssa-func field-a field-b)
  (let* ((stores (field-stores-by-block ssa-func (list field-a field-b)))
         (a-blocks (filter-map
                     (lambda (s) (and (equal? (car s) field-a) (cdr s)))
                     stores))
         (b-blocks (filter-map
                     (lambda (s) (and (equal? (car s) field-b) (cdr s)))
                     stores))
         (blocks (cdr (assoc 'blocks (cdr ssa-func)))))
    (cond
      ((or (null? a-blocks) (null? b-blocks)) 'missing)
      ((or (> (length (unique a-blocks)) 1)
           (> (length (unique b-blocks)) 1)) 'multi-store)
      ((= (car a-blocks) (car b-blocks)) 'same-block)
      ((dominates? blocks (car a-blocks) (car b-blocks)) 'a-dominates-b)
      ((dominates? blocks (car b-blocks) (car a-blocks)) 'b-dominates-a)
      (else 'unordered))))
```

Result categories: `a-dominates-b` and `b-dominates-a` are clear orderings. `same-block` means instruction-level ordering (not yet analyzed). `multi-store` means the field is stored in multiple blocks — complex control flow requiring path-sensitive analysis. `unordered` means neither block dominates the other (e.g., stores in sibling branches).

### Validation Prediction

Using the co-mutation validation data (§ Validation Results):

- **Debugger stepping fields:** Only 2 co-funcs per strong pair ({Continue, StepInto}). Below `min-sites` — the ordering belief will not fire. The funnel property correctly prevents a low-confidence finding.
- **VMCounters:** 10 `inline*` helpers co-mutate `{StackDrains, ForeignCalls, StackElementsDrained}`. Enough sites for ordering to ask: "Do all 10 helpers store `StackDrains` before `ForeignCalls`?" Consistent ordering is evidence of a protocol; a deviation is a potential ordering bug invisible to co-mutation alone.

## Existing Prior Art in This Codebase

### state-trace-full.scm

Passes 3 and 4 implement a specific consistency check: "are boolean fields co-mutated?" (Pass 3) and "do field accesses follow a consistent dominance order?" (Pass 4). These are co-mutation and ordering beliefs respectively, restricted to boolean field clusters.

The traversal infrastructure (`walk`, `nf`, `tag?`, `filter-map`, `flat-map`, `unique`, `member?`, `ordered-pairs`) transfers directly. The `stored-fields-in-func` helper from Pass 3 serves as the characterization function for co-mutation beliefs.

### goast-query.scm

The `returns-error?` and `package-error-funcs` functions demonstrate AST-based classification of function signatures — the same classification check and handling beliefs require.

### unify-detect-pkg.scm

The recursive AST diff engine and substitution collapsing are not directly reusable here, but the module-wide enumeration pattern (load all packages, group candidates, compare pairwise) is the same pattern cross-function consistency analysis needs.

## Validation Results

### Co-Mutation Beliefs: wile/machine

The co-mutation prototype (`examples/goast-query/consistency-comutation.scm`) ran against `github.com/aalpar/wile/machine` (53 structs, ~60 SSA functions with field stores).

**Thresholds:** adherence ≥ 66%, minimum 3 total sites. Chosen empirically — exploration mode (no thresholds) produced 2,164 deviations; the 66%/3 thresholds reduced this to 21 deviations across 11 beliefs.

#### Debugger stepping fields (genuine signal)

| Function | Stores |
|----------|--------|
| `Continue` | `{stepMode, stepFrame, stepFrameDepth}` |
| `StepInto` | `{stepMode, stepFrame, stepFrameDepth}` |
| `StepOver` | `{stepMode, stepFrameDepth}` — missing `stepFrame` |
| `StepOut` | `{stepMode, stepFrame}` — missing `stepFrameDepth` |

Three co-mutation beliefs: `(stepMode, stepFrameDepth)` at 75% adherence, `(stepMode, stepFrame)` at 75%, and `(stepFrameDepth, stepFrame)` at 50% (below threshold). `StepOver` and `StepOut` each omit one of the three stepping fields. Whether these are bugs depends on semantics — `StepOver` may intentionally skip `stepFrame` because it stays at the current frame — but the deviation merits investigation.

#### VMCounters co-mutation (structural insight)

`(StackDrains, ForeignCalls)` and `(StackElementsDrained, ForeignCalls)` at 66% adherence (10/15). Ten `inline*` helper functions store all three counters together. Five store subsets: `callForeignCachedReassigned`, `callPromotedFallback`, `drainAndApply` drain without foreign calls; `applyForeign` and one `Apply` overload count foreign calls without drains. Semantically correct — draining and foreign-calling are correlated but distinct.

#### Field name collision (false positive, fixed)

`SchemeError: (Message, Source)` was initially reported as a deviation: `goErrorToSchemeException` stores `Source` and `StackTrace` but not `Message`. Investigation revealed a **false positive from field name collision**. `goErrorToSchemeException` constructs an `ErrExceptionEscape`, not a `SchemeError`. Both structs share `Source` and `StackTrace` field names, and SSA `field-addr` instructions carry only the field name, not the struct type.

**Fix:** Receiver-type disambiguation. Group `ssa-field-addr` instructions by their receiver operand (`x`). If any field accessed through a receiver falls outside the target struct's field set, that receiver belongs to a different struct — exclude all its field-addrs. This eliminates cross-struct false positives without requiring the Go mapper to expose the struct type. The SSA mapper already computes `structType` (`goastssa/mapper.go:279`) but omits it from the s-expression; exposing it as an explicit field would be a cleaner long-term fix.

#### vmState: (callDepth, marks) (weak signal)

6 co-mutated, 1 callDepth-only, 2 marks-only. The deviations are focused operations (`NewMachineContinuation`, `DeleteMark`, `SetMark`) — setter functions that intentionally touch single concerns. Noise from the setter pattern: a function storing 1 field from a 12-field struct does not violate a co-mutation belief.

### Proposed further validation targets

**Status:** Not yet attempted. These are hypotheses about where the approach would produce useful signal.

#### Target: crdt (github.com/aalpar/crdt)

Previously used to validate the unification detector. The 17-package CRDT library offers:

1. **Method protocol consistency.** The `Merge`, `Value`, `MarshalJSON` methods should follow a consistent pattern across CRDT types. Deviations flag incomplete implementations.
2. **Field access protocols.** Each CRDT struct has a `store` or `dots` field. The access pattern (acquire context, read store, build delta) should be consistent across methods.

### False positive observations

Two classes of false positives emerged:

1. **Field name collision.** Two structs with overlapping field names cause cross-attribution. Fixed by receiver-type disambiguation (see above).
2. **Focused setter functions.** Functions like `SetPC`, `SetThread`, `SetMark` store a single field from a multi-field struct. These are intentional focused APIs, not co-mutation violations. Possible mitigations: exclude functions storing only 1 field, or count deviations only from functions storing 2+ fields. Not yet implemented — the current threshold-based approach suffices for the Debugger-class findings.

## Limitations

### What this approach cannot detect

1. **Majority-is-wrong bugs.** If every caller mishandles an error the same way, adherence is 100% and no deviation appears. The bug is invisible to consistency analysis.
2. **Singleton patterns.** A function called from only 1 site has no peers for comparison. Consistency analysis requires multiple sites.
3. **Intentional variations.** Some deviations are correct — a background goroutine that logs-and-continues an error rather than returning it. The tool reports these; the human must distinguish intentional from accidental.
4. **Cross-module conventions.** Beliefs are extracted per-module. A convention shared across modules but followed inconsistently within one module goes undetected.
5. **Dynamic behavior.** Patterns depending on runtime values (e.g., "this error is retried only when the server returns 503") are invisible to static analysis.
6. **Semantic equivalence behind structural difference.** The approach classifies sites by structural pattern (AST shape, call names). Two sites achieving the same semantic goal through different structures (e.g., wrapping an error with `fmt.Errorf` vs. a project-specific `errors.Wrap`) appear as deviations. Structural inconsistency is a proxy for semantic inconsistency — but an imperfect one.
7. **Field name collisions (partially mitigated).** SSA `ssa-field-addr` instructions carry the field name but not the struct type. When two structs share a field name (e.g., `Source` on both `SchemeError` and `ErrExceptionEscape`), stores to one struct can be mis-attributed to the other. The co-mutation script mitigates this via receiver-type disambiguation — grouping field-addrs by receiver and discarding receivers that access foreign fields. Effective but incomplete: the heuristic fails when two structs have identical field sets (rare in practice). Exposing the struct type in `ssa-field-addr` (the Go mapper already computes it) would eliminate this class of false positive entirely.

### What the ranking cannot capture

- **Severity.** A deviation in error handling on a critical path (payment processing) matters more than one in a debug utility. The z-score measures statistical strength, not business impact.
- **Historical intent.** A deviation introduced in a recent commit is more likely a bug than one present since the module's creation. The tool has no git history context.
- **Compensating code.** A caller that appears to "ignore" an error may have a `defer` recovery mechanism or a surrounding retry loop that the AST pattern classifier fails to recognize.

### Open design questions

1. ~~**Framework vs. scripts.**~~ **Resolved: DSL.** The belief DSL ([BELIEF-DSL.md](BELIEF-DSL.md)) is a pure Scheme library. Beliefs are `define-belief` forms, not hand-written scripts. The DSL handles layer selection, data loading, and statistical comparison. See `BELIEF-DSL.md` for design, trade-offs, and limitations.
2. **Belief discovery.** Engler's system requires the analyst to choose which belief category to check. Belief bootstrapping (§ Belief Bootstrapping) partially addresses this — discovered beliefs generate enumeration sets for other categories, so the analyst specifies starting categories but not cross-category patterns. Full automatic discovery (mining arbitrary patterns) overlaps with specification mining (Ammons, Bodik, Larus 2002).
3. **Incremental analysis.** Running consistency analysis on every commit requires incremental belief updates as code changes. The current architecture (load full module, analyze from scratch) does not support this. CI integration would demand incremental analysis.
4. **Threshold sensitivity.** The validation chose 66%/3 empirically. How sensitive are results to these parameters? Would 50%/3 or 80%/5 tell a substantially different story? No sensitivity analysis has been performed. Future validation should at minimum report results at 2-3 threshold levels to characterize the signal-to-noise tradeoff.
5. **Minimum corpus size.** At what scale does the approach produce meaningful signal? The co-mutation validation used ~60 SSA functions. A 5-function package would produce no beliefs above threshold. Guidance on minimum viable corpus size would help users choose between this tool and manual inspection.

## Future Enhancements

- **Parameter essentiality integration.** Combine with SSA-based parameter usage analysis to answer: "Does this deviation exist because the parameter is a pass-through that should not exist?" A function that ignores an error because the result is a pass-through has a different root cause than one that ignores an error by mistake.
- **Automatic belief discovery.** Mine frequent patterns from the codebase without specifying belief categories. Extract all (operation, context) pairs, cluster by frequency, report outliers. Belief bootstrapping (§ Belief Bootstrapping) is a middle ground — cheaper than full mining, feeding belief output as enumeration input for other categories. Full specification mining eliminates even the starting category requirement but costs more to compute.
- **Git-aware ranking.** Weight deviations by recency: recent deviations in functions with historically consistent behavior are more likely regressions. Requires git history integration, outside the current goast scope.
- **Unification detector cross-reference.** When the unification detector finds two merge candidates, check whether their callers handle them consistently. If callers handle the two functions differently, unification may break caller expectations.
- **Lint-layer meta-analysis.** Run `go-analyze` with all available analyzers, then apply consistency analysis to the *diagnostic pattern* — functions that trigger a diagnostic when peer functions do not. A second-order consistency check: the lint layer's output becomes input to the consistency layer.
