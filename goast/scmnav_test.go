// Copyright 2026 Aaron Alpar
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestScmnavSourceFiles(t *testing.T) {
	engine := newBeliefEngine(t)
	// Recursively enumerate .scm/.sld under the fixture tree, keeping only
	// scheme sources (notes.txt excluded), and count them. Sorted for a
	// stable assertion.
	result := eval(t, engine, `
		(import (wile goast scmnav))
		(let ((fs (scm-source-files (list "../examples/goast-query/testdata/scmnav/enumtree"))))
		  (list (length fs)
		        (map string->symbol
		             (sort string<? (map (lambda (p)
		                                        ;; basename for a path-independent check
		                                        (let loop ((i (- (string-length p) 1)))
		                                          (cond ((< i 0) p)
		                                                ((char=? (string-ref p i) #\/)
		                                                 (substring p (+ i 1) (string-length p)))
		                                                (else (loop (- i 1))))))
		                                      fs)))))
	`)
	c := qt.New(t)
	c.Assert(result.SchemeString(), qt.Equals, `(2 (a.scm b.sld))`)
}

func TestScmnavReadFileForms(t *testing.T) {
	engine := newBeliefEngine(t)
	// a.scm holds exactly one top-level form, on line 1.
	result := eval(t, engine, `
		(import (wile goast scmnav))
		(let ((forms (read-file-forms "../examples/goast-query/testdata/scmnav/enumtree/a.scm")))
		  (list (length forms) (syntax-line (car forms))))
	`)
	c := qt.New(t)
	c.Assert(result.SchemeString(), qt.Equals, `(1 1)`)
}

func TestScmnavClassifyCore(t *testing.T) {
	engine := newBeliefEngine(t)
	// Parse a string of several definition forms and project (name . kind)
	// pairs, in order. read-syntax over a string port keeps line numbers.
	result := eval(t, engine, `
		(import (wile goast scmnav))
		(define src "(define x 1)\n(define (f a) a)\n(define-syntax m (syntax-rules () ((_) 1)))\n(define-values (p q) (values 1 2))\n(begin (define ((curry a) b) b))")
		(define port (open-input-string src))
		(let loop ((acc '()))
		  (let ((form (read-syntax port)))
		    (if (eof-object? form)
		        (reverse acc)
		        (loop (append (reverse (map (lambda (d)
		                                      (cons (cdr (assq 'name d)) (cdr (assq 'kind d))))
		                                    (classify-def-form form)))
		                      acc)))))
	`)
	c := qt.New(t)
	c.Assert(result.SchemeString(), qt.Equals,
		`(("x" . "variable") ("f" . "procedure") ("m" . "syntax") ("p" . "variable") ("q" . "variable") ("curry" . "procedure"))`)
}

func TestScmnavClassifyPositions(t *testing.T) {
	engine := newBeliefEngine(t)
	// The procedure name f sits on line 2; assert its recorded line/col.
	result := eval(t, engine, `
		(import (wile goast scmnav))
		(define port (open-input-string "(define x 1)\n(define (f a) a)"))
		(read-syntax port)                       ; skip form 1
		(let ((d (car (classify-def-form (read-syntax port)))))
		  (list (cdr (assq 'name d)) (cdr (assq 'line d))))
	`)
	c := qt.New(t)
	// f is on line 2. (Column asserted separately once behavior is confirmed.)
	c.Assert(result.SchemeString(), qt.Equals, `("f" 2)`)
}

func TestScmnavClassifyColumn(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	// Test simple definition: (define (f a) a)
	// The identifier 'f' sits at column 9 (0-based)
	result := eval(t, engine, `
		(import (wile goast scmnav))
		(define p1 (open-input-string "(define (f a) a)"))
		(let ((d (car (classify-def-form (read-syntax p1)))))
		  (list (cdr (assq 'name d)) (cdr (assq 'col d))))
	`)
	c.Assert(result.SchemeString(), qt.Equals, `("f" 9)`)

	// Test curried definition: (define ((curry a) b) b)
	// The identifier 'curry' sits at column 10 (0-based)
	result = eval(t, engine, `
		(import (wile goast scmnav))
		(define p2 (open-input-string "(define ((curry a) b) b)"))
		(let ((d (car (classify-def-form (read-syntax p2)))))
		  (list (cdr (assq 'name d)) (cdr (assq 'col d))))
	`)
	c.Assert(result.SchemeString(), qt.Equals, `("curry" 10)`)
}

func TestScmnavClassifyNonDefinition(t *testing.T) {
	engine := newBeliefEngine(t)
	c := qt.New(t)

	// Test non-pair form (a number): should return empty list
	result := eval(t, engine, `
		(import (wile goast scmnav))
		(classify-def-form (read-syntax (open-input-string "42")))
	`)
	c.Assert(result.SchemeString(), qt.Equals, `()`)

	// Test unknown head form: should return empty list
	result = eval(t, engine, `
		(import (wile goast scmnav))
		(classify-def-form (read-syntax (open-input-string "(foo 1 2)")))
	`)
	c.Assert(result.SchemeString(), qt.Equals, `()`)
}

func TestScmnavClassifyRecordType(t *testing.T) {
	engine := newBeliefEngine(t)
	// One record form -> type, constructor, predicate, one accessor-only
	// field, one accessor+mutator field. Project (name . kind) in order.
	result := eval(t, engine, `
		(import (wile goast scmnav))
		(define src "(define-record-type point (make-point x y) point? (x point-x) (y point-y set-point-y!))")
		(map (lambda (d) (cons (cdr (assq 'name d)) (cdr (assq 'kind d))))
		     (classify-def-form (read-syntax (open-input-string src))))
	`)
	c := qt.New(t)
	c.Assert(result.SchemeString(), qt.Equals,
		`(("point" . "record-type") ("make-point" . "record-constructor") ("point?" . "record-predicate") ("point-x" . "record-accessor") ("point-y" . "record-accessor") ("set-point-y!" . "record-mutator"))`)

	// Position tripwire: every accessor identifier on line 1, at distinct
	// increasing columns (they are laid out left-to-right on one line).
	cols := eval(t, engine, `
		(import (wile goast scmnav))
		(define src2 "(define-record-type point (make-point x y) point? (x point-x) (y point-y set-point-y!))")
		(let ((defs (classify-def-form (read-syntax (open-input-string src2)))))
		  (map (lambda (d) (cdr (assq 'col d)))
		       (let loop ((ds defs) (acc '()))
		         (cond
		           ((null? ds) (reverse acc))
		           ((equal? (cdr (assq 'kind (car ds))) "record-accessor")
		            (loop (cdr ds) (cons (car ds) acc)))
		           (else (loop (cdr ds) acc))))))
	`)
	c.Assert(cols.SchemeString(), qt.Equals, `(53 65)`)
}

func TestScmnavClassifyLibrary(t *testing.T) {
	engine := newBeliefEngine(t)
	result := eval(t, engine, `
		(import (wile goast scmnav))
		(define src "(define-library (test lib) (export a b) (import (srfi 13) (only (test dep) c)) (include \"lib.scm\"))")
		(let ((r (classify-library (read-syntax (open-input-string src)))))
		  (list (cdr (assq 'name r))
		        (cdr (assq 'exports r))
		        (cdr (assq 'imports r))
		        (cdr (assq 'includes r))))
	`)
	c := qt.New(t)
	c.Assert(result.SchemeString(), qt.Equals,
		`("(test lib)" ("a" "b") ("(srfi 13)" "(test dep)") ("lib.scm"))`)
}

func TestScmnavClassifyLibraryNonLibrary(t *testing.T) {
	engine := newBeliefEngine(t)
	result := eval(t, engine, `
		(import (wile goast scmnav))
		(if (classify-library (read-syntax (open-input-string "(define x 1)"))) "yes" "no")
	`)
	c := qt.New(t)
	c.Assert(result.SchemeString(), qt.Equals, `"no"`)
}

func TestScmnavBuildIndex(t *testing.T) {
	engine := newBeliefEngine(t)
	// Index the exports fixture: two defs, both annotated with their library.
	result := eval(t, engine, `
		(import (wile goast scmnav))
		(let* ((files (scm-source-files (list "../examples/goast-query/testdata/scmnav/exports")))
		       (idx (build-index files))
		       (defs (cdr (assq 'defs idx))))
		  (list (length defs)
		        (sort string<?
		          (map (lambda (d)
		                 (string-append (cdr (assq 'name d)) ":"
		                                (or (and (assq 'library d) (cdr (assq 'library d))) "-")))
		               defs))
		        (length (cdr (assq 'libraries idx)))))
	`)
	c := qt.New(t)
	c.Assert(result.SchemeString(), qt.Equals,
		`(2 ("make-widget:(test local)" "widget:(test local)") 1)`)
}
