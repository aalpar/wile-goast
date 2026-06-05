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

func TestReceiverAsymmetryL1(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/recvasym"
	out := eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast provenance))
		(reset-beliefs!)
		(define-belief "recv-asym"
		  (sites (methods-of "Server"))
		  (expect (receiver-parameter-asymmetry))
		  (threshold 0 1))
		(define res (car (run-beliefs "`+pkg+`")))
		(render-category "recv-asym" (cdr (assoc 'findings res)))
	`).SchemeString()

	t.Run("all four categories present", func(t *testing.T) {
		for _, want := range []string{"candidate", "mutation", "accessor", "multi-read"} {
			c.Assert(strings.Contains(out, want), qt.IsTrue, qt.Commentf("missing %q in:\n%s", want, out))
		}
	})
	t.Run("candidate located with field+receiver why", func(t *testing.T) {
		c.Assert(strings.Contains(out, "recvasym.go"), qt.IsTrue, qt.Commentf("%s", out))
		c.Assert(strings.Contains(out, "receiver-asymmetry"), qt.IsTrue, qt.Commentf("%s", out))
		c.Assert(strings.Contains(out, "name"), qt.IsTrue, qt.Commentf("%s", out))
	})
}

func TestReceiverAsymmetryInterfaceExcluded(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/recvasym"
	out := eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast provenance))
		(reset-beliefs!)
		(define-belief "recv-asym-iface"
		  (sites (methods-of "Tag"))
		  (expect (receiver-parameter-asymmetry))
		  (threshold 0 1))
		(define res (car (run-beliefs "`+pkg+`")))
		(map finding-value (cdr (assoc 'findings res)))
	`).SchemeString()

	c.Assert(strings.Contains(out, "interface-method"), qt.IsTrue, qt.Commentf("%s", out))
	c.Assert(strings.Contains(out, "candidate"), qt.IsFalse, qt.Commentf("Render must not be a candidate:\n%s", out))
}
