;;; lock-beliefs.scm — Lock/Unlock consistency beliefs for etcd
;;;
;;; Usage: cd etcd/server && wile-goast -f /path/to/lock-beliefs.scm

(import (wile goast belief))

;; Lock must be paired with Unlock
(define-belief "lock-unlock-pairing"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.50 3))

;; RLock must be paired with RUnlock
(define-belief "rlock-runlock-pairing"
  (sites (functions-matching (contains-call "RLock")))
  (expect (paired-with "RLock" "RUnlock"))
  (threshold 0.50 3))

;; Lock should happen before Unlock (ordering)
(define-belief "lock-before-unlock"
  (sites (functions-matching (all-of (contains-call "Lock")
                                      (contains-call "Unlock"))))
  (expect (ordered "Lock" "Unlock"))
  (threshold 0.80 3))

(run-beliefs "go.etcd.io/etcd/server/v3/etcdserver")
