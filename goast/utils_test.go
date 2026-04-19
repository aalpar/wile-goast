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

// Tests for (wile goast utils): shared traversal + list utilities.
// Covered indirectly by nearly every other Scheme library test, but
// explicit exercises here guard against export-shape regressions
// (e.g., accidentally dropping filter or opt-ref from utils.sld).

package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestUtils_NfAndTag(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	c.Assert(
		eval(t, engine, `(nf '(func-decl (name . "F")) 'name)`).SchemeString(),
		qt.Equals, `"F"`)
	c.Assert(
		eval(t, engine, `(nf '(func-decl (name . "F")) 'missing)`).SchemeString(),
		qt.Equals, "#f")
	c.Assert(
		eval(t, engine, `(tag? '(func-decl (name . "F")) 'func-decl)`).SchemeString(),
		qt.Equals, "#t")
	c.Assert(
		eval(t, engine, `(tag? '(func-decl (name . "F")) 'other)`).SchemeString(),
		qt.Equals, "#f")
}

func TestUtils_FilterKeepsFalseElements(t *testing.T) {
	// The named-let filter in utils correctly keeps #f elements when the
	// predicate accepts them. The filter-map wrapper idiom (removed in
	// the 2026-04-19 dedup refactor) silently dropped them.
	c := qt.New(t)
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	result := eval(t, engine, `(filter (lambda (x) (not x)) '(1 #f 2 #f 3))`)
	c.Assert(result.SchemeString(), qt.Equals, "(#f #f)")
}

func TestUtils_FilterMapAndFlatMap(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	c.Assert(
		eval(t, engine, `(filter-map (lambda (x) (and (> x 2) x)) '(1 2 3 4))`).SchemeString(),
		qt.Equals, "(3 4)")
	c.Assert(
		eval(t, engine, `(flat-map (lambda (x) (list x x)) '(1 2))`).SchemeString(),
		qt.Equals, "(1 1 2 2)")
}

func TestUtils_UniqueAndMember(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	c.Assert(
		eval(t, engine, `(unique '(a b a c b))`).SchemeString(),
		qt.Equals, "(a b c)")
	c.Assert(
		eval(t, engine, `(member? 'a '(a b c))`).SchemeString(),
		qt.Equals, "#t")
	c.Assert(
		eval(t, engine, `(member? 'x '(a b c))`).SchemeString(),
		qt.Equals, "#f")
}

func TestUtils_TakeDrop(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	c.Assert(
		eval(t, engine, `(take '(a b c d e) 3)`).SchemeString(),
		qt.Equals, "(a b c)")
	c.Assert(
		eval(t, engine, `(drop '(a b c d e) 3)`).SchemeString(),
		qt.Equals, "(d e)")
	c.Assert(
		eval(t, engine, `(take '(a b) 10)`).SchemeString(),
		qt.Equals, "(a b)")
	c.Assert(
		eval(t, engine, `(drop '(a b) 10)`).SchemeString(),
		qt.Equals, "()")
}

func TestUtils_OptRef(t *testing.T) {
	// opt-ref lives in utils after the 2026-04-19 dedup; belief.scm and
	// split.scm both consume it. Test the canonical contract here.
	c := qt.New(t)
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	c.Assert(
		eval(t, engine, `(opt-ref '(fuel 10 mode fast) 'fuel 5)`).SchemeString(),
		qt.Equals, "10")
	c.Assert(
		eval(t, engine, `(opt-ref '() 'fuel 5)`).SchemeString(),
		qt.Equals, "5")
	c.Assert(
		eval(t, engine, `(opt-ref '(mode fast) 'fuel 5)`).SchemeString(),
		qt.Equals, "5")
}

func TestUtils_OrderedPairs(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	c.Assert(
		eval(t, engine, `(ordered-pairs '(a b c))`).SchemeString(),
		qt.Equals, "((a b) (a c) (b c))")
	c.Assert(
		eval(t, engine, `(ordered-pairs '())`).SchemeString(),
		qt.Equals, "()")
}

func TestUtils_Walk(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	result := eval(t, engine, `
		(walk '(file (decls . ((func-decl (name . "A"))
		                       (func-decl (name . "B")))))
		      (lambda (n) (and (tag? n 'func-decl) (nf n 'name))))
	`)
	c.Assert(result.SchemeString(), qt.Equals, `("A" "B")`)
}
