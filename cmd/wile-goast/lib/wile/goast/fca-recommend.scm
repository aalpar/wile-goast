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

;;; ── SSA helpers ─────────────────────────────────────────

(define (find-ssa-func ssa-funcs func-name)
  (let loop ((fs ssa-funcs))
    (cond ((null? fs) #f)
          ((and (pair? (car fs))
                (string=? (or (nf (car fs) 'name) "") func-name))
           (car fs))
          (else (loop (cdr fs))))))

(define (ssa-stmt-count ssa-funcs func-name)
  (let ((fn (find-ssa-func ssa-funcs func-name)))
    (if fn (length (ssa-all-instrs fn)) 0)))

;;; ── SSA cross-flow detection ────────────────────────────

;; Extract struct.field key from an ssa-field-addr instruction.
(define (field-addr-key instr)
  (let ((struct-name (nf instr 'struct))
        (field-name (nf instr 'field)))
    (and struct-name field-name
         (string-append struct-name "." field-name))))

;; Register names for field-addrs whose struct.field is in intent-fields.
(define (cluster-field-addrs instrs intent-fields)
  (filter-map
    (lambda (instr)
      (and (tag? instr 'ssa-field-addr)
           (let ((key (field-addr-key instr)))
             (and key (set-member? key intent-fields)
                  (nf instr 'name)))))
    instrs))

;; Check cross-cluster data flow within a function.
;; Returns #t if any value from cluster1 fields reaches a store
;; targeting cluster2 fields via def-use chains.
(define (cross-flow-between? ssa-funcs func-name cluster1-fields cluster2-fields)
  (let ((fn (find-ssa-func ssa-funcs func-name)))
    (if (not fn) #f
      (let* ((instrs (ssa-all-instrs fn))
             (c1-names (cluster-field-addrs instrs cluster1-fields))
             (c2-names (cluster-field-addrs instrs cluster2-fields)))
        (if (or (null? c1-names) (null? c2-names)) #f
          (defuse-reachable? fn c1-names
            (lambda (instr)
              (and (tag? instr 'ssa-store)
                   (let ((addr (nf instr 'addr)))
                     (and addr (member? addr c2-names)))))
            10))))))

;;; ── Split candidate detection ──────────────────────────

(define (split-candidates lattice ssa-funcs)
  (let ((all-funcs (unique (flat-map concept-extent lattice))))
    (filter-map
      (lambda (func-name)
        (let* ((sig (concept-signature lattice func-name))
               (sig-nontop (filter (lambda (c) (pair? (concept-intent c))) sig))
               (pairs (incomparable-pairs sig-nontop)))
          (if (null? pairs) #f
            (let* ((pair1 (car pairs))
                   (c1 (car pair1))
                   (c2 (cdr pair1))
                   (i1 (concept-intent c1))
                   (i2 (concept-intent c2))
                   (e1 (concept-extent c1))
                   (e2 (concept-extent c2))
                   (isect (set-intersect i1 i2))
                   (iunion (set-union i1 i2))
                   (disjointness
                     (if (null? iunion) 0
                       (exact->inexact
                         (- 1 (/ (length isect) (length iunion))))))
                   (balance
                     (let ((min-e (min (length e1) (length e2)))
                           (max-e (max (length e1) (length e2))))
                       (if (= max-e 0) 0
                         (exact->inexact (/ min-e max-e)))))
                   (no-cross-flow
                     (if ssa-funcs
                       (not (or (cross-flow-between? ssa-funcs func-name i1 i2)
                                (cross-flow-between? ssa-funcs func-name i2 i1)))
                       #t))
                   (stmt-ct (if ssa-funcs
                              (ssa-stmt-count ssa-funcs func-name) 0))
                   (factors
                     (list (cons 'incomparable-count (length pairs))
                           (cons 'intent-disjointness disjointness)
                           (cons 'no-cross-flow no-cross-flow)
                           (cons 'pattern-balance balance)
                           (cons 'stmt-count stmt-ct))))
              (list (cons 'type 'split)
                    (cons 'function func-name)
                    (cons 'factors factors)
                    (cons 'clusters
                      (list (list (cons 'intent i1) (cons 'extent e1))
                            (list (cons 'intent i2) (cons 'extent e2)))))))))
      all-funcs)))

;;; ── Stubs for remaining exports (implemented in later tasks) ──

(define (merge-candidates lattice) '())
(define (extract-candidates lattice) '())
(define (boundary-recommendations lattice ssa-funcs) '())
