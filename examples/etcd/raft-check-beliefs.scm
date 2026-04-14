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

;;; raft-check-beliefs.scm — Do raft functions guard values before use?
;;;
;;; Usage: cd etcd/raft && wile-goast -f /path/to/raft-check-beliefs.scm

(import (wile goast belief))

;; Functions receiving a raftpb.Message should check msg.Type before processing.
(define-belief "raft-msg-type-guard"
  (sites (functions-matching (has-params "raftpb.Message")))
  (expect (checked-before-use "m"))
  (threshold 0.50 3))

(run-beliefs "go.etcd.io/etcd/raft/v3")
