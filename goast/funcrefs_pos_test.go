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
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestGoFuncRefs_Position(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	// Every func-ref entry for a real declaration must carry a source position.
	out := eval(t, engine, `
		(define refs (go-func-refs
		  "github.com/aalpar/wile-goast/goast/testdata/iface"))
		(let loop ((rs refs))
		  (if (null? rs) #f
		    (let ((p (assoc 'pos (cdr (car rs)))))
		      (if (and p (string? (cdr p))) (cdr p) (loop (cdr rs))))))
	`).SchemeString()
	c.Assert(out, qt.Not(qt.Equals), "#f")
	c.Assert(strings.Contains(out, "iface.go:"), qt.IsTrue, qt.Commentf("pos = %s", out))
}

// qname is the typed-AST func-decl name, and func-uses names the loaded
// package's functions a body references, same package included, by the
// same qnames: promoted, value-receiver and generic targets alike.
func TestGoFuncRefs_QnameAndFuncUses(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/methodset"
	eval(t, engine, `
		(define refs (go-func-refs "`+pkg+`"))
		(define (field fr k) (cdr (assoc k (cdr fr))))
		(define uses
		  (let loop ((rs refs))
		    (if (equal? (field (car rs) 'name) "Uses") (car rs) (loop (cdr rs)))))
		(define ast-names
		  (let ((acc '()))
		    (for-each
		      (lambda (file)
		        (for-each
		          (lambda (d)
		            (if (eq? (car d) 'func-decl)
		              (set! acc (cons (cdr (assoc 'name (cdr d))) acc))))
		          (cdr (assoc 'decls (cdr file)))))
		      (cdr (assoc 'files (cdr (car (go-typecheck-package "`+pkg+`"))))))
		    acc))
	`)

	t.Run("every qname is an AST func-decl name", func(t *testing.T) {
		out := eval(t, engine, `
			(let loop ((rs refs))
			  (cond ((null? rs) #t)
			        ((member (field (car rs) 'qname) ast-names) (loop (cdr rs)))
			        (else (field (car rs) 'qname))))
		`).SchemeString()
		c.Assert(out, qt.Equals, "#t")
	})

	t.Run("func-uses lists the referenced declarations by qname", func(t *testing.T) {
		out := eval(t, engine, `(field uses 'func-uses)`).SchemeString()
		c.Assert(out, qt.Equals, `("(*`+pkg+`.G[T]).Get" "(*`+pkg+`.P).Inc" "(`+pkg+`.V).Upper" "`+pkg+`.Map")`)
	})
}
