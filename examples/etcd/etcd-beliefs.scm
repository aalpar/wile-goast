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
