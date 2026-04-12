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

;; ─── Named axiom declarations ──────────────────
;;
;; Named axioms are the source of truth. Raw axioms for flat normalizers
;; are extracted via named-axiom-axiom.

(define ssa-named-identity
  (list
    (make-named-axiom "identity-add" "x + 0 = x" (make-identity-axiom '+ constant-zero?))
    (make-named-axiom "identity-mul" "x * 1 = x" (make-identity-axiom '* constant-one?))
    (make-named-axiom "identity-or"  "x | 0 = x" (make-identity-axiom '|\|| constant-zero?))
    (make-named-axiom "identity-xor" "x ^ 0 = x" (make-identity-axiom '^ constant-zero?))))

(define ssa-named-absorbing
  (list
    (make-named-axiom "absorbing-mul" "x * 0 = 0" (make-absorbing-axiom '* constant-zero?))
    (make-named-axiom "absorbing-and" "x & 0 = 0" (make-absorbing-axiom '& constant-zero?))))

(define ssa-named-commutative
  (list
    (make-named-axiom "commutative-add" "x + y = y + x" (make-commutativity-axiom '+))
    (make-named-axiom "commutative-mul" "x * y = y * x" (make-commutativity-axiom '*))
    (make-named-axiom "commutative-and" "x & y = y & x" (make-commutativity-axiom '&))
    (make-named-axiom "commutative-or"  "x | y = y | x" (make-commutativity-axiom '|\||))
    (make-named-axiom "commutative-xor" "x ^ y = y ^ x" (make-commutativity-axiom '^))
    (make-named-axiom "commutative-eq"  "x == y = y == x" (make-commutativity-axiom '==))
    (make-named-axiom "commutative-ne"  "x != y = y != x" (make-commutativity-axiom '!=))))

;; ─── Theory (new export) ───────────────────────

(define ssa-theory
  (make-theory
    (append ssa-named-identity ssa-named-absorbing ssa-named-commutative)
    '(+ * & |\|| ^ == !=)))

;; ─── Raw axiom lists for flat normalizers ──────
;; (backward compatible — extracted from named axioms)

(define int-identity-theory (map named-axiom-axiom ssa-named-identity))
(define int-absorbing-theory (map named-axiom-axiom ssa-named-absorbing))
(define comm-theory (map named-axiom-axiom ssa-named-commutative))

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
  "Construct a normalization rule for identity operations.\nRewrites x+0->x, x*1->x, x|0->x, x^0->x for integer types.\nReturns a rule: (lambda (node) -> node-or-#f).\n\nReturns: procedure\nCategory: goast-ssa-normalize\n\nSee also: `ssa-rule-annihilation', `ssa-rule-set', `ssa-normalize'."
  (lambda (node)
    (and (tag? node 'ssa-binop)
         (integer-type? (nf node 'type))
         (int-identity-rewrite node))))

(define (ssa-rule-annihilation)
  "Construct a normalization rule for absorbing operations.\nRewrites x*0->0, x&0->0 for integer types.\nReturns a rule: (lambda (node) -> node-or-#f).\n\nReturns: procedure\nCategory: goast-ssa-normalize\n\nSee also: `ssa-rule-identity', `ssa-rule-set', `ssa-normalize'."
  (lambda (node)
    (and (tag? node 'ssa-binop)
         (integer-type? (nf node 'type))
         (int-absorbing-rewrite node))))

(define (ssa-rule-commutative)
  "Construct a normalization rule for commutative operations.\nSorts operands lexicographically so a+b and b+a produce identical output.\nReturns a rule: (lambda (node) -> node-or-#f).\n\nReturns: procedure\nCategory: goast-ssa-normalize\n\nSee also: `ssa-rule-identity', `ssa-rule-set', `ssa-normalize'."
  (lambda (node)
    (and (tag? node 'ssa-binop)
         (comm-rewrite node))))

;; Compose rules: first non-#f result wins
(define (ssa-rule-set . rules)
  "Compose multiple normalization rules into one.\nApplies rules in order; first non-#f result wins.\n\nParameters:\n  rules : procedure\nReturns: procedure\nCategory: goast-ssa-normalize\n\nExamples:\n  (ssa-rule-set (ssa-rule-identity) (ssa-rule-commutative))\n\nSee also: `ssa-normalize'."
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
    ((node)
     "Normalize an SSA binop node using default algebraic rules.\nWith one arg, applies identity + annihilation + commutativity rules.\nWith two args, applies the given rule set instead.\n\nParameters:\n  node : list\nReturns: any\nCategory: goast-ssa-normalize\n\nExamples:\n  (ssa-normalize binop-node)\n  (ssa-normalize binop-node (ssa-rule-set (ssa-rule-identity)))\n\nSee also: `ssa-rule-set', `ssa-rule-identity', `go-ssa-canonicalize'."
     (let ((r (default-rules node))) (if r r node)))
    ((node rules) (let ((r (rules node))) (if r r node)))))
