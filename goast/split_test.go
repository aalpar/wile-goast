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

func TestSplit_ImportSignatures(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(import (wile goast utils))

		(define refs (go-func-refs
		  "github.com/aalpar/wile-goast/goast/testdata/iface"))
		(define sigs (import-signatures refs))
	`)

	c := qt.New(t)

	t.Run("returns alist", func(t *testing.T) {
		result := eval(t, engine, `(pair? (car sigs))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("car is function name", func(t *testing.T) {
		result := eval(t, engine, `(string? (caar sigs))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("cdr is list of package paths", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((first-sig (car sigs)))
			  (or (null? (cdr first-sig))
			      (string? (cadr first-sig))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
