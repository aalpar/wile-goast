# Consistency-Based Deviation Detection — Remaining Work

**Current state**: All five belief categories validated. Co-mutation (category 5) validated on wile/machine. Categories 1-4 validated against synthetic testdata in v0.5.x. Belief DSL implemented.

**Reference**: `plans/BELIEF-DSL.md`, `examples/goast-query/consistency-comutation.scm`

## Belief Categories (Validated)

Categories 1-4 validated against controlled packages in `examples/goast-query/testdata/`. Each belief found the planted deviation. See "Validation Results" below for details.

### 1. Pairing Beliefs
**Validated.** "Operation A always paired with operation B" (Lock/Unlock, Open/Close). DSL verb: `paired-with`.

### 2. Check Beliefs
**Validated.** "Value V checked for condition C before use." DSL verb: `checked-before-use`. Partially covered by `errcheck`/`nilness` but the cross-caller comparison is novel. Bug fixed during validation: checker now follows one level of data flow (value -> comparison -> ssa-if).

### 3. Handling Beliefs
**Validated.** "All callers of F handle the result the same way." DSL verbs: `callers-of` + `contains-call`. Bug fixed during validation: `callers-of` now returns AST func-decl nodes (was returning incompatible `(name edge)` pairs).

### 4. Ordering Beliefs
**Validated.** "Operation A always precedes operation B." DSL verb: `ordered`. Bug fixed during validation: moved from CFG blocks (which lack instructions) to SSA blocks with `find-ssa-call-blocks` and `ssa-dominates?` helpers.

## Validation Results (v0.5.x)

### Synthetic Testdata

All four categories validated against controlled packages in
`examples/goast-query/testdata/`. Each belief found the planted deviation.

| Category | Checker | Sites | Majority | Deviations | Status |
|----------|---------|-------|----------|------------|--------|
| 1. Pairing | `paired-with` | 5 | paired-defer | 1 (ReadUnsafe -> unpaired) | PASS |
| 2. Check | `checked-before-use` | 5 | guarded | 1 (HandleUnsafe -> unguarded) | PASS (after fix) |
| 3. Handling | `callers-of` + `contains-call` | 5 | present | 1 (CallerBad -> absent) | PASS (after fix) |
| 4. Ordering | `ordered` | 5 | a-dominates-b | 1 (PipelineReversed -> same-block) | PASS (after fix) |

### Bugs Fixed During Validation

1. **`ordered` checker** — used `go-cfg` (blocks lack instructions) and passed cfg
   to `go-cfg-dominates?` (expects dom-tree). Fixed: uses SSA blocks directly with
   `find-ssa-call-blocks` and `ssa-dominates?` helpers.

2. **`callers-of` selector** — returned `(name edge)` pairs incompatible with
   checkers expecting func-decl nodes. Fixed: looks up AST func-decl for each caller
   via `ssa-short-name` matching. Also added `cg-resolve-name` for short-to-qualified
   name resolution.

3. **`checked-before-use` checker** — looked for value directly in `ssa-if` operands,
   but `if err != nil` compiles to `BinOp(err, nil) -> If(t0)`. Fixed: follows one
   level of data flow (value -> comparison -> ssa-if).

### Known Limitations

- `checked-before-use` only matches SSA parameter names (not local variables,
  which get register names like `t0`). Works for error parameters, not for
  `err := f()` locals.
- `ordered` uses first matching block only — if a function calls the same
  operation multiple times, only the first call's block is checked.
- `callers-of` requires fully-qualified SSA names internally; `cg-resolve-name`
  resolves short names but may be ambiguous if multiple packages define the same
  function name.

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
