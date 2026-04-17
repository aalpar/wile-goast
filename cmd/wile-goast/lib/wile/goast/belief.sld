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

(define-library (wile goast belief)
  (export
    ;; Core
    define-belief run-beliefs reset-beliefs! *beliefs* emit-beliefs
    define-aggregate-belief register-aggregate-belief! *aggregate-beliefs*
    aggregate-beliefs string-contains
    ;; Context (needed by custom lambdas)
    make-context ctx-pkgs ctx-ssa ctx-callgraph ctx-find-ssa-func ctx-field-index
    ctx-session
    ;; Site selectors
    functions-matching callers-of methods-of all-functions-in sites-from
    all-func-decls implementors-of interface-methods
    ;; Predicates
    has-params has-receiver name-matches
    contains-call stores-to-fields
    ;; Predicate combinators
    all-of any-of none-of
    ;; Property checkers
    paired-with ordered co-mutated
    checked-before-use custom aggregate-custom
    ;; Utils re-export (for custom lambdas)
    nf tag? walk filter-map flat-map member? unique)
  (import (wile goast utils)
          (wile goast dataflow))
  (include "belief.scm"))
