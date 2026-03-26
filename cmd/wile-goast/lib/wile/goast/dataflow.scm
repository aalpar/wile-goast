;;; (wile goast dataflow) — Def-use reachability via (wile algebra)
;;;
;;; Provides lattice-based bounded reachability on SSA def-use graphs.
;;; Foundation for dataflow analysis (C2).

;; ─── Boolean lattice {#f, #t} ───────────────

(define (boolean-lattice)
  (make-lattice
    (lambda (a b) (or a b))         ; join
    (lambda (a b) (and a b))        ; meet
    #f                               ; bottom
    #t                               ; top
    (lambda (a b) (or (not a) b))))  ; leq? (implication)

;; ─── SSA instruction extraction ─────────────

(define (ssa-all-instrs ssa-fn)
  (let ((blocks (nf ssa-fn 'blocks)))
    (if (pair? blocks)
      (flat-map
        (lambda (b)
          (let ((is (nf b 'instrs)))
            (if (pair? is) is '())))
        blocks)
      '())))

(define (ssa-instruction-names ssa-fn)
  (let ((instrs (ssa-all-instrs ssa-fn)))
    (unique
      (flat-map
        (lambda (instr)
          (let ((nm (nf instr 'name))
                (ops (nf instr 'operands)))
            (append
              (if nm (list nm) '())
              (if (and (tag? instr 'ssa-store) (pair? ops)) ops '()))))
        instrs))))

;; ─── Reachability transfer function ─────────

(define (make-reachability-transfer all-instrs found? names-lat)
  (lambda (state)
    (let ((names (car state)) (guard (cadr state)))
      (if guard state
        (let* ((reached (filter-map
                          (lambda (instr)
                            (let ((ops (nf instr 'operands)))
                              (and (pair? ops)
                                   (let check ((ns names))
                                     (cond ((null? ns) #f)
                                           ((member? (car ns) ops) instr)
                                           (else (check (cdr ns))))))))
                          all-instrs))
               (guard-hit (let loop ((rs reached))
                            (cond ((null? rs) #f)
                                  ((found? (car rs)) #t)
                                  (else (loop (cdr rs))))))
               (new-names (flat-map
                            (lambda (instr)
                              (let ((nm (nf instr 'name)))
                                (cond
                                  ((and nm (not (member? nm names)))
                                   (list nm))
                                  ((tag? instr 'ssa-store)
                                   (filter-map
                                     (lambda (op)
                                       (and (not (member? op names)) op))
                                     (or (nf instr 'operands) '())))
                                  (else '()))))
                            reached))
               (joined (lattice-join names-lat names new-names)))
          (list joined (or guard guard-hit)))))))

;; ─── Block accessor ────────────────────────

(define (block-instrs block)
  (or (nf block 'instrs) '()))

;; ─── Reverse postorder (internal) ──────────

(define (reverse-postorder blocks)
  (let ((block-map (map (lambda (b) (cons (nf b 'index) b)) blocks)))
    (define (succs-of idx)
      (let ((entry (assv idx block-map)))
        (if entry (or (nf (cdr entry) 'succs) '()) '())))
    (let ((visited '()) (result '()))
      (define (dfs idx)
        (unless (memv idx visited)
          (set! visited (cons idx visited))
          (for-each dfs (succs-of idx))
          (set! result (cons idx result))))
      (dfs (nf (car blocks) 'index))
      result)))

;; ─── Top-level query ────────────────────────

(define (defuse-reachable? ssa-fn start-names found? fuel)
  (let* ((instrs (ssa-all-instrs ssa-fn))
         (universe (ssa-instruction-names ssa-fn))
         (names-lat (powerset-lattice universe))
         (guard-lat (boolean-lattice))
         (state-lat (product-lattice names-lat guard-lat))
         (transfer (make-reachability-transfer instrs found? names-lat))
         (initial (list start-names #f))
         (result (fixpoint state-lat transfer initial fuel)))
    (and result (cadr result))))

;; ─── Block-level analysis result accessors ─

(define (analysis-in result block-idx)
  (let ((entry (assv block-idx result)))
    (and entry (cadr entry))))

(define (analysis-out result block-idx)
  (let ((entry (assv block-idx result)))
    (and entry (caddr entry))))

(define (analysis-states result)
  result)

;; ─── Block-level worklist analysis ─────────

(define (run-analysis direction lattice transfer ssa-fn . args)
  ;; Parse optional args: [initial-state] ['check-monotone]
  (let* ((initial-state (if (and (pair? args) (not (symbol? (car args))))
                            (car args)
                            (lattice-bottom lattice)))
         (flags (if (and (pair? args) (not (symbol? (car args))))
                    (cdr args)
                    args))
         (check-mono (and (memq 'check-monotone flags) #t))
         (blocks (nf ssa-fn 'blocks))
         (forward? (eq? direction 'forward))
         (block-map (map (lambda (b) (cons (nf b 'index) b)) blocks))
         (block-ref (lambda (idx) (cdr (assv idx block-map))))
         (rpo (reverse-postorder blocks))
         (order (if forward? rpo (reverse rpo)))
         (rank-map (let loop ((os order) (r 0) (m '()))
                     (if (null? os) m
                         (loop (cdr os) (+ r 1)
                               (cons (cons (car os) r) m)))))
         (rank-of (lambda (idx)
                    (let ((e (assv idx rank-map)))
                      (if e (cdr e) 999))))
         (flow-preds (lambda (b)
                       (or (if forward? (nf b 'preds) (nf b 'succs)) '())))
         (flow-succs (lambda (b)
                       (or (if forward? (nf b 'succs) (nf b 'preds)) '())))
         (entry-idx (nf (car blocks) 'index))
         (exit-idxs (filter-map
                      (lambda (b)
                        (let ((s (nf b 'succs)))
                          (and (or (not s) (null? s)) (nf b 'index))))
                      blocks))
         (seed-idxs (if forward? (list entry-idx) exit-idxs))
         (bot (lattice-bottom lattice))
         (states (map (lambda (b)
                        (let ((idx (nf b 'index)))
                          (if (memv idx seed-idxs)
                              (if forward?
                                  (list idx initial-state bot)
                                  (list idx bot initial-state))
                              (list idx bot bot))))
                      blocks)))
    (define (get-in idx) (cadr (assv idx states)))
    (define (get-out idx) (caddr (assv idx states)))
    (define (set-state! idx in-val out-val)
      (set! states
        (map (lambda (entry)
               (if (= (car entry) idx)
                   (list idx in-val out-val)
                   entry))
             states)))
    (define (worklist-insert wl idx)
      (if (memv idx wl) wl
          (let insert ((rest wl))
            (cond ((null? rest) (list idx))
                  ((<= (rank-of idx) (rank-of (car rest)))
                   (cons idx rest))
                  (else (cons (car rest) (insert (cdr rest))))))))
    (define (worklist-insert-all wl idxs)
      (let loop ((is idxs) (w wl))
        (if (null? is) w
            (loop (cdr is) (worklist-insert w (car is))))))
    (let loop ((wl (worklist-insert-all '() seed-idxs)))
      (if (null? wl)
          states
          (let* ((idx (car wl))
                 (rest-wl (cdr wl))
                 (blk (block-ref idx))
                 (pred-idxs (flow-preds blk))
                 (in-val (if (null? pred-idxs)
                             (if (memv idx seed-idxs)
                                 (if forward? initial-state bot)
                                 bot)
                             (let join-preds ((ps pred-idxs)
                                              (acc (if (and (memv idx seed-idxs) forward?)
                                                      initial-state
                                                      bot)))
                               (if (null? ps) acc
                                   (join-preds (cdr ps)
                                     (lattice-join lattice acc
                                       (get-out (car ps))))))))
                 (out-val (transfer blk in-val))
                 (old-out (get-out idx)))
            (when (and check-mono
                       (not (lattice-leq? lattice old-out out-val)))
              (error (string-append "monotonicity violation at block "
                                    (number->string idx))))
            (set-state! idx in-val out-val)
            (if (lattice-leq? lattice out-val old-out)
                (loop rest-wl)
                (loop (worklist-insert-all rest-wl
                        (flow-succs blk)))))))))
