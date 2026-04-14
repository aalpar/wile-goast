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

;;; belief-validate-categories.scm — Validate belief categories 1-4
;;;
;;; Runs beliefs against synthetic testdata packages with known
;;; deviations. Each category should find exactly 1 deviation.
;;;
;;; Usage: wile-goast -f examples/goast-query/belief-validate-categories.scm

(import (wile goast belief))

;; ── Category 1: Pairing ──────────────────────────

(define-belief "cat1-lock-unlock"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.66 3))

(run-beliefs
  "github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing")

;; ── Category 2: Check ────────────────────────────
(reset-beliefs!)

(define-belief "cat2-err-checked"
  (sites (functions-matching (has-params "error")))
  (expect (checked-before-use "err"))
  (threshold 0.66 3))

(run-beliefs
  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking")

;; ── Category 3: Handling ─────────────────────────
(reset-beliefs!)

(define-belief "cat3-dowork-wrap"
  (sites (callers-of "DoWork"))
  (expect (contains-call "Errorf"))
  (threshold 0.66 3))

(run-beliefs
  "github.com/aalpar/wile-goast/examples/goast-query/testdata/handling")

;; ── Category 4: Ordering ─────────────────────────
(reset-beliefs!)

(define-belief "cat4-validate-process"
  (sites (functions-matching
           (all-of (contains-call "Validate")
                   (contains-call "Process"))))
  (expect (ordered "Validate" "Process"))
  (threshold 0.66 3))

(run-beliefs
  "github.com/aalpar/wile-goast/examples/goast-query/testdata/ordering")
