;;; k8s-controller-beliefs.scm — Consistency deviation detection for k8s controllers
;;;
;;; Usage: cd /path/to/kubernetes && wile-goast -f k8s-controller-beliefs.scm

(import (wile goast belief))

;; ── Belief 1: KeyFunc callers handle errors ──────────────
;; Convention: When you call controller.KeyFunc, you should call
;; HandleError (or a variant) to report the failure.
;; Deviations silently drop errors.

(define-belief "keyfunc-error-handling"
  (sites (functions-matching (contains-call "KeyFunc")))
  (expect (contains-call "HandleError" "HandleErrorWithLogger"
                         "HandleErrorWithContext"))
  (threshold 0.80 3))

;; ── Belief 2: Tombstone handlers report errors ───────────
;; Convention: Functions that handle deletions and process
;; tombstones should call an error handler for unexpected types.

(define-belief "tombstone-error-reporting"
  (sites (functions-matching (name-matches "delete")))
  (expect (contains-call "HandleError" "HandleErrorWithLogger"
                         "HandleErrorWithContext"))
  (threshold 0.80 3))

;; ── Belief 3: resolveControllerRef callers nil-check ─────
;; Convention: After calling resolveControllerRef, the result
;; should be checked for nil before use.

(define-belief "resolve-ref-nil-check"
  (sites (functions-matching (contains-call "resolveControllerRef")))
  (expect (custom (lambda (site ctx)
    ;; Check if the function body contains a nil comparison
    ;; after calling resolveControllerRef.
    ;; Simple heuristic: the function should contain "== nil" or "!= nil"
    ;; as part of the control flow.
    (let ((body-str (go-format site)))
      (if (and (string? body-str)
               (let check ((i 0))
                 (cond
                   ((>= i (- (string-length body-str) 6)) #f)
                   ((string=? (substring body-str i (+ i 6)) "== nil") #t)
                   ((string=? (substring body-str i (+ i 6)) "!= nil") #t)
                   (else (check (+ i 1))))))
        'checked
        'unchecked)))))
  (threshold 0.80 3))

;; ── Belief 4: Informer handler logging ───────────────────
;; Convention: Informer event handlers (add*, update*, delete*)
;; should log at V(4) or higher for observability.

(define-belief "handler-logs"
  (sites (functions-matching
           (any-of (name-matches "add")
                   (name-matches "update")
                   (name-matches "delete"))))
  (expect (contains-call "Info" "V"))
  (threshold 0.70 5))

;; ── Belief 5: Sync functions return errors ───────────────
;; Convention: Functions named sync* should propagate errors
;; (call fmt.Errorf or return an error).

(define-belief "sync-returns-error"
  (sites (functions-matching (name-matches "sync")))
  (expect (contains-call "Errorf" "Wrapf" "NewAggregate"))
  (threshold 0.80 3))

;; ── Belief 6: GetControllerOf callers resolve the ref ────
;; Convention: If you call metav1.GetControllerOf, you should
;; resolve it via resolveControllerRef.

(define-belief "get-controller-resolves"
  (sites (functions-matching (contains-call "GetControllerOf")))
  (expect (contains-call "resolveControllerRef"))
  (threshold 0.80 3))

;; ── Run against workload controllers ─────────────────────

(define controller-packages
  '("k8s.io/kubernetes/pkg/controller/deployment"
    "k8s.io/kubernetes/pkg/controller/replicaset"
    "k8s.io/kubernetes/pkg/controller/statefulset"
    "k8s.io/kubernetes/pkg/controller/daemonset"
    "k8s.io/kubernetes/pkg/controller/job"
    "k8s.io/kubernetes/pkg/controller/cronjob"))

(for-each
  (lambda (pkg)
    (let ((short (let loop ((s pkg))
                   (if (= (string-length s) 0) s
                     (let ((i (let scan ((j 0))
                                (cond ((>= j (string-length s)) -1)
                                      ((char=? (string-ref s j) #\/) (scan (+ j 1)))
                                      (else j)))))
                       (substring s (let find-last ((k (- (string-length s) 1)))
                                      (cond ((< k 0) 0)
                                            ((char=? (string-ref s k) #\/) (+ k 1))
                                            (else (find-last (- k 1)))))
                                  (string-length s)))))))
      (newline)
      (display "▸ ") (display short) (newline)
      (run-beliefs pkg)))
  controller-packages)
