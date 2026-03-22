# Procedure Unification Detection

**Status**: Prototype validated (single codebase; substitution collapsing implemented; SSA pass designed but unbuilt)
**Foundation**: [wile-goast](https://github.com/aalpar/wile-goast) — all five goast layers (see `plans/GO-STATIC-ANALYSIS.md`)
**Dependencies**: None beyond existing goast infrastructure
**Implementation**: Pure Scheme rule using `(wile goast)` primitives

## Problem

Two procedures that share most of their structure but differ in a few parameterizable ways are a unification candidate — a single procedure with parameters could replace both. Mechanical detection of these candidates helps maintain codebases where duplication evolves gradually.

## Objective Precondition for Unification

One objective precondition exists: **agreement on the shared domain** — where input spaces overlap, the procedures must produce identical results. This is a definitional choice: if `f(x) != g(x)` for some `x` in both domains, this plan treats that as dispatch behind a flag, not unification. Other refactoring tools may consider flag-parameterized dispatch valid unification; this plan excludes it because flag parameters cost too much complexity (see diff classification, "Control flow" row).

Beyond that, every question is a design judgment: does unification reduce total complexity or increase it? The rule identifies candidates where the complexity equation favors unification; the human decides.

## Complexity Model

```
cost_before  = size(f) + size(g)
cost_after   = size(h) + size(param_types) + call_sites * size(param_passing)
```

Unification reduces complexity when `cost_after < cost_before`. The shared structure cancels out — the question is whether **parameter overhead** exceeds **duplicated code**.

## Layer Strategy: AST Primary, SSA Secondary

The tool serves two purposes at two layers.

1. **Refactoring advisor (AST, primary).** "These two functions share 95% structure — here are the parameterizable differences, consider unifying." AST with type annotations (via `go-typecheck-package`) is the right level because:

   - **AST preserves structure.** Type differences localize to specific nodes (the `inferred-type` annotation), making "same structure, different types" easy to identify — the core unification signal.
   - **AST retains programmer intent.** Variable names, function names, and structural choices survive. SSA erases these, obscuring the distinction between semantic and accidental differences.
   - **SSA normalizes away structural similarity.** Two functions doing "the same thing with different types" produce different SSA instruction sequences because types flow through every instruction. Structural similarity obvious in the AST disappears in SSA.

2. **Equivalence detector (SSA, secondary — not yet implemented).** "These two expressions compute the same thing via different operator arrangements." SSA should be the right layer because Go's SSA builder canonicalizes some operand orderings and folds constants. Operator algebra (commutativity, identity elimination) should fall out of the representation without custom rewrite rules — but this predicts the v2 pass, not describes an implementation.

The two layers answer different questions. The refactoring advisor asks "same **shape**, different **leaves**?" — a structural question. The equivalence detector asks "same **result**, different **shape**?" — a semantic question. AST comparison is the workhorse; SSA comparison catches what AST comparison cannot.

## Detectable Signals

Three signals, all mechanically computable from the AST:

### 1. Structural Similarity Ratio

Recursive comparison of two function ASTs as s-expressions. Each node is a tagged alist `(tag (key . val) ...)` — same tag means compare children; different tag means structural divergence.

Metric: `shared_nodes / total_nodes`. High ratio = unification candidate.

This primary filter eliminates pairs that share a name or a few lines but differ structurally.

### 2. Difference Classification

Not all AST differences are equal. Each differing node falls into a category:

| Diff kind | Example | Parameterizable as | Unification cost |
|-----------|---------|-------------------|------------------|
| Type name | `int64` vs `uint64` (in `inferred-type` or ident nodes) | Type parameter | Low |
| Literal value | `0` vs `1`, `"add"` vs `"remove"` | Value parameter | Low |
| Identifier (local) | `x` vs `y` (variable names) | Rename (free) | Zero |
| Called function | `f.Inc()` vs `f.Dec()` | Callback / method value | Medium |
| Control flow | `if` present in one, absent in other | Flag parameter + branch | High |
| Structural | Different loop structure, different return paths | Not parameterizable | Reject |

Leaf-only edits (types, literals, identifiers) unify cheaply. Structural edits mean the procedures differ genuinely.

The `inferred-type` annotation from `go-typecheck-package` is essential for distinguishing type-name diffs from identifier diffs — without type info, `int64` and `myVar` are both just `(ident (name . "..."))`.

### 3. Difference Regularity

If all diffs are the *same kind* (e.g., every difference is a type substitution `int64` -> `uint64`), a single parameter covers all of them. Heterogeneous diffs (a type here, a literal there, a callback somewhere else) each need their own parameter, inflating the unified signature.

Metric: `parameter_count / shared_node_count`. Low = good candidate.

## Rule Architecture

### Pass 0: Candidate Enumeration

```scheme
;; Load all packages, extract func-decl nodes
(define pkgs (go-typecheck-package target))

;; Extract all func-decls with their signature shape
;; Shape = (param-count . return-count)
(define (signature-shape func-decl)
  (let* ((ftype (nf func-decl 'type))
         (params (nf ftype 'params))
         (results (nf ftype 'results))
         (param-count (if (pair? params) (length params) 0))
         (result-count (if (pair? results) (length results) 0)))
    (cons param-count result-count)))

;; Group functions by signature shape to avoid O(n^2) full comparison
;; Only compare functions within the same shape group
```

**Pre-filter rationale:** Two functions with different signature shapes (different param/return counts) require adding or removing parameters to unify — a structural difference. Grouping by shape cuts pair enumeration from O(n^2) to O(sum of k_i^2) where k_i is group size.

Additional pre-filters to consider:
- Same receiver type (methods) or both free functions
- ~~Same package (cross-package unification is architecturally suspect)~~ — Superseded by validation: the strongest findings (ewflag/dwflag) are cross-package pairs. Cross-package comparison is valuable.
- Minimum function size (skip trivial 1-3 line functions)

**Signature shape strictness:** This pre-filter eliminates candidates where functions differ by one parameter (e.g., one has a context parameter the other lacks). The strictness prevents comparing pairs where the signature difference itself is the unification parameter. Relaxing to `|param-count-diff| ≤ 1` would widen the candidate set at the cost of more comparisons.

### Pass 1: Structural Comparison (AST Diff)

Core algorithm: recursive s-expression comparison.

```scheme
;; Compare two AST nodes, returning (shared-count diff-count diffs)
;;
;; Two nodes match when:
;;   - Same tag AND all children match (recursively)
;;   - Both are identical atoms (strings, symbols, numbers, booleans)
;;
;; A diff is: (path node-a node-b category)
;;   where path is a list of field keys from root to the diff point

(define (ast-diff node-a node-b path)
  (cond
    ;; Both tagged alists with same tag -> compare field-by-field
    ((and (pair? node-a) (pair? node-b)
          (symbol? (car node-a)) (symbol? (car node-b))
          (eq? (car node-a) (car node-b)))
     (fields-diff (cdr node-a) (cdr node-b) path))

    ;; Both tagged alists with different tags -> structural diff
    ((and (pair? node-a) (pair? node-b)
          (symbol? (car node-a)) (symbol? (car node-b)))
     (make-diff 'structural path node-a node-b))

    ;; Both lists (child node sequences) -> element-wise comparison
    ((and (pair? node-a) (pair? node-b)
          (not (symbol? (car node-a))))
     (list-diff node-a node-b path 0))

    ;; Both atoms, equal -> match
    ((equal? node-a node-b)
     (make-match 1))

    ;; Both atoms, different -> classify the leaf diff
    (else
     (make-diff (classify-leaf node-a node-b) path node-a node-b))))
```

**List diff strategy for v1:** Element-wise positional comparison (zip). Extra elements in unequal-length lists count as structural diffs. Simpler than full tree edit distance, but with a significant limitation: inserting a single statement at the beginning shifts every subsequent pair, making all of them "different" even if the remaining statements are identical. The tool detects substitution similarity only, not insertion/deletion similarity.

On the crdt validation set, this limitation does not matter — candidates differ by type substitution, not insertion. On codebases with more organic duplication, it would reduce recall.

Full tree edit distance (handling insertions and deletions) is a v2 enhancement for functions that differ by a single added or removed statement.

### Pass 2: Difference Classification

```scheme
;; Classify a leaf diff using AST context and type annotations
(define (classify-leaf node-a node-b)
  (cond
    ;; Both are type-position idents with inferred-type
    ;; (the inferred-type differs -> type parameter candidate)
    ((and (tag? node-a 'ident) (tag? node-b 'ident)
          (nf node-a 'inferred-type) (nf node-b 'inferred-type)
          (not (equal? (nf node-a 'inferred-type)
                       (nf node-b 'inferred-type))))
     'type-name)

    ;; Both are idents without type info, or with same type
    ;; (variable rename -> free unification)
    ((and (tag? node-a 'ident) (tag? node-b 'ident))
     'identifier)

    ;; Both are literals with same kind but different value
    ((and (tag? node-a 'lit) (tag? node-b 'lit)
          (equal? (nf node-a 'kind) (nf node-b 'kind)))
     'literal-value)

    ;; Both are literals with different kind
    ((and (tag? node-a 'lit) (tag? node-b 'lit))
     'type-name)

    ;; Both are symbols (operators, tokens) -> operator diff
    ((and (symbol? node-a) (symbol? node-b))
     'operator)

    ;; Fallback
    (else 'structural)))
```

### Pass 3: Scoring and Reporting

```scheme
;; Weights for different diff categories.
;; These values were calibrated on the crdt codebase, where the
;; separation between genuine candidates (cost 0) and noise (cost 100+)
;; is binary. On codebases with more heterogeneous duplication, the
;; intermediate range (cost 5-20) may contain interesting candidates
;; that these weights would reject.
(define diff-weights
  '((type-name . 1)      ;; one type parameter covers all
    (literal-value . 1)   ;; one value parameter per distinct literal
    (identifier . 0)      ;; free: just a rename
    (operator . 2)        ;; callback or flag parameter
    (structural . 100)))  ;; effectively rejects the pair

;; Score a diff result
(define (unification-score shared-count diffs)
  (let* ((diff-cost (apply + (map (lambda (d)
                                     (cdr (assoc (diff-category d) diff-weights)))
                                   diffs)))
         ;; Count distinct parameter types needed
         (type-params (unique (filter-map
                                (lambda (d)
                                  (and (eq? (diff-category d) 'type-name)
                                       (cons (diff-value-a d) (diff-value-b d))))
                                diffs)))
         (value-params (filter (lambda (d) (eq? (diff-category d) 'literal-value)) diffs))
         (param-count (+ (length type-params) (length value-params)))
         ;; Similarity ratio
         (total (+ shared-count (length diffs)))
         (similarity (if (> total 0) (/ shared-count total) 0)))
    (list similarity param-count diff-cost diffs)))

;; Threshold: report pairs with >= 70% structural similarity
;; and no structural diffs
(define similarity-threshold 0.70)

(define (report-candidate func-a func-b score)
  (let ((name-a (nf func-a 'name))
        (name-b (nf func-b 'name))
        (similarity (car score))
        (param-count (cadr score))
        (diffs (cadddr score)))
    (display "  Candidate: ") (display name-a)
    (display " <-> ") (display name-b)
    (display "  similarity=") (display similarity)
    (display "  params-needed=") (display param-count)
    (newline)
    ;; Detail the diffs
    (for-each
      (lambda (d)
        (display "    ") (display (diff-category d))
        (display " at ") (display (diff-path d))
        (display ": ") (display (diff-value-a d))
        (display " -> ") (display (diff-value-b d))
        (newline))
      diffs)))
```

## Existing Prior Art in This Codebase

The `state-trace-full.scm` example (in [wile-goast](https://github.com/aalpar/wile-goast) `examples/goast-query/`) demonstrates this multi-layer analysis pattern:

- **Pass 1 (AST):** `walk` + `tag?` + `nf` for structural pattern detection (boolean clusters)
- **Pass 2 (AST):** If-chain field sweep detection via recursive condition extraction
- **Pass 3 (SSA):** `go-ssa-build` + field-addr/store correlation for mutation independence
- **Pass 4 (CFG):** `go-cfg` + `go-cfg-dominators` + `go-cfg-dominates?` for ordering

The `walk`, `nf` (node-field), `tag?`, `filter-map`, and `flat-map` utilities from that example are directly reusable. The unification rule adds a new visitor pattern (pairwise tree diff) but shares the same traversal infrastructure.

## Validation Results

The prototype (`examples/goast-query/unify-detect-pkg.scm`) ran on the full `crdt` module (17 packages, 132 functions with >=3 statements). Of 399 cross-package pairs above the 60% effective similarity threshold, exactly **4 had zero weighted cost** — the strong unification candidates:

| Eff. Sim | Cost | Root Params | Pair | Category |
|---|---|---|---|---|
| 99.4% | 0 | `ewflag→dwflag`, `EWFlag→DWFlag` | `ewflag.Enable` ↔ `dwflag.Disable` | Dual discovery |
| 99.3% | 0 | `ewflag→dwflag`, `EWFlag→DWFlag` | `ewflag.Disable` ↔ `dwflag.Enable` | Dual discovery |
| 97.9% | 0 | `pncounter→gcounter`, `CounterValue→GValue`, `int64→uint64` | `pncounter.Increment` ↔ `gcounter.Increment` | Known pattern |
| 97.6% | 0 | same 3 roots | `pncounter.Value` ↔ `gcounter.Value` | Known pattern |

The remaining 395 pairs have weighted cost >= 100 (structural diffs, missing elements, cross-domain mismatches). On this codebase, the weighted cost filter produces binary separation — zero cost (genuine candidates) vs. 100+ cost (noise), with nothing in between. Whether this clean separation generalizes to codebases with more heterogeneous duplication remains untested.

### pncounter ↔ gcounter (known, validated)

`pncounter` and `gcounter` share the find-own-dot/replace/build-delta pattern (`int64` vs `uint64`). The rule detects `Increment` and `Value` as candidates with 3 root type substitutions. gcounter's grow-only constraint (no `Decrement` method) is a semantic difference — it manifests as a missing method, not an AST diff within shared methods. The rule correctly identifies the *shared* methods as candidates; the *absent* method never appears.

### ewflag ↔ dwflag (discovered by the rule)

EWFlag (enable-wins) and DWFlag (disable-wins) are exact duals. The rule discovered:

- `ewflag.Enable` is structurally identical to `dwflag.Disable` — both generate a dot via `Context.Next(id)` and add it to the store. The sole diff is the function name.
- `ewflag.Disable` is structurally identical to `dwflag.Enable` — both drain the store into a context via `Range` and return an empty store.

Root substitutions: `ewflag->dwflag` (package) and `EWFlag->DWFlag` (type name). After collapsing 31 and 27 derived `inferred-type` diffs respectively, effective similarity reaches 99.4% and 99.3%.

Previously, this duality existed only in prose (CLAUDE.md: "DWFlag: Exact dual of EWFlag — dots = disable events, empty DotSet = enabled"). The rule discovered the structural relationship mechanically, from AST comparison alone.

**Implications:** EWFlag and DWFlag could share a single implementation parameterized by the semantic interpretation of "dots present" (enabled vs. disabled). Whether dual naming adds more confusion than the duplication removes is a judgment call, but the rule correctly surfaces the candidate.

### Substitution collapsing (load-bearing)

Type annotations from `go-typecheck-package` propagate root type substitutions into every sub-expression. `pncounter.Increment` has 95 raw type-name diffs — but they collapse to 3 root substitutions (`pncounter->gcounter`, `CounterValue->GValue`, `int64->uint64`). Without collapsing, similarity is 73.6%; after collapsing, 97.9%. The collapsing algorithm sorts type-name diff pairs by string length (shortest first), then iteratively checks whether longer pairs derive from known roots via substring replacement.

**This is not a refinement — it is essential.** At the 70% similarity threshold (line 217), the pncounter/gcounter pair barely passes without collapsing (73.6%). At any higher threshold, collapsing separates detecting the tool's strongest candidates from missing them entirely. The 25-percentage-point recovery (73.6% to 97.9%) demonstrates that raw AST comparison systematically underestimates similarity when type annotations propagate through the tree.

## Limitations

### What this rule cannot detect

1. **Semantic equivalence (v1).** Two functions that compute the same result through different algorithms will not be flagged — their ASTs differ structurally. The SSA-level equivalence pass (v2) partially addresses operator-level differences (commutativity, constant folding, strength reduction) but not genuinely different algorithms.
2. **Domain agreement violations.** The objective precondition (agreement on shared domain) is a semantic property, not checkable statically from AST structure alone.
3. **Cross-function duplication.** A pattern repeated *within* different functions (not the entire function body) requires sub-tree matching, not whole-function comparison — a different problem (clone detection at the fragment level).
4. **Macro-generated duplication.** Go lacks macros, but code generators (`go generate`) can produce structural clones that this rule would flag. Whether that helps depends on whether the generator itself should be fixed.

### What the scoring cannot capture

The scoring function approximates "does unification reduce complexity?" but cannot answer it fully:

- **Call-site cost** is not measured (how many callers would need to change?)
- **Cognitive cost** of the unified function's signature is subjective
- **Package boundary effects** (would unification create a new dependency?) are not checked
- **Interface compliance** — two functions might be structurally similar but satisfy different interface contracts; unifying them could break interface compliance

These require human judgment. The rule surfaces candidates; the human decides.

### Validation limitations

- **Single codebase.** All results come from crdt (17 packages). The crdt library deliberately duplicates CRDT types with the same patterns for different value types — nearly ideal for this detection. A codebase with more organic, irregular duplication would stress the tool differently.
- **No false positive rate.** With 4 positives, the false positive rate cannot be estimated. All 4 candidates are genuine structural matches, but whether a human would choose to unify them remains a judgment call (the document acknowledges this for ewflag/dwflag).
- **Type-substitution dominance.** All 4 findings are type-substitution patterns. Since Go 1.18, generics can eliminate this class of duplication at the source. As generics adoption grows, the tool's primary finding category may shrink. The tool remains useful for non-type duplication (literal values, function names), but these produce higher diff costs and less often yield zero-cost candidates.

## Future Enhancements (v2+)

- **SSA-level equivalence detection:** For functions that pass the AST filter, compare SSA representations to detect operator-level equivalence. Go's SSA builder already normalizes operand order, folds constants, and applies strength reductions — yielding commutativity, identity elimination, and similar algebraic properties without custom rewrite rules. The comparison leverages what the compiler knows for free rather than reimplementing algebraic laws in Scheme.
- **CFG isomorphism:** Compare control flow graphs to detect functions with identical branching structure but different computations. Combined with AST diff, this distinguishes "same algorithm, different types" from "different algorithm, same types."
- **Sub-tree matching:** Detect duplicated code *fragments* within different functions, not just whole-function similarity. Requires sliding-window or suffix-tree approaches on the s-expression representation.
- **Call graph context:** Use `go-callgraph` to find functions that call the same dependencies — a pre-filter narrowing candidates to functions with similar "purpose signatures."
- ~~**Cross-package analysis:**~~ Done. The `./...` module-wide scan compares all cross-package function pairs within signature-shape groups. Validated on 17 packages / 132 functions.
