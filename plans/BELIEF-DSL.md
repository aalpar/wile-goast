# Belief DSL — Remaining Work

**Current state**: Implemented as `(wile goast belief)`. Core form (`define-belief`, `run-beliefs`), all site selectors, all property checkers, multi-package support, `go-ssa-field-index` performance optimization — all working.

**Reference**: `cmd/wile-goast/lib/wile/goast/belief.sld`, `cmd/wile-goast/lib/wile/goast/belief.scm`

## Belief Graduation — Discovery `--emit` Mode (unimplemented)

Discovery scripts should gain an `--emit` mode that outputs `define-belief` forms instead of human-readable reports. This closes the discover → review → commit lifecycle:

```
discover → review → commit → enforce
   │                  │         │
   │  human judgment  │  CI     │
   ▼                  ▼         ▼
 candidates       belief file  run-beliefs
 (stdout)         (.scm)       (exit code)
```

Currently discovery scripts output reports. The `--emit` mode would output candidate Scheme code:

```scheme
;; Debugger stepping fields: stepMode + stepFrame
;; Adherence: 75% (3/4 sites), Deviations: StepOver
;;
(define-belief "Debugger:stepMode+stepFrame"
  (sites (functions-matching
           (stores-to-fields "Debugger" "stepMode" "stepFrame")))
  (expect (co-mutated "stepMode" "stepFrame"))
  (threshold 0.66 3))
```

## Suppression (unimplemented)

Future discovery runs should diff output against committed belief files. A belief whose `(sites ...)` and `(expect ...)` match an existing `define-belief` is suppressed — discovery only reports *new* findings. The diff is structural, not textual: same selector type, same target fields/functions, same checker. Names and thresholds don't matter for matching.

## Limitations Worth Addressing

- **No severity ranking** — a deviation in a critical path matters more than in a debug utility. The tool reports deviations uniformly.
- **AST-level call detection** — `contains-call` and `paired-with` miss indirect calls (method values, interfaces, closures). Acceptable for convention detection but limits coverage.
- **The majority assumption** — if the majority behavior is wrong, deviations are the correct code. The tool detects inconsistency, not incorrectness.
