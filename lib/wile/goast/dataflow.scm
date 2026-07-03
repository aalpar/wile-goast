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

;; The truth-value guard lattice now comes from (wile algebra) as
;; `two-point-lattice' — the {#f,#t} lattice is pure algebra and lives in
;; wile, not here (see `defuse-reachable?').

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
         (guard-lat (two-point-lattice))
         (state-lat (product-lattice names-lat guard-lat))
         (transfer (make-reachability-transfer instrs found? names-lat))
         (initial (list start-names #f))
         (result (fixpoint state-lat transfer initial fuel)))
    (and result (cadr result))))

;; ─── Aggregate-aware value-flow reachability ─────────
;;
;; `defuse-reachable?` under-approximates flow THROUGH an aggregate. A value
;; stored into arr[i] — a store whose address is `index-addr(arr, i)` — and
;; later read via `slice(arr)` (or `arr[j]`) is invisible to a plain def-use
;; walk: the store taints the element ADDRESS (`&arr[i]`), while the slice/load
;; reads the backing ARRAY (`arr`), and there is no def-use edge between them.
;;
;; This bites the variadic-call idiom: `f(x)` where `f(...T)` packs `x` into a
;; `[]T` backing array and passes a slice, which is exactly how a captured
;; continuation reaches a callback in PrimCallCC's two mode arms
;; (`mc.ApplyCallable(mcls, capt)` — a variadic `...Value`; wile finding #9,
;; belief B2). A plain `defuse-reachable?` from `capt` to `ApplyCallable` returns
;; #f there, a real false negative.
;;
;; `build-addr-aggregate-map` records, for each element/field address
;; instruction (`index-addr` / `field-addr`), the backing aggregate it addresses:
;; addr-reg -> aggregate-reg. `value-flow-reached` adds the aggregate-alias edge
;; — a store of a tainted value through such an address taints the aggregate — so
;; a subsequent `slice(arr)` carries the value forward under the ordinary operand
;; rule. Adding edges is monotone: it can only make more values reach, never
;; fewer, so it cannot weaken any existing reachability verdict.

(define (build-addr-aggregate-map instrs)
  "Map each element/field address register to the aggregate it addresses.\nScans INSTRS for index-addr / field-addr instructions and returns an alist\n(addr-register . aggregate-register). Used by value-flow-reached to add the\naggregate-alias edge (a store through an element address taints the backing\naggregate).\n\nParameters:\n  instrs : list\nReturns: list\nCategory: goast-dataflow\n\nSee also: `value-flow-reached'."
  (filter-map
    (lambda (i)
      (and (or (tag? i 'ssa-index-addr) (tag? i 'ssa-field-addr))
           (let ((nm (nf i 'name)) (agg (nf i 'x)))
             (and nm agg (cons nm agg)))))
    instrs))

(define (value-flow-reached ssa-fn seed-names)
  "Return the set of SSA value names reachable from SEED-NAMES via def-use,\nincluding flow through aggregates: a tainted value stored through an\nelement/field address taints the backing aggregate, so a later slice/load of\nthat aggregate carries the value forward. This sees value-through-variadic-slice\nflow that `defuse-reachable?` misses. The powerset lattice is monotone and\nbounded by the value universe, so the fixpoint always converges (fuel =\n|universe| + 2).\n\nParameters:\n  ssa-fn : list\n  seed-names : list\nReturns: list\nCategory: goast-dataflow\n\nExamples:\n  (value-flow-reached fn '(\"t0\"))\n\nSee also: `defuse-reachable?', `build-addr-aggregate-map'."
  (let* ((instrs (ssa-all-instrs ssa-fn))
         (universe (ssa-instruction-names ssa-fn))
         (addr-map (build-addr-aggregate-map instrs))
         (L (powerset-lattice universe))
         (fuel (+ (length universe) 2))
         (transfer
           (lambda (names)
             (let* ((reached
                      (filter
                        (lambda (i)
                          (let ((ops (or (nf i 'operands) '())))
                            (let check ((ns names))
                              (cond ((null? ns) #f)
                                    ((member? (car ns) ops) #t)
                                    (else (check (cdr ns)))))))
                        instrs))
                    (new
                      (flat-map
                        (lambda (i)
                          (let ((nm (nf i 'name)))
                            (append
                              (if (and nm (not (member? nm names))) (list nm) '())
                              ;; aggregate-alias edge: a store whose VALUE is
                              ;; tainted taints the aggregate its address backs.
                              (if (and (tag? i 'ssa-store)
                                       (let ((v (nf i 'val)))
                                         (and v (member? v names))))
                                (let* ((p (assoc (nf i 'addr) addr-map))
                                       (agg (and p (cdr p))))
                                  (if (and agg (not (member? agg names)))
                                    (list agg) '()))
                                '()))))
                        reached)))
               (lattice-join L names new))))
         (result (fixpoint L transfer seed-names fuel)))
    (or result '())))

