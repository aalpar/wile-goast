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

;;; raft-storage-consistency.scm — Interface behavioral consistency for raft.Storage
;;;
;;; Do all implementations of raft.Storage handle edge cases consistently?
;;; Uses interface-methods to compare how different types implement the same
;;; method, surfacing behavioral deviations (one impl handles an error,
;;; another doesn't).
;;;
;;; Usage: cd etcd/raft && wile-goast -f /path/to/raft-storage-consistency.scm

(import (wile goast belief))

(define target "go.etcd.io/etcd/raft/v3/...")

;; Qualified name needed — rafttest also defines a Storage interface.
(define iface "go.etcd.io/etcd/raft/v3.Storage")

;; Every Entries() implementation should guard against compacted log
(define-belief "entries-compaction-guard"
  (sites (interface-methods iface "Entries"))
  (expect (contains-call "ErrCompacted" "ErrUnavailable"))
  (threshold 0.80 2))

;; Every Snapshot() should handle ErrSnapshotTemporarilyUnavailable
(define-belief "snapshot-temp-unavail"
  (sites (interface-methods iface "Snapshot"))
  (expect (contains-call "ErrSnapshotTemporarilyUnavailable"))
  (threshold 0.60 2))

;; Every Term() implementation should handle out-of-range indices
(define-belief "term-bounds-check"
  (sites (interface-methods iface "Term"))
  (expect (contains-call "ErrCompacted" "ErrUnavailable"))
  (threshold 0.80 2))

;; Broader: do Storage implementors consistently close/release resources?
(define-belief "storage-resource-cleanup"
  (sites (implementors-of iface))
  (expect (contains-call "Close" "Release" "Unlock"))
  (threshold 0.50 3))

(run-beliefs target)
