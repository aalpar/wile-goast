(define-library (wile goast ssa-normalize)
  (export
    ssa-normalize
    ssa-rule-set
    ssa-rule-identity
    ssa-rule-commutative
    ssa-rule-annihilation
    ;; v2: named theory for discover-equivalences
    ssa-theory
    ssa-binop-protocol)
  (import (wile goast utils)
          (wile algebra rewrite)
          (wile algebra symbolic))
  (include "ssa-normalize.scm"))
