;;; (wile goast domains) — Pre-built abstract domains for C2 dataflow analysis
;;;
;;; Five domains: reaching definitions, liveness, constant propagation,
;;; sign analysis, interval analysis. All built on (wile goast dataflow)
;;; run-analysis and (wile algebra) lattice constructors.

;; ─── Local utilities ────────────────────────────

(define (filter pred lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (loop (cdr xs)
            (if (pred (car xs)) (cons (car xs) acc) acc)))))

;; ─── String helpers ─────────────────────────────

(define (find-char str ch)
  (let loop ((i 0))
    (cond ((>= i (string-length str)) #f)
          ((char=? (string-ref str i) ch) i)
          (else (loop (+ i 1))))))

;; ─── SSA constant parsing ──────────────────────

(define *integer-types*
  '("int" "int8" "int16" "int32" "int64"
    "uint" "uint8" "uint16" "uint32" "uint64"
    "untyped int"))

(define (parse-ssa-const name)
  "Parse an SSA constant string to a Scheme integer, or #f.\nHandles forms like \"3:int\", \"-5:int64\".\n\nParameters:\n  name : string\nReturns: integer-or-false\nCategory: goast-domains"
  (let ((colon (find-char name #\:)))
    (and colon
         (let ((type-str (substring name (+ colon 1) (string-length name))))
           (and (member type-str *integer-types*)
                (string->number (substring name 0 colon)))))))

;; ─── Go token to opcode mapping ────────────────

(define (go-token->opcode sym)
  (case sym
    ((+) 'add) ((-) 'sub) ((*) 'mul)
    ((/) 'div) ((%) 'rem)
    ((&) 'and) ((|\||) 'or) ((^) 'xor)
    (else #f)))

;; ─── Concrete evaluator ────────────────────────

(define (go-concrete-eval opcode a b)
  "Evaluate a Go SSA integer opcode on Scheme integers.\nReturns integer result or 'unknown for unsupported opcodes or division by zero.\n\nParameters:\n  opcode : symbol\n  a : integer\n  b : integer\nReturns: integer-or-symbol\nCategory: goast-domains\n\nExamples:\n  (go-concrete-eval 'add 2 3)  ; => 5\n  (go-concrete-eval 'div 10 0) ; => unknown\n\nSee also: `make-constant-propagation'."
  (case opcode
    ((add) (+ a b))
    ((sub) (- a b))
    ((mul) (* a b))
    ((div) (if (zero? b) 'unknown (quotient a b)))
    ((rem) (if (zero? b) 'unknown (remainder a b)))
    (else  'unknown)))

;; ─── State helpers for map-lattice analyses ────

(define (state-update state key val)
  (map (lambda (entry)
         (if (equal? (car entry) key)
             (cons key val)
             entry))
       state))

(define (state-lookup state name)
  (let ((pair (assoc name state)))
    (if pair (cdr pair) #f)))

;; Resolve an SSA operand to a lattice value.
;; For map-lattice analyses: look up in state, or parse as constant.
(define (resolve-value state name)
  (let ((in-state (state-lookup state name)))
    (cond (in-state in-state)
          ((parse-ssa-const name) =>
           (lambda (v) v))
          (else 'flat-top))))

;; ─── Domain 1: Reaching Definitions (forward, powerset) ───

(define (make-reaching-definitions ssa-fn)
  "Forward reaching definitions analysis using powerset lattice.\nState at each block is the set of SSA names whose definitions reach that point.\n\nParameters:\n  ssa-fn : list\nReturns: list\nCategory: goast-domains\n\nSee also: `make-liveness', `run-analysis'."
  (let* ((universe (ssa-instruction-names ssa-fn))
         (lat (powerset-lattice universe))
         (transfer (lambda (block state)
                     (let ((names (filter-map
                                    (lambda (i) (nf i 'name))
                                    (block-instrs block))))
                       (lattice-join lat state names)))))
    (run-analysis 'forward lat transfer ssa-fn)))

;; ─── Domain 2: Liveness (backward, powerset) ──────────────

(define (make-liveness ssa-fn)
  "Backward liveness analysis using powerset lattice.\nState at each block is the set of SSA names that are live (used before redefined).\n\nParameters:\n  ssa-fn : list\nReturns: list\nCategory: goast-domains\n\nSee also: `make-reaching-definitions', `run-analysis'."
  (let* ((universe (ssa-instruction-names ssa-fn))
         (lat (powerset-lattice universe))
         (transfer
           (lambda (block state)
             (let loop ((instrs (reverse (block-instrs block)))
                        (st state))
               (if (null? instrs) st
                   (let* ((instr (car instrs))
                          (nm (nf instr 'name))
                          ;; Kill: remove defined name
                          (st1 (if nm
                                   (filter (lambda (x) (not (equal? x nm))) st)
                                   st))
                          ;; Gen: add used operands that are in universe
                          (ops (or (nf instr 'operands) '()))
                          (used (filter (lambda (o) (member o universe)) ops))
                          (st2 (lattice-join lat st1 used)))
                     (loop (cdr instrs) st2)))))))
    (run-analysis 'backward lat transfer ssa-fn)))

;; ─── Domain 3: Constant Propagation (forward, flat + map-lattice) ─

(define (make-constant-propagation ssa-fn)
  "Forward constant propagation using flat lattice over integers.\nState maps each SSA name to a concrete integer, flat-bottom (undefined),\nor flat-top (non-constant).\n\nParameters:\n  ssa-fn : list\nReturns: list\nCategory: goast-domains\n\nSee also: `go-concrete-eval', `make-sign-analysis', `run-analysis'."
  (let* ((names (ssa-instruction-names ssa-fn))
         (val-lat (flat-lattice '() equal?))
         (lat (map-lattice names val-lat))
         (transfer
           (lambda (block state)
             (let loop ((instrs (block-instrs block)) (st state))
               (if (null? instrs) st
                   (let ((instr (car instrs)))
                     (loop (cdr instrs)
                       (cond
                         ;; BinOp: fold if both operands are concrete
                         ((tag? instr 'ssa-binop)
                          (let* ((nm (nf instr 'name))
                                 (op-sym (nf instr 'op))
                                 (opcode (go-token->opcode op-sym))
                                 (v1 (resolve-value st (nf instr 'x)))
                                 (v2 (resolve-value st (nf instr 'y)))
                                 (result
                                   (cond
                                     ((or (eq? v1 'flat-bottom) (eq? v2 'flat-bottom))
                                      'flat-bottom)
                                     ((or (eq? v1 'flat-top) (eq? v2 'flat-top))
                                      'flat-top)
                                     ((and opcode (integer? v1) (integer? v2))
                                      (let ((r (go-concrete-eval opcode v1 v2)))
                                        (if (eq? r 'unknown) 'flat-top r)))
                                     (else 'flat-top))))
                            (if nm (state-update st nm result) st)))

                         ;; Phi: join all operand values
                         ((tag? instr 'ssa-phi)
                          (let* ((nm (nf instr 'name))
                                 (ops (or (nf instr 'operands) '()))
                                 (result
                                   (let join-ops ((os ops) (acc 'flat-bottom))
                                     (if (null? os) acc
                                         (join-ops (cdr os)
                                           (lattice-join val-lat acc
                                             (resolve-value st (car os))))))))
                            (if nm (state-update st nm result) st)))

                         ;; Anything else with a name: conservative top
                         ((nf instr 'name)
                          (state-update st (nf instr 'name) 'flat-top))

                         ;; No name (store, jump, etc.): pass through
                         (else st)))))))))
    (run-analysis 'forward lat transfer ssa-fn)))

;; ─── Domain 4: Sign Analysis (forward, 5-element) ────────

(define (sign-lattice)
  "Construct the sign lattice: {flat-bottom, neg, zero, pos, flat-top}.\nThe three middle elements are incomparable.\n\nReturns: lattice\nCategory: goast-domains\n\nSee also: `make-sign-analysis'."
  (flat-lattice '(neg zero pos) eq?))

;; Abstract a concrete integer to its sign.
(define (abstract-sign n)
  (cond ((< n 0) 'neg)
        ((= n 0) 'zero)
        (else    'pos)))

;; Sign transfer tables: (op sign-a sign-b) -> sign-result
(define (sign-binop op a b)
  (let ((bot 'flat-bottom) (top 'flat-top))
    (cond
      ((or (eq? a bot) (eq? b bot)) bot)
      ((and (eq? op 'mul) (or (eq? a 'zero) (eq? b 'zero))) 'zero)
      ((or (eq? a top) (eq? b top)) top)
      (else
        (case op
          ((add)
           (case a
             ((neg)  (case b ((neg) 'neg)  ((zero) 'neg) ((pos) top)))
             ((zero) b)
             ((pos)  (case b ((neg) top)   ((zero) 'pos) ((pos) 'pos)))))
          ((sub)
           (case a
             ((neg)  (case b ((neg) top)   ((zero) 'neg) ((pos) 'neg)))
             ((zero) (case b ((neg) 'pos)  ((zero) 'zero) ((pos) 'neg)))
             ((pos)  (case b ((neg) 'pos)  ((zero) 'pos) ((pos) top)))))
          ((mul)
           (case a
             ((neg)  (case b ((neg) 'pos)  ((zero) 'zero) ((pos) 'neg)))
             ((zero) 'zero)
             ((pos)  (case b ((neg) 'neg)  ((zero) 'zero) ((pos) 'pos)))))
          (else top))))))

;; Resolve an operand to a sign value.
(define (resolve-sign state name)
  (let ((in-state (state-lookup state name)))
    (cond (in-state in-state)
          ((parse-ssa-const name) => abstract-sign)
          (else 'flat-top))))

(define (make-sign-analysis ssa-fn)
  "Forward sign analysis using the 5-element sign lattice.\nState maps each SSA name to neg, zero, pos, flat-bottom, or flat-top.\n\nParameters:\n  ssa-fn : list\nReturns: list\nCategory: goast-domains\n\nSee also: `sign-lattice', `make-constant-propagation', `run-analysis'."
  (let* ((names (ssa-instruction-names ssa-fn))
         (s-lat (sign-lattice))
         (lat (map-lattice names s-lat))
         (transfer
           (lambda (block state)
             (let loop ((instrs (block-instrs block)) (st state))
               (if (null? instrs) st
                   (let ((instr (car instrs)))
                     (loop (cdr instrs)
                       (cond
                         ((tag? instr 'ssa-binop)
                          (let* ((nm (nf instr 'name))
                                 (opcode (go-token->opcode (nf instr 'op)))
                                 (v1 (resolve-sign st (nf instr 'x)))
                                 (v2 (resolve-sign st (nf instr 'y)))
                                 (result (if opcode
                                             (sign-binop opcode v1 v2)
                                             'flat-top)))
                            (if nm (state-update st nm result) st)))

                         ((tag? instr 'ssa-phi)
                          (let* ((nm (nf instr 'name))
                                 (ops (or (nf instr 'operands) '()))
                                 (result
                                   (let join-ops ((os ops) (acc 'flat-bottom))
                                     (if (null? os) acc
                                         (join-ops (cdr os)
                                           (lattice-join s-lat acc
                                             (resolve-sign st (car os))))))))
                            (if nm (state-update st nm result) st)))

                         ((nf instr 'name)
                          (state-update st (nf instr 'name) 'flat-top))

                         (else st)))))))))
    (run-analysis 'forward lat transfer ssa-fn)))

;; ─── Domain 5: Interval Analysis (forward, widening) ──────

;; Infinity-aware comparison and arithmetic.

(define (inf<= a b)
  (cond ((eq? a 'neg-inf) #t)
        ((eq? b 'pos-inf) #t)
        ((eq? b 'neg-inf) #f)
        ((eq? a 'pos-inf) #f)
        (else (<= a b))))

(define (inf-min a b) (if (inf<= a b) a b))
(define (inf-max a b) (if (inf<= a b) b a))

(define (inf+ a b)
  (cond ((or (and (eq? a 'pos-inf) (eq? b 'neg-inf))
             (and (eq? a 'neg-inf) (eq? b 'pos-inf)))
         'pos-inf)
        ((or (eq? a 'pos-inf) (eq? b 'pos-inf)) 'pos-inf)
        ((or (eq? a 'neg-inf) (eq? b 'neg-inf)) 'neg-inf)
        (else (+ a b))))

(define (inf- a b)
  (cond ((or (and (eq? a 'pos-inf) (eq? b 'pos-inf))
             (and (eq? a 'neg-inf) (eq? b 'neg-inf)))
         'pos-inf)
        ((eq? a 'pos-inf) 'pos-inf)
        ((eq? a 'neg-inf) 'neg-inf)
        ((eq? b 'pos-inf) 'neg-inf)
        ((eq? b 'neg-inf) 'pos-inf)
        (else (- a b))))

(define (inf* a b)
  (cond ((or (and (eqv? a 0) (or (eq? b 'pos-inf) (eq? b 'neg-inf)))
             (and (eqv? b 0) (or (eq? a 'pos-inf) (eq? a 'neg-inf))))
         0)
        ((and (eq? a 'pos-inf) (eq? b 'pos-inf)) 'pos-inf)
        ((and (eq? a 'neg-inf) (eq? b 'neg-inf)) 'pos-inf)
        ((or (and (eq? a 'pos-inf) (eq? b 'neg-inf))
             (and (eq? a 'neg-inf) (eq? b 'pos-inf)))
         'neg-inf)
        ((eq? a 'pos-inf) (if (< b 0) 'neg-inf 'pos-inf))
        ((eq? a 'neg-inf) (if (< b 0) 'pos-inf 'neg-inf))
        ((eq? b 'pos-inf) (if (< a 0) 'neg-inf 'pos-inf))
        ((eq? b 'neg-inf) (if (< a 0) 'pos-inf 'neg-inf))
        (else (* a b))))

(define (interval-lattice)
  "Construct the interval lattice with infinity-aware arithmetic.\nElements are (lo . hi) pairs, interval-bot, or (neg-inf . pos-inf) as top.\n\nReturns: lattice\nCategory: goast-domains\n\nSee also: `make-interval-analysis'."
  (make-lattice
    ;; join: widen to encompass both
    (lambda (a b)
      (cond ((eq? a 'interval-bot) b)
            ((eq? b 'interval-bot) a)
            (else (cons (inf-min (car a) (car b))
                        (inf-max (cdr a) (cdr b))))))
    ;; meet: narrow to intersection
    (lambda (a b)
      (cond ((eq? a 'interval-bot) 'interval-bot)
            ((eq? b 'interval-bot) 'interval-bot)
            (else (let ((lo (inf-max (car a) (car b)))
                        (hi (inf-min (cdr a) (cdr b))))
                    (if (inf<= lo hi)
                        (cons lo hi)
                        'interval-bot)))))
    'interval-bot
    (cons 'neg-inf 'pos-inf)
    ;; leq: a contained in b
    (lambda (a b)
      (cond ((eq? a 'interval-bot) #t)
            ((eq? b 'interval-bot) #f)
            (else (and (inf<= (car b) (car a))
                       (inf<= (cdr a) (cdr b))))))))

;; Interval arithmetic on (lo . hi) pairs.
(define (interval-add a b)
  (cons (inf+ (car a) (car b)) (inf+ (cdr a) (cdr b))))

(define (interval-sub a b)
  (cons (inf- (car a) (cdr b)) (inf- (cdr a) (car b))))

(define (interval-mul a b)
  (let* ((corners (list (inf* (car a) (car b))
                        (inf* (car a) (cdr b))
                        (inf* (cdr a) (car b))
                        (inf* (cdr a) (cdr b))))
         (lo (let loop ((cs (cdr corners)) (m (car corners)))
               (if (null? cs) m (loop (cdr cs) (inf-min m (car cs))))))
         (hi (let loop ((cs (cdr corners)) (m (car corners)))
               (if (null? cs) m (loop (cdr cs) (inf-max m (car cs)))))))
    (cons lo hi)))

;; Abstract a concrete integer to a point interval.
(define (abstract-interval n) (cons n n))

;; Resolve an operand to an interval value.
(define (resolve-interval state name)
  (let ((in-state (state-lookup state name)))
    (cond (in-state in-state)
          ((parse-ssa-const name) => abstract-interval)
          (else (cons 'neg-inf 'pos-inf)))))

(define (make-interval-analysis ssa-fn . args)
  "Forward interval analysis with per-block widening.\nOptional second arg is the widening threshold (default 3).\nState maps each SSA name to an interval (lo . hi) or interval-bot.\n\nParameters:\n  ssa-fn : list\nReturns: list\nCategory: goast-domains\n\nSee also: `interval-lattice', `make-constant-propagation', `run-analysis'."
  (let* ((threshold (if (pair? args) (car args) 3))
         (names (ssa-instruction-names ssa-fn))
         (i-lat (interval-lattice))
         (lat (map-lattice names i-lat))
         ;; Per-block visit counter for widening.
         (visit-counts '())
         (get-visits (lambda (idx)
                       (let ((e (assv idx visit-counts)))
                         (if e (cdr e) 0))))
         (inc-visits! (lambda (idx)
                        (let ((e (assv idx visit-counts)))
                          (if e
                              (set-cdr! e (+ (cdr e) 1))
                              (set! visit-counts
                                (cons (cons idx 1) visit-counts))))))
         ;; Widen a single interval: push growing bounds to infinity.
         (widen-interval
           (lambda (old new)
             (cond ((eq? old 'interval-bot) new)
                   ((eq? new 'interval-bot) old)
                   (else
                     (cons (if (inf<= (car new) (car old)) (car new) 'neg-inf)
                           (if (inf<= (cdr old) (cdr new)) (cdr new) 'pos-inf))))))
         ;; Widen an entire map-lattice state: pointwise on each key.
         (widen-state
           (lambda (old-st new-st)
             (map (lambda (old-entry new-entry)
                    (cons (car new-entry)
                          (widen-interval (cdr old-entry) (cdr new-entry))))
                  old-st new-st)))
         (transfer
           (lambda (block state)
             (let* ((idx (nf block 'index))
                    (visits (get-visits idx))
                    (raw-result
                      (let loop ((instrs (block-instrs block)) (st state))
                        (if (null? instrs) st
                            (let ((instr (car instrs)))
                              (loop (cdr instrs)
                                (cond
                                  ((tag? instr 'ssa-binop)
                                   (let* ((nm (nf instr 'name))
                                          (opcode (go-token->opcode
                                                    (nf instr 'op)))
                                          (v1 (resolve-interval
                                                st (nf instr 'x)))
                                          (v2 (resolve-interval
                                                st (nf instr 'y)))
                                          (result
                                            (cond
                                              ((or (eq? v1 'interval-bot)
                                                   (eq? v2 'interval-bot))
                                               'interval-bot)
                                              (else
                                                (case opcode
                                                  ((add) (interval-add v1 v2))
                                                  ((sub) (interval-sub v1 v2))
                                                  ((mul) (interval-mul v1 v2))
                                                  (else (cons 'neg-inf 'pos-inf)))))))
                                     (if nm
                                         (state-update st nm result)
                                         st)))

                                  ((tag? instr 'ssa-phi)
                                   (let* ((nm (nf instr 'name))
                                          (ops (or (nf instr 'operands) '()))
                                          (result
                                            (let join-ops
                                              ((os ops)
                                               (acc 'interval-bot))
                                              (if (null? os) acc
                                                  (join-ops (cdr os)
                                                    (lattice-join i-lat acc
                                                      (resolve-interval
                                                        st (car os))))))))
                                     (if nm
                                         (state-update st nm result)
                                         st)))

                                  ((nf instr 'name)
                                   (state-update st (nf instr 'name)
                                     (cons 'neg-inf 'pos-inf)))

                                  (else st))))))))
               (inc-visits! idx)
               (if (> visits threshold)
                   (widen-state state raw-result)
                   raw-result)))))
    (run-analysis 'forward lat transfer ssa-fn)))
