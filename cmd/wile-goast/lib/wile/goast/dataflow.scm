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
