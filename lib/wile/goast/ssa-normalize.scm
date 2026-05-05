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

(define ssa-named-idempotence
  (list
    (make-named-axiom "idempotent-and" "x & x = x" (make-idempotence-axiom '&))
    (make-named-axiom "idempotent-or"  "x | x = x" (make-idempotence-axiom '|\||))))

(define ssa-named-absorption
  (list
    (make-named-axiom "absorption-or-and"
                      "x | (x & y) = x"
                      (make-absorption-axiom '|\|| '&))
    (make-named-axiom "absorption-and-or"
                      "x & (x | y) = x"
                      (make-absorption-axiom '& '|\||))))

(define ssa-named-associativity
  (list
    (make-named-axiom "associative-add" "(a + b) + c = a + (b + c)" (make-associativity-axiom '+))
    (make-named-axiom "associative-mul" "(a * b) * c = a * (b * c)" (make-associativity-axiom '*))
    (make-named-axiom "associative-and" "(a & b) & c = a & (b & c)" (make-associativity-axiom '&))
    (make-named-axiom "associative-or"  "(a | b) | c = a | (b | c)" (make-associativity-axiom '|\||))
    (make-named-axiom "associative-xor" "(a ^ b) ^ c = a ^ (b ^ c)" (make-associativity-axiom '^))))

;; ─── Theory (new export) ───────────────────────

(define ssa-theory
  (make-theory
    (append ssa-named-identity
            ssa-named-absorbing
            ssa-named-commutative
            ssa-named-idempotence
            ssa-named-absorption
            ssa-named-associativity)
    '(+ * & |\|| ^ == !=)))

;; ─── Raw axiom lists for flat normalizers ──────
;; (backward compatible — extracted from named axioms)

(define int-identity-theory (map named-axiom-axiom ssa-named-identity))
(define int-absorbing-theory (map named-axiom-axiom ssa-named-absorbing))
(define comm-theory (map named-axiom-axiom ssa-named-commutative))
(define int-idempotence-theory (map named-axiom-axiom ssa-named-idempotence))
(define int-absorption-theory (map named-axiom-axiom ssa-named-absorption))
(define int-associativity-theory (map named-axiom-axiom ssa-named-associativity))

;; ─── Normalizers from theories ──────────────

(define int-identity-rewrite
  (make-normalizer int-identity-theory ssa-binop-protocol))

(define int-absorbing-rewrite
  (make-normalizer int-absorbing-theory ssa-binop-protocol))

(define comm-rewrite
  (make-normalizer comm-theory ssa-binop-protocol))

(define int-idempotence-rewrite
  (make-normalizer int-idempotence-theory ssa-binop-protocol))

(define int-absorption-rewrite
  (make-normalizer int-absorption-theory ssa-binop-protocol))

(define int-associativity-rewrite
  (make-normalizer int-associativity-theory ssa-binop-protocol))

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

(define (ssa-rule-idempotence)
  "Construct a normalization rule for idempotent operations.\nRewrites x&x->x, x|x->x for integer types.\nReturns a rule: (lambda (node) -> node-or-#f).\n\nReturns: procedure\nCategory: goast-ssa-normalize\n\nSee also: `ssa-rule-absorption', `ssa-rule-associativity', `ssa-rule-set'."
  (lambda (node)
    (and (tag? node 'ssa-binop)
         (integer-type? (nf node 'type))
         (int-idempotence-rewrite node))))

(define (ssa-rule-absorption)
  "Construct a normalization rule for absorption laws.\nRewrites x|(x&y)->x and x&(x|y)->x for integer types.\nMulti-operator axiom: matches when the outer and inner ops differ\nand the outer term shares an operand with the inner term.\nReturns a rule: (lambda (node) -> node-or-#f).\n\nReturns: procedure\nCategory: goast-ssa-normalize\n\nSee also: `ssa-rule-idempotence', `ssa-rule-set'."
  (lambda (node)
    (and (tag? node 'ssa-binop)
         (integer-type? (nf node 'type))
         (int-absorption-rewrite node))))

(define (ssa-rule-associativity)
  "Construct a normalization rule for associative operations.\nRewrites (a+b)+c -> a+(b+c) (right-associative canonical form) for\n+, *, &, |, ^ on integer types. IEEE 754 floats are excluded because\nfloat +/* are not associative. Directional axiom — rewrites in one\ndirection only. May rarely fire in practice because SSA construction\npre-normalizes left-associative chains during IR build.\nReturns a rule: (lambda (node) -> node-or-#f).\n\nReturns: procedure\nCategory: goast-ssa-normalize\n\nSee also: `ssa-rule-idempotence', `ssa-rule-absorption', `ssa-rule-set'."
  (lambda (node)
    (and (tag? node 'ssa-binop)
         (integer-type? (nf node 'type))
         (int-associativity-rewrite node))))

;; Compose rules: first non-#f result wins
(define (ssa-rule-set . rules)
  "Compose multiple normalization rules into one.\nApplies rules in order; first non-#f result wins.\n\nParameters:\n  rules : procedure\nReturns: procedure\nCategory: goast-ssa-normalize\n\nExamples:\n  (ssa-rule-set (ssa-rule-identity) (ssa-rule-commutative))\n\nSee also: `ssa-normalize'."
  (lambda (node)
    (let loop ((rs rules))
      (if (null? rs) #f
        (let ((result ((car rs) node)))
          (if result result
            (loop (cdr rs))))))))

;; Default rule set: simplifiers first, then canonicalizers.
;; identity → absorbing → idempotence → absorption → commutativity → associativity
(define default-rules
  (ssa-rule-set
    (ssa-rule-identity)
    (ssa-rule-annihilation)
    (ssa-rule-idempotence)
    (ssa-rule-absorption)
    (ssa-rule-commutative)
    (ssa-rule-associativity)))

;; Main entry point — case-lambda for optional rules argument.
;; When rules produce a non-#f result (a replacement), that's the normalized form.
;; When rules return #f (no normalization needed), return the original node.
(define ssa-normalize
  (case-lambda
    ((node)
     "Normalize an SSA binop node using default algebraic rules.\nWith one arg, applies identity + annihilation + commutativity rules.\nWith two args, applies the given rule set instead.\n\nParameters:\n  node : list\nReturns: any\nCategory: goast-ssa-normalize\n\nExamples:\n  (ssa-normalize binop-node)\n  (ssa-normalize binop-node (ssa-rule-set (ssa-rule-identity)))\n\nSee also: `ssa-rule-set', `ssa-rule-identity', `go-ssa-canonicalize'."
     (let ((r (default-rules node))) (if r r node)))
    ((node rules) (let ((r (rules node))) (if r r node)))))
