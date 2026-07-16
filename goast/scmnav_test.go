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
