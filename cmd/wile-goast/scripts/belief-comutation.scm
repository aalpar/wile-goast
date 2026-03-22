;;; belief-comutation.scm — Co-mutation beliefs using the DSL
;;;
;;; Re-expresses consistency-comutation.scm as define-belief forms.
;;; Validates against known results from CONSISTENCY-DEVIATION.md.
;;;
;;; Usage: ./dist/wile-goast '(include "examples/goast-query/belief-comutation.scm")'

(import (wile goast belief))

;; ── Debugger stepping fields — known co-mutation pattern ──
;;
;; From CONSISTENCY-DEVIATION.md §Validation Results:
;;
;;   Continue  stores {stepMode, stepFrame, stepFrameDepth}
;;   StepInto  stores {stepMode, stepFrame, stepFrameDepth}
;;   StepOver  stores {stepMode, stepFrameDepth}         — missing stepFrame
;;   StepOut   stores {stepMode, stepFrame}              — missing stepFrameDepth
;;
;; stepping-mode-frame: 3/4 co-mutate (75%), StepOver deviates.
;; stepping-mode-depth: 3/4 co-mutate (75%), StepOut deviates.

(define-belief "stepping-mode-frame"
  (sites (functions-matching
           (stores-to-fields "Debugger" "stepMode" "stepFrame")))
  (expect (co-mutated "stepMode" "stepFrame"))
  (threshold 0.66 3))

(define-belief "stepping-mode-depth"
  (sites (functions-matching
           (stores-to-fields "Debugger" "stepMode" "stepFrameDepth")))
  (expect (co-mutated "stepMode" "stepFrameDepth"))
  (threshold 0.66 3))

(run-beliefs "github.com/aalpar/wile/machine")
