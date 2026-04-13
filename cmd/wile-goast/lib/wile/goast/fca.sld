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
    boundary-report
    propagate-field-writes
    ;; Sorted string set operations (used by fca-algebra, fca-recommend)
    set-intersect
    set-member?
    set-add
    set-before
    set-union
    set-subset?)
  (import (wile goast utils))
  (include "fca.scm"))
