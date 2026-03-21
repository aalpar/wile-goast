(define-library (wile goast utils)
  (export
    nf tag? walk
    filter-map flat-map
    member? unique has-char?
    ordered-pairs)
  (include "utils.scm"))
