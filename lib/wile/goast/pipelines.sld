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

;; (wile goast pipelines) — Phase 1 MCP pipeline procedures.
;;
;; Each pipeline wraps an already-implemented analysis and returns a
;; (version provenance result) envelope (see pipeline-envelope). The Go
;; handler in cmd/wile-goast/mcp_tools.go evaluates the pipeline and
;; marshals the returned alist to JSON via cmd/wile-goast/marshal.go.
;;
;; Exports grow task-by-task: each pipeline's export is added together
;; with its definition, so the library is loadable at every commit (an
;; R7RS library that exports an unbound identifier fails to instantiate).
(define-library (wile goast pipelines)
  (export
    ;; Shared envelope constructor (per-tool version supplied per call).
    pipeline-envelope
    ;; Shared result view (filter / rank / page / render). Exported so the
    ;; combinator is testable against synthetic rows, without either tool's
    ;; analysis in the way.
    apply-view
    view-sort
    view-name-matcher
    ;; Tool procedures (added per task alongside their definitions).
    pipeline-check-beliefs
    pipeline-discover-beliefs
    pipeline-recommend-split
    pipeline-recommend-boundaries
    pipeline-find-false-boundaries
    pipeline-find-duplicates
    pipeline-scheme-defs
    pipeline-scheme-symbols)
  (import
    (wile goast)
    (wile goast ssa)
    (wile goast belief)
    (wile goast split)
    (wile goast fca)
    (wile goast fca-algebra)
    (wile goast fca-recommend)
    (wile goast scmnav)
    (wile goast utils)       ; filter (pipeline-scheme-defs root validation)
    (srfi 13)               ; string-trim-both (pipeline-scheme-defs root splitting)
    ;; Discovery (duplicate-candidate-concepts, function-ref-context,
    ;; candidate-pairs, func-refs->positions) is run by the pipeline itself so
    ;; the scope/symbol prune and the pair-count guard both see the work list;
    ;; score-candidate-pairs is the scoring half it then drives.
    (only (wile goast dup-detect)
          find-scored-candidates candidate->verdict
          duplicate-candidate-concepts function-ref-context
          candidate-pairs func-refs->positions score-candidate-pairs)
    (only (wile goast provenance) finding-value finding-where finding-score))
  (include "pipelines.scm"))
