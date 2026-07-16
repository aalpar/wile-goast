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

;; ── Reader ───────────────────────────────────────────────
;;
;; read-syntax returns located syntax objects (read would discard the
;; position). Loop to eof, then close the port.

(define (read-file-forms path)
  (let* ((port (open-input-file path))
         (forms (let loop ((acc '()))
                  (let ((form (read-syntax port)))
                    (if (eof-object? form)
                        (reverse acc)
                        (loop (cons form acc)))))))
    (close-input-port port)
    forms))

;; ── Definition classifier ────────────────────────────────
;;
;; Each emitted name carries ITS OWN identifier position, read off the
;; sub-syntax. Navigate with syntax->list (keeps child syntax objects);
;; call syntax->datum only on leaf identifiers to recover the name symbol.

(define (make-scm-def name kind id-stx)
  (list (cons 'name name)
        (cons 'kind kind)
        (cons 'file (or (syntax-source id-stx) ""))
        (cons 'line (syntax-line id-stx))
        (cons 'col  (syntax-column id-stx))))

(define (def-head-identifier stx)
  ;; innermost head id of a (possibly curried) define target
  (if (pair? (syntax->datum stx))
      (def-head-identifier (car (syntax->list stx)))
      stx))

(define (classify-define children)
  ;; children = (define-kw target . body) as syntax objects
  (let ((target (cadr children)))
    (if (pair? (syntax->datum target))
        (let ((id (def-head-identifier target)))
          (list (make-scm-def (symbol->string (syntax->datum id)) "procedure" id)))
        (list (make-scm-def (symbol->string (syntax->datum target)) "variable" target)))))

(define (classify-define-syntax children)
  (let ((name (cadr children)))
    (list (make-scm-def (symbol->string (syntax->datum name)) "syntax" name))))

(define (classify-define-values children)
  ;; formals assumed a proper identifier list (rest-args out of scope, v1)
  (map (lambda (id) (make-scm-def (symbol->string (syntax->datum id)) "variable" id))
       (syntax->list (cadr children))))

(define (classify-record-field spec)
  ;; spec = (field-name accessor) or (field-name accessor mutator)
  (let* ((parts (syntax->list spec))
         (acc (cadr parts))
         (base (list (make-scm-def (symbol->string (syntax->datum acc)) "record-accessor" acc))))
    (if (>= (length parts) 3)
        (let ((mut (caddr parts)))
          (append base
                  (list (make-scm-def (symbol->string (syntax->datum mut)) "record-mutator" mut))))
        base)))

(define (classify-record-type children)
  ;; children = (drt-kw type-name ctor-spec pred . field-specs)
  (let* ((type-id  (cadr children))
         (ctor-id  (car (syntax->list (caddr children))))
         (pred-id  (cadddr children))
         (fields   (cddddr children)))
    (append
      (list (make-scm-def (symbol->string (syntax->datum type-id)) "record-type" type-id)
            (make-scm-def (symbol->string (syntax->datum ctor-id)) "record-constructor" ctor-id)
            (make-scm-def (symbol->string (syntax->datum pred-id)) "record-predicate" pred-id))
      (flat-map classify-record-field fields))))

(define (classify-def-form form)
  (let ((d (syntax->datum form)))
    (if (not (pair? d))
        '()
        (let ((head (car d))
              (children (syntax->list form)))
          (case head
            ((define)        (classify-define children))
            ((define-syntax) (classify-define-syntax children))
            ((define-values) (classify-define-values children))
            ((define-record-type) (classify-record-type children))
            ((begin)         (flat-map classify-def-form (cdr children)))
            (else '()))))))


;; ── Library records ──────────────────────────────────────

(define (paren-list->string syms)
  (string-append "(" (string-join (map (lambda (s)
                                         (if (symbol? s) (symbol->string s) (number->string s)))
                                       syms)
                                  " ")
                 ")"))

(define (library-clause clauses tag)
  ;; datum of the first clause whose head = TAG, or #f
  (let loop ((cs clauses))
    (if (null? cs)
        #f
        (let ((cd (syntax->datum (car cs))))
          (if (and (pair? cd) (eq? (car cd) tag)) cd (loop (cdr cs)))))))

(define (import-spec->name spec)
  ;; spec is a datum: (lib ...) or (only (lib) ...) / (except (lib) ...) /
  ;; (prefix (lib) p) / (rename (lib) ...). Dig to the (lib ...) part.
  (cond
    ((not (pair? spec)) spec)
    ((memq (car spec) '(only except prefix rename)) (import-spec->name (cadr spec)))
    (else spec)))

(define (classify-library form)
  (let ((d (syntax->datum form)))
    (if (and (pair? d) (eq? (car d) 'define-library))
        (let* ((children (syntax->list form))
               (name-stx (cadr children))
               (clauses  (cddr children))
               (exp (or (library-clause clauses 'export) '(export)))
               (imp (or (library-clause clauses 'import) '(import)))
               (inc (or (library-clause clauses 'include) '(include))))
          (list (cons 'name (paren-list->string (syntax->datum name-stx)))
                (cons 'file (or (syntax-source form) ""))
                (cons 'line (syntax-line name-stx))
                (cons 'exports  (map symbol->string (filter symbol? (cdr exp))))
                (cons 'imports  (map (lambda (spec) (paren-list->string (import-spec->name spec)))
                                      (cdr imp)))
                (cons 'includes (cdr inc))))
        #f)))
