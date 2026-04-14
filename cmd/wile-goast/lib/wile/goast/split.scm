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
