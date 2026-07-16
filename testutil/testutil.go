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

// Package testutil provides shared test helpers for wile-goast extension tests.
package testutil

import (
	"context"
	"testing"

	"github.com/aalpar/wile/pkg/wile"

	qt "github.com/frankban/quicktest"
)

// RunScheme runs Scheme code (possibly multiple expressions) and returns the last result.
func RunScheme(t *testing.T, engine *wile.Engine, code string) wile.Value {
	t.Helper()
	result, err := engine.EvalMultiple(context.Background(), code)
	qt.New(t).Assert(err, qt.IsNil)
	return result
}

// RunSchemeExpectError runs Scheme code and asserts that it produces an error.
//
// Uses EvalMultiple, NOT Parse+Eval: Parse accepts only ONE expression, and an
// earlier version treated any parse error as the expected error -- so any
// multi-form body silently passed WITHOUT running the code under test.
func RunSchemeExpectError(t *testing.T, engine *wile.Engine, code string) {
	t.Helper()
	_, err := engine.EvalMultiple(context.Background(), code)
	qt.New(t).Assert(err, qt.IsNotNil)
}
