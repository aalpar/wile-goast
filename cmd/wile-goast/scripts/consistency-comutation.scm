;;; consistency-comutation.scm — Co-mutation consistency analysis
;;;
;;; Engler-style consistency detection for struct field mutations.
;;; Finds field pairs that are almost always stored together,
;;; then reports functions that break the co-mutation pattern.
;;;
;;; Uses two goast layers:
;;;   Pass 0 (AST):  Enumerate structs and their fields
;;;   Pass 1 (SSA):  Per-function store sets for each struct
;;;   Pass 2:        Co-mutation pair analysis + deviation detection
;;;
;;; Usage: ./dist/wile-goast -f examples/goast-query/consistency-comutation.scm

;; ── Target ───────────────────────────────────────────────
(define target "github.com/aalpar/wile/machine")

;; ── Shared utilities ─────────────────────────────────────
;; (same as state-trace-full.scm — reuse verbatim)

(define (nf node key)
  (let ((e (assoc key (cdr node))))
    (if e (cdr e) #f)))

(define (tag? node t)
  (and (pair? node) (eq? (car node) t)))

(define (filter-map f lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (let ((v (f (car xs))))
        (loop (cdr xs) (if v (cons v acc) acc))))))

(define (flat-map f lst)
  (apply append (map f lst)))

(define (walk val visitor)
  (cond
    ((not (pair? val)) '())
    ((symbol? (car val))
     (let ((here (visitor val))
           (children (flat-map
                       (lambda (kv)
                         (if (pair? kv) (walk (cdr kv) visitor) '()))
                       (cdr val))))
       (if here (cons here children) children)))
    ((pair? (car val))
     (flat-map (lambda (child) (walk child visitor)) val))
    (else '())))

(define (member? x lst)
  (cond ((null? lst) #f)
        ((equal? x (car lst)) #t)
        (else (member? x (cdr lst)))))

(define (unique lst)
  (let loop ((xs lst) (seen '()))
    (cond ((null? xs) (reverse seen))
          ((member? (car xs) seen) (loop (cdr xs) seen))
          (else (loop (cdr xs) (cons (car xs) seen))))))

(define (has-char? s c)
  (let loop ((i 0))
    (cond ((>= i (string-length s)) #f)
          ((char=? (string-ref s i) c) #t)
          (else (loop (+ i 1))))))

;; Generate all ordered pairs from a list (each unordered pair once).
(define (ordered-pairs lst)
  (if (null? lst) '()
    (append
      (map (lambda (b) (list (car lst) b)) (cdr lst))
      (ordered-pairs (cdr lst)))))

;; ══════════════════════════════════════════════════════════
;; Pass 0: Struct Field Enumeration (AST layer)
;;
;; Find all named struct types and collect their field names.
;; Unlike state-trace, we collect ALL fields, not just booleans.
;; Returns: ((struct-name (field-name ...)) ...)
;; ══════════════════════════════════════════════════════════

(define (all-field-names field-node)
  (if (tag? field-node 'field)
    (let ((ns (nf field-node 'names)))
      (if (pair? ns) ns '()))
    '()))

(define (find-struct-fields file-ast)
  (walk file-ast
    (lambda (node)
      (and (tag? node 'type-spec)
           (let ((stype (nf node 'type)))
             (and (tag? stype 'struct-type)
                  (let* ((fields (nf stype 'fields))
                         (names (flat-map all-field-names
                                  (if (pair? fields) fields '()))))
                    ;; Only interesting if struct has 2+ fields
                    (and (>= (length names) 2)
                         (list (nf node 'name) names)))))))))

;; ══════════════════════════════════════════════════════════
;; Pass 1: Per-Function Store Sets (SSA layer)
;;
;; For each struct, find which functions store to which fields.
;; Reuses state-trace's field-addr/store join logic, but
;; without the boolean-cluster filter.
;;
;; Returns: ((func-name (stored-field ...)) ...)
;;          for functions that store to at least 1 field.
;; ══════════════════════════════════════════════════════════

(define (collect-field-addrs ssa-func)
  (walk ssa-func
    (lambda (node)
      (and (tag? node 'ssa-field-addr)
           (list (nf node 'name)
                 (nf node 'field)
                 (nf node 'x))))))

(define (collect-stores ssa-func)
  (walk ssa-func
    (lambda (node)
      (and (tag? node 'ssa-store)
           (list (nf node 'addr)
                 (nf node 'val))))))

;; Receiver-type disambiguation: a single receiver in SSA may access
;; fields of only ONE struct type. If a receiver accesses any field
;; NOT in the target struct, it belongs to a different struct —
;; exclude its field-addrs from the count.
;;
;; This eliminates false positives from field name collisions
;; (e.g., SchemeError.Source vs ErrExceptionEscape.Source).
(define (stored-fields-in-func ssa-func struct-fields)
  (let* ((all-field-addrs (collect-field-addrs ssa-func))
         (stores (collect-stores ssa-func))
         (store-addrs (map car stores))
         ;; Group field-addrs by receiver (x operand)
         (receivers (unique (map caddr all-field-addrs)))
         ;; A receiver is valid for this struct if EVERY field it
         ;; accesses is in struct-fields. If any field is foreign,
         ;; this receiver belongs to a different struct type.
         (valid-receivers
           (filter-map
             (lambda (recv)
               (let* ((recv-fas (filter-map
                                  (lambda (fa) (and (equal? (caddr fa) recv) fa))
                                  all-field-addrs))
                      (recv-fields (unique (map cadr recv-fas)))
                      (all-match (let loop ((fs recv-fields))
                                   (cond ((null? fs) #t)
                                         ((not (member? (car fs) struct-fields)) #f)
                                         (else (loop (cdr fs)))))))
                 (and all-match recv)))
             receivers))
         ;; Only count stores through valid receivers
         (stored (filter-map
                   (lambda (fa)
                     (let ((reg (car fa))
                           (field (cadr fa))
                           (recv (caddr fa)))
                       (and (member? recv valid-receivers)
                            (member? reg store-addrs)
                            (member? field struct-fields)
                            field)))
                   all-field-addrs)))
    (unique stored)))

;; Build per-function store sets for one struct across all SSA functions.
;; Filters out compiler-generated functions (contain '$').
;; Returns: ((func-name (field ...)) ...) — only functions that store >= 1 field.
(define (build-store-sets ssa-funcs struct-fields)
  (filter-map
    (lambda (fn)
      (let* ((fname (nf fn 'name))
             (stored (stored-fields-in-func fn struct-fields)))
        (and (pair? stored)
             (not (has-char? fname #\$))
             (list fname stored))))
    ssa-funcs))

;; ══════════════════════════════════════════════════════════
;; Pass 2: Co-Mutation Pair Analysis
;;
;; For each pair of fields (a, b) in a struct, count:
;;   - functions that store both a and b       (co-mutated)
;;   - functions that store a but not b        (a-only)
;;   - functions that store b but not a        (b-only)
;;
;; A strong co-mutation belief: high co-mutation count
;; relative to a-only + b-only, with enough total sites.
;;
;; Deviations: functions in the a-only or b-only sets
;; when co-mutation is the overwhelming majority.
;; ══════════════════════════════════════════════════════════

;; Count co-mutation statistics for one field pair.
;; Returns: (field-a field-b co-count a-only-count b-only-count
;;           co-funcs a-only-funcs b-only-funcs)
(define (pair-stats field-a field-b store-sets)
  (let loop ((sets store-sets)
             (co '()) (a-only '()) (b-only '()))
    (if (null? sets)
      (list field-a field-b
            (length co) (length a-only) (length b-only)
            (reverse co) (reverse a-only) (reverse b-only))
      (let* ((entry (car sets))
             (fname (car entry))
             (fields (cadr entry))
             (has-a (member? field-a fields))
             (has-b (member? field-b fields)))
        (cond
          ((and has-a has-b)
           (loop (cdr sets) (cons fname co) a-only b-only))
          ((and has-a (not has-b))
           (loop (cdr sets) co (cons fname a-only) b-only))
          ((and (not has-a) has-b)
           (loop (cdr sets) co a-only (cons fname b-only)))
          (else
           (loop (cdr sets) co a-only b-only)))))))

;; ── TODO: This is the core design choice ─────────────────
;;
;; score-comutation-belief decides whether a field pair's
;; co-mutation pattern is strong enough to report deviations.
;;
;; Input: a pair-stats result:
;;   (field-a field-b co-count a-only-count b-only-count
;;    co-funcs a-only-funcs b-only-funcs)
;;
;; Return: #f if the belief is too weak to report,
;;         or a list of deviation entries to display.
;;
;; Trade-offs to consider:
;;   - Minimum total sites (co + a-only + b-only).
;;     Too low → noise from structs with few mutators.
;;     Too high → misses real patterns in small packages.
;;
;;   - Adherence threshold (co-count / total).
;;     0.90 is conservative: only reports when 90%+ co-mutate.
;;     0.75 catches more but risks intentional variations
;;     (e.g., an initializer that only sets field-a).
;;
;;   - Should a-only and b-only deviations be weighted equally?
;;     If field-a is the "primary" and field-b is the "derived"
;;     (like a value + valid flag), then b-only is more suspect
;;     than a-only. But that asymmetry is hard to detect
;;     mechanically — it's a semantic property.
;;
;; ──────────────────────────────────────────────────────────
(define min-adherence 2/3)  ;; 66% — from data: captures Debugger stepping fields
(define min-sites 3)        ;; minimum co + a-only + b-only

(define (score-comutation-belief stats)
  (let* ((field-a (list-ref stats 0))
         (field-b (list-ref stats 1))
         (co (list-ref stats 2))
         (a-only (list-ref stats 3))
         (b-only (list-ref stats 4))
         (co-funcs (list-ref stats 5))
         (a-only-funcs (list-ref stats 6))
         (b-only-funcs (list-ref stats 7))
         (total (+ co a-only b-only))
         (deviations (+ a-only b-only)))
    (and (>= total min-sites)
         (> deviations 0)
         (>= (/ co total) min-adherence)
         (append
           (map (lambda (f) (cons f (string-append "only " field-a))) a-only-funcs)
           (map (lambda (f) (cons f (string-append "only " field-b))) b-only-funcs)))))

;; ══════════════════════════════════════════════════════════
;; Main
;; ══════════════════════════════════════════════════════════

(display "Loading and type-checking ") (display target) (display " ...")
(newline)
(define pkgs (go-typecheck-package target))

(display "Building SSA for ") (display target) (display " ...")
(newline)
(define ssa-funcs (go-ssa-build target))
(newline)

(display "══════════════════════════════════════════════════")
(newline)
(display "  Co-Mutation Consistency Analysis                ")
(newline)
(display "══════════════════════════════════════════════════")
(newline) (newline)

;; ── Pass 0 ───────────────────────────────────────────────
(display "── Pass 0: Struct Field Enumeration (AST) ──") (newline)
(define all-structs '())
(for-each
  (lambda (pkg)
    (for-each
      (lambda (file)
        (for-each
          (lambda (s)
            (set! all-structs (cons s all-structs))
            (display "  struct ") (display (car s))
            (display ": fields ") (display (cadr s))
            (newline))
          (find-struct-fields file)))
      (nf pkg 'files)))
  pkgs)
(if (null? all-structs) (begin (display "  (none found)") (newline)))
(newline)

;; ── Pass 1 ───────────────────────────────────────────────
(display "── Pass 1: Per-Function Store Sets (SSA) ──") (newline)
(define all-store-data '())
(for-each
  (lambda (s)
    (let* ((struct-name (car s))
           (fields (cadr s))
           (store-sets (build-store-sets ssa-funcs fields)))
      (set! all-store-data (cons (list struct-name fields store-sets) all-store-data))
      (if (pair? store-sets)
        (begin
          (display "  struct ") (display struct-name) (display ":") (newline)
          (for-each
            (lambda (entry)
              (display "    ") (display (car entry))
              (display " stores: ") (display (cadr entry))
              (newline))
            store-sets)))))
  all-structs)
(newline)

;; ── Pass 2 ───────────────────────────────────────────────
(display "── Pass 2: Co-Mutation Pair Analysis ──") (newline)
(define deviation-count 0)
(for-each
  (lambda (data)
    (let* ((struct-name (car data))
           (fields (cadr data))
           (store-sets (caddr data))
           (pairs (ordered-pairs fields)))
      ;; Analyze each field pair
      (for-each
        (lambda (pair)
          (let* ((stats (pair-stats (car pair) (cadr pair) store-sets))
                 (result (score-comutation-belief stats)))
            (if result
              (begin
                (set! deviation-count (+ deviation-count (length result)))
                (display "  struct ") (display struct-name)
                (display ": (") (display (car pair))
                (display ", ") (display (cadr pair)) (display ")")
                (newline)
                (let ((co (list-ref stats 2))
                      (a-only (list-ref stats 3))
                      (b-only (list-ref stats 4)))
                  (display "    co-mutated: ") (display co)
                  (display "  ") (display (car pair)) (display "-only: ")
                  (display a-only)
                  (display "  ") (display (cadr pair)) (display "-only: ")
                  (display b-only)
                  (newline))
                (for-each
                  (lambda (d)
                    (display "    DEVIATION: ") (display (car d))
                    (display " stores ") (display (cdr d))
                    (newline))
                  result)))))
        pairs)))
  all-store-data)
(if (= deviation-count 0) (begin (display "  (no strong beliefs / no deviations)") (newline)))
(newline)

;; ── Summary ──────────────────────────────────────────────
(display "── Summary ──") (newline)
(display "  Structs analyzed:    ") (display (length all-structs)) (newline)
(display "  Deviations found:    ") (display deviation-count) (newline)
