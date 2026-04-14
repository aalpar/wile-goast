(define-library (wile goast fca-recommend)
  (export
    ;; Pareto dominance
    dominates?
    pareto-frontier

    ;; Lattice analysis
    concept-signature
    incomparable-pairs

    ;; Candidate detection
    split-candidates
    merge-candidates
    extract-candidates

    ;; Top-level
    boundary-recommendations

    ;; Utilities
    string-suffix?)
  (import (wile goast utils)
          (wile goast fca)
          (wile goast dataflow))
  (include "fca-recommend.scm"))
