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

(define-library (wile goast unify)
  (export
    tree-diff ast-diff ssa-diff
    classify-ast-diff classify-ssa-diff
    diff-result-similarity diff-result-diffs diff-result-shared diff-result-diff-count
    score-diffs find-root-substitutions collapse-diffs
    unifiable?
    ;; v2: algebraic equivalence
    ssa-equivalent?)
  (import (wile goast utils)
          (wile algebra symbolic)
          (wile goast ssa-normalize))
  (include "unify.scm"))
