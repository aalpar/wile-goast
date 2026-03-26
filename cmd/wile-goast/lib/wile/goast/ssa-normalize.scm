;;; ssa-normalize.scm — algebraic normalization of SSA binop nodes
;;;
;;; Declares algebraic properties for SSA binary operators and delegates
;;; rule generation to (wile algebra rewrite). Type-scoped to integer
;;; types for identity/absorbing (IEEE 754 safety). Commutativity is
;;; type-agnostic.

;; ─── Domain predicates ──────────────────────

(define integer-types
  '("int" "int8" "int16" "int32" "int64"
    "uint" "uint8" "uint16" "uint32" "uint64" "uintptr"))

(define (integer-type? s)
  (and (string? s) (member s integer-types) #t))

;; SSA constant strings: "0", "0:int", "0:uint64", etc.
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

;; ─── Term protocol for SSA binop nodes ──────

(define ssa-binop-protocol
  (make-term-protocol
    ;; compound-term?: tagged alist nodes are compound terms
    (lambda (x) (and (pair? x) (symbol? (car x))))
    ;; get-operator
    (lambda (node) (nf node 'op))
    ;; get-operands
    (lambda (node) (list (nf node 'x) (nf node 'y)))
    ;; make-term: preserve all fields, replace operands
    (lambda (node new-args)
      (cons 'ssa-binop
            (map (lambda (pair)
                   (cond
                     ((eq? (car pair) 'x) (cons 'x (car new-args)))
                     ((eq? (car pair) 'y) (cons 'y (cadr new-args)))
                     ((eq? (car pair) 'operands)
                      (cons 'operands new-args))
                     (else pair)))
                 (cdr node))))
    ;; compare: lexicographic on strings, #f for non-strings
    (lambda (a b) (and (string? a) (string? b) (string<? a b)))))

;; ─── Theory declarations ────────────────────

(define int-identity-theory
  (list (make-identity-axiom '+ constant-zero?)
        (make-identity-axiom '* constant-one?)
        (make-identity-axiom '|\|| constant-zero?)
        (make-identity-axiom '^ constant-zero?)))

(define int-absorbing-theory
  (list (make-absorbing-axiom '* constant-zero?)
        (make-absorbing-axiom '& constant-zero?)))

(define comm-theory
  (list (make-commutativity-axiom '+)
        (make-commutativity-axiom '*)
        (make-commutativity-axiom '&)
        (make-commutativity-axiom '|\||)
        (make-commutativity-axiom '^)
        (make-commutativity-axiom '==)
        (make-commutativity-axiom '!=)))

;; ─── Normalizers from theories ──────────────

(define int-identity-rewrite
  (make-normalizer int-identity-theory ssa-binop-protocol))

(define int-absorbing-rewrite
  (make-normalizer int-absorbing-theory ssa-binop-protocol))

(define comm-rewrite
  (make-normalizer comm-theory ssa-binop-protocol))

;; ─── Public API (backward-compatible) ───────

;; Each rule constructor returns (node → value-or-#f).
;; Identity and absorbing guard on integer type; commutativity is type-agnostic.

(define (ssa-rule-identity)
  (lambda (node)
    (and (tag? node 'ssa-binop)
         (integer-type? (nf node 'type))
         (int-identity-rewrite node))))

(define (ssa-rule-annihilation)
  (lambda (node)
    (and (tag? node 'ssa-binop)
         (integer-type? (nf node 'type))
         (int-absorbing-rewrite node))))

(define (ssa-rule-commutative)
  (lambda (node)
    (and (tag? node 'ssa-binop)
         (comm-rewrite node))))

;; Compose rules: first non-#f result wins
(define (ssa-rule-set . rules)
  (lambda (node)
    (let loop ((rs rules))
      (if (null? rs) #f
        (let ((result ((car rs) node)))
          (if result result
            (loop (cdr rs))))))))

;; Default rule set: identity → absorbing → commutativity
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
