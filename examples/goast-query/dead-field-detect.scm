;;; dead-field-detect.scm — Dead and unchecked field analysis
;;;
;;; Finds struct fields that are written but never read (dead fields),
;;; and fields that are read but never used in conditionals (unchecked
;;; mutations). Dead fields are guaranteed waste; unchecked mutations
;;; are noise — state that changes without influencing control flow.
;;;
;;; Uses two goast layers:
;;;   Pass 0 (AST):  Enumerate structs and their fields
;;;   Pass 1 (SSA):  Collect per-field write and read sites across all functions
;;;   Pass 2:        Dead field detection (written, never read)
;;;   Pass 3:        Unchecked mutation detection (read, but never in a conditional)
;;;
;;; Usage: ./dist/wile-goast "(begin $(cat examples/goast-query/dead-field-detect.scm))"

;; ── Target ───────────────────────────────────────────────
(define target "github.com/aalpar/wile/machine")

;; ── Shared utilities ─────────────────────────────────────

(define (nf node key)
  (let ((e (assoc key (cdr node))))
    (if e (cdr e) #f)))

(define (tag? node t)
  (and (pair? node) (eq? (car node) t)))

(define (filter-map f lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (let ((v (f (car xs))))
        (loop (cdr xs) (if v (cons v acc) acc))))))

(define (flat-map f lst)
  (apply append (map f lst)))

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

(define (member? x lst)
  (cond ((null? lst) #f)
        ((equal? x (car lst)) #t)
        (else (member? x (cdr lst)))))

(define (unique lst)
  (let loop ((xs lst) (seen '()))
    (cond ((null? xs) (reverse seen))
          ((member? (car xs) seen) (loop (cdr xs) seen))
          (else (loop (cdr xs) (cons (car xs) seen))))))

(define (has-char? s c)
  (let loop ((i 0))
    (cond ((>= i (string-length s)) #f)
          ((char=? (string-ref s i) c) #t)
          (else (loop (+ i 1))))))

;; Set difference: elements in a not in b.
(define (set-diff a b)
  (filter-map (lambda (x) (and (not (member? x b)) x)) a))

;; ══════════════════════════════════════════════════════════
;; Pass 0: Struct Field Enumeration (AST layer)
;; ══════════════════════════════════════════════════════════

(define (all-field-names field-node)
  (if (tag? field-node 'field)
    (let ((ns (nf field-node 'names)))
      (if (pair? ns) ns '()))
    '()))

(define (find-struct-fields file-ast)
  (walk file-ast
    (lambda (node)
      (and (tag? node 'type-spec)
           (let ((stype (nf node 'type)))
             (and (tag? stype 'struct-type)
                  (let* ((fields (nf stype 'fields))
                         (names (flat-map all-field-names
                                  (if (pair? fields) fields '()))))
                    (and (>= (length names) 1)
                         (list (nf node 'name) names)))))))))

;; ══════════════════════════════════════════════════════════
;; Pass 0.5: AST Method-Field Reads
;;
;; Go's SSA builder does not include synthesized promoted
;; method wrappers. Value-receiver methods like
;; (p OperationBase) String() exist in source but may not
;; appear in go-ssa-build output when only called through
;; promoted wrappers on embedding types.
;;
;; Fix: scan AST for methods on each struct type. Extract
;; field accesses (selector-expr on the receiver) from the
;; method body. These reads supplement the SSA-based read set.
;; ══════════════════════════════════════════════════════════

;; Find all methods defined on a type name (value or pointer receiver).
;; Returns selector names accessed on the receiver within each method.
;; e.g., for (p OperationBase) String(): extracts {"opName", "goName"}
(define (find-method-field-reads file-ast struct-name)
  (walk file-ast
    (lambda (node)
      (and (tag? node 'func-decl)
           (let ((recv (nf node 'recv)))
             ;; recv is a field-list; check if receiver type matches struct-name
             (and (pair? recv)
                  (let* ((recv-field (car recv))
                         (recv-type (nf recv-field 'type))
                         ;; Handle both T and *T receivers
                         (base-type (if (tag? recv-type 'star-expr)
                                      (nf recv-type 'x)
                                      recv-type))
                         ;; Handle bare T, T[P] (generic), and index-list-expr
                         (type-name (cond
                                      ((tag? base-type 'ident) (nf base-type 'name))
                                      ;; Generic type: Pool[T] -> (index-expr (x ident (name . "Pool")))
                                      ((tag? base-type 'index-expr)
                                       (let ((x (nf base-type 'x)))
                                         (and (tag? x 'ident) (nf x 'name))))
                                      (else #f))))
                    (and (equal? type-name struct-name)
                         ;; Get receiver parameter name(s)
                         (let* ((recv-names (nf recv-field 'names))
                                (recv-name (and (pair? recv-names) (car recv-names)))
                                (body (nf node 'body)))
                           ;; Find all selector-expr on this receiver in the body
                           (and recv-name body
                                (let ((sels (walk body
                                        (lambda (n)
                                          (and (tag? n 'selector-expr)
                                               (let ((x (nf n 'x)))
                                                 (and (tag? x 'ident)
                                                      (equal? (nf x 'name) recv-name)
                                                      (nf n 'sel))))))))
                                  (and (pair? sels) sels))))))))))))

;; ══════════════════════════════════════════════════════════
;; Pass 1: Field Write/Read Collection (SSA layer)
;;
;; For each struct, scan all SSA functions and collect:
;;   - write-set: fields stored to (ssa-field-addr + ssa-store)
;;   - read-set:  fields read from (ssa-field, or ssa-field-addr
;;                used as operand to something other than ssa-store)
;;
;; Uses receiver-type disambiguation from co-mutation script:
;; group field-addrs by receiver, exclude receivers that access
;; fields not in the target struct.
;; ══════════════════════════════════════════════════════════

;; Collect ALL ssa-field-addr instructions (for both reads and writes).
;; Returns: ((register-name field-name receiver-name) ...)
(define (collect-field-addrs ssa-func)
  (walk ssa-func
    (lambda (node)
      (and (tag? node 'ssa-field-addr)
           (list (nf node 'name)
                 (nf node 'field)
                 (nf node 'x))))))

;; Collect all ssa-field instructions (direct reads).
;; Returns: ((register-name field-name receiver-name) ...)
(define (collect-field-reads ssa-func)
  (walk ssa-func
    (lambda (node)
      (and (tag? node 'ssa-field)
           (list (nf node 'name)
                 (nf node 'field)
                 (nf node 'x))))))

;; Collect all ssa-store instructions.
;; Returns: ((addr-register val-register) ...)
(define (collect-stores ssa-func)
  (walk ssa-func
    (lambda (node)
      (and (tag? node 'ssa-store)
           (list (nf node 'addr)
                 (nf node 'val))))))

;; Collect all ssa-if instructions and their operands.
;; Returns: ((operand ...) ...) — one operand list per ssa-if.
(define (collect-conditionals ssa-func)
  (walk ssa-func
    (lambda (node)
      (and (tag? node 'ssa-if)
           (let ((ops (nf node 'operands)))
             (if (pair? ops) ops '()))))))

;; Collect all instructions and their operands (for forward tracing).
;; Returns: ((tag register-name (operand ...)) ...)
(define (collect-all-uses ssa-func)
  (walk ssa-func
    (lambda (node)
      (and (pair? node)
           (symbol? (car node))
           (nf node 'name)
           (let ((ops (nf node 'operands)))
             (and (pair? ops)
                  (list (car node) (nf node 'name) ops)))))))

;; Receiver-type disambiguation: only include field-addrs from
;; receivers whose entire field set is a subset of struct-fields.
(define (valid-receivers-for-struct all-field-addrs struct-fields)
  (let ((receivers (unique (map caddr all-field-addrs))))
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
      receivers)))

;; Compute write-set and read-set for one function against one struct.
;; Returns: (written-fields read-fields conditional-fields)
;;   written-fields: field names stored to through valid receivers
;;   read-fields: field names read (ssa-field or ssa-field-addr used as non-store operand)
;;   conditional-fields: subset of read-fields that feed into ssa-if
;; known-func-names: set of all SSA function names in the package
;;   (used to distinguish internal static calls from external/dynamic calls)
(define (field-usage-in-func ssa-func struct-fields known-func-names)
  (let* ((all-fas (collect-field-addrs ssa-func))
         (direct-reads (collect-field-reads ssa-func))
         (stores (collect-stores ssa-func))
         (store-addrs (map car stores))
         (conditionals (collect-conditionals ssa-func))
         (cond-operands (apply append conditionals))
         (all-uses (collect-all-uses ssa-func))
         (valid-recvs (valid-receivers-for-struct all-fas struct-fields))

         ;; Written fields: field-addr register is a store target
         (written
           (unique
             (filter-map
               (lambda (fa)
                 (let ((reg (car fa)) (field (cadr fa)) (recv (caddr fa)))
                   (and (member? recv valid-recvs)
                        (member? reg store-addrs)
                        (member? field struct-fields)
                        field)))
               all-fas)))

         ;; Read via ssa-field (direct struct field read)
         ;; No receiver disambiguation needed for ssa-field — it
         ;; carries struct type implicitly (only fires on value types)
         (direct-read-fields
           (unique
             (filter-map
               (lambda (fr)
                 (let ((field (cadr fr)))
                   (and (member? field struct-fields) field)))
               direct-reads)))

         ;; Read via ssa-field-addr: two cases count as reads:
         ;; (a) The addr register is NOT a store target (used for load/comparison)
         ;; (b) The addr register is an operand to any ssa-call (passed to
         ;;     an external function that might read through the pointer —
         ;;     conservative but eliminates false positives from callbacks,
         ;;     fmt.Println, json.Marshal, etc.)
         (call-operands
           (flat-map
             (lambda (use)
               (if (eq? (car use) 'ssa-call) (caddr use) '()))
             all-uses))

         (addr-read-fields
           (unique
             (filter-map
               (lambda (fa)
                 (let ((reg (car fa)) (field (cadr fa)) (recv (caddr fa)))
                   (and (member? recv valid-recvs)
                        (member? field struct-fields)
                        (or (not (member? reg store-addrs))
                            (member? reg call-operands))
                        field)))
               all-fas)))

         ;; Receiver-passed-to-call: if the struct receiver value
         ;; (or a deref/conversion of it) is passed to any ssa-call,
         ;; the callee might read ANY field. This handles:
         ;; - Callbacks: fireImportObserver passes struct to observer(event)
         ;; - fmt/json: fmt.Println(s) reads fields via reflection
         ;; - Interface dispatch: err passed as error interface
         ;;
         ;; Check: does the receiver (or any ssa-unop/ssa-make-interface
         ;; derived from it) appear as a call operand?
         (recv-derived-values
           ;; Start with valid receivers, then add values derived from them
           ;; (one level: unop/deref, make-interface)
           (let ((derived (filter-map
                            (lambda (use)
                              (let ((use-tag (car use))
                                    (use-reg (cadr use))
                                    (use-ops (caddr use)))
                                (and (or (eq? use-tag 'ssa-unop)
                                         (eq? use-tag 'ssa-make-interface)
                                         (eq? use-tag 'ssa-change-type))
                                     (let loop ((ops use-ops))
                                       (cond ((null? ops) #f)
                                             ((member? (car ops) valid-recvs) use-reg)
                                             (else (loop (cdr ops))))))))
                            all-uses)))
             (append valid-recvs derived)))

         ;; Collect ssa-call details: func target + whether receiver is an arg.
         ;; Only flag "all fields read" for calls where:
         ;;   (a) The receiver/derived value is an argument, AND
         ;;   (b) The call target is external or dynamic (function value).
         ;; Internal same-package calls are already analyzed via SSA/AST.
         (call-details
           (walk ssa-func
             (lambda (node)
               (and (tag? node 'ssa-call)
                    (let ((fn (nf node 'func))     ;; call target (name or register)
                          (args (nf node 'args))   ;; argument list
                          (mode (nf node 'mode)))  ;; 'call or 'invoke
                      (list fn args mode))))))

         (receiver-passed-to-external-call
           (let loop ((calls call-details))
             (cond
               ((null? calls) #f)
               (else
                 (let* ((call (car calls))
                        (fn-target (car call))
                        (args (cadr call))
                        (mode (caddr call))
                        ;; Is the receiver or a derived value among the args?
                        (recv-in-args
                          (and (pair? args)
                               (let inner ((as args))
                                 (cond ((null? as) #f)
                                       ((member? (car as) recv-derived-values) #t)
                                       (else (inner (cdr as)))))))
                        ;; Is this an external/dynamic call?
                        ;; - Interface invocations (mode=invoke) are always dynamic
                        ;; - Static calls where fn-target is a known package function
                        ;;   are internal (we already analyze those via SSA/AST)
                        ;; - Everything else is external or a function-value call
                        (is-external-or-dynamic
                          (or (eq? mode 'invoke)
                              (not (member? fn-target known-func-names)))))
                   (if (and recv-in-args is-external-or-dynamic)
                     #t
                     (loop (cdr calls))))))))

         ;; If receiver is passed to an external/dynamic call, ALL fields are potentially read
         (call-read-fields
           (if receiver-passed-to-external-call struct-fields '()))

         (read-fields (unique (append direct-read-fields addr-read-fields call-read-fields)))

         ;; Conditional fields: read registers that appear as operands to ssa-if,
         ;; OR read registers that feed into a value that appears in ssa-if
         ;; (one level of indirection — covers comparisons).
         ;;
         ;; Level 0: direct reads whose register is a conditional operand
         (direct-cond-reads
           (filter-map
             (lambda (fr)
               (let ((reg (car fr)) (field (cadr fr)))
                 (and (member? field struct-fields)
                      (member? reg cond-operands)
                      field)))
             direct-reads))

         ;; Level 1: reads whose register feeds into an instruction
         ;; whose register is then a conditional operand.
         ;; This catches: t1 = s.field; t2 = t1 != nil; if t2 ...
         (read-regs
           (append
             (filter-map
               (lambda (fr)
                 (let ((reg (car fr)) (field (cadr fr)))
                   (and (member? field struct-fields)
                        (cons reg field))))
               direct-reads)
             (filter-map
               (lambda (fa)
                 (let ((reg (car fa)) (field (cadr fa)) (recv (caddr fa)))
                   (and (member? recv valid-recvs)
                        (member? field struct-fields)
                        (not (member? reg store-addrs))
                        (cons reg field))))
               all-fas)))

         (indirect-cond-reads
           (filter-map
             (lambda (use)
               (let ((use-reg (cadr use))
                     (use-ops (caddr use)))
                 (and (member? use-reg cond-operands)
                      ;; Find which read-reg feeds into this use
                      (let loop ((rrs read-regs))
                        (cond ((null? rrs) #f)
                              ((member? (caar rrs) use-ops) (cdar rrs))
                              (else (loop (cdr rrs))))))))
             all-uses))

         (conditional-fields (unique (append direct-cond-reads indirect-cond-reads))))

    (list written read-fields conditional-fields)))

;; ══════════════════════════════════════════════════════════
;; Main
;; ══════════════════════════════════════════════════════════

(display "Loading and type-checking ") (display target) (display " ...")
(newline)
(define pkgs (go-typecheck-package target))

(display "Building SSA for ") (display target) (display " ...")
(newline)
(define ssa-funcs (go-ssa-build target))
(newline)

(display "══════════════════════════════════════════════════")
(newline)
(display "  Dead & Unchecked Field Analysis                 ")
(newline)
(display "══════════════════════════════════════════════════")
(newline) (newline)

;; ── Pass 0 ───────────────────────────────────────────────
(display "── Pass 0: Struct Field Enumeration (AST) ──") (newline)
(define all-structs '())
(for-each
  (lambda (pkg)
    (for-each
      (lambda (file)
        (for-each
          (lambda (s)
            (set! all-structs (cons s all-structs))
            (display "  struct ") (display (car s))
            (display ": ") (display (length (cadr s))) (display " fields")
            (newline))
          (find-struct-fields file)))
      (nf pkg 'files)))
  pkgs)
(if (null? all-structs) (begin (display "  (none found)") (newline)))
(newline)

;; ── Build known function name set for call classification ─
(define known-func-names
  (filter-map
    (lambda (fn) (nf fn 'name))
    ssa-funcs))

;; ── Pass 0.5: AST Method Field Reads ─────────────────────
(display "── Pass 0.5: Method Field Reads (AST) ──") (newline)
;; For each struct, find fields read in methods defined on that type.
;; This catches reads invisible to SSA (promoted value-receiver methods,
;; synthesized wrappers not in go-ssa-build output).
(define all-method-reads '())
(for-each
  (lambda (s)
    (let* ((struct-name (car s))
           (method-reads
             (unique (flat-map
               (lambda (pkg)
                 (flat-map
                   (lambda (file)
                     (flat-map
                       (lambda (reads) reads)
                       (find-method-field-reads file struct-name)))
                   (nf pkg 'files)))
               pkgs))))
      (set! all-method-reads (cons (cons struct-name method-reads) all-method-reads))
      (if (pair? method-reads)
        (begin
          (display "  struct ") (display struct-name)
          (display ": method reads ") (display method-reads) (newline)))))
  all-structs)
(newline)

;; ── Pass 1: Aggregate field usage across all functions ───
(display "── Pass 1: Field Usage Aggregation (SSA + AST methods) ──") (newline)

;; For each struct, aggregate write/read/conditional sets across all functions,
;; plus AST-based method reads.
(define all-usage '())
(for-each
  (lambda (s)
    (let* ((struct-name (car s))
           (fields (cadr s))
           (per-func
             (filter-map
               (lambda (fn)
                 (let* ((fname (nf fn 'name))
                        (usage (field-usage-in-func fn fields known-func-names))
                        (written (car usage))
                        (reads (cadr usage))
                        (conds (caddr usage)))
                   (and (not (has-char? fname #\$))
                        (or (pair? written) (pair? reads))
                        (list fname written reads conds))))
               ssa-funcs))
           ;; SSA-based reads
           (ssa-written (unique (flat-map cadr per-func)))
           (ssa-read (unique (flat-map caddr per-func)))
           (ssa-conditional (unique (flat-map cadddr per-func)))
           ;; AST method reads (fields accessed in methods on this type)
           (method-reads (let ((entry (assoc struct-name all-method-reads)))
                           (if entry (cdr entry) '())))
           ;; Combined read set: SSA reads + AST method reads
           (global-written ssa-written)
           (global-read (unique (append ssa-read method-reads)))
           (global-conditional ssa-conditional))
      (set! all-usage (cons (list struct-name fields
                                  global-written global-read global-conditional
                                  per-func)
                            all-usage))
      (if (or (pair? per-func) (pair? method-reads))
        (begin
          (display "  struct ") (display struct-name)
          (display ": written=") (display (length global-written))
          (display " read=") (display (length global-read))
          (if (pair? method-reads)
            (begin (display " (incl ") (display (length method-reads)) (display " from methods)")))
          (display " checked=") (display (length global-conditional))
          (display " (of ") (display (length fields)) (display " fields)")
          (newline)))))
  all-structs)
(newline)

;; ── Pass 2: Dead Fields ──────────────────────────────────
(display "── Pass 2: Dead Fields (written, never read) ──") (newline)
(define dead-count 0)
(for-each
  (lambda (usage)
    (let* ((struct-name (car usage))
           (fields (cadr usage))
           (global-written (caddr usage))
           (global-read (cadddr usage))
           (dead (set-diff global-written global-read)))
      (if (pair? dead)
        (begin
          (display "  struct ") (display struct-name) (display ":") (newline)
          (for-each
            (lambda (field)
              (set! dead-count (+ dead-count 1))
              ;; Show which functions write this dead field
              (let ((writers (filter-map
                               (lambda (pf)
                                 (and (member? field (cadr pf))
                                      (car pf)))
                               (list-ref usage 5))))
                (display "    DEAD: ") (display field)
                (display " — written by: ") (display writers)
                (newline)))
            dead)))))
  all-usage)
(if (= dead-count 0) (begin (display "  (none found)") (newline)))
(newline)

;; ── Pass 3: Unchecked Mutations ──────────────────────────
;; Fields that are written AND read, but the reads never feed
;; into a conditional. The value changes but nothing branches on it.
(display "── Pass 3: Unchecked Mutations (read, never in conditional) ──") (newline)
(define unchecked-count 0)
(for-each
  (lambda (usage)
    (let* ((struct-name (car usage))
           (fields (cadr usage))
           (global-written (caddr usage))
           (global-read (cadddr usage))
           (global-conditional (list-ref usage 4))
           ;; Fields that are both written and read, but never conditional
           (written-and-read (filter-map
                               (lambda (f) (and (member? f global-read) f))
                               global-written))
           (unchecked (set-diff written-and-read global-conditional)))
      (if (pair? unchecked)
        (begin
          (display "  struct ") (display struct-name) (display ":") (newline)
          (for-each
            (lambda (field)
              (set! unchecked-count (+ unchecked-count 1))
              ;; Show writers and readers
              (let ((writers (filter-map
                               (lambda (pf)
                                 (and (member? field (cadr pf))
                                      (car pf)))
                               (list-ref usage 5)))
                    (readers (filter-map
                               (lambda (pf)
                                 (and (member? field (caddr pf))
                                      (car pf)))
                               (list-ref usage 5))))
                (display "    UNCHECKED: ") (display field)
                (newline)
                (display "      writers: ") (display writers)
                (newline)
                (display "      readers: ") (display readers)
                (newline)))
            unchecked)))))
  all-usage)
(if (= unchecked-count 0) (begin (display "  (none found)") (newline)))
(newline)

;; ── Summary ──────────────────────────────────────────────
(display "── Summary ──") (newline)
(display "  Structs analyzed:      ") (display (length all-structs)) (newline)
(display "  Dead fields:           ") (display dead-count) (newline)
(display "  Unchecked mutations:   ") (display unchecked-count) (newline)
