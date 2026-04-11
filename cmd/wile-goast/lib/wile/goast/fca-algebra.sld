(define-library (wile goast fca-algebra)
  (export
    concept-lattice->algebra-lattice
    annotated-boundary-report
    concept-relationship)
  (import (wile goast utils)
          (wile goast fca)
          (wile algebra lattice))
  (include "fca-algebra.scm"))
