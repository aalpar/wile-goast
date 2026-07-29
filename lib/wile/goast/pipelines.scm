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

;; ── Shared result view ───────────────────────────────────
;;
;; Filter, rank, page and render, uniformly across tools. Five caller
;; parameters live in OPTS: scope and symbol (globs), detail ('summary or
;; 'full), limit, offset. The DEFAULTS ARE WRITTEN HERE AND NOWHERE ELSE --
;; the Go handler forwards only what the caller actually set -- so a client
;; that knows nothing about these parameters gets a usable first response and
;; there is exactly one place to read the defaults off.

(define view-default-limit 50)
(define view-default-detail 'summary)

(define (view-detail opts) (or (assoc-default opts 'detail) view-default-detail))
(define (view-limit opts)  (or (assoc-default opts 'limit)  view-default-limit))
(define (view-offset opts) (or (assoc-default opts 'offset) 0))

;; view-merge / view-sort: a stable bottom-up merge sort in which every loop
;; is a tail call.
;;
;; The builtin `sort' is unusable here: measured on this engine it recurses one
;; frame per element and dies with "call depth 10001 exceeds limit 10000" on a
;; 20000-element list. find_duplicates can reach that -- max-candidate-pairs
;; admits 25000 scored pairs and nothing stops all of them unifying -- and the
;; failure would land AFTER the whole scoring cost is sunk. This is the same
;; class of bug as the old non-tail `map' that produced max-candidate-pairs
;; 9992; the fix there was to make the pass iterative, and it is here too.
(define (view-merge less? a b)
  (let loop ((a a) (b b) (acc '()))
    (cond ((and (null? a) (null? b)) (reverse acc))
          ((null? a) (loop a (cdr b) (cons (car b) acc)))
          ((null? b) (loop (cdr a) b (cons (car a) acc)))
          ;; Take from B only when strictly less, so equal keys keep A's
          ;; order: the sort is stable.
          ((less? (car b) (car a)) (loop a (cdr b) (cons (car b) acc)))
          (else (loop (cdr a) b (cons (car a) acc))))))

(define (view-sort less? lst)
  (if (or (null? lst) (null? (cdr lst)))
    lst
    (let pass ((runs (let split ((xs lst) (acc '()))
                       (if (null? xs)
                         (reverse acc)
                         (split (cdr xs) (cons (list (car xs)) acc))))))
      (if (null? (cdr runs))
        (car runs)
        (pass (let merge-pass ((rs runs) (acc '()))
                (cond ((null? rs) (reverse acc))
                      ((null? (cdr rs)) (reverse (cons (car rs) acc)))
                      (else (merge-pass (cddr rs)
                                        (cons (view-merge less? (car rs) (cadr rs))
                                              acc))))))))))

;; view-name-matcher: OPTS' scope/symbol globs as a predicate over ONE
;; function name, or #f when neither is set.
;;
;; Both tools identify a row by the function names it carries and differ only
;; in WHICH collection they scan -- a concept's extent, or a candidate pair's
;; two members -- so the matching itself belongs here, once. `symbol' matches
;; the name; `scope' matches the FILE of that name's position, because a
;; position is "file:line:col" and an anchored glob could never match one.
;; When both are set they must match the SAME name: the caller is identifying
;; one function by name and path, not two different ones.
(define (view-name-matcher opts pos-index)
  (let ((scope (assoc-default opts 'scope))
        (sym   (assoc-default opts 'symbol)))
    (and (or scope sym)
         (lambda (name)
           (and (or (not sym) (glob-match? sym name #\.))
                (or (not scope)
                    (glob-match? scope
                                 (position-file
                                   (hashtable-ref pos-index name ""))
                                 #\/)))))))

;; view-key<?: lexicographic order over a rank key (a list). NUMERIC
;; components compare DESCENDING -- a higher score, or more struct types tied
;; together, ranks first. STRING components compare ascending. The order is
;; not always total: find_duplicates' candidate-pairs is a not-deduplicated
;; `apply append`, so the same (a . b) pair reached through two FCA concepts
;; yields two rows with byte-identical rank keys. Paging is still safe,
;; because view-sort is a STABLE sort over a deterministic input order: two
;; calls with the same arguments feed it the same rows in the same order, so
;; page 2 cannot repeat a row from page 1 even when keys tie.
(define (view-key<? ka kb)
  (cond ((null? ka) #f)
        ((null? kb) #f)
        (else
          (let ((a (car ka)) (b (car kb)))
            (if (and (number? a) (number? b))
              (cond ((> a b) #t)
                    ((< a b) #f)
                    (else (view-key<? (cdr ka) (cdr kb))))
              (cond ((string<? a b) #t)
                    ((string<? b a) #f)
                    (else (view-key<? (cdr ka) (cdr kb)))))))))

;; apply-view: ROWS -> (values shown view-provenance).
;;   KEEP?     admits a row; #f admits everything, for a tool that already
;;             pruned upstream -- re-applying the predicate here would be a
;;             no-op that hides where the work was actually saved.
;;   RANK-KEY  row -> rank key (see view-key<?).
;;   RENDER    (row detail) -> the row's public shape. Called only on the rows
;;             actually returned, so a withheld row costs nothing to render.
;;             That is what lets find_false_boundaries skip its lattice
;;             annotation pass entirely at detail=summary rather than compute
;;             and discard it.
;;
;; total-count counts rows AFTER filtering and BEFORE offset/limit, so
;; `truncated' means the same thing whether the tool filtered here or upstream.
;; truncated is the load-bearing field: a client seeing total-count 186,
;; returned-count 50, truncated #t knows to narrow or page, and one seeing
;; truncated #f knows it holds the complete answer. Silent truncation with no
;; such signal would be worse than no limit at all.
(define (apply-view rows opts keep? rank-key render)
  (let* ((detail (view-detail opts))
         (limit  (view-limit opts))
         (offset (view-offset opts))
         (kept   (if keep? (filter keep? rows) rows))
         (total  (length kept))
         ;; Decorate once per row, not once per comparison, via a tail
         ;; recursion: `map' is not tail-recursive here and this list is one
         ;; entry per surviving row.
         (keyed  (let loop ((rs kept) (acc '()))
                   (if (null? rs)
                     (reverse acc)
                     (loop (cdr rs) (cons (cons (rank-key (car rs)) (car rs)) acc)))))
         (sorted (view-sort (lambda (a b) (view-key<? (car a) (car b))) keyed))
         ;; Undecorate and render the page in one tail-recursive pass: `take'
         ;; and `map' are both non-tail, and LIMIT is caller-supplied with no
         ;; ceiling of its own (find_duplicates' max-candidate-pairs guard
         ;; alone admits 25000 rows), so a caller asking for everything would
         ;; hit the same call-depth cliff view-sort exists to avoid, only
         ;; after the expensive analysis is already paid for.
         (shown  (let loop ((rs (drop sorted offset)) (n limit) (acc '()))
                   (if (or (null? rs) (<= n 0))
                     (reverse acc)
                     (loop (cdr rs) (- n 1)
                           (cons (render (cdr (car rs)) detail) acc)))))
         (returned (length shown)))
    (values shown
            (append
              (list (cons 'total-count total)
                    (cons 'returned-count returned)
                    (cons 'offset offset)
                    (cons 'limit limit)
                    (cons 'detail detail)
                    (cons 'truncated (> total (+ offset returned))))
              (let ((sc (assoc-default opts 'scope)))
                (if sc (list (cons 'scope sc)) '()))
              (let ((sy (assoc-default opts 'symbol)))
                (if sy (list (cons 'symbol sy)) '()))))))

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

;; ── find_false_boundaries: view hooks ────────────────────

;; fb-keep?: OPTS' scope/symbol as a predicate over a cross-boundary concept,
;; or #f when neither is set.
;;
;; The filter runs on FINISHED concepts, never on the FCA context. Restricting
;; the context recomputes the lattice over a different object set and reports
;; absence where findings exist. Measured on ./goast, write-only, 178 objects,
;; 5 cross-boundary concepts: pushing *restructure* down keeps 3 objects and
;; finds 0 concepts, against the 2 finished concepts whose extent contains
;; restructureBackwardGotos. Telling a caller there are no false boundaries
;; when there are five is worse than declining. There is no cost argument on
;; the other side either: the whole call is ~1.7s over an 89-concept lattice.
;;
;; A concept is admitted when AT LEAST ONE extent member matches -- the caller
;; asked about a function, and a cross-boundary concept is interesting
;; precisely because it ties that function to others.
(define (fb-keep? opts pos-index)
  (let ((match? (view-name-matcher opts pos-index)))
    (and match?
         (lambda (concept)
           (let loop ((ns (concept-extent concept)))
             (cond ((null? ns) #f)
                   ((match? (car ns)) #t)
                   (else (loop (cdr ns)))))))))

;; fb-types: the distinct struct types a concept's intent spans.
;; cross-boundary-concepts computes exactly this at fca.scm:286 to enforce
;; min-types and then discards it, returning the bare concept; the pipeline
;; calls annotated-boundary-report rather than the boundary-report that
;; recomputes it. So this is a one-line recompute, not an exposed value being
;; plumbed through the algebra layer.
(define (fb-types concept)
  (unique (map attr-struct-name (concept-intent concept))))

;; fb-rank-key: types spanned, then intent size, then extent size, all
;; descending; the joined extent is the deterministic tie-break. A concept
;; that ties MORE struct types together across MORE shared fields is the
;; stronger false-boundary signal.
;;
;; NB this ordering is a choice made HERE. cross-boundary-concepts takes
;; min-types as a floor and expresses no preference among concepts above it.
(define (fb-rank-key concept)
  (list (length (fb-types concept))
        (length (concept-intent concept))
        (length (concept-extent concept))
        (string-join (concept-extent concept) " ")))

;; fb-render: a concept -> its public row at DETAIL.
;;   full    = today's shape exactly: extent, intent, extent-size, summary,
;;             and the three lattice-relationship lists.
;;   summary = the finding itself plus types-count, dropping the relationship
;;             lists. Measured on ./goast those three fields are 90.5% of the
;;             13301 result bytes (incomparable-with alone 60.6%), which is
;;             why row WIDTH, not row count, is this tool's failure mode: a
;;             design that only paged would leave it exactly as unusable.
;; Because apply-view renders after paging, annotated-boundary-report runs
;; only for the rows actually returned -- and at summary, not at all.
(define (fb-render lattice)
  (lambda (concept detail)
    (if (eq? detail 'full)
      (car (annotated-boundary-report (list concept) lattice))
      (list (cons 'extent (concept-extent concept))
            (cons 'intent (concept-intent concept))
            (cons 'extent-size (length (concept-extent concept)))
            (cons 'summary (concept-summary concept))
            (cons 'types-count (length (fb-types concept)))))))

;; ── find_false_boundaries ────────────────────────────────
;;
;; Build an FCA context from struct-field access in MODE, filter for
;; concepts that span multiple struct types (cross-boundary candidates), and
;; post-filter/rank/page/render the finished concepts through the shared
;; result view. OPTS is an alist; recognised keys are mode (symbol),
;; min-extent / min-intent / min-types (integers, all default 2), and the
;; shared view parameters (scope, symbol, detail, limit, offset). scope/symbol
;; filter the FINISHED concepts, never the FCA context -- see fb-keep?.

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
         ;; The annotated report carries no position, so scope needs the
         ;; name->position join the FCA layer keeps out of the algebra. Built
         ;; only when scope/symbol is actually set -- same guard as
         ;; view-name-matcher's -- since fb-keep? never touches it otherwise.
         (pos-index (and (or (assoc-default opts 'scope) (assoc-default opts 'symbol))
                          (field-index->positions field-idx))))
    (call-with-values
      (lambda ()
        (apply-view cross opts (fb-keep? opts pos-index)
                    fb-rank-key (fb-render lattice)))
      (lambda (shown view-prov)
        ;; Version 2: detail=summary is the default, so a default call's row
        ;; shape changed (the three relationship lists are gone).
        (pipeline-envelope 2
          (append
            (list (cons 'target target)
                  (cons 'mode mode)
                  (cons 'lattice-size (length lattice))
                  ;; cross-boundary-count is the PRE-filter population;
                  ;; total-count (from the view) is the post-filter one.
                  (cons 'cross-boundary-count (length cross)))
            view-prov)
          shown)))))

;; ── find_duplicates ──────────────────────────────────────
;;
;; Runs (wile goast dup-detect)'s discovery (duplicate-candidate-concepts,
;; function-ref-context, candidate-pairs, func-refs->positions) and scoring
;; (score-candidate-pairs) as separate steps, not via find-scored-candidates,
;; so the scope/symbol prune (dup-keep-pair?, below) can sit between them on
;; the flat pair list, before scoring runs. score-candidate-pairs was split
;; out of find-scored-candidates for this: taking a flat pair list rather
;; than concepts is what lets a prune sit between discovery and scoring at
;; all. The raw
;; candidate carries make-finding records whose leading 'finding tag defeats
;; the JSON marshaller's alist detection (it would emit an array of
;; {car,cdr} objects); clean-finding / candidate->clean project each
;; candidate into marshaller-clean alists.

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
;; now carries only the pairs that unify (kept-tiers), filtered and rendered
;; by apply-view's tail-recursive loops, so that limit no longer binds. The
;; old number
;; also made the tool unable to answer for goast at all: goast is a single
;; package and yields 20404 pairs on its own, so "narrow to one package" had
;; no valid answer.
;;
;; The count checked against this budget is taken AFTER the scope/symbol
;; prune, so a narrow query against a wide target is answered rather than
;; refused. That converts the guard from a hard wall into one the caller can
;; walk around by being specific: ./... is 58551 pairs unscoped and refused,
;; but a scope naming one file is scored and answered.
(define max-candidate-pairs 25000)

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

;; ── find_duplicates: view hooks ──────────────────────────

;; dup-keep-pair?: OPTS' scope/symbol as a predicate over a CANDIDATE PAIR
;; (a . b) of function names, or #f when neither is set.
;;
;; This filter pushes DOWN: it lands on the flat pair list, before scoring, so
;; a scoped query costs less rather than merely returning less. Sound because
;; the FCA concepts are formed from the whole session BEFORE the prune, and
;; score-candidate-pair depends only on the two functions and the threshold --
;; a surviving pair scores identically, so a scoped result is a genuine subset
;; of the unscoped one.
;;
;; A pair survives if EITHER function matches: asking about marshalValue
;; should surface its duplicate wherever that lives.
(define (dup-keep-pair? opts pos-index)
  (let ((match? (view-name-matcher opts pos-index)))
    (and match?
         (lambda (pr) (or (match? (car pr)) (match? (cdr pr)))))))

;; dup-rank-key: score descending, then proven before structural, then the
;; pair's names as the deterministic tie-break. Score is coerced to float64
;; for the same reason candidate->clean coerces it: score-diffs emits a
;; 256-bit BigFloat, and the key must order by the number actually reported.
(define (dup-tier-rank tier) (if (eq? tier 'proven) 1 0))

(define (dup-rank-key cand)
  (let ((measures (cdr (assq 'measures cand))))
    (list (->float64 (cdr (assq 'similarity measures)))
          (dup-tier-rank (cdr (assq 'equiv-tier measures)))
          (string-join (cdr (assq 'pair cand)) " "))))

;; dup-render: a RAW scored candidate -> its public row at DETAIL. Both levels
;; go through candidate->clean, because a raw make-finding record's leading
;; 'finding tag defeats the marshaller's alist detection. summary then drops
;; the measures sub-object, whose `similarity' merely repeats the top-level
;; score. An opt-in verdict survives both levels: a caller who asked for it
;; asked for it.
(define (dup-render verdict?)
  (lambda (cand detail)
    (let ((row (candidate->clean cand verdict?)))
      (if (eq? detail 'full)
        row
        (filter (lambda (kv) (not (eq? (car kv) 'measures))) row)))))

;; pipeline-find-duplicates: scan TARGET for semantic duplicate function pairs
;; (FCA reference clustering + SSA/AST-verified equivalence). OPTS is an alist;
;; recognised keys are threshold (number, default 0.6 -- the unify/similarity
;; threshold), verdict (bool, default #f -- attach the opt-in categorical
;; verdict), and the shared view parameters (scope, symbol, detail, limit,
;; offset). Result carries only the pairs whose tier is in kept-tiers, so
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
         ;; Discovery runs once, here, and its refs and positions are handed
         ;; to the scorer rather than re-derived: pos-index is available from
         ;; the cheap go-func-refs pass, before the expensive canon-index, so
         ;; the prune can see both name and file.
         (refs      (go-func-refs session))
         (concepts  (duplicate-candidate-concepts
                      (concept-lattice (function-ref-context refs))))
         (pos-index (func-refs->positions refs))
         (keep-pair? (dup-keep-pair? opts pos-index))
         (all-prs   (candidate-pairs concepts))
         (pairs     (if keep-pair? (filter keep-pair? all-prs) all-prs))
         (npairs    (length pairs)))
    (if (> npairs max-candidate-pairs)
      (error (string-append
               "find_duplicates: target yields " (number->string npairs)
               " candidate pairs, over the " (number->string max-candidate-pairs)
               " this tool will score; narrow with scope or symbol, or narrow"
               " a '/...' tree to one package")
             target)
      (let* ((cands (score-candidate-pairs session refs pairs threshold))
             ;; The tier filter is adjudication, not the caller's filter, so
             ;; it runs here and apply-view's keep? stays #f: re-applying the
             ;; scope predicate on finished rows would be a no-op that hides
             ;; where the work was actually saved.
             (kept  (filter (lambda (c) (memq (candidate-tier c) kept-tiers))
                            cands)))
        (call-with-values
          (lambda () (apply-view kept opts #f dup-rank-key (dup-render verdict?)))
          (lambda (shown view-prov)
            ;; Version 2: detail=summary is the default, so a default call's
            ;; row shape changed (measures is gone), and candidate-count is
            ;; retired in favour of the view's total-count -- one number, one
            ;; name, the same on both tools.
            (pipeline-envelope 2
              (append
                (list (cons 'target target)
                      (cons 'threshold threshold)
                      (cons 'scored-pair-count (length cands))
                      (cons 'tiers (tier-tally cands))
                      (cons 'candidate-pair-count npairs)
                      (cons 'max-candidate-pairs max-candidate-pairs)
                      (cons 'verdict-included verdict?))
                view-prov)
              shown)))))))

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
