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
      (let* ((verdict (cond (has-defer-b 'paired-defer)
                            (has-call-b  'paired-call)
                            (else        'unpaired)))
             (fname (nf site 'name))
             (pkg-path (nf site 'pkg-path))
             (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname)))
             (pos-a (and ssa-fn (ssa-func-call-position ssa-fn op-a)))
             (pos-b (and ssa-fn (ssa-func-call-position ssa-fn op-b))))
        ;; where = op-a's call (the operation needing pairing; for 'unpaired,
        ;; exactly the bug site). The paired op-b position rides in `why`.
        (if (not pos-a) verdict
          (cons verdict
                (list (cons 'where pos-a)
                      (cons 'why (list 'paired (cons 'a op-a) (cons 'b op-b)
                                       (cons 'relation verdict)
                                       (cons 'a-pos pos-a) (cons 'b-pos pos-b)))
                      (cons 'score #f))))))))

;; (ordered op-a op-b) — checks whether op-a's SSA block dominates op-b's block.
;; Uses SSA representation (blocks have instrs + idom). Does not require go-cfg.
;; Returns: 'a-dominates-b, 'b-dominates-a, 'unordered, 'missing, or
;; 'malformed-ssa (idom chain is broken — data error distinct from 'unordered's
;; "no dominance relationship" verdict). The a/b-dominates verdicts carry an
;; evidence tail when the call positions resolve: (verdict . ((where . W)
;; (why . Y) (score . S))) with the two source positions in `why`. Other
;; verdicts stay bare symbols (no two-position evidence to carry).
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
          ;; with-evidence: attach the recovered call source positions to a
          ;; verdict. A-BLOCK / B-BLOCK are the ssa-blocks holding the op-a /
          ;; op-b calls; (ssa-call-position block op) resolves "file:line:col"
          ;; (or #f). This is the reason a human opens an editor -- ordered
          ;; computes it and (pre-slice-3) threw it away.
          ;;
          ;; Returns (verdict . evidence) with evidence
          ;;   ((where . W) (why . Y) (score . S)):
          ;;   * where -- the *dominating* call's position (op-a for
          ;;     a-dominates-b, op-b for b-dominates-a): the site a human jumps
          ;;     to first. A finding has one position; the pair lives in `why`.
          ;;   * why -- structured (ordered (a . op-a) (b . op-b)
          ;;     (relation . verdict) (a-pos . W-a) (b-pos . W-b)): render-why
          ;;     projects it to a string; downstream Scheme filters on the tag.
          ;;   * score -- #f: ordering has no natural confidence (design Q4).
          ;; When neither call resolves to a position, return the bare verdict --
          ;; no fabricated location; the finding is honestly unlocated.
          (define (with-evidence verdict a-block b-block)
            (let* ((pos-a (and a-block (ssa-call-position a-block op-a)))
                   (pos-b (and b-block (ssa-call-position b-block op-b)))
                   (where (cond ((eq? verdict 'a-dominates-b) pos-a)
                                ((eq? verdict 'b-dominates-a) pos-b)
                                (else (or pos-a pos-b)))))
              (if (and (not pos-a) (not pos-b))
                  verdict
                  (cons verdict
                        (list (cons 'where where)
                              (cons 'why (list 'ordered
                                               (cons 'a op-a)
                                               (cons 'b op-b)
                                               (cons 'relation verdict)
                                               (cons 'a-pos pos-a)
                                               (cons 'b-pos pos-b)))
                              (cons 'score #f))))))
          (cond
            ((or (null? a-blocks) (null? b-blocks)) 'missing)
            ((= (car a-blocks) (car b-blocks))
             (let* ((block (block-by-index blocks (car a-blocks)))
                    (pos-a (and block (find-call-position block op-a)))
                    (pos-b (and block (find-call-position block op-b))))
               (cond
                 ((or (not pos-a) (not pos-b)) 'unordered)
                 ((< pos-a pos-b) (with-evidence 'a-dominates-b block block))
                 (else (with-evidence 'b-dominates-a block block)))))
            (else
              (let ((ab (ssa-dominates? blocks (car a-blocks) (car b-blocks)))
                    (ba (ssa-dominates? blocks (car b-blocks) (car a-blocks))))
                (cond
                  ((or (eq? ab 'malformed-idom) (eq? ba 'malformed-idom))
                   'malformed-ssa)
                  (ab (with-evidence 'a-dominates-b
                        (block-by-index blocks (car a-blocks))
                        (block-by-index blocks (car b-blocks))))
                  (ba (with-evidence 'b-dominates-a
                        (block-by-index blocks (car a-blocks))
                        (block-by-index blocks (car b-blocks))))
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

;; block-by-index: the ssa-block in BLOCKS whose 'index is IDX, or #f.
(define (block-by-index blocks idx)
  (let loop ((bs (if (pair? blocks) blocks '())))
    (cond ((null? bs) #f)
          ((= (nf (car bs) 'index) idx) (car bs))
          (else (loop (cdr bs))))))

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

;; (dominates-call op-a op-b) — checks whether calls to OP-A dominate EVERY call
;; to OP-B in the site function. This is the multi-site generalization of
;; `ordered`: `ordered` compares only the FIRST call block of each op, so when
;; OP-B appears in more than one branch (e.g. a callback applied in both arms of
;; a mode dispatch) it verifies dominance for just one of them. `dominates-call`
;; requires that EVERY block containing an OP-B call be dominated by SOME block
;; containing an OP-A call — the correct primitive for "the capture dominates
;; every arm." Built on the same SSA idom machinery as `ordered`
;; (`find-ssa-call-blocks` + `ssa-dominates?`); SSA idom is the CFG dominator
;; tree, so no separate go-cfg pass is needed. Block-granular: an OP-A call in
;; the SAME block as an OP-B call counts as dominating it (use `ordered` for
;; intra-block textual order). Returns:
;;   'dominates-all — every OP-B block is dominated by some OP-A block
;;   'partial       — some but not all OP-B blocks are dominated
;;   'none          — no OP-B block is dominated by any OP-A block
;;   'missing       — OP-A or OP-B has no call site in the function
;;   'malformed-ssa — an idom chain is broken (data error, not a real verdict)
(define (dominates-call op-a op-b)
  "Property checker: verify that calls to OP-A dominate EVERY call to OP-B.\nThe multi-site generalization of `ordered' (which checks only the first call\nblock of each op): when OP-B appears in multiple branches, every OP-B block\nmust be dominated by some OP-A block. Returns 'dominates-all, 'partial, 'none,\n'missing (OP-A or OP-B absent), or 'malformed-ssa (broken idom chain).\nBlock-granular — same-block counts as dominating; use `ordered' for intra-block\norder.\n\nParameters:\n  op-a : string\n  op-b : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (dominates-call \"SliceContinuationAt\" \"ApplyCallable\")\n\nSee also: `ordered', `reaches-call', `checked-before-use'."
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if (not ssa-fn) 'missing
        (let* ((blocks (nf ssa-fn 'blocks))
               (a-blocks (find-ssa-call-blocks blocks op-a))
               (b-blocks (find-ssa-call-blocks blocks op-b)))
          (if (or (null? a-blocks) (null? b-blocks)) 'missing
            ;; Fold over b-blocks: count how many are dominated by some a-block.
            ;; A broken idom chain becomes 'malformed-ssa (a data error distinct
            ;; from a legitimate non-dominance verdict), matching `ordered'. The
            ;; malformed flag only matters when no clean dominator is found — a
            ;; #t dominance short-circuits the scan.
            (let loop ((bs b-blocks) (dom-count 0) (malformed #f))
              (if (null? bs)
                (cond (malformed 'malformed-ssa)
                      ((= dom-count (length b-blocks)) 'dominates-all)
                      ((> dom-count 0) 'partial)
                      (else 'none))
                (let scan ((as a-blocks) (mal malformed))
                  (if (null? as)
                    (loop (cdr bs) dom-count mal)
                    (let ((r (ssa-dominates? blocks (car as) (car bs))))
                      (cond
                        ((eq? r #t) (loop (cdr bs) (+ dom-count 1) mal))
                        ((eq? r 'malformed-idom) (scan (cdr as) #t))
                        (else (scan (cdr as) mal))))))))))))))

;; ── Value-flow checker ──────────────────────────────────
;;
;; (flows-to-all op-a op-b) — the value-flow analog of `dominates-call`. Where
;; `dominates-call` asks "does the capture DOMINATE every callback arm?"
;; (control), `flows-to-all` asks "does the SAME captured VALUE reach every
;; callback arm?" (data). It is the checker behind wile finding #9's belief B2:
;; the captured continuation handed to the callback must be the same value in
;; both `PrimCallCC` mode arms — neither arm may re-capture or substitute it.
;;
;; Built on `value-flow-reached`, which follows def-use edges INCLUDING flow
;; through aggregates (a store through an element/field address taints the
;; backing aggregate). That aggregate-alias edge is essential: a continuation
;; passed variadically (`mc.ApplyCallable(mcls, capt)`) is packed into a backing
;; array and handed over as a slice, a path a plain `defuse-reachable?` cannot
;; see (it returns a false negative there — the motivating gap for this checker).
;;
;; Verdict is per-source, mirroring `dominates-call`'s "every OP-B dominated by
;; SOME OP-A": the shared-value law holds iff a SINGLE OP-A def's value reaches
;; EVERY OP-B call. Two separate captures each feeding one arm is the violation.
;;   'flows-all — some single OP-A def's value reaches every OP-B call site
;;   'partial   — OP-B calls receive an OP-A value, but no single OP-A def
;;                reaches all of them (an arm re-captures / substitutes), or some
;;                OP-B site is unreached
;;   'none      — no OP-B call receives any OP-A value
;;   'missing   — OP-A or OP-B has no call site in the function
;;
;; An OP-B call "receives" a source iff one of its operands is in that source's
;; value-flow-reached set. Block-agnostic: this is data flow, not dominance —
;; pair with `dominates-call` for the control half.
(define (ssa-call-to? i op)
  (and (or (tag? i 'ssa-call) (tag? i 'ssa-go) (tag? i 'ssa-defer))
       (or (equal? (nf i 'func) op) (equal? (nf i 'method) op))))

(define (flows-to-all op-a op-b)
  "Property checker: verify the same OP-A value reaches EVERY OP-B call.\nThe value-flow analog of `dominates-call': a single OP-A def's value must reach\nevery OP-B call site (via def-use, including flow through aggregates such as\nvariadic argument packing). Returns 'flows-all, 'partial (OP-B receives an OP-A\nvalue but no single OP-A def reaches all — an arm re-captures/substitutes),\n'none, or 'missing (OP-A or OP-B absent).\n\nParameters:\n  op-a : string\n  op-b : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (flows-to-all \"NewCapturedContinuation\" \"ApplyCallable\")\n\nSee also: `dominates-call', `checked-before-use', `reaches-call'."
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if (not ssa-fn) 'missing
        (let* ((instrs (ssa-all-instrs ssa-fn))
               (a-defs (filter-map
                         (lambda (i) (and (ssa-call-to? i op-a) (nf i 'name)))
                         instrs))
               (b-sites (filter (lambda (i) (ssa-call-to? i op-b)) instrs)))
          (if (or (null? a-defs) (null? b-sites)) 'missing
            (let* ((n-b (length b-sites))
                   (receives?
                     (lambda (b-site reached)
                       (let loop ((os (or (nf b-site 'operands) '())))
                         (cond ((null? os) #f)
                               ((member? (car os) reached) #t)
                               (else (loop (cdr os)))))))
                   (count-recv
                     (lambda (reached)
                       (length (filter (lambda (s) (receives? s reached)) b-sites))))
                   (some-reaches-all
                     (let loop ((ds a-defs))
                       (cond ((null? ds) #f)
                             ((= n-b (count-recv (value-flow-reached ssa-fn (list (car ds))))) #t)
                             (else (loop (cdr ds))))))
                   (union-recv (count-recv (value-flow-reached ssa-fn a-defs))))
              (cond (some-reaches-all 'flows-all)
                    ((> union-recv 0) 'partial)
                    (else 'none)))))))))

;; ── Transitive-reachability checker ─────────────────────
;;
;; Map a site's AST func-decl to its call-graph node name. Call-graph node
;; names are Form-3 qualified ("pkg.Func" or "(*pkg.T).Method"). A method
;; func-decl's 'name is already qualified in that form, so try an exact match
;; first. A top-level function's 'name is bare ("Func"), so fall back to a
;; suffix match ("." + name lands after both the package dot and a "(*T)."
;; receiver), constrained by pkg-path to disambiguate same-named functions
;; across packages. Returns the matching node name, or #f.
(define (string-ends-with? suffix s)
  (let ((sl (string-length suffix)) (n (string-length s)))
    (and (<= sl n) (string=? suffix (substring s (- n sl) n)))))

(define (cg-node-name-for cg site)
  (let* ((nm (nf site 'name))
         (pkg-path (nf site 'pkg-path))
         (suffix (string-append "." nm)))
    (let loop ((ns (if (pair? cg) cg '())) (suffix-hit #f))
      (cond ((null? ns) suffix-hit)
            (else
             (let ((qn (nf (car ns) 'name)))
               (cond
                 ((not qn) (loop (cdr ns) suffix-hit))
                 ;; exact match wins immediately (qualified method name)
                 ((equal? qn nm) qn)
                 ;; else remember the first pkg-scoped suffix match (bare func)
                 ((and (not suffix-hit)
                       (string-ends-with? suffix qn)
                       (or (not pkg-path) (string-contains qn pkg-path)))
                  (loop (cdr ns) qn))
                 (else (loop (cdr ns) suffix-hit)))))))))

;; Callee names (edges-out) of the call-graph node named NM, or '() if absent.
(define (cg-callees cg nm)
  (let loop ((ns (if (pair? cg) cg '())))
    (cond ((null? ns) '())
          ((equal? (nf (car ns) 'name) nm)
           (map (lambda (e) (nf e 'callee)) (nf (car ns) 'edges-out)))
          (else (loop (cdr ns))))))

;; BFS the call graph from START (a qualified node name): does any reachable
;; callee's SHORT name equal TARGET-SHORT? Short-name match lets callers pass
;; "MarkChainShared" without the "(*pkg.T)." prefix.
(define (cg-reaches-short? cg start target-short)
  (let loop ((frontier (list start)) (seen '()))
    (cond ((null? frontier) #f)
          ((member (car frontier) seen) (loop (cdr frontier) seen))
          (else
           (let* ((cur (car frontier))
                  (callees (cg-callees cg cur)))
             (if (let scan ((cs callees))
                   (cond ((null? cs) #f)
                         ((equal? (ssa-short-name (car cs)) target-short) #t)
                         (else (scan (cdr cs)))))
                 #t
                 (loop (append callees (cdr frontier)) (cons cur seen))))))))

;; (reaches-call target) — checks whether the site function transitively calls
;; TARGET (matched by short name) anywhere in its call graph. Unlike
;; contains-call (direct calls in the body only), this follows edges through
;; the whole reachable sub-graph, so a site that delegates the call to a helper
;; still counts. Returns 'reaches or 'unreached ('unresolved if the site has no
;; call-graph node — e.g. fully-inlined or generated code).
(define (reaches-call target)
  "Property checker: verify that the site function transitively reaches a call\nto TARGET (short name) through its call graph. Follows edges, so a site that\ndelegates TARGET to a helper still adheres. Returns 'reaches, 'unreached, or\n'unresolved (site absent from the call graph).\n\nParameters:\n  target : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (reaches-call \"MarkChainShared\")\n\nSee also: `contains-call', `paired-with', `callers-of'."
  (lambda (site ctx)
    (let* ((cg (ctx-callgraph ctx))
           (start (cg-node-name-for cg site)))
      (cond ((not start) 'unresolved)
            ((cg-reaches-short? cg start target) 'reaches)
            (else 'unreached)))))

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
        (let* ((writes (writes-for-struct summary #f))
               (verdict (if (all-present? field-names writes) 'co-mutated 'partial))
               (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname)))
               (pos (and ssa-fn
                         (ssa-first-pos ssa-fn
                           (lambda (i) (and (tag? i 'ssa-field-addr)
                                            (member? (nf i 'field) field-names)))))))
          (if (not pos) verdict
            (cons verdict
                  (list (cons 'where pos)
                        (cons 'why (list 'co-mutated (cons 'fields field-names)
                                         (cons 'relation verdict)))
                        (cons 'score #f)))))))))

;; Collect the set of method names declared by any interface type in the
;; loaded packages. Conservative ¬IM proxy for receiver-parameter-asymmetry:
;; an interface-type node's 'methods is a list of field nodes, each field's
;; 'names a list of method-name strings. Over-excludes (a method named Get is
;; spared even if its interface is single-impl) — fewer false candidates.
(define (interface-method-names ctx)
  (let ((acc '()))
    (for-each
      (lambda (pkg)
        (walk pkg
          (lambda (node)
            (and (tag? node 'interface-type)
                 (let ((methods (nf node 'methods)))
                   (when (pair? methods)
                     (for-each
                       (lambda (fld)
                         (let ((names (nf fld 'names)))
                           (when (pair? names)
                             (for-each (lambda (n) (set! acc (cons n acc))) names))))
                       methods))
                   #f)))))
      (ctx-pkgs ctx))
    (unique acc)))

;; True iff instruction I is a call into which register RV flows *directly*:
;; the receiver of an invoke (interface) call, or any argument of a static
;; call. This is the forwarder/delegation signal — the receiver read is the
;; thing the call operates on, not data combined with a parameter. The
;; discriminator works because subject-inversion's canonical form feeds the
;; receiver read through boxing first: a variadic/interface argument is
;; make-interface'd before the call, so it is NOT a direct call operand,
;; whereas a delegated call takes the receiver-read value verbatim
;; (`(ssa-call (args "t1" "k") ...)`). SSA's valName erases method-vs-function
;; (a static method's func is the bare "Get"), so position, not callee kind,
;; is the signal. Limitation: a subject-inversion passing the receiver field
;; *unboxed* directly to a plain function folds into 'forwarder — conservative
;; (fewer false candidates), per the design's stated preference.
(define (forwarder-call? i rv)
  (and (tag? i 'ssa-call)
       (or (and (eq? (nf i 'mode) 'invoke) (equal? (nf i 'recv) rv))
           (let ((args (nf i 'args)))
             (and (list? args) (member? rv args))))))

;; (receiver-parameter-asymmetry) — flags methods whose receiver is read
;; exactly once, written never, with at least one non-receiver parameter:
;; the "receiver as namespace" anti-pattern (Connascence of Meaning hidden
;; by method syntax). The single receiver read is a convert-to-function
;; signal. See plans/2026-04-20-receiver-parameter-asymmetry-design.md.
;; Receiver field reads are ssa-field-addr/ssa-field whose x is the receiver
;; param; a receiver ssa-field-addr whose register is some ssa-store's addr
;; is a write, not a read. Returns one of:
;;   'candidate    — read set singleton, write set empty, >=1 param (the flag)
;;   'mutation     — a receiver field is written (receiver is state-bearing)
;;   'accessor     — zero non-receiver parameters
;;   'multi-read   — more than one distinct receiver field read
;;   'unused-recv  — receiver never read (pure namespace / dispatch)
;;   'no-receiver  — not a method, or SSA receiver not resolvable
;;   'missing      — SSA lookup failed
;; The 'candidate verdict carries located evidence (the receiver read);
;; other verdicts stay bare (located only where it matters, as paired-with).
(define (receiver-parameter-asymmetry)
  "Property checker: flag receiver-as-namespace methods.\nReturns 'candidate (read 1 field, write none, has params), 'mutation,\n'accessor, 'multi-read, 'unused-recv, 'no-receiver, or 'missing.\nThe 'candidate verdict carries the located receiver read as evidence.\n\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (receiver-parameter-asymmetry)\n\nSee also: `co-mutated', `stores-to-fields'."
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if (not ssa-fn) 'missing
        (let* ((params (nf ssa-fn 'params))
               (recv (and (pair? params) (car params)))
               (recv-name (and recv (nf recv 'name)))
               (nparam (if (pair? params) (- (length params) 1) 0)))
          (if (not recv-name) 'no-receiver
            (let* ((instrs (ssa-all-instrs ssa-fn))
                   (store-addrs
                     (filter-map (lambda (i) (and (tag? i 'ssa-store) (nf i 'addr)))
                                 instrs))
                   ;; receiver field-addr instrs as (field . register)
                   (recv-faddr
                     (filter-map
                       (lambda (i)
                         (and (tag? i 'ssa-field-addr)
                              (equal? (nf i 'x) recv-name)
                              (cons (nf i 'field) (nf i 'name))))
                       instrs))
                   ;; value-receiver direct field reads (field names)
                   (recv-field
                     (filter-map
                       (lambda (i)
                         (and (tag? i 'ssa-field)
                              (equal? (nf i 'x) recv-name)
                              (nf i 'field)))
                       instrs))
                   (write-fields
                     (unique (filter-map
                               (lambda (fa) (and (member? (cdr fa) store-addrs) (car fa)))
                               recv-faddr)))
                   (read-fields
                     (unique (append
                               (filter-map
                                 (lambda (fa) (and (not (member? (cdr fa) store-addrs)) (car fa)))
                                 recv-faddr)
                               recv-field))))
              (cond
                ((positive? (length write-fields)) 'mutation)
                ((= nparam 0) 'accessor)
                ((= (length read-fields) 0) 'unused-recv)
                ((> (length read-fields) 1) 'multi-read)
                ((member? (ssa-short-name fname) (interface-method-names ctx))
                 'interface-method)
                (else
                  (let* ((field (car read-fields))
                         ;; field-addr register for this field (pointer receiver)
                         (fa (let loop ((xs recv-faddr))
                               (cond ((null? xs) #f)
                                     ((equal? (car (car xs)) field) (cdr (car xs)))
                                     (else (loop (cdr xs))))))
                         ;; the receiver-read VALUE register: value-receiver
                         ;; reads it directly (ssa-field); pointer receivers
                         ;; load the field address (ssa-unop * on fa).
                         (rv (or
                               (let loop ((is instrs))
                                 (cond ((null? is) #f)
                                       ((and (tag? (car is) 'ssa-field)
                                             (equal? (nf (car is) 'x) recv-name)
                                             (equal? (nf (car is) 'field) field))
                                        (nf (car is) 'name))
                                       (else (loop (cdr is)))))
                               (and fa
                                    (let loop ((is instrs))
                                      (cond ((null? is) #f)
                                            ((and (tag? (car is) 'ssa-unop)
                                                  (equal? (nf (car is) 'x) fa))
                                             (nf (car is) 'name))
                                            (else (loop (cdr is))))))))
                         (forwarder?
                           (and rv
                                (let loop ((is instrs))
                                  (cond ((null? is) #f)
                                        ((forwarder-call? (car is) rv) #t)
                                        (else (loop (cdr is)))))))
                         (pos (ssa-first-pos ssa-fn
                                (lambda (i)
                                  (and (or (tag? i 'ssa-field-addr) (tag? i 'ssa-field))
                                       (equal? (nf i 'x) recv-name)
                                       (equal? (nf i 'field) field))))))
                    (cond
                      (forwarder? 'forwarder)
                      ((not pos) 'candidate)
                      (else
                        (cons 'candidate
                              (list (cons 'where pos)
                                    (cons 'why (list 'receiver-asymmetry
                                                     (cons 'field field)
                                                     (cons 'receiver recv-name)
                                                     (cons 'relation 'candidate)))
                                    (cons 'score #f)))))))))))))))

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
         ;; located at the comparison feeding the guard (the ssa-if itself
         ;; carries no position; the ssa-binop that computes the test does).
         (let ((pos (or (ssa-first-pos ssa-fn (lambda (i) (tag? i 'ssa-binop)))
                        (ssa-first-pos ssa-fn
                          (lambda (i) (equal? (nf i 'name) value-pattern))))))
           (if (not pos) 'guarded
             (cons 'guarded
                   (list (cons 'where pos)
                         (cons 'why (list 'checked-before-use
                                          (cons 'value value-pattern)
                                          (cons 'relation 'guarded)))
                         (cons 'score #f))))))
        (else
          ;; unguarded use: locate the value's def when it resolves (no guard to
          ;; point at). Stays bare when the def has no position.
          (let ((pos (ssa-first-pos ssa-fn
                       (lambda (i) (equal? (nf i 'name) value-pattern)))))
            (if (not pos) 'unguarded
              (cons 'unguarded
                    (list (cons 'where pos)
                          (cons 'why (list 'checked-before-use
                                           (cons 'value value-pattern)
                                           (cons 'relation 'unguarded)))
                          (cons 'score #f))))))))))

;; (custom proc) — escape hatch. proc is (lambda (site ctx) -> symbol).
(define (custom proc)
  "Property checker: escape hatch for user-defined checks.\nPROC receives (site ctx) and returns a symbol categorizing the result.\nSite is a func-decl AST node (tagged alist). Common fields:\n  (nf site 'name)      => function name string\n  (nf site 'body)      => function body AST (list of statements)\n  (nf site 'recv)      => receiver list (methods) or #f (functions)\n  (nf site 'type)      => function type node with params, results\n  (nf site 'pkg-path)  => import path of the containing package\nCtx is the analysis context. Use ctx-pkgs, ctx-ssa, ctx-callgraph,\nctx-find-ssa-func to access loaded analysis data.\n\nParameters:\n  proc : procedure\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (custom (lambda (site ctx)\n    (if (nf site 'recv) 'is-method 'is-function)))\n  (custom (lambda (site ctx)\n    (let ((ssa (ctx-find-ssa-func ctx (nf site 'pkg-path) (nf site 'name))))\n      (if ssa 'has-ssa 'no-ssa))))\n\nSee also: `functions-matching', `ctx-find-ssa-func', `nf'."
  proc)

(define (aggregate-custom proc)
  "Aggregate analyzer: escape hatch for user-defined analyzers.\nPROC receives (sites ctx) and returns a result alist.\nSites is the list of func-decl nodes from the (sites ...) clause.\nCtx is the analysis context.\n\nParameters:\n  proc : procedure\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (define-aggregate-belief \"my-check\"\n    (sites (all-functions-in))\n    (analyze (aggregate-custom\n      (lambda (sites ctx)\n        (list (cons 'type 'aggregate)\n              (cons 'verdict 'OK))))))\n\nSee also: `custom', `define-aggregate-belief'."
  proc)
