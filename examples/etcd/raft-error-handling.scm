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

;;; raft-error-handling.scm — Do callers of raft.Step handle errors consistently?
;;;
;;; Usage: cd etcd/raft && wile-goast -f /path/to/raft-error-handling.scm

(import (wile goast belief))

;; All callers of Step should check the returned error.
(define-belief "step-error-handling"
  (sites (callers-of "Step"))
  (expect (contains-call "Errorf" "Error" "Fatalf" "Warn"))
  (threshold 0.50 3))

(run-beliefs "go.etcd.io/etcd/raft/v3/...")
