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

;;; (wile goast split) — Package splitting via import signature analysis
;;;
;;; Analyzes a Go package's functions by their external dependency profiles
;;; to discover natural package boundaries.

(define (filter pred lst)
  (filter-map (lambda (x) (and (pred x) x)) lst))

(define (import-signatures func-refs)
  "Extract per-function import signatures from go-func-refs output.
Each function maps to the set of external package paths it references.

Parameters:
  func-refs : list — output from (go-func-refs ...)
Returns: list — alist mapping function name to list of package paths
Category: goast-split

Examples:
  (import-signatures (go-func-refs \"my/pkg\"))
  ;; => ((\"MyFunc\" \"io\" \"fmt\") (\"Helper\" \"strings\"))

See also: `compute-idf', `filter-noise'."
  (map (lambda (fr)
         (cons (nf fr 'name)
               (map (lambda (r) (nf r 'pkg))
                    (let ((refs (nf fr 'refs)))
                      (if refs refs '())))))
       func-refs))

(define (compute-idf signatures)
  "Compute IDF weights for each external package.
IDF(pkg) = log(N / df(pkg)), where N = total functions, df = functions referencing pkg.
High IDF = rare dependency (informative). Low IDF = ubiquitous (noise).

Parameters:
  signatures : list — output from (import-signatures ...)
Returns: list — alist mapping package path to IDF score (inexact)
Category: goast-split

Examples:
  (compute-idf '((\"F1\" \"io\" \"fmt\") (\"F2\" \"io\")))
  ;; => ((\"io\" . 0.0) (\"fmt\" . 0.693...))

See also: `import-signatures', `filter-noise'."
  (let* ((n (length signatures))
         (df (make-df-table signatures)))
    (map (lambda (entry)
           (cons (car entry) (log (/ n (cdr entry)))))
         df)))

(define (make-df-table signatures)
  "Build document-frequency table: pkg -> count of functions referencing it."
  (let ((table '()))
    (for-each
      (lambda (sig)
        (for-each
          (lambda (pkg)
            (let ((entry (assoc pkg table)))
              (if entry
                (set-cdr! entry (+ (cdr entry) 1))
                (set! table (cons (cons pkg 1) table)))))
          (cdr sig)))
      signatures)
    table))

(define (filter-noise signatures idf-weights . opts)
  "Remove low-IDF (noise) packages from import signatures.
Default threshold: 0.36 (excludes packages in >70% of functions).

Parameters:
  signatures  : list — output from (import-signatures ...)
  idf-weights : list — output from (compute-idf ...)
  opts        : optional number — IDF threshold (default 0.36)
Returns: list — filtered signatures (same shape, fewer packages per entry)
Category: goast-split

Examples:
  (filter-noise sigs idf 0.5)

See also: `compute-idf', `build-package-context'."
  (let ((threshold (if (null? opts) 0.36 (car opts))))
    (map (lambda (sig)
           (cons (car sig)
                 (filter (lambda (pkg)
                           (let ((w (assoc pkg idf-weights)))
                             (and w (>= (cdr w) threshold))))
                         (cdr sig))))
         signatures)))

(define (build-package-context filtered-signatures)
  "Build FCA formal context from filtered import signatures.
Objects = function names. Attributes = high-IDF package paths.

Parameters:
  filtered-signatures : list — output from (filter-noise ...)
Returns: fca-context — formal context for (concept-lattice)
Category: goast-split

Examples:
  (define ctx (build-package-context filtered))
  (concept-lattice ctx)

See also: `filter-noise', `refine-by-api-surface', `context-from-alist'."
  (context-from-alist
    (filter (lambda (sig) (not (null? (cdr sig))))
            filtered-signatures)))

(define (refine-by-api-surface func-refs filtered-signatures)
  "Refine FCA context to (package, object-name) granularity.
Replaces package-level attributes with pkg:object pairs for finer
sub-clustering when two functions import the same package but use
different API surfaces.

Parameters:
  func-refs             : list — raw output from (go-func-refs ...)
  filtered-signatures   : list — output from (filter-noise ...)
Returns: fca-context — refined formal context
Category: goast-split

Examples:
  (define ctx (refine-by-api-surface raw-refs filtered))
  (concept-lattice ctx)

See also: `build-package-context', `import-signatures'."
  (let* ((high-idf-pkgs
           (let loop ((sigs filtered-signatures) (acc '()))
             (if (null? sigs) acc
               (loop (cdr sigs)
                     (append (cdr (car sigs)) acc)))))
         (high-set (unique high-idf-pkgs)))
    (context-from-alist
      (filter-map
        (lambda (fr)
          (let* ((name (nf fr 'name))
                 (refs (let ((r (nf fr 'refs))) (if r r '())))
                 (attrs
                   (let loop ((rs refs) (acc '()))
                     (if (null? rs) acc
                       (let* ((r (car rs))
                              (pkg (nf r 'pkg)))
                         (if (member pkg high-set)
                           (let ((objs (nf r 'objects)))
                             (loop (cdr rs)
                                   (append
                                     (map (lambda (o)
                                            (string-append pkg ":" o))
                                          (if objs objs '()))
                                     acc)))
                           (loop (cdr rs) acc)))))))
            (if (null? attrs) #f
              (cons name attrs))))
        func-refs))))

(define (set-difference a b)
  "Return elements in sorted list a not in sorted list b."
  (let loop ((xs a) (ys b) (acc '()))
    (cond ((null? xs) (reverse acc))
          ((null? ys) (append (reverse acc) xs))
          ((string<? (car xs) (car ys))
           (loop (cdr xs) ys (cons (car xs) acc)))
          ((string=? (car xs) (car ys))
           (loop (cdr xs) (cdr ys) acc))
          (else (loop xs (cdr ys) acc)))))

(define (sort-strings lst)
  "Sort a list of strings lexicographically (insertion sort with dedup)."
  (let loop ((xs lst) (acc '()))
    (if (null? xs) acc
      (loop (cdr xs) (set-add (car xs) acc)))))

(define (incomparable-concept-pairs concepts context)
  "Find all pairs of concepts that are lattice-incomparable."
  (let loop ((cs concepts) (acc '()))
    (if (null? cs) acc
      (let inner ((rest (cdr cs)) (acc acc))
        (if (null? rest)
          (loop (cdr cs) acc)
          (let* ((c1 (car cs))
                 (c2 (car rest))
                 (e1 (concept-extent c1))
                 (e2 (concept-extent c2)))
            (if (and (not (set-subset? e1 e2))
                     (not (set-subset? e2 e1)))
              (inner (cdr rest) (cons (cons c1 c2) acc))
              (inner (cdr rest) acc))))))))

(define (best-split-pair pairs context)
  "Select the pair that yields the most balanced partition.
Primary: maximize min(|e1|, |e2|) — prefer balanced groups.
Secondary: maximize coverage — prefer pairs that touch more objects."
  (if (null? pairs) #f
    (let loop ((ps pairs)
               (best #f) (best-bal -1) (best-cov -1))
      (if (null? ps) best
        (let* ((pair (car ps))
               (e1 (concept-extent (car pair)))
               (e2 (concept-extent (cdr pair)))
               (bal (min (length e1) (length e2)))
               (cov (length (set-union e1 e2))))
          (if (or (> bal best-bal)
                  (and (= bal best-bal) (> cov best-cov)))
            (loop (cdr ps) pair bal cov)
            (loop (cdr ps) best best-bal best-cov)))))))

(define (assign-remainder objs c1 c2 context)
  "Assign functions not in either concept by attribute affinity.
Returns (a-additions b-additions ambiguous)."
  (let ((i1 (concept-intent c1))
        (i2 (concept-intent c2)))
    (let loop ((os objs) (a '()) (b '()) (amb '()))
      (if (null? os) (list a b amb)
        (let* ((o (car os))
               (attrs (intent context (list o)))
               (a-overlap (length (set-intersect attrs i1)))
               (b-overlap (length (set-intersect attrs i2))))
          (cond ((> a-overlap b-overlap) (loop (cdr os) (cons o a) b amb))
                ((> b-overlap a-overlap) (loop (cdr os) a (cons o b) amb))
                (else (loop (cdr os) a b (cons o amb)))))))))

(define (find-split context lattice)
  "Find a two-way partition of functions minimizing cross-group coupling.
Uses the concept lattice to identify two large incomparable concepts,
then classifies each function by which concept's attributes it shares more.

Parameters:
  context : fca-context
  lattice : list — output from (concept-lattice context)
Returns: alist with keys: group-a, group-b, cut, cut-ratio
Category: goast-split

Examples:
  (define result (find-split ctx lat))
  (assoc 'group-a result)
  (assoc 'cut-ratio result)

See also: `build-package-context', `verify-acyclic'."
  (let* ((concepts (filter (lambda (c)
                             (> (length (concept-extent c)) 1))
                           lattice))
         (pairs (incomparable-concept-pairs concepts context))
         (best (best-split-pair pairs context)))
    (if (not best)
      '((group-a) (group-b) (cut) (cut-ratio . 1.0))
      (let* ((c1 (car best))
             (c2 (cdr best))
             (e1 (concept-extent c1))
             (e2 (concept-extent c2))
             (all-objs (context-objects context))
             (only-a (set-difference e1 e2))
             (only-b (set-difference e2 e1))
             (both (set-intersect e1 e2))
             (neither (set-difference
                        (set-difference all-objs e1) e2))
             (assigned (assign-remainder neither c1 c2 context))
             (group-a (append only-a (car assigned)))
             (group-b (append only-b (cadr assigned)))
             (cut-items (append both (caddr assigned)))
             (total (length all-objs))
             (ratio (if (zero? total) 1.0
                      (/ (length cut-items) total))))
        (list (cons 'group-a (sort-strings group-a))
              (cons 'group-b (sort-strings group-b))
              (cons 'cut (sort-strings cut-items))
              (cons 'cut-ratio (exact->inexact ratio)))))))

(define (verify-acyclic group-a group-b func-refs)
  "Check that a proposed split doesn't create Go import cycles.
Group A functions may depend on group B's package or vice versa, but not both.
Uses go-func-refs data to detect cross-references within the original package.

Parameters:
  group-a    : list — function names in group A
  group-b    : list — function names in group B
  func-refs  : list — raw output from (go-func-refs ...)
Returns: alist — (acyclic . #t/#f) (a-refs-b . count) (b-refs-a . count)
Category: goast-split

Examples:
  (verify-acyclic '(\"F1\" \"F2\") '(\"F3\" \"F4\") refs)
  ;; => ((acyclic . #t) (a-refs-b . 2) (b-refs-a . 0))

See also: `find-split', `recommend-split'."
  (let* ((pkg (if (null? func-refs) ""
               (nf (car func-refs) 'pkg)))
         (a-refs-b (count-internal-refs group-a group-b func-refs pkg))
         (b-refs-a (count-internal-refs group-b group-a func-refs pkg)))
    (list (cons 'acyclic (or (zero? a-refs-b) (zero? b-refs-a)))
          (cons 'a-refs-b a-refs-b)
          (cons 'b-refs-a b-refs-a))))

(define (count-internal-refs from-group to-group func-refs pkg)
  "Count how many functions in from-group reference identifiers belonging to to-group functions."
  (let loop ((frs func-refs) (count 0))
    (if (null? frs) count
      (let* ((fr (car frs))
             (name (nf fr 'name))
             (refs (let ((r (nf fr 'refs))) (if r r '()))))
        (if (not (member name from-group))
          (loop (cdr frs) count)
          (let ((has-internal
                  (let check ((rs refs))
                    (cond ((null? rs) #f)
                          ((equal? (nf (car rs) 'pkg) pkg)
                           (let ((objs (nf (car rs) 'objects)))
                             (if (and objs (any-member? objs to-group))
                               #t
                               (check (cdr rs)))))
                          (else (check (cdr rs)))))))
            (loop (cdr frs) (if has-internal (+ count 1) count))))))))

(define (any-member? items lst)
  "True if any element of items is a member of lst."
  (let loop ((is items))
    (cond ((null? is) #f)
          ((member (car is) lst) #t)
          (else (loop (cdr is))))))

(define (recommend-split func-refs . opts)
  "Analyze a package's functions and recommend a two-way split.
Computes IDF-weighted import signatures, builds FCA concept lattice,
finds min-cut partition, and verifies acyclicity.

Parameters:
  func-refs : list — output from (go-func-refs ...)
  opts      : optional — keyword options:
              'idf-threshold N (default 0.36)
              'refine         (use API-surface refinement)
Returns: alist with keys:
  functions  — total function count
  high-idf   — high-IDF packages with scores
  groups     — (find-split) result
  acyclic    — (verify-acyclic) result
  confidence — HIGH / MEDIUM / LOW / NONE
Category: goast-split

Examples:
  (define report (recommend-split (go-func-refs \"my/pkg\")))
  (assoc 'confidence report)

See also: `import-signatures', `find-split', `verify-acyclic'."
  (let* ((threshold (opt-ref opts 'idf-threshold 0.36))
         (refine? (memq 'refine opts))
         (sigs (import-signatures func-refs))
         (idf (compute-idf sigs))
         (filtered (filter-noise sigs idf threshold))
         (context (if refine?
                    (refine-by-api-surface func-refs filtered)
                    (build-package-context filtered)))
         (lattice (concept-lattice context))
         (groups (find-split context lattice))
         (group-a (cdr (assoc 'group-a groups)))
         (group-b (cdr (assoc 'group-b groups)))
         (acyclic-info (verify-acyclic group-a group-b func-refs))
         (high-idf-pkgs (filter (lambda (e) (>= (cdr e) threshold))
                                idf))
         (confidence (compute-confidence groups acyclic-info)))
    (list (cons 'functions (length func-refs))
          (cons 'high-idf high-idf-pkgs)
          (cons 'groups groups)
          (cons 'acyclic acyclic-info)
          (cons 'confidence confidence))))

(define (opt-ref opts key default)
  "Look up a keyword option: (opt-ref '(key1 val1 key2 val2) 'key1 #f) => val1."
  (let loop ((os opts))
    (cond ((null? os) default)
          ((and (not (null? (cdr os)))
                (eq? (car os) key))
           (cadr os))
          (else (loop (cdr os))))))

(define (compute-confidence groups acyclic-info)
  "Compute confidence level from split quality metrics."
  (let* ((cut-ratio (cdr (assoc 'cut-ratio groups)))
         (group-a (cdr (assoc 'group-a groups)))
         (group-b (cdr (assoc 'group-b groups)))
         (acyclic? (cdr (assoc 'acyclic acyclic-info)))
         (has-groups? (and (not (null? group-a))
                           (not (null? group-b)))))
    (cond ((not has-groups?) 'NONE)
          ((and acyclic? (<= cut-ratio 0.15)) 'HIGH)
          ((and acyclic? (<= cut-ratio 0.30)) 'MEDIUM)
          (else 'LOW))))
