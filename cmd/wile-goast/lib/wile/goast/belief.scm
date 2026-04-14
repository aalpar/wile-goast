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
;;   (name sites-fn expect-fn min-adherence min-sites)
;;
;; sites-fn:  (lambda (ctx) ...) -> list of sites
;; expect-fn: (lambda (site ctx) ...) -> category symbol
;; ctx: the analysis context built by run-beliefs

(define *beliefs* '())

(define *aggregate-beliefs* '())

(define (aggregate-beliefs)
  "Return the current list of registered aggregate beliefs.\n\nCategory: goast-belief\n\nSee also: `define-aggregate-belief', `reset-beliefs!'."
  *aggregate-beliefs*)

(define (reset-beliefs!)
  "Clear all registered beliefs (per-site and aggregate).\n\nCategory: goast-belief\n\nSee also: `run-beliefs'."
  (set! *beliefs* '())
  (set! *aggregate-beliefs* '()))

(define (register-belief! name sites-fn expect-fn min-adherence min-sites)
  (set! *beliefs*
    (append *beliefs*
      (list (list name sites-fn expect-fn min-adherence min-sites)))))

(define (belief-name b) (list-ref b 0))
(define (belief-sites-fn b) (list-ref b 1))
(define (belief-expect-fn b) (list-ref b 2))
(define (belief-min-adherence b) (list-ref b 3))
(define (belief-min-sites b) (list-ref b 4))

(define (register-aggregate-belief! name sites-fn analyzer)
  (set! *aggregate-beliefs*
    (append *aggregate-beliefs*
      (list (list name sites-fn analyzer)))))

(define (aggregate-belief-name b) (list-ref b 0))
(define (aggregate-belief-sites-fn b) (list-ref b 1))
(define (aggregate-belief-analyzer b) (list-ref b 2))

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

(define-syntax define-aggregate-belief
  (syntax-rules (sites analyze)
    ((_ name (sites selector) (analyze analyzer))
     (register-aggregate-belief! name selector analyzer))))

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
;; Returns: 'a-dominates-b, 'b-dominates-a, 'same-block, 'unordered, or 'missing
(define (ordered op-a op-b)
  "Property checker: verify that OP-A's SSA block dominates OP-B's block.\nReturns 'a-dominates-b, 'b-dominates-a, 'same-block, or 'unordered.\n\nParameters:\n  op-a : string\n  op-b : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (ordered \"Validate\" \"Execute\")\n\nSee also: `paired-with', `checked-before-use'."
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
(define (checked-before-use value-pattern)
  "Property checker: verify that a value matching VALUE-PATTERN is tested\nbefore use. Uses bounded def-use reachability (fuel=5) to check whether\nthe value flows through a comparison before reaching a non-guard use.\nReturns 'guarded or 'unguarded.\n\nParameters:\n  value-pattern : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (checked-before-use \"err\")\n\nSee also: `ordered', `defuse-reachable?'."
  (define fuel 5) ;; max-hops + 1: fixpoint needs one extra iteration to confirm convergence
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


;; ── Aggregate belief evaluation ────────────────────────

(define (evaluate-aggregate-beliefs ctx)
  "Evaluate all registered aggregate beliefs. Returns list of result alists."
  (let loop ((beliefs *aggregate-beliefs*) (results '()))
    (if (null? beliefs)
      (reverse results)
      (let* ((belief (car beliefs))
             (name (aggregate-belief-name belief)))
        (guard (exn
                 (#t (loop (cdr beliefs)
                           (cons (list (cons 'name name)
                                       (cons 'type 'aggregate)
                                       (cons 'status 'error)
                                       (cons 'message
                                             (if (error-object? exn)
                                               (error-object-message exn)
                                               (display-to-string exn))))
                                 results))))
          (let* ((sites-fn (aggregate-belief-sites-fn belief))
                 (analyzer (aggregate-belief-analyzer belief))
                 (sites (sites-fn ctx))
                 (result (analyzer sites ctx)))
            (loop (cdr beliefs)
                  (cons (append (list (cons 'name name)
                                     (cons 'type 'aggregate)
                                     (cons 'status 'ok))
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
                          (#t (cons 'error
                                (if (error-object? exn)
                                  (error-object-message exn)
                                  (display-to-string exn)))))
                   (evaluate-belief belief ctx))))
          (cond
            ((and (pair? result) (eq? (car result) 'error))
             (loop (cdr beliefs)
                   (cons (list (cons 'name name)
                               (cons 'type 'per-site)
                               (cons 'status 'error)
                               (cons 'message (cdr result)))
                         results)))
            ((not result)
             ;; No sites found
             (loop (cdr beliefs)
                   (cons (list (cons 'name name)
                               (cons 'type 'per-site)
                               (cons 'status 'no-sites))
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
                                 (cons 'adherence
                                       (map site-display-name adherence))
                                 (cons 'deviations
                                       (map (lambda (d)
                                              (cons (site-display-name (car d))
                                                    (cdr d)))
                                            deviations)))
                           results))))))))))
