# E31 — the three-carrier points-to solver

The probe behind [[2026-07-14-pre-composition/E29]], [[E30]] and [[E31]], rescued from a
temp scratchpad. E29's own build order says why: the Andersen oracle "had been living in a
temp scratchpad, one cleanup away from taking E24's result with it."

Extracts the six constraint forms of `plans/2026-07-14-andersen-pointsto-design.md` from
SSA, then solves `lfp(F)` with difference propagation. **No HVN, no cycle elimination** —
the design bars both, so that a result speaks to the algebraic bet rather than to solver
engineering.

## The three arms

`main.go` is the extractor and is **shared verbatim** by all three arms — the only thing
that varies is which `diff-*.go` is compiled alongside it. That is the whole experiment:
one variable.

| file | carrier | points-to set elements |
|------|---------|------------------------|
| `diff-map.go` | `map[int]bool` | node ids | 
| `diff-bitset.go` | `intsets.Sparse` | node ids |
| `diff-dense.go` | `intsets.Sparse` | **compacted object ids** |

Arm 3 exists because arm 2 indicted itself: it measured 4.84 bits set per 256-bit block.
`main.go` draws value nodes and object nodes from one counter, so object ids are sprayed
across the whole node space and a bit vector indexed by node id is ~90% structurally empty.
Arm 3 compacts the element space — a bijection, so same lattice, same fixpoint, same
schedule.

## Build and run

Each arm is `main.go` + exactly one `diff-*.go`, as `package main`:

    mkdir arm1 && cp main.go go.mod go.sum arm1/ && cp diff-map.go arm1/diff.go
    cd arm1 && go build -o arm1 . && ./arm1 /path/to/target

`BUDGET=short` caps the solve at 60s (default 600s). `CALLEES=1` prints the resolved
dynamic callee set, which is what `validate.sh` diffs against the oracle.

**Pin `x/tools` to v0.42.0.** `go.mod`/`go.sum` here do. A stray
`go get golang.org/x/tools/container/intsets` silently bumped it to v0.48.0 during this
experiment, which changed the `go/ssa` builder and therefore the *program under analysis*
(96,965 objects instead of 76,938) — a confound that would have compared a bitset on
program A against a map on program B. `intsets` already ships inside v0.42.0; nothing needs
fetching.

## Validation is not optional

    ./validate.sh <solver-binary>

Runs the solver and the vendored `go/pointer` oracle (`goastcg/testdata/oracle/genprobe`,
built with `GOWORK=off`) over the 126 interface-dispatch corpora and diffs the resolved
callee sets. **All three arms: 126 agree, 0 differ.**

Do not skip this. E29 bug #2 **under-approximated and converged in 3.3 seconds** — a fast
wrong answer, which is precisely the failure mode a "does it converge?" experiment is most
exposed to. Convergence without oracle agreement is worthless.

## Results

`run-arm{1,2,3}-*.txt`, all on one wile snapshot (29,151 functions / 850,295 nodes /
77,001 objects / 832,233 constraints), 10-minute budget, one at a time.

| arm | total pts pairs | peak RSS | IPC | status |
|-----|-----------------|----------|-----|--------|
| 1 map | **158,992,115** | 8.61 GB | 3.31 | timed out |
| 2 bitset | 76,840,622 | 2.65 GB | 0.76 | timed out |
| 3 dense | 142,764,258 | **2.18 GB** | 1.06 | timed out |

None converges, and **the hash map is the fastest**. See `../E31-representation-refuted.txt`.
