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

func TestRenderCategory(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast provenance))`)

	t.Run("header has label and count", func(t *testing.T) {
		out := eval(t, engine, `
			(render-category "dogs"
			  (list (make-finding 'spaniel "a.go:1:1" 'barks #f)
			        (make-finding 'beagle  "b.go:2:1" 'barks #f)))
		`).SchemeString()
		c.Assert(strings.Contains(out, "dogs (2)"), qt.IsTrue, qt.Commentf("%s", out))
	})

	t.Run("one render-finding line per member, located", func(t *testing.T) {
		out := eval(t, engine, `
			(render-category "dogs"
			  (list (make-finding 'spaniel "a.go:1:1" 'barks #f)))
		`).SchemeString()
		c.Assert(strings.Contains(out, "a.go:1:1 — barks"), qt.IsTrue, qt.Commentf("%s", out))
	})

	t.Run("unlocated member shows <unlocated>; score shown when present", func(t *testing.T) {
		out := eval(t, engine, `
			(render-category "scored"
			  (list (make-finding 'x #f 'why 3/4)))
		`).SchemeString()
		c.Assert(strings.Contains(out, "<unlocated> — why [3/4]"), qt.IsTrue, qt.Commentf("%s", out))
	})

	t.Run("empty category renders just the header", func(t *testing.T) {
		out := eval(t, engine, `(render-category "none" (list))`).SchemeString()
		c.Assert(out, qt.Equals, `"none (0)"`)
	})
}
