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

package goastssa

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"golang.org/x/tools/go/ssa"
)

func TestWrapUnwrapSSAFunctionRef(t *testing.T) {
	c := qt.New(t)
	fn := &ssa.Function{}
	wrapped := WrapSSAFunctionRef(fn)
	c.Assert(wrapped, qt.IsNotNil)

	got, ok := UnwrapSSAFunctionRef(wrapped)
	c.Assert(ok, qt.IsTrue)
	c.Assert(got, qt.Equals, fn)
}

func TestUnwrapSSAFunctionRefWrongTag(t *testing.T) {
	c := qt.New(t)
	_, ok := UnwrapSSAFunctionRef(nil)
	c.Assert(ok, qt.IsFalse)
}
