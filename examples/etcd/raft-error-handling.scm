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
