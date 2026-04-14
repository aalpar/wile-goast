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
