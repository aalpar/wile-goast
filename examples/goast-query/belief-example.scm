;; Copyright 2026 Aaron Alpar
;;
;; Licensed under the Apache License, Version 2.0 (the "License");
;; you may not use this file except in compliance with the License.
;; You may obtain a copy of the License at
;;
;;     http://www.apache.org/licenses/LICENSE-2.0
;;
;; Unless required by applicable law or agreed to in writing, software
;; distributed under the License is distributed on an "AS IS" BASIS,
;; WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
;; See the License for the specific language governing permissions and
;; limitations under the License.

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
