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

	"github.com/aalpar/wile/values"

	qt "github.com/frankban/quicktest"
)

func TestInlinePipeline(t *testing.T) {
	engine := newBeliefEngine(t)

	result := eval(t, engine, `
		(import (wile goast utils))

		(define source "
			package example

			func double(x int) int { return x * 2 }
			func addOne(x int) int { return x + 1 }

			func clamp(x, lo, hi int) int {
				if x < lo { return lo }
				if x > hi { return hi }
				return x
			}

			func compute(a, b int) int {
				x := double(a)
				y := addOne(b)
				z := clamp(x+y, 0, 100)
				return z
			}")

		(define file (go-parse-string source))

		;; Helpers
		(define (find-func name)
		  (let loop ((ds (nf file 'decls)))
		    (cond ((null? ds) #f)
		          ((and (tag? (car ds) 'func-decl)
		                (equal? (nf (car ds) 'name) name))
		           (car ds))
		          (else (loop (cdr ds))))))

		(define (formal-names fn)
		  (flat-map
		    (lambda (f) (let ((ns (nf f 'names))) (if (pair? ns) ns '())))
		    (let ((ps (nf (nf fn 'type) 'params)))
		      (if (pair? ps) ps '()))))

		(define (subst-idents node mapping)
		  (ast-transform node
		    (lambda (n)
		      (and (tag? n 'ident)
		           (let ((e (assoc (nf n 'name) mapping)))
		             (and e (cdr e)))))))

		(define (build-map fs as)
		  (if (or (null? fs) (null? as)) '()
		    (cons (cons (car fs) (car as))
		          (build-map (cdr fs) (cdr as)))))

		;; Single-expr return body -> expression
		(define (single-ret-expr fn)
		  (let* ((stmts (nf (nf fn 'body) 'list)))
		    (and (pair? stmts) (= (length stmts) 1)
		         (tag? (car stmts) 'return-stmt)
		         (let ((rs (nf (car stmts) 'results)))
		           (and (pair? rs) (= (length rs) 1) (car rs))))))

		;; Phase 1: inline single-expression callees
		(define (inline-expr node)
		  (ast-transform node
		    (lambda (n)
		      (and (tag? n 'call-expr)
		           (let* ((fun (nf n 'fun))
		                  (name (and (tag? fun 'ident) (nf fun 'name)))
		                  (callee (and name (find-func name)))
		                  (ret (and callee (single-ret-expr callee))))
		             (and ret
		                  (let ((m (build-map (formal-names callee)
		                                     (or (nf n 'args) '()))))
		                    (inline-expr (subst-idents ret m)))))))))

		;; Phase 2: inline multi-statement callees via restructuring
		(define target (find-func "compute"))
		(define phase1 (inline-expr target))

		(define phase2-body
		  (ast-splice (nf (nf phase1 'body) 'list)
		    (lambda (stmt)
		      (and (tag? stmt 'assign-stmt)
		           (let* ((rhs (nf stmt 'rhs))
		                  (call (and (pair? rhs) (car rhs)))
		                  (lhs (nf stmt 'lhs))
		                  (target-name (and (pair? lhs)
		                                   (tag? (car lhs) 'ident)
		                                   (nf (car lhs) 'name)))
		                  (callee-name (and (tag? call 'call-expr)
		                                   (let ((f (nf call 'fun)))
		                                     (and (tag? f 'ident) (nf f 'name)))))
		                  (callee (and callee-name (find-func callee-name))))
		             (and callee (not (single-ret-expr callee))
		                  (let* ((structured (go-cfg-to-structured (nf callee 'body)))
		                         (m (build-map (formal-names callee)
		                                      (or (nf call 'args) '())))
		                         (substituted (subst-idents structured m))
		                         (rewritten (ast-transform substituted
		                                      (lambda (n)
		                                        (and (tag? n 'return-stmt)
		                                             (list 'assign-stmt
		                                                   (cons 'lhs (list (list 'ident (cons 'name target-name))))
		                                                   (cons 'tok '=)
		                                                   (cons 'rhs (nf n 'results))))))))
		                    (nf rewritten 'list))))))))

		;; Rebuild function with new body
		(define phase2
		  (ast-transform phase1
		    (lambda (n)
		      (and (tag? n 'block)
		           (equal? (nf n 'list) (nf (nf phase1 'body) 'list))
		           (list 'block (cons 'list phase2-body))))))

		(go-format phase2)
	`)

	c := qt.New(t)
	s := result.Internal().(*values.String).Value

	// Phase 1: double and addOne should be inlined
	c.Assert(strings.Contains(s, "double"), qt.IsFalse,
		qt.Commentf("double should be inlined, got:\n%s", s))
	c.Assert(strings.Contains(s, "addOne"), qt.IsFalse,
		qt.Commentf("addOne should be inlined, got:\n%s", s))

	// Phase 2: clamp should be inlined with correct control flow
	c.Assert(strings.Contains(s, "clamp"), qt.IsFalse,
		qt.Commentf("clamp should be inlined, got:\n%s", s))
	c.Assert(strings.Contains(s, "else"), qt.IsTrue,
		qt.Commentf("should have if/else structure, got:\n%s", s))
}
