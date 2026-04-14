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

;;; mvcc-beliefs.scm — Consistency beliefs for etcd's mvcc package
;;;
;;; The mvcc package is the core key-value store — heavily concurrent,
;;; with watchers, transactions, and compaction. Rich locking patterns.
;;;
;;; Usage: cd etcd/server && wile-goast -f /path/to/mvcc-beliefs.scm

(import (wile goast belief))

;; ── Lock/Unlock pairing ──
(define-belief "lock-unlock"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.50 3))

(define-belief "rlock-runlock"
  (sites (functions-matching (contains-call "RLock")))
  (expect (paired-with "RLock" "RUnlock"))
  (threshold 0.50 3))

;; ── Every method that calls Lock should also call Unlock ──
(define-belief "lock-has-unlock"
  (sites (functions-matching (contains-call "Lock")))
  (expect (contains-call "Unlock"))
  (threshold 0.90 5))

;; ── RLock must have RUnlock ──
(define-belief "rlock-has-runlock"
  (sites (functions-matching (contains-call "RLock")))
  (expect (contains-call "RUnlock"))
  (threshold 0.90 5))

;; ── Transaction lifecycle: End after Begin ──
(define-belief "txn-end"
  (sites (functions-matching (contains-call "ReadTx" "BatchTx")))
  (expect (contains-call "End" "Unlock" "Commit"))
  (threshold 0.60 3))

(run-beliefs "go.etcd.io/etcd/server/v3/storage/mvcc")
