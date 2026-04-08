;;; lock-sweep.scm — Lock/Unlock consistency sweep across etcd server
;;;
;;; Scans all packages under go.etcd.io/etcd/server/v3 for:
;;;   1. Lock without paired Unlock (missing cleanup)
;;;   2. RLock without paired RUnlock
;;;   3. Lock without any Unlock call (stronger: not even present)
;;;   4. RLock without any RUnlock call
;;;
;;; Filters out Lock/Unlock *implementations* (methods named Lock, Unlock,
;;; etc.) and gRPC generated handlers to reduce false positives.
;;;
;;; Usage: cd etcd/server && wile-goast -f lock-sweep.scm

(import (wile goast belief))

;; Exclude functions that ARE lock methods or gRPC handlers
(define not-lock-impl
  (none-of (name-matches "Lock")
           (name-matches "Unlock")
           (name-matches "RLock")
           (name-matches "RUnlock")
           (name-matches "_Handler")
           (name-matches "request_")))

;; ── Pairing: Lock must pair with Unlock on all CFG paths ──
(define-belief "lock-unlock-paired"
  (sites (functions-matching not-lock-impl (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.80 5))

;; ── Pairing: RLock must pair with RUnlock on all CFG paths ──
(define-belief "rlock-runlock-paired"
  (sites (functions-matching not-lock-impl (contains-call "RLock")))
  (expect (paired-with "RLock" "RUnlock"))
  (threshold 0.80 5))

;; ── Presence: Lock without Unlock anywhere in the function ──
(define-belief "lock-has-unlock"
  (sites (functions-matching not-lock-impl (contains-call "Lock")))
  (expect (contains-call "Unlock"))
  (threshold 0.90 5))

;; ── Presence: RLock without RUnlock anywhere in the function ──
(define-belief "rlock-has-runlock"
  (sites (functions-matching not-lock-impl (contains-call "RLock")))
  (expect (contains-call "RUnlock"))
  (threshold 0.90 5))

(run-beliefs "go.etcd.io/etcd/server/v3/...")
