# Belief DSL

**Status**: Proposed
**Foundation**: [CONSISTENCY-DEVIATION.md](CONSISTENCY-DEVIATION.md) — belief model, five categories, bootstrapping
**Dependencies**: All five goast layers (AST, SSA, CFG, call graph, lint)
**Implementation**: Pure Scheme library on top of existing goast primitives

## Problem

Writing a consistency belief requires ~70 lines of domain logic plus ~170 lines of shared infrastructure (traversal, filtering, statistical comparison). The infrastructure is duplicated across scripts. The domain logic follows a rigid pattern — enumerate sites, characterize each, compare statistically — but must be reimplemented from scratch for each belief.

A declarative DSL lets the user (or an LLM) express beliefs as data. The DSL interpreter handles layer selection, data loading, and statistical comparison. Custom beliefs become 3-5 lines instead of 70+.

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

Predicates compose with `and`, `or`, `not`:

```scheme
(sites (functions-matching
         (and (has-receiver "*Server")
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

The `custom` escape hatch accepts a lambda that receives the site value and returns a category symbol. This covers domain-specific checks that don't decompose into built-in verbs. Frequent `custom` usage is a signal that a new verb should be promoted to the core set.

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
  (expect (custom (lambda (site)
    (if (contains-call? site "WithIdempotencyKey") 'present 'missing))))
  (threshold 1.0 3))
```

## Implementation Strategy

### Pure Scheme Library

The DSL is a Scheme library file, not new Go code. It builds on existing goast primitives:

- `go-typecheck-package`, `go-ssa-build`, `go-callgraph` for data loading
- `go-cfg-dominates?` for ordering checks
- AST traversal via the shared `walk`/`nf`/`tag?` utilities

New verbs are Scheme functions. If a verb proves to be a performance bottleneck, it can be promoted to a Go primitive later.

### File Organization

```
lib/
  belief.scm           ;; define-belief, run-beliefs, registry, report
  belief-selectors.scm ;; site selector implementations
  belief-checkers.scm  ;; property checker implementations
  belief-utils.scm     ;; shared traversal utilities (extracted from examples)
```

This is a Scheme library under `lib/`, importable as `(wile goast belief)`. It depends on all five goast extension libraries but does not modify them.

### Shared Utilities

The traversal functions duplicated across example scripts (`nf`, `walk`, `tag?`, `filter-map`, `flat-map`, `member?`, `unique`, `ordered-pairs`) are extracted into `belief-utils.scm` and reused by both the DSL internals and the `custom` escape hatch.

## Trade-offs

| Gain | Cost |
|------|------|
| Beliefs are 3-5 lines instead of 70+ | New verbs require adding combinators to the library |
| LLM can generate beliefs from English descriptions | Combinator semantics must be precisely documented |
| Bootstrapping composition via `sites-from` | Topological sort adds complexity to the runner |
| Layer selection is implicit (verb determines layer) | Debugging layer issues requires understanding the mapping |
| Lazy loading avoids unnecessary analysis passes | Runner must track which layers each verb needs |
| `custom` escape hatch prevents DSL dead-ends | Overuse of `custom` defeats the purpose of the DSL |

## Relationship to Existing Plans

This plan implements the "Framework vs. scripts" open question from `CONSISTENCY-DEVIATION.md` §Open Design Questions (line 691). The answer: a combinator library (framework-lite) that generates the same pipeline the scripts implement manually. Each belief category from `CONSISTENCY-DEVIATION.md` §Belief Categories maps to one or more built-in verbs:

| Category | Site Selector | Property Checker |
|----------|--------------|-----------------|
| Pairing (§1) | `functions-matching` + `contains-call` | `paired-with` |
| Check (§2) | `functions-matching` + `has-params` | `checked-before-use` |
| Handling (§3) | `callers-of` | `contains-call` (for handler classification) |
| Ordering (§4) | `functions-matching` or `callers-of` | `ordered` |
| Co-mutation (§5) | `functions-matching` + `stores-to-fields` | `co-mutated` |

The bootstrapping model from `CONSISTENCY-DEVIATION.md` §Belief Bootstrapping maps to `sites-from` with `#:which` keyword. The funnel property is preserved: each `sites-from` step draws from a subset of the prior belief's sites.
