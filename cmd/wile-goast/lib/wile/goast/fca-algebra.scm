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

;;; fca-algebra.scm — Concept lattice as an algebraic lattice
;;;
;;; Bridges FCA concept lattices with (wile algebra lattice) so that
;;; lattice-theoretic operations (join, meet, leq) and algebraic
;;; annotations (subconcept, superconcept, incomparable) are available
;;; for boundary reports.

;;; ── Concept lattice → algebra lattice ───────────────────

;; Find the concept in the lattice matching the given intent.
(define (find-concept-by-intent lattice int)
  (let loop ((cs lattice))
    (cond ((null? cs) #f)
          ((equal? (concept-intent (car cs)) int) (car cs))
          (else (loop (cdr cs))))))

;; Construct a (wile algebra lattice) from an FCA concept lattice.
;; ctx: the FCA context (needed for Galois connection operations)
;; concepts: the list of concepts from (concept-lattice ctx)
;;
;; Lattice ordering: C1 <= C2 iff E1 ⊆ E2 (equiv. I2 ⊆ I1)
;; Join: concept whose intent = closure(I1 ∩ I2)
;; Meet: concept whose intent = closure(I1 ∪ I2)
(define (concept-lattice->algebra-lattice ctx concepts)
  "Construct a (wile algebra lattice) from an FCA concept lattice.\nCTX is the FCA context, CONCEPTS is the list from (concept-lattice ctx).\nThe resulting lattice has join/meet via the Galois closure operator.\n\nParameters:\n  ctx : list\n  concepts : list\nReturns: any\nCategory: goast-fca\n\nSee also: `concept-relationship', `annotated-boundary-report'."
  (if (null? concepts)
    (error "concept-lattice->algebra-lattice: concepts list is empty"))
  (let* ((all-attrs (context-attributes ctx))
         ;; Closure operator: cl(A) = intent(extent(A))
         (cl (make-closure-operator
               (lambda (attrs) (intent ctx (extent ctx attrs)))
               (powerset-lattice all-attrs)))
         ;; Top: concept with cl('()) as intent (shared by all objects)
         (top-intent (closure-close cl '()))
         (top-concept (find-concept-by-intent concepts top-intent))
         ;; Bottom: concept with cl(all-attrs) as intent
         (bottom-intent (closure-close cl all-attrs))
         (bottom-concept (find-concept-by-intent concepts bottom-intent)))
    (make-lattice
      ;; join: least upper bound — intent = cl(I1 ∩ I2)
      (lambda (c1 c2)
        (let ((int (closure-close cl (set-intersect (concept-intent c1) (concept-intent c2)))))
          (or (find-concept-by-intent concepts int)
              (cons (extent ctx int) int))))
      ;; meet: greatest lower bound — intent = cl(I1 ∪ I2)
      (lambda (c1 c2)
        (let ((int (closure-close cl (set-union (concept-intent c1) (concept-intent c2)))))
          (or (find-concept-by-intent concepts int)
              (cons (extent ctx int) int))))
      ;; bottom
      bottom-concept
      ;; top
      top-concept
      ;; leq: C1 <= C2 iff I2 ⊆ I1 (more attributes = lower in lattice)
      (lambda (c1 c2)
        (set-subset? (concept-intent c2) (concept-intent c1))))))

;;; ── Concept relationship ────────────────────────────────

;; Determine the relationship between two concepts.
;; Returns one of: 'subconcept, 'superconcept, 'equal, 'incomparable
(define (concept-relationship c1 c2)
  "Classify the lattice relationship between two concepts.\nReturns: subconcept (C1 <= C2), superconcept (C1 >= C2), equal, or incomparable.\n\nParameters:\n  c1 : list\n  c2 : list\nReturns: symbol\nCategory: goast-fca\n\nSee also: `concept-lattice->algebra-lattice', `annotated-boundary-report'."
  (let ((i1 (concept-intent c1))
        (i2 (concept-intent c2)))
    (let ((i2-sub-i1 (set-subset? i2 i1))
          (i1-sub-i2 (set-subset? i1 i2)))
      (cond ((and i1-sub-i2 i2-sub-i1) 'equal)
            (i2-sub-i1 'subconcept)    ;; I2 ⊆ I1 means E1 ⊆ E2, C1 <= C2
            (i1-sub-i2 'superconcept)  ;; I1 ⊆ I2 means E2 ⊆ E1, C2 <= C1
            (else 'incomparable)))))

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
