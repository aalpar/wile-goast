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
    pipeline-envelope)
  (import
    (wile goast))
  (include "pipelines.scm"))
