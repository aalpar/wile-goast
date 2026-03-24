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

func TestGoSession_SchemeString(t *testing.T) {
	c := qt.New(t)
	s := goast.NewGoSession([]string{"my/pkg"}, nil, nil, false)
	c.Assert(s.SchemeString(), qt.Matches, `#<go-session.*my/pkg.*>`)
}

func TestGoSession_IsVoid(t *testing.T) {
	c := qt.New(t)
	s := goast.NewGoSession([]string{"my/pkg"}, nil, nil, false)
	c.Assert(s.IsVoid(), qt.IsFalse)
	var nilSession *goast.GoSession
	c.Assert(nilSession.IsVoid(), qt.IsTrue)
}

func TestGoSession_EqualTo(t *testing.T) {
	c := qt.New(t)
	s1 := goast.NewGoSession([]string{"my/pkg"}, nil, nil, false)
	s2 := goast.NewGoSession([]string{"my/pkg"}, nil, nil, false)
	c.Assert(s1.EqualTo(s1), qt.IsTrue)
	c.Assert(s1.EqualTo(s2), qt.IsFalse) // identity, not structural
}

func TestGoSession_OpaqueTag(t *testing.T) {
	c := qt.New(t)
	s := goast.NewGoSession([]string{"my/pkg"}, nil, nil, false)
	c.Assert(s.OpaqueTag(), qt.Equals, "go-session")
}

func TestGoSession_ImplementsValue(t *testing.T) {
	var _ values.Value = (*goast.GoSession)(nil)
}

func TestGoSession_ImplementsOpaque(t *testing.T) {
	var _ values.Opaque = (*goast.GoSession)(nil)
}
