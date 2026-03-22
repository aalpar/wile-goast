;;; k8s-kubelet-beliefs.scm — Consistency deviation detection for kubelet
;;;
;;; Usage: cd /path/to/kubernetes && wile-goast -f k8s-kubelet-beliefs.scm

(import (wile goast belief))

;; ── Belief 1: Instrumented methods record operations ─────
;; Convention: Methods on instrumentedRuntimeService should
;; call recordOperation and recordError consistently.

(define-belief "instrumented-records-operation"
  (sites (methods-of "instrumentedRuntimeService"))
  (expect (contains-call "recordOperation"))
  (threshold 0.80 3))

(define-belief "instrumented-records-error"
  (sites (methods-of "instrumentedRuntimeService"))
  (expect (contains-call "recordError"))
  (threshold 0.80 3))

;; ── Belief 2: Lock/Unlock pairing ────────────────────────
;; Convention: Functions that call Lock should also call Unlock.

(define-belief "lock-unlock-pairing"
  (sites (functions-matching (contains-call "Lock")))
  (expect (contains-call "Unlock"))
  (threshold 0.90 3))

(define-belief "rlock-runlock-pairing"
  (sites (functions-matching (contains-call "RLock")))
  (expect (contains-call "RUnlock"))
  (threshold 0.90 3))

;; ── Belief 3: Context propagation ────────────────────────
;; Convention: Functions that receive a context.Context parameter
;; should pass it to sub-calls (not drop it silently).
;; Heuristic: if you have ctx, you should call something that
;; also takes ctx — proxied by calling FromContext or using logger.

(define-belief "context-used"
  (sites (functions-matching (has-params "context.Context")))
  (expect (contains-call "FromContext" "WithContext" "WithCancel"
                         "WithTimeout" "WithDeadline" "WithValue"
                         "Err" "Done"))
  (threshold 0.70 5))

;; ── Belief 4: Error wrapping ─────────────────────────────
;; Convention: Functions that return errors should wrap them
;; with context (fmt.Errorf or similar).

(define-belief "error-wrapping"
  (sites (functions-matching (name-matches "sync")
                             (contains-call "Get")))
  (expect (contains-call "Errorf" "Wrapf" "WrapForeignErrorf"))
  (threshold 0.80 3))

;; ── Belief 5: Metric recording consistency ───────────────
;; Convention: Functions that record one metric (Inc, Observe)
;; should also record duration (ObserveSince, SinceInSeconds).

(define-belief "metrics-with-duration"
  (sites (functions-matching (contains-call "Inc")))
  (expect (contains-call "Observe" "ObserveSince" "SinceInSeconds"
                         "Since"))
  (threshold 0.70 5))

;; ── Run against all kubelet packages ─────────────────────

(run-beliefs "k8s.io/kubernetes/pkg/kubelet/...")
