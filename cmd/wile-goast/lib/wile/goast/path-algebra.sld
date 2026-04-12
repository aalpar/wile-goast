(define-library (wile goast path-algebra)
  (export
    make-path-analysis
    path-analysis?
    path-query
    path-query-all)
  (import (wile algebra semiring)
          (wile goast utils))
  (include "path-algebra.scm"))
