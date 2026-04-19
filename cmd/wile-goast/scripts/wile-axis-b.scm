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

;;; wile-axis-b.scm — Phase 3.B analyzer script.
;;;
;;; Consumes wile/plans/axis-b-manifest.scm + go-ssa-narrow to produce:
;;;   - plans/axis-b-raw.scm                 (structured per-primitive data)
;;;   - plans/2026-04-19-axis-b-inventory.md (bucketed markdown inventory)
;;;   - stdout summary                       (bucket counts + reason tally)
;;;
;;; Invocation:
;;;   wile-goast --run wile-axis-b
;;;
;;; Env overrides:
;;;   WILE_AXIS_B_MANIFEST   — manifest path (default: ../wile/plans/axis-b-manifest.scm)
;;;   WILE_AXIS_B_RAW_OUTPUT — raw output path (default: ../wile/plans/axis-b-raw.scm)
;;;   WILE_AXIS_B_INVENTORY  — inventory path  (default: ../wile/plans/2026-04-19-axis-b-inventory.md)
;;;
;;; See plans/2026-04-19-pr-3-axis-b-script-impl.md for the implementation plan.
;;; See plans/2026-04-19-axis-b-analyzer-impl-design.md §7 for the overall design.

(import (wile goast)
        (wile goast ssa)
        (wile goast utils))

;; ---------------------------------------------------------------------------
;; Path resolution
;; ---------------------------------------------------------------------------

;; env-or returns the env var value if set and non-empty, else the default.
(define (env-or name default)
  (let ((v (get-environment-variable name)))
    (if (and v (not (string=? v "")))
        v
        default)))

(define default-manifest-path   "../wile/plans/axis-b-manifest.scm")
(define default-raw-output-path "../wile/plans/axis-b-raw.scm")
(define default-inventory-path  "../wile/plans/2026-04-19-axis-b-inventory.md")

(define (manifest-path)   (env-or "WILE_AXIS_B_MANIFEST"   default-manifest-path))
(define (raw-output-path) (env-or "WILE_AXIS_B_RAW_OUTPUT" default-raw-output-path))
(define (inventory-path)  (env-or "WILE_AXIS_B_INVENTORY"  default-inventory-path))

;; ---------------------------------------------------------------------------
;; Manifest parsing
;; ---------------------------------------------------------------------------

;; read-manifest reads the manifest S-expression list. Each entry is:
;;   ("name" "declared-return-type" "go-function-name" "source-file:line")
;; The manifest file contains exactly one list; we read it directly.
(define (read-manifest path)
  (let* ((port (open-input-file path))
         (data (read port)))
    (close-input-port port)
    (if (list? data)
        data
        (error "manifest is not a list" path))))

;; manifest-entry accessors.
(define (entry-name e)            (list-ref e 0))
(define (entry-declared-return e) (list-ref e 1))
(define (entry-go-function e)     (list-ref e 2))
(define (entry-go-source e)       (list-ref e 3))

;; ---------------------------------------------------------------------------
;; Sink-call enumeration
;; ---------------------------------------------------------------------------

;; string-index finds the first index of ch in s, or #f. Local helper —
;; defined before use because EvalMultiple compiles each top-level define
;; independently and forward references fail.
(define (string-index s ch)
  (let ((n (string-length s)))
    (let loop ((i 0))
      (cond
        ((= i n) #f)
        ((char=? (string-ref s i) ch) i)
        (else (loop (+ i 1)))))))

;; strip-type-annotation turns "0:int" into "0", "t3" into "t3". Constant
;; operands are rendered by the mapper with a "<value>:<type>" suffix.
(define (strip-type-annotation s)
  (let ((idx (string-index s #\:)))
    (if idx (substring s 0 idx) s)))

;; Result-writing sink methods on wile's CallContext / *MachineContext. The
;; mapper exposes these as ssa-call instructions whose 'method field (invoke
;; mode) or 'func field (static call to a method-expression) equals one of
;; these strings. Research in plans/2026-04-19-pr-3-axis-b-script-impl.md.
(define sink-method-names '("SetValue" "SetValues"))

;; sink-call? returns #t if instr is an ssa-call to one of the sink methods.
;; Matches either invoke-mode (method field) or static-call-mode (func field).
(define (sink-call? instr)
  (and (tag? instr 'ssa-call)
       (let ((mode (nf instr 'mode)))
         (cond
           ((eq? mode 'invoke)
            (and (member (nf instr 'method) sink-method-names) #t))
           ((eq? mode 'call)
            (and (member (nf instr 'func) sink-method-names) #t))
           (else #f)))))

;; extract-arg-name returns the name of the first positional argument passed
;; to a sink call. The mapper renders args as a list of bare strings
;; ("t3", "0:int") or the literal value — we extract the string part before ':'.
;; Returns #f if the args list is empty.
(define (extract-arg-name instr)
  (let ((args (nf instr 'args)))
    (if (or (not args) (null? args))
        #f
        (let ((first (car args)))
          (cond
            ((string? first) (strip-type-annotation first))
            ((symbol? first) (symbol->string first))
            (else #f))))))

;; find-sink-calls walks an (ssa-func ...) alist and returns a list of
;; (sink-call-info (method <name>) (value-arg <name-string>)) tuples —
;; one per reachable call to SetValue / SetValues.
(define (find-sink-calls ssa-func)
  (let loop ((bs (nf ssa-func 'blocks)) (acc '()))
    (cond
      ((or (not bs) (null? bs)) (reverse acc))
      (else
       (let instr-loop ((is (nf (car bs) 'instrs)) (acc2 acc))
         (cond
           ((or (not is) (null? is))
            (loop (cdr bs) acc2))
           ((sink-call? (car is))
            (let ((arg (extract-arg-name (car is)))
                  (m   (or (nf (car is) 'method) (nf (car is) 'func))))
              (instr-loop (cdr is)
                          (cons (list 'sink-call-info
                                      (cons 'method m)
                                      (cons 'value-arg arg))
                                acc2))))
           (else (instr-loop (cdr is) acc2))))))))

;; ---------------------------------------------------------------------------
;; Function resolution from manifest entry to SSA function
;; ---------------------------------------------------------------------------

;; build-func-index returns an alist mapping (fn-name-string . ssa-func-alist).
;; Linear construction, then O(log N) lookups via assoc on the hot path.
(define (build-func-index ssa-funcs)
  (map (lambda (f) (cons (nf f 'name) f)) ssa-funcs))

;; resolve-function looks up a manifest entry in a func-index. Returns the
;; matching (ssa-func ...) alist, or #f for binding-only primitives (Impl is
;; nil — about 47 in the current manifest) or for primitives whose Go
;; function lives in an SSA-package not loaded in this run.
(define (resolve-function entry func-index)
  (let* ((go-fn (entry-go-function entry))
         (hit (assoc go-fn func-index)))
    (if hit (cdr hit) #f)))

;; ---------------------------------------------------------------------------
;; Per-primitive narrowing pipeline
;; ---------------------------------------------------------------------------

;; string-member? — membership by string=? (not eq?).
(define (string-member? s lst)
  (cond
    ((null? lst) #f)
    ((string=? s (car lst)) #t)
    (else (string-member? s (cdr lst)))))

;; union-strings deduplicates a list of strings (preserves first-seen order).
(define (union-strings xs)
  (let loop ((xs xs) (acc '()))
    (cond
      ((null? xs) (reverse acc))
      ((string-member? (car xs) acc) (loop (cdr xs) acc))
      (else (loop (cdr xs) (cons (car xs) acc))))))

;; union-symbols deduplicates a list of symbols.
(define (union-symbols xs)
  (let loop ((xs xs) (acc '()))
    (cond
      ((null? xs) (reverse acc))
      ((memq (car xs) acc) (loop (cdr xs) acc))
      (else (loop (cdr xs) (cons (car xs) acc))))))

;; merge-narrow-results — Scheme-side mirror of Go-side mergeResults. Unions
;; types + reasons across all inputs; confidence picks 'widened if any is
;; widened, else 'no-paths if all are no-paths, else 'narrow.
(define (merge-narrow-results rs)
  (if (null? rs)
      (list 'narrow-result
            (cons 'types '())
            (cons 'confidence 'no-paths)
            (cons 'reasons '()))
      (let loop ((rs rs)
                 (types '())
                 (reasons '())
                 (any-widened #f)
                 (all-no-paths #t))
        (cond
          ((null? rs)
           (let ((conf (cond (any-widened 'widened)
                             (all-no-paths 'no-paths)
                             (else 'narrow))))
             (list 'narrow-result
                   (cons 'types (union-strings (reverse types)))
                   (cons 'confidence conf)
                   (cons 'reasons (union-symbols (reverse reasons))))))
          (else
           (let* ((r (car rs))
                  (r-types (or (nf r 'types) '()))
                  (r-reasons (or (nf r 'reasons) '()))
                  (r-conf (nf r 'confidence))
                  (now-any-widened (or any-widened (eq? r-conf 'widened)))
                  (now-all-no-paths (and all-no-paths
                                         (eq? r-conf 'no-paths))))
             (loop (cdr rs)
                   (append (reverse r-types) types)
                   (append (reverse r-reasons) reasons)
                   now-any-widened
                   now-all-no-paths)))))))

;; narrow-sink-call invokes go-ssa-narrow on the value-arg of a sink call.
;; Returns the narrow-result alist.
(define (narrow-sink-call ssa-func sink-info)
  (let ((arg-name (nf sink-info 'value-arg)))
    (if arg-name
        (go-ssa-narrow ssa-func arg-name)
        (list 'narrow-result
              (cons 'types '())
              (cons 'confidence 'no-paths)
              (cons 'reasons '(missing-arg))))))

;; analyze-primitive orchestrates resolution → sink enumeration → narrowing →
;; merge, returning an (axis-b-entry ...) alist. For unresolved entries
;; (binding-only, or out-of-package), emits an axis-b-entry with
;; (status unresolved).
(define (analyze-primitive entry func-index)
  (let ((fn (resolve-function entry func-index)))
    (if (not fn)
        (list 'axis-b-entry
              (cons 'name (entry-name entry))
              (cons 'declared-return-type (entry-declared-return entry))
              (cons 'go-function (entry-go-function entry))
              (cons 'go-source (entry-go-source entry))
              (cons 'status 'unresolved)
              (cons 'narrowed '())
              (cons 'confidence 'no-paths)
              (cons 'reasons '()))
        (let* ((sinks (find-sink-calls fn))
               (narrows (map (lambda (s) (narrow-sink-call fn s)) sinks))
               (merged (merge-narrow-results narrows)))
          (list 'axis-b-entry
                (cons 'name (entry-name entry))
                (cons 'declared-return-type (entry-declared-return entry))
                (cons 'go-function (entry-go-function entry))
                (cons 'go-source (entry-go-source entry))
                (cons 'status 'resolved)
                (cons 'sink-count (length sinks))
                (cons 'narrowed (nf merged 'types))
                (cons 'confidence (nf merged 'confidence))
                (cons 'reasons (nf merged 'reasons)))))))

;; ---------------------------------------------------------------------------
;; Main entry
;; ---------------------------------------------------------------------------

(define (main)
  (let* ((mpath (manifest-path))
         (entries (read-manifest mpath)))
    (display "wile-axis-b: loaded ")
    (display (length entries))
    (display " primitives from ")
    (display mpath)
    (newline)
    ;; Sanity probe: build SSA for wile's registry/core, build the func
    ;; index, and analyze a few known primitives to exercise the pipeline.
    (parameterize ((current-go-target "github.com/aalpar/wile/registry/core"))
      (let* ((funcs (go-ssa-build))
             (idx (build-func-index funcs))
             (probes '("cons" "null?" "length" "car")))
        (display "  pipeline probe (core primitives):") (newline)
        (for-each
         (lambda (name)
           (let ((e (let loop ((es entries))
                      (cond ((null? es) #f)
                            ((string=? (entry-name (car es)) name) (car es))
                            (else (loop (cdr es)))))))
             (if e
                 (let ((r (analyze-primitive e idx)))
                   (display "    ") (display name)
                   (display ": conf=") (display (nf r 'confidence))
                   (display " types=") (display (nf r 'narrowed))
                   (display " reasons=") (display (nf r 'reasons))
                   (newline))
                 (begin (display "    ") (display name)
                        (display ": (not in manifest)") (newline)))))
         probes)))))

(main)
