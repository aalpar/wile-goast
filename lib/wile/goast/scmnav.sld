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

;; (wile goast scmnav) — lexical navigation over Scheme source.
;;
;; Jump-to-def for .scm/.sld: enumerate sources, parse with read-syntax
;; (positions preserved), classify definition forms. Exports grow
;; task-by-task alongside their definitions.
(define-library (wile goast scmnav)
  (export
    scm-source-files
    try-directory-files
    read-file-forms
    classify-def-form
    classify-library
    with-source-file
    build-index
    defs-query
    basename)
  (import
    (srfi 13)              ; string-suffix?, string-trim
    (wile goast utils))    ; filter, filter-map, flat-map
  (include "scmnav.scm"))
