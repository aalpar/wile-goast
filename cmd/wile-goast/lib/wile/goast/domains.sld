(define-library (wile goast domains)
  (export
    go-concrete-eval
    make-reaching-definitions
    make-liveness
    make-constant-propagation
    sign-lattice
    make-sign-analysis
    interval-lattice
    make-interval-analysis)
  (import (wile algebra)
          (wile goast dataflow)
          (wile goast utils))
  (include "domains.scm"))
