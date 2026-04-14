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
