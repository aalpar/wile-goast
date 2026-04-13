# SSA Equivalence v2 — Symbolic Theory Integration

Upgrade `ssa-normalize` axioms to `(wile algebra symbolic)` theories. Wire `discover-equivalences` into the unification pipeline.

**Status:** Complete

**Depends on:** `(wile algebra symbolic)` (wile v1.13+), `(wile goast ssa-normalize)`, `(wile goast unify)`

---

## Part 1: Migrate axioms to named-axiom / theory

### What changes in `ssa-normalize.scm`

Axiom declarations become `named-axiom` objects (source of truth). Raw axioms for `make-normalizer` are extracted via `named-axiom-axiom`. A new `ssa-theory` wraps all named axioms into a `theory` object.

```scheme
;; Named axiom is the source of truth
(define identity-add-na
  (make-named-axiom "identity-add" "x + 0 = x"
    (make-identity-axiom '+ constant-zero?)))

;; Raw axiom extracted for flat normalizer (backward compat)
(define int-identity-theory
  (map named-axiom-axiom (list identity-add-na ...)))

;; Theory for discover-equivalences (new export)
(define ssa-theory
  (make-theory (list identity-add-na ...) '(+ * & |\|| ^ == !=)))
```

Flat normalizers (`int-identity-rewrite`, `int-absorbing-rewrite`, `comm-rewrite`) stay as-is — they use `make-normalizer` with raw axioms. The `ssa-normalize` API is unchanged.

### New exports from `ssa-normalize`

- `ssa-theory` — the named theory, usable with `discover-equivalences`
- `ssa-binop-protocol` — the term protocol for SSA binop nodes

### Import changes

```scheme
;; Before
(import (wile goast utils)
        (wile algebra rewrite))

;; After
(import (wile goast utils)
        (wile algebra rewrite)
        (wile algebra symbolic))
```

### Behavioral change

None. Same rules, same results. All existing tests pass unchanged.

## Part 2: Wire discover-equivalences into unify

### New function in `unify.scm`

```scheme
(define (ssa-equivalent? node-a node-b theory proto)
  (let ((forms-a (map car (discover-equivalences theory proto node-a)))
        (forms-b (map car (discover-equivalences theory proto node-b))))
    (let loop ((fa forms-a))
      (cond ((null? fa) #f)
            ((member (car fa) forms-b) #t)
            (else (loop (cdr fa)))))))
```

Takes two SSA binop nodes and a theory. Returns `#t` if any normal form of A matches any normal form of B.

### New export from `unify`

- `ssa-equivalent?`

### Import changes for `unify.sld`

```scheme
;; Before
(import (wile goast utils))

;; After
(import (wile goast utils)
        (wile algebra symbolic)
        (wile goast ssa-normalize))
```

### Integration with score-diffs

Deferred. The `ssa-equivalent?` function is exported as a standalone predicate. Callers (like unify-detect-pkg.scm) can use it directly. Wiring it into `score-diffs` can happen when domain-specific theories make it fire on real code.

## Testing

### Part 1 tests (ssa-normalize)

1. All existing `ssa_normalize_test.go` tests pass unchanged
2. New test: `ssa-theory` is a `theory?` object
3. New test: `ssa-binop-protocol` is a `term-protocol?` object

### Part 2 tests (unify)

1. All existing `unify_test.go` tests pass unchanged
2. Commutativity equivalence: `(+ a b)` and `(+ b a)` → `ssa-equivalent?` returns `#t`
3. Non-equivalence: `(+ a b)` and `(- a b)` → returns `#f`
4. Custom theory: define `min/max absorption` axiom, verify `(min x (max x y))` ≡ `x`
