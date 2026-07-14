# E34 — offline HVN diagnostic

The probe behind [[2026-07-14-pre-composition/E34]]: does offline HVN collapse the
`errors.errorString` god-set that floods the Andersen solve ([[E33]])? Answer: no, ~2×,
not to a handful. Full record in `E34-hvn-diagnostic.txt`.

## Files

| file | role |
|------|------|
| `extract.go` | [[E31]]'s extractor (`main-v2-typefilter.go`) with `func main()` stripped → `extract(dir)` |
| `solve.go`   | [[E31]]'s difference-propagation solver, modified only to RETURN `pts` + the god object |
| `hvn.go`     | the offline HVN: two-pass (Tarjan SCC, then topological PE labeling); a port of `go/pointer/hvn.go` |
| `main.go`    | driver: extract → partial solve → HVN → god-set cross-tab → soundness check |

`extract.go` and `solve.go` are frozen copies; the one variable is `hvn.go`.

## Build and run

    GOWORK=off go build -o hvn .
    BUDGET=short GOWORK=off ./hvn /path/to/target      # 60s solve budget

`BUDGET`: `short` 60s, default 120s, `long` 600s. The solve TIMES OUT on real targets
(expected; a partial pts still identifies the flood). `DUMPNP=1` lists nodes HVN calls
non-pointer that the solver floods (the soundness tripwire). `SOUND=1` forces the exact
per-class pts check even on a timed-out solve (expensive).

## Soundness gate

No `go/pointer` oracle runs on real Go ([[E32]]), so validation checks HVN's defining
property instead: same PE label ⟺ identical pts. On a CONVERGED target every PE class must
be pts-uniform. `run-soundfix-CONVERGED.txt` is that gate: 14,952 classes, 0 over-merges.
Run it on any small module that converges (a `main` package pulls in the whole runtime,
which is a comprehensive exercise).

## Artifacts

`run-wile.txt`, `run-etcd.txt`, `run-ollama.txt` (60s budget, the measured targets);
`run-soundfix-CONVERGED.txt` (the validation).
