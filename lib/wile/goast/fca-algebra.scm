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

;;; fca-algebra.scm — Annotated boundary reports
;;;
;;; Go-specific annotation layer over FCA concept lattices.
;;; Core lattice construction and concept relationships are provided
;;; by (wile algebra fca). This module adds Go-specific formatting:
;;; dot-notation type extraction from attribute strings.

;;; ── Annotated boundary report ───────────────────────────

;; Summarize a concept for annotation (short description from intent types).
(define (concept-summary concept)
  (let ((types (unique (filter-map
                         (lambda (attr)
                           (let ((dot (let loop ((i 0))
                                        (cond ((>= i (string-length attr)) #f)
                                              ((char=? (string-ref attr i) #\.) i)
                                              (else (loop (+ i 1)))))))
                             (and dot (substring attr 0 dot))))
                         (concept-intent concept)))))
    (if (null? types) "(top)"
      (let loop ((ts types) (acc ""))
        (if (null? ts) acc
          (loop (cdr ts)
                (if (string=? acc "")
                  (car ts)
                  (string-append acc "+" (car ts)))))))))

;; Produce an annotated boundary report.
;; Extends boundary-report with lattice relationship annotations
;; between each cross-boundary concept and all other concepts.
(define (annotated-boundary-report cross-concepts all-concepts)
  "Annotate cross-boundary concepts with lattice relationships.\nEach entry includes subconcept-of, superconcept-of, and incomparable-with\nlists describing how the concept relates to all other concepts.\n\nParameters:\n  cross-concepts : list\n  all-concepts : list\nReturns: list\nCategory: goast-fca\n\nSee also: `concept-relationship', `concept-lattice->algebra-lattice'."
  (map (lambda (xb-concept)
         (let* ((int (concept-intent xb-concept))
                ;; Classify relationship to every other concept
                (relations
                  (filter-map
                    (lambda (other)
                      (if (equal? (concept-intent other) int) #f
                        (let ((rel (concept-relationship xb-concept other)))
                          (list (cons 'relationship rel)
                                (cons 'concept-summary (concept-summary other))
                                (cons 'extent-size (length (concept-extent other)))))))
                    all-concepts))
                ;; Group by relationship type
                (subconcepts
                  (filter-map
                    (lambda (r)
                      (and (eq? (cdr (assoc 'relationship r)) 'subconcept) r))
                    relations))
                (superconcepts
                  (filter-map
                    (lambda (r)
                      (and (eq? (cdr (assoc 'relationship r)) 'superconcept) r))
                    relations))
                (incomparables
                  (filter-map
                    (lambda (r)
                      (and (eq? (cdr (assoc 'relationship r)) 'incomparable) r))
                    relations)))
           (list (cons 'extent (concept-extent xb-concept))
                 (cons 'intent int)
                 (cons 'extent-size (length (concept-extent xb-concept)))
                 (cons 'summary (concept-summary xb-concept))
                 (cons 'subconcept-of
                   (map (lambda (r) (cdr (assoc 'concept-summary r))) subconcepts))
                 (cons 'superconcept-of
                   (map (lambda (r) (cdr (assoc 'concept-summary r))) superconcepts))
                 (cons 'incomparable-with
                   (map (lambda (r) (cdr (assoc 'concept-summary r))) incomparables)))))
       cross-concepts))
