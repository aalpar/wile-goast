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
