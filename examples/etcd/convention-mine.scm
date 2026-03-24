;;; convention-mine.scm — Statistical call-convention discovery for etcd
;;;
;;; For each receiver type with enough methods, discovers which callees
;;; appear in >= threshold% of methods, then reports deviations.
;;;
;;; Usage: cd etcd/server && wile-goast -f /path/to/convention-mine.scm

(import (wile goast utils))

;; ── Configuration ──────────────────────────────────────
(define target "go.etcd.io/etcd/server/v3/etcdserver")
(define convention-threshold 0.60)  ; callee in >= 60% of methods = convention
(define min-methods 5)              ; ignore types with fewer methods

;; ── Utilities ──────────────────────────────────────────

(define (filter pred lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (loop (cdr xs) (if (pred (car xs)) (cons (car xs) acc) acc)))))

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

(define (count pred lst)
  (let loop ((xs lst) (n 0))
    (if (null? xs) n
      (loop (cdr xs) (if (pred (car xs)) (+ n 1) n)))))

(define (sort-by-descending key lst)
  ;; Insertion sort — fine for small lists.
  (define (insert x sorted)
    (cond ((null? sorted) (list x))
          ((>= (key x) (key (car sorted))) (cons x sorted))
          (else (cons (car sorted) (insert x (cdr sorted))))))
  (let loop ((xs lst) (acc '()))
    (if (null? xs) acc
      (loop (cdr xs) (insert (car xs) acc)))))

;; ── AST extraction ─────────────────────────────────────

;; Extract receiver type name from a func-decl.
;; Handles *T (pointer receiver) and T (value receiver).
(define (receiver-type func)
  (let ((recv (nf func 'recv)))
    (and recv (pair? recv)
         (let* ((field (car recv))
                (ftype (nf field 'type)))
           (cond
             ;; *T — star-expr wrapping ident
             ((and (tag? ftype 'star-expr)
                   (tag? (nf ftype 'x) 'ident))
              (nf (nf ftype 'x) 'name))
             ;; T — plain ident
             ((tag? ftype 'ident)
              (nf ftype 'name))
             (else #f))))))

;; Collect all call-expression function names from a func-decl body.
;; Walks the entire AST looking for call-expr nodes.
(define (collect-callees func)
  (let ((body (nf func 'body)))
    (if (not body) '()
      (let ((calls '()))
        (walk body
          (lambda (node)
            (when (tag? node 'call-expr)
              (let ((fun (nf node 'fun)))
                (cond
                  ;; Direct call: foo(...)
                  ((tag? fun 'ident)
                   (set! calls (cons (nf fun 'name) calls)))
                  ;; Method/selector call: x.Foo(...)
                  ((tag? fun 'selector-expr)
                   (set! calls (cons (nf fun 'sel) calls))))))))
        (unique (reverse calls))))))

;; ── Load package ───────────────────────────────────────

(display "Loading ") (display target) (display " ...") (newline)
(define pkg (car (go-typecheck-package target)))
(display "  Package: ") (display (nf pkg 'name)) (newline)

;; Extract all func-decls across all files.
(define all-funcs
  (flat-map
    (lambda (file)
      (filter (lambda (d) (tag? d 'func-decl))
              (or (nf file 'decls) '())))
    (nf pkg 'files)))

(display "  Total functions: ") (display (length all-funcs)) (newline)

;; ── Group methods by receiver type ─────────────────────

;; Build alist: ((type-name . (func ...)) ...)
(define type-methods
  (let loop ((funcs all-funcs) (acc '()))
    (if (null? funcs) acc
      (let* ((f (car funcs))
             (rt (receiver-type f)))
        (loop (cdr funcs)
              (if (not rt) acc
                (let ((entry (assoc rt acc)))
                  (if entry
                    (begin (set-cdr! entry (cons f (cdr entry))) acc)
                    (cons (cons rt (list f)) acc)))))))))

;; Filter to types with enough methods.
(define large-types
  (filter (lambda (e) (>= (length (cdr e)) min-methods))
          type-methods))

(display "  Types with >= ") (display min-methods)
(display " methods: ") (display (length large-types)) (newline)

(for-each
  (lambda (e)
    (display "    ") (display (car e))
    (display ": ") (display (length (cdr e)))
    (display " methods") (newline))
  (sort-by-descending (lambda (e) (length (cdr e))) large-types))

(newline)

;; ── Mine conventions per type ──────────────────────────

(for-each
  (lambda (type-entry)
    (let* ((type-name (car type-entry))
           (methods (cdr type-entry))
           (n (length methods))
           (min-count (inexact->exact (ceiling (* convention-threshold n))))
           ;; For each method, compute its callee set.
           (method-callees
             (map (lambda (m)
                    (cons (nf m 'name) (collect-callees m)))
                  methods))
           ;; Collect all unique callees across all methods.
           (all-callees (unique (flat-map cdr method-callees)))
           ;; For each callee, count how many methods call it.
           (callee-counts
             (filter-map
               (lambda (callee)
                 (let ((cnt (count (lambda (mc)
                                     (member callee (cdr mc)))
                                   method-callees)))
                   (and (>= cnt min-count)
                        (cons callee cnt))))
               all-callees))
           ;; Sort conventions by frequency (descending).
           (conventions
             (sort-by-descending cdr callee-counts)))

      (display "══════════════════════════════════════════════════")
      (newline)
      (display "  ") (display type-name)
      (display " (") (display n) (display " methods, threshold ")
      (display convention-threshold) (display ")")
      (newline)
      (display "══════════════════════════════════════════════════")
      (newline)

      (if (null? conventions)
        (begin
          (display "  No conventions at threshold ")
          (display convention-threshold) (newline) (newline))
        (begin
          ;; Show discovered conventions.
          (display "  Conventions:") (newline)
          (for-each
            (lambda (conv)
              (let ((pct (exact->inexact (/ (cdr conv) n))))
                (display "    ") (display (car conv))
                (display "  (") (display (cdr conv))
                (display "/") (display n)
                (display " = ")
                (display (exact->inexact
                           (/ (round (* pct 1000)) 1000)))
                (display ")") (newline)))
            conventions)
          (newline)

          ;; Show deviations: methods missing a convention callee.
          (display "  Deviations:") (newline)
          (let ((deviation-count 0))
            (for-each
              (lambda (mc)
                (let* ((method-name (car mc))
                       (callees (cdr mc))
                       (missing
                         (filter-map
                           (lambda (conv)
                             (and (not (member (car conv) callees))
                                  (car conv)))
                           conventions)))
                  (when (pair? missing)
                    (set! deviation-count (+ deviation-count 1))
                    (display "    ") (display method-name)
                    (display "  missing: ")
                    (display missing) (newline))))
              method-callees)
            (if (= deviation-count 0)
              (display "    (none — all methods follow all conventions)"))
            (newline))
          (newline)))))

  (sort-by-descending (lambda (e) (length (cdr e))) large-types))

(display "Done.") (newline)
