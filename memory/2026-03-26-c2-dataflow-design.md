# C2 — Dataflow Analysis Framework

Extends `(wile goast dataflow)` with a general-purpose worklist-based dataflow
analysis framework over SSA block graphs.

**Status:** Design approved. Not yet implemented.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Block representation | SSA only | CFG blocks lack instructions; AST↔SSA correlation deferred |
| Transfer granularity | Per-block | `(lambda (block state) → state)` — user controls iteration |
| API shape | Eager `run-analysis` | No intermediate analysis object; one call, clear inputs/outputs |
| `defuse-reachable?` | Unchanged | Flat def-use chain walk; structurally different from CFG-aware dataflow |
| Initial state | Explicit parameter | Enables specialization and context-sensitive graduation (C6) |
| Monotonicity | Trailing `'check-monotone` flag | Opt-in debug check after each block transfer |
| Result format | Plain alist `((idx in out) ...)` | No new record types |

## API

Five new exports. Existing exports unchanged.

```scheme
(run-analysis direction lattice transfer ssa-fn)
(run-analysis direction lattice transfer ssa-fn initial-state)
(run-analysis direction lattice transfer ssa-fn initial-state 'check-monotone)
;; direction: 'forward or 'backward
;; lattice: from (wile algebra)
;; transfer: (lambda (ssa-block state) → state)
;; ssa-fn: SSA function from go-ssa-build
;; initial-state: entry in-state (forward) or exit out-state (backward); default lattice-bottom
;; Returns: analysis-result (alist)

(analysis-in result block-idx)    ;; → state entering block
(analysis-out result block-idx)   ;; → state leaving block
(analysis-states result)          ;; → ((idx in out) ...) — the alist itself

(block-instrs ssa-block)          ;; → list of SSA instructions
```

## Worklist Algorithm

### Forward

```
1. Compute reverse postorder (RPO) via DFS on succs from entry
2. Initialize: every block gets in=⊥, out=⊥
3. Set entry block in-state to initial-state (default: lattice-bottom)
4. Seed worklist with entry block
5. While worklist non-empty:
   a. Pick block with lowest RPO number
   b. in-state = join of all predecessors' out-states
      (entry block: use initial-state instead)
   c. out-state = transfer(block, in-state)
   d. If 'check-monotone: verify old-out ⊑ new-out via lattice-leq?
      Raise error with block index if violated
   e. If out-state changed (not lattice-leq? in both directions):
      add all successors to worklist
   f. Store in-state and out-state
6. Return alist of (block-idx in-state out-state)
```

### Backward

Same worklist, edges and ordering reversed:

| Aspect | Forward | Backward |
|--------|---------|----------|
| Start from | entry (block 0) | exit blocks (succs = empty) |
| Propagate to | successors | predecessors |
| Join inputs from | predecessors' out-states | successors' in-states |
| Block ordering | reverse postorder on succs | postorder on succs |
| Initial state applies to | entry block's in | exit blocks' out |

Multiple exit blocks all receive the initial state.

Transfer function signature is identical in both directions: `(block, state) → state`.
"State" means "state flowing in the analysis direction" — forward: in→out,
backward: out→in.

## Block Ordering

Reverse postorder via DFS from entry:

1. DFS on successor edges from block 0
2. Append block index on backtrack (postorder)
3. Reverse → reverse postorder
4. Build index: block-idx → RPO number (for worklist priority)

~15 lines. Computed once per `run-analysis` call.

## Worklist Data Structure

Sorted list by RPO number. No priority queue — SSA functions in real Go code
rarely exceed ~50 blocks. Re-sort on insertion. Swap for heap if profiling
shows it matters.

## Monotonicity Assertion

When `'check-monotone` is passed:

After step 5c, check `(lattice-leq? lattice old-out new-out)`. If false,
raise an error: `"monotonicity violation at block <idx>"`.

Catches buggy transfer functions — especially useful when writing C3's
hand-rolled abstract domains.

## File Layout

All new code goes in existing files:

- `cmd/wile-goast/lib/wile/goast/dataflow.scm` — implementation (~80-100 new lines)
- `cmd/wile-goast/lib/wile/goast/dataflow.sld` — add 5 exports

No new files. Existing exports and implementation unchanged.

## Updated Exports

```scheme
(define-library (wile goast dataflow)
  (export
    ;; Existing — unchanged
    boolean-lattice
    ssa-all-instrs
    ssa-instruction-names
    make-reachability-transfer
    defuse-reachable?
    ;; New — C2
    run-analysis
    analysis-in
    analysis-out
    analysis-states
    block-instrs)
  (import (wile algebra)
          (wile goast utils))
  (include "dataflow.scm"))
```

## Dependencies

- `(wile algebra)` — `lattice-join`, `lattice-bottom`, `lattice-leq?` (all exist)
- `(wile goast utils)` — `nf`, `tag?` (already imported)
- SSA block structure from `go-ssa-build` — `index`, `preds`, `succs`, `instrs` fields

No new dependencies.

## Relationship to Other Tracks

- **C3 (abstract domains):** First consumer. Constant propagation, liveness, reaching
  definitions, sign, interval — all use `run-analysis` with domain-specific lattices
  and transfer functions.
- **C4 (path algebra):** Independent. Uses semirings on call graphs, not CFG dataflow.
- **C5 (Galois connections):** Validates C3 transfer functions. No direct C2 dependency.
- **C6 (belief graduation):** Compiles 100% beliefs into `run-analysis` calls with
  appropriate lattices. May use initial-state parameter for context sensitivity.
- **`defuse-reachable?`:** Stays as-is. Different iteration strategy (flat def-use
  chains vs block-aware worklist). Both coexist in `(wile goast dataflow)`.
