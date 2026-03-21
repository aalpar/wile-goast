;;; belief-example.scm — Example belief definitions using the DSL
;;;
;;; Demonstrates the belief DSL against wile-goast's own codebase.
;;;
;;; Usage: ./dist/wile-goast '(begin (load "examples/goast-query/belief-example.scm"))'

(import (wile goast belief))

(define-belief "prim-functions-have-body"
  (sites (functions-matching (name-matches "Prim")))
  (expect (custom (lambda (site ctx)
    (if (nf site 'body) 'has-body 'no-body))))
  (threshold 0.90 3))

(run-beliefs "github.com/aalpar/wile-goast/goast")
