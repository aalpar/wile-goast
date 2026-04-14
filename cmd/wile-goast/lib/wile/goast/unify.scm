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

;;; unify.scm — Structural diff and unification detection for Go AST/SSA
;;;
;;; Extracted from examples/goast-query/unify-detect-pkg.scm with added
;;; SSA-aware classification. Provides a pluggable classifier design:
;;; the core tree-diff-walk is generic; a classifier function determines
;;; how string diffs are categorized.

;; ══════════════════════════════════════════════════════════
;; Local utilities (not exported)
;; ══════════════════════════════════════════════════════════

(define (tagged-node? v)
  (and (pair? v) (symbol? (car v))))

(define (last-element lst)
  (if (null? (cdr lst)) (car lst)
    (last-element (cdr lst))))

(define (filter pred lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (loop (cdr xs)
            (if (pred (car xs)) (cons (car xs) acc) acc)))))

;; ══════════════════════════════════════════════════════════
;; Core data structures
;;
;; A diff result is a 3-element list: (shared diff-count entries)
;; where entries is a list of (category path val-a val-b).
;; ══════════════════════════════════════════════════════════

(define (merge-results a b)
  (list (+ (car a) (car b))
        (+ (cadr a) (cadr b))
        (append (caddr a) (caddr b))))

(define (shared-result)
  (list 1 0 '()))

(define (diff-result category path val-a val-b)
  (list 0 1 (list (list category path val-a val-b))))

(define (merge-all results)
  (let loop ((rs results) (acc (list 0 0 '())))
    (if (null? rs) acc
      (loop (cdr rs) (merge-results acc (car rs))))))

;; ══════════════════════════════════════════════════════════
;; Result accessors
;; ══════════════════════════════════════════════════════════

(define (diff-result-shared r)
  "Extract the shared-node count from a diff result.\n\nParameters:\n  r : list\nReturns: integer\nCategory: goast-unify\n\nSee also: `diff-result-diff-count', `diff-result-similarity'."
  (car r))
(define (diff-result-diff-count r)
  "Extract the diff count from a diff result.\n\nParameters:\n  r : list\nReturns: integer\nCategory: goast-unify\n\nSee also: `diff-result-shared', `diff-result-diffs'."
  (cadr r))
(define (diff-result-diffs r)
  "Extract the list of classified diffs from a diff result.\nEach diff is (category path val-a val-b).\n\nParameters:\n  r : list\nReturns: list\nCategory: goast-unify\n\nSee also: `diff-result-diff-count', `score-diffs'."
  (caddr r))

(define (diff-result-similarity r)
  "Extract the raw similarity ratio from a diff result.\nReturns shared/(shared+diffs) as an exact rational.\n\nParameters:\n  r : list\nReturns: number\nCategory: goast-unify\n\nSee also: `score-diffs', `unifiable?'."
  (let ((total (+ (car r) (cadr r))))
    (if (> total 0)
      (exact->inexact (/ (car r) total))
      1.0)))

;; ══════════════════════════════════════════════════════════
;; AST classifier — path-based
;; ══════════════════════════════════════════════════════════

(define ast-type-fields '(type inferred-type asserted-type obj-pkg signature))
(define ast-identifier-fields '(name sel label))

(define (in-type-position? path)
  (let loop ((xs path))
    (cond
      ((null? xs) #f)
      ((null? (cdr xs)) #f)
      ((and (symbol? (car xs)) (memq (car xs) ast-type-fields)) #t)
      (else (loop (cdr xs))))))

(define (classify-ast-diff tag field str-a str-b path)
  "Classify an AST diff by path position.\nReturns a category symbol: 'type, 'identifier, 'literal, or 'structural.\n\nParameters:\n  tag : symbol\n  field : symbol\n  str-a : string\n  str-b : string\n  path : list\nReturns: symbol\nCategory: goast-unify\n\nSee also: `ast-diff', `classify-ssa-diff'."
  (cond
    ((and (symbol? field) (memq field ast-type-fields))        'type-name)
    ((and (symbol? field) (eq? field 'name)
          (in-type-position? path))                            'type-name)
    ((and (symbol? field) (memq field ast-identifier-fields))  'identifier)
    ((and (symbol? field) (eq? field 'value))                  'literal-value)
    ((and (symbol? field) (eq? field 'tok))                    'operator)
    (else                                                      'identifier)))

;; ══════════════════════════════════════════════════════════
;; SSA classifier — tag-based
;; ══════════════════════════════════════════════════════════

(define ssa-identity-name-tags '(ssa-func ssa-param))

(define (classify-ssa-diff tag field str-a str-b path)
  "Classify an SSA diff by node tag.\nReturns a category symbol: 'type, 'register, or 'structural.\n\nParameters:\n  tag : symbol\n  field : symbol\n  str-a : string\n  str-b : string\n  path : list\nReturns: symbol\nCategory: goast-unify\n\nSee also: `ssa-diff', `classify-ast-diff'."
  (cond
    ((and (symbol? field) (memq field '(type asserted-type)))  'type-name)
    ((and (symbol? field) (eq? field 'op))                     'operator)
    ((and (symbol? field) (memq field '(func method)))         'call-target)
    ((and (symbol? field) (memq field '(index preds succs idom then else target)))
                                                               'structural)
    ((and (symbol? field) (eq? field 'name)
          (memq tag ssa-identity-name-tags))                   'identifier)
    ((and (symbol? field) (eq? field 'name))                   'register)
    (else                                                      'identifier)))

;; ══════════════════════════════════════════════════════════
;; Diff engine
;;
;; tree-diff-walk is the recursive core. It takes:
;;   node-a, node-b — the two trees to compare
;;   parent-tag — the tag of the enclosing node (for classifier)
;;   path — accumulated field keys
;;   classifier — (tag field str-a str-b path) -> category
;; ══════════════════════════════════════════════════════════

(define (fields-diff fields-a fields-b parent-tag path classifier)
  (let ((results
          (filter-map
            (lambda (pair-a)
              (if (not (pair? pair-a)) #f
                (let* ((key (car pair-a))
                       (val-a (cdr pair-a))
                       (entry-b (assoc key fields-b)))
                  (if entry-b
                    (tree-diff-walk val-a (cdr entry-b) parent-tag
                                   (append path (list key)) classifier)
                    (diff-result 'missing-field
                                 (append path (list key))
                                 val-a #f)))))
            fields-a)))
    (let ((extra
            (filter-map
              (lambda (pair-b)
                (if (not (pair? pair-b)) #f
                  (let ((key (car pair-b)))
                    (if (assoc key fields-a) #f
                      (diff-result 'extra-field
                                   (append path (list key))
                                   #f (cdr pair-b))))))
              fields-b)))
      (merge-all (append results extra)))))

(define (list-diff lst-a lst-b parent-tag path idx classifier)
  (cond
    ((and (null? lst-a) (null? lst-b))
     (list 0 0 '()))
    ((null? lst-b)
     (merge-all
       (map (lambda (ea)
              (diff-result 'extra-element
                           (append path (list idx))
                           ea #f))
            lst-a)))
    ((null? lst-a)
     (merge-all
       (map (lambda (eb)
              (diff-result 'extra-element
                           (append path (list idx))
                           #f eb))
            lst-b)))
    (else
      (merge-results
        (tree-diff-walk (car lst-a) (car lst-b) parent-tag
                        (append path (list idx)) classifier)
        (list-diff (cdr lst-a) (cdr lst-b) parent-tag path (+ idx 1) classifier)))))

(define (tree-diff-walk node-a node-b parent-tag path classifier)
  (cond
    ((and (tagged-node? node-a) (tagged-node? node-b))
     (if (eq? (car node-a) (car node-b))
       (merge-results
         (shared-result)
         (fields-diff (cdr node-a) (cdr node-b) (car node-a) path classifier))
       (diff-result 'structural path node-a node-b)))

    ((and (list? node-a) (list? node-b))
     (list-diff node-a node-b parent-tag path 0 classifier))

    ((and (eq? node-a #f) (eq? node-b #f))
     (shared-result))

    ((equal? node-a node-b)
     (shared-result))

    ((and (string? node-a) (string? node-b))
     (diff-result (classifier parent-tag
                              (if (null? path) #f (last-element path))
                              node-a node-b path)
                  path node-a node-b))

    ((and (symbol? node-a) (symbol? node-b))
     (diff-result 'operator path node-a node-b))

    ((and (boolean? node-a) (boolean? node-b))
     (diff-result 'literal-value path node-a node-b))

    ((and (number? node-a) (number? node-b))
     (diff-result 'literal-value path node-a node-b))

    (else
      (diff-result 'structural path node-a node-b))))

;; ══════════════════════════════════════════════════════════
;; Convenience wrappers
;; ══════════════════════════════════════════════════════════

(define (tree-diff node-a node-b classifier)
  "Generic structural diff of two s-expression trees.\nCLASSIFIER categorizes each leaf difference. Returns a diff-result\nwith shared count, diff count, and classified diff list.\n\nParameters:\n  node-a : list\n  node-b : list\n  classifier : procedure\nReturns: list\nCategory: goast-unify\n\nSee also: `ast-diff', `ssa-diff'."
  (tree-diff-walk node-a node-b #f '() classifier))

(define (ast-diff node-a node-b)
  "Diff two AST nodes using path-based classification.\nConvenience wrapper around tree-diff with classify-ast-diff.\n\nParameters:\n  node-a : list\n  node-b : list\nReturns: list\nCategory: goast-unify\n\nExamples:\n  (ast-diff func-a func-b)\n\nSee also: `ssa-diff', `tree-diff', `unifiable?'."
  (tree-diff node-a node-b classify-ast-diff))

(define (ssa-diff node-a node-b)
  "Diff two SSA nodes using tag-based classification.\nConvenience wrapper around tree-diff with classify-ssa-diff.\n\nParameters:\n  node-a : list\n  node-b : list\nReturns: list\nCategory: goast-unify\n\nExamples:\n  (ssa-diff (go-ssa-canonicalize fn-a) (go-ssa-canonicalize fn-b))\n\nSee also: `ast-diff', `tree-diff', `unifiable?'."
  (tree-diff node-a node-b classify-ssa-diff))

;; ══════════════════════════════════════════════════════════
;; Substitution collapsing
;;
;; Type annotations propagate root type substitutions into
;; every sub-expression. A single root change generates
;; dozens of inferred-type diffs. We collapse by finding
;; root substitutions and reclassifying derived ones.
;; ══════════════════════════════════════════════════════════

(define (string-replace-all str old new)
  (let ((old-len (string-length old))
        (str-len (string-length str)))
    (if (or (= old-len 0) (< str-len old-len))
      str
      (let loop ((start 0) (parts '()))
        (let search ((i start))
          (cond
            ((> (+ i old-len) str-len)
             (apply string-append
                    (reverse (cons (substring str start str-len) parts))))
            ((string=? (substring str i (+ i old-len)) old)
             (loop (+ i old-len)
                   (cons new (cons (substring str start i) parts))))
            (else
             (search (+ i 1)))))))))

(define (apply-substitutions str roots)
  (let loop ((s str) (rs roots))
    (if (null? rs) s
      (loop (string-replace-all s (caar rs) (cdar rs))
            (cdr rs)))))

(define (derivable? val-a val-b roots)
  (equal? (apply-substitutions val-a roots) val-b))

(define (sort-by-length pairs)
  (define (insert p sorted)
    (cond
      ((null? sorted) (list p))
      ((<= (string-length (car p)) (string-length (caar sorted)))
       (cons p sorted))
      (else (cons (car sorted) (insert p (cdr sorted))))))
  (let loop ((ps pairs) (acc '()))
    (if (null? ps) acc
      (loop (cdr ps) (insert (car ps) acc)))))

(define (find-root-substitutions pairs)
  "Find root substitution pairs from a list of (val-a . val-b) diffs.\nRoot substitutions are the minimal set from which others are derivable.\n\nParameters:\n  pairs : list\nReturns: list\nCategory: goast-unify\n\nSee also: `collapse-diffs', `score-diffs'."
  (let ((sorted (sort-by-length (unique pairs))))
    (let loop ((ps sorted) (roots '()))
      (if (null? ps) roots
        (let ((a (caar ps)) (b (cdar ps)))
          (if (derivable? a b roots)
            (loop (cdr ps) roots)
            (loop (cdr ps) (cons (cons a b) roots))))))))

(define (collapse-diffs diffs roots)
  "Remove diffs that are derivable from root substitutions.\nA diff is derivable if applying the root substitutions to val-a\nproduces val-b.\n\nParameters:\n  diffs : list\n  roots : list\nReturns: list\nCategory: goast-unify\n\nSee also: `find-root-substitutions', `score-diffs'."
  (map (lambda (d)
         (if (and (eq? (car d) 'type-name)
                  (string? (caddr d))
                  (string? (cadddr d))
                  (derivable? (caddr d) (cadddr d) roots))
           (cons 'derived-type (cdr d))
           d))
       diffs))

;; ══════════════════════════════════════════════════════════
;; Scoring
;; ══════════════════════════════════════════════════════════

(define diff-weights
  '((type-name . 1)
    (derived-type . 0)
    (literal-value . 1)
    (identifier . 0)
    (register . 0)
    (operator . 2)
    (call-target . 3)
    (structural . 100)
    (missing-field . 50)
    (extra-field . 50)
    (extra-element . 50)))

(define (diff-weight category)
  (let ((entry (assoc category diff-weights)))
    (if entry (cdr entry) 10)))

(define (score-diffs shared-count diff-count diffs)
  "Compute effective similarity with substitution collapsing.\nFinds root substitutions, collapses derivable diffs, and returns\nthe adjusted similarity as a weighted score.\n\nParameters:\n  shared-count : integer\n  diff-count : integer\n  diffs : list\nReturns: number\nCategory: goast-unify\n\nSee also: `find-root-substitutions', `collapse-diffs', `unifiable?'."
  (let* ((type-pairs
           (filter-map
             (lambda (d)
               (and (eq? (car d) 'type-name)
                    (string? (caddr d))
                    (string? (cadddr d))
                    (cons (caddr d) (cadddr d))))
             diffs))
         (roots (find-root-substitutions type-pairs))
         (collapsed (collapse-diffs diffs roots))
         (total (+ shared-count diff-count))
         (derived-count (length (filter (lambda (d)
                                          (eq? (car d) 'derived-type))
                                        collapsed)))
         (effective-shared (+ shared-count derived-count))
         (effective-diffs (- diff-count derived-count))
         (effective-total (+ effective-shared effective-diffs))
         (raw-similarity (if (> total 0)
                           (exact->inexact (/ shared-count total))
                           0.0))
         (effective-similarity (if (> effective-total 0)
                                 (exact->inexact
                                   (/ effective-shared effective-total))
                                 0.0))
         (weighted-cost
           (apply + (map (lambda (d) (diff-weight (car d))) collapsed)))
         (value-diffs
           (filter-map
             (lambda (d)
               (and (eq? (car d) 'literal-value)
                    (cons (caddr d) (cadddr d))))
             collapsed))
         (unique-value-params (unique value-diffs)))
    (list raw-similarity
          effective-similarity
          (+ (length roots) (length unique-value-params))
          weighted-cost
          roots
          unique-value-params
          collapsed
          derived-count)))

;; ══════════════════════════════════════════════════════════
;; Verdict predicate
;; ══════════════════════════════════════════════════════════

(define (unifiable? result threshold)
  "Determine whether two nodes are similar enough to unify.\nReturns #t when effective similarity >= THRESHOLD and all remaining\ndiffs are type/register (not structural).\n\nParameters:\n  result : list\n  threshold : number\nReturns: boolean\nCategory: goast-unify\n\nExamples:\n  (unifiable? (ast-diff fn-a fn-b) 0.85)\n\nSee also: `ast-diff', `ssa-diff', `score-diffs'."
  (let* ((shared-count (diff-result-shared result))
         (diff-count (diff-result-diff-count result))
         (diffs (diff-result-diffs result))
         (score (score-diffs shared-count diff-count diffs))
         (eff-sim (list-ref score 1))
         (collapsed (list-ref score 6))
         (non-type-diffs
           (filter-map
             (lambda (d)
               (and (not (memq (car d) '(type-name derived-type identifier register)))
                    d))
             collapsed)))
    (and (>= eff-sim threshold)
         (null? non-type-diffs))))

;; ══════════════════════════════════════════════════════════
;; Algebraic equivalence (v2)
;;
;; Uses discover-equivalences from (wile algebra symbolic) to
;; check if two SSA binop nodes normalize to the same form
;; under any sub-theory of ssa-theory.
;;
;; Normal forms may differ in register names (the 'name field
;; of ssa-binop nodes is an identity, not algebraic content).
;; We compare via ssa-diff, accepting forms where all diffs
;; are register-class.
;; ══════════════════════════════════════════════════════════

(define (ssa-form-equal? a b)
  "Compare two SSA normal forms ignoring register names.
Returns #t when ssa-diff shows only register/identifier diffs."
  (if (equal? a b) #t
    (let* ((r (ssa-diff a b))
           (diffs (diff-result-diffs r)))
      (let loop ((ds diffs))
        (cond ((null? ds) #t)
              ((memq (caar ds) '(register identifier)) (loop (cdr ds)))
              (else #f))))))

(define ssa-equivalent?
  (case-lambda
    ((node-a node-b)
     (ssa-equivalent* node-a node-b ssa-theory ssa-binop-protocol))
    ((node-a node-b theory proto)
     (ssa-equivalent* node-a node-b theory proto))))

(define (ssa-equivalent* node-a node-b theory proto)
  (let ((forms-a (map car (discover-equivalences theory proto node-a)))
        (forms-b (map car (discover-equivalences theory proto node-b))))
    (let outer ((fa forms-a))
      (cond ((null? fa) #f)
            ((let inner ((fb forms-b))
               (cond ((null? fb) #f)
                     ((ssa-form-equal? (car fa) (car fb)) #t)
                     (else (inner (cdr fb)))))
             #t)
            (else (outer (cdr fa)))))))
