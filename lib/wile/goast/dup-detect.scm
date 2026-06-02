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

;;; dup-detect.scm — deduplication FCA audit trace.
;;;
;;; The mirror of fca.scm's boundary-findings on a function×external-ref concept
;;; lattice. Functions sharing a maximal informative reference set (an FCA
;;; concept with extent >= 2) are duplicate candidates; each extent member is a
;;; located finding whose why is the shared ref intent. Composes the split.scm
;;; clustering chain (objects are function names) with fca + provenance.

;; func-refs->positions: name->position hashtable from go-func-refs output (now
;; carrying 'pos). The field-index->positions twin; keys are func-ref names,
;; identical to the FCA context objects, so the join is exact-match. Functions
;; without a 'pos (synthetic/positionless) are skipped — unlocated when looked up.
(define (func-refs->positions func-refs)
  (let ((h (make-hashtable)))
    (for-each
      (lambda (fr)
        (let ((name (nf fr 'name))
              (pos  (nf fr 'pos)))
          (if (and (string? name) (string? pos))
            (hashtable-set! h name pos))))
      (if (pair? func-refs) func-refs '()))
    h))

;; function-ref-context: function×external-ref FCA context, IDF-filtered. Reuses
;; the split.scm chain verbatim — the same machinery split applies at package
;; granularity, here for dedup clustering. Objects = function names; attributes =
;; informative external package paths. THRESHOLD defaults to 0.36 (split's).
(define (function-ref-context func-refs . opts)
  (let* ((threshold (if (pair? opts) (car opts) 0.36))
         (sigs      (import-signatures func-refs))
         (idf       (compute-idf sigs))
         (filtered  (filter-noise sigs idf threshold)))
    (build-package-context filtered)))

;; duplicate-candidate-concepts: concepts whose extent has >= MIN-EXTENT (default
;; 2) functions sharing a non-empty intent. By FCA closure, such a concept is a
;; duplicate-candidate cluster: every function in the extent uses every ref in
;; the intent, and the intent is the maximal shared informative ref-set.
(define (duplicate-candidate-concepts lattice . opts)
  (let ((min-ext (if (pair? opts) (car opts) 2)))
    (filter (lambda (c)
              (and (>= (length (concept-extent c)) min-ext)
                   (>= (length (concept-intent c)) 1)))
            lattice)))

;; dup-candidate-findings: the boundary-findings twin for deduplication. POS-INDEX
;; is from func-refs->positions. Each entry mirrors a boundary-findings entry:
;; per candidate concept, each extent member -> a located finding. value = the
;; function name; where = its source position (or #f when unlocated); why = the
;; shared ref intent as a structured reason (duplicate-candidate (refs . intent))
;; so render-why projects it and a script can filter on the shared packages;
;; score = #f (no structural-confidence measure yet — that is slice 5b).
(define (dup-candidate-findings concepts pos-index)
  (map (lambda (concept)
         (let* ((ext (concept-extent concept))
                (int (concept-intent concept))
                (why (cons 'duplicate-candidate (list (cons 'refs int))))
                (findings
                  (map (lambda (fn)
                         (make-finding fn (hashtable-ref pos-index fn #f) why #f))
                       ext)))
           (list (cons 'refs int)
                 (cons 'findings findings)
                 (cons 'extent-size (length ext)))))
       concepts))

;; find-duplicate-candidates: top-level. TARGET is a package pattern string or a
;; GoSession. Runs the full chain — func-refs -> IDF-filtered context -> concept
;; lattice -> candidate concepts -> located findings. Returns a list of entries
;; (one per candidate cluster), each ((refs . intent) (findings . (...))
;; (extent-size . N)). Optional THRESHOLD (default 0.36) tunes IDF noise removal.
(define (find-duplicate-candidates target . opts)
  (let* ((threshold (if (pair? opts) (car opts) 0.36))
         (refs      (go-func-refs target))
         (ctx       (function-ref-context refs threshold))
         (lat       (concept-lattice ctx))
         (cands     (duplicate-candidate-concepts lat))
         (pos-index (func-refs->positions refs)))
    (dup-candidate-findings cands pos-index)))

;;; ── Slice 5b: structural measure surface ──────────────────

;; short-name: the trailing component of a qualified name — the cross-layer join
;; key. Collapses every name form to the func/method short name:
;;   "pkg.EncodeA" -> "EncodeA", "(*pkg.Cache).Update" -> "Update",
;;   "Cache.Update" -> "Update", "EncodeA" -> "EncodeA".
;; The (wile goast belief) ssa-short-name twin (not exported there); duplicated
;; here to avoid depending on belief internals. LIMITATION: two methods sharing a
;; short name across receiver types collide (rare); the index keeps the last.
(define (short-name full-name)
  (let ((len (string-length full-name)))
    (let loop ((i (- len 1)))
      (cond ((<= i 0) full-name)
            ((char=? (string-ref full-name i) #\.)
             (substring full-name (+ i 1) len))
            (else (loop (- i 1)))))))

;; all-pairs: unordered pairs (a . b), a before b, from a list.
(define (all-pairs lst)
  (if (or (null? lst) (null? (cdr lst))) '()
    (append (map (lambda (y) (cons (car lst) y)) (cdr lst))
            (all-pairs (cdr lst)))))

;; package-func-decls: flatten all func-decl AST nodes from go-typecheck-package
;; output (a list of packages, each with 'files, each with 'decls).
(define (package-func-decls pkgs)
  (let ((acc '()))
    (for-each
      (lambda (pkg)
        (for-each
          (lambda (file)
            (for-each
              (lambda (decl)
                (if (tag? decl 'func-decl) (set! acc (cons decl acc))))
              (let ((d (nf file 'decls))) (if (pair? d) d '()))))
          (let ((fs (nf pkg 'files))) (if (pair? fs) fs '()))))
      (if (pair? pkgs) pkgs '()))
    (reverse acc)))

;; build-func-ast-index / build-func-ssa-index: short-name -> node hashtables.
(define (build-func-ast-index pkgs)
  (let ((h (make-hashtable)))
    (for-each
      (lambda (fd)
        (let ((nm (nf fd 'name)))
          (if (string? nm) (hashtable-set! h (short-name nm) fd))))
      (package-func-decls pkgs))
    h))

(define (build-func-ssa-index ssa-funcs)
  (let ((h (make-hashtable)))
    (for-each
      (lambda (fn)
        (let ((nm (nf fn 'name)))
          (if (string? nm) (hashtable-set! h (short-name nm) fn))))
      (if (pair? ssa-funcs) ssa-funcs '()))
    h))

;; score-candidate-pair: benefit measures + equivalence tier for a candidate pair
;; (joined to AST/SSA via short-name). Returns an alist or #f when the AST nodes
;; cannot be resolved. Tier (prior-art pattern, NOT binop ssa-equivalent?):
;;   proven     = unifiable? over canonicalized SSA
;;   structural = unifiable? over AST (but not SSA-proven)
;;   divergent  = neither.
;; benefit = shared AST node count; type-params/value-params from score-diffs;
;; similarity = effective similarity (the per-pair confidence).
(define (score-candidate-pair name-a name-b ast-index ssa-index threshold)
  (let ((ast-a (hashtable-ref ast-index (short-name name-a) #f))
        (ast-b (hashtable-ref ast-index (short-name name-b) #f)))
    (if (not (and ast-a ast-b)) #f
      (let* ((ar      (ast-diff ast-a ast-b))
             (shared  (diff-result-shared ar))
             (dcount  (diff-result-diff-count ar))
             (diffs   (diff-result-diffs ar))
             (sc      (score-diffs shared dcount diffs))
             (eff     (list-ref sc 1))
             (roots   (list-ref sc 4))
             (vparams (list-ref sc 5))
             (ast-unif (unifiable? ar threshold))
             (ssa-a   (hashtable-ref ssa-index (short-name name-a) #f))
             (ssa-b   (hashtable-ref ssa-index (short-name name-b) #f))
             (ssa-unif
               (and ssa-a ssa-b
                    (unifiable? (ssa-diff (go-ssa-canonicalize ssa-a)
                                          (go-ssa-canonicalize ssa-b))
                                threshold)))
             (tier (cond (ssa-unif 'proven)
                         (ast-unif 'structural)
                         (else     'divergent))))
        (list (cons 'benefit shared)
              (cons 'type-params (length roots))
              (cons 'value-params (length vparams))
              (cons 'equiv-tier tier)
              (cons 'similarity eff))))))

;; pair-findings: the two located findings for a scored candidate pair. Each
;; finding's value is the function name; where from POS-INDEX; why is structured
;; (unify-candidate (peer . other) (measures . M)) so render-why projects it and
;; a script can filter on the measures; score = the pair's effective similarity
;; (a real per-pair confidence, unlike 5a's #f).
(define (pair-findings name-a name-b measures pos-index)
  (let ((sim (cdr (assq 'similarity measures))))
    (list (make-finding name-a (hashtable-ref pos-index name-a #f)
                        (cons 'unify-candidate
                              (list (cons 'peer name-b) (cons 'measures measures)))
                        sim)
          (make-finding name-b (hashtable-ref pos-index name-b #f)
                        (cons 'unify-candidate
                              (list (cons 'peer name-a) (cons 'measures measures)))
                        sim))))

;; scored-candidates: for each candidate concept, every within-cluster pair that
;; resolves to AST nodes becomes a scored candidate entry:
;;   ((pair . (a b)) (measures . M) (findings . (finding-a finding-b))).
;; Pairs whose AST nodes cannot be resolved (short-name miss) are dropped.
(define (scored-candidates concepts ast-index ssa-index pos-index threshold)
  (apply append
    (map (lambda (concept)
           (filter-map
             (lambda (pr)
               (let* ((a (car pr)) (b (cdr pr))
                      (m (score-candidate-pair a b ast-index ssa-index threshold)))
                 (and m
                      (list (cons 'pair (list a b))
                            (cons 'measures m)
                            (cons 'findings (pair-findings a b m pos-index))))))
             (all-pairs (concept-extent concept))))
         concepts)))

;; find-scored-candidates: top-level. TARGET is a package pattern or a GoSession.
;; Runs 5a discovery (FCA-on-refs clusters) then 5b structural scoring over each
;; cluster's pairs. Optional THRESHOLD (default 0.6) is the similarity/unifiable?
;; threshold. Returns a flat list of scored candidate entries.
(define (find-scored-candidates target . opts)
  (let* ((threshold (if (pair? opts) (car opts) 0.6))
         (s         (if (go-session? target) target (go-load target)))
         (refs      (go-func-refs s))
         (ctx       (function-ref-context refs))
         (lat       (concept-lattice ctx))
         (cands     (duplicate-candidate-concepts lat))
         (pos-index (func-refs->positions refs))
         (ast-index (build-func-ast-index (go-typecheck-package s)))
         (ssa-index (build-func-ssa-index (go-ssa-build s))))
    (scored-candidates cands ast-index ssa-index pos-index threshold)))

;; candidate->verdict: an OPT-IN projection of a scored candidate's measures into
;; a categorical verdict. The default analysis output is the measure surface; the
;; verdict is requested, never imposed (auditable-categorization principle #2 —
;; the categorical analog of finding->scalar). Tier-driven:
;;   proven -> 'duplicate, structural -> 'likely-duplicate, divergent -> 'distinct.
(define (candidate->verdict cand)
  (let ((tier (cdr (assq 'equiv-tier (cdr (assq 'measures cand))))))
    (cond ((eq? tier 'proven)     'duplicate)
          ((eq? tier 'structural) 'likely-duplicate)
          (else                   'distinct))))

;;; ── Slice 5c: cost-half measures ──────────────────────────

;; callers-set: distinct qualified caller names of NAME in CG ('caller field of
;; the edges-in returned by go-callgraph-callers; #f -> no callers).
(define (callers-set cg name)
  (let ((edges (go-callgraph-callers cg name)))
    (if (pair? edges)
      (unique (filter-map (lambda (e) (nf e 'caller)) edges))
      '())))

;; cand-new-edges: |callers(a) ∪ callers(b)| — the in-degree the merged function
;; would carry. Merging retargets edges rather than adding them; this measures
;; the coupling concentrated at the shared abstraction (the cost signal).
(define (cand-new-edges name-a name-b cg)
  (length (unique (append (callers-set cg name-a) (callers-set cg name-b)))))

;; callees-of: distinct qualified callee names of NAME in CG.
(define (callees-of cg name)
  (let ((edges (go-callgraph-callees cg name)))
    (if (pair? edges)
      (filter-map (lambda (e) (nf e 'callee)) edges)
      '())))

;; reaches?: does FROM transitively call TO? BFS over callees (there is no
;; go-callgraph-reachable primitive). Candidates are short names; callee names
;; are qualified, so compare via short-name. `seen' (short names) bounds cycles.
(define (reaches? cg from to)
  (let ((target (short-name to)))
    (let loop ((frontier (list from)) (seen '()))
      (if (null? frontier) #f
        (let ((n (car frontier)))
          (if (member (short-name n) seen)
            (loop (cdr frontier) seen)
            (let ((callees (callees-of cg n)))
              (if (member target (map short-name callees))
                #t
                (loop (append (cdr frontier) callees)
                      (cons (short-name n) seen))))))))))

;; cand-creates-cycle?: merging two functions on a call path collapses it into a
;; self-cycle. The merge-side analog of verify-acyclic.
(define (cand-creates-cycle? name-a name-b cg)
  (or (reaches? cg name-a name-b) (reaches? cg name-b name-a)))

;; func-ref-pkgs: the external package paths a func-ref entry references.
(define (func-ref-pkgs entry)
  (if entry
    (unique (map (lambda (r) (nf r 'pkg))
                 (let ((rs (nf entry 'refs))) (if (pair? rs) rs '()))))
    '()))

;; jaccard: |A ∩ B| / |A ∪ B|, 0 when both empty.
(define (jaccard a b)
  (let ((inter (length (filter (lambda (x) (member x b)) a)))
        (uni   (length (unique (append a b)))))
    (if (= uni 0) 0 (/ inter uni))))

;; build-func-ref-index: name -> func-ref entry (for pkg + ref-set lookup).
(define (build-func-ref-index func-refs)
  (let ((h (make-hashtable)))
    (for-each
      (lambda (fr)
        (let ((n (nf fr 'name)))
          (if (string? n) (hashtable-set! h n fr))))
      (if (pair? func-refs) func-refs '()))
    h))

;; cand-locality: a ledger fact (never a verdict). scope = same-pkg (both in one
;; package) / shared-callers (a common caller) / disjoint; dep-overlap = Jaccard
;; of external ref sets. The human reads this to judge coincidental vs. real
;; duplication.
(define (cand-locality name-a name-b fr-index cg)
  (let* ((ea (hashtable-ref fr-index name-a #f))
         (eb (hashtable-ref fr-index name-b #f))
         (pa (and ea (nf ea 'pkg)))
         (pb (and eb (nf eb 'pkg)))
         (ca (callers-set cg name-a))
         (cb (callers-set cg name-b))
         (shared (filter (lambda (x) (member x cb)) ca))
         (scope (cond ((and pa pb (equal? pa pb)) 'same-pkg)
                      ((pair? shared) 'shared-callers)
                      (else 'disjoint))))
    (list (cons 'scope scope)
          (cons 'dep-overlap (jaccard (func-ref-pkgs ea) (func-ref-pkgs eb))))))

;; find-candidates-with-cost: the full benefit/cost ledger. Like
;; find-scored-candidates, but each candidate's measures also carry new-edges,
;; creates-cycle?, and locality, and the findings embed the full set. TARGET is a
;; package pattern or GoSession; optional THRESHOLD (default 0.6).
(define (find-candidates-with-cost target . opts)
  (let* ((threshold (if (pair? opts) (car opts) 0.6))
         (s         (if (go-session? target) target (go-load target)))
         (refs      (go-func-refs s))
         (ctx       (function-ref-context refs))
         (lat       (concept-lattice ctx))
         (concepts  (duplicate-candidate-concepts lat))
         (pos-index (func-refs->positions refs))
         (ast-index (build-func-ast-index (go-typecheck-package s)))
         (ssa-index (build-func-ssa-index (go-ssa-build s)))
         (fr-index  (build-func-ref-index refs))
         (cg        (go-callgraph s 'static)))
    (apply append
      (map (lambda (concept)
             (filter-map
               (lambda (pr)
                 (let* ((a (car pr)) (b (cdr pr))
                        (bm (score-candidate-pair a b ast-index ssa-index threshold)))
                   (and bm
                        (let ((m (append bm
                                   (list (cons 'new-edges (cand-new-edges a b cg))
                                         (cons 'creates-cycle? (cand-creates-cycle? a b cg))
                                         (cons 'locality (cand-locality a b fr-index cg))))))
                          (list (cons 'pair (list a b))
                                (cons 'measures m)
                                (cons 'findings (pair-findings a b m pos-index)))))))
               (all-pairs (concept-extent concept))))
           concepts))))
