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

;; --- SCC side-query API (call-graph mutual-recursion detection) ---
;;
;; A non-trivial SCC in the call graph is a mutual-recursion cluster:
;; functions that can reach each other along call edges. Self-loops
;; (direct recursion) also count as non-trivial. Trivial SCCs are
;; non-recursive functions.

(define path-analysis-sccs graph-analysis-sccs)

(define path-node-in-cycle? graph-node-in-cycle?)

(define path-cyclic-nodes graph-cyclic-nodes)

;; --- Fast-path introspection ---
;;
;; The bigint-counting fast path activates when:
;;   - semiring is (bigint-counting-semiring), and
;;   - edge-weight is #f (unit weights), and
;;   - the (wile algebragraph) extension is present (kitchen-sink profile).
;; When active, queries route through an in-place big.Int kernel instead
;; of the per-relaxation allocating Scheme loop. wile-goast builds with
;; kitchen-sink, so the kernel is available.

(define path-analysis-fast-path? graph-analysis-fast-path?)

(define path-analysis-fast-path-kind graph-analysis-fast-path-kind)
