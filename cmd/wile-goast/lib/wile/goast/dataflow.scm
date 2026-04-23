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

;;; (wile goast dataflow) — Def-use reachability via (wile algebra)
;;;
;;; Provides lattice-based bounded reachability on SSA def-use graphs.
;;; Foundation for dataflow analysis (C2).

;; ─── Boolean lattice {#f, #t} ───────────────

(define (boolean-lattice)
  "Construct a boolean lattice: bottom=#f, join=or, equal?=eq?.\nReturns an alist suitable for run-analysis.\n\nReturns: list\nCategory: goast-dataflow\n\nSee also: `run-analysis'."
  (make-lattice
    (lambda (a b) (or a b))         ; join
    (lambda (a b) (and a b))        ; meet
    #f                               ; bottom
    #t                               ; top
    (lambda (a b) (or (not a) b))))  ; leq? (implication)

;; ─── SSA instruction extraction ─────────────

(define (ssa-all-instrs ssa-fn)
  "Flatten all instructions from all blocks of an SSA function.\n\nParameters:\n  ssa-fn : list\nReturns: list\nCategory: goast-dataflow\n\nSee also: `ssa-instruction-names', `block-instrs'."
  (let ((blocks (nf ssa-fn 'blocks)))
    (if (pair? blocks)
      (flat-map
        (lambda (b)
          (let ((is (nf b 'instrs)))
            (if (pair? is) is '())))
        blocks)
      '())))

(define (ssa-instruction-names ssa-fn)
  "Extract all named values (registers) from an SSA function.\n\nParameters:\n  ssa-fn : list\nReturns: list\nCategory: goast-dataflow\n\nSee also: `ssa-all-instrs'."
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
  "Build a transfer function for def-use reachability analysis.\nALL-INSTRS is the flat instruction list. FOUND? is a predicate\nthat recognizes the target instruction. NAMES-LAT is a powerset\nlattice over instruction names.\n\nParameters:\n  all-instrs : list\n  found? : procedure\n  names-lat : list\nReturns: procedure\nCategory: goast-dataflow\n\nSee also: `defuse-reachable?'."
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
  "Extract the instruction list from an SSA block.\n\nParameters:\n  block : list\nReturns: list\nCategory: goast-dataflow\n\nSee also: `ssa-all-instrs'."
  (or (nf block 'instrs) '()))

;; ─── CFG protocol adapter for SSA functions ─
;;
;; Bridges an SSA function (as produced by goast's Go-SSA parsing) to
;; the generic `<cfg-protocol>` record in (wile algebra dataflow). Callers
;; of `run-analysis` pass `(ssa-cfg-protocol)` as the protocol argument.

(define (ssa-cfg-protocol)
  "Build a CFG protocol adapter for SSA-function shapes.
Blocks are accessed via `(nf fn 'blocks)`; each block exposes
`'index`, `'preds`, `'succs` via `nf`. `preds` and `succs` may be
`#f` for blocks with no predecessors/successors; the protocol
closures return `'()` in that case to match the generic solver's
`#f`-tolerance convention.

Returns: cfg-protocol
Category: goast-dataflow

See also: `run-analysis'."
  (make-cfg-protocol
    (lambda (fn)  (or (nf fn 'blocks) '()))
    (lambda (b)   (nf b 'index))
    (lambda (b)   (or (nf b 'preds) '()))
    (lambda (b)   (or (nf b 'succs) '()))))

;; ─── Top-level query ────────────────────────

(define (defuse-reachable? ssa-fn start-names found? fuel)
  "Test whether any START-NAMES value reaches an instruction matching FOUND?\nvia def-use chains within FUEL iterations. Uses product lattice fixpoint.\n\nParameters:\n  ssa-fn : list\n  start-names : list\n  found? : procedure\n  fuel : integer\nReturns: boolean\nCategory: goast-dataflow\n\nExamples:\n  (defuse-reachable? fn '(\"t0\") (lambda (i) (tag? i 'ssa-if)) 5)\n\nSee also: `run-analysis', `make-reachability-transfer'."
  (let* ((instrs (ssa-all-instrs ssa-fn))
         (universe (ssa-instruction-names ssa-fn))
         (names-lat (powerset-lattice universe))
         (guard-lat (boolean-lattice))
         (state-lat (product-lattice names-lat guard-lat))
         (transfer (make-reachability-transfer instrs found? names-lat))
         (initial (list start-names #f))
         (result (fixpoint state-lat transfer initial fuel)))
    (and result (cadr result))))

