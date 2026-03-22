# Call-Convention Mining — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** A Scheme script that discovers call conventions per receiver type in a Go package and reports deviations from majority patterns.

**Architecture:** Single-file Scheme script using AST layer only. Imports `(wile goast utils)` for shared traversal primitives. Four passes: inventory, call extraction, convention discovery, deviation report.

**Tech Stack:** wile-goast (Scheme + Go AST), `(wile goast utils)`, `go-typecheck-package`

**Design:** `docs/plans/2026-03-22-call-convention-mining-design.md`

---

### Task 1: Script Skeleton with Configuration and Imports

**Files:**
- Create: `examples/goast-query/call-convention-mine.scm`

**Step 1: Write the script header with configuration and imports**

```scheme
;;; call-convention-mine.scm — Statistical call-convention discovery
;;;
;;; For each receiver type in a Go package, computes per-type callee
;;; frequency and reports deviations from majority patterns.
;;;
;;; Usage: wile-goast -f examples/goast-query/call-convention-mine.scm

(import (wile goast utils))

;; ── Configuration ────────────────────────────────────────
(if (not (defined? 'target))
  (define target "github.com/aalpar/wile/machine"))

(define min-frequency 0.60)   ;; minimum % to qualify as convention
(define min-sites 5)          ;; minimum absolute call count
(define min-type-methods 5)   ;; skip types with fewer methods
```

Note: `(defined? 'target)` allows overriding target from the command line
via `(begin (define target "other/pkg") (include "..."))`. If wile doesn't
have `defined?`, use a different guard or just hardcode the default.

**Step 2: Verify the script loads without error**

Run: `./dist/darwin/arm64/wile-goast -e '(import (wile goast utils)) (display "ok") (newline)'`
Expected: `ok`

**Step 3: Commit**

```bash
git add examples/goast-query/call-convention-mine.scm
git commit -m "feat: add call-convention mining script skeleton"
```

---

### Task 2: Pass 0 — Inventory (Group Methods by Receiver Type)

**Files:**
- Modify: `examples/goast-query/call-convention-mine.scm`

**Step 1: Write Pass 0 — receiver type extraction and grouping**

The receiver type name comes from the `recv` field of `func-decl`. The recv
is a list of `field` nodes. The field's `type` is either:
- `(star-expr (x ident (name . "Foo")))` for `*Foo` receiver
- `(ident (name . "Foo"))` for value receiver
- `(index-expr (x ident (name . "Foo")) ...)` for generic `Foo[T]`

```scheme
;; ── Pass 0: Inventory ────────────────────────────────────

;; Extract receiver type name from a func-decl, or #f for free functions.
(define (receiver-type-name func)
  (let ((recv (nf func 'recv)))
    (and recv (pair? recv)
         (let* ((recv-field (car recv))
                (recv-type (nf recv-field 'type))
                (base-type (if (tag? recv-type 'star-expr)
                             (nf recv-type 'x)
                             recv-type)))
           (cond
             ((tag? base-type 'ident) (nf base-type 'name))
             ((tag? base-type 'index-expr)
              (let ((x (nf base-type 'x)))
                (and (tag? x 'ident) (nf x 'name))))
             (else #f))))))

;; Group methods by receiver type.
;; Returns: ((type-name (func-decl ...)) ...)
;; Free functions are excluded.
(define (group-by-receiver funcs)
  (let loop ((fs funcs) (groups '()))
    (if (null? fs)
      ;; Filter to types with >= min-type-methods methods
      (filter-map
        (lambda (g)
          (and (>= (length (cdr g)) min-type-methods) g))
        (reverse groups))
      (let* ((func (car fs))
             (tname (receiver-type-name func)))
        (if (not tname)
          (loop (cdr fs) groups)
          (let ((existing (assoc tname groups)))
            (if existing
              (begin
                (set-cdr! existing (cons func (cdr existing)))
                (loop (cdr fs) groups))
              (loop (cdr fs)
                    (cons (list tname func) groups)))))))))

(display "Loading ") (display target) (display " ...") (newline)
(define pkgs (go-typecheck-package target))

(define all-funcs
  (flat-map
    (lambda (pkg)
      (flat-map
        (lambda (file)
          (filter-map
            (lambda (decl)
              (and (tag? decl 'func-decl)
                   (nf decl 'body)  ;; skip external declarations
                   decl))
            (nf file 'decls)))
        (nf pkg 'files)))
    pkgs))

(define type-groups (group-by-receiver all-funcs))

(display "  ") (display (length all-funcs)) (display " functions total")
(newline)
(display "  ") (display (length type-groups))
(display " types with >= ") (display min-type-methods)
(display " methods") (newline)
(for-each
  (lambda (g)
    (display "    ") (display (car g))
    (display ": ") (display (length (cdr g)))
    (display " methods") (newline))
  type-groups)
(newline)
```

Note on `set-cdr!`: If wile doesn't support `set-cdr!`, use a purely
functional accumulation instead — build an alist and merge at the end with
a fold. The existing scripts use `set!` for global accumulators, so mutation
is available, but `set-cdr!` on pairs may differ.

**Alternative without `set-cdr!`:**
```scheme
(define (group-by-receiver funcs)
  (let loop ((fs funcs) (groups '()))
    (if (null? fs)
      (filter-map
        (lambda (g)
          (and (>= (length (cdr g)) min-type-methods)
               (cons (car g) (reverse (cdr g)))))
        (reverse groups))
      (let* ((func (car fs))
             (tname (receiver-type-name func)))
        (if (not tname)
          (loop (cdr fs) groups)
          (let ((existing (assoc tname groups)))
            (if existing
              ;; Replace existing entry with appended method
              (loop (cdr fs)
                    (map (lambda (g)
                           (if (equal? (car g) tname)
                             (cons tname (cons func (cdr g)))
                             g))
                         groups))
              (loop (cdr fs)
                    (cons (list tname func) groups)))))))))
```

**Step 2: Test Pass 0 in isolation**

Run: `./dist/darwin/arm64/wile-goast -f examples/goast-query/call-convention-mine.scm`
Expected: List of types with method counts. For `machine/`, expect to see
`CompileTimeContinuation`, `MachineContext`, etc. with counts roughly
matching: CTC ~97, MC ~79, NT ~34.

If `(import (wile goast utils))` fails, check that the binary was built
with `go:embed` including the `lib/` directory.

If `set-cdr!` fails, switch to the functional alternative.

**Step 3: Commit**

```bash
git add examples/goast-query/call-convention-mine.scm
git commit -m "feat(mining): pass 0 — group methods by receiver type"
```

---

### Task 3: Pass 1 — Call Extraction

**Files:**
- Modify: `examples/goast-query/call-convention-mine.scm`

**Step 1: Write Pass 1 — extract callees from each method body**

A `call-expr` has a `fun` field which is either:
- `(ident (name . "foo"))` → direct call, callee = `"foo"`
- `(selector-expr (x ...) (sel . "Bar"))` → selector call, callee = `"Bar"`
- Something else (function literal, index expression) → skip

```scheme
;; ── Pass 1: Call Extraction ──────────────────────────────

;; Extract callee name from a call-expr's fun field.
;; Returns a string or #f.
(define (callee-name call-node)
  (let ((fun (nf call-node 'fun)))
    (cond
      ((tag? fun 'ident) (nf fun 'name))
      ((tag? fun 'selector-expr) (nf fun 'sel))
      (else #f))))

;; Collect all unique callee names from a function body.
(define (extract-callees func)
  (let ((body (nf func 'body)))
    (if (not body) '()
      (unique
        (walk body
          (lambda (node)
            (and (tag? node 'call-expr)
                 (callee-name node))))))))

;; Build call sets for a type group.
;; Returns: ((method-name (callee ...)) ...)
(define (build-call-sets methods)
  (filter-map
    (lambda (func)
      (let* ((name (nf func 'name))
             (callees (extract-callees func)))
        (and name (list name callees))))
    methods))
```

**Step 2: Add diagnostic output to verify call extraction**

Append after the Pass 0 output:

```scheme
(display "── Pass 1: Call Extraction ──") (newline)
(define all-call-data
  (map (lambda (g)
         (let* ((tname (car g))
                (methods (cdr g))
                (call-sets (build-call-sets methods)))
           (display "  ") (display tname) (display ": ")
           (display (length call-sets)) (display " methods, ")
           (display (apply + (map (lambda (cs) (length (cadr cs))) call-sets)))
           (display " total call edges") (newline)
           (list tname methods call-sets)))
       type-groups))
(newline)
```

**Step 3: Run and verify**

Run: `./dist/darwin/arm64/wile-goast -f examples/goast-query/call-convention-mine.scm`
Expected: Each type shows method count and total call edges. Should see
non-zero call edges for all types. Spot-check: pick a small method you know,
verify its callees look right.

**Step 4: Commit**

```bash
git add examples/goast-query/call-convention-mine.scm
git commit -m "feat(mining): pass 1 — extract call sets per method"
```

---

### Task 4: Pass 2 — Convention Discovery

**Files:**
- Modify: `examples/goast-query/call-convention-mine.scm`

**Step 1: Write Pass 2 — compute callee frequency and identify conventions**

```scheme
;; ── Pass 2: Convention Discovery ─────────────────────────

;; Count how many methods in a group call each callee.
;; Returns: ((callee-name count frequency) ...) sorted by frequency desc.
(define (callee-frequencies call-sets)
  (let* ((n (length call-sets))
         ;; Collect all callees across all methods
         (all-callees (unique (flat-map cadr call-sets)))
         ;; Count each
         (counted
           (map (lambda (callee)
                  (let ((count (length
                                 (filter-map
                                   (lambda (cs)
                                     (and (member? callee (cadr cs))
                                          #t))
                                   call-sets))))
                    (list callee count (/ count n))))
                all-callees)))
    ;; Sort by count descending (simple insertion sort, n is small)
    (let sort-loop ((items counted) (sorted '()))
      (if (null? items) sorted
        (let insert ((item (car items)) (acc '()) (rest sorted))
          (cond
            ((null? rest)
             (sort-loop (cdr items) (reverse (cons item acc))))
            ((> (cadr item) (cadr (car rest)))
             (sort-loop (cdr items)
                        (append (reverse (cons item acc)) rest)))
            (else
             (insert item (cons (car rest) acc) (cdr rest)))))))))

;; Filter to conventions: meets min-frequency and min-sites.
(define (find-conventions freqs)
  (filter-map
    (lambda (f)
      (and (>= (caddr f) min-frequency)
           (>= (cadr f) min-sites)
           f))
    freqs))
```

**Step 2: Add Pass 2 output**

```scheme
(display "── Pass 2: Convention Discovery ──") (newline)
(define all-conventions
  (map (lambda (data)
         (let* ((tname (car data))
                (call-sets (caddr data))
                (freqs (callee-frequencies call-sets))
                (conventions (find-conventions freqs))
                (n (length call-sets)))
           (display "  ") (display tname)
           (display " (") (display n) (display " methods): ")
           (display (length conventions)) (display " conventions")
           (newline)
           (for-each
             (lambda (c)
               (display "    ") (display (car c))
               (display " — ") (display (cadr c))
               (display "/") (display n)
               (display " (")
               (display (exact->inexact (* 100 (caddr c))))
               (display "%)") (newline))
             conventions)
           (list tname call-sets conventions n)))
       all-call-data))
(newline)
```

**Step 3: Run and verify**

Run: `./dist/darwin/arm64/wile-goast -f examples/goast-query/call-convention-mine.scm`
Expected: See discovered conventions with percentages. If nothing shows up,
lower `min-frequency` to 0.40 temporarily to see what's there.

**Step 4: Commit**

```bash
git add examples/goast-query/call-convention-mine.scm
git commit -m "feat(mining): pass 2 — discover conventions by callee frequency"
```

---

### Task 5: Pass 3 — Deviation Report and Summary

**Files:**
- Modify: `examples/goast-query/call-convention-mine.scm`

**Step 1: Write Pass 3 — report deviations per convention**

```scheme
;; ── Pass 3: Deviation Report ─────────────────────────────

(display "══════════════════════════════════════════════════")
(newline)
(display "  Call-Convention Deviation Report                ")
(newline)
(display "══════════════════════════════════════════════════")
(newline) (newline)

(define total-conventions 0)
(define total-deviations 0)

(for-each
  (lambda (data)
    (let* ((tname (car data))
           (call-sets (cadr data))
           (conventions (caddr data))
           (n (cadddr data)))
      (if (pair? conventions)
        (begin
          (display "══ ") (display tname)
          (display " (") (display n) (display " methods) ══")
          (newline)
          (display "  Conventions discovered: ")
          (display (length conventions)) (newline)
          (newline)
          (set! total-conventions
                (+ total-conventions (length conventions)))
          (for-each
            (lambda (conv)
              (let* ((callee (car conv))
                     (count (cadr conv))
                     (deviants
                       (filter-map
                         (lambda (cs)
                           (and (not (member? callee (cadr cs)))
                                (car cs)))
                         call-sets))
                     (dev-count (length deviants)))
                (display "── Convention: ") (display tname)
                (display " → ") (display callee)
                (display " (")
                (display (exact->inexact (* 100 (caddr conv))))
                (display "%, ") (display count)
                (display "/") (display n) (display ") ──")
                (newline)
                (if (pair? deviants)
                  (begin
                    (set! total-deviations
                          (+ total-deviations dev-count))
                    (display "  Deviations (") (display dev-count)
                    (display "):") (newline)
                    (display "    ")
                    (let show ((ds deviants) (col 0))
                      (if (pair? ds)
                        (let* ((name (car ds))
                               (sep (if (pair? (cdr ds)) ", " "")))
                          (display name) (display sep)
                          (if (and (> col 60) (pair? (cdr ds)))
                            (begin (newline) (display "    ")
                                   (show (cdr ds) 0))
                            (show (cdr ds)
                                  (+ col (string-length name) 2))))))
                    (newline))
                  (begin
                    (display "  (no deviations — universal convention)")
                    (newline)))
                (newline)))
            conventions)))))
  all-conventions)
```

**Step 2: Write summary**

```scheme
;; ── Summary ──────────────────────────────────────────────
(display "── Summary ──") (newline)
(display "  Types analyzed:        ")
(display (length type-groups)) (newline)
(display "  Conventions found:     ")
(display total-conventions) (newline)
(display "  Total deviations:      ")
(display total-deviations) (newline)
(display "  Thresholds: frequency=")
(display min-frequency) (display " sites=")
(display min-sites) (display " type-methods=")
(display min-type-methods) (newline)
```

**Step 3: Run the complete script**

Run: `./dist/darwin/arm64/wile-goast -f examples/goast-query/call-convention-mine.scm`
Expected: Full report with conventions and deviations. Review output for
sanity: do the conventions look real? Are the deviations surprising or
expected?

**Step 4: Commit**

```bash
git add examples/goast-query/call-convention-mine.scm
git commit -m "feat(mining): pass 3 — deviation report and summary"
```

---

### Task 6: Tune Thresholds and Validate Against machine/

**Files:**
- Modify: `examples/goast-query/call-convention-mine.scm` (threshold values only)

**Step 1: Run with default thresholds and assess output quality**

Run: `./dist/darwin/arm64/wile-goast -f examples/goast-query/call-convention-mine.scm`

Evaluate:
- If too noisy (many weak conventions, >10 per type): raise `min-frequency` to 0.70
- If too quiet (fewer than 2 conventions total): lower `min-frequency` to 0.50
- If small types produce garbage: raise `min-type-methods` to 8

**Step 2: Adjust thresholds based on output**

Edit the configuration block. Run again. Repeat until the output has 2-5
conventions per major type and the deviations are interpretable.

**Step 3: Commit final thresholds**

```bash
git add examples/goast-query/call-convention-mine.scm
git commit -m "chore(mining): tune thresholds from machine/ validation"
```
