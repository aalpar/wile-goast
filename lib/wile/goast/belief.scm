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
;;   (name sites-fn expect-fn min-adherence min-sites sites-expr expect-expr)
;;
;; sites-fn:  (lambda (ctx) ...) -> list of sites
;; expect-fn: (lambda (site ctx) ...) -> category symbol
;; ctx: the analysis context built by run-beliefs

(define *beliefs* '())

(define *aggregate-beliefs* '())

(define (aggregate-beliefs)
  "Return the current list of registered aggregate beliefs.\n\nCategory: goast-belief\n\nSee also: `define-aggregate-belief', `reset-beliefs!'."
  *aggregate-beliefs*)

(define (current-beliefs)
  "Return the current list of registered per-site beliefs.\n\nReading *beliefs* directly from user code returns a stale snapshot under\nWile's library import semantics; this procedure closes over the live\nbinding inside the library and returns its current value. Symmetric to\n`aggregate-beliefs'.\n\nCategory: goast-belief\n\nSee also: `define-belief', `aggregate-beliefs', `reset-beliefs!'."
  *beliefs*)

(define (reset-beliefs!)
  "Clear all registered beliefs (per-site and aggregate).\n\nCategory: goast-belief\n\nSee also: `run-beliefs'."
  (set! *beliefs* '())
  (set! *aggregate-beliefs* '()))

(define (register-belief! name sites-fn expect-fn min-adherence min-sites
                          sites-expr expect-expr)
  (set! *beliefs*
    (append *beliefs*
      (list (list name sites-fn expect-fn min-adherence min-sites
                  sites-expr expect-expr)))))

(define (belief-name b) (list-ref b 0))
(define (belief-sites-fn b) (list-ref b 1))
(define (belief-expect-fn b) (list-ref b 2))
(define (belief-min-adherence b) (list-ref b 3))
(define (belief-min-sites b) (list-ref b 4))
(define (belief-sites-expr b) (list-ref b 5))
(define (belief-expect-expr b) (list-ref b 6))

(define (register-aggregate-belief! name sites-fn analyzer
                                    sites-expr analyze-expr)
  (set! *aggregate-beliefs*
    (append *aggregate-beliefs*
      (list (list name sites-fn analyzer sites-expr analyze-expr)))))

(define (aggregate-belief-name b) (list-ref b 0))
(define (aggregate-belief-sites-fn b) (list-ref b 1))
(define (aggregate-belief-analyzer b) (list-ref b 2))
(define (aggregate-belief-sites-expr b) (list-ref b 3))
(define (aggregate-belief-analyze-expr b) (list-ref b 4))

;; ── define-belief macro ─────────────────────────────────
;;
;; (define-belief "name"
;;   (sites <selector-expr>)
;;   (expect <checker-expr>)
;;   (threshold <adherence> <min-sites>))

(define-syntax define-belief
  (syntax-rules (sites expect threshold)
    ((_ name (sites selector) (expect checker) (threshold min-adh min-n))
     (register-belief! name selector checker min-adh min-n
                       '(sites selector)
                       '(expect checker)))))

(define-syntax define-aggregate-belief
  (syntax-rules (sites analyze)
    ((_ name (sites selector) (analyze analyzer))
     (register-aggregate-belief! name selector analyzer
                                 '(sites selector)
                                 '(analyze analyzer)))))

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
  "Create a lazy-loading analysis context for TARGET package pattern.\nThe context loads AST, SSA, call graph, and field index on demand.\n\nParameters:\n  target : string\nReturns: list\nCategory: goast-belief\n\nSee also: `ctx-pkgs', `ctx-ssa', `ctx-callgraph'."
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
  "Return the type-checked package ASTs from CTX, loading if needed.\n\nParameters:\n  ctx : list\nReturns: list\nCategory: goast-belief\n\nSee also: `make-context', `ctx-ssa'."
  (or (ctx-ref ctx 'pkgs)
      (let ((pkgs (go-typecheck-package (ctx-session ctx))))
        (ctx-set! ctx 'pkgs pkgs)
        pkgs)))

(define (ctx-ssa ctx)
  "Return the SSA functions from CTX, building if needed.\n\nParameters:\n  ctx : list\nReturns: list\nCategory: goast-belief\n\nSee also: `make-context', `ctx-pkgs', `ctx-find-ssa-func'."
  (or (ctx-ref ctx 'ssa)
      (let ((ssa (go-ssa-build (ctx-session ctx))))
        (ctx-set! ctx 'ssa ssa)
        ssa)))

(define (ctx-callgraph ctx)
  "Return the call graph from CTX. Tries RTA (most precise for programs\nwith a main function), falls back to CHA for library packages.\n\nParameters:\n  ctx : list\nReturns: list\nCategory: goast-belief\n\nSee also: `make-context', `callers-of'."
  (or (ctx-ref ctx 'callgraph)
      (let ((cg (guard (exn (#t (go-callgraph (ctx-session ctx) 'cha)))
                  (go-callgraph (ctx-session ctx) 'rta))))
        (ctx-set! ctx 'callgraph cg)
        cg)))

(define (ctx-field-index ctx)
  "Return the SSA field access index from CTX, building if needed.\n\nParameters:\n  ctx : list\nReturns: list\nCategory: goast-belief\n\nSee also: `make-context', `stores-to-fields'."
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
;; Both field-index func and func-name are Form 3 — exact match.
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

;; Extract method name from Form 3 qualified name.
;; Used for interface method matching where go-interface-implementors
;; returns short method names (e.g., "Close", "Read").
;; "(*pkg.Debugger).Continue" -> "Continue", "pkg.init" -> "init"
(define (ssa-short-name full-name)
  (let ((len (string-length full-name)))
    (let loop ((i (- len 1)))
      (cond ((<= i 0) full-name)
            ((char=? (string-ref full-name i) #\.)
             (substring full-name (+ i 1) len))
            (else (loop (- i 1)))))))

;; SSA name lookup index — flat alist keyed by Form 3 name.
;; Form 3 is globally unique, so no two-level (pkg, short) structure needed.
(define (build-ssa-index ssa-funcs)
  (let loop ((fns (if (pair? ssa-funcs) ssa-funcs '()))
             (index '()))
    (if (null? fns) index
      (let* ((fn (car fns))
             (name (nf fn 'name)))
        (if name
          (loop (cdr fns) (cons (cons name fn) index))
          (loop (cdr fns) index))))))

(define (ctx-ssa-index ctx)
  (or (ctx-ref ctx 'ssa-index)
      (let ((index (build-ssa-index (ctx-ssa ctx))))
        (ctx-set! ctx 'ssa-index index)
        index)))

;; SSA function lookup by Form 3 name.
;; Form 3 is globally unique — pkg-path is kept in the signature
;; for backward compatibility but not used for lookup.
(define (ctx-find-ssa-func ctx pkg-path name)
  "Look up an SSA function by Form 3 name.\nBuilds an index on first call for O(1) subsequent lookups.\nReturns an ssa-func node or #f if not found.\nThe ssa-func has: name, signature, params, free-vars, blocks, pkg.\nUse (nf ssa-func 'blocks) to access SSA basic blocks.\n\nParameters:\n  ctx : list\n  pkg-path : string\n  name : string\nReturns: any\nCategory: goast-belief\n\nExamples:\n  (define ssa-fn (ctx-find-ssa-func ctx \"my/pkg\" \"my/pkg.handleRequest\"))\n  (when ssa-fn\n    (nf ssa-fn 'name)       ; => \"my/pkg.handleRequest\"\n    (nf ssa-fn 'blocks)     ; => list of ssa-block nodes\n    (nf ssa-fn 'signature)) ; => \"func(w http.ResponseWriter, r *http.Request)\"\n\nSee also: `ctx-ssa', `make-context', `custom'."
  (let ((entry (assoc name (ctx-ssa-index ctx))))
    (and entry (cdr entry))))

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
  "Extract all func-decl nodes from a list of typed package ASTs.\nEach func-decl is annotated with (pkg-path . import-path).\nCommon fields: name, type, recv, body, pkg-path.\n\nParameters:\n  pkgs : list\nReturns: list\nCategory: goast-belief\n\nExamples:\n  (define funcs (all-func-decls (ctx-pkgs ctx)))\n  (nf (car funcs) 'name)      ; => \"handleRequest\"\n  (nf (car funcs) 'pkg-path)  ; => \"my/pkg\"\n  (nf (car funcs) 'recv)      ; => receiver list or #f\n\nSee also: `functions-matching', `custom'."
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
  "Site selector: functions matching all predicates.\nReturns a procedure (lambda (ctx) -> list-of-func-decls).\n\nParameters:\n  preds : procedure\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (functions-matching (contains-call \"Lock\"))\n  (functions-matching (has-receiver \"*Server\") (contains-call \"Close\"))\n\nSee also: `callers-of', `methods-of', `has-params', `contains-call'."
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

;; (callers-of func-name) -> (lambda (ctx) -> list-of-func-decls)
;; Finds all callers of func-name via the call graph, then looks up
;; each caller's AST func-decl from the loaded packages. Callers
;; without an AST func-decl (e.g., generated code) are skipped.
;; Accepts Form 1 short names or Form 3 qualified names — the call
;; graph primitive handles resolution.
(define (callers-of func-name)
  "Site selector: all callers of a function.\nReturns a procedure (lambda (ctx) -> list-of-func-decls).\nUses the call graph to resolve callers, then maps back to AST func-decls.\nAccepts short names (\"Step\") or qualified names (\"(*pkg.raft).Step\").\n\nParameters:\n  func-name : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (callers-of \"handleRequest\")\n\nSee also: `functions-matching', `go-callgraph-callers'."
  (lambda (ctx)
    (let* ((cg (ctx-callgraph ctx))
           (funcs (all-func-decls (ctx-pkgs ctx)))
           (edges (go-callgraph-callers cg func-name)))
      (if (and edges (pair? edges))
        (filter-map
          (lambda (e)
            (let ((caller (nf e 'caller)))
              (and caller
                   (let loop ((fs funcs))
                     (cond ((null? fs) #f)
                           ((equal? caller (nf (car fs) 'name)) (car fs))
                           (else (loop (cdr fs))))))))
          edges)
        '()))))

;; (methods-of type-name) -> (lambda (ctx) -> list-of-func-decls)
;; Matches methods whose receiver type contains type-name.
(define (methods-of type-name)
  "Site selector: all methods on a receiver type.\nShorthand for (functions-matching (has-receiver TYPE-NAME)).\n\nParameters:\n  type-name : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (methods-of \"*Server\")\n\nSee also: `functions-matching', `has-receiver'."
  (functions-matching (has-receiver type-name)))

;; (all-functions-in) -> (lambda (ctx) -> list-of-func-decls)
;; Returns all func-decl nodes from the context's loaded packages.
;; The package scope is determined by the target argument to run-beliefs,
;; not by this selector — use this for aggregate beliefs where the
;; analyzer is self-contained.
(define (all-functions-in)
  "Site selector: all functions in the context's loaded packages.\nReturns all func-decl nodes annotated with pkg-path.\nThe package scope is determined by run-beliefs' target argument.\n\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (define-aggregate-belief \"pkg-check\"\n    (sites (all-functions-in))\n    (analyze ...))\n\nSee also: `functions-matching', `define-aggregate-belief'."
  (lambda (ctx)
    (all-func-decls (ctx-pkgs ctx))))

;; (implementors-of iface-name) -> (lambda (ctx) -> list-of-func-decls)
;; Returns all func-decls whose receiver type implements the named interface.
(define (implementors-of iface-name)
  "Site selector: methods of all types implementing an interface.\nFinds concrete implementors via go-interface-implementors, then collects\ntheir methods.\n\nParameters:\n  iface-name : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (implementors-of \"Storage\")\n\nSee also: `interface-methods', `go-interface-implementors'."
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
  "Site selector: methods of interface implementors, optionally filtered by name.\nWith one arg, returns all methods. With two, filters to methods matching\nthe given name.\n\nParameters:\n  iface-name : string\n  args : any\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (interface-methods \"Storage\")\n  (interface-methods \"Storage\" \"Save\")\n\nSee also: `implementors-of'."
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
                                (and fn-name
                                     (let ((short (ssa-short-name fn-name)))
                                       (member? short target-methods))))
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
  "Site selector: reuse results from a previously evaluated belief.\nOPTS control filtering: 'adherence or 'deviation, and optionally\na specific result symbol.\n\nParameters:\n  belief-name : string\n  opts : any\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (sites-from \"lock-unlock\" 'deviation)\n  (sites-from \"lock-unlock\" 'adherence 'paired-defer)\n\nSee also: `run-beliefs'."
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
  "Predicate: function signature contains these parameter types.\nReturns a procedure (lambda (func-decl) -> boolean).\n\nParameters:\n  type-strings : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (has-params \"context.Context\" \"*http.Request\")\n\nSee also: `has-receiver', `name-matches', `functions-matching'."
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
  "Predicate: method receiver matches type string.\nMatches against both the type name and pointer variants.\n\nParameters:\n  type-str : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (has-receiver \"*Server\")\n\nSee also: `has-params', `methods-of', `functions-matching'."
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
  "Predicate: function name contains PATTERN as a substring.\n\nParameters:\n  pattern : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (name-matches \"Test\")\n\nSee also: `has-params', `functions-matching'."
  (lambda (fn ctx)
    (let ((name (nf fn 'name)))
      (and name (string-contains name pattern)))))

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
  "Predicate and property checker: function body calls any of FUNC-NAMES.\nAs a predicate for functions-matching, returns #t/#f.\nAs a property checker for expect, returns 'present or 'absent.\n\nParameters:\n  func-names : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (contains-call \"Lock\" \"RLock\")\n\nSee also: `functions-matching', `paired-with'."
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
  "Predicate: SSA function stores to the named fields of STRUCT-NAME.\nDisambiguates receivers against the full struct field set.\n\nParameters:\n  struct-name : string\n  field-names : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (stores-to-fields \"Config\" \"Host\" \"Port\")\n\nSee also: `co-mutated', `go-ssa-field-index'."
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
  "Predicate combinator: all predicates must match.\n\nParameters:\n  preds : procedure\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (all-of (has-receiver \"*Server\") (contains-call \"Close\"))\n\nSee also: `any-of', `none-of'."
  (lambda (fn ctx)
    (let loop ((ps preds))
      (cond ((null? ps) #t)
            (((car ps) fn ctx) (loop (cdr ps)))
            (else #f)))))

;; (any-of pred ...) — at least one predicate must match
(define (any-of . preds)
  "Predicate combinator: at least one predicate must match.\n\nParameters:\n  preds : procedure\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (any-of (name-matches \"Get\") (name-matches \"Fetch\"))\n\nSee also: `all-of', `none-of'."
  (lambda (fn ctx)
    (let loop ((ps preds))
      (cond ((null? ps) #f)
            (((car ps) fn ctx) #t)
            (else (loop (cdr ps)))))))

;; (none-of pred ...) — no predicate matches
(define (none-of . preds)
  "Predicate combinator: no predicate matches.\n\nParameters:\n  preds : procedure\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (none-of (name-matches \"Test\") (name-matches \"Benchmark\"))\n\nSee also: `all-of', `any-of'."
  (lambda (fn ctx)
    (not ((apply any-of preds) fn ctx))))

;; SSA helpers and property checkers live in belief-checkers.scm (included
;; by belief.sld). Split for file-level cohesion — all definitions remain
;; in the (wile goast belief) library namespace.

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
;; Form 3 names are already qualified (e.g., "pkg.Func" or
;; "(*pkg.Type).Method"), so no prefixing is needed.
(define (site-display-name site)
  (cond
    ((and (pair? site) (tag? site 'func-decl))
     (or (nf site 'name) "<anonymous>"))
    (else
     (display-to-string site))))

;; Convert a value to a string for display.
(define (display-to-string val)
  (let ((port (open-output-string)))
    (display val port)
    (get-output-string port)))

;; ── Error classification ──────────────────────────────
;;
;; run-beliefs wraps every belief evaluation in (guard (exn (#t ...)))
;; so one bad belief doesn't abort an entire batch. The broad catch
;; also swallows Scheme-level developer errors (undefined bindings,
;; arity mismatches) that should be visible as "your belief code is
;; broken" rather than "this belief didn't match". classify-belief-error
;; tags results with 'developer (likely a Scheme-level bug in the
;; belief code) or 'belief (domain/runtime error during evaluation).
;;
;; The tagging is a best-effort heuristic on the error message; the
;; full message is always retained in the 'message field so callers
;; can do their own triage. When in doubt, we tag as 'belief to avoid
;; false positives — a Scheme bug will usually still be obvious from
;; the message content.

(define (classify-belief-error msg)
  "Classify a belief-evaluation error message as 'developer or 'belief.\n\nThe classifier matches on substrings that wile's evaluator uses for\ncommon developer bugs: undefined bindings, arity mismatches, and\nmacro/hygiene errors. Anything else tags as 'belief (a\ndomain/runtime error during belief evaluation).\n\nParameters:\n  msg : string-or-any\nReturns: symbol (developer | belief)\nCategory: goast-belief"
  (cond
    ((not (string? msg)) 'belief)
    ((or (string-contains msg "no such binding")
         (string-contains msg "unbound")
         (string-contains msg "undefined")
         (string-contains msg "wrong number of arguments")
         (string-contains msg "wrong-number-of-arguments")
         (string-contains msg "compilation"))
     'developer)
    (else 'belief)))

(define (exn->message exn)
  "Extract a printable message from a condition object."
  (if (error-object? exn)
      (error-object-message exn)
      (display-to-string exn)))


;; ── Aggregate belief evaluation ────────────────────────

(define (evaluate-aggregate-beliefs ctx)
  "Evaluate all registered aggregate beliefs. Returns list of result alists."
  (let loop ((beliefs *aggregate-beliefs*) (results '()))
    (if (null? beliefs)
      (reverse results)
      (let* ((belief (car beliefs))
             (name (aggregate-belief-name belief)))
        (guard (exn
                 (#t (let ((msg (exn->message exn)))
                       (loop (cdr beliefs)
                             (cons (list (cons 'name name)
                                         (cons 'type 'aggregate)
                                         (cons 'status 'error)
                                         (cons 'error-kind (classify-belief-error msg))
                                         (cons 'message msg)
                                         (cons 'sites-expr (aggregate-belief-sites-expr belief))
                                         (cons 'analyze-expr (aggregate-belief-analyze-expr belief)))
                                   results)))))
          (let* ((sites-fn (aggregate-belief-sites-fn belief))
                 (analyzer (aggregate-belief-analyzer belief))
                 (sites (sites-fn ctx))
                 (result (analyzer sites ctx)))
            (loop (cdr beliefs)
                  (cons (append (list (cons 'name name)
                                     (cons 'type 'aggregate)
                                     (cons 'status 'ok)
                                     (cons 'sites-expr (aggregate-belief-sites-expr belief))
                                     (cons 'analyze-expr (aggregate-belief-analyze-expr belief)))
                                (if (pair? result) result '()))
                        results))))))))

;; Main entry point.
;; Evaluates all registered beliefs against the target package.
(define (run-beliefs target)
  "Evaluate all registered beliefs against the target package pattern.\nReturns a flat list of result alists — one per belief (per-site and aggregate).\nBeliefs are registered via define-belief.\n\nParameters:\n  target : string\nReturns: list\nCategory: goast-belief\n\nExamples:\n  (run-beliefs \"my/package/...\")\n\nSee also: `reset-beliefs!'."
  (let ((ctx (make-context target)))
    (let loop ((beliefs *beliefs*)
               (results '()))
      (if (null? beliefs)
        ;; Append aggregate belief results
        (append (reverse results) (evaluate-aggregate-beliefs ctx))
        ;; Evaluate next belief
        (let* ((belief (car beliefs))
               (name (belief-name belief))
               (result
                 (guard (exn
                          (#t (cons 'error (exn->message exn))))
                   (evaluate-belief belief ctx))))
          (cond
            ((and (pair? result) (eq? (car result) 'error))
             (loop (cdr beliefs)
                   (cons (list (cons 'name name)
                               (cons 'type 'per-site)
                               (cons 'status 'error)
                               (cons 'error-kind (classify-belief-error (cdr result)))
                               (cons 'message (cdr result))
                               (cons 'sites-expr (belief-sites-expr belief))
                               (cons 'expect-expr (belief-expect-expr belief)))
                         results)))
            ((not result)
             ;; No sites found
             (loop (cdr beliefs)
                   (cons (list (cons 'name name)
                               (cons 'type 'per-site)
                               (cons 'status 'no-sites)
                               (cons 'sites-expr (belief-sites-expr belief))
                               (cons 'expect-expr (belief-expect-expr belief)))
                         results)))
            (else
             (let* ((maj-cat (list-ref result 1))
                    (ratio (list-ref result 2))
                    (total (list-ref result 3))
                    (adherence (list-ref result 4))
                    (deviations (list-ref result 5))
                    (min-adh (belief-min-adherence belief))
                    (min-n (belief-min-sites belief))
                    (is-strong (and (>= ratio min-adh) (>= total min-n)))
                    (status (if is-strong 'strong 'weak)))
               ;; Store results for bootstrapping
               (ctx-store-result! ctx name
                 adherence
                 (map car deviations))
               (loop (cdr beliefs)
                     (cons (list (cons 'name name)
                                 (cons 'type 'per-site)
                                 (cons 'status status)
                                 (cons 'pattern maj-cat)
                                 (cons 'ratio ratio)
                                 (cons 'total total)
                                 (cons 'min-adherence min-adh)
                                 (cons 'min-sites min-n)
                                 (cons 'adherence
                                       (map site-display-name adherence))
                                 (cons 'deviations
                                       (map (lambda (d)
                                              (cons (site-display-name (car d))
                                                    (cdr d)))
                                            deviations))
                                 (cons 'sites-expr (belief-sites-expr belief))
                                 (cons 'expect-expr (belief-expect-expr belief)))
                           results))))))))))

;; ── Emit mode ─────────────────────────────────────────

;; Format a Scheme value as a string suitable for source output.
;; Uses write (not display) so strings get quoted.
(define (write-to-string val)
  (let ((port (open-output-string)))
    (write val port)
    (get-output-string port)))

;; Emit define-belief and define-aggregate-belief forms for
;; strong per-site beliefs and ok aggregate beliefs.
;; Returns a string of Scheme source code.
(define (emit-beliefs results)
  (let ((port (open-output-string)))
    (let loop ((rs results))
      (cond
        ((null? rs)
         (get-output-string port))
        (else
         (let* ((r (car rs))
                (type (cdr (assoc 'type r)))
                (status (cdr (assoc 'status r))))
           (cond
             ;; Per-site: emit only strong beliefs
             ((and (eq? type 'per-site) (eq? status 'strong))
              (emit-per-site-belief r port)
              (loop (cdr rs)))
             ;; Aggregate: emit only ok beliefs
             ((and (eq? type 'aggregate) (eq? status 'ok))
              (emit-aggregate-belief r port)
              (loop (cdr rs)))
             ;; Skip weak, no-sites, error
             (else (loop (cdr rs))))))))))

(define (emit-per-site-belief r port)
  (let ((name (cdr (assoc 'name r)))
        (pattern (cdr (assoc 'pattern r)))
        (ratio (cdr (assoc 'ratio r)))
        (total (cdr (assoc 'total r)))
        (min-adherence (cdr (assoc 'min-adherence r)))
        (min-sites (cdr (assoc 'min-sites r)))
        (deviations (cdr (assoc 'deviations r)))
        (sites-expr (cdr (assoc 'sites-expr r)))
        (expect-expr (cdr (assoc 'expect-expr r))))
    ;; Comment header
    (display ";; " port) (display name port) (newline port)
    (display ";; Adherence: " port)
    (display (exact->inexact ratio) port)
    (display " (" port) (display (- total (length deviations)) port)
    (display "/" port) (display total port) (display ")" port)
    (display ", Pattern: " port) (display pattern port) (newline port)
    (when (pair? deviations)
      (display ";; Deviations: " port)
      (display (string-join (map (lambda (d) (car d)) deviations) ", ") port)
      (newline port))
    (display ";;" port) (newline port)
    ;; Form — threshold uses the configured min-adherence/min-sites,
    ;; not the observed ratio/total, so emitted beliefs preserve the
    ;; original strictness rather than ratcheting to the current run.
    (display "(define-belief " port) (write name port) (newline port)
    (display "  " port) (write sites-expr port) (newline port)
    (display "  " port) (write expect-expr port) (newline port)
    (display "  (threshold " port)
    (display min-adherence port)
    (display " " port) (display min-sites port)
    (display "))" port) (newline port)
    (newline port)))

(define (emit-aggregate-belief r port)
  (let ((name (cdr (assoc 'name r)))
        (sites-expr (cdr (assoc 'sites-expr r)))
        (analyze-expr (cdr (assoc 'analyze-expr r))))
    ;; Comment header
    (display ";; " port) (display name port) (newline port)
    (display ";; Status: ok" port) (newline port)
    (display ";;" port) (newline port)
    ;; Form
    (display "(define-aggregate-belief " port) (write name port) (newline port)
    (display "  " port) (write sites-expr port) (newline port)
    (display "  " port) (write analyze-expr port) (display ")" port) (newline port)
    (newline port)))

;; ── Belief suppression ──────────────────────────────────
;;
;; Close the discover → review → commit → enforce lifecycle.
;; `with-belief-scope` isolates a thunk from the caller's belief registry;
;; `load-committed-beliefs` snapshots a directory or file of .scm beliefs;
;; `suppress-known` drops results whose sites+expect/analyze expressions
;; match any committed belief structurally (via equal?).

(define (with-belief-scope thunk)
  "Save the belief registry, reset it, invoke THUNK, then restore.
Uses dynamic-wind so the registry is restored even on early exit.

Parameters:
  thunk : procedure of zero arguments
Returns: the value returned by THUNK
Category: goast-belief

See also: `load-committed-beliefs', `reset-beliefs!'."
  (let ((saved-per-site *beliefs*)
        (saved-aggregate *aggregate-beliefs*))
    (dynamic-wind
      (lambda () (reset-beliefs!))
      thunk
      (lambda ()
        (set! *beliefs* saved-per-site)
        (set! *aggregate-beliefs* saved-aggregate)))))

;; Insertion-sort a list of strings lexicographically.
;; Used for deterministic directory-load order.
(define (sort-scheme-filenames lst)
  (define (insert x sorted)
    (cond
      ((null? sorted) (list x))
      ((string<? x (car sorted)) (cons x sorted))
      (else (cons (car sorted) (insert x (cdr sorted))))))
  (let loop ((xs lst) (acc '()))
    (if (null? xs)
      acc
      (loop (cdr xs) (insert (car xs) acc)))))

;; Return the list of .scm file paths (full paths) directly under DIR.
;; Top-level only — no recursion. Sorted lexicographically.
(define (list-scheme-files-in-dir dir)
  (let* ((names (directory-files dir))
         (scm-only
           (let loop ((ns names) (acc '()))
             (cond
               ((null? ns) acc)
               ((and (>= (string-length (car ns)) 4)
                     (string=? (substring (car ns)
                                          (- (string-length (car ns)) 4)
                                          (string-length (car ns)))
                               ".scm"))
                (loop (cdr ns) (cons (car ns) acc)))
               (else (loop (cdr ns) acc)))))
         (sorted (sort-scheme-filenames scm-only)))
    (map (lambda (n) (string-append dir "/" n)) sorted)))

;; Load a single .scm file into the current registry. On failure,
;; write "wile-goast: skipping <path>: <msg>\n" to (current-error-port)
;; and return #f. On success, return #t.
(define (load-belief-file path)
  (guard (exn (#t
               (let ((msg (cond
                            ((error-object? exn)
                             (error-object-message exn))
                            (else "unknown error"))))
                 (display "wile-goast: skipping " (current-error-port))
                 (display path (current-error-port))
                 (display ": " (current-error-port))
                 (display msg (current-error-port))
                 (newline (current-error-port))
                 #f)))
    (load path)
    #t))

(define (load-committed-beliefs path)
  "Load committed beliefs from PATH (a directory or a single .scm file).
Each .scm file is evaluated with `load` inside `with-belief-scope`, so
the caller's belief registry is not clobbered.

Returns a pair:
  (per-site-snapshot . aggregate-snapshot)

where each snapshot is the list of belief tuples (7-tuples for per-site,
5-tuples for aggregate) in registration order.

Files that fail to load are skipped with a stderr warning; the partial
snapshot is still returned. A nonexistent PATH raises an error.

Parameters:
  path : string — directory or file path
Returns: pair of (list . list)
Category: goast-belief

See also: `suppress-known', `with-belief-scope'."
  (cond
    ((not (file-exists? path))
     (error "load-committed-beliefs: path does not exist" path))
    (else
      (with-belief-scope
        (lambda ()
          ;; Probe directory-files to distinguish directory vs file.
          ;; Success => directory; exception => treat as single file.
          (let ((files
                  (guard (exn (#t (list path)))
                    (list-scheme-files-in-dir path))))
            (for-each load-belief-file files)
            (cons *beliefs* *aggregate-beliefs*)))))))

;; Structural equality on a pair of belief expression keys.
;; Returns #t if both keys match in both RESULT and TUPLE.
;; Committed per-site tuple layout:
;;   (name fn fn min-adh min-n sites-expr expect-expr)
;; Committed aggregate tuple layout:
;;   (name fn analyzer sites-expr analyze-expr)
(define (belief-expressions-match? result result-sites-key result-expect-key
                                    tuple-sites-getter tuple-expect-getter
                                    tuple)
  (let ((r-sites (assoc result-sites-key result))
        (r-expect (assoc result-expect-key result)))
    (and r-sites r-expect
         (equal? (cdr r-sites) (tuple-sites-getter tuple))
         (equal? (cdr r-expect) (tuple-expect-getter tuple)))))

;; Walk a list of committed tuples; return #t if any matches RESULT's
;; expressions under the given key + getter pair.
(define (any-tuple-matches? result r-sites-key r-expect-key
                             tuple-sites-getter tuple-expect-getter
                             tuples)
  (let loop ((ts tuples))
    (cond
      ((null? ts) #f)
      ((belief-expressions-match? result r-sites-key r-expect-key
                                   tuple-sites-getter tuple-expect-getter
                                   (car ts)) #t)
      (else (loop (cdr ts))))))

;; Dispatch on RESULT's 'type key:
;;   'per-site   => compare (sites-expr, expect-expr) to per-site tuples.
;;   'aggregate  => compare (sites-expr, analyze-expr) to aggregate tuples.
;;   missing/other => #f (pass through).
(define (result-matches-any? result per-site-tuples aggregate-tuples)
  (let ((type-entry (assoc 'type result)))
    (cond
      ((not type-entry) #f)
      ((eq? (cdr type-entry) 'per-site)
       (any-tuple-matches? result 'sites-expr 'expect-expr
                           belief-sites-expr belief-expect-expr
                           per-site-tuples))
      ((eq? (cdr type-entry) 'aggregate)
       (any-tuple-matches? result 'sites-expr 'analyze-expr
                           aggregate-belief-sites-expr
                           aggregate-belief-analyze-expr
                           aggregate-tuples))
      (else #f))))

(define (suppress-known results committed)
  "Filter RESULTS (output of run-beliefs), dropping any whose
expressions match a belief in COMMITTED (output of
load-committed-beliefs). Matching is structural via `equal?' on
`sites-expr' / `expect-expr' (per-site) or `sites-expr' /
`analyze-expr' (aggregate). Names, thresholds, ratios, and all
other fields are ignored.

Parameters:
  results   : list of result alists
  committed : pair of (per-site-tuples . aggregate-tuples) from
              load-committed-beliefs
Returns: list of result alists (filtered)
Category: goast-belief

See also: `load-committed-beliefs', `emit-beliefs'."
  (let ((per-site (car committed))
        (aggregate (cdr committed)))
    (let loop ((rs results) (acc '()))
      (cond
        ((null? rs) (reverse acc))
        ((result-matches-any? (car rs) per-site aggregate)
         (loop (cdr rs) acc))
        (else (loop (cdr rs) (cons (car rs) acc)))))))

;; string-join: moved to (wile goast utils)
