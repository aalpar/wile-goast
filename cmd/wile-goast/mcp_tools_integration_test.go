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
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	qt "github.com/frankban/quicktest"
)

// inProcessClient returns a ready-to-use in-process MCP client backed by
// the real protocol server (ms.newServer), so the test exercises the
// actual registration path — eval tool, Phase 1 tools, prompts, and the
// session-cleanup hook — exactly as stdio/HTTP build it.
//
// ms.closeAll is registered before the client cleanup so cleanups run
// LIFO: the client closes first, then the server and its engines.
func inProcessClient(t *testing.T) *client.Client {
	t.Helper()
	ms := &mcpServer{}
	t.Cleanup(ms.closeAll)

	s, err := ms.newServer()
	qt.Assert(t, err, qt.IsNil)

	c, err := client.NewInProcessClient(s)
	qt.Assert(t, err, qt.IsNil)
	t.Cleanup(func() { _ = c.Close() })

	ctx := context.Background()
	err = c.Start(ctx)
	qt.Assert(t, err, qt.IsNil)

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "1.0.0"}
	_, err = c.Initialize(ctx, initReq)
	qt.Assert(t, err, qt.IsNil)
	return c
}

// callTool calls a tool and returns its result as a generic map. Tools
// return JSON via NewToolResultJSON, which populates both the text
// content (JSON string) and structuredContent. structuredContent is the
// canonical typed form; if the transport doesn't surface it as a map,
// fall back to parsing the text JSON so callers get a stable shape.
func callTool(t *testing.T, c *client.Client, name string, args map[string]any) map[string]any {
	t.Helper()
	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := c.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if res.IsError {
		if len(res.Content) > 0 {
			if tc, ok := mcp.AsTextContent(res.Content[0]); ok {
				t.Fatalf("tool %s reported error: %s", name, tc.Text)
			}
		}
		t.Fatalf("tool %s reported error (no text content)", name)
	}
	if m, ok := res.StructuredContent.(map[string]any); ok {
		return m
	}
	if len(res.Content) > 0 {
		if tc, ok := mcp.AsTextContent(res.Content[0]); ok {
			var m map[string]any
			if err := json.Unmarshal([]byte(tc.Text), &m); err != nil {
				t.Fatalf("tool %s: parse text JSON %q: %v", name, tc.Text, err)
			}
			return m
		}
	}
	t.Fatalf("tool %s returned neither structured map nor text JSON (%T)",
		name, res.StructuredContent)
	return nil
}

// envelopeOK fails the test if the returned envelope is missing the
// version or provenance/result keys, or if version differs from
// expected. Leaves result-shape assertions to per-tool tests.
//
// expectedVersion is float64 because encoding/json decodes numbers into
// float64 when unmarshalling into any; "version": 1 parses back as 1.0.
func envelopeOK(t *testing.T, envelope map[string]any, expectedVersion float64) {
	t.Helper()
	qt.Assert(t, envelope["version"], qt.Equals, expectedVersion)
	qt.Assert(t, envelope["provenance"], qt.Not(qt.IsNil))
	qt.Assert(t, envelope["result"], qt.Not(qt.IsNil))
}

// TestInProcessClientInitializes proves the shared protocol server
// builds and an in-process client can complete the MCP handshake. The
// Phase 1 tool tests (Tasks 2-6) build on this harness; for now it just
// confirms the always-present eval tool is registered.
func TestInProcessClientInitializes(t *testing.T) {
	mc := inProcessClient(t)
	res, err := mc.ListTools(context.Background(), mcp.ListToolsRequest{})
	qt.Assert(t, err, qt.IsNil)

	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	qt.Assert(t, names["eval"], qt.IsTrue)
}

// mustWriteFile writes content to path, failing the test on error.
// Package-local to cmd/wile-goast (the goast package has its own copy).
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// phase1Pkg is the import path of the Phase 1 integration test fixture.
const phase1Pkg = "github.com/aalpar/wile-goast/cmd/wile-goast/testdata/phase1"

// check_beliefs must load a directory of .scm beliefs, run them against
// the target, and report per-belief results in the envelope. Both Counter
// methods pair Lock with a deferred Unlock, so the lock-pairing belief is
// strong.
func TestCheckBeliefs_LockPairing(t *testing.T) {
	mc := inProcessClient(t)

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "lock.scm"), `
		(import (wile goast belief))
		(define-belief "lock-unlock"
		  (sites (functions-matching (contains-call "Lock")))
		  (expect (paired-with "Lock" "Unlock"))
		  (threshold 0.9 1))
	`)

	env := callTool(t, mc, "check_beliefs", map[string]any{
		"target":       phase1Pkg,
		"beliefs_path": dir,
	})
	envelopeOK(t, env, 1.0)

	prov := env["provenance"].(map[string]any)
	qt.Assert(t, prov["belief_count"], qt.Equals, float64(1))

	results := env["result"].([]any)
	qt.Assert(t, len(results) > 0, qt.IsTrue)
	first := results[0].(map[string]any)
	qt.Assert(t, first["status"], qt.Equals, "strong")
}

// discover_beliefs runs discovery beliefs against a target, suppresses
// any matching a committed belief, and returns the survivors as Scheme
// source ready to commit. With no committed beliefs (empty dir), every
// strong discovery belief should appear in the emitted source.
func TestDiscoverBeliefs_EmitsFiltered(t *testing.T) {
	mc := inProcessClient(t)

	discoveryDir := t.TempDir()
	mustWriteFile(t, filepath.Join(discoveryDir, "discovery.scm"), `
		(import (wile goast belief))
		(import (wile goast utils))
		(define-belief "methods-have-body"
		  (sites (functions-matching (name-matches "")))
		  (expect (custom (lambda (site ctx)
		    (if (nf site 'body) 'has-body 'no-body))))
		  (threshold 0.9 1))
	`)
	committedDir := t.TempDir() // empty: no suppression

	env := callTool(t, mc, "discover_beliefs", map[string]any{
		"target":         phase1Pkg,
		"discovery_path": discoveryDir,
		"committed_path": committedDir,
	})
	envelopeOK(t, env, 1.0)

	result := env["result"].(map[string]any)
	emitted := result["emitted_source"].(string)
	qt.Assert(t, strings.Contains(emitted, "define-belief"), qt.IsTrue)
	qt.Assert(t, strings.Contains(emitted, "methods-have-body"), qt.IsTrue)
}

// recommend_split analyzes package cohesion and returns a split proposal
// with a confidence verdict. The phase1 fixture is too small and cohesive
// to split (no incomparable concepts), so confidence is NONE.
func TestRecommendSplit_Phase1Fixture(t *testing.T) {
	mc := inProcessClient(t)

	env := callTool(t, mc, "recommend_split", map[string]any{"target": phase1Pkg})
	envelopeOK(t, env, 1.0)

	result := env["result"].(map[string]any)
	qt.Assert(t, result["confidence"], qt.Equals, "NONE")
}
