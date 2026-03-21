# Belief DSL Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the belief DSL from `plans/BELIEF-DSL.md` as a pure Scheme library, providing `define-belief`, site selectors, property checkers, and `run-beliefs` runner.

**Architecture:** Four Scheme files under `lib/wile/goast/` following the R7RS `define-library` + `.sld` pattern used by wile (`lib/wile/control.sld` is the reference). The library loader searches `"."` and `"./lib"` by default, so placing files under `lib/` makes them importable as `(wile goast belief)`. Shared utilities (`nf`, `walk`, `tag?`, etc.) are extracted from `examples/goast-query/consistency-comutation.scm` into a reusable module. Tests are Go tests that create a wile engine with all goast extensions loaded, evaluate Scheme code, and assert results — same pattern as `goast/prim_goast_test.go`.

**Tech Stack:** R7RS Scheme (wile), existing goast primitives (`go-typecheck-package`, `go-ssa-build`, `go-callgraph`, `go-cfg-dominates?`), Go test harness with `quicktest`.

---

### Task 1: Extract Shared Utilities into `(wile goast utils)`

The utilities `nf`, `walk`, `tag?`, `filter-map`, `flat-map`, `member?`, `unique`, `has-char?`, `ordered-pairs` are duplicated verbatim across `state-trace-full.scm`, `consistency-comutation.scm`, `unify-detect.scm`, and `dead-field-detect.scm`. Extract them into a reusable library.

**Files:**
- Create: `lib/wile/goast/utils.sld`
- Create: `lib/wile/goast/utils.scm`

**Step 1: Create the `.sld` library definition**

Create `lib/wile/goast/utils.sld`:

```scheme
(define-library (wile goast utils)
  (export
    nf tag? walk
    filter-map flat-map
    member? unique has-char?
    ordered-pairs)
  (import (scheme base))
  (include "utils.scm"))
```

**Step 2: Create the `.scm` implementation**

Create `lib/wile/goast/utils.scm`. Extract verbatim from `examples/goast-query/consistency-comutation.scm` lines 20-72:

```scheme
;;; (wile goast utils) — Shared s-expression traversal utilities
;;;
;;; Extracted from goast example scripts. These operate on the tagged
;;; alist format produced by all five goast layers:
;;;   (tag (key . val) ...)

;; Node field accessor: (nf node 'key) -> val or #f
(define (nf node key)
  (let ((e (assoc key (cdr node))))
    (if e (cdr e) #f)))

;; Tag predicate: (tag? node 'func-decl) -> #t/#f
(define (tag? node t)
  (and (pair? node) (eq? (car node) t)))

;; Map f over lst, keeping only non-#f results.
(define (filter-map f lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (let ((v (f (car xs))))
        (loop (cdr xs) (if v (cons v acc) acc))))))

;; Map f over lst (f must return a list), concatenate results.
(define (flat-map f lst)
  (apply append (map f lst)))

;; Depth-first walk over goast s-expressions.
;; Calls (visitor node) at each tagged alist node.
;; Collects non-#f return values.
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

;; List membership test using equal?.
(define (member? x lst)
  (cond ((null? lst) #f)
        ((equal? x (car lst)) #t)
        (else (member? x (cdr lst)))))

;; Remove duplicates, preserving order.
(define (unique lst)
  (let loop ((xs lst) (seen '()))
    (cond ((null? xs) (reverse seen))
          ((member? (car xs) seen) (loop (cdr xs) seen))
          (else (loop (cdr xs) (cons (car xs) seen))))))

;; Does string s contain character c?
(define (has-char? s c)
  (let loop ((i 0))
    (cond ((>= i (string-length s)) #f)
          ((char=? (string-ref s i) c) #t)
          (else (loop (+ i 1))))))

;; Generate all unordered pairs from a list (each pair once).
(define (ordered-pairs lst)
  (if (null? lst) '()
    (append
      (map (lambda (b) (list (car lst) b)) (cdr lst))
      (ordered-pairs (cdr lst)))))
```

**Step 3: Verify the library loads**

Run: `./dist/wile-goast '(import (wile goast utils)) (nf (quote (foo (bar . 42))) (quote bar))'`

Expected: `42`

**Step 4: Commit**

```bash
git add lib/wile/goast/utils.sld lib/wile/goast/utils.scm
git commit -m "feat(belief): extract shared utilities into (wile goast utils)"
```

---

### Task 2: Belief Registry and `define-belief` Form

Implement the core data model: beliefs as records in a mutable registry. `define-belief` stores a belief definition; `run-beliefs` (Task 5) iterates them.

**Files:**
- Create: `lib/wile/goast/belief.sld`
- Create: `lib/wile/goast/belief.scm`

**Step 1: Create the `.sld` library definition**

Create `lib/wile/goast/belief.sld`:

```scheme
(define-library (wile goast belief)
  (export
    ;; Core form
    define-belief run-beliefs

    ;; Site selectors
    functions-matching callers-of methods-of sites-from

    ;; Selector predicates
    has-params has-receiver name-matches
    contains-call stores-to-fields

    ;; Property checkers
    ;; contains-call is dual-use (selector predicate + checker)
    paired-with ordered co-mutated
    checked-before-use custom)
  (import (scheme base)
          (wile goast utils))
  (include "belief.scm"))
```

**Step 2: Implement belief registry and `define-belief`**

Create `lib/wile/goast/belief.scm`. Start with just the registry and `define-belief` — selectors and checkers come in later tasks.

```scheme
;;; (wile goast belief) — Declarative consistency belief DSL
;;;
;;; Usage:
;;;   (import (wile goast belief))
;;;   (define-belief "name"
;;;     (sites <selector>)
;;;     (expect <checker>)
;;;     (threshold <min-adherence> <min-sites>))
;;;   (run-beliefs "target/package/...")

;; ── Belief registry ─────────────────────────────────────
;;
;; Each belief is stored as:
;;   (name sites-fn expect-fn min-adherence min-sites)
;;
;; sites-fn:  (lambda (ctx) ...) -> list of sites
;; expect-fn: (lambda (site) ...) -> category symbol
;; ctx: the analysis context built by run-beliefs

(define *beliefs* '())

(define (register-belief! name sites-fn expect-fn min-adherence min-sites)
  (set! *beliefs*
    (append *beliefs*
      (list (list name sites-fn expect-fn min-adherence min-sites)))))

(define (belief-name b) (list-ref b 0))
(define (belief-sites-fn b) (list-ref b 1))
(define (belief-expect-fn b) (list-ref b 2))
(define (belief-min-adherence b) (list-ref b 3))
(define (belief-min-sites b) (list-ref b 4))

;; ── define-belief macro ─────────────────────────────────
;;
;; (define-belief "name"
;;   (sites <selector-expr>)
;;   (expect <checker-expr>)
;;   (threshold <adherence> <min-sites>))
;;
;; <selector-expr> and <checker-expr> are evaluated to produce
;; functions. The macro wraps them into the registry.

(define-syntax define-belief
  (syntax-rules (sites expect threshold)
    ((_ name (sites selector) (expect checker) (threshold min-adh min-n))
     (register-belief! name selector checker min-adh min-n))))
```

**Step 3: Verify `define-belief` registers a belief**

Run:
```bash
./dist/wile-goast '(import (wile goast belief)) (define-belief "test" (sites (lambda (ctx) (list))) (expect (lambda (site) (quote ok))) (threshold 0.90 5)) (display "registered")'
```

Expected: `registered`

**Step 4: Commit**

```bash
git add lib/wile/goast/belief.sld lib/wile/goast/belief.scm
git commit -m "feat(belief): define-belief form and belief registry"
```

---

### Task 3: Analysis Context and Lazy Layer Loading

The runner needs a context object that lazily loads AST/SSA/callgraph data. This is the shared state that site selectors and property checkers read from.

**Files:**
- Modify: `lib/wile/goast/belief.scm`

**Step 1: Add context constructor and lazy accessors**

Append to `lib/wile/goast/belief.scm`:

```scheme
;; ── Analysis context ────────────────────────────────────
;;
;; Lazy-loading wrapper around goast primitives.
;; Each layer is loaded at most once per run-beliefs call.
;;
;; Context is a mutable alist:
;;   ((target . "pkg/...")
;;    (pkgs . <loaded-or-#f>)
;;    (ssa . <loaded-or-#f>)
;;    (callgraph . <loaded-or-#f>)
;;    (results . ((belief-name . (adherence-sites deviation-sites)) ...)))

(define (make-context target)
  (list (cons 'target target)
        (cons 'pkgs #f)
        (cons 'ssa #f)
        (cons 'callgraph #f)
        (cons 'results '())))

(define (ctx-ref ctx key)
  (cdr (assoc key ctx)))

(define (ctx-set! ctx key val)
  (set-cdr! (assoc key ctx) val))

(define (ctx-target ctx) (ctx-ref ctx 'target))

(define (ctx-pkgs ctx)
  (or (ctx-ref ctx 'pkgs)
      (let ((pkgs (go-typecheck-package (ctx-target ctx))))
        (ctx-set! ctx 'pkgs pkgs)
        pkgs)))

(define (ctx-ssa ctx)
  (or (ctx-ref ctx 'ssa)
      (let ((ssa (go-ssa-build (ctx-target ctx))))
        (ctx-set! ctx 'ssa ssa)
        ssa)))

(define (ctx-callgraph ctx)
  (or (ctx-ref ctx 'callgraph)
      (let ((cg (go-callgraph (ctx-target ctx) 'static)))
        (ctx-set! ctx 'callgraph cg)
        cg)))

;; Store belief results for bootstrapping via sites-from.
(define (ctx-store-result! ctx name adherence-sites deviation-sites)
  (let ((results (ctx-ref ctx 'results)))
    (ctx-set! ctx 'results
      (cons (list name adherence-sites deviation-sites) results))))

;; Retrieve belief results by name.
(define (ctx-get-result ctx name)
  (let ((entry (assoc name (ctx-ref ctx 'results))))
    (if entry (cdr entry)
      (error "belief not found for sites-from" name))))
```

**Step 2: Verify lazy loading works**

Run:
```bash
./dist/wile-goast '(import (wile goast belief)) (define ctx (make-context "github.com/aalpar/wile-goast/goast")) (display (pair? (ctx-pkgs ctx))) (display " ") (display (pair? (ctx-pkgs ctx)))'
```

Expected: `#t #t` (second call reuses cached value)

**Step 3: Commit**

```bash
git add lib/wile/goast/belief.scm
git commit -m "feat(belief): analysis context with lazy layer loading"
```

---

### Task 4: Site Selectors

Implement the site selector functions. Each returns a function `(lambda (ctx) -> list-of-sites)`.

**Files:**
- Modify: `lib/wile/goast/belief.scm`

**Step 1: Implement `functions-matching`**

Append to `lib/wile/goast/belief.scm`:

```scheme
;; ── Site selectors ──────────────────────────────────────
;;
;; Each selector returns (lambda (ctx) -> list-of-sites).
;; A "site" is a func-decl AST node (for function-based selectors)
;; or a call-graph edge (for caller-based selectors).

;; Extract all func-decl nodes from a package list.
(define (all-func-decls pkgs)
  (flat-map
    (lambda (pkg)
      (flat-map
        (lambda (file)
          (filter-map
            (lambda (decl)
              (and (tag? decl 'func-decl) decl))
            (let ((decls (nf file 'decls)))
              (if (pair? decls) decls '()))))
        (let ((files (nf pkg 'files)))
          (if (pair? files) files '()))))
    pkgs))

;; (functions-matching pred ...) -> (lambda (ctx) -> list-of-func-decls)
;; All predicates must return #t for a function to be included.
(define (functions-matching . preds)
  (lambda (ctx)
    (let ((funcs (all-func-decls (ctx-pkgs ctx))))
      (filter-map
        (lambda (fn)
          (and (let loop ((ps preds))
                 (cond ((null? ps) #t)
                       (((car ps) fn ctx) (loop (cdr ps)))
                       (else #f)))
               fn))
        funcs))))

;; (callers-of func-name) -> (lambda (ctx) -> list-of-caller-sites)
;; Each site is: (caller-name edge func-decl-or-#f)
(define (callers-of func-name)
  (lambda (ctx)
    (let* ((cg (ctx-callgraph ctx))
           (edges (go-callgraph-callers cg func-name)))
      (if (pair? edges)
        (map (lambda (e) (list (nf e 'caller) e)) edges)
        '()))))

;; (methods-of type-name) -> (lambda (ctx) -> list-of-func-decls)
;; Matches methods whose receiver type contains type-name.
(define (methods-of type-name)
  (functions-matching (has-receiver type-name)))

;; (sites-from belief-name [#:which 'adherence|'deviation])
;; -> (lambda (ctx) -> list-of-sites)
;; Retrieves results from a previously evaluated belief.
;; Default: adherence sites.
(define (sites-from belief-name . opts)
  (let ((which (if (and (pair? opts) (pair? (cdr opts))
                        (eq? (car opts) 'which))
                 (cadr opts)
                 'adherence)))
    (lambda (ctx)
      (let ((result (ctx-get-result ctx belief-name)))
        (if (eq? which 'adherence)
          (car result)
          (cadr result))))))
```

**Step 2: Implement selector predicates**

Continue appending to `lib/wile/goast/belief.scm`:

```scheme
;; ── Selector predicates ─────────────────────────────────
;;
;; Each predicate returns (lambda (func-decl ctx) -> #t/#f).
;; Used as arguments to functions-matching.

;; (has-params type-str ...) — function signature contains these param types.
;; Checks the 'type field of each param in the func-decl's 'params list.
(define (has-params . type-strings)
  (lambda (fn ctx)
    (let* ((ftype (nf fn 'type))
           (params (and ftype (nf ftype 'params)))
           (param-types
             (if (pair? params)
               (flat-map
                 (lambda (p)
                   (let ((t (nf p 'type)))
                     (if t (list (nf p 'type-string)) '())))
                 params)
               '())))
      (let loop ((ts type-strings))
        (cond ((null? ts) #t)
              ((member? (car ts) param-types) (loop (cdr ts)))
              (else #f))))))

;; (has-receiver type-str) — method receiver matches type string.
(define (has-receiver type-str)
  (lambda (fn ctx)
    (let* ((recv (nf fn 'recv))
           (recv-list (and recv (if (pair? recv) recv '())))
           (recv-type
             (if (pair? recv-list)
               (nf (car recv-list) 'type-string)
               #f)))
      (and recv-type (equal? recv-type type-str)))))

;; (name-matches pattern) — function name matches substring.
;; Simple substring match; can be extended to glob/regex later.
(define (name-matches pattern)
  (lambda (fn ctx)
    (let ((name (nf fn 'name)))
      (and name (string-contains name pattern)))))

;; Helper: does string s contain substring sub?
(define (string-contains s sub)
  (let ((slen (string-length s))
        (sublen (string-length sub)))
    (let loop ((i 0))
      (cond ((> (+ i sublen) slen) #f)
            ((string=? (substring s i (+ i sublen)) sub) #t)
            (else (loop (+ i 1)))))))

;; (contains-call func-name ...) — function body contains a call to any
;; of the named functions. Used both as a selector predicate AND as a
;; property checker (dual-use, see Task 5).
;;
;; As a predicate: (lambda (func-decl ctx) -> #t/#f)
;; Wraps the walk in a predicate interface.
(define (contains-call . func-names)
  (define (call-matches? node)
    (and (tag? node 'call-expr)
         (let ((fn (nf node 'fun)))
           (cond
             ;; Direct call: (ident name)
             ((tag? fn 'ident)
              (member? (nf fn 'name) func-names))
             ;; Method call: (selector-expr (sel . name))
             ((tag? fn 'selector-expr)
              (member? (nf fn 'sel) func-names))
             (else #f)))))
  ;; Return value is dual-use:
  ;; - As predicate for functions-matching: called as (pred fn ctx) -> #t/#f
  ;; - As checker for expect: called as (checker site) -> 'present/'absent
  ;; We return a closure that handles both arities via the belief system.
  ;; For now, return the predicate form. The checker form wraps this in Task 5.
  (lambda (fn ctx)
    (pair? (walk (or (nf fn 'body) '()) call-matches?))))

;; (stores-to-fields struct-name field ...) — SSA: function stores to
;; these fields. Requires SSA layer.
(define (stores-to-fields struct-name . field-names)
  (lambda (fn ctx)
    ;; For functions-matching, we need to check the SSA representation.
    ;; Find the SSA function matching this func-decl's name, then check
    ;; field stores.
    (let* ((fname (nf fn 'name))
           (ssa-funcs (ctx-ssa ctx))
           (ssa-fn (find-ssa-func-by-name ssa-funcs fname)))
      (if ssa-fn
        (let ((stored (stored-fields-in-func ssa-fn field-names)))
          (pair? stored))
        #f))))

;; Find an SSA function by name.
(define (find-ssa-func-by-name ssa-funcs name)
  (let loop ((fns (if (pair? ssa-funcs) ssa-funcs '())))
    (cond ((null? fns) #f)
          ((equal? (nf (car fns) 'name) name) (car fns))
          (else (loop (cdr fns))))))

;; Reuse the store-set logic from consistency-comutation.scm.
;; Collects field-addrs, joins with stores, returns list of stored field names.
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

(define (stored-fields-in-func ssa-func target-fields)
  (let* ((all-field-addrs (collect-field-addrs ssa-func))
         (stores (collect-stores ssa-func))
         (store-addrs (map car stores))
         (receivers (unique (map caddr all-field-addrs)))
         (valid-receivers
           (filter-map
             (lambda (recv)
               (let* ((recv-fas (filter-map
                                  (lambda (fa) (and (equal? (caddr fa) recv) fa))
                                  all-field-addrs))
                      (recv-fields (unique (map cadr recv-fas)))
                      (all-match (let loop ((fs recv-fields))
                                   (cond ((null? fs) #t)
                                         ((not (member? (car fs) target-fields)) #f)
                                         (else (loop (cdr fs)))))))
                 (and all-match recv)))
             receivers))
         (stored (filter-map
                   (lambda (fa)
                     (let ((reg (car fa))
                           (field (cadr fa))
                           (recv (caddr fa)))
                       (and (member? recv valid-receivers)
                            (member? reg store-addrs)
                            (member? field target-fields)
                            field)))
                   all-field-addrs)))
    (unique stored)))
```

**Step 3: Verify `functions-matching` works**

Run:
```bash
./dist/wile-goast '(import (wile goast belief)) (define ctx (make-context "github.com/aalpar/wile-goast/goast")) (define sites ((functions-matching (name-matches "Prim")) ctx)) (display (length sites))'
```

Expected: a number > 0 (the goast package has multiple `Prim*` functions)

**Step 4: Commit**

```bash
git add lib/wile/goast/belief.scm
git commit -m "feat(belief): site selectors and predicates"
```

---

### Task 5: Property Checkers

Implement the checker functions. Each returns a function `(lambda (site) -> category-symbol)`.

**Files:**
- Modify: `lib/wile/goast/belief.scm`

**Step 1: Implement property checkers**

Append to `lib/wile/goast/belief.scm`:

```scheme
;; ── Property checkers ───────────────────────────────────
;;
;; Each checker returns (lambda (site ctx) -> category-symbol).
;; The majority category becomes the belief; minorities are deviations.
;;
;; Note: contains-call is dual-use. As a checker, it wraps the
;; predicate form to return 'present/'absent.

;; When contains-call is used as a checker (in expect position),
;; we need to detect the usage context. Since define-belief passes
;; the checker directly, and contains-call returns a (fn ctx -> #t/#f)
;; predicate, the runner wraps it: #t -> 'present, #f -> 'absent.
;; This wrapper is applied in the runner (Task 6), not here.
;; The checker protocol is: (lambda (site ctx) -> symbol).
;; contains-call already returns (lambda (fn ctx) -> #t/#f).
;; The runner normalizes #t/#f to 'present/'absent.

;; (paired-with op-a op-b) — checks if function body contains both
;; operations, with preference for defer pairing.
;; Returns: 'paired-defer, 'paired-call, or 'unpaired
(define (paired-with op-a op-b)
  (lambda (site ctx)
    (let* ((body (or (nf site 'body) '()))
           ;; Check for deferred release
           (has-defer-b
             (pair? (walk body
               (lambda (node)
                 (and (tag? node 'defer-stmt)
                      (let ((call (nf node 'call)))
                        (and call (tag? call 'call-expr)
                             (let ((fn (nf call 'fun)))
                               (or (and (tag? fn 'ident)
                                        (equal? (nf fn 'name) op-b))
                                   (and (tag? fn 'selector-expr)
                                        (equal? (nf fn 'sel) op-b)))))))))))
           ;; Check for non-deferred release
           (has-call-b
             (pair? (walk body
               (lambda (node)
                 (and (tag? node 'call-expr)
                      (let ((fn (nf node 'fun)))
                        (or (and (tag? fn 'ident)
                                 (equal? (nf fn 'name) op-b))
                            (and (tag? fn 'selector-expr)
                                 (equal? (nf fn 'sel) op-b))))))))))
      (cond
        (has-defer-b 'paired-defer)
        (has-call-b 'paired-call)
        (else 'unpaired)))))

;; (ordered op-a op-b) — checks whether op-a's block dominates op-b's block.
;; Requires CFG layer.
;; Returns: 'a-dominates-b, 'b-dominates-a, 'same-block, 'unordered, or 'missing
(define (ordered op-a op-b)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (cfg (go-cfg fname)))
      (if (not cfg) 'missing
        (let* ((blocks (nf cfg 'blocks))
               ;; Find blocks containing calls to op-a and op-b
               (a-blocks (find-call-blocks blocks op-a))
               (b-blocks (find-call-blocks blocks op-b)))
          (cond
            ((or (null? a-blocks) (null? b-blocks)) 'missing)
            ((= (car a-blocks) (car b-blocks)) 'same-block)
            ((go-cfg-dominates? cfg (car a-blocks) (car b-blocks)) 'a-dominates-b)
            ((go-cfg-dominates? cfg (car b-blocks) (car a-blocks)) 'b-dominates-a)
            (else 'unordered)))))))

;; Find block indices containing a call to the named function.
(define (find-call-blocks blocks func-name)
  (filter-map
    (lambda (block)
      (let ((idx (nf block 'index))
            (stmts (or (nf block 'stmts) '())))
        (and (pair? (walk stmts
               (lambda (node)
                 (and (tag? node 'call-expr)
                      (let ((fn (nf node 'fun)))
                        (or (and (tag? fn 'ident)
                                 (equal? (nf fn 'name) func-name))
                            (and (tag? fn 'selector-expr)
                                 (equal? (nf fn 'sel) func-name))))))))
             idx)))
    (if (pair? blocks) blocks '())))

;; (co-mutated field ...) — checks whether all named fields are stored
;; together in the function.
;; Returns: 'co-mutated or 'partial
(define (co-mutated . field-names)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (ssa-funcs (ctx-ssa ctx))
           (ssa-fn (find-ssa-func-by-name ssa-funcs fname)))
      (if (not ssa-fn) 'partial
        (let ((stored (stored-fields-in-func ssa-fn field-names)))
          (if (= (length stored) (length field-names))
            'co-mutated
            'partial))))))

;; (checked-before-use value-pattern) — checks whether a value is
;; tested before use via SSA + CFG dominance.
;; Returns: 'guarded or 'unguarded
(define (checked-before-use value-pattern)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (ssa-funcs (ctx-ssa ctx))
           (ssa-fn (find-ssa-func-by-name ssa-funcs fname)))
      (if (not ssa-fn) 'unguarded
        (let* ((blocks (nf ssa-fn 'blocks))
               (all-instrs (if (pair? blocks)
                             (flat-map
                               (lambda (b) (let ((is (nf b 'instrs)))
                                             (if (pair? is) is '())))
                               blocks)
                             '()))
               ;; Find instructions referencing the pattern
               (uses (filter-map
                       (lambda (instr)
                         (let ((ops (nf instr 'operands)))
                           (and (pair? ops) (member? value-pattern ops)
                                instr)))
                       all-instrs))
               ;; Check if any use is an ssa-if (guard)
               (has-guard (let loop ((us uses))
                            (cond ((null? us) #f)
                                  ((tag? (car us) 'ssa-if) #t)
                                  (else (loop (cdr us)))))))
          (if has-guard 'guarded 'unguarded))))))

;; (custom proc) — escape hatch. proc is (lambda (site ctx) -> symbol).
;; Returns proc directly since it already has the right shape.
(define (custom proc) proc)
```

**Step 2: Commit**

```bash
git add lib/wile/goast/belief.scm
git commit -m "feat(belief): property checkers — paired-with, ordered, co-mutated, checked-before-use, custom"
```

---

### Task 6: Statistical Comparison Engine

Implement the generic majority-detection and deviation-reporting logic shared by all beliefs.

**Files:**
- Modify: `lib/wile/goast/belief.scm`

**Step 1: Implement statistical comparison**

Append to `lib/wile/goast/belief.scm`:

```scheme
;; ── Statistical comparison ──────────────────────────────
;;
;; Given a list of (site . category) pairs, find the majority
;; category and report deviations.

;; Count occurrences of each category.
;; Returns: ((category . count) ...)
(define (count-categories pairs)
  (let loop ((ps pairs) (counts '()))
    (if (null? ps) counts
      (let* ((cat (cdr (car ps)))
             (entry (assoc cat counts)))
        (if entry
          (begin (set-cdr! entry (+ (cdr entry) 1))
                 (loop (cdr ps) counts))
          (loop (cdr ps) (cons (cons cat 1) counts)))))))

;; Find the category with the highest count.
(define (majority-category counts)
  (let loop ((cs counts) (best-cat #f) (best-count 0))
    (if (null? cs) (cons best-cat best-count)
      (if (> (cdr (car cs)) best-count)
        (loop (cdr cs) (car (car cs)) (cdr (car cs)))
        (loop (cdr cs) best-cat best-count)))))

;; Evaluate a single belief against its sites.
;; Returns: (name majority-cat adherence-ratio total adherence-sites deviation-sites)
;; or #f if the belief has no sites.
(define (evaluate-belief belief ctx)
  (let* ((name (belief-name belief))
         (sites-fn (belief-sites-fn belief))
         (expect-fn (belief-expect-fn belief))
         (sites (sites-fn ctx)))
    (if (null? sites) #f
      (let* (;; Classify each site
             (classified
               (map (lambda (site)
                      (let ((cat (expect-fn site ctx)))
                        ;; Normalize #t/#f to 'present/'absent
                        ;; (for contains-call dual-use)
                        (cons site
                          (cond ((eq? cat #t) 'present)
                                ((eq? cat #f) 'absent)
                                (else cat)))))
                    sites))
             (counts (count-categories classified))
             (maj (majority-category counts))
             (maj-cat (car maj))
             (maj-count (cdr maj))
             (total (length classified))
             (ratio (/ maj-count total))
             (adherence (filter-map
                          (lambda (p) (and (eq? (cdr p) maj-cat) (car p)))
                          classified))
             (deviations (filter-map
                           (lambda (p) (and (not (eq? (cdr p) maj-cat)) p))
                           classified)))
        (list name maj-cat ratio total adherence deviations)))))
```

**Step 2: Commit**

```bash
git add lib/wile/goast/belief.scm
git commit -m "feat(belief): statistical comparison — majority detection and deviation reporting"
```

---

### Task 7: `run-beliefs` Runner and Report Output

Implement the top-level runner that evaluates all registered beliefs and prints the report.

**Files:**
- Modify: `lib/wile/goast/belief.scm`

**Step 1: Implement `run-beliefs`**

Append to `lib/wile/goast/belief.scm`:

```scheme
;; ── Runner ──────────────────────────────────────────────

;; Extract a display name from a site (func-decl or caller edge).
(define (site-display-name site)
  (cond
    ((and (pair? site) (tag? site 'func-decl))
     (or (nf site 'name) "<anonymous>"))
    ((and (pair? site) (pair? (car site)))
     ;; Caller site: (caller-name edge)
     (let ((name (car site)))
       (if (string? name) name (display-to-string name))))
    (else
     (display-to-string site))))

;; Convert a value to a string for display.
(define (display-to-string val)
  (let ((port (open-output-string)))
    (display val port)
    (get-output-string port)))

;; Print report header.
(define (print-header)
  (display "══════════════════════════════════════════════════")
  (newline)
  (display "  Consistency Analysis")
  (newline)
  (display "══════════════════════════════════════════════════")
  (newline) (newline))

;; Print a single belief result.
(define (print-belief-result result)
  (let* ((name (list-ref result 0))
         (maj-cat (list-ref result 1))
         (ratio (list-ref result 2))
         (total (list-ref result 3))
         (adherence (list-ref result 4))
         (deviations (list-ref result 5))
         (adherence-count (length adherence)))
    (display "── Belief: ") (display name) (display " ──")
    (newline)
    (display "  Pattern: ") (display maj-cat)
    (display " (") (display adherence-count)
    (display "/") (display total) (display " sites)")
    (newline)
    (if (null? deviations)
      (begin (display "    (no deviations)") (newline))
      (for-each
        (lambda (d)
          (display "    DEVIATION: ")
          (display (site-display-name (car d)))
          (display " -> ") (display (cdr d))
          (newline))
        deviations))
    (newline)))

;; Main entry point.
;; Evaluates all registered beliefs against the target package.
(define (run-beliefs target)
  (let ((ctx (make-context target)))
    (print-header)
    (let loop ((beliefs *beliefs*)
               (evaluated 0)
               (strong 0)
               (total-deviations 0))
      (if (null? beliefs)
        ;; Summary
        (begin
          (display "── Summary ──") (newline)
          (display "  Beliefs evaluated:   ") (display evaluated) (newline)
          (display "  Strong beliefs:      ") (display strong) (newline)
          (display "  Deviations found:    ") (display total-deviations) (newline))
        ;; Evaluate next belief
        (let* ((belief (car beliefs))
               (result (evaluate-belief belief ctx)))
          (if (not result)
            ;; No sites found
            (begin
              (display "── Belief: ") (display (belief-name belief)) (display " ──")
              (newline)
              (display "  (no sites found)") (newline) (newline)
              (loop (cdr beliefs) (+ evaluated 1) strong total-deviations))
            (let* ((name (list-ref result 0))
                   (ratio (list-ref result 2))
                   (total (list-ref result 3))
                   (adherence (list-ref result 4))
                   (deviations (list-ref result 5))
                   (min-adh (belief-min-adherence belief))
                   (min-n (belief-min-sites belief))
                   (is-strong (and (>= ratio min-adh) (>= total min-n))))
              ;; Store results for bootstrapping
              (ctx-store-result! ctx name
                adherence
                (map car deviations))
              (if is-strong
                (begin
                  (print-belief-result result)
                  (loop (cdr beliefs)
                        (+ evaluated 1) (+ strong 1)
                        (+ total-deviations (length deviations))))
                (begin
                  (display "── Belief: ") (display name) (display " ──")
                  (newline)
                  (display "  (weak: ") (display ratio)
                  (display " adherence, ") (display total)
                  (display " sites — below threshold)") (newline) (newline)
                  (loop (cdr beliefs)
                        (+ evaluated 1) strong total-deviations))))))))))
```

**Step 2: Commit**

```bash
git add lib/wile/goast/belief.scm
git commit -m "feat(belief): run-beliefs runner with report output"
```

---

### Task 8: Integration Test — End-to-End Belief Script

Write an example belief script that exercises the full pipeline against wile-goast's own codebase, and a Go test that runs it.

**Files:**
- Create: `examples/goast-query/belief-example.scm`
- Create: `lib/wile/goast/belief_test.go`

**Step 1: Create the example belief script**

Create `examples/goast-query/belief-example.scm`:

```scheme
;;; belief-example.scm — Example belief definitions using the DSL
;;;
;;; Demonstrates the belief DSL against wile-goast's own codebase.
;;;
;;; Usage: ./dist/wile-goast '(load "examples/goast-query/belief-example.scm")'

(import (wile goast belief))

;; Belief: functions with "Prim" in their name should follow
;; a consistent pattern (this is a smoke test, not a real belief).
(define-belief "prim-functions-have-body"
  (sites (functions-matching (name-matches "Prim")))
  (expect (custom (lambda (site ctx)
    (if (nf site 'body) 'has-body 'no-body))))
  (threshold 0.90 3))

(run-beliefs "github.com/aalpar/wile-goast/goast")
```

**Step 2: Run the example script to verify end-to-end**

Run: `./dist/wile-goast '(load "examples/goast-query/belief-example.scm")'`

Expected: Report output showing the belief evaluated, with pattern and site count. All `Prim*` functions should have bodies, so 100% adherence.

**Step 3: Create Go integration test**

Create `lib/wile/goast/belief_test.go`:

```go
package belief_test

import (
	"context"
	"testing"

	"github.com/aalpar/wile"
	"github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile-goast/goastcfg"
	"github.com/aalpar/wile-goast/goastcg"
	"github.com/aalpar/wile-goast/goastlint"
	"github.com/aalpar/wile-goast/goastssa"

	qt "github.com/frankban/quicktest"
)

func newEngine(t *testing.T) *wile.Engine {
	t.Helper()
	engine, err := wile.NewEngine(context.Background(),
		wile.WithSafeExtensions(),
		wile.WithExtension(goast.Extension),
		wile.WithExtension(goastssa.Extension),
		wile.WithExtension(goastcg.Extension),
		wile.WithExtension(goastcfg.Extension),
		wile.WithExtension(goastlint.Extension),
	)
	qt.New(t).Assert(err, qt.IsNil)
	return engine
}

func TestBeliefDSLImport(t *testing.T) {
	engine := newEngine(t)
	_, err := engine.Eval(context.Background(),
		`(import (wile goast belief))`)
	qt.New(t).Assert(err, qt.IsNil)
}

func TestDefineAndRunBelief(t *testing.T) {
	engine := newEngine(t)
	_, err := engine.Eval(context.Background(), `
		(import (wile goast belief))
		(define-belief "test-belief"
			(sites (functions-matching (name-matches "Prim")))
			(expect (custom (lambda (site ctx)
				(if (nf site 'body) 'has-body 'no-body))))
			(threshold 0.90 3))
		(run-beliefs "github.com/aalpar/wile-goast/goast")
	`)
	qt.New(t).Assert(err, qt.IsNil)
}
```

**Step 4: Run the Go test**

Run: `go test ./lib/wile/goast/ -v -run TestBelief`

Expected: Both tests pass.

**Step 5: Commit**

```bash
git add examples/goast-query/belief-example.scm lib/wile/goast/belief_test.go
git commit -m "test(belief): integration tests — import, define, run"
```

---

### Task 9: Predicate Composition (`and`, `or`, `not`)

Add the boolean combinators for composing selector predicates.

**Files:**
- Modify: `lib/wile/goast/belief.scm`

**Step 1: Add predicate combinators**

Insert after the selector predicates section in `lib/wile/goast/belief.scm`:

```scheme
;; ── Predicate combinators ───────────────────────────────
;;
;; Note: these shadow Scheme's built-in and/or/not when used
;; in the (wile goast belief) context. We use belief-specific
;; names to avoid shadowing.

;; (all-of pred ...) — all predicates must match
(define (all-of . preds)
  (lambda (fn ctx)
    (let loop ((ps preds))
      (cond ((null? ps) #t)
            (((car ps) fn ctx) (loop (cdr ps)))
            (else #f)))))

;; (any-of pred ...) — at least one predicate must match
(define (any-of . preds)
  (lambda (fn ctx)
    (let loop ((ps preds))
      (cond ((null? ps) #f)
            (((car ps) fn ctx) #t)
            (else (loop (cdr ps)))))))

;; (none-of pred ...) — no predicate matches
(define (none-of . preds)
  (lambda (fn ctx)
    (not ((apply any-of preds) fn ctx))))
```

**Step 2: Update `.sld` exports**

Add `all-of`, `any-of`, `none-of` to the export list in `lib/wile/goast/belief.sld`.

**Step 3: Verify composition works**

Run:
```bash
./dist/wile-goast '(import (wile goast belief)) (define ctx (make-context "github.com/aalpar/wile-goast/goast")) (define sites ((functions-matching (all-of (name-matches "Prim") (name-matches "Parse"))) ctx)) (display (length sites))'
```

Expected: a number > 0 (matches `PrimGoParseFile`, `PrimGoParseString`, `PrimGoParseExpr`)

**Step 4: Commit**

```bash
git add lib/wile/goast/belief.sld lib/wile/goast/belief.scm
git commit -m "feat(belief): predicate combinators — all-of, any-of, none-of"
```

---

### Task 10: Validate Against Co-Mutation Example

Re-express the existing `consistency-comutation.scm` script as belief DSL definitions and verify the output matches.

**Files:**
- Create: `examples/goast-query/belief-comutation.scm`

**Step 1: Write the DSL version**

Create `examples/goast-query/belief-comutation.scm`:

```scheme
;;; belief-comutation.scm — Co-mutation beliefs using the DSL
;;;
;;; Re-expresses consistency-comutation.scm as define-belief forms.
;;; Validates against known results from CONSISTENCY-DEVIATION.md.
;;;
;;; Usage: ./dist/wile-goast '(load "examples/goast-query/belief-comutation.scm")'

(import (wile goast belief))

;; Debugger stepping fields — known co-mutation pattern
(define-belief "stepping-mode-frame"
  (sites (functions-matching
           (stores-to-fields "Debugger" "stepMode" "stepFrame")))
  (expect (co-mutated "stepMode" "stepFrame"))
  (threshold 0.66 3))

(define-belief "stepping-mode-depth"
  (sites (functions-matching
           (stores-to-fields "Debugger" "stepMode" "stepFrameDepth")))
  (expect (co-mutated "stepMode" "stepFrameDepth"))
  (threshold 0.66 3))

(run-beliefs "github.com/aalpar/wile/machine")
```

**Step 2: Run and compare**

Run: `./dist/wile-goast '(load "examples/goast-query/belief-comutation.scm")'`

Expected: Output should show the stepping field co-mutation beliefs with deviations matching those documented in `plans/CONSISTENCY-DEVIATION.md` §Validation Results (StepOver missing stepFrame, StepOut missing stepFrameDepth).

**Step 3: Commit**

```bash
git add examples/goast-query/belief-comutation.scm
git commit -m "test(belief): validate co-mutation DSL against known results"
```

---

## Notes for the Implementer

### Library Search Path

The wile engine searches `"."` and `"./lib"` for `.sld` files. The belief library at `lib/wile/goast/belief.sld` is importable as `(wile goast belief)` when the working directory is the project root. Tests must either run from the project root or configure the search path.

### `nf` Availability in User Scripts

The `(wile goast belief)` library imports and re-exports `(wile goast utils)`, so `nf`, `walk`, `tag?` etc. are available to `custom` lambdas without a separate import. Verify this is the case — if `define-library` doesn't transitively export, add explicit re-exports.

### `contains-call` Dual-Use

`contains-call` returns `(lambda (fn ctx) -> #t/#f)`. When used as a predicate in `functions-matching`, this works directly. When used as a checker in `expect`, the runner normalizes `#t -> 'present`, `#f -> 'absent`. This normalization happens in `evaluate-belief` (Task 6).

### Testing Pattern

Go tests create a wile engine with all five extensions, then `engine.Eval()` Scheme code that imports the belief library. Tests assert on evaluation success (no error) rather than parsing output text. For deeper assertions, Scheme code can return specific values that the Go test inspects via `values.Value`.
