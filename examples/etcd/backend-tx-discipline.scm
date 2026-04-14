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

;;; backend-tx-discipline.scm — Backend transaction lifecycle consistency
;;;
;;; etcd's backend wraps bbolt — consumers never touch bolt.Tx directly.
;;; The transaction lifecycle is expressed through lock discipline:
;;;
;;;   LockInsideApply / LockOutsideApply  →  Unlock
;;;   ConcurrentReadTx()                  →  RUnlock  (decrements txWg)
;;;
;;; This script checks four properties across all server packages:
;;;
;;;   1. LockInsideApply paired with Unlock on all CFG paths
;;;   2. LockOutsideApply paired with Unlock on all CFG paths
;;;   3. ConcurrentReadTx callers must call RUnlock (prevents txWg leak)
;;;   4. Unsafe* callers hold a lock (documented precondition)
;;;
;;; Usage: cd etcd/server && wile-goast -f backend-tx-discipline.scm

(import (wile goast belief))

(define target "go.etcd.io/etcd/server/v3/...")

;; Exclude the backend package's own Lock/Unlock implementations
(define not-backend-impl
  (none-of (name-matches "lock")
           (name-matches "Lock")
           (name-matches "Unlock")
           (name-matches "RLock")
           (name-matches "RUnlock")
           (name-matches "commit")
           (name-matches "Commit")))

;; ── 1. Apply-path lock discipline ────────────────────────
;;
;; LockInsideApply is for the raft state machine apply path.
;; Every acquire must reach Unlock on all control flow paths.
(define-belief "apply-lock-unlock"
  (sites (functions-matching not-backend-impl
                             (contains-call "LockInsideApply")))
  (expect (paired-with "LockInsideApply" "Unlock"))
  (threshold 0.50 5))

;; ── 2. External-path lock discipline ─────────────────────
;;
;; LockOutsideApply is for operations outside raft apply.
;; Same pairing requirement.
(define-belief "external-lock-unlock"
  (sites (functions-matching not-backend-impl
                             (contains-call "LockOutsideApply")))
  (expect (paired-with "LockOutsideApply" "Unlock"))
  (threshold 0.50 3))

;; ── 3. ConcurrentReadTx cleanup ──────────────────────────
;;
;; ConcurrentReadTx() increments txWg. The only decrement is
;; concurrentReadTx.RUnlock(). Missing RUnlock = txWg never
;; reaches zero = read tx never rolled back = resource leak.
(define-belief "concurrent-readtx-cleanup"
  (sites (functions-matching not-backend-impl
                             (contains-call "ConcurrentReadTx")))
  (expect (contains-call "RUnlock"))
  (threshold 0.50 1))

;; ── 4. Unsafe* callers hold a lock ───────────────────────
;;
;; UnsafePut, UnsafeRange, UnsafeDelete, UnsafeForEach are
;; documented as "must be called holding the lock."
;;
;; Using callers-of (call graph layer) rather than
;; functions-matching so we check the actual callers through
;; interface dispatch, not just AST name matching.

(define lock-evidence
  (contains-call "Lock" "LockInsideApply" "LockOutsideApply" "RLock"))

(define-belief "unsafe-put-under-lock"
  (sites (callers-of "UnsafePut"))
  (expect lock-evidence)
  (threshold 0.80 5))

(define-belief "unsafe-delete-under-lock"
  (sites (callers-of "UnsafeDelete"))
  (expect lock-evidence)
  (threshold 0.80 5))

(define-belief "unsafe-range-under-lock"
  (sites (callers-of "UnsafeRange"))
  (expect lock-evidence)
  (threshold 0.80 5))

(define-belief "unsafe-foreach-under-lock"
  (sites (callers-of "UnsafeForEach"))
  (expect lock-evidence)
  (threshold 0.80 5))

(run-beliefs target)
