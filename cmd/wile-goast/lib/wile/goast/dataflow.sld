(define-library (wile goast dataflow)
  (export
    boolean-lattice
    ssa-all-instrs
    ssa-instruction-names
    make-reachability-transfer
    defuse-reachable?)
  (import (wile algebra)
          (wile goast utils))
  (include "dataflow.scm"))
