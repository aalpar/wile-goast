(define-library (wile goast ssa-normalize)
  (export
    ssa-normalize
    ssa-rule-set
    ssa-rule-identity
    ssa-rule-commutative
    ssa-rule-annihilation)
  (import (wile goast utils)
          (wile algebra rewrite))
  (include "ssa-normalize.scm"))
