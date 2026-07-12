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
;;
;; *** CAVEAT: VTA's soundness is NOT unconditional. *** golang.org/x/tools
;; go/callgraph/vta/vta.go:74-75: "CallGraph is then sound, MODULO USE OF
;; REFLECTION AND UNSAFE, if the initial call graph is sound." The `must` class
;; is built entirely on that soundness claim, so it inherits the exception. A
;; concrete type injected into an interface ONLY through reflection --
;; reflect.New(t).Elem().Interface().(I), the reflective-registry idiom used by
;; encoding/json, database/sql, protobuf, and apimachinery's runtime.Scheme --
;; never appears in an ssa.MakeInterface instruction, so VTA cannot see it flow
;; in. `must` CAN THEREFORE BE WRONG in a scope that uses reflect/unsafe: VTA
;; may report a singleton candidate set that omits the type actually invoked at
;; runtime. Do not gate anything on `must` in such a scope.
;;
;; Because this cannot be fixed by better computation (VTA's own doc names it
;; an inherent limit, not a bug), the finding instead DISCLOSES it: every `why`
;; carries `reflection-in-scope` (#t/#f). This is a DEFEATER PRESENCE flag, NOT
;; a proof that any particular site is wrong -- #t means a mechanism VTA cannot
;; see (reflect/unsafe) is reachable SOMEWHERE in the analyzed scope, so `must`
;; there needs independent verification before being trusted.

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

;; count-at: CHA's count for a VTA site-key, or #f on a KEY MISS. #f, never a
;; fabricated 0: VTA's candidate set is a subset of CHA's (VTA only prunes,
;; never invents), so "narrowed-from: 0, n: 5" would claim CHA found FEWER
;; candidates than VTA -- an impossible reading. A miss means "no CHA count is
;; available for this site", not "CHA counted zero". (max narrowed n) is NOT
;; used here on purpose: clamping to n would HIDE a genuine key-mismatch
;; between VTA's and CHA's site enumeration rather than surface it.
(define (count-at counts k)
  (let ((hit (assoc k counts))) (if hit (cdr hit) #f)))

;; class is a pure function of n. That is the whole rule.
(define (class-of n)
  (cond ((= n 0) 'none)
        ((= n 1) 'must)
        (else    'may)))

;; exported?: a Go type is exported iff the identifier after the last "." begins
;; with an uppercase letter. An exported interface can be implemented by a type
;; OUTSIDE the analyzed scope, so `must` on one is must-WITHIN-SCOPE.
(define (upper? c) (and (char>=? c #\A) (char<=? c #\Z)))

;; type-literal?: is S a Go TYPE LITERAL (e.g. "interface{Close() error}")
;; rather than a QUALIFIED TYPE NAME (e.g. "pkg.Type")? A literal has no
;; package to be "exported" FROM at all -- types.TypeString renders an
;; anonymous interface's method set inline, so "{" or "(" appearing anywhere
;; means there is no single trailing identifier to test. This is a STRUCTURAL
;; test on Go's own printed syntax, not a hardcoded name list.
(define (type-literal? s) (or (has-char? s #\{) (has-char? s #\()))

;; type-exported?: #t / #f for a qualified type name; 'unnamed for a TYPE
;; LITERAL (a structural/anonymous interface), where "exported" is not a
;; coherent question.
;;
;; The original version assumed every input was a qualified name and read
;; whatever capital letter it found scanning from the end. On a literal that
;; is WRONG both ways: `interface{Close() error}` -> #f (FALSE REASSURANCE --
;; any package anywhere can structurally satisfy it, so a `must` there is MORE
;; scope-limited than a named exported interface, not less); `interface{Write
;; (b *bytes.Buffer) error}` -> #t, correct only BY ACCIDENT, off the B in
;; bytes.Buffer INSIDE A METHOD SIGNATURE, not the interface's own name (it
;; has none). Returning 'unnamed instead of fabricating a boolean means a
;; structural interface can never silently read as "not exported / safely in
;; scope" -- the caller must handle the third state explicitly.
(define (type-exported? s)
  (cond
    ((or (not (string? s)) (= (string-length s) 0)) #f)
    ((type-literal? s) 'unnamed)
    (else
      ;; Walk DOWN from the end: the first '.' found is the LAST one, so the
      ;; char after it starts the type identifier. No dot => the whole string
      ;; is it.
      (let loop ((i (- (string-length s) 1)))
        (cond ((< i 0)                      (upper? (string-ref s 0)))
              ((char=? (string-ref s i) #\.)
               (and (< (+ i 1) (string-length s))
                    (upper? (string-ref s (+ i 1)))))
              (else (loop (- i 1))))))))

;; witness-index: SSA -> alist of (concrete-type . list of witness).
;; Each witness is a tagged node: (witness (func . f) (pos . p-or-#f) (iface . i)).
;;
;; The index is keyed on CONCRETE TYPE ALONE — it does not consult which interface
;; the site dispatches on. A witness list for concrete type T therefore MAY contain
;; conversions of T into a DIFFERENT interface than the site that looked it up. Each
;; witness is LABELLED with `iface`, the interface it was actually converted INTO
;; (ssa.MakeInterface's own `type`, not a new field this library computes), so the
;; consumer can tell which witness matches this site's interface and which does not.
;; Nothing is filtered out and nothing is fabricated: the witness stops claiming to
;; be "this site's conversion" and instead reports, truthfully, what it is.
;;
;; A strict (concrete, iface) string-equality FILTER was considered and REJECTED: it
;; deletes legitimate witnesses under interface embedding/widening. Example: interface
;; K embeds I; a value enters only via `return K(T{})`, so the MakeInterface records
;; type = K; the call site later narrows to I via ChangeInterface, so the site's iface
;; is I. Requiring type == iface demands K == I, matches nothing, and the only witness
;; for T vanishes — even though it is the correct, and only, explanation. Correct
;; filtering would require interface IMPLICATION (types.Implements), not string
;; equality; that is a new Go primitive and out of scope here.
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
                                                (cons 'pos (ssa-instr-pos i))
                                                (cons 'iface (nf i 'type))))
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

;; reflect-or-unsafe-node?: does N's function belong to package "reflect" or
;; "unsafe"? Prefer `pkg` (go/types-derived, exact) and fall back to a
;; substring test on the fully-qualified `name` for nodes whose `pkg` is unset
;; (some synthetic/external nodes carry no pkg field). Both signals come from
;; data the callgraph already computed -- neither is a hardcoded function list.
(define (reflect-or-unsafe-node? n)
  (let ((p  (nf n 'pkg))
        (nm (or (nf n 'name) "")))
    (or (equal? p "reflect") (equal? p "unsafe")
        (string-contains? nm "reflect.") (string-contains? nm "unsafe."))))

;; reflection-in-scope?: does the analyzed callgraph reach package reflect or
;; unsafe ANYWHERE? See the header comment on VTA soundness "modulo reflection
;; and unsafe". This is a DEFEATER PRESENCE flag, not a per-site proof: #t
;; means the mechanism that can hide a type from VTA is reachable somewhere in
;; scope, not that any specific finding is wrong. Computed once per
;; dispatch-sites call over the callgraph it already builds -- no new Go
;; primitive, no extra build.
(define (reflection-in-scope? cg)
  (let loop ((ns cg))
    (cond ((null? ns) #f)
          ((reflect-or-unsafe-node? (car ns)) #t)
          (else (loop (cdr ns))))))

;; make-dispatch-site: assemble ONE finding.
;;
;; `candidates` is ABSENT (not '()) when elided. An empty list would let a careless
;; consumer read "no candidates" off a 27-way site — the silent false negative,
;; reintroduced through the encoding. `n` is ALWAYS the true count, so the knob can
;; never make a site look smaller than it is.
;;
;; `refl` (reflection-in-scope) is a single value computed ONCE for the whole
;; dispatch-sites call and stamped onto every finding -- see reflection-in-scope?.
;;
;; `synthetic-caller` is #t when the SITE'S CALLER is a compiler-generated
;; forwarding function (ssa.Function.Synthetic != "", surfaced as cg-edge's
;; `caller-synthetic`). Such a site is a PHANTOM: its single invoke has no
;; source position because it does not exist as a call site in source at all
;; ($bound/$thunk closures, interface method-set wrappers, promoted-embedding
;; stubs). It is trivially `must` (one forwarding call, one target) and would
;; otherwise silently inflate a `must`-rate census with sites that are not
;; really there.
(define (make-dispatch-site key edges scope narrowed k idx refl)
  (let* ((n     (length edges))
         (e0    (car edges))
         (iface (nf e0 'iface))
         (where (or (nf e0 'pos) #f))
         (full? (<= n k))
         (base  (list (cons 'iface              iface)
                      (cons 'method             (nf e0 'method))
                      (cons 'caller             (car (split-key key)))
                      (cons 'n                  n)
                      (cons 'narrowed-from      narrowed)
                      (cons 'scope              scope)
                      (cons 'iface-exported     (type-exported? iface))
                      (cons 'reflection-in-scope refl)
                      (cons 'synthetic-caller   (and (nf e0 'caller-synthetic) #t))
                      (cons 'detail             (if full? 'full 'elided))))
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
;;
;; Sites are sorted by site-key before mapping. CHA/VTA build their graphs by
;; traversing Go maps internally (e.g. ssautil.AllFunctions), so raw discovery
;; order is NOT reproducible call to call even for byte-identical input — two
;; `dispatch-sites` calls that differ only in k rebuild the callgraph from
;; scratch and can enumerate the same sites in a different order. Without this
;; sort, list POSITION would not identify a site, silently breaking any
;; consumer (including TestDispatch_KInvariant) that compares sites across k
;; by position. The sort key (caller@pos) is derived from static program
;; structure, so it is itself deterministic regardless of map order.
(define (dispatch-sites pattern . rest)
  (let* ((k      (if (null? rest) default-dispatch-k (car rest)))
         (vta    (go-callgraph pattern 'vta))
         (cha    (go-callgraph pattern 'cha))
         (counts (counts-by-key cha))
         (idx    (witness-index pattern))
         (refl   (reflection-in-scope? vta))
         (sites  (sort (lambda (a b) (string<? (car a) (car b)))
                        (invoke-sites vta))))
    (map (lambda (p)
           (make-dispatch-site (car p) (cdr p) pattern
                               (count-at counts (car p)) k idx refl))
         sites)))

;; --- accessors --------------------------------------------------------------
;; dispatch-candidates returns #f when elided (the key is absent), NEVER '().
;; dispatch-narrowed-from returns #f on a CHA key miss ("no count available"),
;; NEVER a fabricated 0 -- see count-at.
;; dispatch-iface-exported returns #t / #f for a qualified type name, or
;; 'unnamed for a type literal (a structural/anonymous interface) -- see
;; type-exported?.

(define (dispatch-class f)              (finding-value f))
(define (dispatch-n f)                  (nf (finding-why f) 'n))
(define (dispatch-iface f)              (nf (finding-why f) 'iface))
(define (dispatch-method f)             (nf (finding-why f) 'method))
(define (dispatch-narrowed-from f)      (nf (finding-why f) 'narrowed-from))
(define (dispatch-detail f)             (nf (finding-why f) 'detail))
(define (dispatch-candidates f)         (nf (finding-why f) 'candidates))
(define (dispatch-iface-exported f)     (nf (finding-why f) 'iface-exported))
(define (dispatch-reflection-in-scope f) (nf (finding-why f) 'reflection-in-scope))
(define (dispatch-synthetic-caller f)   (nf (finding-why f) 'synthetic-caller))
