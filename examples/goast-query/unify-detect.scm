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

;;; unify-detect.scm — Prototype: Procedure unification detection via goast
;;;
;;; Parses two Go source strings, extracts functions by name,
;;; computes a recursive AST diff, classifies differences, and scores.
;;;
;;; Usage: wile -f examples/embedding/goast-query/unify-detect.scm

(import (wile goast utils)    ; nf, tag?, filter, filter-map, unique
        (wile goast unify))   ; ast-diff, diff-result-*, score-diffs

;; ══════════════════════════════════════════════════════════
;; Function extraction
;; ══════════════════════════════════════════════════════════

;; Extract all func-decl nodes from a parsed file.
(define (extract-funcs file-ast)
  (filter-map
    (lambda (decl)
      (and (tag? decl 'func-decl) decl))
    (nf file-ast 'decls)))

;; Find a func-decl by name.
(define (find-func file-ast name)
  (let loop ((funcs (extract-funcs file-ast)))
    (cond
      ((null? funcs) #f)
      ((equal? (nf (car funcs) 'name) name) (car funcs))
      (else (loop (cdr funcs))))))

;; ══════════════════════════════════════════════════════════
;; Reporting
;; ══════════════════════════════════════════════════════════

;; Truncate large AST nodes for display.
(define (summarize val)
  (cond
    ((not val) "#f")
    ((string? val) val)
    ((symbol? val) (symbol->string val))
    ((number? val) (number->string val))
    ((boolean? val) (if val "#t" "#f"))
    ((and (pair? val) (symbol? (car val)))
     (string-append "(" (symbol->string (car val)) " ...)"))
    ((pair? val) "(list ...)")
    (else "?")))

(define (display-diff-entry d)
  (let ((category (list-ref d 0))
        (path (list-ref d 1))
        (val-a (list-ref d 2))
        (val-b (list-ref d 3)))
    (display "    ")
    (display category)
    (display "  at ")
    (display path)
    (newline)
    (display "      a: ")
    (display (summarize val-a))
    (newline)
    (display "      b: ")
    (display (summarize val-b))
    (newline)))

;; score-diffs returns the collapsed 8-element vector:
;;   0 raw-similarity  1 effective-similarity  2 param-count
;;   3 weighted-cost   4 roots  5 value-params  6 collapsed-diffs
;;   7 derived-count
(define (display-result name-a name-b result)
  (let* ((shared-count (diff-result-shared result))
         (diff-count (diff-result-diff-count result))
         (score (score-diffs shared-count diff-count
                             (diff-result-diffs result)))
         (roots (list-ref score 4))
         (value-params (list-ref score 5))
         (diffs (list-ref score 6)))
    (newline)
    (display "  ")
    (display name-a) (display " <-> ") (display name-b)
    (newline)
    (display "  shared nodes:    ") (display shared-count) (newline)
    (display "  diff nodes:      ") (display diff-count) (newline)
    (display "  raw similarity:  ") (display (list-ref score 0)) (newline)
    (display "  eff similarity:  ") (display (list-ref score 1)) (newline)
    (display "  params needed:   ") (display (list-ref score 2)) (newline)
    (display "  weighted cost:   ") (display (list-ref score 3)) (newline)
    (display "  derived types:   ") (display (list-ref score 7)) (newline)
    (if (pair? roots)
      (begin
        (display "  type params:     ") (display roots)
        (newline)))
    (if (pair? value-params)
      (begin
        (display "  value params:    ") (display value-params)
        (newline)))
    (newline)
    ;; Group collapsed diffs by category.
    (let ((categories (unique (map car diffs))))
      (for-each
        (lambda (cat)
          (let ((count (length (filter (lambda (d) (eq? (car d) cat))
                                       diffs))))
            (display "  [") (display cat) (display "] x")
            (display count) (display ":") (newline)
            (for-each
              (lambda (d)
                (if (eq? (car d) cat)
                  (display-diff-entry d)))
              diffs)
            (newline)))
        categories))))

;; ══════════════════════════════════════════════════════════
;; Test: pncounter.Increment vs gcounter.Increment
;;
;; Inline Go source — self-contained, no disk dependencies.
;; Stripped to Increment + supporting types to focus the diff.
;; ══════════════════════════════════════════════════════════

(define source-pncounter "
package pncounter

type Dot struct {
	ID  string
	Seq uint64
}

type CounterValue struct {
	N int64
}

type Counter struct {
	id    string
	store map[Dot]CounterValue
}

func (p *Counter) Increment(n int64) *Counter {
	var oldDot Dot
	var oldValue int64
	hasOld := false

	for d, v := range p.store {
		if d.ID == p.id {
			oldDot = d
			oldValue = v.N
			hasOld = true
			break
		}
	}

	newVal := CounterValue{N: oldValue + n}
	if hasOld {
		delete(p.store, oldDot)
	}
	p.store[Dot{ID: p.id}] = newVal

	delta := make(map[Dot]CounterValue)
	delta[Dot{ID: p.id}] = newVal

	return &Counter{store: delta}
}
")

(define source-gcounter "
package gcounter

type Dot struct {
	ID  string
	Seq uint64
}

type GValue struct {
	N uint64
}

type Counter struct {
	id    string
	store map[Dot]GValue
}

func (g *Counter) Increment(n uint64) *Counter {
	var oldDot Dot
	var oldValue uint64
	hasOld := false

	for d, v := range g.store {
		if d.ID == g.id {
			oldDot = d
			oldValue = v.N
			hasOld = true
			break
		}
	}

	newVal := GValue{N: oldValue + n}
	if hasOld {
		delete(g.store, oldDot)
	}
	g.store[Dot{ID: g.id}] = newVal

	delta := make(map[Dot]GValue)
	delta[Dot{ID: g.id}] = newVal

	return &Counter{store: delta}
}
")

;; ── Run ───────────────────────────────────────────────────

(display "Parsing source A (pncounter) ...") (newline)
(define file-a (go-parse-string source-pncounter))

(display "Parsing source B (gcounter) ...") (newline)
(define file-b (go-parse-string source-gcounter))

(define func-a (find-func file-a "Increment"))
(define func-b (find-func file-b "Increment"))

(if (not func-a) (error "Increment not found in source A"))
(if (not func-b) (error "Increment not found in source B"))

(display "Diffing Increment functions ...") (newline)

(display-result "pncounter.Increment" "gcounter.Increment"
                (ast-diff func-a func-b))

;; ── Also diff the full files to find all matching function pairs ──

(display "══════════════════════════════════════════")
(newline)
(display "  Full file comparison: all shared names  ")
(newline)
(display "══════════════════════════════════════════")
(newline)

(let* ((funcs-a (extract-funcs file-a))
       (funcs-b (extract-funcs file-b))
       (pairs (filter-map
                (lambda (fa)
                  (let* ((name (nf fa 'name))
                         (fb (find-func file-b name)))
                    (and fb (list name fa fb))))
                funcs-a)))
  (if (null? pairs)
    (begin (display "  (no functions with matching names)") (newline))
    (for-each
      (lambda (entry)
        (let ((name (car entry))
              (fa (cadr entry))
              (fb (caddr entry)))
          (display-result
            (string-append "A." name)
            (string-append "B." name)
            (ast-diff fa fb))))
      pairs)))
