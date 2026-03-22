# Belief DSL

**Status**: Implemented
**Foundation**: [CONSISTENCY-DEVIATION.md](CONSISTENCY-DEVIATION.md) — belief model, five categories, bootstrapping
**Dependencies**: All five goast layers (AST, SSA, CFG, call graph, lint)
**Implementation**: Pure Scheme library on top of existing goast primitives

## Problem

Writing a consistency belief requires ~70 lines of domain logic plus ~170 lines of shared infrastructure (traversal, filtering, statistical comparison). The infrastructure is duplicated across scripts. The domain logic follows a rigid pattern — enumerate sites, characterize each, compare statistically — but must be reimplemented from scratch for each belief.

A declarative DSL lets the user (or an LLM) express beliefs as data. The DSL interpreter handles layer selection, data loading, and statistical comparison. Simple beliefs become 3-5 lines; composed beliefs with bootstrapping chains are 10-15 lines. Both are substantially less than 70+.

## Design

### Belief Definition Form

```scheme
(define-belief <name:string>
  (sites <site-selector>)
  (expect <property-checker>)
  (threshold <min-adherence:number> <min-sites:number>))
```

- **`name`** — string identifier, used in output and for bootstrapping references between beliefs.
- **`sites`** — enumerates code locations to analyze. Returns a list of sites. Each site is an opaque value whose structure depends on the selector (function AST, call site, SSA function).
- **`expect`** — classifies each site into a category symbol (`'present`, `'absent`, `'paired`, `'unpaired`, etc.). The majority category becomes the belief. Minority categories are deviations.
- **`threshold`** — minimum adherence ratio and minimum site count for reporting. Same semantics as `CONSISTENCY-DEVIATION.md` §Ranking.

### Site Selectors

```scheme
;; Functions matching structural criteria (composable predicates)
(functions-matching <predicate>...)

;; All callers of a named function (call graph layer)
(callers-of <func-name:string>)

;; All methods on a receiver type (AST layer)
(methods-of <type-name:string>)

;; Output of another belief — bootstrapping hook
(sites-from <belief-name:string> [#:which 'adherence|'deviation])
```

`sites-from` references another belief's results. `#:which` selects adherence sites (default) or deviation sites. This is the bootstrapping mechanism from `CONSISTENCY-DEVIATION.md` §Belief Bootstrapping — vertical refinement becomes:

```scheme
(define-belief "counter-comutation"
  (sites (functions-matching (stores-to-fields "VMCounters" "StackDrains" "ForeignCalls")))
  (expect (co-mutated "StackDrains" "ForeignCalls"))
  (threshold 0.66 3))

(define-belief "counter-ordering"
  (sites (sites-from "counter-comutation" #:which 'adherence))
  (expect (ordered "StackDrains" "ForeignCalls"))
  (threshold 0.85 5))
```

#### Predicates for `functions-matching`

```scheme
(has-params <type-string>...)          ;; signature contains these param types
(has-receiver <type-string>)           ;; method receiver matches
(name-matches <pattern:string>)        ;; function name glob/regex
(contains-call <func-name:string>...)  ;; body calls any of these (pre-filter)
(stores-to-fields <struct> <field>...) ;; SSA: function stores to these fields
```

Predicates compose with `all-of`, `any-of`, `none-of`:

```scheme
(sites (functions-matching
         (all-of (has-receiver "*Server")
                 (contains-call "Write"))))
```

### Property Checkers

Each checker classifies a site into a category symbol. The majority wins; minorities are deviations.

```scheme
;; Does the function body call any of these? (AST)
(contains-call <func-name:string>...)
;; -> 'present | 'absent

;; Are two operations both present? (AST + CFG)
(paired-with <op-a:string> <op-b:string>)
;; -> 'paired-defer | 'paired-call | 'unpaired

;; Does op-a dominate op-b in CFG? (CFG)
(ordered <op-a:string> <op-b:string>)
;; -> 'a-dominates-b | 'b-dominates-a | 'same-block | 'unordered

;; Are these fields all stored together? (SSA)
(co-mutated <field:string>...)
;; -> 'co-mutated | 'partial

;; Is a value checked before use? (SSA + CFG)
(checked-before-use <value-pattern:string>)
;; -> 'guarded | 'unguarded

;; Escape hatch: full Scheme lambda, must return a category symbol
(custom <lambda:(site -> symbol)>)
```

The checker determines which goast layer is used. The user does not select layers — the verb implies the layer. `contains-call` uses AST. `ordered` uses CFG + dominance. `co-mutated` uses SSA. Layer data is loaded lazily by the runner (see below).

**Limitation:** AST-level call detection (`contains-call`, `paired-with`) sees only direct calls by name. Method values assigned to variables, calls through interfaces, and calls through closures are invisible. This is acceptable for convention detection (conventions are typically expressed as direct calls) but means the DSL cannot verify properties that depend on indirect call resolution.

The `custom` escape hatch accepts a lambda that receives the site value and a context object, and returns a category symbol. This covers domain-specific checks that don't decompose into built-in verbs. The site value is a function AST node; the context provides access to loaded analysis layers. If many beliefs require `custom`, the built-in verb vocabulary is likely too narrow — consider promoting frequent patterns to core verbs.

### Runner

```scheme
;; Load target, run all beliefs defined in this file
(run-beliefs <target:string>)
```

`run-beliefs` performs:

1. **Load AST** — `go-typecheck-package` the target. Always loaded (all beliefs need at least AST).
2. **Lazy SSA** — `go-ssa-build` only if any belief uses `co-mutated`, `checked-before-use`, or `stores-to-fields`.
3. **Lazy call graph** — `go-callgraph` only if any belief uses `callers-of`.
4. **Topological sort** — order beliefs by `sites-from` dependencies. Cycle = error.
5. **Evaluate each belief** — enumerate sites, map checker, statistical comparison, collect results.
6. **Store results** — adherence/deviation site lists keyed by belief name, available to downstream `sites-from`.
7. **Print report**.

Lazy loading matters: a file with only AST-level beliefs should not pay for SSA or call graph construction.

### Output Format

Matches existing example scripts:

```
══════════════════════════════════════════════════
  Consistency Analysis: my-beliefs.scm
══════════════════════════════════════════════════

── Belief: auth-before-business ──
  Pattern: present (14/15 sites)
    DEVIATION: handleLegacyWebhook -> absent

── Belief: lock-unlock-pairing ──
  Pattern: paired-defer (8/8 sites)
    (no deviations)

── Summary ──
  Beliefs evaluated:   4
  Strong beliefs:      3
  Deviations found:    1
```

## Examples

### Pairing Belief

```scheme
(define-belief "lock-unlock-pairing"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.90 5))
```

### Check Belief

```scheme
(define-belief "nil-check-before-use"
  (sites (functions-matching (has-params "*Config")))
  (expect (checked-before-use "Config"))
  (threshold 0.90 5))
```

### Handling Belief (cross-layer)

```scheme
(define-belief "query-error-handling"
  (sites (callers-of "(*DB).Query"))
  (expect (contains-call "fmt.Errorf" "errors.Wrap"))
  (threshold 0.85 5))
```

### Ordering Belief

```scheme
(define-belief "validate-before-write"
  (sites (callers-of "(*DB).Write"))
  (expect (ordered "Validate" "Write"))
  (threshold 0.85 5))
```

### Bootstrapping Chain

```scheme
(define-belief "stepping-comutation"
  (sites (functions-matching
           (stores-to-fields "Debugger" "stepMode" "stepFrame" "stepFrameDepth")))
  (expect (co-mutated "stepMode" "stepFrame" "stepFrameDepth"))
  (threshold 0.66 3))

(define-belief "stepping-ordering"
  (sites (sites-from "stepping-comutation" #:which 'adherence))
  (expect (ordered "stepMode" "stepFrame"))
  (threshold 0.85 3))
```

### Custom Escape Hatch

```scheme
(define-belief "idempotency-key-present"
  (sites (callers-of "(*PaymentService).Charge"))
  (expect (custom (lambda (site ctx)
    (let ((body (nf site 'Body)))
      (if (walk body (lambda (n) (and (tag? n 'CallExpr)
                                       (member? "WithIdempotencyKey"
                                                (nf n 'Fun)))))
          'present 'missing)))))
  (threshold 1.0 3))
```

The `custom` lambda receives both the site (function AST node) and the analysis context. It uses the shared utilities (`nf`, `walk`, `tag?`, `member?`) re-exported from `(wile goast utils)`. This example is longer than the built-in verbs — that's the trade-off of the escape hatch.

## Implementation Strategy

### Pure Scheme Library

The DSL is a Scheme library file, not new Go code. It builds on existing goast primitives:

- `go-typecheck-package`, `go-ssa-build`, `go-callgraph` for data loading
- `go-cfg-dominates?` for ordering checks
- AST traversal via the shared `walk`/`nf`/`tag?` utilities

New verbs are Scheme functions. If a verb proves to be a performance bottleneck, it can be promoted to a Go primitive later.

### File Organization

```
cmd/wile-goast/lib/wile/goast/
  belief.sld     ;; R7RS library definition, exports
  belief.scm     ;; Full implementation: define-belief, run-beliefs, selectors, checkers
  utils.sld      ;; Shared utility library definition
  utils.scm      ;; Traversal utilities (nf, walk, tag?, filter-map, etc.)
```

The planned four-file split (selectors, checkers, utils, core) collapsed to two files during implementation — the separation added indirection without benefit at this scale. The library is importable as `(wile goast belief)` and embedded in the binary via `go:embed` + `WithSourceFS`.

### Shared Utilities

The traversal functions duplicated across example scripts (`nf`, `walk`, `tag?`, `filter-map`, `flat-map`, `member?`, `unique`, `ordered-pairs`) are extracted into `(wile goast utils)` and re-exported by the belief library for use in `custom` lambdas.

## Belief Graduation

Discovery scripts (e.g., `consistency-comutation.scm`) sweep broadly — all field pairs, all callers — and find patterns statistically. Once a pattern is verified, it should become a coded belief. The question: what's the artifact of discovery?

**Answer: discovery produces `define-belief` forms.**

The output of a discovery run is not just a report — it's candidate Scheme code:

```
══════════════════════════════════════════════════
  Discovery: Co-Mutation
══════════════════════════════════════════════════

;; Debugger stepping fields: stepMode + stepFrame
;; Adherence: 75% (3/4 sites), Deviations: StepOver
;;
(define-belief "Debugger:stepMode+stepFrame"
  (sites (functions-matching
           (stores-to-fields "Debugger" "stepMode" "stepFrame")))
  (expect (co-mutated "stepMode" "stepFrame"))
  (threshold 0.66 3))
```

### Lifecycle

```
discover → review → commit → enforce
   │                  │         │
   │  human judgment  │  CI     │
   ▼                  ▼         ▼
 candidates       belief file  run-beliefs
 (stdout)         (.scm)       (exit code)
```

1. **Discover** — broad sweep produces `define-belief` candidates on stdout.
2. **Review** — human reads candidates, discards false positives, accepts real conventions.
3. **Commit** — accepted beliefs go into a `.scm` file checked into the repo.
4. **Enforce** — `run-beliefs` in CI evaluates committed beliefs, fails on deviations.

### Suppression

Future discovery runs diff their output against committed belief files. A belief whose `(sites ...)` and `(expect ...)` match an existing `define-belief` is suppressed — discovery only reports *new* findings. This avoids re-reporting known conventions.

The diff is structural, not textual: two beliefs match if they have the same selector type, the same target fields/functions, and the same checker. Names and thresholds don't matter for matching.

### Discovery as Code Generation

Discovery scripts gain a `--emit` mode (or equivalent flag) that switches output from human-readable report to `define-belief` forms. The default remains the report for exploration; `--emit` is for graduation.

This means the discovery scripts and the belief DSL share a common intermediate representation — `define-belief` is both the output of discovery and the input to evaluation. The lifecycle has a shared representation for its first three stages (discover → review → commit), though enforcement produces a report, not `define-belief` forms.

## Trade-offs

| Gain | Cost |
|------|------|
| Simple beliefs are 3-5 lines instead of 70+ | New verbs require adding combinators to the library |
| Bootstrapping composition via `sites-from` | Topological sort adds complexity to the runner |
| Layer selection is implicit (verb determines layer) | Debugging layer issues requires understanding the mapping |
| Lazy loading avoids unnecessary analysis passes | Runner must track which layers each verb needs |
| `custom` escape hatch prevents DSL dead-ends | Overuse of `custom` defeats the purpose of the DSL |

**Hypothesis (untested):** LLMs can generate beliefs from English descriptions. This is plausible given Scheme's syntactic regularity and the DSL's constrained vocabulary, but no controlled comparison exists against alternative representations (Go DSL, Python bindings, structured JSON).

## Limitations

Inherited from [CONSISTENCY-DEVIATION.md](CONSISTENCY-DEVIATION.md) — these apply to the belief DSL as a specific implementation of that model.

### The majority assumption

The statistical model assumes the majority category represents the intended convention. If a codebase has 60% incorrect handling and 40% correct, the DSL reports the correct code as deviations. The threshold mechanism mitigates this (high thresholds require strong majorities) but does not eliminate it. Beliefs with low adherence ratios should be treated as candidates for human review, not as confirmed conventions.

### AST-level call detection

`contains-call` and `paired-with` detect calls by name in the AST. They miss: method values assigned to variables, calls through interfaces, indirect calls through closures, and calls generated by code generation. This is acceptable for convention detection (conventions are typically direct calls) but means some valid adherence sites may be misclassified as deviations.

### Field name ambiguity

`stores-to-fields` and `co-mutated` identify fields by name string, not by qualified type. When two struct types in the same package share a field name, stores to one type can be mis-attributed to the other. The implementation mitigates this via receiver-type disambiguation (see `CONSISTENCY-DEVIATION.md` §Limitations, item 6) but the heuristic fails when two structs have identical field sets.

### No severity ranking

A deviation in error handling for a critical path matters more than one in a debug utility. The DSL reports deviations uniformly — it measures statistical strength, not business impact.

## Boundary Conditions

The DSL is unlikely to produce useful results when:

- **Too few sites.** The `min-sites` threshold filters small populations, but even with the filter, beliefs over 5-10 sites have high variance. Meaningful statistical signal requires at least ~20 sites per belief.
- **No conventions exist.** Greenfield code or code with deliberately varied patterns won't produce strong majorities. The DSL will either find no beliefs above threshold or report noise.
- **Beliefs require interprocedural reasoning.** The built-in verbs operate within a single function's scope (its AST, its CFG, its SSA). Properties that span call chains ("every path from A to B passes through C") require `custom` lambdas with manual call graph traversal.
- **Belief conflicts.** Two beliefs can produce contradictory signals about the same function (adherence in one, deviation in another). There is no priority or conflict resolution mechanism — the user must interpret conflicting signals.

## Relationship to Existing Plans

This plan implements the "Framework vs. scripts" open question from `CONSISTENCY-DEVIATION.md` §Open Design Questions (line 691). The answer: a combinator library (framework-lite) that generates the same pipeline the scripts implement manually. Each belief category from `CONSISTENCY-DEVIATION.md` §Belief Categories maps to one or more built-in verbs:

| Category | Site Selector | Property Checker |
|----------|--------------|-----------------|
| Pairing (§1) | `functions-matching` + `contains-call` | `paired-with` |
| Check (§2) | `functions-matching` + `has-params` | `checked-before-use` |
| Handling (§3) | `callers-of` | `contains-call` (for handler classification) |
| Ordering (§4) | `functions-matching` or `callers-of` | `ordered` |
| Co-mutation (§5) | `functions-matching` + `stores-to-fields` | `co-mutated` |

The bootstrapping model from `CONSISTENCY-DEVIATION.md` §Belief Bootstrapping maps to `sites-from` with `#:which` keyword. The funnel property holds by construction: `sites-from` draws from either the adherence or deviation partition of a prior belief's results, which is always a subset of that belief's site set. A downstream belief cannot produce more sites than its upstream source. However, the funnel only constrains site *count* — a downstream belief can reclassify adherence sites from the upstream belief as deviations under a stricter property.
