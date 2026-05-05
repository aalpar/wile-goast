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

(define-library (wile goast ssa-normalize)
  (export
    ssa-normalize
    ssa-rule-set
    ssa-rule-identity
    ssa-rule-commutative
    ssa-rule-annihilation
    ssa-rule-idempotence
    ssa-rule-absorption
    ssa-rule-associativity
    ;; v2: named theory for discover-equivalences
    ssa-theory
    ssa-binop-protocol)
  (import (wile goast utils)
          (wile algebra rewrite)
          (wile algebra symbolic))
  (include "ssa-normalize.scm"))
