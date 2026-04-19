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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
)

// TestAxisBSmoke runs the wile-axis-b script against a 4-entry fixture
// manifest and asserts the raw output contains bucket classifications for
// all four primitives. Regression guard: if the script crashes, misses
// entries, or changes output format, this test surfaces it.
//
// Uses `go run` rather than requiring a pre-built binary — slower but
// self-contained.
func TestAxisBSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke test runs go build + script — slow")
	}
	c := qt.New(t)

	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..")
	fixture := filepath.Join(repoRoot, "goastssa", "testdata", "axis-b-fixture-manifest.scm")

	_, err := os.Stat(fixture)
	c.Assert(err, qt.IsNil, qt.Commentf("fixture missing: %s", fixture))

	rawOut := filepath.Join(t.TempDir(), "axis-b-raw.scm")
	invOut := filepath.Join(t.TempDir(), "axis-b-inventory.md")

	cmd := exec.Command("go", "run", "./cmd/wile-goast", "--run", "wile-axis-b")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"WILE_AXIS_B_MANIFEST="+fixture,
		"WILE_AXIS_B_RAW_OUTPUT="+rawOut,
		"WILE_AXIS_B_INVENTORY="+invOut,
	)
	output, err := cmd.CombinedOutput()
	c.Assert(err, qt.IsNil, qt.Commentf("script failed: %s", string(output)))

	raw, err := os.ReadFile(rawOut)
	c.Assert(err, qt.IsNil)
	rawStr := string(raw)

	// Each fixture primitive should appear as a (primitive (name "X") ...)
	// block in the raw output. Don't pin exact bucket — that's subject to
	// narrowing changes — just assert each primitive was analyzed.
	for _, name := range []string{"cons", "null?", "length", "car"} {
		c.Assert(strings.Contains(rawStr, `(name "`+name+`")`), qt.IsTrue,
			qt.Commentf("primitive %q missing from raw output", name))
	}

	// Stdout summary should show bucket counts. Easiest invariant: the
	// word "Summary" appears and the fixture's total is 4.
	c.Assert(strings.Contains(string(output), "4 primitives"), qt.IsTrue,
		qt.Commentf("stdout did not report 4 primitives: %s", string(output)))
}
