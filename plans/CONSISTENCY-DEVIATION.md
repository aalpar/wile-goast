# Consistency-Based Deviation Detection — Remaining Work

**Current state**: Co-mutation (category 5) validated on wile/machine. Belief DSL implemented. Categories 1-4 designed but unvalidated against real codebases.

**Reference**: `plans/BELIEF-DSL.md`, `examples/goast-query/consistency-comutation.scm`

## Unvalidated Belief Categories

Categories 1-4 have DSL verbs but no empirical validation. The belief DSL implements `paired-with`, `checked-before-use`, `ordered`, and `callers-of`+`contains-call` for these, but they haven't been exercised against real codebases.

### 1. Pairing Beliefs
"Operation A always paired with operation B" (Lock/Unlock, Open/Close). DSL verb: `paired-with`. Needs validation on a codebase with known pairing conventions.

### 2. Check Beliefs
"Value V checked for condition C before use." DSL verb: `checked-before-use`. Needs SSA+CFG validation. Partially covered by `errcheck`/`nilness` but the cross-caller comparison is novel.

### 3. Handling Beliefs
"All callers of F handle the result the same way." DSL verbs: `callers-of` + `contains-call`. The error-handling classifier (`wrap-return`, `raw-return`, `log-continue`, `ignore`) is designed but untested.

### 4. Ordering Beliefs
"Operation A always precedes operation B." DSL verb: `ordered`. Fixed to pass package pattern to `go-cfg`. Needs validation. The bootstrapping chain (co-mutation → ordering) predicted signal on kubelet VMCounters but hasn't been tested.

## Proposed Validation Targets

- **crdt** — method protocol consistency (`Merge`, `Value`, `MarshalJSON`) and field access protocols across CRDT types.
- **kubelet** — VMCounters ordering chain (10 `inline*` helpers, enough sites for ordering signal).

## Open Design Questions

1. **Belief discovery** — currently requires analyst to choose starting categories. `sites-from` bootstrapping partially addresses this. Full automatic discovery (mining arbitrary patterns) overlaps with specification mining (Ammons, Bodik, Larus 2002).
2. **Incremental analysis** — running on every commit requires incremental belief updates. Current architecture loads full module from scratch.
3. **Threshold sensitivity** — the 66%/3 thresholds were chosen empirically. No sensitivity analysis at different levels.
4. **Minimum corpus size** — at what scale does the approach produce meaningful signal? ~60 SSA functions worked for co-mutation; guidance for other categories unknown.

## Known False Positive Patterns

1. **Focused setter functions** — `SetPC`, `SetThread`, `SetMark` store a single field from a multi-field struct. Intentional, not a co-mutation violation. Possible mitigation: exclude functions storing only 1 field. Not yet implemented.
2. **Field name collision** — mitigated by receiver-type disambiguation but fails when two structs have identical field sets. The `go-ssa-field-index` primitive now provides struct type from Go's type system, largely eliminating this.
