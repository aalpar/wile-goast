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

// The two real strings the globs are written against: an absolute position
// path and a qualified function name. Both tools carry the first in the same
// form; they disagree on the second, which is why the suffix rule matches at
// ANY separator boundary rather than only the last one.
const (
	globPos     = "/Users/aalpar/projects/wile-workspace/wile-goast/goast/marshal.go:568:1"
	globDupName = "ssaMapper.mapChangeInterface"
	globFbName  = "(*github.com/aalpar/wile-goast/goast.loopRewriter).replaceInStmts"
	globPlainFn = "github.com/aalpar/wile-goast/goast.cloneTypeExpr"
)

func TestGlobMatch_Scope(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	cases := []struct {
		pat  string
		want string
	}{
		// Whole-string match.
		{"/Users/*/wile-goast/goast/marshal.go", "#t"},
		{"*/goast/*.go", "#t"},
		// Suffix at a separator boundary — the common caller inputs.
		{"marshal.go", "#t"},
		{"goast/marshal.go", "#t"},
		{"wile-goast/goast/marshal.go", "#t"},
		{"*.go", "#t"},
		// A last-segment-only rule would pass "marshal.go" and fail this;
		// this case is why the rule matches at every boundary.
		{"goast/*.go", "#t"},
		// Not aligned to a separator: "st/marshal.go" is a suffix of the
		// string but does not begin just after a '/'.
		{"st/marshal.go", "#f"},
		{"other.go", "#f"},
		{"goast/other.go", "#f"},
	}
	for _, tc := range cases {
		got := eval(t, engine, `(glob-match? "`+tc.pat+`" (position-file "`+globPos+`") #\/)`)
		qt.Assert(t, got.SchemeString(), qt.Equals, tc.want,
			qt.Commentf("scope glob %q", tc.pat))
	}
}

func TestGlobMatch_Symbol(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	cases := []struct {
		pat, subject, want string
	}{
		// find_duplicates names: Type.Method or bare.
		{"mapChangeInterface", globDupName, "#t"},
		{"ssaMapper.*", globDupName, "#t"},
		{"ssaMapper.mapChangeInterface", globDupName, "#t"},
		{"*map*", globDupName, "#t"},
		{"SumSlice", "SumSlice", "#t"},
		{"changeInterface", globDupName, "#f"}, // not at a '.' boundary
		// find_false_boundaries names: fully package-qualified SSA form.
		// The same short pattern must work despite the qualification.
		{"cloneTypeExpr", globPlainFn, "#t"},
		{"replaceInStmts", globFbName, "#t"},
		{"*loopRewriter*", globFbName, "#t"},
		{"loopRewriter.*", globFbName, "#f"}, // the qualified form has ")." not "."
		{"restructureBackwardGotos", globPlainFn, "#f"},
	}
	for _, tc := range cases {
		got := eval(t, engine, `(glob-match? "`+tc.pat+`" "`+tc.subject+`" #\.)`)
		qt.Assert(t, got.SchemeString(), qt.Equals, tc.want,
			qt.Commentf("symbol glob %q against %q", tc.pat, tc.subject))
	}
}

// A literal '*' in the subject can only ever be consumed by the pattern's
// wildcard (one character at a time via backtracking, or as part of a wider
// span): PAT has no way to hold a literal '*' of its own, since a '*' in PAT
// is always tested as the wildcard before literal equality is tried. A
// non-string subject (an unlocated row) matches nothing rather than erroring.
func TestGlobMatch_EdgeCases(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	for expr, want := range map[string]string{
		`(glob-match? "*" "anything/at/all" #\/)`: "#t",
		`(glob-match? "" "" #\/)`:                 "#t",
		`(glob-match? "" "abc" #\/)`:              "#f",
		`(glob-match? "a*c" "abc" #\/)`:           "#t",
		`(glob-match? "a*c" "ac" #\/)`:            "#t",
		`(glob-match? "a*c" "abd" #\/)`:           "#f",
		`(glob-match? "a**c" "abbbc" #\/)`:        "#t",
		`(glob-match? "*x" "ax" #\/)`:             "#t",
		`(glob-match? "x*" "xa" #\/)`:             "#t",
		`(glob-match? "abc" #f #\/)`:              "#f",
		// The subject itself contains a literal '*' character: the
		// wildcard's one-character backtrack step consumes it like any
		// other character, there being no way to write a literal '*' in
		// PAT to match it "as itself".
		`(glob-match? "a*c" "a*c" #\/)`: "#t",
		// Same, but the wildcard's span covers several characters,
		// the literal '*' among them.
		`(glob-match? "a*c" "ab*c" #\/)`: "#t",
		// Control: with no wildcard in PAT at all, a subject '*' is just
		// another character PAT must match literally (impossible for "abc"
		// against a 4-byte subject), so it does not slip through unmatched.
		`(glob-match? "abc" "ab*c" #\/)`: "#f",
	} {
		got := eval(t, engine, expr)
		qt.Assert(t, got.SchemeString(), qt.Equals, want, qt.Commentf("%s", expr))
	}
}

func TestPositionFile(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	for in, want := range map[string]string{
		"/a/b.go:568:1": "/a/b.go",
		"/a/b.go:1:1":   "/a/b.go",
		"marshal.go":    "marshal.go",   // already a bare path
		"/a/b.go:568:":  "/a/b.go:568:", // malformed: left alone
		"/a/b.go:568":   "/a/b.go:568",  // only one number: left alone
		"":              "",
	} {
		got := eval(t, engine, `(position-file "`+in+`")`)
		qt.Assert(t, got.SchemeString(), qt.Equals, `"`+want+`"`,
			qt.Commentf("position-file %q", in))
	}
}
