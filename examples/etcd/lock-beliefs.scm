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
