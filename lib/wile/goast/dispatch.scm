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

;; (wile goast dispatch) — interface dispatch as located, justified findings.
;;
;; A dispatch site IS a (wile goast provenance) finding:
;;   value = class   (none | must | may)
;;   where = the call site "file:line:col"
;;   why   = (dispatch (iface . ...) (method . ...) (n . k) ...)
;;   score = #f      -- no natural confidence exists; do NOT fabricate one
;;
;; class is a PURE FUNCTION OF n. No judgment enters, so the tool issues no
;; verdict it cannot support:
;;   n = 0  -> none   no concrete type flows here WITHIN SCOPE
;;   n = 1  -> must   VTA's SOUND set is a singleton, so the true callee set is a
;;                    subset of it: if the call executes, it calls that function
;;   n > 1  -> may    one of these n
;;
;; "Genuine polymorphism" is NOT decidable and is never claimed: given 27
;; candidates the tool cannot know whether the site is truly 27-way or whether
;; VTA merely failed to narrow. See the design doc for the measurement.

(define default-dispatch-k 8)

;; An invoke (interface-dispatch) edge is one carrying `iface`. This is a FIELD
;; TEST, not a match on the `description` string — the string heuristic has a
;; known blind spot and this replaces it.
(define (invoke-edge? e) (and (nf e 'iface) #t))

;; A call site is (caller, position). Position alone is not a key: a position can
;; be shared across wrapper/thunk functions.
(define (site-key caller e)
  (string-append caller "@" (or (nf e 'pos) "?")))

;; invoke-sites: cg -> alist of (site-key . (caller iface method pos . edges))
(define (invoke-sites cg)
  (let nloop ((ns cg) (acc '()))
    (if (null? ns)
        acc
        (let ((caller (nf (car ns) 'name)))
          (let eloop ((es (or (nf (car ns) 'edges-out) '())) (a acc))
            (if (null? es)
                (nloop (cdr ns) a)
                (let ((e (car es)))
                  (if (not (invoke-edge? e))
                      (eloop (cdr es) a)
                      (let* ((k (site-key caller e))
                             (hit (assoc k a)))
                        (if hit
                            (begin (set-cdr! hit (cons e (cdr hit)))
                                   (eloop (cdr es) a))
                            (eloop (cdr es)
                                   (cons (cons k (list e)) a))))))))))))

;; counts-by-key: cg -> alist of (site-key . count). Used for narrowed-from.
(define (counts-by-key cg)
  (map (lambda (p) (cons (car p) (length (cdr p)))) (invoke-sites cg)))

(define (count-at counts k)
  (let ((hit (assoc k counts))) (if hit (cdr hit) 0)))

;; class is a pure function of n. That is the whole rule.
(define (class-of n)
  (cond ((= n 0) 'none)
        ((= n 1) 'must)
        (else    'may)))

;; exported?: a Go type is exported iff the identifier after the last "." begins
;; with an uppercase letter. An exported interface can be implemented by a type
;; OUTSIDE the analyzed scope, so `must` on one is must-WITHIN-SCOPE.
(define (upper? c) (and (char>=? c #\A) (char<=? c #\Z)))

(define (type-exported? s)
  (if (or (not (string? s)) (= (string-length s) 0))
      #f
      ;; Walk DOWN from the end: the first '.' found is the LAST one, so the char
      ;; after it starts the type identifier. No dot => the whole string is it.
      (let loop ((i (- (string-length s) 1)))
        (cond ((< i 0)                      (upper? (string-ref s 0)))
              ((char=? (string-ref s i) #\.)
               (and (< (+ i 1) (string-length s))
                    (upper? (string-ref s (+ i 1)))))
              (else (loop (- i 1)))))))

;; witness-index: SSA -> alist of (concrete-type . list of witness).
;; Each witness is a tagged node: (witness (func . f) (pos . p-or-#f)).
;;
;; The witness answers "where did this concrete type ENTER this interface?", not
;; "how did it reach this site" — VTA's type-flow graph is not exported by x/tools,
;; so the stronger claim would be fabricated.
;;
;; POSITIONS ARE OFTEN ABSENT. ssa.MakeInterface carries a valid Pos() only for an
;; EXPLICIT conversion (I(T{})). The three implicit forms — var decl, call arg,
;; assignment — yield NoPos, and they are nearly all real Go. So `func` is the
;; always-available part of the witness and `pos` may be #f. An absent position is
;; reported as absent: degrade to a MISSING witness, never a WRONG one.
;;
;; 'positions is REQUIRED here (a variadic SYMBOL rest-arg, not a list).
(define (witness-index pattern)
  (let ((fns (go-ssa-build pattern 'positions)))
    (let floop ((fs fns) (acc '()))
      (if (null? fs)
          acc
          (let ((fname (nf (car fs) 'name)))
            (let bloop ((bs (or (nf (car fs) 'blocks) '())) (a acc))
              (if (null? bs)
                  (floop (cdr fs) a)
                  (let iloop ((is (or (nf (car bs) 'instrs) '())) (a2 a))
                    (if (null? is)
                        (bloop (cdr bs) a2)
                        (let ((i (car is)))
                          (if (not (tag? i 'ssa-make-interface))
                              (iloop (cdr is) a2)
                              (let* ((ct  (nf i 'concrete))
                                     ;; Tagged like every other node in this codebase
                                     ;; (candidate, dispatch, ...) so `nf` — which
                                     ;; discards the node's car as the tag — can find
                                     ;; 'func and 'pos. An untagged alist here would
                                     ;; make (nf w 'func) always #f.
                                     (w   (list 'witness
                                                (cons 'func fname)
                                                (cons 'pos (ssa-instr-pos i))))
                                     (hit (assoc ct a2)))
                                (if hit
                                    (begin (set-cdr! hit (cons w (cdr hit)))
                                           (iloop (cdr is) a2))
                                    (iloop (cdr is)
                                           (cons (cons ct (list w)) a2)))))))))))))))

(define (witnesses-for idx concrete)
  (let ((hit (assoc concrete idx)))
    (if hit (cdr hit) '())))

(define (edge->candidate idx e)
  (let ((recv (nf e 'recv)))
    (list 'candidate
          (cons 'callee   (nf e 'callee))
          (cons 'concrete recv)
          ;; '() is honest here: the conversion is real, but no MakeInterface for
          ;; this type was found in scope (generics, synthetic SSA, external pkg).
          (cons 'witness  (witnesses-for idx recv)))))

;; make-dispatch-site: assemble ONE finding.
;;
;; `candidates` is ABSENT (not '()) when elided. An empty list would let a careless
;; consumer read "no candidates" off a 27-way site — the silent false negative,
;; reintroduced through the encoding. `n` is ALWAYS the true count, so the knob can
;; never make a site look smaller than it is.
(define (make-dispatch-site key edges scope narrowed k idx)
  (let* ((n     (length edges))
         (e0    (car edges))
         (iface (nf e0 'iface))
         (where (or (nf e0 'pos) #f))
         (full? (<= n k))
         (base  (list (cons 'iface          iface)
                      (cons 'method         (nf e0 'method))
                      (cons 'caller         (car (split-key key)))
                      (cons 'n              n)
                      (cons 'narrowed-from  narrowed)
                      (cons 'scope          scope)
                      (cons 'iface-exported (type-exported? iface))
                      (cons 'detail         (if full? 'full 'elided))))
         (why   (cons 'dispatch
                      (if full?
                          (append base
                                  (list (cons 'candidates
                                              (map (lambda (e) (edge->candidate idx e))
                                                   edges))))
                          base))))
    (make-finding (class-of n) where why #f)))

(define (split-key k)
  (let loop ((i 0))
    (cond ((>= i (string-length k)) (list k ""))
          ((char=? (string-ref k i) #\@)
           (list (substring k 0 i) (substring k (+ i 1) (string-length k))))
          (else (loop (+ i 1))))))

;; dispatch-sites: the entry point. K controls DETAIL, never SITES — every site is
;; always returned.
(define (dispatch-sites pattern . rest)
  (let* ((k      (if (null? rest) default-dispatch-k (car rest)))
         (vta    (go-callgraph pattern 'vta))
         (cha    (go-callgraph pattern 'cha))
         (counts (counts-by-key cha))
         (idx    (witness-index pattern)))
    (map (lambda (p)
           (make-dispatch-site (car p) (cdr p) pattern
                               (count-at counts (car p)) k idx))
         (invoke-sites vta))))

;; --- accessors --------------------------------------------------------------
;; dispatch-candidates returns #f when elided (the key is absent), NEVER '().

(define (dispatch-class f)         (finding-value f))
(define (dispatch-n f)             (nf (finding-why f) 'n))
(define (dispatch-iface f)         (nf (finding-why f) 'iface))
(define (dispatch-method f)        (nf (finding-why f) 'method))
(define (dispatch-narrowed-from f) (nf (finding-why f) 'narrowed-from))
(define (dispatch-detail f)        (nf (finding-why f) 'detail))
(define (dispatch-candidates f)    (nf (finding-why f) 'candidates))
