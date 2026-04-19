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

;;; belief-checkers.scm — Property checkers and their SSA helpers
;;;
;;; Included by belief.sld alongside belief.scm. All definitions land in
;;; the same (wile goast belief) library namespace — this is a file split
;;; for cohesion, not a separate library.
;;;
;;; Each checker returns (lambda (site ctx) -> category-symbol).
;;; The majority category becomes the belief; minorities are deviations.
;;;
;;; Dependencies on belief.scm:
;;;   ctx-field-index, ctx-find-ssa-func  (analysis context accessors)
;;;   find-field-summary, writes-for-struct, all-present?  (ctx helpers)
;;; Dependencies on (wile goast utils): nf, tag?, walk, filter-map, opt-ref
;;; Dependencies on (wile goast dataflow): defuse-reachable?

;; ── SSA helpers ─────────────────────────────────────────

;; Look up all field names for a struct by name from AST packages.
(define (struct-field-names pkgs struct-name)
  (let loop ((ps (if (pair? pkgs) pkgs '())))
    (if (null? ps) '()
      (let file-loop ((files (let ((fs (nf (car ps) 'files)))
                               (if (pair? fs) fs '()))))
        (if (null? files) (loop (cdr ps))
          (let ((found (walk (car files)
                   (lambda (node)
                     (and (tag? node 'type-spec)
                          (equal? (nf node 'name) struct-name)
                          (let ((stype (nf node 'type)))
                            (and (tag? stype 'struct-type)
                                 (let ((fields (nf stype 'fields)))
                                   (flat-map
                                     (lambda (f)
                                       (if (tag? f 'field)
                                         (let ((ns (nf f 'names)))
                                           (if (pair? ns) ns '()))
                                         '()))
                                     (if (pair? fields) fields '()))))))))))
            (if (pair? found)
              (car found)
              (file-loop (cdr files)))))))))

;; ── Property checkers ───────────────────────────────────

;; (paired-with op-a op-b) — checks if function body contains both
;; operations, with preference for defer pairing.
;; Returns: 'paired-defer, 'paired-call, or 'unpaired
(define (paired-with op-a op-b)
  "Property checker: verify that calls to OP-A are paired with OP-B.\nReturns 'paired-defer if paired via defer, 'paired-call if paired\nvia regular call, or 'unpaired.\n\nParameters:\n  op-a : string\n  op-b : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (paired-with \"Lock\" \"Unlock\")\n  (paired-with \"Open\" \"Close\")\n\nSee also: `contains-call', `ordered'."
  (lambda (site ctx)
    (let* ((body (or (nf site 'body) '()))
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

;; (ordered op-a op-b) — checks whether op-a's SSA block dominates op-b's block.
;; Uses SSA representation (blocks have instrs + idom). Does not require go-cfg.
;; Returns: 'a-dominates-b, 'b-dominates-a, 'same-block, 'unordered, 'missing,
;; or 'malformed-ssa (idom chain is broken — data error distinct from
;; 'unordered's "no dominance relationship" verdict).
(define (ordered op-a op-b)
  "Property checker: verify that OP-A's SSA block dominates OP-B's block.\nReturns 'a-dominates-b, 'b-dominates-a, 'same-block, 'unordered, 'missing,\nor 'malformed-ssa (idom chain is broken — distinct from 'unordered's\nlegitimate 'no dominance' verdict).\n\nParameters:\n  op-a : string\n  op-b : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (ordered \"Validate\" \"Execute\")\n\nSee also: `paired-with', `checked-before-use'."
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if (not ssa-fn) 'missing
        (let* ((blocks (nf ssa-fn 'blocks))
               (a-blocks (find-ssa-call-blocks blocks op-a))
               (b-blocks (find-ssa-call-blocks blocks op-b)))
          (cond
            ((or (null? a-blocks) (null? b-blocks)) 'missing)
            ((= (car a-blocks) (car b-blocks))
             (let* ((blk-idx (car a-blocks))
                    (block (let find ((bs (if (pair? blocks) blocks '())))
                             (cond ((null? bs) #f)
                                   ((= (nf (car bs) 'index) blk-idx) (car bs))
                                   (else (find (cdr bs))))))
                    (pos-a (and block (find-call-position block op-a)))
                    (pos-b (and block (find-call-position block op-b))))
               (cond
                 ((or (not pos-a) (not pos-b)) 'unordered)
                 ((< pos-a pos-b) 'a-dominates-b)
                 (else 'b-dominates-a))))
            (else
              (let ((ab (ssa-dominates? blocks (car a-blocks) (car b-blocks)))
                    (ba (ssa-dominates? blocks (car b-blocks) (car a-blocks))))
                (cond
                  ((or (eq? ab 'malformed-idom) (eq? ba 'malformed-idom))
                   'malformed-ssa)
                  (ab 'a-dominates-b)
                  (ba 'b-dominates-a)
                  (else 'unordered))))))))))

;; Find SSA block indices containing a call to the named function.
;; Walks SSA instructions (not AST statements). Checks both static
;; calls (func field) and method calls (method field).
(define (find-ssa-call-blocks blocks func-name)
  (filter-map
    (lambda (block)
      (let ((idx (nf block 'index))
            (instrs (or (nf block 'instrs) '())))
        (and (pair? (walk instrs
               (lambda (node)
                 (and (or (tag? node 'ssa-call) (tag? node 'ssa-go)
                          (tag? node 'ssa-defer))
                      (or (equal? (nf node 'func) func-name)
                          (equal? (nf node 'method) func-name))))))
             idx)))
    (if (pair? blocks) blocks '())))

;; Find the instruction index of the first call to func-name in a block.
;; Returns the 0-based position in the block's instrs list, or #f.
(define (find-call-position block func-name)
  (let ((instrs (or (nf block 'instrs) '())))
    (let loop ((is instrs) (pos 0))
      (cond
        ((null? is) #f)
        ((and (or (tag? (car is) 'ssa-call) (tag? (car is) 'ssa-go)
                  (tag? (car is) 'ssa-defer))
              (or (equal? (nf (car is) 'func) func-name)
                  (equal? (nf (car is) 'method) func-name)))
         pos)
        (else (loop (cdr is) (+ pos 1)))))))

;; Check whether block a-idx dominates block b-idx using the idom chain.
;; Walks b's immediate dominator chain upward. Returns:
;;   #t              — a dominates b (found in chain)
;;   #f              — b's chain reached entry block without a
;;   'malformed-idom — chain references a block not in block-map, or a
;;                     non-entry block has no idom field
;; By SSA convention, the entry block (index 0) has no idom field OR
;; idom == itself. Missing idom on any other block signals data error.
(define (ssa-dominates? blocks a-idx b-idx)
  (let ((block-map (map (lambda (b) (cons (nf b 'index) b))
                        (if (pair? blocks) blocks '()))))
    (let loop ((current b-idx))
      (cond
        ((= current a-idx) #t)
        (else
          (let ((entry (assoc current block-map)))
            (cond
              ((not entry) 'malformed-idom)  ;; block referenced but missing
              (else
                (let ((idom (nf (cdr entry) 'idom)))
                  (cond
                    ;; idom missing or same as self: end of chain
                    ((or (not idom) (= idom current))
                     (if (= current 0) #f 'malformed-idom))
                    (else (loop idom))))))))))))

;; (co-mutated field ...) — checks whether all named fields are stored
;; together in the function. Uses the pre-built field index from Go.
;; Returns: 'co-mutated, 'partial, or 'missing
(define (co-mutated . field-names)
  "Property checker: verify that all named fields are stored together.\nReturns 'co-mutated if all fields written, 'partial otherwise.\nSkips receiver disambiguation -- stores-to-fields already filtered.\n\nParameters:\n  field-names : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (co-mutated \"Host\" \"Port\" \"Scheme\")\n\nSee also: `stores-to-fields'."
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (summary (find-field-summary (ctx-field-index ctx) pkg-path fname)))
      (if (not summary) 'missing
        (let ((writes (writes-for-struct summary #f)))
          (if (all-present? field-names writes)
            'co-mutated
            'partial))))))

;; (checked-before-use value-pattern) — checks whether a value is
;; tested before use via bounded transitive reachability on the SSA
;; def-use graph. Uses (wile algebra) fixpoint over a product lattice
;; (powerset x boolean) for early exit when the guard is found.
;; Returns: 'guarded, 'unguarded, or 'missing (SSA lookup failed)
(define (checked-before-use value-pattern . opts)
  "Property checker: verify that a value matching VALUE-PATTERN is tested\nbefore use. Uses bounded def-use reachability to check whether the value\nflows through a comparison before reaching a non-guard use.\nReturns 'guarded or 'unguarded.\n\nParameters:\n  value-pattern : string\n  opts : keyword list — optional 'fuel N (default 5, max def-use hops + 1)\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (checked-before-use \"err\")\n  (checked-before-use \"err\" 'fuel 10)\n\nSee also: `ordered', `defuse-reachable?'."
  (define fuel (opt-ref opts 'fuel 5)) ;; max-hops + 1: fixpoint needs one extra iteration to confirm convergence
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (cond
        ((not ssa-fn) 'missing)
        ((defuse-reachable? ssa-fn (list value-pattern)
                            (lambda (i) (tag? i 'ssa-if)) fuel)
         'guarded)
        (else 'unguarded)))))

;; (custom proc) — escape hatch. proc is (lambda (site ctx) -> symbol).
(define (custom proc)
  "Property checker: escape hatch for user-defined checks.\nPROC receives (site ctx) and returns a symbol categorizing the result.\nSite is a func-decl AST node (tagged alist). Common fields:\n  (nf site 'name)      => function name string\n  (nf site 'body)      => function body AST (list of statements)\n  (nf site 'recv)      => receiver list (methods) or #f (functions)\n  (nf site 'type)      => function type node with params, results\n  (nf site 'pkg-path)  => import path of the containing package\nCtx is the analysis context. Use ctx-pkgs, ctx-ssa, ctx-callgraph,\nctx-find-ssa-func to access loaded analysis data.\n\nParameters:\n  proc : procedure\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (custom (lambda (site ctx)\n    (if (nf site 'recv) 'is-method 'is-function)))\n  (custom (lambda (site ctx)\n    (let ((ssa (ctx-find-ssa-func ctx (nf site 'pkg-path) (nf site 'name))))\n      (if ssa 'has-ssa 'no-ssa))))\n\nSee also: `functions-matching', `ctx-find-ssa-func', `nf'."
  proc)

(define (aggregate-custom proc)
  "Aggregate analyzer: escape hatch for user-defined analyzers.\nPROC receives (sites ctx) and returns a result alist.\nSites is the list of func-decl nodes from the (sites ...) clause.\nCtx is the analysis context.\n\nParameters:\n  proc : procedure\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (define-aggregate-belief \"my-check\"\n    (sites (all-functions-in))\n    (analyze (aggregate-custom\n      (lambda (sites ctx)\n        (list (cons 'type 'aggregate)\n              (cons 'verdict 'OK))))))\n\nSee also: `custom', `define-aggregate-belief'."
  proc)
