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
;; expect-fn: (lambda (site ctx) ...) -> category symbol
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

(define-syntax define-belief
  (syntax-rules (sites expect threshold)
    ((_ name (sites selector) (expect checker) (threshold min-adh min-n))
     (register-belief! name selector checker min-adh min-n))))

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
        (cons 'ssa-index #f)
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

;; Extract short name from SSA qualified name.
;; "(*Debugger).Continue" -> "Continue", "init" -> "init"
(define (ssa-short-name full-name)
  (let ((len (string-length full-name)))
    (let loop ((i (- len 1)))
      (cond ((<= i 0) full-name)
            ((char=? (string-ref full-name i) #\.)
             (substring full-name (+ i 1) len))
            (else (loop (- i 1)))))))

;; SSA name lookup index — two-level, keyed by (pkg-path, short-name).
;; First level: package path -> alist of (short-name . ssa-func).
;; Methods like (*Debugger).Continue are indexed as "Continue" within
;; their package's sub-index.
(define (build-ssa-index ssa-funcs)
  (let loop ((fns (if (pair? ssa-funcs) ssa-funcs '()))
             (index '()))
    (if (null? fns) index
      (let* ((fn (car fns))
             (name (nf fn 'name))
             (pkg (nf fn 'pkg))
             (short (and name (ssa-short-name name))))
        (if (and short pkg)
          (let ((pkg-entry (assoc pkg index)))
            (if pkg-entry
              (begin
                (set-cdr! pkg-entry
                  (cons (cons short fn) (cdr pkg-entry)))
                (loop (cdr fns) index))
              (loop (cdr fns)
                    (cons (list pkg (cons short fn)) index))))
          (loop (cdr fns) index))))))

(define (ctx-ssa-index ctx)
  (or (ctx-ref ctx 'ssa-index)
      (let ((index (build-ssa-index (ctx-ssa ctx))))
        (ctx-set! ctx 'ssa-index index)
        index)))

;; Package-qualified SSA function lookup.
;; Returns the SSA function for the given package path and short name,
;; or #f if not found.
(define (ctx-find-ssa-func ctx pkg-path name)
  (let ((pkg-entry (assoc pkg-path (ctx-ssa-index ctx))))
    (and pkg-entry
         (let ((entry (assoc name (cdr pkg-entry))))
           (and entry (cdr entry))))))

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

;; ── Site selectors ──────────────────────────────────────
;;
;; Each selector returns (lambda (ctx) -> list-of-sites).
;; A "site" is a func-decl AST node (for function-based selectors)
;; or a call-graph edge (for caller-based selectors).

;; Extract all func-decl nodes from a package list.
;; Each func-decl is annotated with (pkg-path . <import-path>) from its
;; parent package, enabling cross-package SSA/CFG disambiguation.
(define (all-func-decls pkgs)
  (flat-map
    (lambda (pkg)
      (let ((pkg-path (nf pkg 'path)))
        (flat-map
          (lambda (file)
            (filter-map
              (lambda (decl)
                (and (tag? decl 'func-decl)
                     (cons (car decl)
                           (cons (cons 'pkg-path pkg-path)
                                 (cdr decl)))))
              (let ((decls (nf file 'decls)))
                (if (pair? decls) decls '()))))
          (let ((files (nf pkg 'files)))
            (if (pair? files) files '())))))
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

;; ── Selector predicates ─────────────────────────────────
;;
;; Each predicate returns (lambda (func-decl ctx) -> #t/#f).
;; Used as arguments to functions-matching.

;; Extract type strings from a type AST node for matching.
;; Returns a list of candidate strings: the inferred-type (full path)
;; and a short form built from the AST (e.g., "klog.Logger" from a
;; selector-expr). The short form handles Go module versioning where
;; inferred-type contains paths like "k8s.io/klog/v2.Logger" that
;; don't match the source-level "klog.Logger".
(define (type-match-strings t)
  (let ((inferred (and t (nf t 'inferred-type)))
        (short (and t (tag? t 'selector-expr)
                    (let ((x (nf t 'x))
                          (sel (nf t 'sel)))
                      (and x sel (tag? x 'ident)
                           (let ((name (nf x 'name)))
                             (and name
                                  (string-append name "." sel))))))))
    (filter-map (lambda (x) (and (pair? x) x))
                (list (and inferred (list inferred))
                      (and short (list short))))))

;; Flatten a list of lists.
(define (flatten-1 xss)
  (let loop ((xs xss) (acc '()))
    (if (null? xs) acc
      (loop (cdr xs) (append acc (car xs))))))

;; (has-params type-str ...) — function signature contains these param types.
;; Matches each type-str as a substring of the param's inferred-type or
;; its short source-level name (e.g., "klog.Logger" matches both
;; "k8s.io/klog/v2.Logger" and "klog.Logger").
(define (has-params . type-strings)
  (lambda (fn ctx)
    (let* ((ftype (nf fn 'type))
           (params (and ftype (nf ftype 'params)))
           (param-types
             (if (pair? params)
               (flatten-1
                 (filter-map
                   (lambda (p)
                     (let ((t (nf p 'type)))
                       (let ((strs (type-match-strings t)))
                         (if (pair? strs) (flatten-1 strs) #f))))
                   params))
               '())))
      (let loop ((ts type-strings))
        (cond ((null? ts) #t)
              ((let check ((pts param-types))
                 (cond ((null? pts) #f)
                       ((string-contains (car pts) (car ts)) #t)
                       (else (check (cdr pts)))))
               (loop (cdr ts)))
              (else #f))))))

;; (has-receiver type-str) — method receiver matches type string.
;; Matches type-str as a substring of the receiver's inferred-type
;; or short name, so "MyType" matches "pkg.MyType", "*pkg.MyType",
;; and versioned paths like "pkg/v2.MyType".
(define (has-receiver type-str)
  (lambda (fn ctx)
    (let* ((recv (nf fn 'recv))
           (recv-list (and recv (if (pair? recv) recv '())))
           (recv-t (if (pair? recv-list)
                     (nf (car recv-list) 'type)
                     #f))
           ;; For pointer receivers, unwrap *T to get T's type node.
           (inner-t (if (and recv-t (tag? recv-t 'star-expr))
                      (nf recv-t 'x)
                      recv-t))
           (candidates (flatten-1
                         (type-match-strings
                           (or inner-t recv-t)))))
      (let check ((cs candidates))
        (cond ((null? cs) #f)
              ((string-contains (car cs) type-str) #t)
              (else (check (cdr cs))))))))

;; (name-matches pattern) — function name matches substring.
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

;; Extract short package name from full import path.
;; "k8s.io/kubernetes/pkg/kubelet/kuberuntime" -> "kuberuntime"
(define (package-short-name path)
  (let ((len (string-length path)))
    (let loop ((i (- len 1)))
      (cond ((<= i 0) path)
            ((char=? (string-ref path i) #\/)
             (substring path (+ i 1) len))
            (else (loop (- i 1)))))))

;; (contains-call func-name ...) — function body contains a call to any
;; of the named functions. Dual-use:
;;   As a predicate: (lambda (func-decl ctx) -> #t/#f)
;;   As a checker:   runner normalizes #t -> 'present, #f -> 'absent
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
  (lambda (fn ctx)
    (pair? (walk (or (nf fn 'body) '()) call-matches?))))

;; (stores-to-fields struct-name field ...) — SSA: function stores to
;; these fields. Requires SSA layer.
(define (stores-to-fields struct-name . field-names)
  (lambda (fn ctx)
    (let* ((fname (nf fn 'name))
           (pkg-path (nf fn 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if ssa-fn
        (let* ((all-fields (struct-field-names (ctx-pkgs ctx) struct-name))
               (stored (stored-fields-in-func ssa-fn field-names all-fields)))
          (pair? stored))
        #f))))

;; ── Predicate combinators ───────────────────────────────

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

;; Collect ssa-field-addr instructions from an SSA function.
(define (collect-field-addrs ssa-func)
  (walk ssa-func
    (lambda (node)
      (and (tag? node 'ssa-field-addr)
           (list (nf node 'name)
                 (nf node 'field)
                 (nf node 'x))))))

;; Collect ssa-store instructions from an SSA function.
(define (collect-stores ssa-func)
  (walk ssa-func
    (lambda (node)
      (and (tag? node 'ssa-store)
           (list (nf node 'addr)
                 (nf node 'val))))))

;; Determine which target fields are stored in an SSA function.
;; Uses receiver-type disambiguation: a receiver is valid only
;; if every field it accesses is in struct-fields (the full struct).
;; Only fields in target-fields are counted in the result.
(define (stored-fields-in-func ssa-func target-fields struct-fields)
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
                                         ((not (member? (car fs) struct-fields)) #f)
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

;; ── Property checkers ───────────────────────────────────
;;
;; Each checker returns (lambda (site ctx) -> category-symbol).
;; The majority category becomes the belief; minorities are deviations.

;; (paired-with op-a op-b) — checks if function body contains both
;; operations, with preference for defer pairing.
;; Returns: 'paired-defer, 'paired-call, or 'unpaired
(define (paired-with op-a op-b)
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

;; (ordered op-a op-b) — checks whether op-a's block dominates op-b's block.
;; Requires CFG layer.
;; Returns: 'a-dominates-b, 'b-dominates-a, 'same-block, 'unordered, or 'missing
(define (ordered op-a op-b)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (cfg (and pkg-path (go-cfg pkg-path fname))))
      (if (not cfg) 'missing
        (let* ((blocks (nf cfg 'blocks))
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
;; (co-mutated field ...) — checks whether all named fields are stored
;; together in the function.
;; Skips receiver-type disambiguation: the site selector (stores-to-fields)
;; already filtered to functions that store to this struct's fields.
;; Returns: 'co-mutated or 'partial
(define (co-mutated . field-names)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if (not ssa-fn) 'partial
        (let* ((all-field-addrs (collect-field-addrs ssa-fn))
               (stores (collect-stores ssa-fn))
               (store-addrs (map car stores))
               ;; Collect stored field names without disambiguation
               (stored (unique (filter-map
                         (lambda (fa)
                           (let ((reg (car fa))
                                 (field (cadr fa)))
                             (and (member? reg store-addrs)
                                  (member? field field-names)
                                  field)))
                         all-field-addrs))))
          (if (= (length stored) (length field-names))
            'co-mutated
            'partial))))))

;; (checked-before-use value-pattern) — checks whether a value is
;; tested before use via SSA + CFG dominance.
;; Returns: 'guarded or 'unguarded
(define (checked-before-use value-pattern)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if (not ssa-fn) 'unguarded
        (let* ((blocks (nf ssa-fn 'blocks))
               (all-instrs (if (pair? blocks)
                             (flat-map
                               (lambda (b) (let ((is (nf b 'instrs)))
                                             (if (pair? is) is '())))
                               blocks)
                             '()))
               (uses (filter-map
                       (lambda (instr)
                         (let ((ops (nf instr 'operands)))
                           (and (pair? ops) (member? value-pattern ops)
                                instr)))
                       all-instrs))
               (has-guard (let loop ((us uses))
                            (cond ((null? us) #f)
                                  ((tag? (car us) 'ssa-if) #t)
                                  (else (loop (cdr us)))))))
          (if has-guard 'guarded 'unguarded))))))

;; (custom proc) — escape hatch. proc is (lambda (site ctx) -> symbol).
(define (custom proc) proc)

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
      (let* ((classified
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

;; ── Runner ──────────────────────────────────────────────

;; Extract a display name from a site (func-decl or caller edge).
(define (site-display-name site)
  (cond
    ((and (pair? site) (tag? site 'func-decl))
     (let ((name (or (nf site 'name) "<anonymous>"))
           (pkg-path (nf site 'pkg-path)))
       (if pkg-path
         (string-append (package-short-name pkg-path) "." name)
         name)))
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
