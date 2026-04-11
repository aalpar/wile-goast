;;; fca-algebra.scm — Concept lattice as an algebraic lattice
;;;
;;; Bridges FCA concept lattices with (wile algebra lattice) so that
;;; lattice-theoretic operations (join, meet, leq) and algebraic
;;; annotations (subconcept, superconcept, incomparable) are available
;;; for boundary reports.

;;; ── Local helpers ───────────────────────────────────────

(define (keep pred lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (loop (cdr xs)
            (if (pred (car xs)) (cons (car xs) acc) acc)))))

;;; ── Sorted string set helpers (local) ───────────────────

(define (sset-subset? a b)
  ;; #t if every element of sorted list a is in sorted list b.
  (cond ((null? a) #t)
        ((null? b) #f)
        ((string<? (car a) (car b)) #f)
        ((string=? (car a) (car b)) (sset-subset? (cdr a) (cdr b)))
        (else (sset-subset? a (cdr b)))))

(define (sset-intersect a b)
  (cond ((null? a) '())
        ((null? b) '())
        ((string<? (car a) (car b)) (sset-intersect (cdr a) b))
        ((string<? (car b) (car a)) (sset-intersect a (cdr b)))
        (else (cons (car a) (sset-intersect (cdr a) (cdr b))))))

(define (sset-union a b)
  (cond ((null? a) b)
        ((null? b) a)
        ((string<? (car a) (car b))
         (cons (car a) (sset-union (cdr a) b)))
        ((string<? (car b) (car a))
         (cons (car b) (sset-union a (cdr b))))
        (else (cons (car a) (sset-union (cdr a) (cdr b))))))

;;; ── Concept lattice → algebra lattice ───────────────────

;; Find the concept in the lattice matching the given intent,
;; or construct one from the context if not present.
(define (find-concept-by-intent lattice int)
  (let loop ((cs lattice))
    (cond ((null? cs) #f)
          ((equal? (concept-intent (car cs)) int) (car cs))
          (else (loop (cdr cs))))))

;; Construct a (wile algebra lattice) from an FCA concept lattice.
;; ctx: the FCA context (needed for Galois connection operations)
;; concepts: the list of concepts from (concept-lattice ctx)
;;
;; Lattice ordering: C1 ≤ C2 iff E1 ⊆ E2 (equiv. I2 ⊆ I1)
;; Join: concept whose intent = closure(I1 ∩ I2)
;; Meet: concept whose intent = closure(I1 ∪ I2)
(define (concept-lattice->algebra-lattice ctx concepts)
  (let* (;; Top: largest extent = concept with empty or smallest intent
         (top-concept
           (let loop ((cs concepts) (best (car concepts)))
             (if (null? cs) best
               (loop (cdr cs)
                     (if (> (length (concept-extent (car cs)))
                            (length (concept-extent best)))
                       (car cs) best)))))
         ;; Bottom: smallest extent = concept with largest intent
         (bottom-concept
           (let loop ((cs concepts) (best (car concepts)))
             (if (null? cs) best
               (loop (cdr cs)
                     (if (< (length (concept-extent (car cs)))
                            (length (concept-extent best)))
                       (car cs) best))))))
    (make-lattice
      ;; join: least upper bound
      ;; Intent of join = closure of (I1 ∩ I2)
      (lambda (c1 c2)
        (let* ((i-isect (sset-intersect (concept-intent c1) (concept-intent c2)))
               (ext (extent ctx i-isect))
               (int (intent ctx ext)))
          (or (find-concept-by-intent concepts int)
              (cons ext int))))
      ;; meet: greatest lower bound
      ;; Intent of meet = closure of (I1 ∪ I2)
      (lambda (c1 c2)
        (let* ((i-union (sset-union (concept-intent c1) (concept-intent c2)))
               (ext (extent ctx i-union))
               (int (intent ctx ext)))
          (or (find-concept-by-intent concepts int)
              (cons ext int))))
      ;; bottom
      bottom-concept
      ;; top
      top-concept
      ;; leq: C1 ≤ C2 iff I2 ⊆ I1 (more attributes = lower in lattice)
      (lambda (c1 c2)
        (sset-subset? (concept-intent c2) (concept-intent c1))))))

;;; ── Concept relationship ────────────────────────────────

;; Determine the relationship between two concepts.
;; Returns one of: 'subconcept, 'superconcept, 'equal, 'incomparable
(define (concept-relationship c1 c2)
  (let ((i1 (concept-intent c1))
        (i2 (concept-intent c2)))
    (let ((i2-sub-i1 (sset-subset? i2 i1))
          (i1-sub-i2 (sset-subset? i1 i2)))
      (cond ((and i1-sub-i2 i2-sub-i1) 'equal)
            (i2-sub-i1 'subconcept)    ;; I2 ⊆ I1 means E1 ⊆ E2, C1 ≤ C2
            (i1-sub-i2 'superconcept)  ;; I1 ⊆ I2 means E2 ⊆ E1, C2 ≤ C1
            (else 'incomparable)))))

;;; ── Annotated boundary report ───────────────────────────

;; Summarize a concept for annotation (short description from intent).
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
  (map (lambda (xb-concept)
         (let* ((ext (concept-extent xb-concept))
                (int (concept-intent xb-concept))
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
                  (keep (lambda (r) (eq? (cdr (assoc 'relationship r)) 'subconcept))
                          relations))
                (superconcepts
                  (keep (lambda (r) (eq? (cdr (assoc 'relationship r)) 'superconcept))
                          relations))
                (incomparables
                  (keep (lambda (r) (eq? (cdr (assoc 'relationship r)) 'incomparable))
                          relations)))
           (list (cons 'extent ext)
                 (cons 'intent int)
                 (cons 'extent-size (length ext))
                 (cons 'summary (concept-summary xb-concept))
                 (cons 'subconcept-of
                   (map (lambda (r) (cdr (assoc 'concept-summary r))) subconcepts))
                 (cons 'superconcept-of
                   (map (lambda (r) (cdr (assoc 'concept-summary r))) superconcepts))
                 (cons 'incomparable-with
                   (map (lambda (r) (cdr (assoc 'concept-summary r))) incomparables)))))
       cross-concepts))
