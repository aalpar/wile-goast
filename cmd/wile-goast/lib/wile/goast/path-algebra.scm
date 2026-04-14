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
  "Construct a path analysis from a semiring, call graph, and edge-weight function.\nEDGE-WEIGHT receives a cg-edge and returns a semiring value.\nPass #f for unit weights (each edge = semiring-one).\n\nParameters:\n  semiring : any\n  cg : list\n  edge-weight : procedure-or-false\nReturns: path-analysis\nCategory: goast-path\n\nExamples:\n  (make-path-analysis (boolean-semiring) cg #f)\n  (make-path-analysis (tropical-semiring) cg (lambda (e) 1))\n\nSee also: `path-query', `path-query-all'."
  (let ((adj (build-adjacency cg))
        (wfn (or edge-weight (lambda (_) (semiring-one semiring)))))
    (make-path-analysis* semiring adj wfn '())))

;; --- Local utilities ---

(define (filter pred lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (loop (cdr xs)
            (if (pred (car xs)) (cons (car xs) acc) acc)))))

;; --- Single-source computation ---

;; Compute distances from source using worklist Bellman-Ford.
;; Returns alist ((name . value) ...) for all reachable nodes.
(define (compute-single-source pa source)
  (let ((S   (pa-semiring pa))
        (adj (pa-adjacency pa))
        (wfn (pa-weight-fn pa)))
    (let loop ((worklist (list source))
               (dist (list (cons source (semiring-one S)))))
      (if (null? worklist) dist
          (let* ((node (car worklist))
                 (rest (cdr worklist))
                 (node-dist (cdr (assoc node dist))))
            ;; Get outgoing edges for this node
            (let ((entry (assoc node adj)))
              (if (not entry)
                  (loop rest dist)
                  (let edge-loop ((edges (cdr entry))
                                  (wl rest)
                                  (d dist))
                    (if (null? edges)
                        (loop wl d)
                        (let* ((callee-name (caar edges))
                               (edge (cdar edges))
                               (w (wfn edge))
                               (candidate (semiring-times S node-dist w))
                               (old-entry (assoc callee-name d))
                               (old-val (if old-entry (cdr old-entry) (semiring-zero S)))
                               (merged (semiring-plus S old-val candidate)))
                          (if (equal? merged old-val)
                              (edge-loop (cdr edges) wl d)
                              (let ((new-d (cons (cons callee-name merged)
                                                 (if old-entry
                                                     (filter (lambda (p) (not (equal? (car p) callee-name))) d)
                                                     d))))
                                (edge-loop (cdr edges)
                                           (if (member callee-name wl) wl (cons callee-name wl))
                                           new-d)))))))))))))

;; --- Cache layer ---

(define (get-or-compute pa source)
  (let ((cached (assoc source (pa-cache pa))))
    (if cached (cdr cached)
        (let ((result (compute-single-source pa source)))
          (set-pa-cache! pa (cons (cons source result) (pa-cache pa)))
          result))))

;; --- Public API ---

(define (path-query pa source target)
  "Query the semiring value between source and target.\nReturns semiring-zero if target is unreachable. Lazily computes\nand caches single-source distances on first query per source.\n\nParameters:\n  pa : path-analysis\n  source : string\n  target : string\nReturns: any\nCategory: goast-path\n\nSee also: `path-query-all', `make-path-analysis'."
  (let* ((dist (get-or-compute pa source))
         (entry (assoc target dist)))
    (if entry (cdr entry) (semiring-zero (pa-semiring pa)))))

(define (path-query-all pa source)
  "Return distance alist for all reachable nodes from source.\nEach entry is (name . semiring-value). Lazily computed and cached.\n\nParameters:\n  pa : path-analysis\n  source : string\nReturns: list\nCategory: goast-path\n\nSee also: `path-query', `make-path-analysis'."
  (get-or-compute pa source))
