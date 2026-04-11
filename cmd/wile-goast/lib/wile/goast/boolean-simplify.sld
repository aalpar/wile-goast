(define-library (wile goast boolean-simplify)
  (export
    ;; Core normalization
    boolean-normalize
    boolean-equivalent?
    ;; Belief selector projection
    selector->symbolic
    ;; Go AST condition projection
    ast-condition->symbolic)
  (import (wile goast utils)
          (wile algebra boolean)
          (wile algebra lattice)
          (wile algebra symbolic)
          (wile algebra rewrite))
  (include "boolean-simplify.scm"))
