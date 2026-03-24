;;; raft-storage-consistency.scm — Interface behavioral consistency for raft.Storage
;;;
;;; Do all implementations of raft.Storage handle edge cases consistently?
;;; Uses interface-methods to compare how different types implement the same
;;; method, surfacing behavioral deviations (one impl handles an error,
;;; another doesn't).
;;;
;;; Usage: cd etcd && wile-goast -f /path/to/raft-storage-consistency.scm

(import (wile goast belief))

(define target "go.etcd.io/raft/v3/...")

;; Qualified name needed — rafttest also defines a Storage interface.
(define iface "go.etcd.io/raft/v3.Storage")

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
