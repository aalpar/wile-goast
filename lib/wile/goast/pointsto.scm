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

;;; (wile goast pointsto) — Value-level (instance-sensitive) points-to, and
;;; the lock-escape measure built on it.
;;;
;;; PURPOSE. A measure surface (the M-operator of
;;; plans/2026-05-31-objective-measure-subjective-norm-design.md): per
;;; allocation instance, how many distinct goroutine roots can reach a lock on
;;; it. escape == 1 marks a vestigial lock — a candidate for collapse back to
;;; sequential code. The tool never decides the collapse; the human holds that
;;; subjective judgment. This answers a question a glance cannot: which
;;; *allocation* a lock receiver came from, across call frames.
;;;
;;; ENGINE. A forward, per-function dataflow over the powerset of allocation
;;; sites, reusing (wile goast dataflow) run-analysis exactly as the domains in
;;; domains.scm do. A value's state is the set of abstract allocation sites it
;;; may reference (its points-to set). The ⊤ sentinel *pointsto-unknown* means
;;; "provenance unresolved — assume it may reference anything," and is the
;;; sound default that keeps the measure from ever reporting a false candidate.
;;;
;;; SCOPE TODAY. Intraprocedural provenance is modeled (alloc gen, address/copy
;;; propagation, phi join). The single interprocedural edge — a parameter's
;;; provenance, pulled from callers — is left as a TODO in `make-param-seed`.
;;; This is not a detail: a method's lock receiver `c` in `c.mu.Lock()` IS a
;;; parameter of the method, so until that clause is written, essentially every
;;; real lock base is ⊤ and `lock-escape-measure` reports nothing actionable.
;;; The receiver-instance question and the interprocedural points-to question
;;; are the same question. That is what makes the TODO the analysis.

;; ─── Set utilities (powerset elements are plain lists) ───

(define (set-union a b)
  (let loop ((xs b) (acc a))
    (cond ((null? xs) acc)
          ((member (car xs) acc) (loop (cdr xs) acc))
          (else (loop (cdr xs) (cons (car xs) acc))))))

;; ─── Tagged-node + string helpers ───

(define (tag-in? node tags)
  (and (pair? node) (symbol? (car node)) (memq (car node) tags) #t))

(define (ends-with? s suffix)
  (let ((ls (string-length s)) (lf (string-length suffix)))
    (and (>= ls lf)
         (string=? (substring s (- ls lf) ls) suffix))))

;; ─── Per-value state: an alist (value-name . points-to-set) ───

(define (state-lookup st key)
  (let ((p (assoc key st))) (and p (cdr p))))

(define (state-update st key val)
  (cond ((null? st) (list (cons key val)))
        ((equal? (caar st) key) (cons (cons key val) (cdr st)))
        (else (cons (car st) (state-update (cdr st) key val)))))

;; ─── Allocation-site identity and value universe ───

;; The ⊤ sentinel. A points-to set containing it means the value may reference
;; an unknown object — treated as escaping (escape = 'unbounded), never dropped.
(define *pointsto-unknown* "?")

(define (unknown-pointee? set)
  (and (member *pointsto-unknown* set) #t))

;; A globally-unique id for an allocation site: "<fn>::<reg>". Uniqueness across
;; the program is what makes two same-typed allocations distinguishable — the
;; whole point of value-level over type-level.
(define (alloc-id fn-name instr)
  (string-append fn-name "::" (or (nf instr 'name) "?")))

(define (param-names fn)
  (map (lambda (p) (nf p 'name)) (or (nf fn 'params) '())))

;; Map-lattice keys: parameters (defined on entry) plus instruction results.
(define (value-universe fn)
  (append (param-names fn) (ssa-instruction-names fn)))

;; Allocation sites generated within fn.
(define (alloc-sites fn)
  (let ((fn-name (nf fn 'name)))
    (filter-map
      (lambda (i) (and (tag? i 'ssa-alloc) (alloc-id fn-name i)))
      (ssa-all-instrs fn))))

;; Powerset universe for one function's points-to sets.
;;
;; NOTE (interprocedural widening): today a points-to set holds only this
;; function's own alloc ids plus the ⊤ sentinel, because `make-param-seed`
;; injects ⊤ for parameters. When the parameter TODO is implemented to import
;; caller alloc ids, widen this universe to the PROGRAM-WIDE allocation set
;; (union of alloc-sites over all functions), or the imported ids fall outside
;; the lattice.
(define (alloc-universe fn)
  (cons *pointsto-unknown* (alloc-sites fn)))

;; ─── Intraprocedural transfer (forward) ───

;; Copy a base value's points-to set, defaulting absent → ⊤ (sound: an
;; unresolved base must be assumed to reference anything).
(define (copy-of st name)
  (or (and name (state-lookup st name)) (list *pointsto-unknown*)))

(define (join-operands st operands)
  (let loop ((os operands) (acc '()))
    (if (null? os) acc
        (loop (cdr os) (set-union acc (copy-of st (car os)))))))

(define (make-transfer fn)
  (let ((fn-name (nf fn 'name)))
    (lambda (block state)
      (let loop ((instrs (block-instrs block)) (st state))
        (if (null? instrs)
            st
            (let* ((instr (car instrs))
                   (nm (nf instr 'name)))
              (loop (cdr instrs)
                (cond
                  ;; gen: a fresh abstract object lives here.
                  ((tag? instr 'ssa-alloc)
                   (if nm (state-update st nm (list (alloc-id fn-name instr))) st))

                  ;; address-of / derived pointer: &b.f, b[i], conversions, and
                  ;; slices all denote the same instance(s) as their base `x`.
                  ;; Field-insensitive on the heap — the instance identity, not
                  ;; the field, is what the lock-escape measure needs.
                  ((tag-in? instr '(ssa-field-addr ssa-field
                                    ssa-index-addr ssa-index
                                    ssa-convert ssa-change-type
                                    ssa-change-interface ssa-multi-convert
                                    ssa-slice ssa-slice-to-array-ptr))
                   (if nm (state-update st nm (copy-of st (nf instr 'x))) st))

                  ;; load/deref through a pointer: pointer-copy approximation.
                  ((tag? instr 'ssa-unop)
                   (if nm (state-update st nm (copy-of st (nf instr 'x))) st))

                  ;; phi: provenance is the join of all incoming values.
                  ((tag? instr 'ssa-phi)
                   (if nm
                       (state-update st nm
                         (join-operands st (or (nf instr 'operands) '())))
                       st))

                  ;; closure object: not itself a lock receiver. Its captured
                  ;; bindings flow to the callee's free vars — an interprocedural
                  ;; capture edge handled with the parameter seed (see TODO).
                  ((tag? instr 'ssa-make-closure)
                   (if nm (state-update st nm '()) st))

                  ;; Any other named value (call results, lookups, …): provenance
                  ;; unmodeled ⇒ ⊤. ⊤ (not ∅) is the SOUND default — a lock whose
                  ;; base we cannot resolve must be treated as possibly-shared,
                  ;; never silently attributed to no instance.
                  (nm (state-update st nm (list *pointsto-unknown*)))

                  ;; stores, jumps, returns, sends: no defined pointer value.
                  (else st)))))))))

;; ─── The parameter seed: THE interprocedural edge (TODO) ───

(define (make-param-seed fn keys)
  ;; Initial entry-block in-state: the points-to set of every parameter.
  ;;
  ;; TODO(value-level interprocedural points-to). Resolve each parameter's set
  ;; from the call graph instead of returning ⊤:
  ;;
  ;;     pt(param_i of fn) = ⋃ over call sites c calling fn
  ;;                           (from go-callgraph-callers)
  ;;                         of pt_{caller(c)}(arg_i at c)
  ;;
  ;; CONTEXT POLICY — the precision knob you chose value-level to exercise:
  ;;   • context-insensitive: merge all callers' arg sets into one. Cheap, one
  ;;     pass — but conflates the per-request and the shared allocation, which
  ;;     is exactly the instance split value-level exists to preserve. Choosing
  ;;     this throws away the win.
  ;;   • context-sensitive (k-CFA, k=1 suggested): keep arg sets separate per
  ;;     call site so the two instances stay distinct. Costs a factor per k.
  ;; Decide here; this clause IS the detector's false-positive profile.
  ;;
  ;; RECURSION GUARD: a parameter on a call cycle must not recurse unboundedly.
  ;; Collapse strongly-connected call-graph regions to context-insensitive via
  ;; (wile goast path-algebra) path-analysis-sccs / path-node-in-cycle?, and
  ;; bound non-cyclic context at depth k.
  ;;
  ;; CAPTURE EDGES: a "parameter" that is actually a closure free var (bound at
  ;; an ssa-make-closure feeding a `go`/call) must pull provenance from the
  ;; captured value rather than a positional argument — same union shape. This
  ;; is how shared state most often enters a goroutine in Go; miss it and the
  ;; measure under-counts roots (an UNSAFE false candidate).
  ;;
  ;; WIDEN `alloc-universe` to program scope (see its note) once foreign alloc
  ;; ids can appear in a parameter's set.
  ;;
  ;; SOUND DEFAULT until implemented: every parameter ↦ ⊤. A method receiver is
  ;; a parameter, so this makes essentially every lock base ⊤ and the measure
  ;; abstains everywhere — correct, and the precise reason this clause is the
  ;; analysis, not a finishing touch.
  (let ((ps (param-names fn)))
    (map (lambda (k)
           (cons k (if (member k ps) (list *pointsto-unknown*) '())))
         keys)))

;; ─── Engine entry point and queries ───

(define (pointsto-function fn)
  "Value-level points-to over one SSA function.\nForward powerset dataflow: each value maps to the set of abstract allocation\nsites it may reference. Returns the run-analysis result alist ((idx in out) ...).\n\nParameters:\n  fn : ssa-func node\nReturns: list\nCategory: goast-pointsto\n\nSee also: `pointsto-anywhere', `lock-escape-measure', `make-param-seed'."
  (let* ((keys (value-universe fn))
         (val-lat (powerset-lattice (alloc-universe fn)))
         (lat (map-lattice keys val-lat))
         (transfer (make-transfer fn))
         (initial (make-param-seed fn keys)))
    (run-analysis 'forward lat transfer fn (ssa-cfg-protocol)
                  (init-state initial))))

(define (pointsto-at result block-idx value-name)
  "Points-to set of value-name in the out-state of block-idx, or #f.\n\nParameters:\n  result : run-analysis result\n  block-idx : integer\n  value-name : string\nReturns: list-or-false\nCategory: goast-pointsto"
  (let ((st (analysis-out result block-idx)))
    (and st (state-lookup st value-name))))

(define (pointsto-anywhere result value-name)
  "Union of value-name's points-to set across every block's out-state.\nSSA values are single-assignment, so this is the value's invariant provenance\nregardless of where it is referenced.\n\nParameters:\n  result : run-analysis result\n  value-name : string\nReturns: list\nCategory: goast-pointsto"
  (let loop ((states (analysis-states result)) (acc '()))
    (if (null? states)
        acc
        (let ((out (caddr (car states))))
          (loop (cdr states)
                (set-union acc (or (state-lookup out value-name) '())))))))

;; ─── Lock layer: where mutexes are acquired ───

(define (lock-call? instr)
  "True iff instr acquires a lock — a call/defer to a method named \"Lock\".\nCovers (*sync.Mutex).Lock and (*sync.RWMutex).Lock. NOT RLock (add a suffix\ntest if read-lock escape is wanted); \"Unlock\" is excluded — we key the\nacquisition site, where the receiver instance is named.\n\nParameters:\n  instr : ssa instruction node\nReturns: boolean\nCategory: goast-pointsto"
  (and (tag-in? instr '(ssa-call ssa-defer))
       (case (nf instr 'mode)
         ;; Interface dispatch: method name only.
         ((invoke) (equal? (nf instr 'method) "Lock"))
         ;; Static method call: callee is the bare method name ("Lock") for an
         ;; intra-module method, or a qualified "pkg.Type.Lock" otherwise.
         ;; "Unlock" is excluded (capital-L "Lock" suffix does not match it).
         ((call)   (let ((f (or (nf instr 'func) "")))
                     (and (string? f)
                          (or (string=? f "Lock") (ends-with? f ".Lock")))))
         (else #f))))

;; The receiver pointer of a lock acquisition — the value whose provenance
;; decides which instance is locked.
(define (lock-base instr)
  (case (nf instr 'mode)
    ((invoke) (nf instr 'recv))
    ((call)   (let ((args (nf instr 'args))) (and (pair? args) (car args))))
    (else #f)))

(define (lock-sites fn)
  "Lock acquisitions in fn as (fn-name . base-value-name) pairs, where base is\nthe receiver pointer. The fn-name lets the measure group sites across\nfunctions before resolving provenance.\n\nParameters:\n  fn : ssa-func node\nReturns: list\nCategory: goast-pointsto"
  (let ((fn-name (nf fn 'name)))
    (filter-map
      (lambda (i)
        (and (lock-call? i)
             (let ((b (lock-base i)))
               (and b (cons fn-name b)))))
      (ssa-all-instrs fn))))

;; ─── Goroutine roots and the reachability seam ───

(define (goroutine-roots program)
  "Functions launched as goroutines anywhere in the program (targets of `go`).\nThese are the roots whose call-graph reachability defines what can run\nconcurrently. Resolving these value names to call-graph nodes is the\nintegration seam for make-roots-of-fn.\n\nParameters:\n  program : list of ssa-func nodes\nReturns: list\nCategory: goast-pointsto"
  (let loop ((fns program) (acc '()))
    (if (null? fns)
        acc
        (loop (cdr fns)
          (set-union acc
            (filter-map
              (lambda (i)
                (and (tag? i 'ssa-go)
                     (case (nf i 'mode)
                       ((call)   (nf i 'func))
                       ((invoke) (nf i 'method))
                       (else #f))))
              (ssa-all-instrs (car fns))))))))

(define (make-roots-of-fn program reachable-from? . opt-entry)
  "Build a (fn-name -> list of goroutine-root ids) lookup for lock-escape-measure.\n\nreachable-from? : (root-name target-name) -> boolean. Wire this to the call\ngraph — e.g. a closure over (wile goast path-algebra) go-callgraph-reachable\nfrom each root. Injected, not assumed, so the engine here makes no claim about\nthe call graph it hasn't verified.\n\nopt-entry : extra synchronous roots (default '(\"main\")).\n\nParameters:\n  program : list of ssa-func nodes\n  reachable-from? : procedure\nReturns: procedure\nCategory: goast-pointsto"
  (let ((roots (set-union (goroutine-roots program)
                          (if (pair? opt-entry) (car opt-entry) '("main")))))
    (lambda (fn-name)
      (filter (lambda (r) (reachable-from? r fn-name)) roots))))

;; ─── The measure ───

(define (lock-escape-measure program roots-of-fn)
  "Escape measure over locked allocation instances. Returns a self-describing\ntwo-key alist:\n\n  ((resolved   . ((alloc-id . escape) ...))\n   (unresolved . ((fn-name . base) ...)))\n\nresolved   — instances whose lock-receiver provenance was traced to a concrete\n             allocation. escape is:\n               1          ⇒ one goroutine root reaches every lock on it —\n                            a vestigial lock, candidate for collapse;\n               n > 1      ⇒ genuine cross-goroutine sharing — leave it;\n               'unbounded ⇒ poisoned: some OTHER lock site is unresolved (⊤)\n                            and could alias this instance, so its escape\n                            cannot be bounded.\nunresolved — lock sites whose receiver provenance is ⊤, as (fn-name . base).\n\nThe two keys disambiguate the cases an empty answer used to conflate:\n  no locks at all          ⇒ resolved = (),  unresolved = ()\n  locks present, all ⊤     ⇒ resolved = (),  unresolved = (sites...)\n  some resolved, some ⊤    ⇒ resolved = (...,'unbounded), unresolved = (sites...)\nWith make-param-seed returning ⊤ for parameters, and method receivers being\nparameters, real code lands in the middle case until the parameter TODO ships.\n\nprogram     : list of ssa-func nodes (the package).\nroots-of-fn : (fn-name -> list of goroutine-root ids), see make-roots-of-fn.\n\nReturns: alist with keys resolved, unresolved.\nCategory: goast-pointsto\n\nSee also: `pointsto-function', `make-roots-of-fn'."
  (let* ((result-of
           (let ((cache (map (lambda (fn)
                               (cons (nf fn 'name) (pointsto-function fn)))
                             program)))
             (lambda (fn-name) (cdr (assoc fn-name cache)))))
         ;; Each lock site: ((fn-name . base) . points-to-set-of-base).
         (sites
           (apply append
             (map (lambda (fn)
                    (let ((res (result-of (nf fn 'name))))
                      (map (lambda (fb)
                             (cons fb (pointsto-anywhere res (cdr fb))))
                           (lock-sites fn))))
                  program)))
         ;; Lock sites whose receiver provenance is ⊤, reported by location.
         (unresolved
           (let loop ((ss sites) (acc '()))
             (cond ((null? ss) (reverse acc))
                   ((unknown-pointee? (cdr (car ss)))
                    (loop (cdr ss) (cons (car (car ss)) acc)))
                   (else (loop (cdr ss) acc)))))
         (any-unknown? (pair? unresolved))
         ;; Distinct allocation instances that are ever locked (sentinel aside).
         (locked-allocs
           (filter (lambda (a) (not (equal? a *pointsto-unknown*)))
                   (let loop ((ss sites) (acc '()))
                     (if (null? ss) acc
                         (loop (cdr ss) (set-union acc (cdr (car ss))))))))
         (resolved
           (map (lambda (a)
                  (cons a
                        (if any-unknown?
                            'unbounded
                            (let ((locking-fns
                                    (filter-map
                                      (lambda (s)
                                        (and (member a (cdr s)) (car (car s))))
                                      sites)))
                              (length
                                (let loop ((fns locking-fns) (acc '()))
                                  (if (null? fns) acc
                                      (loop (cdr fns)
                                            (set-union acc
                                              (roots-of-fn (car fns)))))))))))
                locked-allocs)))
    (list (cons 'resolved resolved)
          (cons 'unresolved unresolved))))
