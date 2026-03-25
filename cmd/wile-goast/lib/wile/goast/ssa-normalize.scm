;;; ssa-normalize.scm — algebraic normalization of SSA binop nodes
;;;
;;; Normalizes SSA binary operations by applying algebraic rules:
;;; commutative operand sorting, identity elimination, and annihilation.
;;; Rules return a replacement value (string for register/constant, or
;;; rebuilt ssa-binop node) when they fire, #f when they don't apply.

;; Commutative ops — safe to reorder operands
(define commutative-ops '(+ * & |\|| ^ == !=))

;; Predicate: is this an integer type string?
;; Go SSA represents byte as uint8 and rune as int32, so those
;; aliases are covered by the base types in this list.
(define integer-types
  '("int" "int8" "int16" "int32" "int64"
    "uint" "uint8" "uint16" "uint32" "uint64" "uintptr"))

(define (integer-type? s)
  (and (string? s) (member s integer-types) #t))

;; Constant matching for SSA constant strings like "0:int" or just "0"
(define (constant-zero? s)
  (and (string? s)
       (or (string=? s "0")
           (and (> (string-length s) 1)
                (char=? (string-ref s 0) #\0)
                (char=? (string-ref s 1) #\:)))))

(define (constant-one? s)
  (and (string? s)
       (or (string=? s "1")
           (and (> (string-length s) 1)
                (char=? (string-ref s 0) #\1)
                (char=? (string-ref s 1) #\:)))))

;; Rule: commutative operand sorting
;; For commutative ops, ensure x <= y lexicographically
(define (ssa-rule-commutative)
  (lambda (node)
    (if (not (tag? node 'ssa-binop)) #f
      (let ((op (nf node 'op))
            (x (nf node 'x))
            (y (nf node 'y)))
        (if (and op x y
                 (symbol? op)
                 (memq op commutative-ops)
                 (string? x) (string? y)
                 (string>? x y))
          ;; Swap x and y, rebuild operands too
          (cons 'ssa-binop
                (map (lambda (pair)
                       (cond
                         ((eq? (car pair) 'x) (cons 'x y))
                         ((eq? (car pair) 'y) (cons 'y x))
                         ((eq? (car pair) 'operands)
                          (cons 'operands (list y x)))
                         (else pair)))
                     (cdr node)))
          #f)))))

;; Rule: identity elimination (integer types only)
;; x + 0 -> x, x * 1 -> x, x | 0 -> x, x ^ 0 -> x
;; Skips float types (IEEE 754: -0.0 + 0.0 = 0.0, not -0.0)
(define (ssa-rule-identity)
  (lambda (node)
    (if (not (tag? node 'ssa-binop)) #f
      (let ((op (nf node 'op))
            (x (nf node 'x))
            (y (nf node 'y))
            (typ (nf node 'type)))
        (if (not (and op (symbol? op) (string? x) (string? y)
                      (integer-type? typ)))
          #f
          (cond
            ;; x + 0 -> x, 0 + x -> x
            ((and (eq? op '+)
                  (constant-zero? y)) x)
            ((and (eq? op '+)
                  (constant-zero? x)) y)
            ;; x * 1 -> x, 1 * x -> x
            ((and (eq? op '*)
                  (constant-one? y)) x)
            ((and (eq? op '*)
                  (constant-one? x)) y)
            ;; x | 0 -> x, 0 | x -> x
            ((and (eq? op '|\||)
                  (constant-zero? y)) x)
            ((and (eq? op '|\||)
                  (constant-zero? x)) y)
            ;; x ^ 0 -> x, 0 ^ x -> x
            ((and (eq? op '^)
                  (constant-zero? y)) x)
            ((and (eq? op '^)
                  (constant-zero? x)) y)
            (else #f)))))))

;; Rule: annihilation (integer types only)
;; x * 0 -> 0, x & 0 -> 0
(define (ssa-rule-annihilation)
  (lambda (node)
    (if (not (tag? node 'ssa-binop)) #f
      (let ((op (nf node 'op))
            (x (nf node 'x))
            (y (nf node 'y))
            (typ (nf node 'type)))
        (if (not (and op (symbol? op) (string? x) (string? y)
                      (integer-type? typ)))
          #f
          (cond
            ;; x * 0 -> 0, 0 * x -> 0  (return the zero constant)
            ((and (eq? op '*)
                  (constant-zero? y)) y)
            ((and (eq? op '*)
                  (constant-zero? x)) x)
            ;; x & 0 -> 0, 0 & x -> 0
            ((and (eq? op '&)
                  (constant-zero? y)) y)
            ((and (eq? op '&)
                  (constant-zero? x)) x)
            (else #f)))))))

;; Compose rules: first non-#f result wins
(define (ssa-rule-set . rules)
  (lambda (node)
    (let loop ((rs rules))
      (if (null? rs) #f
        (let ((result ((car rs) node)))
          (if result result
            (loop (cdr rs))))))))

;; Default rule set
(define default-rules
  (ssa-rule-set
    (ssa-rule-identity)
    (ssa-rule-annihilation)
    (ssa-rule-commutative)))

;; Main entry point — case-lambda for optional rules argument.
;; When rules produce a non-#f result (a replacement), that's the normalized form.
;; When rules return #f (no normalization needed), return the original node.
(define ssa-normalize
  (case-lambda
    ((node) (let ((r (default-rules node))) (if r r node)))
    ((node rules) (let ((r (rules node))) (if r r node)))))
