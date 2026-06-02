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

;;; dup-detect.scm — deduplication FCA audit trace.
;;;
;;; The mirror of fca.scm's boundary-findings on a function×external-ref concept
;;; lattice. Functions sharing a maximal informative reference set (an FCA
;;; concept with extent >= 2) are duplicate candidates; each extent member is a
;;; located finding whose why is the shared ref intent. Composes the split.scm
;;; clustering chain (objects are function names) with fca + provenance.

;; func-refs->positions: name->position hashtable from go-func-refs output (now
;; carrying 'pos). The field-index->positions twin; keys are func-ref names,
;; identical to the FCA context objects, so the join is exact-match. Functions
;; without a 'pos (synthetic/positionless) are skipped — unlocated when looked up.
(define (func-refs->positions func-refs)
  (let ((h (make-hashtable)))
    (for-each
      (lambda (fr)
        (let ((name (nf fr 'name))
              (pos  (nf fr 'pos)))
          (if (and (string? name) (string? pos))
            (hashtable-set! h name pos))))
      (if (pair? func-refs) func-refs '()))
    h))
