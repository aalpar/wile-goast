;;; boolean-simplify.scm — Boolean normalization for predicates and Go AST conditions
;;;
;;; Normalizes boolean S-expression terms using (wile algebra symbolic)'s
;;; recursive normalizer with a Boolean algebra theory. Supports two
;;; projection modes:
;;;   1. Belief selector expressions (quoted predicate combinators)
;;;   2. Go AST condition expressions (binary-expr/unary-expr nodes)

;;; ── Shared boolean algebra and normalizer ───────────────

;; Singleton boolean theory and normalizer, constructed lazily.
;; Uses a minimal powerset Boolean algebra — the specific universe
;; doesn't matter since we only use the equational theory (commutativity,
;; absorption, idempotence, involution), not ground-truth computation.

(define *bool-theory* #f)
(define *bool-normalizer* #f)

(define (atom-compare a b)
  ;; Lexicographic ordering for commutativity normalization.
  ;; Atoms are arbitrary S-expressions; convert to string for comparison.
  (let ((sa (let ((p (open-output-string))) (write a p) (get-output-string p)))
        (sb (let ((p (open-output-string))) (write b p) (get-output-string p))))
    (string<? sa sb)))

(define (ensure-normalizer!)
  (unless *bool-theory*
    (let* ((B (powerset-boolean '(_)))
           (th (boolean->theory B 'or 'and 'not))
           (proto (sexp-term-protocol atom-compare)))
      (set! *bool-theory* th)
      (set! *bool-normalizer* (make-recursive-normalizer th proto)))))

;;; ── Core normalization ──────────────────────────────────

;; Normalize a boolean S-expression term.
;; Returns two values: the normal form and the rewrite trace.
;; Terms use (and ...), (or ...), (not ...) as operators.
;; All other forms are treated as opaque atoms.
(define (boolean-normalize term)
  "Normalize a boolean S-expression under standard Boolean algebra laws.\nReturns two values: the normal form and the rewrite trace.\n\nParameters:\n  term : any\nReturns: any\nCategory: goast-boolean\n\nExamples:\n  (boolean-normalize '(and x (or x y)))  ; => x, (trace...)\n  (boolean-normalize '(not (not x)))      ; => x, (trace...)\n\nSee also: `boolean-equivalent?', `selector->symbolic'."
  (ensure-normalizer!)
  (*bool-normalizer* term))

;; Check if two boolean S-expression terms are equivalent
;; under Boolean algebra laws.
(define (boolean-equivalent? term1 term2)
  "Check if two boolean S-expression terms normalize to the same form.\n\nParameters:\n  term1 : any\n  term2 : any\nReturns: boolean\nCategory: goast-boolean\n\nExamples:\n  (boolean-equivalent? '(and a b) '(and b a))  ; => #t\n\nSee also: `boolean-normalize'."
  (ensure-normalizer!)
  (let-values (((n1 _t1) (*bool-normalizer* term1))
               ((n2 _t2) (*bool-normalizer* term2)))
    (equal? n1 n2)))

;;; ── Belief selector projection ──────────────────────────

;; Project a quoted belief selector predicate expression into a
;; symbolic boolean term suitable for normalization.
;;
;; Compound selectors map to boolean operators:
;;   (all-of p1 p2 ...)   → (and p1' p2' ...)  (nested binary)
;;   (any-of p1 p2 ...)   → (or p1' p2' ...)
;;   (none-of p1 p2 ...)  → (not (or p1' p2' ...))
;;
;; Atomic selectors map to opaque terms:
;;   (contains-call "F")       → (calls "F")
;;   (has-params "T" ...)      → (has-params "T" ...)
;;   (has-receiver "T")        → (has-receiver "T")
;;   (name-matches "P")        → (name-matches "P")
;;   (stores-to-fields ...)    → (stores-to ...)
;;   anything else             → kept as-is (opaque)
(define (selector->symbolic expr)
  "Project a quoted belief selector expression into a symbolic boolean term.\n\nParameters:\n  expr : list\nReturns: any\nCategory: goast-boolean\n\nExamples:\n  (selector->symbolic '(all-of (contains-call \"Lock\") (contains-call \"Unlock\")))\n  ; => (and (calls \"Lock\") (calls \"Unlock\"))\n\nSee also: `boolean-equivalent?', `boolean-normalize'."
  (if (not (pair? expr)) expr
    (let ((head (car expr)))
      (cond
        ;; Compound boolean combinators
        ((eq? head 'all-of)
         (let ((args (map selector->symbolic (cdr expr))))
           (if (= (length args) 1) (car args)
             (let loop ((rest (cdr args)) (acc (car args)))
               (if (null? rest) acc
                 (loop (cdr rest) (list 'and acc (car rest))))))))
        ((eq? head 'any-of)
         (let ((args (map selector->symbolic (cdr expr))))
           (if (= (length args) 1) (car args)
             (let loop ((rest (cdr args)) (acc (car args)))
               (if (null? rest) acc
                 (loop (cdr rest) (list 'or acc (car rest))))))))
        ((eq? head 'none-of)
         (list 'not (selector->symbolic (cons 'any-of (cdr expr)))))
        ;; Atomic selectors → named atoms
        ((eq? head 'contains-call)
         (list 'calls (cadr expr)))
        ((eq? head 'has-params)
         (cons 'has-params (cdr expr)))
        ((eq? head 'has-receiver)
         (list 'has-receiver (cadr expr)))
        ((eq? head 'name-matches)
         (list 'name-matches (cadr expr)))
        ((eq? head 'stores-to-fields)
         (cons 'stores-to (cdr expr)))
        ;; Unknown → opaque
        (else expr)))))

;;; ── Go AST condition projection ─────────────────────────

;; Project a Go AST condition expression (from go-parse-file/go-parse-string)
;; into a symbolic boolean term.
;;
;; Go AST boolean operators:
;;   (binary-expr (op . &&) (x . left) (y . right))  → (and left' right')
;;   (binary-expr (op . ||) (x . left) (y . right))  → (or left' right')
;;   (unary-expr  (op . !)  (x . operand))            → (not operand')
;;
;; Comparison operators become opaque atoms:
;;   (binary-expr (op . ==) (x . a) (y . b))          → (eq a' b')
;;   (binary-expr (op . !=) (x . a) (y . b))          → (neq a' b')
;;   (binary-expr (op . <)  (x . a) (y . b))          → (lt a' b')
;;   etc.
;;
;; All other AST nodes become opaque atoms (identifiers, literals, etc.).
(define (ast-condition->symbolic node)
  "Project a Go AST condition expression into a symbolic boolean term.\n\nParameters:\n  node : any\nReturns: any\nCategory: goast-boolean\n\nExamples:\n  (ast-condition->symbolic (go-parse-expr \"x != nil && (x != nil || y > 0)\"))\n\nSee also: `boolean-normalize', `boolean-equivalent?'."
  (if (not (pair? node)) node
    (let ((tag (car node)))
      (cond
        ((eq? tag 'binary-expr)
         (let* ((op (nf node 'op))
                (op-name (if (symbol? op) (symbol->string op) ""))
                (x (nf node 'x))
                (y (nf node 'y)))
           (cond
             ((string=? op-name "&&") (list 'and
                                        (ast-condition->symbolic x)
                                        (ast-condition->symbolic y)))
             ((string=? op-name "||") (list 'or
                                        (ast-condition->symbolic x)
                                        (ast-condition->symbolic y)))
             ;; Comparisons → opaque atoms preserving structure
             ((string=? op-name "==") (list 'eq (ast-condition->symbolic x) (ast-condition->symbolic y)))
             ((string=? op-name "!=") (list 'neq (ast-condition->symbolic x) (ast-condition->symbolic y)))
             ((string=? op-name "<")  (list 'lt (ast-condition->symbolic x) (ast-condition->symbolic y)))
             ((string=? op-name ">")  (list 'gt (ast-condition->symbolic x) (ast-condition->symbolic y)))
             ((string=? op-name "<=") (list 'le (ast-condition->symbolic x) (ast-condition->symbolic y)))
             ((string=? op-name ">=") (list 'ge (ast-condition->symbolic x) (ast-condition->symbolic y)))
             ;; Other binary ops → opaque
             (else (list op (ast-condition->symbolic x) (ast-condition->symbolic y))))))
        ((eq? tag 'unary-expr)
         (let* ((op (nf node 'op))
                (op-name (if (symbol? op) (symbol->string op) ""))
                (x (nf node 'x)))
           (if (string=? op-name "!")
             (list 'not (ast-condition->symbolic x))
             (list op (ast-condition->symbolic x)))))
        ;; Identifiers
        ((eq? tag 'ident)
         (let ((name (nf node 'name)))
           (if name (string->symbol name) node)))
        ;; Literals: (lit (kind . INT) (value . "0")) → the value string as symbol
        ((eq? tag 'lit)
         (let ((val (nf node 'value)))
           (if val (string->symbol val) node)))
        ;; Paren expression — unwrap
        ((eq? tag 'paren-expr)
         (ast-condition->symbolic (nf node 'x)))
        ;; Everything else → opaque
        (else node)))))
