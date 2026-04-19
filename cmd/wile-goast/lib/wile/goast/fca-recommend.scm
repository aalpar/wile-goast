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

;;; fca-recommend.scm — Function boundary recommendations via FCA + SSA
;;;
;;; Analyzes concept lattice structure to produce ranked split/merge/extract
;;; recommendations for function boundaries. SSA data flow filtering
;;; distinguishes intentional coordination from accidental aggregation.
;;; Pareto dominance ranking with separate frontiers per type.

;;; ── Local utilities ─────────────────────────────────────

(define (string-suffix? suffix s)
  (let ((slen (string-length s))
        (sufflen (string-length suffix)))
    (and (>= slen sufflen)
         (string=? (substring s (- slen sufflen) slen) suffix))))

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

;;; ── Merge candidate detection ───────────────────────────

;; Merge candidates: concepts with |E| >= 2 and non-empty intent.
;; Multiple functions sharing the same field access pattern.
(define (merge-candidates lattice)
  (filter-map
    (lambda (concept)
      (let ((ext (concept-extent concept))
            (int (concept-intent concept)))
        (if (or (< (length ext) 2) (null? int)) #f
          (let* ((func-intents
                   (map (lambda (f)
                          (let loop ((cs lattice) (best int))
                            (cond ((null? cs) best)
                                  ((and (member? f (concept-extent (car cs)))
                                        (set-subset? best (concept-intent (car cs))))
                                   (loop (cdr cs) (concept-intent (car cs))))
                                  (else (loop (cdr cs) best)))))
                        ext))
                 (all-union (let loop ((fis func-intents) (acc '()))
                              (if (null? fis) acc
                                (loop (cdr fis) (set-union acc (car fis))))))
                 (overlap (if (null? all-union) 0
                            (exact->inexact (/ (length int) (length all-union)))))
                 (write-fields (filter
                                 (lambda (a) (not (string-suffix? ":r" a))) int))
                 (all-write (filter
                              (lambda (a) (not (string-suffix? ":r" a))) all-union))
                 (write-ovl (if (null? all-write) 0
                              (exact->inexact (/ (length write-fields)
                                                 (length all-write)))))
                 (factors
                   (list (cons 'intent-overlap overlap)
                         (cons 'write-overlap write-ovl)
                         (cons 'extent-count (length ext)))))
            (list (cons 'type 'merge)
                  (cons 'functions ext)
                  (cons 'factors factors)
                  (cons 'shared-intent int))))))
    lattice))

;;; ── Extract candidate detection ─────────────────────────

;; Lattice depth of a concept (distance from top).
(define (concept-depth lattice concept)
  (let ((target-int (concept-intent concept)))
    (let loop ((cs lattice) (depth 0) (prev-size 0))
      (cond ((null? cs) depth)
            ((equal? (concept-intent (car cs)) target-int) depth)
            ((and (set-subset? (concept-intent (car cs)) target-int)
                  (> (length (concept-intent (car cs))) prev-size))
             (loop (cdr cs) (+ depth 1) (length (concept-intent (car cs)))))
            (else (loop (cdr cs) depth prev-size))))))

;; Extract candidates: concept pairs (C_broad, C_narrow) where
;; C_broad has broader extent and smaller intent. The broad concept's
;; intent is the shared sub-operation.
(define (extract-candidates lattice)
  (let ((multi-extent
          (filter (lambda (c)
                    (and (>= (length (concept-extent c)) 2)
                         (pair? (concept-intent c))))
                  lattice)))
    (filter-map
      (lambda (c-broad)
        (let* ((e-broad (concept-extent c-broad))
               (i-broad (concept-intent c-broad))
               (narrower
                 (filter
                   (lambda (c)
                     (and (pair? (concept-extent c))
                          (< (length (concept-extent c)) (length e-broad))
                          (set-subset? i-broad (concept-intent c))
                          (not (equal? (concept-intent c) i-broad))))
                   lattice))
               (best-narrow
                 (if (null? narrower) #f
                   (let loop ((ns (cdr narrower)) (best (car narrower)))
                     (if (null? ns) best
                       (loop (cdr ns)
                             (if (> (length (concept-extent (car ns)))
                                    (length (concept-extent best)))
                               (car ns) best)))))))
          (if (not best-narrow) #f
            (let* ((e-narrow (concept-extent best-narrow))
                   (ratio (exact->inexact
                            (/ (length e-broad) (length e-narrow))))
                   (depth (concept-depth lattice c-broad))
                   (factors
                     (list (cons 'extent-ratio ratio)
                           (cons 'intent-size (length i-broad))
                           (cons 'sub-concept-depth depth))))
              (list (cons 'type 'extract)
                    (cons 'sub-operation i-broad)
                    (cons 'factors factors)
                    (cons 'broad-extent e-broad)
                    (cons 'narrow-extent e-narrow))))))
      multi-extent)))

;;; ── Top-level recommendation pipeline ───────────────────

;; Produce ranked recommendations: three Pareto frontiers.
;; lattice: from (concept-lattice ctx)
;; ssa-funcs: from (go-ssa-build session), or #f to skip SSA filtering
(define (boundary-recommendations lattice ssa-funcs)
  (let* ((splits (split-candidates lattice ssa-funcs))
         (merges (merge-candidates lattice))
         (extracts (extract-candidates lattice))
         (split-input
           (map (lambda (s)
                  (list (cdr (assoc 'function s))
                        (cdr (assoc 'factors s))))
                splits))
         (merge-input
           (map (lambda (m)
                  (list (cdr (assoc 'functions m))
                        (cdr (assoc 'factors m))))
                merges))
         (extract-input
           (map (lambda (e)
                  (list (cdr (assoc 'sub-operation e))
                        (cdr (assoc 'factors e))))
                extracts))
         (empty-frontier '((frontier) (dominated)))
         (split-frontier
           (if (null? split-input) empty-frontier
             (pareto-frontier split-input
               '(incomparable-count intent-disjointness no-cross-flow
                 pattern-balance stmt-count))))
         (merge-frontier
           (if (null? merge-input) empty-frontier
             (pareto-frontier merge-input
               '(intent-overlap write-overlap extent-count))))
         (extract-frontier
           (if (null? extract-input) empty-frontier
             (pareto-frontier extract-input
               '(extent-ratio intent-size sub-concept-depth)))))
    (list (cons 'splits split-frontier)
          (cons 'merges merge-frontier)
          (cons 'extracts extract-frontier))))
