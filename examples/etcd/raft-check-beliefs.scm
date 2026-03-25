;;; raft-check-beliefs.scm — Do raft functions guard values before use?
;;;
;;; Usage: cd etcd && wile-goast -f /path/to/raft-check-beliefs.scm

(import (wile goast belief))

;; Functions receiving a raftpb.Message should check msg.Type before processing.
(define-belief "raft-msg-type-guard"
  (sites (functions-matching (has-params "raftpb.Message")))
  (expect (checked-before-use "m"))
  (threshold 0.50 3))

(run-beliefs "go.etcd.io/raft/v3")
