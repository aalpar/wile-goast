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

;;; (wile goast taint) — interprocedural taint flows over a Go call graph.
;;; Function-summary fidelity: nodes are functions; each call site f->g is a
;;; call (open) + return (close) edge; functions taint-transparent unless a
;;; sanitizer. taint-flows reports (source-name . sink-name) pairs connected by
;;; a VALID interprocedural path (see (wile goast ifds)).
;;; LIMITATION: function granularity over-approximates (no intraprocedural
;;; def-use) — sound-ish with false positives.

(define (taint-flows cg sources sinks . opt)
  "Report taint flows over call graph CG. SOURCES and SINKS are predicates
(cg-node -> bool). Optional SANITIZER predicate cuts flow through matching
nodes. Returns a list of (source-name . sink-name) pairs joined by a valid
interprocedural path. Over-approximate (function granularity).

Parameters:
  cg : list
  sources : procedure
  sinks : procedure
  sanitizer : procedure (optional)
Returns: list
Category: goast-taint
See also: `make-ifds-analysis', `ifds-reachable?'."
  (let* ((san? (if (pair? opt) (car opt) (lambda (n) #f)))
         ;; live = non-sanitizer nodes that HAVE a name. Name-less nodes
         ;; (anonymous / synthetic funcs that real call graphs can emit) cannot
         ;; be graph node ids; excluding them upholds the invariant that every
         ;; id reaching the engine is in LIVE-NAMES, so a #f never poisons
         ;; cfl-solve / ifds-reachable? (both fail-fast on an undeclared node).
         (live (filter (lambda (n) (and (not (san? n)) (nf n 'name))) cg))
         (live-names (map (lambda (n) (nf n 'name)) live))
         (live? (lambda (nm) (and (member nm live-names) #t)))
         (call-sites
           (let loop ((ns live) (i 0) (acc '()))
             (if (null? ns) (reverse acc)
                 (let ((f (nf (car ns) 'name)))
                   (let inner ((es (or (nf (car ns) 'edges-out) '())) (i i) (acc acc))
                     (cond ((null? es) (loop (cdr ns) i acc))
                           ((live? (nf (car es) 'callee))
                            (inner (cdr es) (+ i 1)
                                   (cons (list f i (nf (car es) 'callee)) acc)))
                           (else (inner (cdr es) i acc))))))))
         (analysis (make-ifds-analysis live-names call-sites))
         (srcs (filter sources live))
         (snks (filter sinks live)))
    (append-map
      (lambda (s)
        (filter-map
          (lambda (t)
            (and (ifds-reachable? analysis (nf s 'name) (nf t 'name))
                 (cons (nf s 'name) (nf t 'name))))
          snks))
      srcs)))

;; --- Predicate builders (composable, LLM-authorable) ---

(define (taint-from-names names)
  "Return a predicate matching cg-nodes whose name is an element of NAMES.
NAMES is a list of exact strings.

Parameters:
  names : list
Returns: procedure
Category: goast-taint
See also: `taint-from-pattern', `taint-flows'."
  (lambda (n)
    (and (member (nf n 'name) names) #t)))

(define (taint-from-pattern substr)
  "Return a predicate matching cg-nodes whose name contains SUBSTR.

Parameters:
  substr : string
Returns: procedure
Category: goast-taint
See also: `taint-from-names', `taint-flows'."
  (lambda (n)
    (let ((nm (nf n 'name)))
      (and (string? nm) (string-contains? nm substr)))))

;; --- Default Go security sets (starter; overridable) ---

(define taint-default-sources
  (taint-from-names '("net/http.Request.FormValue" "net/http.Request.PostFormValue"
                      "net/url.Values.Get" "os.Getenv" "bufio.Reader.ReadString")))

(define taint-default-sinks
  (taint-from-names '("os/exec.Command" "os/exec.CommandContext"
                      "database/sql.DB.Query" "database/sql.DB.Exec"
                      "os.OpenFile" "os.ReadFile")))

(define taint-default-sanitizers
  (taint-from-names '("strconv.Atoi" "path/filepath.Clean")))
