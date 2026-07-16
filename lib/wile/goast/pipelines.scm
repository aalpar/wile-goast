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

;; ->float64: coerce an inexact real to a JSON-marshallable float64.
;; score-diffs emits effective-similarity via exact->inexact over Rational,
;; which produces BigFloat (256-bit). The marshaller maps *values.BigFloat
;; to errMarshalUnsupported; inexact-with-accuracy is the only primitive
;; that explicitly truncates BigFloat to float64 (it returns (values Float
;; acc-symbol)). Returns Float unchanged.
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

;; pipeline-find-duplicates: scan TARGET for semantic duplicate function pairs
;; (FCA reference clustering + SSA/AST-verified equivalence). OPTS is an alist;
;; recognised keys are threshold (number, default 0.6 -- the unify/similarity
;; threshold that sets each pair's equiv-tier) and verdict (bool, default #f --
;; attach the opt-in categorical verdict). NB: threshold does not filter which
;; pairs are returned; every within-cluster pair that resolves to AST is a
;; candidate. An empty result (no clusterable pairs) is a success, count 0.
(define (pipeline-find-duplicates target opts)
  (let* ((threshold (or (assoc-default opts 'threshold) 0.6))
         (verdict?  (and (assoc-default opts 'verdict) #t))
         (cands     (find-scored-candidates target threshold))
         (clean     (map (lambda (c) (candidate->clean c verdict?)) cands)))
    (pipeline-envelope 1
      (list (cons 'target target)
            (cons 'threshold threshold)
            (cons 'candidate-count (length cands))
            (cons 'verdict-included verdict?))
      clean)))

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
    (let ((files (scm-source-files roots)))
      (let ((idx (build-index files)))
        (pipeline-envelope 1
          (list (cons 'method "lexical read-syntax scan (pre-macro-expansion)")
                (cons 'roots roots)
                (cons 'files-scanned (length files))
                (cons 'parse-errors (cdr (assq 'parse-errors idx)))
                (cons 'limits scmnav-limits))
          (defs-query symbol idx))))))

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
