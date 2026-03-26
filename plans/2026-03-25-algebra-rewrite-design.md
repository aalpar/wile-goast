# (wile algebra rewrite) — Equational Term Rewriting Design

## Summary

General-purpose term rewriting library for wile's `(wile algebra)` stdlib.
Generates rewrite rules from declared algebraic axioms (identity, commutativity,
absorbing elements, idempotence, involution). Primary consumer: wile-goast's
`ssa-normalize` for SSA equivalence detection.

## Motivation

wile-goast's `ssa-normalize.scm` hand-codes algebraic rewrite rules as per-operator
cond chains. Each axiom (identity elimination, commutativity, annihilation) is
written separately for each operator (+, *, &, |, ^). Adding new normalizations
for SSA equivalence v2 (idempotence, involution, and eventually distributivity,
De Morgan) would multiply the hand-coded cases.

The rewrite library inverts this: declare which algebraic properties hold for
each operator, and the library generates rewrite rules automatically.

## Design decisions

- **Term protocol:** User-provided accessor functions (get-operator, get-operands,
  make-term, compare). Generic — not tied to tagged alists or SSA nodes.
- **Axioms as values:** First-class axiom records, composed into theories via
  plain lists. `append` is theory composition.
- **Normalizer:** `(make-normalizer theory protocol)` → procedure `(term → term)`.
  Single pass, first-match wins. No fixed-point iteration — caller controls that.
- **Type scoping:** Consumer's responsibility (e.g., integer-only guard). Keeps
  the rewrite library domain-agnostic.
- **v1 scope:** 5 single-operator axiom types. Multi-operator axioms (distributivity,
  De Morgan) deferred — they require nested-term matching.

## Architecture

```
Term Protocol          Axioms              Normalizer
─────────────    ×    ──────────    →    ────────────
get-operator          identity            make-normalizer
get-operands          commutativity       → (term → term)
make-term             absorbing
compare               idempotence
                      involution
```

The normalizer bridges axioms and protocol: for each axiom, it calls internal
`axiom->rules` with the protocol to produce concrete rewrite functions
`(term → term-or-#f)`.

## Term protocol

```scheme
(make-term-protocol get-operator get-operands make-term compare)
;; get-operator : term → symbol
;; get-operands : term → (list operand ...)
;; make-term    : symbol × (list operand ...) → term
;; compare      : value × value → boolean (less-than for canonical ordering)
```

The protocol is passed to `make-normalizer`, not to individual axioms.
Axioms are abstract declarations; the protocol is the concrete bridge.

## Axiom types

Five axiom constructors, each declaring a property for a specific operator:

| Constructor | Declaration | Generated rules |
|-------------|-------------|-----------------|
| `(identity-axiom op element)` | `op(x, e) = x` | `op(x,e)→x`, `op(e,x)→x` |
| `(commutativity-axiom op)` | `op(x,y) = op(y,x)` | Reorder when `compare(x,y)` is false |
| `(absorbing-axiom op element)` | `op(x,z) = z` | `op(x,z)→z`, `op(z,x)→z` |
| `(idempotence-axiom op)` | `op(x,x) = x` | `op(x,x)→x` |
| `(involution-axiom op)` | `op(op(x)) = x` | Unary: `op(op(x))→x` |

Type predicates: `identity-axiom?`, `commutativity-axiom?`, etc. Plus generic `axiom?`.

## Theories

Theories are plain lists of axioms. No special type.

```scheme
(define int-additive
  (list (identity-axiom '+ "0")
        (commutativity-axiom '+)))

(define int-multiplicative
  (list (identity-axiom '* "1")
        (commutativity-axiom '*)
        (absorbing-axiom '* "0")))

;; Compose by appending
(define int-arithmetic (append int-additive int-multiplicative))
```

## Normalizer

```scheme
(make-normalizer theory protocol) → (term → term)
```

Construction: converts each axiom to rewrite rules via `axiom->rules`.
Application: tries rules in order, first non-`#f` wins. Returns
original term if no rule fires.

Consumer handles type scoping:

```scheme
(define int-normalize (make-normalizer int-theory ssa-protocol))

(define (ssa-normalize node)
  (if (integer-type? (nf node 'type))
      (int-normalize node)
      node))
```

## Validation

`(validate-theory theory protocol samples)` — spot-checks that declared
axioms hold for sample terms. Same pattern as `validate-ring`, `validate-lattice`.

## Exports

```scheme
(define-library (wile algebra rewrite)
  (export
    ;; Term protocol
    make-term-protocol term-protocol?
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
    validate-theory))
```

Re-exported from `(wile algebra)` umbrella.

## Files

```
wile/stdlib/lib/wile/algebra/rewrite.sld   — library definition
wile/stdlib/lib/wile/algebra/rewrite.scm   — implementation
wile/stdlib/lib/wile/algebra.sld           — add re-export
```

## Consumer migration (wile-goast)

`ssa-normalize.scm` replaces hand-coded rules with declarations:

```scheme
(import (wile algebra rewrite))

(define ssa-protocol
  (make-term-protocol
    (lambda (t) (nf t 'op))
    (lambda (t) (list (nf t 'x) (nf t 'y)))
    (lambda (op args) <rebuild ssa-binop from op and args>)
    string<?))

(define int-theory
  (list (identity-axiom '+ "0") (commutativity-axiom '+)
        (identity-axiom '* "1") (commutativity-axiom '*)
        (absorbing-axiom '* "0")
        (identity-axiom '|\|| "0") (commutativity-axiom '|\||)
        (identity-axiom '^ "0") (commutativity-axiom '^)
        (absorbing-axiom '& "0")))

(define int-normalize (make-normalizer int-theory ssa-protocol))
```

## Not in scope (deferred)

- Multi-operator axioms: distributivity `×(x, +(y,z)) → +(×(x,y), ×(x,z))`,
  De Morgan `¬(∧(x,y)) → ∨(¬x, ¬y)`, absorption `∧(x, ∨(x,y)) → x`
- AC-unification, confluence checking, Knuth-Bendix completion
- Associativity normalization (flattening nested same-op expressions)
