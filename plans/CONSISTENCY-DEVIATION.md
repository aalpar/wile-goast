# Consistency-Based Deviation Detection — Remaining Work

**Current state**: All five belief categories validated. Co-mutation (category 5) validated on wile/machine. Categories 1-4 validated against synthetic testdata in v0.5.x. Belief DSL implemented.

**Reference**: `plans/BELIEF-DSL.md`, `examples/goast-query/consistency-comutation.scm`

## Belief Categories (Validated)

Categories 1-5 validated against controlled packages in `examples/goast-query/testdata/`. Each belief found the planted deviation. See "Validation Results" below for details.

### 1. Pairing Beliefs
**Validated.** "Operation A always paired with operation B" (Lock/Unlock, Open/Close). DSL verb: `paired-with`.

### 2. Check Beliefs
**Validated.** "Value V checked for condition C before use." DSL verb: `checked-before-use`. Partially covered by `errcheck`/`nilness` but the cross-caller comparison is novel. Uses bounded transitive reachability on the SSA def-use graph (up to 4 hops). For `ssa-store` instructions, follows value-to-address connections to traverse struct field access patterns.

### 3. Handling Beliefs
**Validated.** "All callers of F handle the result the same way." DSL verbs: `callers-of` + `contains-call`. Bug fixed during validation: `callers-of` now returns AST func-decl nodes (was returning incompatible `(name edge)` pairs).

### 4. Ordering Beliefs
**Validated.** "Operation A always precedes operation B." DSL verb: `ordered`. Bug fixed during validation: moved from CFG blocks (which lack instructions) to SSA blocks with `find-ssa-call-blocks` and `ssa-dominates?` helpers. Same-block cases now resolved via instruction position comparison (`find-call-position`).

## Validation Results (v0.5.x)

### Synthetic Testdata

All five categories validated against controlled packages in
`examples/goast-query/testdata/`. Each belief found the planted deviation.

| Category | Checker | Sites | Majority | Deviations | Status |
|----------|---------|-------|----------|------------|--------|
| 1. Pairing | `paired-with` | 5 | paired-defer | 1 (ReadUnsafe -> unpaired) | PASS |
| 2. Check | `checked-before-use` | 5 | guarded | 1 (HandleUnsafe -> unguarded) | PASS (after fix) |
| 3. Handling | `callers-of` + `contains-call` | 5 | present | 1 (CallerBad -> absent) | PASS (after fix) |
| 4. Ordering | `ordered` | 5 | a-dominates-b | 1 (PipelineReversed -> b-dominates-a) | PASS |
| 5. Co-mutation | `co-mutated` | 5 | co-mutated | 1 (SetServer -> partial) | PASS |

### Bugs Fixed During Validation

1. **`ordered` checker** — used `go-cfg` (blocks lack instructions) and passed cfg
   to `go-cfg-dominates?` (expects dom-tree). Fixed: uses SSA blocks directly with
   `find-ssa-call-blocks` and `ssa-dominates?` helpers.

2. **`callers-of` selector** — returned `(name edge)` pairs incompatible with
   checkers expecting func-decl nodes. Fixed: looks up AST func-decl for each caller
   via `ssa-short-name` matching. Also added `cg-resolve-name` for short-to-qualified
   name resolution.

3. **`checked-before-use` checker** — looked for value directly in `ssa-if` operands,
   but `if err != nil` compiles to `BinOp(err, nil) -> If(t0)`. Fixed: bounded
   transitive reachability on the def-use graph (up to 4 hops). For `ssa-store` instructions (no output
   name), tracks operands to follow value-to-address connections. Handles struct field
   guard patterns like `if r.Valid` where the chain is:
   `r -> store(t0, r) -> field-addr(t0) -> unop(t1) -> if(t2)`.

4. **`ordered` same-block** — when both calls are in the same SSA block, returned
   `'same-block` which split the majority vote. Fixed: `find-call-position` compares
   instruction indices within the block to return `'a-dominates-b` or `'b-dominates-a`.

### Known Limitations

- `checked-before-use` only matches SSA parameter names (not local variables,
  which get register names like `t0`). Works for error parameters, not for
  `err := f()` locals.
- `ordered` uses first matching block only — if a function calls the same
  operation multiple times, only the first call's block is checked.
- `callers-of` requires fully-qualified SSA names internally; `cg-resolve-name`
  resolves short names but may be ambiguous if multiple packages define the same
  function name.

### etcd Raft

Validated against `go.etcd.io/etcd` (multi-module repo). Scripts in `examples/etcd/`.

**lock-beliefs.scm** (categories 1 & 4, from `etcd/` root):

| Belief | Category | Sites | Majority | Deviations |
|--------|----------|-------|----------|------------|
| lock-unlock-pairing | 1. Pairing | 13 | paired-call (8) | 5 use paired-defer |
| rlock-runlock-pairing | 1. Pairing | 11 | paired-defer (6) | 5 use paired-call |
| lock-before-unlock | 4. Ordering | 13 | a-dominates-b (11/13) | 2 (init, run -> missing) |

Finding: etcd genuinely mixes `defer Unlock()` and direct `Unlock()` calls. The
deviations are style inconsistencies (paired-defer vs paired-call), not missing
unlocks. After the same-block ordering fix, `lock-before-unlock` is now a strong
belief (11/13 `a-dominates-b`). The 2 `missing` deviations (`init`, `run`) are
functions where the SSA doesn't find the expected call pair.

**raft-check-beliefs.scm** (category 2, from `etcd/raft/`):

| Belief | Sites | Majority |
|--------|-------|----------|
| raft-msg-type-guard | 22 | unguarded (11/22) | 11 guarded |

After the def-use reachability fix, the checker now detects 11/22 functions as `guarded`.
The traversal follows the chain `m -> store -> field-addr -> unop -> if` to detect
struct field guards like `if m.Type == ...`. The majority is still `unguarded`
(11/22), surfacing the 11 guarded functions as deviations — but these are the
core dispatch functions (`Step`, `stepLeader`, `stepCandidate`, `stepFollower`,
etc.) that correctly check `m.Type` before routing.

**raft-error-handling.scm** (category 3, from `etcd/raft/`):

| Belief | Sites | Majority |
|--------|-------|----------|
| step-error-handling | 13 | absent (13/13) |

Callers of `Step` uniformly propagate errors up the stack without wrapping or
logging. Consistent behavior, just not the pattern hypothesized.

**raft-storage-consistency.scm** (interface methods, from `etcd/raft/`):

| Belief | Sites | Majority | Deviations |
|--------|-------|----------|------------|
| entries-compaction-guard | 1 | weak (below threshold) | — |
| snapshot-temp-unavail | 2 | absent (2/2) | none |
| term-bounds-check | 1 | weak (below threshold) | — |
| storage-resource-cleanup | 34 | absent (24) | 10 (MemoryStorage methods use Unlock) |

Finding: `MemoryStorage` methods use locks (10 deviations calling `Unlock`)
while other Storage implementors don't. Genuine behavioral divergence —
MemoryStorage is thread-safe, others delegate locking to their callers.

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
