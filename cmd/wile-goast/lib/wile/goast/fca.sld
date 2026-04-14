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

(define-library (wile goast fca)
  (export
    ;; Re-exported from (wile algebra fca)
    make-context
    context-from-alist
    fca-context?
    context-objects
    context-attributes
    intent
    extent
    concept-lattice
    concept-extent
    concept-intent
    concept-lattice->algebra-lattice
    concept-relationship
    set-intersect
    set-member?
    set-add
    set-before
    set-union
    set-subset?
    sort-strings
    ;; Defined locally (Go SSA/callgraph bridge)
    field-index->context
    cross-boundary-concepts
    boundary-report
    propagate-field-writes)
  (import (wile goast utils)
          (wile algebra fca))
  (include "fca.scm"))
