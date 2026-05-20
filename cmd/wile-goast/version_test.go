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

package main

import (
	"bytes"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
)

// TestDoVersionPrintsBothLines exercises the ldflags-injected path: when
// BuildVersion is populated the output mirrors `make build` output.
func TestDoVersionPrintsBothLines(t *testing.T) {
	c := qt.New(t)

	origVersion, origSHA := BuildVersion, BuildSHA
	t.Cleanup(func() {
		BuildVersion = origVersion
		BuildSHA = origSHA
	})

	BuildVersion = "v0.5.200"
	BuildSHA = "abc1234"

	var buf bytes.Buffer
	doVersion(&buf)
	out := buf.String()

	c.Assert(out, qt.Contains, "wile-goast v0.5.200 (abc1234)\n")
	// Wile line is always present; its value depends on the test binary's
	// build info (workspace builds show "(devel)", module builds show the
	// pinned tag). Assert structure, not value.
	c.Assert(strings.HasSuffix(out, "\n"), qt.IsTrue)
	c.Assert(strings.Count(out, "\n"), qt.Equals, 2)
	c.Assert(out, qt.Contains, "Wile Scheme ")
}

// TestDoVersionWithoutLdflagsFallsBackToDev guards the empty-BuildVersion
// path: when no ldflags were injected and BuildInfo is unhelpful, the
// output still names the program rather than printing a blank line.
func TestDoVersionWithoutLdflagsFallsBackToDev(t *testing.T) {
	c := qt.New(t)

	origVersion, origSHA := BuildVersion, BuildSHA
	t.Cleanup(func() {
		BuildVersion = origVersion
		BuildSHA = origSHA
	})

	BuildVersion = ""
	BuildSHA = ""

	var buf bytes.Buffer
	doVersion(&buf)
	out := buf.String()

	// Either BuildInfo.Main.Version supplies a value or we fall back to "dev"
	// — but the first token after "wile-goast" must be non-empty.
	first := strings.SplitN(out, "\n", 2)[0]
	c.Assert(strings.HasPrefix(first, "wile-goast "), qt.IsTrue)
	c.Assert(len(first) > len("wile-goast "), qt.IsTrue)
}
