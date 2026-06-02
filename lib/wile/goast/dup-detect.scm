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

;; function-ref-context: function×external-ref FCA context, IDF-filtered. Reuses
;; the split.scm chain verbatim — the same machinery split applies at package
;; granularity, here for dedup clustering. Objects = function names; attributes =
;; informative external package paths. THRESHOLD defaults to 0.36 (split's).
(define (function-ref-context func-refs . opts)
  (let* ((threshold (if (pair? opts) (car opts) 0.36))
         (sigs      (import-signatures func-refs))
         (idf       (compute-idf sigs))
         (filtered  (filter-noise sigs idf threshold)))
    (build-package-context filtered)))

;; duplicate-candidate-concepts: concepts whose extent has >= MIN-EXTENT (default
;; 2) functions sharing a non-empty intent. By FCA closure, such a concept is a
;; duplicate-candidate cluster: every function in the extent uses every ref in
;; the intent, and the intent is the maximal shared informative ref-set.
(define (duplicate-candidate-concepts lattice . opts)
  (let ((min-ext (if (pair? opts) (car opts) 2)))
    (filter (lambda (c)
              (and (>= (length (concept-extent c)) min-ext)
                   (>= (length (concept-intent c)) 1)))
            lattice)))

;; dup-candidate-findings: the boundary-findings twin for deduplication. POS-INDEX
;; is from func-refs->positions. Each entry mirrors a boundary-findings entry:
;; per candidate concept, each extent member -> a located finding. value = the
;; function name; where = its source position (or #f when unlocated); why = the
;; shared ref intent as a structured reason (duplicate-candidate (refs . intent))
;; so render-why projects it and a script can filter on the shared packages;
;; score = #f (no structural-confidence measure yet — that is slice 5b).
(define (dup-candidate-findings concepts pos-index)
  (map (lambda (concept)
         (let* ((ext (concept-extent concept))
                (int (concept-intent concept))
                (why (cons 'duplicate-candidate (list (cons 'refs int))))
                (findings
                  (map (lambda (fn)
                         (make-finding fn (hashtable-ref pos-index fn #f) why #f))
                       ext)))
           (list (cons 'refs int)
                 (cons 'findings findings)
                 (cons 'extent-size (length ext)))))
       concepts))

;; find-duplicate-candidates: top-level. TARGET is a package pattern string or a
;; GoSession. Runs the full chain — func-refs -> IDF-filtered context -> concept
;; lattice -> candidate concepts -> located findings. Returns a list of entries
;; (one per candidate cluster), each ((refs . intent) (findings . (...))
;; (extent-size . N)). Optional THRESHOLD (default 0.36) tunes IDF noise removal.
(define (find-duplicate-candidates target . opts)
  (let* ((threshold (if (pair? opts) (car opts) 0.36))
         (refs      (go-func-refs target))
         (ctx       (function-ref-context refs threshold))
         (lat       (concept-lattice ctx))
         (cands     (duplicate-candidate-concepts lat))
         (pos-index (func-refs->positions refs)))
    (dup-candidate-findings cands pos-index)))
