# E35 — demand-driven points-to

The probe behind [[2026-07-14-pre-composition/E35]]: does querying only dispatch sites
(demand-driven points-to) sidestep the `errorString` flood ([[E33]]) that makes batch
Andersen time out? Answer: for the median dispatch query yes (slice ~11 nodes), for the
error-typed ~15% tail no. Full record in `E35-cflreach-diagnostic.txt`.

## Files

| file | role |
|------|------|
| `extract.go` | [[E31]]'s extractor, frozen |
| `solve.go`   | [[E31]]'s batch difference-propagation solver, frozen — the equivalence oracle |
| `cfl.go`     | the demand engine: activate backward feeders + demand heap-match; == batch by the equivalence theorem |
| `slice.go`   | the resolved-graph backward slice — the OPTIMAL demand footprint, unconfounded by the engine's loose writer-finding |
| `main.go`    | driver: extract → batch → EQUIV (all nodes) + DISPATCH (receivers) + flood/slice measurement |

`extract.go`/`solve.go` frozen; the variables are `cfl.go` and `slice.go`.

## Build and run

    GOWORK=off go build -o cflreach .
    GOWORK=off ./cflreach /path/to/converged/pkg      # EQUIV + DISPATCH pass/fail
    BUDGET=short GOWORK=off ./cflreach /path/to/target # 60s batch, then slice footprint

`DUMPDIFF=1` lists receivers where demand != batch (the completeness tripwire).

## Validation

- **126 corpora** (`LLMAccuracy/code-restructuring/packages_iface_decoy`): the demand engine
  must reproduce batch dispatch resolution exactly. `run-corpora-126.txt`: EQUIV 126/126,
  DISPATCH 126/126.
- **soundfix** (converged, heap-exercising): EQUIV pass; 7/513 DISPATCH failures mark the
  runtime-heap completeness boundary (v2 RSM).
- The equivalence theorem `demand-pts == batch-pts` is the test oracle — a bug
  under-approximates and the check fires.

## Artifacts

`run-corpora-126.txt` (the gate), `run-soundfix-CONVERGED.txt` (EQUIV + boundary),
`run-wile.txt`, `run-etcd.txt`, `run-ollama.txt` (the backward-slice footprint).
