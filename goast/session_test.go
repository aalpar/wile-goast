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

	"github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile/values"

	qt "github.com/frankban/quicktest"
)

func TestWrapUnwrapSession(t *testing.T) {
	c := qt.New(t)
	s := goast.NewGoSession([]string{"my/pkg"}, nil, nil, false)

	wrapped := goast.WrapSession(s)
	c.Assert(wrapped.OpaqueTag(), qt.Equals, "go-session")

	unwrapped, ok := goast.UnwrapSession(wrapped)
	c.Assert(ok, qt.IsTrue)
	c.Assert(unwrapped, qt.Equals, s)
}

func TestUnwrapSession_WrongType(t *testing.T) {
	c := qt.New(t)
	_, ok := goast.UnwrapSession(values.NewString("not a session"))
	c.Assert(ok, qt.IsFalse)
}

func TestUnwrapSession_WrongTag(t *testing.T) {
	c := qt.New(t)
	other := values.NewOpaqueValue("something-else", 42)
	_, ok := goast.UnwrapSession(other)
	c.Assert(ok, qt.IsFalse)
}

func TestWrapSession_Identity(t *testing.T) {
	c := qt.New(t)
	s1 := goast.NewGoSession([]string{"a"}, nil, nil, false)
	s2 := goast.NewGoSession([]string{"a"}, nil, nil, false)
	w1 := goast.WrapSession(s1)
	w2 := goast.WrapSession(s2)
	// Different OpaqueValues are not equal (identity semantics).
	c.Assert(w1.EqualTo(w2), qt.IsFalse)
	c.Assert(w1.EqualTo(w1), qt.IsTrue)
}

func TestDispatchSessionOrPattern_Session(t *testing.T) {
	c := qt.New(t)
	s := goast.NewGoSession([]string{"x/y"}, nil, nil, false)
	wrapped := goast.WrapSession(s)

	var gotSession *goast.GoSession
	var patternCalls int
	err := goast.DispatchSessionOrPattern(wrapped, "go-test",
		func(sess *goast.GoSession) error { gotSession = sess; return nil },
		func(*values.String) error { patternCalls++; return nil })

	c.Assert(err, qt.IsNil)
	c.Assert(gotSession, qt.Equals, s)
	c.Assert(patternCalls, qt.Equals, 0)
}

func TestDispatchSessionOrPattern_Pattern(t *testing.T) {
	c := qt.New(t)
	pat := values.NewString("my/pkg/...")

	var sessionCalls int
	var gotPattern *values.String
	err := goast.DispatchSessionOrPattern(pat, "go-test",
		func(*goast.GoSession) error { sessionCalls++; return nil },
		func(p *values.String) error { gotPattern = p; return nil })

	c.Assert(err, qt.IsNil)
	c.Assert(sessionCalls, qt.Equals, 0)
	c.Assert(gotPattern, qt.Equals, pat)
}

func TestDispatchSessionOrPattern_WrongType(t *testing.T) {
	c := qt.New(t)
	arg := values.NewInteger(42)

	err := goast.DispatchSessionOrPattern(arg, "go-probe",
		func(*goast.GoSession) error { return nil },
		func(*values.String) error { return nil })

	c.Assert(err, qt.IsNotNil)
	c.Assert(err.Error(), qt.Contains, "go-probe")
	c.Assert(err.Error(), qt.Contains, "string or go-session")
}
