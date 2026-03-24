;;; raft-dispatch-consistency.scm — Which methods use raftRequest vs raftRequestOnce?
;;;
;;; Since raftRequest just delegates to raftRequestOnce, the choice is arbitrary.
;;; This script finds the inconsistency.
;;;
;;; Usage: cd etcd/server && wile-goast -f /path/to/raft-dispatch-consistency.scm

(import (wile goast utils))

;; ── Utilities ──────────────────────────────────────────

(define (filter pred lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (loop (cdr xs) (if (pred (car xs)) (cons (car xs) acc) acc)))))

(define (flat-map f lst)
  (apply append (map f lst)))

;; ── Load package ───────────────────────────────────────

(define target "go.etcd.io/etcd/server/v3/etcdserver")
(display "Loading ") (display target) (display " ...") (newline)
(define pkg (car (go-typecheck-package target)))

;; Extract all func-decls
(define all-funcs
  (flat-map
    (lambda (file)
      (filter (lambda (d) (tag? d 'func-decl))
              (or (nf file 'decls) '())))
    (nf pkg 'files)))

;; ── Classify each function's raft dispatch calls ───────

(define (calls-in func name)
  ;; Return #t if func's body contains a call to `name`.
  (let ((found #f))
    (let ((body (nf func 'body)))
      (when body
        (walk body
          (lambda (node)
            (when (tag? node 'call-expr)
              (let ((fun (nf node 'fun)))
                (when (and (tag? fun 'selector-expr)
                           (equal? (nf fun 'sel) name))
                  (set! found #t))))))))
    found))

(define once-callers '())
(define wrapper-callers '())

(for-each
  (lambda (func)
    (let ((name (nf func 'name))
          (calls-once (calls-in func "raftRequestOnce"))
          (calls-wrap (calls-in func "raftRequest")))
      ;; Only count direct calls to raftRequest (not raftRequestOnce).
      ;; A function calling both is counted in both lists.
      (when calls-once
        (set! once-callers (cons name once-callers)))
      (when (and calls-wrap (not calls-once))
        (set! wrapper-callers (cons name wrapper-callers)))))
  all-funcs)

(display "──────────────────────────────────────────────────")
(newline)
(display "  raftRequestOnce vs raftRequest usage")
(newline)
(display "──────────────────────────────────────────────────")
(newline)
(newline)

(display "  Direct callers of raftRequestOnce (no retry):")
(newline)
(for-each (lambda (n) (display "    ") (display n) (newline))
          (reverse once-callers))
(newline)

(display "  Callers of raftRequest (trivial wrapper):")
(newline)
(for-each (lambda (n) (display "    ") (display n) (newline))
          (reverse wrapper-callers))
(newline)

(display "  Summary: ")
(display (length once-callers)) (display " direct, ")
(display (length wrapper-callers)) (display " via wrapper")
(newline)
(display "  (raftRequest just calls raftRequestOnce — they're identical)")
(newline)
