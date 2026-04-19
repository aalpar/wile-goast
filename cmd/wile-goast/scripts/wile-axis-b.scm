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

;;; wile-axis-b.scm — Phase 3.B analyzer script.
;;;
;;; Consumes wile/plans/axis-b-manifest.scm + go-ssa-narrow to produce:
;;;   - plans/axis-b-raw.scm                 (structured per-primitive data)
;;;   - plans/2026-04-19-axis-b-inventory.md (bucketed markdown inventory)
;;;   - stdout summary                       (bucket counts + reason tally)
;;;
;;; Invocation:
;;;   wile-goast --run wile-axis-b
;;;
;;; Env overrides:
;;;   WILE_AXIS_B_MANIFEST   — manifest path (default: ../wile/plans/axis-b-manifest.scm)
;;;   WILE_AXIS_B_RAW_OUTPUT — raw output path (default: ../wile/plans/axis-b-raw.scm)
;;;   WILE_AXIS_B_INVENTORY  — inventory path  (default: ../wile/plans/2026-04-19-axis-b-inventory.md)
;;;
;;; See plans/2026-04-19-pr-3-axis-b-script-impl.md for the implementation plan.
;;; See plans/2026-04-19-axis-b-analyzer-impl-design.md §7 for the overall design.

(import (wile goast)
        (wile goast ssa)
        (wile goast utils))

;; ---------------------------------------------------------------------------
;; Path resolution
;; ---------------------------------------------------------------------------

;; env-or returns the env var value if set and non-empty, else the default.
(define (env-or name default)
  (let ((v (get-environment-variable name)))
    (if (and v (not (string=? v "")))
        v
        default)))

(define default-manifest-path   "../wile/plans/axis-b-manifest.scm")
(define default-raw-output-path "../wile/plans/axis-b-raw.scm")
(define default-inventory-path  "../wile/plans/2026-04-19-axis-b-inventory.md")

(define (manifest-path)   (env-or "WILE_AXIS_B_MANIFEST"   default-manifest-path))
(define (raw-output-path) (env-or "WILE_AXIS_B_RAW_OUTPUT" default-raw-output-path))
(define (inventory-path)  (env-or "WILE_AXIS_B_INVENTORY"  default-inventory-path))

;; ---------------------------------------------------------------------------
;; Manifest parsing
;; ---------------------------------------------------------------------------

;; read-manifest reads the manifest S-expression list. Each entry is:
;;   ("name" "declared-return-type" "go-function-name" "source-file:line")
;; The manifest file contains exactly one list; we read it directly.
(define (read-manifest path)
  (let* ((port (open-input-file path))
         (data (read port)))
    (close-input-port port)
    (if (list? data)
        data
        (error "manifest is not a list" path))))

;; manifest-entry accessors.
(define (entry-name e)            (list-ref e 0))
(define (entry-declared-return e) (list-ref e 1))
(define (entry-go-function e)     (list-ref e 2))
(define (entry-go-source e)       (list-ref e 3))

;; ---------------------------------------------------------------------------
;; Main entry
;; ---------------------------------------------------------------------------

(define (main)
  (let* ((mpath (manifest-path))
         (entries (read-manifest mpath)))
    (display "wile-axis-b: loaded ")
    (display (length entries))
    (display " primitives from ")
    (display mpath)
    (newline)))

(main)
