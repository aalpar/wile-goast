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
;;; Thin wrapper over (wile algebra graph) that converts Go call graph
;;; nodes into the adjacency alist format expected by graph-analysis.

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
  "Construct a path analysis from a semiring, call graph, and edge-weight function.\nEDGE-WEIGHT receives a cg-edge and returns a semiring value.\nPass #f for unit weights (each edge = semiring-one).\n\nParameters:\n  semiring : any\n  cg : list\n  edge-weight : procedure-or-false\nReturns: graph-analysis\nCategory: goast-path\n\nExamples:\n  (make-path-analysis (boolean-semiring) cg #f)\n  (make-path-analysis (tropical-semiring) cg (lambda (e) 1))\n\nSee also: `path-query', `path-query-all'."
  (make-graph-analysis semiring (build-adjacency cg) edge-weight))

;; --- Public API ---

(define path-analysis? graph-analysis?)

(define path-query graph-query)

(define path-query-all graph-query-all)
