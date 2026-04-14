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

;;; etcd-beliefs.scm — Consistency beliefs for etcd's storage layer
;;;
;;; Targets mvcc and storage packages where co-mutation patterns
;;; and field access consistency are most likely to reveal issues.
;;;
;;; Usage: cd etcd/server && wile-goast -f /path/to/etcd-beliefs.scm

(import (wile goast belief))

;; ── WAL: every Write should be paired with Close ──
(define-belief "wal-write-close"
  (sites (functions-matching (contains-call "Write")))
  (expect (contains-call "Close" "Sync"))
  (threshold 0.50 3))

;; ── Error logging: Warn/Error should include zap.Error ──
(define-belief "error-logging-context"
  (sites (functions-matching (contains-call "Warn" "Error")))
  (expect (contains-call "Error" "String"))
  (threshold 0.50 5))

;; ── Span lifecycle: Start should pair with End ──
(define-belief "span-start-end"
  (sites (functions-matching (contains-call "Start")))
  (expect (paired-with "Start" "End"))
  (threshold 0.70 3))

(run-beliefs "go.etcd.io/etcd/server/v3/etcdserver")
