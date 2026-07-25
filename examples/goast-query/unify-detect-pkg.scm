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

;;; unify-detect-pkg.scm — Unification detection across a Go module
;;;
;;; Loads all packages matching a pattern with go-typecheck-package,
;;; compares all cross-package function pairs within signature-shape
;;; groups. Reports candidates above the effective similarity threshold.
;;;
;;; Usage: cd /path/to/module && wile-goast -f unify-detect-pkg.scm
;;;   (must run from a directory where the Go module resolves)

(import (wile strings)        ; string-replace-all
        (wile goast utils)    ; nf, tag?, filter, filter-map, unique
        (wile goast unify))   ; ast-diff, classify-ast-diff, derivable?, score-diffs

;; ── Target ────────────────────────────────────────────────
(define target "./...")

;; ══════════════════════════════════════════════════════════
;; Local helpers the library does not export
;; ══════════════════════════════════════════════════════════

(define (tagged-node? v)
  (and (pair? v) (symbol? (car v))))

(define (last-element lst)
  (if (null? (cdr lst)) (car lst)
    (last-element (cdr lst))))

;; ══════════════════════════════════════════════════════════
;; Template construction (anti-unification)
;;
;; Walks both ASTs in lockstep. At each leaf:
;;   - Root type substitution → named variable (?T0, ?T1, ...)
;;   - Derived type diff → apply variable names to string
;;   - Literal/operator diff → flag for human review
;;   - Identifier diff → keep side-A value (cosmetic)
;; ══════════════════════════════════════════════════════════

;; Root variable index: list of (val-a val-b var-name).
(define (make-root-var-index roots)
  (let loop ((rs roots) (i 0) (acc '()))
    (if (null? rs) (reverse acc)
      (loop (cdr rs) (+ i 1)
            (cons (list (caar rs) (cdar rs)
                        (string-append "?T" (number->string i)))
                  acc)))))

;; Look up (node-a, node-b) in root var index. Returns var-name or #f.
(define (find-root-var node-a node-b rvi)
  (let loop ((idx rvi))
    (cond
      ((null? idx) #f)
      ((and (equal? (car (car idx)) node-a)
            (equal? (cadr (car idx)) node-b))
       (caddr (car idx)))
      (else (loop (cdr idx))))))

;; Replace root a-values with variable names in a string.
(define (templatize-string str rvi)
  (let loop ((s str) (rs rvi))
    (if (null? rs) s
      (loop (string-replace-all s (car (car rs)) (caddr (car rs)))
            (cdr rs)))))

;; Convert rvi to roots format for derivable? checks.
(define (rvi->roots rvi)
  (map (lambda (e) (cons (car e) (cadr e))) rvi))

;; Build a template from two ASTs and a root variable index.
;; Returns (template . flags); each flag is (category path val-a val-b),
;; in encounter order.
;;
;; The walk is three mutually recursive procedures over a flag
;; accumulator. Script toplevel resolves neither forward references nor
;; set! on its own bindings, so the whole nest lives in one letrec* body
;; — which also makes the clear-before-use protocol unforgettable.
(define build-template
  (let ((flags (make-vector 1 '())))

    (define (add-flag! category path val-a val-b)
      (vector-set! flags 0
                   (cons (list category path val-a val-b)
                         (vector-ref flags 0))))

    (define (bt node-a node-b rvi roots path)
      (cond
        ((and (tagged-node? node-a) (tagged-node? node-b))
         (if (eq? (car node-a) (car node-b))
           (cons (car node-a)
                 (bt-fields (cdr node-a) (cdr node-b) rvi roots path))
           (begin (add-flag! 'structural path node-a node-b)
                  node-a)))

        ((and (list? node-a) (list? node-b))
         (bt-list node-a node-b rvi roots path 0))

        ((and (eq? node-a #f) (eq? node-b #f)) #f)

        ((equal? node-a node-b) node-a)

        ((and (string? node-a) (string? node-b))
         (let ((var (find-root-var node-a node-b rvi)))
           (if var
             var
             (if (derivable? node-a node-b roots)
               (templatize-string node-a rvi)
               ;; classify-ast-diff ignores the tag argument; it decides from
               ;; the leaf field and the path.
               (let ((cat (classify-ast-diff
                            #f (if (null? path) #f (last-element path))
                            node-a node-b path)))
                 (if (memq cat '(type-name literal-value))
                   (begin (add-flag! cat path node-a node-b) node-a)
                   node-a))))))

        ((and (symbol? node-a) (symbol? node-b))
         (begin (add-flag! 'operator path node-a node-b) node-a))

        ((and (boolean? node-a) (boolean? node-b))
         (begin (add-flag! 'literal-value path node-a node-b) node-a))

        ((and (number? node-a) (number? node-b))
         (begin (add-flag! 'literal-value path node-a node-b) node-a))

        (else
          (begin (add-flag! 'structural path node-a node-b) node-a))))

    (define (bt-fields fields-a fields-b rvi roots path)
      (let ((result
              (filter-map
                (lambda (pair-a)
                  (if (not (pair? pair-a)) #f
                    (let* ((key (car pair-a))
                           (val-a (cdr pair-a))
                           (entry-b (assoc key fields-b)))
                      (if entry-b
                        (cons key (bt val-a (cdr entry-b) rvi roots
                                     (append path (list key))))
                        (begin
                          (add-flag! 'missing-field
                                     (append path (list key)) val-a #f)
                          pair-a)))))
                fields-a)))
        (for-each
          (lambda (pair-b)
            (if (and (pair? pair-b)
                     (not (assoc (car pair-b) fields-a)))
              (add-flag! 'extra-field
                         (append path (list (car pair-b))) #f (cdr pair-b))))
          fields-b)
        result))

    (define (bt-list lst-a lst-b rvi roots path idx)
      (cond
        ((and (null? lst-a) (null? lst-b)) '())
        ((null? lst-b) lst-a)
        ((null? lst-a) '())
        (else
          (cons (bt (car lst-a) (car lst-b) rvi roots
                    (append path (list idx)))
                (bt-list (cdr lst-a) (cdr lst-b) rvi roots
                         path (+ idx 1))))))

    (lambda (node-a node-b rvi)
      (vector-set! flags 0 '())
      (let ((template (bt node-a node-b rvi (rvi->roots rvi) '())))
        (cons template (reverse (vector-ref flags 0)))))))

;; ══════════════════════════════════════════════════════════
;; Package → function extraction
;; ══════════════════════════════════════════════════════════

;; Extract all func-decl nodes from a package s-expression.
(define (package-funcs pkg)
  (flat-map
    (lambda (file)
      (filter-map
        (lambda (decl)
          (and (tag? decl 'func-decl) decl))
        (nf file 'decls)))
    (nf pkg 'files)))

;; Compute signature shape: (param-count . result-count).
;; Receiver is excluded from param count (it's structural, not a param).
(define (signature-shape func)
  (let* ((ftype (nf func 'type))
         (params (nf ftype 'params))
         (results (nf ftype 'results))
         (pc (if (and params (pair? params)) (length params) 0))
         (rc (if (and results (pair? results)) (length results) 0)))
    (cons pc rc)))

;; Is this a method (has receiver)?
(define (method? func)
  (let ((recv (nf func 'recv)))
    (and recv (pair? recv))))

;; ══════════════════════════════════════════════════════════
;; Reporting
;; ══════════════════════════════════════════════════════════

(define (summarize val)
  (cond
    ((not val) "#f")
    ((string? val) val)
    ((symbol? val) (symbol->string val))
    ((number? val) (number->string val))
    ((boolean? val) (if val "#t" "#f"))
    ((and (pair? val) (symbol? (car val)))
     (string-append "(" (symbol->string (car val)) " ...)"))
    ((pair? val) "(list ...)")
    (else "?")))

(define (display-diff-entry d)
  (let ((category (list-ref d 0))
        (path (list-ref d 1))
        (val-a (list-ref d 2))
        (val-b (list-ref d 3)))
    (display "      ")
    (display category)
    (display "  at ")
    (display path)
    (newline)
    (display "        a: ")
    (display (summarize val-a))
    (newline)
    (display "        b: ")
    (display (summarize val-b))
    (newline)))

(define similarity-threshold 0.60)

(define (display-comparison label-a label-b func-a func-b result)
  (let* ((shared-count (diff-result-shared result))
         (diff-count (diff-result-diff-count result))
         (diffs (diff-result-diffs result))
         (score (score-diffs shared-count diff-count diffs))
         (eff-sim (list-ref score 1))
         (roots (list-ref score 4))
         (collapsed (list-ref score 6))
         (derived-count (list-ref score 7)))
    (if (>= eff-sim similarity-threshold)
      (let ((rvi (make-root-var-index roots)))
        (let* ((built (build-template func-a func-b rvi))
               (template (car built))
               (flags (cdr built))
               (flag-literals
                 (filter (lambda (f) (eq? (car f) 'literal-value)) flags))
               (flag-operators
                 (filter (lambda (f) (eq? (car f) 'operator)) flags))
               (flag-types
                 (filter (lambda (f) (eq? (car f) 'type-name)) flags))
               (flag-structural
                 (filter (lambda (f)
                           (memq (car f) '(structural missing-field
                                           extra-field)))
                         flags))
               (cosmetic-count
                 (+ derived-count
                    (length (filter (lambda (d)
                                     (eq? (car d) 'identifier))
                                   collapsed)))))
          (newline)
          (display "  ") (display label-a)
          (display "  <->  ") (display label-b) (newline)
          ;; Score vector.
          (display "    ── Score ──") (newline)
          (display "      similarity:      ")
          (display eff-sim) (newline)
          (display "      type params:     ")
          (display (length roots)) (newline)
          (for-each
            (lambda (e)
              (display "        ") (display (caddr e))
              (display ": ") (display (car e))
              (display " -> ") (display (cadr e)) (newline))
            rvi)
          (display "      literals:        ")
          (display (length flag-literals))
          (if (> (length flag-literals) 0)
            (display "  (review)"))
          (newline)
          (display "      operators:       ")
          (display (length flag-operators))
          (if (> (length flag-operators) 0)
            (display "  (review)"))
          (newline)
          (display "      cosmetic:        ")
          (display cosmetic-count) (newline)
          (display "      structural:      ")
          (display (length flag-structural)) (newline)
          ;; Flagged positions for human review.
          (let ((reviewable (append flag-types flag-literals flag-operators)))
            (if (pair? reviewable)
              (begin
                (display "    ── Flagged ──") (newline)
                (for-each display-diff-entry reviewable))))
          (newline)
          ;; template s-expression is built but not printed;
          ;; uncomment to dump: (write template) (newline)
          #t))
      #f)))

;; ══════════════════════════════════════════════════════════
;; Main
;; ══════════════════════════════════════════════════════════

(display "Loading ") (display target) (display " ...") (newline)
(define all-pkgs (go-typecheck-package target))
(display "Loaded ") (display (length all-pkgs)) (display " packages")
(newline)

;; Build a flat list of (qualified-name pkg-name func-decl shape) entries.
;; Skip functions with < 3 statements (too small to be interesting).
(define (body-size func)
  (let ((body (nf func 'body)))
    (if (and body (tag? body 'block))
      (let ((stmts (nf body 'list)))
        (if (pair? stmts) (length stmts) 0))
      0)))

(define min-body-size 3)

(define all-funcs
  (flat-map
    (lambda (pkg)
      (let ((pkg-name (nf pkg 'name)))
        (filter-map
          (lambda (func)
            (let ((name (nf func 'name)))
              (and (>= (body-size func) min-body-size)
                   (list (string-append pkg-name "." name)
                         pkg-name
                         func
                         (signature-shape func)))))
          (package-funcs pkg))))
    all-pkgs))

(display "  ") (display (length all-funcs))
(display " functions (>= ") (display min-body-size)
(display " statements)") (newline)
(newline)

;; Show per-package counts.
(let ((pkg-names (unique (map cadr all-funcs))))
  (for-each
    (lambda (pn)
      (let ((count (length (filter (lambda (e) (equal? (cadr e) pn))
                                   all-funcs))))
        (display "    ") (display pn) (display ": ")
        (display count) (newline)))
    pkg-names))
(newline)

;; ── Cross-package comparison within signature-shape groups ──

(display "══════════════════════════════════════════════════")
(newline)
(display "  Cross-package unification candidates            ")
(newline)
(display "══════════════════════════════════════════════════")
(newline)

;; For each ordered pair (i, j) where i < j, different packages, same shape.
;; The tally is threaded through both loops: script toplevel bindings are
;; immutable, so an accumulator stands in for a set! counter.
(define candidate-count
  (let ((n (length all-funcs))
        (v (list->vector all-funcs)))
    (let outer ((i 0) (total 0))
      (if (>= i n)
        total
        (let ((entry-a (vector-ref v i)))
          (let inner ((j (+ i 1)) (count total))
            (if (>= j n)
              (outer (+ i 1) count)
              (let ((entry-b (vector-ref v j)))
                (inner
                  (+ j 1)
                  ;; Same shape, different package.
                  (if (and (equal? (list-ref entry-a 3)
                                   (list-ref entry-b 3))
                           (not (equal? (cadr entry-a) (cadr entry-b))))
                    (let* ((func-a (caddr entry-a))
                           (func-b (caddr entry-b))
                           (label-a (car entry-a))
                           (label-b (car entry-b))
                           (result (ast-diff func-a func-b)))
                      (if (display-comparison label-a label-b
                                              func-a func-b result)
                        (+ count 1)
                        count))
                    count))))))))))

(if (= candidate-count 0)
  (begin (display "  (no candidates above threshold)") (newline)))

;; ── Summary ──────────────────────────────────────────────

(newline)
(display "── Summary ──") (newline)
(display "  Packages:    ") (display (length all-pkgs)) (newline)
(display "  Functions:   ") (display (length all-funcs))
(display " (>= ") (display min-body-size) (display " stmts)") (newline)
(display "  Candidates:  ") (display candidate-count)
(display " (effective similarity >= ")
(display similarity-threshold) (display ")") (newline)
