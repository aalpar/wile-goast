;;; ssa-unify-detect.scm — SSA equivalence pass validation
;;;
;;; Runs the full pipeline on test case pairs, measuring similarity
;;; at three stages to isolate each layer's contribution:
;;;   1. AST diff (go-typecheck-package)
;;;   2. SSA canonicalized only (go-ssa-canonicalize)
;;;   3. SSA canonicalized + normalized (ssa-normalize)
;;;
;;; Usage: cd /path/to/wile-goast && wile-goast -f examples/goast-query/ssa-unify-detect.scm

(import (wile goast utils)
        (wile goast ssa-normalize)
        (wile goast unify))

;; ── Helpers ──────────────────────────────────────────────

(define (find-func-decl pkg-list func-name)
  "Find a func-decl by name in a list of package s-expressions."
  (let pkg-loop ((pkgs pkg-list))
    (if (null? pkgs) #f
      (let* ((pkg (car pkgs))
             (files (nf pkg 'files)))
        (let file-loop ((fs files))
          (if (null? fs)
            (pkg-loop (cdr pkgs))
            (let ((decls (nf (car fs) 'decls)))
              (let decl-loop ((ds decls))
                (if (null? ds)
                  (file-loop (cdr fs))
                  (let ((d (car ds)))
                    (if (and (tag? d 'func-decl)
                             (equal? (nf d 'name) func-name))
                      d
                      (decl-loop (cdr ds)))))))))))))

(define (find-ssa-func func-list func-name)
  "Find an ssa-func by name in a flat list."
  (let loop ((fs func-list))
    (if (null? fs) #f
      (let ((fn (car fs)))
        (if (equal? (nf fn 'name) func-name)
          fn
          (loop (cdr fs)))))))

(define (ssa-normalize-func fn)
  "Apply ssa-normalize to all instructions in all blocks of a canonicalized ssa-func."
  (let* ((blocks (nf fn 'blocks))
         (new-blocks
           (map (lambda (block)
                  (let* ((instrs (nf block 'instrs))
                         (new-instrs (map ssa-normalize instrs)))
                    ;; Rebuild block with normalized instructions
                    (cons (car block)
                          (map (lambda (pair)
                                 (if (and (pair? pair) (eq? (car pair) 'instrs))
                                   (cons 'instrs new-instrs)
                                   pair))
                               (cdr block)))))
                blocks)))
    ;; Rebuild func with normalized blocks
    (cons (car fn)
          (map (lambda (pair)
                 (if (and (pair? pair) (eq? (car pair) 'blocks))
                   (cons 'blocks new-blocks)
                   pair))
               (cdr fn)))))

(define (fmt-sim n)
  "Format a similarity number to 4 decimal places."
  (let* ((rounded (/ (round (* n 10000)) 10000))
         (s (number->string (exact->inexact rounded))))
    ;; Pad to at least 6 chars
    (if (< (string-length s) 6)
      (string-append s (make-string (- 6 (string-length s)) #\0))
      s)))

;; ── Test case runner ─────────────────────────────────────

(define (run-test-case label pkg-a-path pkg-b-path func-name-a func-name-b threshold)
  "Run the full pipeline on one test case and return results."
  (display "  ") (display label) (display " ...") (newline)

  ;; Stage 1: AST diff
  (let* ((ast-a (go-typecheck-package pkg-a-path))
         (ast-b (go-typecheck-package pkg-b-path))
         (func-a (find-func-decl ast-a func-name-a))
         (func-b (find-func-decl ast-b func-name-b))
         (ast-result (if (and func-a func-b)
                       (ast-diff func-a func-b)
                       (list 0 1 '())))
         (ast-sim (diff-result-similarity ast-result)))

    ;; Stage 2: SSA canonicalized
    (let* ((ssa-a (go-ssa-build pkg-a-path))
           (ssa-b (go-ssa-build pkg-b-path))
           (sfn-a (find-ssa-func ssa-a func-name-a))
           (sfn-b (find-ssa-func ssa-b func-name-b))
           (canon-a (if sfn-a (go-ssa-canonicalize sfn-a) #f))
           (canon-b (if sfn-b (go-ssa-canonicalize sfn-b) #f))
           (canon-result (if (and canon-a canon-b)
                           (ssa-diff canon-a canon-b)
                           (list 0 1 '())))
           (canon-sim (diff-result-similarity canon-result)))

      ;; Stage 3: SSA canonicalized + normalized
      (let* ((norm-a (if canon-a (ssa-normalize-func canon-a) #f))
             (norm-b (if canon-b (ssa-normalize-func canon-b) #f))
             (norm-result (if (and norm-a norm-b)
                            (ssa-diff norm-a norm-b)
                            (list 0 1 '())))
             (norm-sim (diff-result-similarity norm-result))
             (verdict (if (and norm-a norm-b)
                        (if (unifiable? norm-result threshold)
                          "unifiable"
                          "divergent")
                        "error")))

        (list label ast-sim canon-sim norm-sim verdict)))))

;; ── Main ─────────────────────────────────────────────────

(define base "github.com/aalpar/wile-goast/examples/goast-query/testdata")
(define threshold 0.60)

(display "SSA Equivalence Pass — Validation") (newline)
(display "══════════════════════════════════════════════════════════════════════") (newline)
(newline)

(define results
  (list
    (run-test-case
      "pncounter/gcounter Increment"
      (string-append base "/pncounter")
      (string-append base "/gcounter")
      "Increment" "Increment"
      threshold)
    (run-test-case
      "identity AddZero/JustReturn"
      (string-append base "/identity")
      (string-append base "/identity")
      "AddZero" "JustReturn"
      threshold)))

(newline)
(display "  Test case                       AST      SSA-canon  SSA-norm   Verdict") (newline)
(display "  ────────────────────────────────────────────────────────────────────────") (newline)

(for-each
  (lambda (r)
    (display "  ")
    (let ((label (list-ref r 0)))
      (display label)
      (display (make-string (max 1 (- 34 (string-length label))) #\space)))
    (display (fmt-sim (list-ref r 1))) (display "   ")
    (display (fmt-sim (list-ref r 2))) (display "   ")
    (display (fmt-sim (list-ref r 3))) (display "   ")
    (display (list-ref r 4))
    (newline))
  results)

(newline)
(display "  threshold: ") (display threshold) (newline)
(display "══════════════════════════════════════════════════════════════════════") (newline)
