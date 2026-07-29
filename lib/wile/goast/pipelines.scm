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

;; ── Shared envelope ─────────────────────────────────────
;;
;; Every Phase 1 tool returns:
;;   ((version    . <integer>)
;;    (provenance . <alist>)
;;    (result     . <alist-or-list>))
;;
;; The version is a per-tool integer, supplied by each pipeline. It is
;; bumped only on breaking changes to that tool's result shape;
;; additive provenance changes do not bump.
;;
;; The Go handler converts this alist to JSON via the marshaller in
;; cmd/wile-goast/marshal.go. Alist keys are emitted in kebab-case
;; here; the marshaller normalises to snake_case at the JSON boundary.
;;
;; No `tool` field: peer protocols (JSON-RPC, MCP, GraphQL, REST) do
;; not echo the call name in responses.

(define (pipeline-envelope version provenance result)
  (list (cons 'version version)
        (cons 'provenance provenance)
        (cons 'result result)))

;; ── check_beliefs ────────────────────────────────────────
;;
;; Load the .scm beliefs at BELIEFS-PATH (a directory or single file),
;; run them against TARGET, and return the per-belief result list under
;; result. with-belief-scope confines the loaded beliefs to this call;
;; load-beliefs! activates them so run-beliefs sees them (load-committed-
;; beliefs would isolate them instead). Provenance records the target,
;; the path probed, and the number of belief files loaded.

(define (pipeline-check-beliefs target beliefs-path)
  (with-belief-scope
    (lambda ()
      (let* ((count (load-beliefs! beliefs-path))
             (results (run-beliefs target)))
        (pipeline-envelope 1
          (list (cons 'target target)
                (cons 'beliefs-path beliefs-path)
                (cons 'belief-count count))
          results)))))

;; ── discover_beliefs ─────────────────────────────────────
;;
;; Run the DISCOVERY-PATH beliefs against TARGET, suppress any result
;; whose expressions match a belief in COMMITTED-PATH, and emit the
;; survivors as Scheme source ready to commit. COMMITTED-PATH may be ""
;; (or a path to an empty directory) — in either case no suppression is
;; applied (raw discovery is returned). Provenance records raw vs
;; filtered counts so the caller sees how much suppression removed.

(define (pipeline-discover-beliefs target discovery-path committed-path)
  (let* ((results
           (with-belief-scope
             (lambda ()
               (load-beliefs! discovery-path)
               (run-beliefs target))))
         (committed
           (if (or (not committed-path) (equal? committed-path ""))
             (cons '() '())
             (load-committed-beliefs committed-path)))
         (filtered (suppress-known results committed))
         (emitted (emit-beliefs filtered)))
    (pipeline-envelope 1
      (list (cons 'target target)
            (cons 'discovery-path discovery-path)
            (cons 'committed-path (or committed-path ""))
            (cons 'raw-count (length results))
            (cons 'filtered-count (length filtered)))
      (list (cons 'emitted-source emitted)
            (cons 'filtered-results filtered)))))

;; ── recommend_split ──────────────────────────────────────
;;
;; Apply IDF-weighted FCA + min-cut to TARGET's per-function import
;; signatures. OPTS is an alist of overrides; recognised keys are
;; idf-threshold and max-attributes (key value) and refine (bare flag).
;; recommend-split reads opts as a flat plist (opt-ref / memq), so the
;; alist is flattened to (idf-threshold N max-attributes N refine).

;; maybe-kv: ((key . val)) -> (key val), or () when key is absent.
(define (maybe-kv opts key)
  (let ((e (assoc key opts)))
    (if e (list key (cdr e)) '())))

(define (pipeline-recommend-split target opts)
  (let* ((session (go-load target))
         (refs (go-func-refs session))
         (kw (append
               (maybe-kv opts 'idf-threshold)
               (maybe-kv opts 'max-attributes)
               (if (let ((e (assoc 'refine opts))) (and e (cdr e)))
                 (list 'refine)
                 '())))
         (report (apply recommend-split refs kw)))
    (pipeline-envelope 1
      (list (cons 'target target)
            (cons 'options kw)
            (cons 'function-count (length refs)))
      report)))

;; assoc-default: (cdr (assoc key alist)) or #f when key is absent.
(define (assoc-default alist key)
  (let ((e (assoc key alist)))
    (if e (cdr e) #f)))

;; ── recommend_boundaries ─────────────────────────────────
;;
;; Build a (function -> struct-field) FCA context from SSA field-access
;; data in MODE ('write-only / 'read-write / 'type-only), compute the
;; concept lattice, and ask fca-recommend for the three Pareto frontiers
;; (split / merge / extract). MODE may be #f (defaults to 'write-only).

(define (pipeline-recommend-boundaries target mode)
  (let* ((session (go-load target))
         (field-idx (go-ssa-field-index session))
         (m (or mode 'write-only))
         (ctx (field-index->context field-idx m))
         (lattice (concept-lattice ctx))
         (ssa-funcs (go-ssa-build session))
         (rec (boundary-recommendations lattice ssa-funcs)))
    (pipeline-envelope 1
      (list (cons 'target target)
            (cons 'mode m)
            (cons 'concept-count (length lattice)))
      rec)))

;; ── find_false_boundaries ────────────────────────────────
;;
;; Build an FCA context from struct-field access in MODE, filter for
;; concepts that span multiple struct types (cross-boundary candidates),
;; and annotate each with its lattice relationships. OPTS is an alist;
;; recognised keys are mode (symbol) and min-extent / min-intent /
;; min-types (integers, all default 2).

(define (pipeline-find-false-boundaries target opts)
  (let* ((session (go-load target))
         (field-idx (go-ssa-field-index session))
         (mode (or (assoc-default opts 'mode) 'write-only))
         (min-ext (or (assoc-default opts 'min-extent) 2))
         (min-int (or (assoc-default opts 'min-intent) 2))
         (min-typ (or (assoc-default opts 'min-types) 2))
         (ctx (field-index->context field-idx mode))
         (lattice (concept-lattice ctx))
         (cross (cross-boundary-concepts lattice
                  'min-extent min-ext
                  'min-intent min-int
                  'min-types min-typ))
         (annotated (annotated-boundary-report cross lattice)))
    (pipeline-envelope 1
      (list (cons 'target target)
            (cons 'mode mode)
            (cons 'lattice-size (length lattice))
            (cons 'cross-boundary-count (length cross)))
      annotated)))

;; ── find_duplicates ──────────────────────────────────────
;;
;; Surface (wile goast dup-detect)'s find-scored-candidates as a
;; command-level tool. The raw candidate carries make-finding records whose
;; leading 'finding tag defeats the JSON marshaller's alist detection (it
;; would emit an array of {car,cdr} objects); clean-finding / candidate->clean
;; project each candidate into marshaller-clean alists. Pure surfacing: no
;; change to dup-detect.scm.

;; ->float64: coerce an inexact real to float64.
;; score-diffs emits effective-similarity via exact->inexact over Rational,
;; which produces BigFloat (256-bit). inexact-with-accuracy is the only
;; primitive that explicitly truncates BigFloat to float64 (it returns
;; (values Float acc-symbol)). Returns Float unchanged.
;;
;; No longer load-bearing for marshalling: marshalValue now maps
;; *values.BigFloat to float64 itself (same truncation, via
;; Float64Truncated), so an uncoerced BigFloat no longer fails the envelope.
;; Kept because the projected value should be float64 in Scheme too -- a
;; caller reading the alist directly, or any arithmetic done on it before the
;; envelope is built, should not silently be at 256-bit precision.
(define (->float64 n)
  (call-with-values (lambda () (inexact-with-accuracy n))
    (lambda (v _) v)))

;; clean-finding: a make-finding record -> ((name . fn) (position . where)
;; (score . s)). Drops the leading 'finding tag and the redundant nested
;; measures carried in the finding's why. Score is coerced to float64 so
;; the marshaller can emit it as a JSON number.
(define (clean-finding f)
  (list (cons 'name     (finding-value f))
        (cons 'position (finding-where f))
        (cons 'score    (->float64 (finding-score f)))))

;; candidate->clean: a scored candidate -> the tool's public shape. Lifts
;; equiv-tier and similarity to top level, keeps the numeric measures
;; sub-object, reshapes the two located findings. VERDICT? attaches the
;; opt-in categorical verdict (computed from the RAW candidate, which still
;; carries measures.equiv-tier).
(define (candidate->clean cand verdict?)
  (let* ((measures (cdr (assq 'measures cand)))
         (findings (cdr (assq 'findings cand)))
         (tier     (cdr (assq 'equiv-tier measures)))
         (sim      (->float64 (cdr (assq 'similarity measures))))
         (base
           (list (cons 'functions (map clean-finding findings))
                 (cons 'score sim)
                 (cons 'equiv-tier tier)
                 (cons 'measures
                       (list (cons 'benefit      (cdr (assq 'benefit measures)))
                             (cons 'type-params  (cdr (assq 'type-params measures)))
                             (cons 'value-params (cdr (assq 'value-params measures)))
                             (cons 'similarity   sim))))))
    (if verdict?
      (append base (list (cons 'verdict (candidate->verdict cand))))
      base)))

;; max-candidate-pairs: the widest target this tool will score.
;;
;; A LATENCY budget, not a projection ceiling. The whole scoring cost is sunk
;; before any result exists, so a target past the budget is refused up front
;; rather than paid for and then abandoned. Measured on this repo: ./goast,
;; 20404 pairs, 60s (340/s); ./..., 58551 pairs, 3m31s (279/s -- the rate
;; falls as the session grows). 25000 is ~80s at that range: it admits the
;; largest single package and refuses the whole-tree pattern.
;;
;; It is deliberately NOT the old 9992, which was Wile's 10000-frame call
;; depth showing through a non-tail `map` over every scored pair. The result
;; now carries only the pairs that unify (kept-tiers) and filter-map projects
;; them in constant call depth, so that limit no longer binds. The old number
;; also made the tool unable to answer for goast at all: goast is a single
;; package and yields 20404 pairs on its own, so "narrow to one package" had
;; no valid answer.
(define max-candidate-pairs 25000)

;; candidate-pair-count: how many pairs scoring will produce, sum of C(k,2)
;; over the candidate concepts' extents. Derived from discovery alone (refs ->
;; lattice -> concepts, no SSA build), which is the ~1s half of the analysis,
;; so the guard is cheap against the 60-80s it avoids.
(define (candidate-pair-count concepts)
  (let loop ((cs concepts) (n 0))
    (if (null? cs)
      n
      (let ((k (length (concept-extent (car cs)))))
        (loop (cdr cs) (+ n (quotient (* k (- k 1)) 2)))))))

;; candidate-tier: a scored candidate's equiv-tier (proven / structural /
;; divergent), as set by score-candidate-pair at the caller's threshold.
(define (candidate-tier cand)
  (cdr (assq 'equiv-tier (cdr (assq 'measures cand)))))

;; kept-tiers: the tiers that constitute a finding. FCA clustering is candidate
;; GENERATION ("these functions reference overlapping things", C(k,2) per
;; concept); the tier is the ADJUDICATION that follows it. A divergent pair is
;; one the unifier tried and rejected, so shipping it as a result row would
;; report the question, not the answer -- and dominate the payload doing it
;; (./goast: 20336 of 20404 pairs are divergent). The rejected pairs stay
;; visible as counts under provenance's tiers.
(define kept-tiers '(proven structural))

;; tier-tally: per-tier counts over the scored candidates, so provenance
;; reports the work done even when every pair was rejected and result is [].
(define (tier-tally cands)
  (let loop ((cs cands) (p 0) (s 0) (d 0))
    (if (null? cs)
      (list (cons 'proven p) (cons 'structural s) (cons 'divergent d))
      (let ((t (candidate-tier (car cs))))
        (cond ((eq? t 'proven)     (loop (cdr cs) (+ p 1) s d))
              ((eq? t 'structural) (loop (cdr cs) p (+ s 1) d))
              (else                (loop (cdr cs) p s (+ d 1))))))))

;; pipeline-find-duplicates: scan TARGET for semantic duplicate function pairs
;; (FCA reference clustering + SSA/AST-verified equivalence). OPTS is an alist;
;; recognised keys are threshold (number, default 0.6 -- the unify/similarity
;; threshold) and verdict (bool, default #f -- attach the opt-in categorical
;; verdict). Result carries only the pairs whose tier is in kept-tiers, so
;; threshold IS the selection knob: it decides unifiable?, unifiable? decides
;; the tier, and the tier decides inclusion. An empty result (nothing unified,
;; or nothing clusterable) is a success, count 0.
;;
;; Runs discovery first and refuses a target over max-candidate-pairs. The
;; refusal is an error, not an empty result: result is a list here, so an empty
;; one already means "no duplicates" and could not carry a refusal unambiguously.
(define (pipeline-find-duplicates target opts)
  (let* ((threshold (or (assoc-default opts 'threshold) 0.6))
         (verdict?  (and (assoc-default opts 'verdict) #t))
         (session   (if (go-session? target) target (go-load target)))
         (concepts  (duplicate-candidate-concepts
                      (concept-lattice
                        (function-ref-context (go-func-refs session)))))
         (npairs    (candidate-pair-count concepts)))
    (if (> npairs max-candidate-pairs)
      (error (string-append
               "find_duplicates: target yields " (number->string npairs)
               " candidate pairs, over the " (number->string max-candidate-pairs)
               " this tool will score; narrow a '/...' tree to one package")
             target)
      ;; filter-map, not filter + map: one tail-recursive pass that both drops
      ;; the divergent pairs and projects the survivors, in constant call depth.
      (let* ((cands (find-scored-candidates session threshold))
             (clean (filter-map
                      (lambda (c)
                        (and (memq (candidate-tier c) kept-tiers)
                             (candidate->clean c verdict?)))
                      cands)))
        (pipeline-envelope 1
          (list (cons 'target target)
                (cons 'threshold threshold)
                (cons 'candidate-count (length clean))
                (cons 'scored-pair-count (length cands))
                (cons 'tiers (tier-tally cands))
                (cons 'candidate-pair-count npairs)
                (cons 'max-candidate-pairs max-candidate-pairs)
                (cons 'verdict-included verdict?))
          clean)))))

;; ── scheme_defs / scheme_symbols ─────────────────────────
;;
;; Lexical jump-to-def over caller-supplied roots (comma-separated). See
;; (wile goast scmnav). Result is the defs-query list; provenance carries
;; the pre-macro-expansion soundness boundary.

(define (scmnav-split-roots s)
  ;; string-trim (SRFI 13) trims only the LEFT side, so a root written
  ;; "../lib , ../examples" would leave a trailing space on "../lib" while
  ;; correctly stripping the leading space off "../examples" -- an
  ;; asymmetric bug that only shows up with space-after-comma input.
  ;; string-trim-both trims both ends.
  (let loop ((i 0) (start 0) (acc '()))
    (cond ((= i (string-length s))
           (reverse (cons (string-trim-both (substring s start i)) acc)))
          ((char=? (string-ref s i) #\,)
           (loop (+ i 1) (+ i 1) (cons (string-trim-both (substring s start i)) acc)))
          (else (loop (+ i 1) start acc)))))

(define scmnav-limits
  (list "cond-expand not evaluated"
        "macro-generated bindings invisible"
        "references/callers out of scope (v1)"))

(define (pipeline-scheme-defs symbol roots-string)
  (let ((roots (scmnav-split-roots roots-string)))
    (let ((unreadable (filter (lambda (r) (not (try-directory-files r))) roots)))
      (if (not (null? unreadable))
          (error "scheme_defs: root is not a readable directory" (string-join unreadable " "))
          (let ((files (scm-source-files roots)))
            (let ((idx (build-index files)))
              (pipeline-envelope 1
                (list (cons 'method "lexical read-syntax scan (pre-macro-expansion)")
                      (cons 'roots roots)
                      (cons 'files-scanned (length files))
                      (cons 'parse-errors (cdr (assq 'parse-errors idx)))
                      (cons 'limits scmnav-limits))
                (defs-query symbol idx))))))))

(define (pipeline-scheme-symbols file)
  ;; NOTE: `read-syntax` never populates `syntax-source` over a file port
  ;; (wile's PrimReadSyntax parses with an empty filename), so every def's
  ;; 'file arrives as "". This pipeline calls classify-def-form DIRECTLY,
  ;; bypassing index-file, so it MUST apply `with-source-file` itself —
  ;; otherwise scheme_symbols ships every row with an empty filename.
  ;; `with-source-file` REPLACES the stale entry (never prepends a second
  ;; one: marshal.go assigns obj[key]=val front-to-back, so a duplicate key
  ;; resolves to the LAST/stale value at the JSON boundary).
  (let* ((forms (read-file-forms file))
         (defs (sort (lambda (a b) (< (cdr (assq 'line a)) (cdr (assq 'line b))))
                          (map (lambda (d) (with-source-file file d))
                               (flat-map classify-def-form forms))))
         (lib (let loop ((fs forms))
                (if (null? fs) #f
                    (let ((l (classify-library (car fs))))
                      (if l (with-source-file file l) (loop (cdr fs))))))))
    (pipeline-envelope 1
      (list (cons 'method "lexical read-syntax scan (pre-macro-expansion)")
            (cons 'file file)
            (cons 'limits scmnav-limits))
      (list (cons 'defs defs)
            (cons 'library lib)))))
