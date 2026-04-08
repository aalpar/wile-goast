;;; auth-gate-consistency.scm — Auth enforcement across three layers
;;;
;;; etcd enforces authorization at three architectural layers,
;;; each with a different mechanism:
;;;
;;;   Layer 1 — RPC decorator: authMaintenanceServer wraps every
;;;             maintenance method with isPermitted() (admin check)
;;;
;;;   Layer 2 — Apply: authApplierV3 checks per-operation permissions
;;;             during raft apply (Put->IsPutPermitted, Range->IsRangePermitted,
;;;             Txn->CheckTxnAuth, etc.)
;;;
;;;   Layer 3 — v3_server: read ops check auth via doSerialize(),
;;;             write ops carry auth info through processInternalRaftRequestOnce(),
;;;             lease ops check inline via requireAuthInfo/checkLease*
;;;
;;; KV operations (Range, Put, DeleteRange, Txn) do NOT check auth at
;;; the RPC handler level — that is by design.  Reads check auth in
;;; doSerialize(); writes serialize auth info into the raft proposal
;;; header and check at apply time.
;;;
;;; This script detects drift: a new method added to any layer
;;; without the corresponding auth check.
;;;
;;; Usage: cd etcd/server && wile-goast -f auth-gate-consistency.scm

(import (wile goast belief))

(define target "go.etcd.io/etcd/server/v3/...")

;; ── Layer 1: RPC auth decorator ──────────────────────────
;;
;; authMaintenanceServer wraps maintenanceServer.  Every method
;; must call isPermitted() before delegating.  A method without
;; isPermitted is a privilege escalation — maintenance operations
;; (Defragment, Snapshot, Hash, Alarm, etc.) are admin-only.

(define-belief "maintenance-auth-decorator"
  (sites (methods-of "authMaintenanceServer"))
  (expect (contains-call "isPermitted"))
  (threshold 0.95 5))

;; ── Layer 2: Apply-layer auth ────────────────────────────
;;
;; authApplierV3 wraps the base applierV3.  Each method that
;; handles a mutating operation should call a permission check.
;; Apply() checks needAdminPermission/IsAdminPermitted for
;; admin-only ops; individual methods check per-key permissions.

(define auth-permission-check
  (contains-call "IsPutPermitted" "IsRangePermitted"
                 "IsDeleteRangePermitted" "IsAdminPermitted"
                 "CheckTxnAuth" "checkLeasePuts"
                 "checkLeasePutsKeys" "needAdminPermission"
                 "HasRole"))

(define-belief "apply-auth-check"
  (sites (methods-of "authApplierV3"))
  (expect auth-permission-check)
  (threshold 0.80 5))

;; ── Layer 3a: Read-path auth (doSerialize) ───────────────
;;
;; Read operations on EtcdServer route through doSerialize(),
;; which accepts an auth-check closure.  The calling function
;; must reference a permission function for that closure.
;; Currently: Range passes IsRangePermitted, read-only Txn
;; passes CheckTxnAuth.

(define-belief "read-path-auth"
  (sites (functions-matching (has-receiver "EtcdServer")
                             (contains-call "doSerialize")))
  (expect (contains-call "IsRangePermitted" "CheckTxnAuth"
                         "IsAdminPermitted"))
  (threshold 1.0 1))

;; ── Layer 3b: Write-path auth pipeline ───────────────────
;;
;; processInternalRaftRequestOnce extracts auth info from the
;; gRPC context and serializes it into the raft entry header.
;; If this function stops calling AuthInfoFromCtx, ALL write
;; operations lose auth.  Single-site assertion.

(define-belief "raft-proposal-carries-auth"
  (sites (functions-matching (name-matches "processInternalRaftRequestOnce")
                             (has-receiver "EtcdServer")))
  (expect (contains-call "AuthInfoFromCtx"))
  (threshold 1.0 1))

;; ── Layer 3c: Lease-specific auth ────────────────────────
;;
;; Lease methods on EtcdServer use a mix of auth mechanisms:
;;   LeaseGrant      -> requireAuthInfo (user exists)
;;   LeaseRevoke     -> requireAuthInfo (user exists)
;;   LeaseRenew      -> checkLeaseRenew (IsPutPermitted per key)
;;   LeaseTimeToLive -> requireAuthInfo + checkLeaseTimeToLive
;;   LeaseLeases     -> checkLeaseLeases (AuthInfoFromCtx)
;;   checkLease*     -> AuthInfoFromCtx directly
;;
;; Excludes internal functions that don't serve external requests:
;;   LeaseHandler       — HTTP handler registrar
;;   revokeExpiredLeases — timer-driven internal cleanup
;;   leaseTimeToLive    — private helper (auth checked by caller)

(define lease-auth-evidence
  (contains-call "requireAuthInfo" "checkLeaseRenew"
                 "checkLeaseTimeToLive" "checkLeaseLeases"
                 "AuthInfoFromCtx"))

(define not-lease-internal
  (none-of (name-matches "Handler")
           (name-matches "revokeExpired")
           (name-matches "leaseTimeToLive")))

(define-belief "lease-auth"
  (sites (functions-matching (has-receiver "EtcdServer")
                             (name-matches "Lease")
                             not-lease-internal))
  (expect lease-auth-evidence)
  (threshold 0.95 3))

(run-beliefs target)
