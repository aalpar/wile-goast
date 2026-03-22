;;; call-cluster.scm — Cluster methods by call-set Jaccard similarity
;;;
;;; For a given receiver type, computes pairwise Jaccard similarity
;;; between method call sets, then greedily clusters methods into
;;; subgroups. Reveals structure within types that lack a single
;;; dominant call convention.
;;;
;;; Companion to call-convention-mine.scm, which discovered that
;;; CompileTimeContinuation (95 methods) has no call convention
;;; above 60%. This script clusters those methods to see if
;;; subgroups exist.
;;;
;;; Pure AST analysis — no SSA or CFG needed.
;;;
;;; Usage: wile-goast -f examples/goast-query/call-cluster.scm

(import (wile goast utils))

;; ── Configuration ────────────────────────────────────────
(define target "github.com/aalpar/wile/machine")
(define target-type "CompileTimeContinuation")
(define min-cluster-similarity 0.30)

;; ══════════════════════════════════════════════════════════
;; Call Extraction (same as call-convention-mine.scm)
;; ══════════════════════════════════════════════════════════

;; Extract receiver type name from a func-decl node.
;; Handles *Foo, Foo, and Foo[T] (generic) receivers.
;; Returns string or #f.
(define (receiver-type-name func)
  (let ((recv (nf func 'recv)))
    (and recv (pair? recv)
         (let* ((recv-field (car recv))
                (recv-type (nf recv-field 'type))
                (base-type (if (tag? recv-type 'star-expr)
                             (nf recv-type 'x)
                             recv-type)))
           (cond
             ((tag? base-type 'ident)
              (nf base-type 'name))
             ((tag? base-type 'index-expr)
              (let ((x (nf base-type 'x)))
                (and (tag? x 'ident) (nf x 'name))))
             ((tag? base-type 'index-list-expr)
              (let ((x (nf base-type 'x)))
                (and (tag? x 'ident) (nf x 'name))))
             (else #f))))))

;; Extract callee name from a call-expr node.
(define (callee-name call-node)
  (let ((fun (nf call-node 'fun)))
    (cond
      ((tag? fun 'ident)
       (nf fun 'name))
      ((tag? fun 'selector-expr)
       (nf fun 'sel))
      (else #f))))

;; Extract unique callee names from a function body.
(define (extract-callees func)
  (let ((body (nf func 'body)))
    (if body
      (unique
        (walk body
          (lambda (node)
            (and (tag? node 'call-expr)
                 (callee-name node)))))
      '())))

;; ══════════════════════════════════════════════════════════
;; Jaccard Similarity
;; ══════════════════════════════════════════════════════════

;; Count elements of A present in B.
(define (set-intersect-count a b)
  (let loop ((xs a) (count 0))
    (if (null? xs) count
      (loop (cdr xs)
            (if (member? (car xs) b) (+ count 1) count)))))

;; Jaccard similarity: |A∩B| / |A∪B|
;; |A∪B| = |A| + |B| - |A∩B|
(define (jaccard a b)
  (let* ((isect (set-intersect-count a b))
         (union (- (+ (length a) (length b)) isect)))
    (if (= union 0) 0
      (exact->inexact (/ isect union)))))

;; ══════════════════════════════════════════════════════════
;; Insertion Sort
;; ══════════════════════════════════════════════════════════

;; Insert a similarity triple into a descending-sorted list.
;; Triple: (method-a method-b similarity)
(define (insert-by-sim entry sorted)
  (cond
    ((null? sorted) (list entry))
    ((>= (caddr entry) (caddr (car sorted)))
     (cons entry sorted))
    (else (cons (car sorted) (insert-by-sim entry (cdr sorted))))))

;; Insert a cluster entry into a descending-sorted-by-size list.
;; Entry: (cluster-id (member ...))
(define (insert-by-size entry sorted)
  (cond
    ((null? sorted) (list entry))
    ((>= (length (cadr entry)) (length (cadr (car sorted))))
     (cons entry sorted))
    (else (cons (car sorted) (insert-by-size entry (cdr sorted))))))

;; ══════════════════════════════════════════════════════════
;; Word-wrap utility
;; ══════════════════════════════════════════════════════════

(define (wrap-names names width)
  (let loop ((ns names) (line "") (lines '()))
    (cond
      ((null? ns)
       (reverse (if (> (string-length line) 0)
                  (cons line lines)
                  lines)))
      (else
       (let* ((name (car ns))
              (sep (if (> (string-length line) 0) ", " ""))
              (candidate (string-append line sep name)))
         (if (and (> (string-length line) 0)
                  (> (string-length candidate) width))
           (loop ns "" (cons line lines))
           (loop (cdr ns) candidate lines)))))))

;; ══════════════════════════════════════════════════════════
;; Greedy Agglomerative Clustering
;; ══════════════════════════════════════════════════════════

;; Find the cluster containing a method name.
;; Returns the cluster pair (id (members ...)) or #f.
(define (find-cluster method clusters)
  (cond
    ((null? clusters) #f)
    ((member? method (cadr (car clusters))) (car clusters))
    (else (find-cluster method (cdr clusters)))))

;; Average Jaccard similarity of a method to all members of a cluster.
;; call-sets is an alist: ((method-name (callee ...)) ...)
(define (avg-sim-to-cluster method cluster call-sets)
  (let ((method-callees (cadr (assoc method call-sets)))
        (members (cadr cluster)))
    (let loop ((ms members) (sum 0) (count 0))
      (if (null? ms)
        (if (= count 0) 0
          (exact->inexact (/ sum count)))
        (let* ((member-entry (assoc (car ms) call-sets))
               (member-callees (cadr member-entry))
               (sim (jaccard method-callees member-callees)))
          (loop (cdr ms) (+ sum sim) (+ count 1)))))))

;; Run greedy agglomerative clustering on sorted similarity triples.
;; Returns a list of clusters: ((id (member ...)) ...)
(define (cluster-methods sim-triples call-sets)
  (let ((clusters '())
        (next-id 0))
    (for-each
      (lambda (triple)
        (let* ((a (car triple))
               (b (cadr triple))
               (ca (find-cluster a clusters))
               (cb (find-cluster b clusters)))
          (cond
            ;; Both unclustered: create new cluster
            ((and (not ca) (not cb))
             (set! clusters (cons (list next-id (list a b)) clusters))
             (set! next-id (+ next-id 1)))
            ;; One in a cluster, other unclustered: maybe add
            ((and ca (not cb))
             (let ((avg (avg-sim-to-cluster b ca call-sets)))
               (if (>= avg min-cluster-similarity)
                 (set-cdr! ca (list (cons b (cadr ca)))))))
            ((and (not ca) cb)
             (let ((avg (avg-sim-to-cluster a cb call-sets)))
               (if (>= avg min-cluster-similarity)
                 (set-cdr! cb (list (cons a (cadr cb)))))))
            ;; Both in clusters (same or different): skip
            (else #f))))
      sim-triples)
    clusters))

;; ══════════════════════════════════════════════════════════
;; Cluster Characterization
;; ══════════════════════════════════════════════════════════

;; Core calls: intersection of all members' call sets.
;; Returns a list of callee names present in every member's call set.
(define (core-calls cluster call-sets)
  (let ((members (cadr cluster)))
    (if (null? members) '()
      (let ((first-callees (cadr (assoc (car members) call-sets))))
        (let loop ((candidates first-callees) (result '()))
          (if (null? candidates) (reverse result)
            (let ((callee (car candidates)))
              (if (let check ((ms (cdr members)))
                    (cond
                      ((null? ms) #t)
                      ((member? callee (cadr (assoc (car ms) call-sets)))
                       (check (cdr ms)))
                      (else #f)))
                (loop (cdr candidates) (cons callee result))
                (loop (cdr candidates) result)))))))))

;; ══════════════════════════════════════════════════════════
;; Main
;; ══════════════════════════════════════════════════════════

(display "Loading and type-checking ") (display target) (display " ...")
(newline)
(define pkgs (go-typecheck-package target))
(newline)

;; ── Extract methods for target type ──────────────────────

(define all-func-decls
  (flat-map
    (lambda (pkg)
      (flat-map
        (lambda (file)
          (filter-map
            (lambda (decl)
              (and (tag? decl 'func-decl)
                   (nf decl 'body)
                   decl))
            (nf file 'decls)))
        (nf pkg 'files)))
    pkgs))

;; Filter to methods on target-type only
(define target-methods
  (filter-map
    (lambda (func)
      (let ((tname (receiver-type-name func)))
        (and tname (equal? tname target-type) func)))
    all-func-decls))

(define total-methods (length target-methods))

;; Build call sets, excluding methods with no calls
(define call-sets
  (filter-map
    (lambda (func)
      (let* ((name (nf func 'name))
             (callees (extract-callees func)))
        (and (pair? callees)
             (list name callees))))
    target-methods))

(define n-with-calls (length call-sets))

;; ── Pairwise Jaccard Similarity ──────────────────────────

;; Use vectors for O(1) indexed access during pairwise computation
(define cs-vec (list->vector call-sets))
(define n-methods (vector-length cs-vec))

;; Compute all pairwise similarities, collect non-zero triples
(define sim-triples
  (let outer ((i 0) (sorted '()))
    (if (>= i n-methods) sorted
      (let* ((cs-i (vector-ref cs-vec i))
             (name-i (car cs-i))
             (callees-i (cadr cs-i)))
        (let inner ((j (+ i 1)) (acc sorted))
          (if (>= j n-methods)
            (outer (+ i 1) acc)
            (let* ((cs-j (vector-ref cs-vec j))
                   (name-j (car cs-j))
                   (callees-j (cadr cs-j))
                   (sim (jaccard callees-i callees-j)))
              (if (> sim 0)
                (inner (+ j 1) (insert-by-sim (list name-i name-j sim) acc))
                (inner (+ j 1) acc)))))))))

(define n-pairs (length sim-triples))

;; ── Report Header ────────────────────────────────────────

(display "==  ") (display target-type)
(display ": Method Clusters  ==") (newline)
(display "  ") (display n-with-calls)
(display " methods with calls (of ") (display total-methods)
(display " total)") (newline)
(display "  ") (display n-pairs)
(display " pairs with non-zero similarity") (newline)
(newline)

;; ── Top 10 Similarity Pairs ─────────────────────────────

(display "-- Top 10 Similarity Pairs --") (newline)
(let loop ((triples sim-triples) (i 0))
  (if (or (null? triples) (>= i 10)) #f
    (let* ((triple (car triples))
           (a (car triple))
           (b (cadr triple))
           (sim (caddr triple)))
      (display "  ") (display a)
      (display " <-> ") (display b)
      (display "  ") (display (exact->inexact sim))
      (newline)
      (loop (cdr triples) (+ i 1)))))
(newline)

;; ── Clustering ───────────────────────────────────────────

(define clusters (cluster-methods sim-triples call-sets))

;; Sort clusters by size descending
(define sorted-clusters
  (let loop ((cs clusters) (sorted '()))
    (if (null? cs) sorted
      (loop (cdr cs) (insert-by-size (car cs) sorted)))))

;; Count clustered methods
(define clustered-methods
  (apply + (map (lambda (c) (length (cadr c))) sorted-clusters)))
(define unclustered-count (- n-with-calls clustered-methods))

(display "  Clusters: ") (display (length sorted-clusters))
(display " (") (display clustered-methods)
(display " methods), Unclustered: ") (display unclustered-count)
(newline) (newline)

;; ── Display Each Cluster ─────────────────────────────────

(let loop ((cs sorted-clusters) (idx 1))
  (if (null? cs) #f
    (let* ((cluster (car cs))
           (members (cadr cluster))
           (n-members (length members))
           (core (core-calls cluster call-sets)))
      (display "-- Cluster ") (display idx)
      (display " (") (display n-members) (display " methods) --")
      (newline)
      (if (pair? core)
        (begin
          (display "  Core calls: ")
          (let ((wrapped (wrap-names core 60)))
            (display (car wrapped)) (newline)
            (for-each
              (lambda (line)
                (display "              ") (display line) (newline))
              (cdr wrapped))))
        (begin (display "  Core calls: (none)") (newline)))
      (display "  Members:") (newline)
      (let ((wrapped (wrap-names members 60)))
        (for-each
          (lambda (line)
            (display "    ") (display line) (newline))
          wrapped))
      (newline)
      (loop (cdr cs) (+ idx 1)))))

;; ── Unclustered Methods ──────────────────────────────────

(define all-clustered
  (flat-map (lambda (c) (cadr c)) sorted-clusters))

(define unclustered
  (filter-map
    (lambda (cs)
      (let ((name (car cs)))
        (and (not (member? name all-clustered))
             name)))
    call-sets))

(display "-- Unclustered (") (display (length unclustered))
(display " methods) --") (newline)
(let ((wrapped (wrap-names unclustered 60)))
  (for-each
    (lambda (line)
      (display "    ") (display line) (newline))
    wrapped))
(newline)
