;;; inline-expand.scm — Inline all resolved calls, flatten function boundaries
;;;
;;; Purpose: stress-test wile-goast's primitives for AST TRANSFORMATION
;;; (vs. the query-only operations the belief DSL uses).
;;;
;;; Findings:
;;;   MISSING PRIMITIVE #1: ast-transform (generic tree rewriter)
;;;     - walk is a collector (read-only). Every transformation script
;;;       must re-implement the 3-way dispatch: tagged node / list / atom.
;;;     - ~15 lines, needed by every transformation.
;;;
;;;   MISSING PRIMITIVE #2: statement splicing
;;;     - Inlining a multi-statement function into expression position
;;;       requires inserting statements BEFORE the containing statement
;;;       in the enclosing block. No primitive for this structural surgery.
;;;     - Also needed for: extracting functions, loop unrolling, etc.
;;;
;;; Usage: wile-goast -f examples/goast-query/inline-expand.scm

(import (wile goast utils))

;; ── Test source ──────────────────────────────────
;;
;; Three tiers of inlining difficulty:
;;   double/addOne — single-expression return (easy)
;;   clamp         — multi-statement, multi-return (hard)
;;   compute       — calls double, addOne, clamp

(define source "
package example

func double(x int) int {
	return x * 2
}

func addOne(x int) int {
	return x + 1
}

func clamp(x, lo, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func compute(a, b int) int {
	x := double(a)
	y := addOne(b)
	z := clamp(x+y, 0, 100)
	return z
}
")

;; ── Parse ────────────────────────────────────────

(display "Parsing ...") (newline)
(define file-ast (go-parse-string source))

(define (extract-funcs file)
  (filter-map
    (lambda (d) (and (tag? d 'func-decl) d))
    (nf file 'decls)))

(define (find-func-by-name file name)
  (let loop ((fns (extract-funcs file)))
    (cond ((null? fns) #f)
          ((equal? (nf (car fns) 'name) name) (car fns))
          (else (loop (cdr fns))))))

;; ══════════════════════════════════════════════════
;; MISSING PRIMITIVE #1: ast-transform
;;
;; walk (utils.scm:29) visits nodes and collects results.
;; It does NOT produce a new tree. Every transformation
;; must reimplement this 15-line function.
;;
;; Proposed primitive:
;;   (ast-transform node f) -> new-node
;;   f returns replacement node or #f (keep original, recurse)
;; ══════════════════════════════════════════════════

(define (ast-transform node f)
  (let ((replacement (f node)))
    (if replacement replacement
      (cond
        ;; Tagged alist: recurse into field values
        ((and (pair? node) (symbol? (car node)))
         (cons (car node)
               (map (lambda (field)
                      (if (pair? field)
                        (cons (car field)
                              (ast-transform (cdr field) f))
                        field))
                    (cdr node))))
        ;; List of child nodes
        ((and (pair? node) (pair? (car node)))
         (map (lambda (n) (ast-transform n f)) node))
        ;; Atom
        (else node)))))

;; ── Param extraction ─────────────────────────────

(define (formal-param-names fn)
  (let* ((ftype (nf fn 'type))
         (params (and ftype (nf ftype 'params))))
    (if (pair? params)
      (flat-map
        (lambda (field)
          (let ((names (nf field 'names)))
            (if (pair? names) names '())))
        params)
      '())))

;; ── Substitution ─────────────────────────────────
;;
;; Replace ident nodes whose name appears in mapping.
;; Trivial once ast-transform exists. 5 lines.

(define (subst-idents node mapping)
  (ast-transform node
    (lambda (n)
      (and (tag? n 'ident)
           (let ((entry (assoc (nf n 'name) mapping)))
             (and entry (cdr entry)))))))

(define (build-param-map formals actuals)
  (let loop ((fs formals) (as actuals) (acc '()))
    (if (or (null? fs) (null? as)) acc
      (loop (cdr fs) (cdr as)
            (cons (cons (car fs) (car as)) acc)))))

;; ── Phase 1: expression-level inlining ───────────
;;
;; Handles: func f(x T) R { return <expr> }
;; Replace call f(arg) with <expr>[x := arg].
;; Token cost: low — 20 lines on top of ast-transform.

(define (single-return-expr fn)
  (let* ((body (nf fn 'body))
         (stmts (and body (nf body 'list))))
    (and (pair? stmts)
         (= (length stmts) 1)
         (tag? (car stmts) 'return-stmt)
         (let ((results (nf (car stmts) 'results)))
           (and (pair? results)
                (= (length results) 1)
                (car results))))))

(define (try-inline-expr call-node file)
  (and (tag? call-node 'call-expr)
       (let* ((fun (nf call-node 'fun))
              (name (and (tag? fun 'ident) (nf fun 'name)))
              (callee (and name (find-func-by-name file name))))
         (and callee
              (let ((ret-expr (single-return-expr callee)))
                (and ret-expr
                     (let* ((formals (formal-param-names callee))
                            (actuals (or (nf call-node 'args) '()))
                            (mapping (build-param-map formals actuals)))
                       (subst-idents ret-expr mapping))))))))

(define (inline-expr-calls node file)
  (ast-transform node
    (lambda (n)
      (let ((inlined (try-inline-expr n file)))
        ;; Recurse into result to handle nested inlinable calls
        (and inlined (inline-expr-calls inlined file))))))

;; ── Run Phase 1 ──────────────────────────────────

(define target (find-func-by-name file-ast "compute"))
(if (not target) (error "compute not found"))

(display "── Original ──────────────────────────────") (newline)
(display (go-format target)) (newline) (newline)

(define phase1 (inline-expr-calls target file-ast))

(display "── Phase 1: expression-level inlining ────") (newline)
(display "   (double, addOne inlined; clamp left — multi-statement)")
(newline) (newline)
(display (go-format phase1)) (newline) (newline)

;; ══════════════════════════════════════════════════
;; Phase 2: statement-level inlining (multi-statement bodies)
;;
;; MISSING PRIMITIVE #2: statement splicing
;;
;; clamp has 3 statements with early returns.
;; To inline `z := clamp(x+y, 0, 100)`:
;;
;;   1. Extract clamp's body statements
;;   2. Substitute params: x→(x+y), lo→0, hi→100
;;   3. Replace `return <val>` with `z = <val>; goto afterClamp`
;;      (or restructure into if/else chain)
;;   4. Splice the rewritten statements into compute's body
;;      BEFORE the assign-stmt, removing the original assign
;;   5. Add `afterClamp:` label (or use nested blocks)
;;
;; This requires:
;;   - Recognizing call position (expression vs. assign RHS)
;;   - Rewriting return-stmts in the inlined body
;;   - Splicing statements into the enclosing block
;;   - Fresh name generation (avoiding collisions)
;;
;; None of these have primitives. Estimated token cost:
;;   ~80-120 lines of manual AST surgery.
;;
;; The fundamental problem: ast-transform works node-by-node,
;; but statement splicing changes the PARENT's structure.
;; A transformer visiting an assign-stmt needs to replace it
;; with MULTIPLE statements in the parent block. ast-transform
;; returns one node per input node — it can't expand.
;;
;; A splice-aware transformer would need a different contract:
;;   f returns a LIST of replacement nodes (possibly empty,
;;   possibly multiple), and the parent list concatenates them.
;; ══════════════════════════════════════════════════

;; Attempt it anyway to measure the cost.

;; Rewrite return-stmts: `return <val>` → `<target> = <val>`
;; where target is the LHS variable name from the assign-stmt.
(define (rewrite-returns body-stmts target-name param-map)
  (map (lambda (stmt)
         (let ((substituted (subst-idents stmt param-map)))
           (ast-transform substituted
             (lambda (n)
               (and (tag? n 'return-stmt)
                    (let ((results (nf n 'results)))
                      (and (pair? results)
                           ;; return <val> → target = <val>
                           (list 'assign-stmt
                                 (cons 'lhs (list (list 'ident
                                                        (cons 'name target-name))))
                                 (cons 'tok ':=)
                                 (cons 'rhs results)))))))))
       body-stmts))

;; List take/drop (not in utils)
(define (take lst n)
  (if (or (= n 0) (null? lst)) '()
    (cons (car lst) (take (cdr lst) (- n 1)))))

(define (drop lst n)
  (if (or (= n 0) (null? lst)) lst
    (drop (cdr lst) (- n 1))))

;; Splice inlined body into a block, replacing the assign-stmt.
;; This is the manual version of the missing primitive.
(define (splice-inline-into-block block-node assign-idx inlined-stmts)
  (let* ((stmts (nf block-node 'list))
         (before (take stmts assign-idx))
         (after (drop stmts (+ assign-idx 1)))
         (new-stmts (append before inlined-stmts after)))
    (list 'block (cons 'list new-stmts))))

;; Find assign-stmts with inlinable calls on the RHS.
;; Returns: ((index target-name callee-name) ...) or '()
(define (find-assign-calls block-stmts file)
  (let loop ((stmts block-stmts) (idx 0) (acc '()))
    (if (null? stmts) (reverse acc)
      (let* ((stmt (car stmts))
             (found
               (and (tag? stmt 'assign-stmt)
                    (let* ((rhs (nf stmt 'rhs))
                           (lhs (nf stmt 'lhs))
                           (call (and (pair? rhs) (car rhs)))
                           (target (and (pair? lhs)
                                        (tag? (car lhs) 'ident)
                                        (nf (car lhs) 'name)))
                           (callee-name
                             (and (tag? call 'call-expr)
                                  (let ((fun (nf call 'fun)))
                                    (and (tag? fun 'ident)
                                         (nf fun 'name)))))
                           (callee (and callee-name
                                        (find-func-by-name file callee-name))))
                      (and callee target
                           ;; Only multi-statement bodies (single-expr
                           ;; already handled by Phase 1)
                           (not (single-return-expr callee))
                           (list idx target callee-name call callee))))))
        (loop (cdr stmts) (+ idx 1)
              (if found (cons found acc) acc))))))

;; Phase 2: inline multi-statement calls in assign position.
;; Process from last to first to preserve indices.
(define (inline-stmt-calls fn file)
  (let* ((body (nf fn 'body))
         (stmts (nf body 'list))
         (targets (find-assign-calls stmts file)))
    (if (null? targets) fn
      ;; Process in reverse order to preserve indices
      (let loop ((ts (reverse targets)) (current-body body))
        (if (null? ts)
          ;; Rebuild func-decl with new body
          (cons (car fn)
                (map (lambda (field)
                       (if (and (pair? field) (eq? (car field) 'body))
                         (cons 'body current-body)
                         field))
                     (cdr fn)))
          (let* ((t (car ts))
                 (idx (list-ref t 0))
                 (target-name (list-ref t 1))
                 (call-node (list-ref t 3))
                 (callee (list-ref t 4))
                 (formals (formal-param-names callee))
                 (actuals (or (nf call-node 'args) '()))
                 (param-map (build-param-map formals actuals))
                 (callee-stmts (nf (nf callee 'body) 'list))
                 (rewritten (rewrite-returns callee-stmts
                                             target-name param-map))
                 (spliced (splice-inline-into-block
                            current-body idx rewritten)))
            (loop (cdr ts) spliced)))))))

;; ── Run Phase 2 ──────────────────────────────────

(define phase2 (inline-stmt-calls phase1 file-ast))

(display "── Phase 2: statement-level inlining ─────") (newline)
(display "   (clamp inlined into compute)")
(newline) (newline)
(display (go-format phase2)) (newline) (newline)

;; ── Token cost summary ───────────────────────────

(display "── Token cost analysis ───────────────────") (newline)
(display "  ast-transform (missing primitive):   15 lines") (newline)
(display "  subst-idents (uses ast-transform):    5 lines") (newline)
(display "  Phase 1 (expr-level inline):         20 lines") (newline)
(display "  Phase 2 (stmt-level inline):         75 lines") (newline)
(display "  Scaffolding (parse, extract, run):   30 lines") (newline)
(display "  take/drop (missing from utils):       5 lines") (newline)
(display "  TOTAL:                              ~150 lines") (newline)
(newline)
(display "  With ast-transform + ast-splice primitives:") (newline)
(display "  Estimated:                          ~60 lines") (newline)
(newline)
(display "  Known limitations:") (newline)
(display "    - No fresh name generation (collision risk)") (newline)
(display "    - return-rewriting is naive (early returns") (newline)
(display "      produce wrong control flow — need goto or") (newline)
(display "      if/else restructuring)") (newline)
(display "    - Only handles local calls (no method calls,") (newline)
(display "      no cross-package)") (newline)
(display "    - No recursion detection") (newline)
