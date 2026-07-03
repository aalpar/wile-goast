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

(define-library (wile goast dataflow)
  (export
    ;; (The {#f,#t} guard lattice moved to (wile algebra) as
    ;;  `two-point-lattice'; this module now consumes it.)
    ;; SSA-specific helpers
    ssa-all-instrs
    ssa-instruction-names
    make-reachability-transfer
    block-instrs
    ;; SSA adapter for the generic MFP solver in (wile algebra dataflow)
    ssa-cfg-protocol
    ;; SSA-specific analysis entry point
    defuse-reachable?
    ;; Aggregate-aware value-flow reachability (sees value-through-variadic-slice
    ;; flow that defuse-reachable? misses; backs the flows-to-all checker)
    value-flow-reached build-addr-aggregate-map
    ;; Re-exports of the extracted solver (callers may still import
    ;; them from (wile goast dataflow) directly).
    run-analysis
    analysis-in
    analysis-out
    analysis-states)
  (import (wile algebra)
          (wile goast utils))
  (include "dataflow.scm"))
