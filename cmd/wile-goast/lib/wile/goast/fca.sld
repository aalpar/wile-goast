(define-library (wile goast fca)
  (export
    make-context
    context-from-alist
    context-objects
    context-attributes
    field-index->context
    intent
    extent
    concept-lattice
    concept-extent
    concept-intent
    cross-boundary-concepts
    boundary-report)
  (import (wile goast utils))
  (include "fca.scm"))
