;;; fca-recommend.scm — Function boundary recommendations via FCA + SSA
;;;
;;; Analyzes concept lattice structure to produce ranked split/merge/extract
;;; recommendations for function boundaries. SSA data flow filtering
;;; distinguishes intentional coordination from accidental aggregation.
;;; Pareto dominance ranking with separate frontiers per type.

;;; ── Local utilities ─────────────────────────────────────

(define (filter pred lst)
  (filter-map (lambda (x) (and (pred x) x)) lst))

(define (string-suffix? suffix s)
  (let ((slen (string-length s))
        (sufflen (string-length suffix)))
    (and (>= slen sufflen)
         (string=? (substring s (- slen sufflen) slen) suffix))))

;;; ── Factor comparison ───────────────────────────────────

;; Compare two factor values. Booleans: #f < #t. Numbers: standard <=.
(define (factor-leq? a b)
  (cond ((boolean? a) (or (not a) b))
        ((boolean? b) (and a b))
        (else (<= a b))))

(define (factor-less? a b)
  (and (factor-leq? a b) (not (equal? a b))))

;;; ── Pareto dominance ────────────────────────────────────

;; X dominates Y iff X >= Y on every factor and X > Y on at least one.
(define (dominates? factors-x factors-y)
  (let loop ((fx factors-x) (any-strict #f))
    (if (null? fx)
      any-strict
      (let* ((key (car (car fx)))
             (vx (cdr (car fx)))
             (vy (cdr (assoc key factors-y))))
        (if (factor-leq? vy vx)
          (loop (cdr fx) (or any-strict (factor-less? vy vx)))
          #f)))))

;; Compute Pareto frontier and dominated groups.
;; candidates: list of (id factors-alist) pairs.
;; factor-names: list of factor name symbols (documentation only).
;; Returns: ((frontier id ...) (dominated (dominator-id dominated-id ...) ...))
(define (pareto-frontier candidates factor-names)
  (let* ((ids (map car candidates))
         (factors-of (lambda (id)
                       (cadr (let loop ((cs candidates))
                               (cond ((null? cs) #f)
                                     ((equal? (car (car cs)) id) (car cs))
                                     (else (loop (cdr cs))))))))
         (frontier-ids
           (filter-map
             (lambda (c)
               (let ((c-id (car c))
                     (c-factors (cadr c)))
                 (let dominated? ((rest candidates))
                   (cond ((null? rest) c-id)
                         ((equal? (car (car rest)) c-id) (dominated? (cdr rest)))
                         ((dominates? (cadr (car rest)) c-factors) #f)
                         (else (dominated? (cdr rest)))))))
             candidates))
         (dominated-ids (filter (lambda (id) (not (member? id frontier-ids))) ids))
         (dom-groups
           (filter-map
             (lambda (fid)
               (let* ((fid-factors (factors-of fid))
                      (doms (filter
                              (lambda (did)
                                (dominates? fid-factors (factors-of did)))
                              dominated-ids)))
                 (if (null? doms) #f
                   (cons fid doms))))
             frontier-ids)))
    (list (cons 'frontier frontier-ids)
          (cons 'dominated dom-groups))))

;;; ── Lattice analysis utilities ──────────────────────────

(define (concept-signature lattice func-name)
  (filter
    (lambda (concept)
      (member? func-name (concept-extent concept)))
    lattice))

;; Two concepts are incomparable when neither intent is a subset of the other.
(define (concepts-incomparable? c1 c2)
  (let ((i1 (concept-intent c1))
        (i2 (concept-intent c2)))
    (and (not (set-subset? i1 i2))
         (not (set-subset? i2 i1)))))

;; All incomparable pairs in a concept set.
(define (incomparable-pairs concepts)
  (let loop ((cs concepts) (acc '()))
    (if (null? cs) acc
      (let inner ((rest (cdr cs)) (acc acc))
        (if (null? rest)
          (loop (cdr cs) acc)
          (inner (cdr rest)
                 (if (concepts-incomparable? (car cs) (car rest))
                   (cons (cons (car cs) (car rest)) acc)
                   acc)))))))

;;; ── Stubs for remaining exports (implemented in later tasks) ──

(define (split-candidates lattice ssa-funcs) '())
(define (merge-candidates lattice) '())
(define (extract-candidates lattice) '())
(define (boundary-recommendations lattice ssa-funcs) '())
