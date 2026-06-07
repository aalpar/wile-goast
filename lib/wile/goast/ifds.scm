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

;;; (wile goast ifds) — valid-path (realizable interprocedural path) reachability
;;; over (wile algebra cfl). Reps-Horwitz-Sagiv-Rosay valid-path grammar: every
;;; return matches its own call; returns to ancestors and descents into callees
;;; are allowed, but returning from an uncalled function is not.

(define (%id->string id)
  (cond ((string? id) id)
        ((symbol? id) (symbol->string id))
        ((number? id) (number->string id))
        (else (error "ifds: call-site id must be string/symbol/number" id))))

(define (ifds-open-label id)  (string->symbol (string-append "cfl-open-"  (%id->string id))))
(define (ifds-close-label id) (string->symbol (string-append "cfl-close-" (%id->string id))))
(define (%nt prefix id)       (string->symbol (string-append prefix (%id->string id))))

(define (make-valid-path-grammar call-site-ids)
  "Build the valid-path <cfl-grammar> (start symbol VP) over CALL-SITE-IDS, a
list of distinct call-site identifiers. Accepts exactly the valid
interprocedural paths: every return matches its own call.

Parameters:
  call-site-ids : list
Returns: cfl-grammar
Category: goast-ifds
See also: `make-ifds-analysis', `ifds-reachable?'."
  (let loop ((ids call-site-ids)
             (prods (list (cfl-epsilon 'B) (cfl-binary 'B 'B 'B) (cfl-unary 'VP 'B))))
    (if (null? ids)
        (make-cfl-grammar 'VP prods)
        (let* ((i (car ids)) (oi (%nt "O-" i)) (ci (%nt "C-" i)) (bx (%nt "Bx-" i)))
          (loop (cdr ids)
                (append (list (cfl-binary 'B oi bx) (cfl-binary bx 'B ci)
                              (cfl-binary 'VP ci 'VP) (cfl-binary 'VP 'VP oi)
                              (cfl-terminal oi (ifds-open-label i))
                              (cfl-terminal ci (ifds-close-label i)))
                        prods))))))

(define (make-ifds-analysis nodes call-sites)
  "Solve valid-path reachability over NODES with CALL-SITES, a list of
(from-node id to-node). Each call site adds a forward open edge from->to and a
return close edge to->from, then solves with cfl-solve. Returns a solved
analysis queryable by `ifds-reachable?'.

Parameters:
  nodes : list
  call-sites : list
Returns: cfl-solution
Category: goast-ifds
See also: `ifds-reachable?', `make-valid-path-grammar'."
  (let* ((ids (delete-duplicates (map cadr call-sites)))
         (edges (append-map
                  (lambda (cs)
                    (let ((from (car cs)) (id (cadr cs)) (to (caddr cs)))
                      (list (list from (ifds-open-label id) to)
                            (list to (ifds-close-label id) from))))
                  call-sites)))
    (cfl-solve (make-valid-path-grammar ids) (make-cfl-graph nodes edges))))

(define (ifds-reachable? analysis from to)
  "True iff TO is reachable from FROM along a valid interprocedural path in
ANALYSIS (the result of make-ifds-analysis).

Parameters:
  analysis : cfl-solution
  from : node
  to : node
Returns: boolean
Category: goast-ifds
See also: `make-ifds-analysis'."
  (cfl-reachable? analysis from to))
