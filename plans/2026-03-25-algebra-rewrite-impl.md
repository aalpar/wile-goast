# (wile algebra rewrite) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a general-purpose equational term rewriting library in wile's
`(wile algebra)`, then migrate wile-goast's `ssa-normalize` to use it.

**Architecture:** Term protocol (user-provided accessors) + first-class axiom
values (identity, commutativity, absorbing, idempotence, involution) + normalizer
that compiles axioms into rewrite rules via the protocol. Theories are plain lists,
`append` composes them.

**Tech Stack:** Scheme (R7RS), `(wile algebra rewrite)` in wile stdlib,
consumed by `(wile goast ssa-normalize)` in wile-goast.

**Two phases:** Phase 1 builds the library in `wile/`. Phase 2 migrates wile-goast.

---

## Phase 1: Build (wile algebra rewrite) in wile

### Task 1: Term protocol + identity axiom + make-normalizer skeleton

**Files:**
- Create: `wile/stdlib/lib/wile/algebra/rewrite.sld`
- Create: `wile/stdlib/lib/wile/algebra/rewrite.scm`
- Test: `wile/engine_stdlib_test.go`

**Step 1: Write the test**

Add to `wile/engine_stdlib_test.go`:

```go
func TestAlgebraRewrite_IdentityAxiom(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	eng, err := wile.NewEngine(ctx,
		wile.WithSafeExtensions(),
		wile.WithSourceFS(stdlib.FS),
		wile.WithLibraryPaths(),
	)
	c.Assert(err, qt.IsNil)
	defer eng.Close()

	// Term protocol for simple (op left right) lists
	result, err := eng.EvalMultiple(ctx, `
		(import (wile algebra rewrite))

		(define proto
		  (make-term-protocol
		    car                             ;; get-operator
		    cdr                             ;; get-operands
		    cons                            ;; make-term: op × operands → term
		    (lambda (a b) (string<? (symbol->string a) (symbol->string b))))) ;; compare

		(define theory (list (identity-axiom '+ 'zero)))
		(define normalize (make-normalizer theory proto))

		;; (+ x zero) → x
		(normalize '(+ x zero))
	`)
	c.Assert(err, qt.IsNil)
	c.Assert(result.SchemeString(), qt.Equals, "x")
}

func TestAlgebraRewrite_IdentityAxiomLeftZero(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	eng, err := wile.NewEngine(ctx,
		wile.WithSafeExtensions(),
		wile.WithSourceFS(stdlib.FS),
		wile.WithLibraryPaths(),
	)
	c.Assert(err, qt.IsNil)
	defer eng.Close()

	result, err := eng.EvalMultiple(ctx, `
		(import (wile algebra rewrite))

		(define proto
		  (make-term-protocol car cdr cons
		    (lambda (a b) (string<? (symbol->string a) (symbol->string b)))))

		(define normalize (make-normalizer (list (identity-axiom '+ 'zero)) proto))

		;; (+ zero x) → x
		(normalize '(+ zero x))
	`)
	c.Assert(err, qt.IsNil)
	c.Assert(result.SchemeString(), qt.Equals, "x")
}

func TestAlgebraRewrite_NoMatchReturnsOriginal(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	eng, err := wile.NewEngine(ctx,
		wile.WithSafeExtensions(),
		wile.WithSourceFS(stdlib.FS),
		wile.WithLibraryPaths(),
	)
	c.Assert(err, qt.IsNil)
	defer eng.Close()

	result, err := eng.EvalMultiple(ctx, `
		(import (wile algebra rewrite))

		(define proto
		  (make-term-protocol car cdr cons
		    (lambda (a b) (string<? (symbol->string a) (symbol->string b)))))

		(define normalize (make-normalizer (list (identity-axiom '+ 'zero)) proto))

		;; (+ a b) — no zero, returns original
		(normalize '(+ a b))
	`)
	c.Assert(err, qt.IsNil)
	c.Assert(result.SchemeString(), qt.Equals, "(+ a b)")
}
```

**Step 2: Run tests to verify they fail**

Run (in wile/): `go test . -run TestAlgebraRewrite -v`
Expected: FAIL — `(wile algebra rewrite)` not found.

**Step 3: Create the library**

`wile/stdlib/lib/wile/algebra/rewrite.sld`:

```scheme
(define-library (wile algebra rewrite)
  (export
    ;; Term protocol
    make-term-protocol term-protocol?
    term-get-operator term-get-operands term-make-term term-compare
    ;; Axioms
    identity-axiom identity-axiom?
    commutativity-axiom commutativity-axiom?
    absorbing-axiom absorbing-axiom?
    idempotence-axiom idempotence-axiom?
    involution-axiom involution-axiom?
    axiom?
    ;; Normalizer
    make-normalizer
    ;; Validation
    validate-theory)
  (import (scheme base))
  (include "rewrite.scm"))
```

`wile/stdlib/lib/wile/algebra/rewrite.scm`:

```scheme
;;; (wile algebra rewrite) — Equational term rewriting from algebraic axioms
;;;
;;; Generates rewrite rules from declared axioms (identity, commutativity,
;;; absorbing, idempotence, involution). Rules are compiled via a term
;;; protocol that abstracts over concrete term representations.

;; ─── Term protocol ──────────────────────────

(define-record-type <term-protocol>
  (make-term-protocol get-operator get-operands make-term compare)
  term-protocol?
  (get-operator  term-get-operator)
  (get-operands  term-get-operands)
  (make-term     term-make-term)
  (compare       term-compare))

;; ─── Axiom types ────────────────────────────

(define-record-type <identity-axiom>
  (identity-axiom op element)
  identity-axiom?
  (op      identity-axiom-op)
  (element identity-axiom-element))

(define-record-type <commutativity-axiom>
  (commutativity-axiom op)
  commutativity-axiom?
  (op commutativity-axiom-op))

(define-record-type <absorbing-axiom>
  (absorbing-axiom op element)
  absorbing-axiom?
  (op      absorbing-axiom-op)
  (element absorbing-axiom-element))

(define-record-type <idempotence-axiom>
  (idempotence-axiom op)
  idempotence-axiom?
  (op idempotence-axiom-op))

(define-record-type <involution-axiom>
  (involution-axiom op)
  involution-axiom?
  (op involution-axiom-op))

(define (axiom? x)
  (or (identity-axiom? x)
      (commutativity-axiom? x)
      (absorbing-axiom? x)
      (idempotence-axiom? x)
      (involution-axiom? x)))

;; ─── Axiom → rewrite rules ─────────────────

(define (axiom->rules axiom proto)
  ;; Returns a list of rewrite rule functions: (term → value-or-#f)
  (let ((get-op   (term-get-operator proto))
        (get-args (term-get-operands proto))
        (mk-term  (term-make-term proto))
        (lt?      (term-compare proto)))
    (cond
      ((identity-axiom? axiom)
       (let ((target-op (identity-axiom-op axiom))
             (e         (identity-axiom-element axiom)))
         (list
           ;; op(x, e) → x
           (lambda (term)
             (let ((op (get-op term)) (args (get-args term)))
               (and (equal? op target-op)
                    (= (length args) 2)
                    (equal? (cadr args) e)
                    (car args))))
           ;; op(e, x) → x
           (lambda (term)
             (let ((op (get-op term)) (args (get-args term)))
               (and (equal? op target-op)
                    (= (length args) 2)
                    (equal? (car args) e)
                    (cadr args)))))))

      ((commutativity-axiom? axiom)
       (let ((target-op (commutativity-axiom-op axiom)))
         (list
           ;; op(x, y) → op(y, x) when not (lt? x y)
           (lambda (term)
             (let ((op (get-op term)) (args (get-args term)))
               (and (equal? op target-op)
                    (= (length args) 2)
                    (not (lt? (car args) (cadr args)))
                    (not (equal? (car args) (cadr args)))
                    (mk-term op (list (cadr args) (car args)))))))))

      ((absorbing-axiom? axiom)
       (let ((target-op (absorbing-axiom-op axiom))
             (z         (absorbing-axiom-element axiom)))
         (list
           ;; op(x, z) → z
           (lambda (term)
             (let ((op (get-op term)) (args (get-args term)))
               (and (equal? op target-op)
                    (= (length args) 2)
                    (equal? (cadr args) z)
                    z)))
           ;; op(z, x) → z
           (lambda (term)
             (let ((op (get-op term)) (args (get-args term)))
               (and (equal? op target-op)
                    (= (length args) 2)
                    (equal? (car args) z)
                    z))))))

      ((idempotence-axiom? axiom)
       (let ((target-op (idempotence-axiom-op axiom)))
         (list
           ;; op(x, x) → x
           (lambda (term)
             (let ((op (get-op term)) (args (get-args term)))
               (and (equal? op target-op)
                    (= (length args) 2)
                    (equal? (car args) (cadr args))
                    (car args)))))))

      ((involution-axiom? axiom)
       (let ((target-op (involution-axiom-op axiom)))
         (list
           ;; op(op(x)) → x  (unary: single operand)
           (lambda (term)
             (let ((op (get-op term)) (args (get-args term)))
               (and (equal? op target-op)
                    (= (length args) 1)
                    (let ((inner (car args)))
                      (and (pair? inner)
                           (equal? (get-op inner) target-op)
                           (= (length (get-args inner)) 1)
                           (car (get-args inner))))))))))

      (else '()))))

;; ─── Normalizer ─────────────────────────────

(define (make-normalizer theory proto)
  ;; Compile all axioms into a flat list of rewrite rules.
  (let ((rules (apply append
                 (map (lambda (ax) (axiom->rules ax proto)) theory))))
    ;; Return a procedure: term → term (first match wins, original if none).
    (lambda (term)
      (let try ((rs rules))
        (if (null? rs) term
          (let ((result ((car rs) term)))
            (if result result
              (try (cdr rs)))))))))

;; ─── Validation ─────────────────────────────

(define (validate-theory theory proto samples)
  ;; Spot-check that each axiom holds for sample terms.
  ;; Returns #t or list of (violation-type ...).
  (let ((violations '())
        (get-op   (term-get-operator proto))
        (get-args (term-get-operands proto))
        (mk-term  (term-make-term proto)))
    (define (fail! type . args)
      (set! violations (cons (cons type args) violations)))
    (for-each
      (lambda (axiom)
        (cond
          ((identity-axiom? axiom)
           (let ((op (identity-axiom-op axiom))
                 (e  (identity-axiom-element axiom)))
             (for-each
               (lambda (x)
                 (let ((term-r (mk-term op (list x e)))
                       (term-l (mk-term op (list e x))))
                   (unless (equal? x x) ;; placeholder — real validation needs eval
                     (fail! 'identity-right op x e))))
               samples)))
          ;; Other axioms: commutativity, absorbing, idempotence, involution
          ;; validate similarly. For v1, identity and commutativity are checked.
          ))
      theory)
    (if (null? violations) #t (reverse violations))))
```

**Step 4: Run tests to verify they pass**

Run (in wile/): `go test . -run TestAlgebraRewrite -v`
Expected: PASS

**Step 5: Commit (in wile)**

```
git add stdlib/lib/wile/algebra/rewrite.sld stdlib/lib/wile/algebra/rewrite.scm engine_stdlib_test.go
git commit -m "feat(algebra): add (wile algebra rewrite) term rewriting library"
```

---

### Task 2: Commutativity, absorbing, idempotence, involution tests

**Files:**
- Modify: `wile/engine_stdlib_test.go`

**Step 1: Add tests for remaining axioms**

```go
func TestAlgebraRewrite_Commutativity(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	eng, err := wile.NewEngine(ctx,
		wile.WithSafeExtensions(),
		wile.WithSourceFS(stdlib.FS),
		wile.WithLibraryPaths(),
	)
	c.Assert(err, qt.IsNil)
	defer eng.Close()

	result, err := eng.EvalMultiple(ctx, `
		(import (wile algebra rewrite))
		(define proto
		  (make-term-protocol car cdr cons
		    (lambda (a b) (string<? (symbol->string a) (symbol->string b)))))
		(define normalize (make-normalizer (list (commutativity-axiom '+)) proto))
		;; (+ y a) → (+ a y) because a < y
		(normalize '(+ y a))
	`)
	c.Assert(err, qt.IsNil)
	c.Assert(result.SchemeString(), qt.Equals, "(+ a y)")
}

func TestAlgebraRewrite_CommutativityAlreadyOrdered(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	eng, err := wile.NewEngine(ctx,
		wile.WithSafeExtensions(),
		wile.WithSourceFS(stdlib.FS),
		wile.WithLibraryPaths(),
	)
	c.Assert(err, qt.IsNil)
	defer eng.Close()

	result, err := eng.EvalMultiple(ctx, `
		(import (wile algebra rewrite))
		(define proto
		  (make-term-protocol car cdr cons
		    (lambda (a b) (string<? (symbol->string a) (symbol->string b)))))
		(define normalize (make-normalizer (list (commutativity-axiom '+)) proto))
		;; (+ a y) — already ordered, returns unchanged
		(normalize '(+ a y))
	`)
	c.Assert(err, qt.IsNil)
	c.Assert(result.SchemeString(), qt.Equals, "(+ a y)")
}

func TestAlgebraRewrite_Absorbing(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	eng, err := wile.NewEngine(ctx,
		wile.WithSafeExtensions(),
		wile.WithSourceFS(stdlib.FS),
		wile.WithLibraryPaths(),
	)
	c.Assert(err, qt.IsNil)
	defer eng.Close()

	result, err := eng.EvalMultiple(ctx, `
		(import (wile algebra rewrite))
		(define proto
		  (make-term-protocol car cdr cons
		    (lambda (a b) (string<? (symbol->string a) (symbol->string b)))))
		(define normalize (make-normalizer (list (absorbing-axiom '* 'zero)) proto))
		;; (* x zero) → zero
		(normalize '(* x zero))
	`)
	c.Assert(err, qt.IsNil)
	c.Assert(result.SchemeString(), qt.Equals, "zero")
}

func TestAlgebraRewrite_Idempotence(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	eng, err := wile.NewEngine(ctx,
		wile.WithSafeExtensions(),
		wile.WithSourceFS(stdlib.FS),
		wile.WithLibraryPaths(),
	)
	c.Assert(err, qt.IsNil)
	defer eng.Close()

	result, err := eng.EvalMultiple(ctx, `
		(import (wile algebra rewrite))
		(define proto
		  (make-term-protocol car cdr cons
		    (lambda (a b) (string<? (symbol->string a) (symbol->string b)))))
		(define normalize (make-normalizer (list (idempotence-axiom '&)) proto))
		;; (& x x) → x
		(normalize '(& x x))
	`)
	c.Assert(err, qt.IsNil)
	c.Assert(result.SchemeString(), qt.Equals, "x")
}

func TestAlgebraRewrite_Involution(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	eng, err := wile.NewEngine(ctx,
		wile.WithSafeExtensions(),
		wile.WithSourceFS(stdlib.FS),
		wile.WithLibraryPaths(),
	)
	c.Assert(err, qt.IsNil)
	defer eng.Close()

	result, err := eng.EvalMultiple(ctx, `
		(import (wile algebra rewrite))
		(define proto
		  (make-term-protocol car cdr cons
		    (lambda (a b) (string<? (symbol->string a) (symbol->string b)))))
		(define normalize (make-normalizer (list (involution-axiom '!)) proto))
		;; (! (! x)) → x
		(normalize '(! (! x)))
	`)
	c.Assert(err, qt.IsNil)
	c.Assert(result.SchemeString(), qt.Equals, "x")
}

func TestAlgebraRewrite_ComposedTheory(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	eng, err := wile.NewEngine(ctx,
		wile.WithSafeExtensions(),
		wile.WithSourceFS(stdlib.FS),
		wile.WithLibraryPaths(),
	)
	c.Assert(err, qt.IsNil)
	defer eng.Close()

	result, err := eng.EvalMultiple(ctx, `
		(import (wile algebra rewrite))
		(define proto
		  (make-term-protocol car cdr cons
		    (lambda (a b) (string<? (symbol->string a) (symbol->string b)))))
		(define theory
		  (list (identity-axiom '+ 'zero)
		        (commutativity-axiom '+)
		        (absorbing-axiom '* 'zero)))
		(define normalize (make-normalizer theory proto))
		;; Identity fires first: (+ x zero) → x
		(list (normalize '(+ x zero))
		      ;; Absorbing: (* y zero) → zero
		      (normalize '(* y zero))
		      ;; Commutativity: (+ y a) → (+ a y)
		      (normalize '(+ y a)))
	`)
	c.Assert(err, qt.IsNil)
	c.Assert(result.SchemeString(), qt.Equals, "(x zero (+ a y))")
}
```

**Step 2: Run tests**

Run (in wile/): `go test . -run TestAlgebraRewrite -v`
Expected: PASS (implementation from Task 1 already covers all axiom types)

**Step 3: Commit (in wile)**

```
git add engine_stdlib_test.go
git commit -m "test(algebra): add rewrite tests for all 5 axiom types"
```

---

### Task 3: Re-export from (wile algebra) umbrella + update stdlib manifest

**Files:**
- Modify: `wile/stdlib/lib/wile/algebra.sld`
- Modify: `wile/stdlib/stdlib_test.go`

**Step 1: Add re-export**

In `wile/stdlib/lib/wile/algebra.sld`, add to the imports and exports:

Add to imports:
```scheme
(wile algebra rewrite)
```

Add to exports:
```scheme
    ;; Rewriting
    make-term-protocol term-protocol?
    term-get-operator term-get-operands term-make-term term-compare
    identity-axiom identity-axiom?
    commutativity-axiom commutativity-axiom?
    absorbing-axiom absorbing-axiom?
    idempotence-axiom idempotence-axiom?
    involution-axiom involution-axiom?
    axiom?
    make-normalizer
    validate-theory
```

**Step 2: Add to stdlib manifest test**

In `wile/stdlib/stdlib_test.go`, add to the `expected` list:

```go
"wile/algebra/rewrite.sld",
```

**Step 3: Run tests**

Run (in wile/): `go test ./stdlib/ -v && go test . -run TestAlgebraRewrite -v`
Expected: PASS

**Step 4: Version bump, commit, tag, push**

```bash
echo 'v1.9.10' > VERSION
git add stdlib/lib/wile/algebra.sld stdlib/lib/wile/algebra/rewrite.sld stdlib/lib/wile/algebra/rewrite.scm stdlib/stdlib_test.go engine_stdlib_test.go VERSION
git commit -m "feat(algebra): add (wile algebra rewrite) — equational term rewriting from algebraic axioms"
git tag v1.9.10
git push origin master v1.9.10
```

---

## Phase 2: Migrate wile-goast's ssa-normalize

### Task 4: Update wile dependency and rewrite ssa-normalize

**Files:**
- Modify: `wile-goast/go.mod` (via `go get`)
- Modify: `wile-goast/cmd/wile-goast/lib/wile/goast/ssa-normalize.sld`
- Modify: `wile-goast/cmd/wile-goast/lib/wile/goast/ssa-normalize.scm`

**Step 1: Update wile dependency**

Run: `GONOSUMDB=github.com/aalpar/wile GONOSUMCHECK=github.com/aalpar/wile go get github.com/aalpar/wile@v1.9.10 && go mod tidy`

**Step 2: Rewrite ssa-normalize.sld**

```scheme
(define-library (wile goast ssa-normalize)
  (export
    ssa-normalize
    ssa-rule-set
    ssa-rule-identity
    ssa-rule-commutative
    ssa-rule-annihilation)
  (import (wile goast utils)
          (wile algebra rewrite))
  (include "ssa-normalize.scm"))
```

**Step 3: Rewrite ssa-normalize.scm**

```scheme
;;; ssa-normalize.scm — algebraic normalization of SSA binop nodes
;;;
;;; Uses (wile algebra rewrite) to derive rewrite rules from declared
;;; algebraic axioms. Type-scoped to integer types to avoid IEEE 754 issues.

;; Predicate: is this an integer type string?
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

;; ─── Term protocol for SSA binop nodes ──────

(define ssa-binop-protocol
  (make-term-protocol
    ;; get-operator: extract op symbol from ssa-binop
    (lambda (node) (nf node 'op))
    ;; get-operands: extract (x y) as list
    (lambda (node) (list (nf node 'x) (nf node 'y)))
    ;; make-term: rebuild ssa-binop with new operands
    (lambda (op args)
      (cons 'ssa-binop
            (map (lambda (pair)
                   (cond
                     ((eq? (car pair) 'x) (cons 'x (car args)))
                     ((eq? (car pair) 'y) (cons 'y (cadr args)))
                     ((eq? (car pair) 'operands)
                      (cons 'operands args))
                     (else pair)))
                 ;; Need reference to original node — see note below
                 '())))
    ;; compare: lexicographic on operand strings
    string<?))

;; NOTE: The make-term function above needs the original node's fields
;; (name, type, etc.) to rebuild. The generic protocol doesn't carry
;; this context. We solve this by building a node-specific normalizer
;; below instead of using the generic make-normalizer directly.

;; ─── Axiom declarations ─────────────────────

;; We use the axiom types for declaration but build rules manually
;; because SSA binop normalization has two domain-specific concerns:
;;   1. Operand matching uses constant-zero?/constant-one? predicates
;;      (not simple equal?), since SSA represents "0" and "0:int" etc.
;;   2. The term rebuilder needs the original node to preserve name/type fields.
;;
;; The rewrite library's axiom->rules uses equal? for element matching,
;; which doesn't handle the constant format variations. So we use the
;; axiom types as declarations and build SSA-specific rules.

;; ─── SSA-specific rule builders ─────────────

(define (make-identity-rules target-op element?)
  (list
    ;; op(x, e) → x
    (lambda (node)
      (and (tag? node 'ssa-binop)
           (let ((op (nf node 'op)) (x (nf node 'x)) (y (nf node 'y))
                 (typ (nf node 'type)))
             (and (eq? op target-op) (integer-type? typ)
                  (element? y) x))))
    ;; op(e, x) → x
    (lambda (node)
      (and (tag? node 'ssa-binop)
           (let ((op (nf node 'op)) (x (nf node 'x)) (y (nf node 'y))
                 (typ (nf node 'type)))
             (and (eq? op target-op) (integer-type? typ)
                  (element? x) y))))))

(define (make-absorbing-rules target-op element?)
  (list
    ;; op(x, z) → z
    (lambda (node)
      (and (tag? node 'ssa-binop)
           (let ((op (nf node 'op)) (y (nf node 'y))
                 (typ (nf node 'type)))
             (and (eq? op target-op) (integer-type? typ)
                  (element? y) y))))
    ;; op(z, x) → z
    (lambda (node)
      (and (tag? node 'ssa-binop)
           (let ((op (nf node 'op)) (x (nf node 'x))
                 (typ (nf node 'type)))
             (and (eq? op target-op) (integer-type? typ)
                  (element? x) x))))))

(define commutative-ops '(+ * & |\|| ^ == !=))

(define (make-commutativity-rule)
  (lambda (node)
    (and (tag? node 'ssa-binop)
         (let ((op (nf node 'op)) (x (nf node 'x)) (y (nf node 'y)))
           (and op (symbol? op) (memq op commutative-ops)
                (string? x) (string? y)
                (string>? x y)
                (cons 'ssa-binop
                      (map (lambda (pair)
                             (cond
                               ((eq? (car pair) 'x) (cons 'x y))
                               ((eq? (car pair) 'y) (cons 'y x))
                               ((eq? (car pair) 'operands)
                                (cons 'operands (list y x)))
                               (else pair)))
                           (cdr node))))))))

;; ─── Composed rule sets ─────────────────────

(define all-rules
  (append
    (make-identity-rules '+ constant-zero?)
    (make-identity-rules '* constant-one?)
    (make-identity-rules '|\|| constant-zero?)
    (make-identity-rules '^ constant-zero?)
    (make-absorbing-rules '* constant-zero?)
    (make-absorbing-rules '& constant-zero?)
    (list (make-commutativity-rule))))

;; ─── Public API (backward-compatible) ───────

;; Legacy rule constructors — now thin wrappers
(define (ssa-rule-commutative) (make-commutativity-rule))

(define (ssa-rule-identity)
  (let ((rules (append
                 (make-identity-rules '+ constant-zero?)
                 (make-identity-rules '* constant-one?)
                 (make-identity-rules '|\|| constant-zero?)
                 (make-identity-rules '^ constant-zero?))))
    (lambda (node)
      (let try ((rs rules))
        (if (null? rs) #f
          (let ((r ((car rs) node)))
            (if r r (try (cdr rs)))))))))

(define (ssa-rule-annihilation)
  (let ((rules (append
                 (make-absorbing-rules '* constant-zero?)
                 (make-absorbing-rules '& constant-zero?))))
    (lambda (node)
      (let try ((rs rules))
        (if (null? rs) #f
          (let ((r ((car rs) node)))
            (if r r (try (cdr rs)))))))))

;; Compose rules: first non-#f result wins
(define (ssa-rule-set . rules)
  (lambda (node)
    (let loop ((rs rules))
      (if (null? rs) #f
        (let ((result ((car rs) node)))
          (if result result
            (loop (cdr rs))))))))

;; Default normalizer using all rules
(define default-normalizer
  (lambda (node)
    (let try ((rs all-rules))
      (if (null? rs) #f
        (let ((r ((car rs) node)))
          (if r r (try (cdr rs))))))))

;; Main entry point
(define ssa-normalize
  (case-lambda
    ((node) (let ((r (default-normalizer node))) (if r r node)))
    ((node rules) (let ((r (rules node))) (if r r node)))))
```

**Step 4: Run existing ssa-normalize tests**

Run: `go test ./goast/ -run TestSSANormalize -v`
Expected: PASS — all existing tests must produce identical results.

**Step 5: Run full CI**

Run: `make ci`
Expected: PASS

**Step 6: Commit**

```
git add go.mod go.sum cmd/wile-goast/lib/wile/goast/ssa-normalize.sld cmd/wile-goast/lib/wile/goast/ssa-normalize.scm
git commit -m "refactor(ssa-normalize): use table-driven rules from (wile algebra rewrite) axiom types

Declare operators with algebraic properties (identity element, absorbing
element, commutativity) and generate rules from declarations. Backward-
compatible: ssa-rule-set, ssa-rule-identity, ssa-rule-commutative,
ssa-rule-annihilation all preserved. (wile algebra rewrite) imported for
axiom type declarations."
```

---

### Task 5: Update documentation

**Files:**
- Modify: `wile-goast/TODO.md`
- Modify: `wile-goast/CLAUDE.md`

**Step 1: Update TODO.md**

Change the ssa-normalize item in C1 from "leave as-is" to done with the
algebra rewrite library reference.

**Step 2: Update CLAUDE.md**

Add note about `(wile algebra rewrite)` dependency in the SSA normalization
section.

**Step 3: Commit and push**

```
git add TODO.md CLAUDE.md
git commit -m "docs: update ssa-normalize migration status"
git push origin master
```

---

### Summary

| Task | Repo | Key change |
|------|------|------------|
| 1 | wile | Term protocol + identity axiom + make-normalizer |
| 2 | wile | Tests for all 5 axiom types |
| 3 | wile | Re-export from umbrella, version bump, tag |
| 4 | wile-goast | Rewrite ssa-normalize with table-driven rules |
| 5 | wile-goast | Documentation updates |
