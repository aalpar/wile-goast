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

;;; Provenance: resolve SSA instructions to source positions.
;;;
;;; The SSA mapper injects a (pos . "file:line:col") field per instruction when
;;; go-ssa-build is called with the 'positions option (goastssa/mapper.go). The
;;; field is omitted for synthetic/positionless instructions. These accessors
;;; surface that position so analyses stop discarding the provenance they hold.

;; ssa-instr-pos: the resolved source position of an SSA instruction node, or
;; #f when the instruction carries no position. The string? guard normalizes
;; any non-string value (e.g. nf's #f on a missing field) to #f, giving callers
;; a clean #f / string? contract.
(define (ssa-instr-pos instr)
  (let ((p (nf instr 'pos)))
    (if (string? p) p #f)))
