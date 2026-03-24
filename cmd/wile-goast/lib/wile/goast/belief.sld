(define-library (wile goast belief)
  (export
    ;; Core
    define-belief run-beliefs
    ;; Context (needed by custom lambdas)
    make-context ctx-pkgs ctx-ssa ctx-callgraph ctx-find-ssa-func ctx-field-index
    ;; Site selectors
    functions-matching callers-of methods-of sites-from all-func-decls
    implementors-of interface-methods
    ;; Predicates
    has-params has-receiver name-matches
    contains-call stores-to-fields
    ;; Predicate combinators
    all-of any-of none-of
    ;; Property checkers
    paired-with ordered co-mutated
    checked-before-use custom
    ;; Utils re-export (for custom lambdas)
    nf tag? walk filter-map flat-map member? unique)
  (import (wile goast utils))
  (include "belief.scm"))
