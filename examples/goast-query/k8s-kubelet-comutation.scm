;;; k8s-kubelet-comutation.scm — Cross-package co-mutation beliefs for kubelet
;;;
;;; Usage: cd /path/to/kubernetes && wile-goast -f k8s-kubelet-comutation.scm
;;;
;;; Tests the multi-package belief context: struct types defined in
;;; core/v1 and lifecycle/ are mutated by functions across kubelet/.
;;; SSA-based beliefs need cross-package visibility to find these stores.
;;;
;;; Note: stores-to-fields uses Go's type system for struct matching.
;;; "PodCondition" matches only k8s.io/api/core/v1.PodCondition, not
;;; other structs that happen to have fields named Reason or Message.

(import (wile goast belief))

;; ── Belief 1: PodAdmitResult Admit+Reason+Message co-mutation ──
;; Convention: admission handlers set all three fields together.
;; Functions that store to Reason should also store to Admit and Message.

(define-belief "podadmitresult-reason-message"
  (sites (functions-matching
           (stores-to-fields "PodAdmitResult" "Reason" "Message")))
  (expect (co-mutated "Admit" "Reason" "Message"))
  (threshold 0.90 5))

;; ── Belief 2: PodCondition Reason+Message co-mutation ──────────
;; Convention: condition generators set both Reason and Message when
;; explaining why a condition has a particular status. Functions that
;; store to Reason should also store to Message.

(define-belief "podcondition-reason-message"
  (sites (functions-matching
           (stores-to-fields "PodCondition" "Reason")))
  (expect (co-mutated "Reason" "Message"))
  (threshold 0.66 3))

;; ── Belief 3: PodCondition full condition fields ───────────────
;; Stricter: Status, Type, Reason, Message should all be set together
;; for a complete condition update.

(define-belief "podcondition-full"
  (sites (functions-matching
           (stores-to-fields "PodCondition" "Status" "Reason")))
  (expect (co-mutated "Status" "Type" "Reason" "Message"))
  (threshold 0.66 3))

;; ── Run against all kubelet packages ─────────────────────

(run-beliefs "k8s.io/kubernetes/pkg/kubelet/...")
