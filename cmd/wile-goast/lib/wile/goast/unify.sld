(define-library (wile goast unify)
  (export
    tree-diff ast-diff ssa-diff
    classify-ast-diff classify-ssa-diff
    diff-result-similarity diff-result-diffs diff-result-shared diff-result-diff-count
    score-diffs find-root-substitutions collapse-diffs
    unifiable?
    ;; v2: algebraic equivalence
    ssa-equivalent?)
  (import (wile goast utils)
          (wile algebra symbolic)
          (wile goast ssa-normalize))
  (include "unify.scm"))
