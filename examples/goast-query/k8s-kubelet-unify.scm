;;; k8s-kubelet-unify.scm — Structural duplication in kubelet subpackages
;;;
;;; Usage: cd /path/to/kubernetes && wile-goast -f k8s-kubelet-unify.scm

;; ══════════════════════════════════════════════════════════
;; Configuration
;; ══════════════════════════════════════════════════════════

;; Cluster 1: runtime-related packages
;; Cluster 2: lifecycle/management packages
;; Also include root kubelet + status for cross-cutting patterns
(define targets
  '("k8s.io/kubernetes/pkg/kubelet/kuberuntime"
    "k8s.io/kubernetes/pkg/kubelet/container"
    "k8s.io/kubernetes/pkg/kubelet/images"
    "k8s.io/kubernetes/pkg/kubelet/lifecycle"
    "k8s.io/kubernetes/pkg/kubelet/prober"
    "k8s.io/kubernetes/pkg/kubelet/status"
    "k8s.io/kubernetes/pkg/kubelet/pleg"
    "k8s.io/kubernetes/pkg/kubelet/eviction"))

(define similarity-threshold 0.60)
(define min-body-size 3)

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

(define (filter pred lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (loop (cdr xs)
            (if (pred (car xs)) (cons (car xs) acc) acc)))))

(define (filter-map f lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (let ((v (f (car xs))))
        (loop (cdr xs) (if v (cons v acc) acc))))))

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
;; Classification (must precede diff engine)
;; ══════════════════════════════════════════════════════════

(define identifier-fields '(name sel label))

(define type-fields '(type inferred-type asserted-type obj-pkg
                      signature))

(define (in-type-position? path)
  (let loop ((xs path))
    (cond
      ((null? xs) #f)
      ((null? (cdr xs)) #f)
      ((and (symbol? (car xs)) (memq (car xs) type-fields)) #t)
      (else (loop (cdr xs))))))

(define (classify-string-diff str-a str-b path)
  (let ((field (if (null? path) #f (last-element path))))
    (cond
      ((and (symbol? field) (memq field type-fields)) 'type-name)
      ((and (symbol? field) (eq? field 'name)
            (in-type-position? path)) 'type-name)
      ((and (symbol? field) (memq field identifier-fields)) 'identifier)
      ((and (symbol? field) (eq? field 'value)) 'literal-value)
      ((and (symbol? field) (eq? field 'tok)) 'operator)
      (else 'identifier))))

;; ══════════════════════════════════════════════════════════
;; Core diff engine
;; ══════════════════════════════════════════════════════════

(define (merge-results a b)
  (list (+ (car a) (car b))
        (+ (cadr a) (cadr b))
        (append (caddr a) (caddr b))))

(define (shared-result) (list 1 0 '()))

(define (diff-result category path val-a val-b)
  (list 0 1 (list (list category path val-a val-b))))

(define (merge-all results)
  (let loop ((rs results) (acc (list 0 0 '())))
    (if (null? rs) acc
      (loop (cdr rs) (merge-results acc (car rs))))))

(define (ast-diff node-a node-b path)
  (define (fields-diff fields-a fields-b fpath)
    (let ((results
            (filter-map
              (lambda (pair-a)
                (if (not (pair? pair-a)) #f
                  (let* ((key (car pair-a))
                         (val-a (cdr pair-a))
                         (entry-b (assoc key fields-b)))
                    (if entry-b
                      (ast-diff val-a (cdr entry-b)
                                (append fpath (list key)))
                      (diff-result 'missing-field
                                   (append fpath (list key))
                                   val-a #f)))))
              fields-a)))
      (let ((extra
              (filter-map
                (lambda (pair-b)
                  (if (not (pair? pair-b)) #f
                    (let ((key (car pair-b)))
                      (if (assoc key fields-a) #f
                        (diff-result 'extra-field
                                     (append fpath (list key))
                                     #f (cdr pair-b))))))
                fields-b)))
        (merge-all (append results extra)))))

  (define (list-diff lst-a lst-b lpath idx)
    (cond
      ((and (null? lst-a) (null? lst-b)) (list 0 0 '()))
      ((null? lst-b)
       (merge-all
         (map (lambda (ea)
                (diff-result 'extra-element (append lpath (list idx)) ea #f))
              lst-a)))
      ((null? lst-a)
       (merge-all
         (map (lambda (eb)
                (diff-result 'extra-element (append lpath (list idx)) #f eb))
              lst-b)))
      (else
        (merge-results
          (ast-diff (car lst-a) (car lst-b) (append lpath (list idx)))
          (list-diff (cdr lst-a) (cdr lst-b) lpath (+ idx 1))))))

  (cond
    ((and (tagged-node? node-a) (tagged-node? node-b))
     (if (eq? (car node-a) (car node-b))
       (merge-results (shared-result)
                      (fields-diff (cdr node-a) (cdr node-b) path))
       (diff-result 'structural path node-a node-b)))
    ((and (list? node-a) (list? node-b))
     (list-diff node-a node-b path 0))
    ((and (eq? node-a #f) (eq? node-b #f)) (shared-result))
    ((equal? node-a node-b) (shared-result))
    ((and (string? node-a) (string? node-b))
     (diff-result (classify-string-diff node-a node-b path)
                  path node-a node-b))
    ((and (symbol? node-a) (symbol? node-b))
     (diff-result 'operator path node-a node-b))
    ((and (boolean? node-a) (boolean? node-b))
     (diff-result 'literal-value path node-a node-b))
    ((and (number? node-a) (number? node-b))
     (diff-result 'literal-value path node-a node-b))
    (else (diff-result 'structural path node-a node-b))))

;; ══════════════════════════════════════════════════════════
;; Substitution collapsing
;; ══════════════════════════════════════════════════════════

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
            (else (search (+ i 1)))))))))

(define (apply-substitutions str roots)
  (let loop ((s str) (rs roots))
    (if (null? rs) s
      (loop (string-replace-all s (caar rs) (cdar rs))
            (cdr rs)))))

(define (derivable? val-a val-b roots)
  (equal? (apply-substitutions val-a roots) val-b))

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

(define (find-root-substitutions pairs)
  (let ((sorted (sort-by-length (unique pairs))))
    (let loop ((ps sorted) (roots '()))
      (if (null? ps) roots
        (let ((a (caar ps)) (b (cdar ps)))
          (if (derivable? a b roots)
            (loop (cdr ps) roots)
            (loop (cdr ps) (cons (cons a b) roots))))))))

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
;; Scoring
;; ══════════════════════════════════════════════════════════

(define diff-weights
  '((type-name . 1) (derived-type . 0) (literal-value . 1)
    (identifier . 0) (operator . 2) (structural . 100)
    (missing-field . 50) (extra-field . 50) (extra-element . 50)))

(define (diff-weight category)
  (let ((entry (assoc category diff-weights)))
    (if entry (cdr entry) 10)))

(define (score-diffs shared-count diff-count diffs)
  (let* ((type-pairs
           (filter-map
             (lambda (d)
               (and (eq? (car d) 'type-name)
                    (string? (caddr d)) (string? (cadddr d))
                    (cons (caddr d) (cadddr d))))
             diffs))
         (roots (find-root-substitutions type-pairs))
         (collapsed (collapse-diffs diffs roots))
         (derived-count (length (filter (lambda (d) (eq? (car d) 'derived-type))
                                        collapsed)))
         (effective-shared (+ shared-count derived-count))
         (effective-diffs (- diff-count derived-count))
         (effective-total (+ effective-shared effective-diffs))
         (effective-similarity (if (> effective-total 0)
                                 (exact->inexact
                                   (/ effective-shared effective-total))
                                 0.0))
         (weighted-cost
           (apply + (map (lambda (d) (diff-weight (car d))) collapsed))))
    (list effective-similarity weighted-cost roots collapsed derived-count)))

;; ══════════════════════════════════════════════════════════
;; Function extraction
;; ══════════════════════════════════════════════════════════

(define (package-funcs pkg)
  (flat-map
    (lambda (file)
      (filter-map
        (lambda (decl) (and (tag? decl 'func-decl) decl))
        (nf file 'decls)))
    (nf pkg 'files)))

(define (signature-shape func)
  (let* ((ftype (nf func 'type))
         (params (nf ftype 'params))
         (results (nf ftype 'results))
         (pc (if (and params (pair? params)) (length params) 0))
         (rc (if (and results (pair? results)) (length results) 0)))
    (cons pc rc)))

(define (body-size func)
  (let ((body (nf func 'body)))
    (if (and body (tag? body 'block))
      (let ((stmts (nf body 'list)))
        (if (pair? stmts) (length stmts) 0))
      0)))

;; ══════════════════════════════════════════════════════════
;; Reporting (compact — only show cost <= 50)
;; ══════════════════════════════════════════════════════════

(define (display-comparison label-a label-b func-a func-b result)
  (let* ((shared-count (car result))
         (diff-count (cadr result))
         (diffs (caddr result))
         (score (score-diffs shared-count diff-count diffs))
         (eff-sim (list-ref score 0))
         (weighted-cost (list-ref score 1))
         (roots (list-ref score 2)))
    (if (and (>= eff-sim similarity-threshold)
             (<= weighted-cost 50))
      (begin
        (newline)
        (display "  ") (display label-a)
        (display "  <->  ") (display label-b) (newline)
        (display "    similarity:  ")
        (display (exact->inexact (/ (round (* eff-sim 1000)) 1000)))
        (newline)
        (display "    cost:        ") (display weighted-cost) (newline)
        (display "    type params: ") (display (length roots)) (newline)
        (for-each
          (lambda (r)
            (display "      ") (display (car r))
            (display " -> ") (display (cdr r)) (newline))
          roots)
        (newline)
        #t)
      #f)))

;; ══════════════════════════════════════════════════════════
;; Main
;; ══════════════════════════════════════════════════════════

(display "══════════════════════════════════════════════════")
(newline)
(display "  kubelet Structural Duplication Detection")
(newline)
(display "══════════════════════════════════════════════════")
(newline) (newline)

(define all-pkgs
  (flat-map
    (lambda (target)
      (display "  Loading ") (display target) (display " ...")
      (let ((pkgs (go-typecheck-package target)))
        (display " ") (display (length pkgs)) (display " pkg(s)")
        (newline)
        pkgs))
    targets))

(display "  Total: ") (display (length all-pkgs))
(display " packages") (newline) (newline)

(define all-funcs
  (flat-map
    (lambda (pkg)
      (let ((pkg-name (nf pkg 'name)))
        (filter-map
          (lambda (func)
            (let ((name (nf func 'name)))
              (and (>= (body-size func) min-body-size)
                   (list (string-append pkg-name "." name)
                         pkg-name func
                         (signature-shape func)))))
          (package-funcs pkg))))
    all-pkgs))

(display "  ") (display (length all-funcs))
(display " functions (>= ") (display min-body-size)
(display " statements)") (newline) (newline)

(let ((pkg-names (unique (map cadr all-funcs))))
  (for-each
    (lambda (pn)
      (let ((count (length (filter (lambda (e) (equal? (cadr e) pn))
                                   all-funcs))))
        (display "    ") (display pn) (display ": ")
        (display count) (newline)))
    pkg-names))
(newline)

;; Cross-package comparison (cost <= 50 filter in display)

(display "══════════════════════════════════════════════════")
(newline)
(display "  Cross-package candidates (cost <= 50)")
(newline)
(display "══════════════════════════════════════════════════")
(newline)

(define candidate-count 0)
(define comparisons 0)

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
                  (if (and (equal? (list-ref entry-a 3)
                                   (list-ref entry-b 3))
                           (not (equal? (cadr entry-a) (cadr entry-b))))
                    (begin
                      (set! comparisons (+ comparisons 1))
                      (let* ((func-a (caddr entry-a))
                             (func-b (caddr entry-b))
                             (result (ast-diff func-a func-b
                                       (list (string->symbol
                                               (string-append
                                                 (car entry-a) "/"
                                                 (car entry-b)))))))
                        (if (display-comparison (car entry-a) (car entry-b)
                                                func-a func-b result)
                          (set! candidate-count
                                (+ candidate-count 1)))))))
                (inner (+ j 1))))))
        (outer (+ i 1))))))

(if (= candidate-count 0)
  (begin (display "  (no candidates above threshold)") (newline)))

(newline)
(display "── Summary ──") (newline)
(display "  Packages:     ") (display (length all-pkgs)) (newline)
(display "  Functions:    ") (display (length all-funcs)) (newline)
(display "  Comparisons:  ") (display comparisons) (newline)
(display "  Candidates:   ") (display candidate-count)
(display " (cost <= 50)") (newline)
