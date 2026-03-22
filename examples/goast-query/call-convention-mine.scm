;;; call-convention-mine.scm — Statistical call-convention discovery
;;;
;;; For each receiver type in a Go package, computes per-type callee
;;; frequency and reports deviations from majority patterns.
;;;
;;; A "call convention" is a callee that the majority of methods on a
;;; receiver type invoke. When 80% of methods on *Machine call
;;; m.checkInterrupt(), the 20% that don't are worth investigating —
;;; they may be missing a required check.
;;;
;;; Pure AST analysis — no SSA or CFG needed.
;;;
;;; Usage: wile-goast -f examples/goast-query/call-convention-mine.scm

(import (wile goast utils))

;; ── Configuration ────────────────────────────────────────
(define target "github.com/aalpar/wile/machine")
(define min-frequency 0.60)
(define min-sites 5)
(define min-type-methods 5)

;; ══════════════════════════════════════════════════════════
;; Pass 0: Inventory — group methods by receiver type
;;
;; Extract receiver type name from each func-decl.
;; Group methods by type. Filter to types with enough methods.
;; ══════════════════════════════════════════════════════════

;; Extract receiver type name from a func-decl node.
;; Handles *Foo, Foo, and Foo[T] (generic) receivers.
;; Returns string or #f.
(define (receiver-type-name func)
  (let ((recv (nf func 'recv)))
    (and recv (pair? recv)
         (let* ((recv-field (car recv))
                (recv-type (nf recv-field 'type))
                ;; Strip pointer: *Foo -> Foo
                (base-type (if (tag? recv-type 'star-expr)
                             (nf recv-type 'x)
                             recv-type)))
           (cond
             ;; Plain type: Foo
             ((tag? base-type 'ident)
              (nf base-type 'name))
             ;; Generic type: Foo[T] -> (index-expr (x ident (name . "Foo")))
             ((tag? base-type 'index-expr)
              (let ((x (nf base-type 'x)))
                (and (tag? x 'ident) (nf x 'name))))
             ;; Generic with multiple params: Foo[T, U] -> index-list-expr
             ((tag? base-type 'index-list-expr)
              (let ((x (nf base-type 'x)))
                (and (tag? x 'ident) (nf x 'name))))
             (else #f))))))

;; Group func-decls by receiver type name.
;; Returns: ((type-name func-decl ...) ...)
;; Uses set-cdr! to accumulate methods into groups.
(define (group-by-receiver func-decls)
  (let ((groups '()))
    (for-each
      (lambda (func)
        (let ((tname (receiver-type-name func)))
          (if tname
            (let ((entry (assoc tname groups)))
              (if entry
                (set-cdr! entry (cons func (cdr entry)))
                (set! groups (cons (list tname func) groups)))))))
      func-decls)
    ;; Filter to types with >= min-type-methods
    (filter-map
      (lambda (group)
        (and (>= (length (cdr group)) min-type-methods)
             group))
      groups)))

;; ══════════════════════════════════════════════════════════
;; Pass 1: Call Extraction
;;
;; For each method, extract the set of unique callee names.
;; ══════════════════════════════════════════════════════════

;; Extract callee name from a call-expr node.
;; fun is either ident (return name) or selector-expr (return sel).
;; Returns string or #f.
(define (callee-name call-node)
  (let ((fun (nf call-node 'fun)))
    (cond
      ((tag? fun 'ident)
       (nf fun 'name))
      ((tag? fun 'selector-expr)
       (nf fun 'sel))
      (else #f))))

;; Extract unique callee names from a function body.
(define (extract-callees func)
  (let ((body (nf func 'body)))
    (if body
      (unique
        (walk body
          (lambda (node)
            (and (tag? node 'call-expr)
                 (callee-name node)))))
      '())))

;; Build call sets for a list of methods.
;; Returns: ((method-name (callee ...)) ...)
(define (build-call-sets methods)
  (filter-map
    (lambda (func)
      (let* ((name (nf func 'name))
             (callees (extract-callees func)))
        (and (pair? callees)
             (list name callees))))
    methods))

;; ══════════════════════════════════════════════════════════
;; Pass 2: Convention Discovery
;;
;; For each type, count how many methods call each callee.
;; A convention: callee called by >= min-frequency of methods
;; with >= min-sites total call sites.
;; ══════════════════════════════════════════════════════════

;; Insertion sort by count descending.
(define (insert-by-count entry sorted)
  (cond
    ((null? sorted) (list entry))
    ((>= (cadr entry) (cadr (car sorted)))
     (cons entry sorted))
    (else (cons (car sorted) (insert-by-count entry (cdr sorted))))))

;; Count how many call-sets contain each callee.
;; Returns: ((callee-name count frequency) ...) sorted by count desc.
(define (callee-frequencies call-sets)
  (let ((n (length call-sets)))
    (if (= n 0) '()
      (let ((counts '()))
        ;; Tally: for each callee in each method, increment count
        (for-each
          (lambda (cs)
            (let ((callees (cadr cs)))
              (for-each
                (lambda (callee)
                  (let ((entry (assoc callee counts)))
                    (if entry
                      (set-cdr! entry (+ (cdr entry) 1))
                      (set! counts (cons (cons callee 1) counts)))))
                callees)))
          call-sets)
        ;; Build (callee count frequency) triples, sorted by count desc
        (let loop ((cs counts) (sorted '()))
          (if (null? cs) sorted
            (let* ((callee (caar cs))
                   (count (cdar cs))
                   (freq (exact->inexact (/ count n)))
                   (entry (list callee count freq)))
              (loop (cdr cs) (insert-by-count entry sorted)))))))))

;; Filter frequencies to those meeting both thresholds.
(define (find-conventions freqs)
  (filter-map
    (lambda (entry)
      (let ((count (cadr entry))
            (freq (caddr entry)))
        (and (>= freq min-frequency)
             (>= count min-sites)
             entry)))
    freqs))

;; ══════════════════════════════════════════════════════════
;; Pass 3: Deviation Report
;;
;; For each convention, find methods that don't call it.
;; ══════════════════════════════════════════════════════════

;; Word-wrap a list of names at a given column width.
;; Returns a list of strings, each fitting within width.
(define (wrap-names names width)
  (let loop ((ns names) (line "") (lines '()))
    (cond
      ((null? ns)
       (reverse (if (> (string-length line) 0)
                  (cons line lines)
                  lines)))
      (else
       (let* ((name (car ns))
              (sep (if (> (string-length line) 0) ", " ""))
              (candidate (string-append line sep name)))
         (if (and (> (string-length line) 0)
                  (> (string-length candidate) width))
           ;; Start a new line
           (loop ns "" (cons line lines))
           (loop (cdr ns) candidate lines)))))))

;; ══════════════════════════════════════════════════════════
;; Main
;; ══════════════════════════════════════════════════════════

(display "Loading and type-checking ") (display target) (display " ...")
(newline)
(define pkgs (go-typecheck-package target))
(newline)

(display "==================================================")
(newline)
(display "  Call Convention Mining                           ")
(newline)
(display "==================================================")
(newline) (newline)

;; ── Pass 0: Inventory ────────────────────────────────────
(display "-- Pass 0: Method Inventory (AST) --") (newline)

;; Extract all func-decls with bodies from all packages/files.
(define all-func-decls
  (flat-map
    (lambda (pkg)
      (flat-map
        (lambda (file)
          (filter-map
            (lambda (decl)
              (and (tag? decl 'func-decl)
                   (nf decl 'body)
                   decl))
            (nf file 'decls)))
        (nf pkg 'files)))
    pkgs))

(define type-groups (group-by-receiver all-func-decls))

(for-each
  (lambda (group)
    (display "  ") (display (car group))
    (display ": ") (display (length (cdr group)))
    (display " methods") (newline))
  type-groups)
(if (null? type-groups)
  (begin (display "  (no types with >= ")
         (display min-type-methods)
         (display " methods)") (newline)))
(newline)

;; ── Pass 1: Call Extraction ──────────────────────────────
(display "-- Pass 1: Call Extraction (AST) --") (newline)

(define type-call-data '())
(for-each
  (lambda (group)
    (let* ((tname (car group))
           (methods (cdr group))
           (call-sets (build-call-sets methods))
           (total-edges (apply + (map (lambda (cs) (length (cadr cs))) call-sets))))
      (set! type-call-data
        (cons (list tname methods call-sets) type-call-data))
      (display "  ") (display tname)
      (display ": ") (display (length call-sets))
      (display " methods, ") (display total-edges)
      (display " call edges") (newline)))
  type-groups)
(newline)

;; ── Pass 2: Convention Discovery ─────────────────────────
(display "-- Pass 2: Convention Discovery --") (newline)

(define type-conventions '())
(for-each
  (lambda (data)
    (let* ((tname (car data))
           (call-sets (caddr data))
           (freqs (callee-frequencies call-sets))
           (conventions (find-conventions freqs)))
      (set! type-conventions
        (cons (list tname call-sets conventions) type-conventions))
      (if (pair? conventions)
        (begin
          (display "  ") (display tname) (display ":") (newline)
          (for-each
            (lambda (conv)
              (let ((callee (car conv))
                    (count (cadr conv))
                    (freq (caddr conv)))
                (display "    ") (display callee)
                (display "  ") (display count)
                (display "/") (display (length call-sets))
                (display " (")
                ;; Display percentage with 1 decimal
                (let ((pct (exact->inexact (* freq 100))))
                  (display (truncate pct))
                  (display "%"))
                (display ")") (newline)))
            conventions))
        (begin
          (display "  ") (display tname)
          (display ": (no conventions above threshold)")
          (newline)))))
  type-call-data)
(newline)

;; ── Pass 3: Deviation Report ─────────────────────────────
(display "-- Pass 3: Deviations --") (newline)

(define total-deviations 0)
(for-each
  (lambda (tc)
    (let* ((tname (car tc))
           (call-sets (cadr tc))
           (conventions (caddr tc)))
      (if (pair? conventions)
        (begin
          (display "  ") (display tname) (display ":") (newline)
          (for-each
            (lambda (conv)
              (let* ((callee (car conv))
                     (count (cadr conv))
                     ;; Find methods that DON'T call this callee
                     (violators
                       (filter-map
                         (lambda (cs)
                           (let ((method-name (car cs))
                                 (callees (cadr cs)))
                             (and (not (member? callee callees))
                                  method-name)))
                         call-sets))
                     (n-violators (length violators)))
                (if (> n-violators 0)
                  (begin
                    (set! total-deviations
                      (+ total-deviations n-violators))
                    (display "    ") (display callee)
                    (display " — ") (display n-violators)
                    (display " methods skip this call:")
                    (newline)
                    (let ((wrapped (wrap-names violators 60)))
                      (for-each
                        (lambda (line)
                          (display "      ") (display line)
                          (newline))
                        wrapped))))))
            conventions)))))
  type-conventions)
(if (= total-deviations 0)
  (begin (display "  (no deviations found)") (newline)))
(newline)

;; ── Summary ──────────────────────────────────────────────
(display "-- Summary --") (newline)
(display "  Types analyzed:      ")
(display (length type-groups)) (newline)
(display "  Conventions found:   ")
(display (apply + (map (lambda (tc) (length (caddr tc))) type-conventions)))
(newline)
(display "  Total deviations:    ")
(display total-deviations) (newline)
(display "  Thresholds:          ")
(display "frequency >= ") (display min-frequency)
(display ", sites >= ") (display min-sites)
(display ", type methods >= ") (display min-type-methods)
(newline)
