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

;; ── File enumeration ─────────────────────────────────────
;;
;; The files extension has no directory? predicate and directory-files
;; raises on a non-directory, so a guarded directory-files call is how we
;; tell a subdir from a file. directory-files already excludes "." and
;; ".." and returns bare filenames (not full paths).

(define (scheme-source-file? name)
  (or (string-suffix? ".scm" name)
      (string-suffix? ".sld" name)))

(define (try-directory-files dir)
  ;; entry list if DIR is a readable directory, else #f.
  (guard (e (#t #f))
    (directory-files dir)))

(define (scm-source-files-1 dir)
  (flat-map
    (lambda (name)
      (let ((full (string-append dir "/" name)))
        (cond
          ((scheme-source-file? name) (list full))
          ((try-directory-files full) (scm-source-files-1 full)) ; a subdir
          (else '()))))                                          ; other file
    (or (try-directory-files dir) '())))

(define (scm-source-files roots)
  (flat-map scm-source-files-1 roots))
