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

;;; boolean-simplify.scm — Boolean projections for predicates and Go AST conditions
;;;
;;; The core normalizer (Boolean equational-theory rewriting) lives in
;;; (wile algebra symbolic) as `symbolic-boolean-normalize` /
;;; `symbolic-boolean-equivalent?`. This file re-binds those under the
;;; shorter internal names `boolean-normalize` / `boolean-equivalent?` for
;;; the Go-specific projection layer below, which converts belief selector
;;; expressions and Go AST condition nodes into symbolic boolean terms.

;;; ── Re-bind upstream normalizer ─────────────────────────

(define (boolean-normalize term)
  "Normalize a boolean S-expression under the Boolean equational theory.\n\nThin goast-layer alias for `symbolic-boolean-normalize' from\n(wile algebra symbolic). Exposed under this name for use by the\nGo-specific projection layer (`selector->symbolic',\n`ast-condition->symbolic') and related belief machinery.\n\nReturns two values: the canonical normal form and the rewrite trace.\n\nParameters:\n  term : any\nReturns: any (two values)\nCategory: goast-boolean\n\nExamples:\n  (boolean-normalize '(and x (or x y)))  ; absorption => x\n  (boolean-normalize '(not (not x)))     ; involution => x\n\nSee also: `boolean-equivalent?', `selector->symbolic',\n`ast-condition->symbolic', `symbolic-boolean-normalize'."
  (symbolic-boolean-normalize term))

(define (boolean-equivalent? term1 term2)
  "Test whether two boolean S-expression terms share a canonical form.\n\nThin goast-layer alias for `symbolic-boolean-equivalent?' from\n(wile algebra symbolic). Both terms are normalized via\n`boolean-normalize' and compared with `equal?'.\n\nEquivalence is decided up to the axioms exposed by the underlying\nBoolean theory (commutativity, associativity, identity, idempotence,\nabsorption, involution) — NOT full Boolean-algebra equivalence.\n\nParameters:\n  term1 : any\n  term2 : any\nReturns: boolean\nCategory: goast-boolean\n\nExamples:\n  (boolean-equivalent? '(and a b) '(and b a))      ; => #t\n  (boolean-equivalent? '(and x (or x y)) 'x)       ; => #t\n\nSee also: `boolean-normalize', `symbolic-boolean-equivalent?'."
  (symbolic-boolean-equivalent? term1 term2))

;;; ── Belief selector projection ──────────────────────────

;; Project a quoted belief selector predicate expression into a
;; symbolic boolean term suitable for normalization.
;;
;; Compound selectors map to boolean operators:
;;   (all-of p1 p2 ...)   -> (and p1' p2' ...)  (nested binary)
;;   (any-of p1 p2 ...)   -> (or p1' p2' ...)
;;   (none-of p1 p2 ...)  -> (not (or p1' p2' ...))
;;
;; Atomic selectors map to opaque terms:
;;   (contains-call "F")       -> (calls "F")
;;   (has-params "T" ...)      -> (has-params "T" ...)
;;   (has-receiver "T")        -> (has-receiver "T")
;;   (name-matches "P")        -> (name-matches "P")
;;   (stores-to-fields ...)    -> (stores-to ...)
;;   anything else             -> kept as-is (opaque)
(define (selector->symbolic expr)
  "Project a quoted belief selector expression into a symbolic boolean term.\n\nParameters:\n  expr : list\nReturns: any\nCategory: goast-boolean\n\nExamples:\n  (selector->symbolic '(all-of (contains-call \"Lock\") (contains-call \"Unlock\")))\n  ; => (and (calls \"Lock\") (calls \"Unlock\"))\n\nSee also: `boolean-equivalent?', `boolean-normalize'."
  (if (not (pair? expr)) expr
    (let ((head (car expr)))
      (cond
        ;; Compound boolean combinators — require at least one predicate
        ((eq? head 'all-of)
         (let ((args (map selector->symbolic (cdr expr))))
           (cond ((null? args) (error "selector->symbolic: (all-of) requires at least one predicate"))
                 ((= (length args) 1) (car args))
                 (else (let loop ((rest (cdr args)) (acc (car args)))
                         (if (null? rest) acc
                           (loop (cdr rest) (list 'and acc (car rest)))))))))
        ((eq? head 'any-of)
         (let ((args (map selector->symbolic (cdr expr))))
           (cond ((null? args) (error "selector->symbolic: (any-of) requires at least one predicate"))
                 ((= (length args) 1) (car args))
                 (else (let loop ((rest (cdr args)) (acc (car args)))
                         (if (null? rest) acc
                           (loop (cdr rest) (list 'or acc (car rest)))))))))
        ((eq? head 'none-of)
         (if (null? (cdr expr))
           (error "selector->symbolic: (none-of) requires at least one predicate")
           (list 'not (selector->symbolic (cons 'any-of (cdr expr))))))
        ;; Atomic selectors — require argument, fall through to opaque if missing
        ((eq? head 'contains-call)
         (if (null? (cdr expr)) expr
           (list 'calls (cadr expr))))
        ((eq? head 'has-params)
         (cons 'has-params (cdr expr)))
        ((eq? head 'has-receiver)
         (if (null? (cdr expr)) expr
           (list 'has-receiver (cadr expr))))
        ((eq? head 'name-matches)
         (if (null? (cdr expr)) expr
           (list 'name-matches (cadr expr))))
        ((eq? head 'stores-to-fields)
         (cons 'stores-to (cdr expr)))
        ;; Unknown -> opaque
        (else expr)))))

;;; ── Go AST condition projection ─────────────────────────

;; Project a Go AST condition expression (from go-parse-file/go-parse-string)
;; into a symbolic boolean term.
;;
;; Go AST boolean operators:
;;   (binary-expr (op . &&) (x . left) (y . right))  -> (and left' right')
;;   (binary-expr (op . ||) (x . left) (y . right))  -> (or left' right')
;;   (unary-expr  (op . !)  (x . operand))            -> (not operand')
;;
;; Comparison operators become opaque atoms:
;;   (binary-expr (op . ==) (x . a) (y . b))          -> (eq a' b')
;;   (binary-expr (op . !=) (x . a) (y . b))          -> (neq a' b')
;;   (binary-expr (op . <)  (x . a) (y . b))          -> (lt a' b')
;;   etc.
;;
;; All other AST nodes become opaque atoms (identifiers, literals, etc.).
(define (ast-condition->symbolic node)
  "Project a Go AST condition expression into a symbolic boolean term.\n\nParameters:\n  node : any\nReturns: any\nCategory: goast-boolean\n\nExamples:\n  (ast-condition->symbolic (go-parse-expr \"x != nil && (x != nil || y > 0)\"))\n\nSee also: `boolean-normalize', `boolean-equivalent?'."
  (if (not (pair? node)) node
    (let ((tag (car node)))
      (cond
        ((eq? tag 'binary-expr)
         (let ((op (nf node 'op))
               (x (nf node 'x))
               (y (nf node 'y)))
           ;; Guard: if any required field is missing, treat as opaque
           (if (not (and op x y)) node
             (let ((op-name (if (symbol? op) (symbol->string op) "")))
               (cond
                 ((string=? op-name "&&") (list 'and
                                            (ast-condition->symbolic x)
                                            (ast-condition->symbolic y)))
                 ((string=? op-name "||") (list 'or
                                            (ast-condition->symbolic x)
                                            (ast-condition->symbolic y)))
                 ;; Comparisons -> opaque atoms preserving structure
                 ((string=? op-name "==") (list 'eq (ast-condition->symbolic x) (ast-condition->symbolic y)))
                 ((string=? op-name "!=") (list 'neq (ast-condition->symbolic x) (ast-condition->symbolic y)))
                 ((string=? op-name "<")  (list 'lt (ast-condition->symbolic x) (ast-condition->symbolic y)))
                 ((string=? op-name ">")  (list 'gt (ast-condition->symbolic x) (ast-condition->symbolic y)))
                 ((string=? op-name "<=") (list 'le (ast-condition->symbolic x) (ast-condition->symbolic y)))
                 ((string=? op-name ">=") (list 'ge (ast-condition->symbolic x) (ast-condition->symbolic y)))
                 ;; Other binary ops -> opaque
                 (else (list op (ast-condition->symbolic x) (ast-condition->symbolic y))))))))
        ((eq? tag 'unary-expr)
         (let ((op (nf node 'op))
               (x (nf node 'x)))
           (if (not (and op x)) node
             (let ((op-name (if (symbol? op) (symbol->string op) "")))
               (if (string=? op-name "!")
                 (list 'not (ast-condition->symbolic x))
                 (list op (ast-condition->symbolic x)))))))
        ;; Identifiers
        ((eq? tag 'ident)
         (let ((name (nf node 'name)))
           (if name (string->symbol name) node)))
        ;; Literals: (lit (kind . INT) (value . "0")) -> the value string as symbol
        ((eq? tag 'lit)
         (let ((val (nf node 'value)))
           (if val (string->symbol val) node)))
        ;; Paren expression — unwrap
        ((eq? tag 'paren-expr)
         (let ((x (nf node 'x)))
           (if x (ast-condition->symbolic x) node)))
        ;; Everything else -> opaque
        (else node)))))
