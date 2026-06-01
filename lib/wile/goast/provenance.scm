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

;;; Provenance: resolve SSA instructions to source positions.
;;;
;;; The SSA mapper injects a (pos . "file:line:col") field per instruction when
;;; go-ssa-build is called with the 'positions option (goastssa/mapper.go). The
;;; field is omitted for synthetic/positionless instructions. These accessors
;;; surface that position so analyses stop discarding the provenance they hold.

;; ssa-instr-pos: the resolved source position of an SSA instruction node, or
;; #f when the instruction carries no position. The string? guard normalizes
;; any non-string value (e.g. nf's #f on a missing field) to #f, giving callers
;; a clean #f / string? contract.
(define (ssa-instr-pos instr)
  (let ((p (nf instr 'pos)))
    (if (string? p) p #f)))

;; ssa-call-to?: does NODE call FUNC-NAME? Matches static calls (func field)
;; and method calls (method field) across ssa-call/ssa-go/ssa-defer.
;; NOTE: belief-checkers.scm inlines the equivalent predicate in
;; find-call-position and find-ssa-call-blocks. When adding a new call-like
;; tag, update all three sites (candidate for unification — see follow-up).
(define (ssa-call-to? node func-name)
  (and (or (tag? node 'ssa-call) (tag? node 'ssa-go) (tag? node 'ssa-defer))
       (or (equal? (nf node 'func) func-name)
           (equal? (nf node 'method) func-name))))

;; ssa-call-position: resolved source position of the first call to FUNC-NAME
;; in BLOCK's instruction list, or #f when the call is absent or positionless.
(define (ssa-call-position block func-name)
  (let loop ((is (or (nf block 'instrs) '())))
    (cond
      ((null? is) #f)
      ((ssa-call-to? (car is) func-name) (ssa-instr-pos (car is)))
      (else (loop (cdr is))))))

;; make-finding: construct an auditable finding — a value (category symbol or
;; measure) paired with its provenance: WHERE ("file:line:col" or #f when
;; unlocated), WHY (a structured reason (reason-tag . data-alist)), and SCORE
;; (a number, or #f when no natural confidence exists). A tagged alist, read
;; via the finding-* accessors. All four fields are always present, so a #f
;; accessor result means the field's value is #f (e.g. unlocated / no score).
(define (make-finding value where why score)
  (list 'finding
        (cons 'value value)
        (cons 'where where)
        (cons 'why   why)
        (cons 'score score)))

(define (finding-value f) (nf f 'value))
(define (finding-where f) (nf f 'where))
(define (finding-why   f) (nf f 'why))
(define (finding-score f) (nf f 'score))

;; val->string: display-form of any value, for rendering.
(define (val->string v)
  (let ((port (open-output-string)))
    (display v port)
    (get-output-string port)))

;; render-why: project a structured reason to a human string. A structured
;; reason is (reason-tag . data-alist), rendered "reason-tag (k=v, k=v ...)";
;; an empty data list renders just the tag. A bare symbol or string passes
;; through. The structure is what downstream Scheme filters on; this is only
;; the human projection.
(define (render-why why)
  (cond
    ((string? why) why)
    ((symbol? why) (symbol->string why))
    ((pair? why)
     (let ((tag  (car why))
           (data (cdr why)))
       (string-append
         (val->string tag)
         (if (pair? data)
             (string-append
               " ("
               (string-join
                 (map (lambda (kv)
                        (string-append (val->string (car kv)) "="
                                       (val->string (cdr kv))))
                      data)
                 ", ")
               ")")
             ""))))
    (else (val->string why))))

;; render-finding: a one-line audit string for a finding —
;; "where — why [score]". Unlocated findings show "<unlocated>"; a #f score
;; is omitted.
(define (render-finding f)
  (let ((where (or (finding-where f) "<unlocated>"))
        (why   (render-why (finding-why f)))
        (score (finding-score f)))
    (string-append where " — " why
      (if score (string-append " [" (val->string score) "]") ""))))
