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

func TestPairedWithEvidence(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing"
	out := eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast provenance))
		(reset-beliefs!)
		(define-belief "lock-pairing"
		  (sites (functions-matching (contains-call "Lock")))
		  (expect (paired-with "Lock" "Unlock"))
		  (threshold 0.5 1))
		(define res (car (run-beliefs "`+pkg+`")))
		(render-category "lock-pairing" (cdr (assoc 'findings res)))
	`).SchemeString()

	t.Run("findings located at pairing.go", func(t *testing.T) {
		c.Assert(strings.Contains(out, "pairing.go"), qt.IsTrue, qt.Commentf("%s", out))
	})
	t.Run("why carries paired relation and the Unlock op", func(t *testing.T) {
		c.Assert(strings.Contains(out, "paired"), qt.IsTrue, qt.Commentf("%s", out))
		c.Assert(strings.Contains(out, "Unlock"), qt.IsTrue, qt.Commentf("%s", out))
	})
}

func TestCoMutatedEvidence(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/comutation"
	out := eval(t, engine, `
		(import (wile goast belief))
		(import (wile goast provenance))
		(reset-beliefs!)
		(define-belief "config-comutation"
		  (sites (functions-matching (stores-to-fields "Config" "Host")))
		  (expect (co-mutated "Host" "Port" "Timeout"))
		  (threshold 0.5 1))
		(define res (car (run-beliefs "`+pkg+`")))
		(render-category "config-comutation" (cdr (assoc 'findings res)))
	`).SchemeString()

	t.Run("findings located at comutation.go with co-mutated why", func(t *testing.T) {
		c.Assert(strings.Contains(out, "comutation.go"), qt.IsTrue, qt.Commentf("%s", out))
		c.Assert(strings.Contains(out, "co-mutated"), qt.IsTrue, qt.Commentf("%s", out))
	})
}
