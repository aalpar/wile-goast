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

(define (reset-beliefs!)
  (set! *beliefs* '()))

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
  (let ((session (go-load target)))
    (list (cons 'target target)
          (cons 'session session)
          (cons 'pkgs #f)
          (cons 'ssa #f)
          (cons 'ssa-index #f)
          (cons 'field-index #f)
          (cons 'callgraph #f)
          (cons 'interface-cache '())
          (cons 'results '()))))

(define (ctx-ref ctx key)
  (cdr (assoc key ctx)))

(define (ctx-set! ctx key val)
  (set-cdr! (assoc key ctx) val))

(define (ctx-target ctx) (ctx-ref ctx 'target))

(define (ctx-session ctx) (ctx-ref ctx 'session))

(define (ctx-pkgs ctx)
  (or (ctx-ref ctx 'pkgs)
      (let ((pkgs (go-typecheck-package (ctx-session ctx))))
        (ctx-set! ctx 'pkgs pkgs)
        pkgs)))

(define (ctx-ssa ctx)
  (or (ctx-ref ctx 'ssa)
      (let ((ssa (go-ssa-build (ctx-session ctx))))
        (ctx-set! ctx 'ssa ssa)
        ssa)))

(define (ctx-callgraph ctx)
  (or (ctx-ref ctx 'callgraph)
      (let ((cg (go-callgraph (ctx-session ctx) 'static)))
        (ctx-set! ctx 'callgraph cg)
        cg)))

(define (ctx-field-index ctx)
  (or (ctx-ref ctx 'field-index)
      (let ((idx (go-ssa-field-index (ctx-session ctx))))
        (ctx-set! ctx 'field-index idx)
        idx)))

;; Lazy interface info lookup, cached by (iface-name . session).
(define (ctx-interface-info ctx iface-name)
  (let* ((cache (ctx-ref ctx 'interface-cache))
         (session (ctx-session ctx))
         (key (cons iface-name session))
         (cached (assoc key cache)))
    (if cached (cdr cached)
      (let ((info (go-interface-implementors iface-name session)))
        (ctx-set! ctx 'interface-cache (cons (cons key info) cache))
        info))))

;; Find the field summary for a function by package path and name.
(define (find-field-summary index pkg-path func-name)
  (let loop ((entries (if (pair? index) index '())))
    (if (null? entries) #f
      (let ((entry (car entries)))
        (if (and (equal? (nf entry 'func) func-name)
                 (equal? (nf entry 'pkg) pkg-path))
          entry
          (loop (cdr entries)))))))

;; Extract written field names for a given struct from a summary.
;; If struct-name is #f, returns writes for all structs.
(define (writes-for-struct summary struct-name)
  (let ((fields (nf summary 'fields)))
    (if (not (pair? fields)) '()
      (filter-map
        (lambda (access)
          (and (eq? (nf access 'mode) 'write)
               (or (not struct-name)
                   (equal? (nf access 'struct) struct-name))
               (nf access 'field)))
        fields))))

;; Check that all names in required are present in available.
(define (all-present? required available)
  (let loop ((rs required))
    (cond ((null? rs) #t)
          ((member? (car rs) available) (loop (cdr rs)))
          (else #f))))

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

;; Resolve a short function name to its fully-qualified call graph name.
;; Walks the call graph nodes looking for an exact match or a name ending
;; with ".func-name". Returns the qualified name, or #f if not found.
(define (cg-resolve-name cg func-name)
  (let ((suffix (string-append "." func-name)))
    (let loop ((nodes (if (pair? cg) cg '())))
      (cond ((null? nodes) #f)
            (else
              (let ((name (nf (car nodes) 'name)))
                (if (and name
                         (or (equal? name func-name)
                             (let ((nlen (string-length name))
                                   (slen (string-length suffix)))
                               (and (>= nlen slen)
                                    (equal? (substring name (- nlen slen) nlen)
                                            suffix)))))
                  name
                  (loop (cdr nodes)))))))))

;; (callers-of func-name) -> (lambda (ctx) -> list-of-func-decls)
;; Finds all callers of func-name via the call graph, then looks up
;; each caller's AST func-decl from the loaded packages. Callers
;; without an AST func-decl (e.g., generated code) are skipped.
(define (callers-of func-name)
  (lambda (ctx)
    (let* ((cg (ctx-callgraph ctx))
           (funcs (all-func-decls (ctx-pkgs ctx)))
           (qualified (cg-resolve-name cg func-name))
           (edges (if qualified
                    (go-callgraph-callers cg qualified)
                    #f)))
      (if (and edges (pair? edges))
        (filter-map
          (lambda (e)
            (let ((caller (nf e 'caller)))
              (and caller
                   (let ((short (ssa-short-name caller)))
                     (let loop ((fs funcs))
                       (cond ((null? fs) #f)
                             ((equal? (nf (car fs) 'name) short) (car fs))
                             (else (loop (cdr fs)))))))))
          edges)
        '()))))

;; (methods-of type-name) -> (lambda (ctx) -> list-of-func-decls)
;; Matches methods whose receiver type contains type-name.
(define (methods-of type-name)
  (functions-matching (has-receiver type-name)))

;; (implementors-of iface-name) -> (lambda (ctx) -> list-of-func-decls)
;; Returns all func-decls whose receiver type implements the named interface.
(define (implementors-of iface-name)
  (lambda (ctx)
    (let* ((info (ctx-interface-info ctx iface-name))
           (implementors (nf info 'implementors))
           (funcs (all-func-decls (ctx-pkgs ctx))))
      (filter-map
        (lambda (fn)
          (and (let loop ((impls (if (pair? implementors) implementors '())))
                 (cond ((null? impls) #f)
                       (((has-receiver (cdr (assoc 'type (car impls)))) fn ctx) #t)
                       (else (loop (cdr impls)))))
               fn))
        funcs))))

;; (interface-methods iface-name [method-name]) -> (lambda (ctx) -> list-of-func-decls)
;; Returns func-decls that implement interface methods. Without method-name,
;; returns all interface methods across all implementors. With method-name,
;; narrows to a single method — the primary form for behavioral consistency
;; analysis ("compare how different types implement the same method").
;; Each returned func-decl is annotated with (impl-type . type-name) for
;; type-qualified display names in deviation reports.
(define (interface-methods iface-name . args)
  (let ((method-name (if (pair? args) (car args) #f)))
    (lambda (ctx)
      (let* ((info (ctx-interface-info ctx iface-name))
             (iface-methods (nf info 'methods))
             (implementors (nf info 'implementors))
             (funcs (all-func-decls (ctx-pkgs ctx)))
             (target-methods
               (if method-name (list method-name)
                 (if (pair? iface-methods) iface-methods '()))))
        (let impl-loop ((impls (if (pair? implementors) implementors '()))
                        (result '()))
          (if (null? impls) result
            (let* ((impl (car impls))
                   (type-name (cdr (assoc 'type impl)))
                   (matching
                     (filter-map
                       (lambda (fn)
                         (and ((has-receiver type-name) fn ctx)
                              (let ((fn-name (nf fn 'name)))
                                (and fn-name (member? fn-name target-methods)))
                              ;; Annotate with impl-type for display.
                              (cons (car fn)
                                    (cons (cons 'impl-type type-name)
                                          (cdr fn)))))
                       funcs)))
              (impl-loop (cdr impls) (append result matching)))))))))

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
;; these fields. Uses the pre-built field index from Go.
(define (stores-to-fields struct-name . field-names)
  (lambda (fn ctx)
    (let* ((fname (nf fn 'name))
           (pkg-path (nf fn 'pkg-path))
           (summary (find-field-summary (ctx-field-index ctx) pkg-path fname)))
      (and summary
           (let ((writes (writes-for-struct summary struct-name)))
             (all-present? field-names writes))))))

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

;; (ordered op-a op-b) — checks whether op-a's SSA block dominates op-b's block.
;; Uses SSA representation (blocks have instrs + idom). Does not require go-cfg.
;; Returns: 'a-dominates-b, 'b-dominates-a, 'same-block, 'unordered, or 'missing
(define (ordered op-a op-b)
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
            ((ssa-dominates? blocks (car a-blocks) (car b-blocks)) 'a-dominates-b)
            ((ssa-dominates? blocks (car b-blocks) (car a-blocks)) 'b-dominates-a)
            (else 'unordered)))))))

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
;; Walks b's immediate dominator chain upward. If a is encountered, a dominates b.
(define (ssa-dominates? blocks a-idx b-idx)
  (let ((block-map (map (lambda (b) (cons (nf b 'index) b))
                        (if (pair? blocks) blocks '()))))
    (let loop ((current b-idx))
      (cond
        ((= current a-idx) #t)
        (else
          (let ((entry (assoc current block-map)))
            (if (not entry) #f
              (let ((idom (nf (cdr entry) 'idom)))
                (if (or (not idom) (eq? idom #f) (= idom current))
                  #f
                  (loop idom))))))))))

;; (co-mutated field ...) — checks whether all named fields are stored
;; together in the function. Uses the pre-built field index from Go.
;; Returns: 'co-mutated, 'partial, or 'missing
(define (co-mutated . field-names)
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
;; tested before use via bounded transitive reachability on the
;; SSA def-use graph. Starting from value-pattern, each iteration
;; expands the tracked name set by one hop through instruction
;; operands, up to max-depth rounds. If any ssa-if is reached,
;; the value is guarded.
;; Covers: direct comparison (if err != nil), field access
;; (if m.Type == x), and any chain up to 4 hops.
;; Returns: 'guarded, 'unguarded, or 'missing (SSA lookup failed)
(define (checked-before-use value-pattern)
  (define max-depth 4)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if (not ssa-fn) 'missing
        (let* ((blocks (nf ssa-fn 'blocks))
               (all-instrs (if (pair? blocks)
                             (flat-map
                               (lambda (b) (let ((is (nf b 'instrs)))
                                             (if (pair? is) is '())))
                               blocks)
                             '())))
          ;; Bounded Kleene iteration: expand the tracked name set through
          ;; the def-use chain. Each round finds instructions whose operands
          ;; intersect the tracked set, adds their output names, checks for ssa-if.
          (let chase ((tracked (list value-pattern)) (depth 0))
            (if (> depth max-depth) 'unguarded
              (let* ((reached (filter-map
                                (lambda (instr)
                                  (let ((ops (nf instr 'operands)))
                                    (and (pair? ops)
                                         (let check ((ts tracked))
                                           (cond ((null? ts) #f)
                                                 ((member? (car ts) ops) instr)
                                                 (else (check (cdr ts))))))))
                                all-instrs))
                     ;; Check if any reached instruction is an ssa-if.
                     (found-guard (let loop ((rs reached))
                                   (cond ((null? rs) #f)
                                         ((tag? (car rs) 'ssa-if) #t)
                                         (else (loop (cdr rs))))))
                     ;; Collect output names for the next iteration.
                     ;; For stores (no output name), track operands
                     ;; to follow value-to-address connections.
                     (new-names (flat-map
                                  (lambda (instr)
                                    (let ((nm (nf instr 'name)))
                                      (cond
                                        ((and nm (not (member? nm tracked)))
                                         (list nm))
                                        ((tag? instr 'ssa-store)
                                         (filter-map
                                           (lambda (op)
                                             (and (not (member? op tracked)) op))
                                           (or (nf instr 'operands) '())))
                                        (else '()))))
                                  reached)))
                (cond
                  (found-guard 'guarded)
                  ((null? new-names) 'unguarded)
                  (else (chase (append tracked new-names) (+ depth 1))))))))))))

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

;; Extract a display name from a site (func-decl node).
(define (site-display-name site)
  (cond
    ((and (pair? site) (tag? site 'func-decl))
     (let ((name (or (nf site 'name) "<anonymous>"))
           (impl-type (nf site 'impl-type))
           (pkg-path (nf site 'pkg-path)))
       (cond
         (impl-type (string-append impl-type "." name))
         (pkg-path (string-append (package-short-name pkg-path) "." name))
         (else name))))
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
