;;; unify-detect-pkg.scm — Unification detection across a Go module
;;;
;;; Loads all packages matching a pattern with go-typecheck-package,
;;; compares all cross-package function pairs within signature-shape
;;; groups. Reports candidates above the effective similarity threshold.
;;;
;;; Usage: cd /path/to/module && wile-goast -f unify-detect-pkg.scm
;;;   (must run from a directory where the Go module resolves)

;; ── Target ────────────────────────────────────────────────
(define target "./...")

;; ══════════════════════════════════════════════════════════
;; Utilities
;; ══════════════════════════════════════════════════════════

(define (nf node key)
  (let ((e (assoc key (cdr node))))
    (if e (cdr e) #f)))

(define (tag? node t)
  (and (pair? node) (symbol? (car node)) (eq? (car node) t)))

(define (tagged-node? v)
  (and (pair? v) (symbol? (car v))))

(define (filter-map f lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (let ((v (f (car xs))))
        (loop (cdr xs) (if v (cons v acc) acc))))))

(define (filter pred lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (loop (cdr xs)
            (if (pred (car xs)) (cons (car xs) acc) acc)))))

(define (flat-map f lst)
  (apply append (map f lst)))

(define (unique lst)
  (let loop ((xs lst) (seen '()))
    (cond ((null? xs) (reverse seen))
          ((member (car xs) seen) (loop (cdr xs) seen))
          (else (loop (cdr xs) (cons (car xs) seen))))))

(define (last-element lst)
  (if (null? (cdr lst)) (car lst)
    (last-element (cdr lst))))

;; ══════════════════════════════════════════════════════════
;; Core diff engine
;; ══════════════════════════════════════════════════════════

(define (merge-results a b)
  (list (+ (car a) (car b))
        (+ (cadr a) (cadr b))
        (append (caddr a) (caddr b))))

(define (shared-result)
  (list 1 0 '()))

(define (diff-result category path val-a val-b)
  (list 0 1 (list (list category path val-a val-b))))

(define (merge-all results)
  (let loop ((rs results) (acc (list 0 0 '())))
    (if (null? rs) acc
      (loop (cdr rs) (merge-results acc (car rs))))))

(define (fields-diff fields-a fields-b path)
  (let ((results
          (filter-map
            (lambda (pair-a)
              (if (not (pair? pair-a)) #f
                (let* ((key (car pair-a))
                       (val-a (cdr pair-a))
                       (entry-b (assoc key fields-b)))
                  (if entry-b
                    (ast-diff val-a (cdr entry-b)
                              (append path (list key)))
                    (diff-result 'missing-field
                                 (append path (list key))
                                 val-a #f)))))
            fields-a)))
    (let ((extra
            (filter-map
              (lambda (pair-b)
                (if (not (pair? pair-b)) #f
                  (let ((key (car pair-b)))
                    (if (assoc key fields-a) #f
                      (diff-result 'extra-field
                                   (append path (list key))
                                   #f (cdr pair-b))))))
              fields-b)))
      (merge-all (append results extra)))))

(define (list-diff lst-a lst-b path idx)
  (cond
    ((and (null? lst-a) (null? lst-b))
     (list 0 0 '()))
    ((null? lst-b)
     (merge-all
       (map (lambda (ea)
              (diff-result 'extra-element
                           (append path (list idx))
                           ea #f))
            lst-a)))
    ((null? lst-a)
     (merge-all
       (map (lambda (eb)
              (diff-result 'extra-element
                           (append path (list idx))
                           #f eb))
            lst-b)))
    (else
      (merge-results
        (ast-diff (car lst-a) (car lst-b)
                  (append path (list idx)))
        (list-diff (cdr lst-a) (cdr lst-b) path (+ idx 1))))))

(define (ast-diff node-a node-b path)
  (cond
    ((and (tagged-node? node-a) (tagged-node? node-b))
     (if (eq? (car node-a) (car node-b))
       (merge-results
         (shared-result)
         (fields-diff (cdr node-a) (cdr node-b) path))
       (diff-result 'structural path node-a node-b)))

    ((and (list? node-a) (list? node-b))
     (list-diff node-a node-b path 0))

    ((and (eq? node-a #f) (eq? node-b #f))
     (shared-result))

    ((equal? node-a node-b)
     (shared-result))

    ((and (string? node-a) (string? node-b))
     (diff-result (classify-string-diff node-a node-b path)
                  path node-a node-b))

    ((and (symbol? node-a) (symbol? node-b))
     (diff-result 'operator path node-a node-b))

    ((and (boolean? node-a) (boolean? node-b))
     (diff-result 'literal-value path node-a node-b))

    ((and (number? node-a) (number? node-b))
     (diff-result 'literal-value path node-a node-b))

    (else
      (diff-result 'structural path node-a node-b))))

;; ══════════════════════════════════════════════════════════
;; Classification
;;
;; With go-typecheck-package, idents carry inferred-type and
;; obj-pkg annotations. The classifier uses these directly
;; when available, falling back to path heuristics.
;; ══════════════════════════════════════════════════════════

(define identifier-fields '(name sel label))

(define type-fields '(type inferred-type asserted-type obj-pkg
                      signature))

(define (in-type-position? path)
  (let loop ((xs path))
    (cond
      ((null? xs) #f)
      ((null? (cdr xs)) #f)
      ((and (symbol? (car xs)) (memq (car xs) type-fields))
       #t)
      (else (loop (cdr xs))))))

(define (classify-string-diff str-a str-b path)
  (let ((field (if (null? path) #f (last-element path))))
    (cond
      ((and (symbol? field) (memq field type-fields))
       'type-name)
      ((and (symbol? field) (eq? field 'name)
            (in-type-position? path))
       'type-name)
      ((and (symbol? field) (memq field identifier-fields))
       'identifier)
      ((and (symbol? field) (eq? field 'value))
       'literal-value)
      ((and (symbol? field) (eq? field 'tok))
       'operator)
      (else 'identifier))))

;; ══════════════════════════════════════════════════════════
;; Substitution collapsing
;;
;; Type annotations (inferred-type, obj-pkg) propagate root
;; type substitutions into every sub-expression. A single
;; root change like CounterValue→GValue generates dozens of
;; inferred-type diffs. We collapse these by:
;;
;;   1. Collecting all (val-a . val-b) type-name diff pairs
;;   2. Sorting by string length (shortest first)
;;   3. Iterating: if applying known roots to val-a yields
;;      val-b, the pair is derived. Otherwise it's a new root.
;;   4. Reclassifying derived diffs as 'derived-type (weight 0)
;; ══════════════════════════════════════════════════════════

;; Replace all non-overlapping occurrences of old with new in str.
(define (string-replace-all str old new)
  (let ((old-len (string-length old))
        (str-len (string-length str)))
    (if (or (= old-len 0) (< str-len old-len))
      str
      (let loop ((start 0) (parts '()))
        (let search ((i start))
          (cond
            ((> (+ i old-len) str-len)
             (apply string-append
                    (reverse (cons (substring str start str-len) parts))))
            ((string=? (substring str i (+ i old-len)) old)
             (loop (+ i old-len)
                   (cons new (cons (substring str start i) parts))))
            (else
             (search (+ i 1)))))))))

;; Apply a list of substitution pairs sequentially to a string.
(define (apply-substitutions str roots)
  (let loop ((s str) (rs roots))
    (if (null? rs) s
      (loop (string-replace-all s (caar rs) (cdar rs))
            (cdr rs)))))

;; Is (val-a . val-b) derivable from the known root substitutions?
(define (derivable? val-a val-b roots)
  (equal? (apply-substitutions val-a roots) val-b))

;; Sort pairs by length of car (ascending). Simple insertion sort — n is small.
(define (sort-by-length pairs)
  (define (insert p sorted)
    (cond
      ((null? sorted) (list p))
      ((<= (string-length (car p)) (string-length (caar sorted)))
       (cons p sorted))
      (else (cons (car sorted) (insert p (cdr sorted))))))
  (let loop ((ps pairs) (acc '()))
    (if (null? ps) acc
      (loop (cdr ps) (insert (car ps) acc)))))

;; Find the minimal set of root substitutions that explain all pairs.
;; Input: list of (val-a . val-b) from type-name diffs.
;; Output: list of root (val-a . val-b) pairs.
(define (find-root-substitutions pairs)
  (let ((sorted (sort-by-length (unique pairs))))
    (let loop ((ps sorted) (roots '()))
      (if (null? ps) roots
        (let ((a (caar ps)) (b (cdar ps)))
          (if (derivable? a b roots)
            (loop (cdr ps) roots)
            (loop (cdr ps) (cons (cons a b) roots))))))))

;; Reclassify type-name diffs: those derivable from roots → 'derived-type.
;; Returns new diff list with categories updated.
(define (collapse-diffs diffs roots)
  (map (lambda (d)
         (if (and (eq? (car d) 'type-name)
                  (string? (caddr d))
                  (string? (cadddr d))
                  (derivable? (caddr d) (cadddr d) roots))
           (cons 'derived-type (cdr d))
           d))
       diffs))

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

;; Flags accumulated during template construction.
;; Each entry: (category path val-a val-b)
(define template-flags '())

(define (reset-template-flags!)
  (set! template-flags '()))

(define (add-flag! category path val-a val-b)
  (set! template-flags
        (cons (list category path val-a val-b) template-flags)))

;; Top-level: build template from two ASTs and root variable index.
;; Returns the template s-expression. Populates template-flags as
;; a side effect (call reset-template-flags! before).
(define (build-template node-a node-b rvi)
  (bt node-a node-b rvi (rvi->roots rvi) '()))

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
           (let ((cat (classify-string-diff node-a node-b path)))
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

;; ══════════════════════════════════════════════════════════
;; Scoring
;; ══════════════════════════════════════════════════════════

(define diff-weights
  '((type-name . 1)
    (derived-type . 0)
    (literal-value . 1)
    (identifier . 0)
    (operator . 2)
    (structural . 100)
    (missing-field . 50)
    (extra-field . 50)
    (extra-element . 50)))

(define (diff-weight category)
  (let ((entry (assoc category diff-weights)))
    (if entry (cdr entry) 10)))

(define (score-diffs shared-count diff-count diffs)
  ;; Extract type-name pairs, find roots, collapse.
  (let* ((type-pairs
           (filter-map
             (lambda (d)
               (and (eq? (car d) 'type-name)
                    (string? (caddr d))
                    (string? (cadddr d))
                    (cons (caddr d) (cadddr d))))
             diffs))
         (roots (find-root-substitutions type-pairs))
         (collapsed (collapse-diffs diffs roots))
         (total (+ shared-count diff-count))
         ;; Similarity with derived diffs promoted to shared.
         (derived-count (length (filter (lambda (d)
                                          (eq? (car d) 'derived-type))
                                        collapsed)))
         (effective-shared (+ shared-count derived-count))
         (effective-diffs (- diff-count derived-count))
         (effective-total (+ effective-shared effective-diffs))
         (raw-similarity (if (> total 0)
                           (exact->inexact (/ shared-count total))
                           0.0))
         (effective-similarity (if (> effective-total 0)
                                 (exact->inexact
                                   (/ effective-shared effective-total))
                                 0.0))
         (weighted-cost
           (apply + (map (lambda (d) (diff-weight (car d))) collapsed)))
         (value-diffs
           (filter-map
             (lambda (d)
               (and (eq? (car d) 'literal-value)
                    (cons (caddr d) (cadddr d))))
             collapsed))
         (unique-value-params (unique value-diffs)))
    (list raw-similarity
          effective-similarity
          (+ (length roots) (length unique-value-params))
          weighted-cost
          roots
          unique-value-params
          collapsed
          derived-count)))

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
  (let* ((shared-count (car result))
         (diff-count (cadr result))
         (diffs (caddr result))
         (score (score-diffs shared-count diff-count diffs))
         (eff-sim (list-ref score 1))
         (roots (list-ref score 4))
         (collapsed (list-ref score 6))
         (derived-count (list-ref score 7)))
    (if (>= eff-sim similarity-threshold)
      (let ((rvi (make-root-var-index roots)))
        (reset-template-flags!)
        (let* ((template (build-template func-a func-b rvi))
               (flags (reverse template-flags))
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

(define candidate-count 0)

;; For each ordered pair (i, j) where i < j, different packages, same shape.
(let ((n (length all-funcs))
      (v (list->vector all-funcs)))
  (let outer ((i 0))
    (if (< i n)
      (begin
        (let ((entry-a (vector-ref v i)))
          (let inner ((j (+ i 1)))
            (if (< j n)
              (begin
                (let ((entry-b (vector-ref v j)))
                  ;; Same shape, different package.
                  (if (and (equal? (list-ref entry-a 3)
                                   (list-ref entry-b 3))
                           (not (equal? (cadr entry-a) (cadr entry-b))))
                    (let* ((func-a (caddr entry-a))
                           (func-b (caddr entry-b))
                           (label-a (car entry-a))
                           (label-b (car entry-b))
                           (result (ast-diff func-a func-b
                                     (list (string->symbol
                                             (string-append
                                               (car entry-a) "/"
                                               (car entry-b)))))))
                      (if (display-comparison label-a label-b
                                              func-a func-b result)
                        (set! candidate-count
                              (+ candidate-count 1))))))
                (inner (+ j 1))))))
        (outer (+ i 1))))))

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
