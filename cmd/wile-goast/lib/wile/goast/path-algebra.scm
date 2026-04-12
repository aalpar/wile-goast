;;; (wile goast path-algebra) — Semiring path computation over call graphs
;;;
;;; Lazy single-source Bellman-Ford parameterized by semiring.
;;; Boolean semiring = reachability, tropical = shortest path,
;;; counting = path count.

;; --- Record type ---

(define-record-type <path-analysis>
  (make-path-analysis* semiring adjacency weight-fn cache)
  path-analysis?
  (semiring   pa-semiring)
  (adjacency  pa-adjacency)
  (weight-fn  pa-weight-fn)
  (cache      pa-cache set-pa-cache!))

;; --- Adjacency construction ---

;; Build adjacency alist from CG node list: ((name . ((callee . edge) ...)) ...)
(define (build-adjacency cg)
  (let loop ((nodes cg) (adj '()))
    (if (null? nodes) adj
        (let* ((node (car nodes))
               (name (nf node 'name))
               (edges-out (nf node 'edges-out))
               (targets (map (lambda (e) (cons (nf e 'callee) e)) edges-out)))
          (loop (cdr nodes) (cons (cons name targets) adj))))))

;; --- Constructor ---

(define (make-path-analysis semiring cg edge-weight)
  (let ((adj (build-adjacency cg))
        (wfn (or edge-weight (lambda (_) (semiring-one semiring)))))
    (make-path-analysis* semiring adj wfn '())))

;; --- Stub query (implemented in Task 2) ---

(define (path-query pa source target)
  (error "path-query: not yet implemented"))

(define (path-query-all pa source)
  (error "path-query-all: not yet implemented"))
