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
	"reflect"
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
			tc, ok := mcp.AsTextContent(res.Content[0])
			if ok {
				t.Fatalf("tool %s reported error: %s", name, tc.Text)
			}
		}
		t.Fatalf("tool %s reported error (no text content)", name)
	}
	m, ok := res.StructuredContent.(map[string]any)
	if ok {
		return m
	}
	if len(res.Content) > 0 {
		tc, ok := mcp.AsTextContent(res.Content[0])
		if ok {
			var parsed map[string]any
			err := json.Unmarshal([]byte(tc.Text), &parsed)
			if err != nil {
				t.Fatalf("tool %s: parse text JSON %q: %v", name, tc.Text, err)
			}
			return parsed
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
	err := os.WriteFile(path, []byte(content), 0o644)
	if err != nil {
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

	// Pass idf_threshold to exercise the options path (alist -> plist).
	env := callTool(t, mc, "recommend_split", map[string]any{
		"target":        phase1Pkg,
		"idf_threshold": 0.5,
	})
	envelopeOK(t, env, 1.0)

	result := env["result"].(map[string]any)
	qt.Assert(t, result["confidence"], qt.Equals, "NONE")
}

// recommend_boundaries returns three Pareto frontiers (split/merge/extract).
// The phase1 fixture yields no candidates, but the keys must still be present.
func TestRecommendBoundaries_Phase1Fixture(t *testing.T) {
	mc := inProcessClient(t)

	env := callTool(t, mc, "recommend_boundaries", map[string]any{"target": phase1Pkg})
	envelopeOK(t, env, 1.0)

	result := env["result"].(map[string]any)
	qt.Assert(t, result["splits"], qt.Not(qt.IsNil))
	qt.Assert(t, result["merges"], qt.Not(qt.IsNil))
	qt.Assert(t, result["extracts"], qt.Not(qt.IsNil))
}

// find_false_boundaries returns the cross-boundary report. Counter and
// Cache share no fields, so no cross-boundary concept is expected — the
// test just verifies a valid envelope round-trips.
func TestFindFalseBoundaries_Phase1Fixture(t *testing.T) {
	mc := inProcessClient(t)

	env := callTool(t, mc, "find_false_boundaries", map[string]any{"target": phase1Pkg})
	envelopeOK(t, env, 1.0)
}

// dupclusterPkg is the proven duplicate-bearing fixture: clones.go holds the
// SumSlice/TotalSlice clone pair (share "sort"); dupcluster.go holds json/log
// clusters. See goast/dup_detect_test.go for the library-level proof.
const dupclusterPkg = "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"

// pipeline-find-duplicates must load (exercises the new .sld imports) and
// project the clone pair into the clean contract shape. Driven through the
// always-present eval tool so the Scheme layer is testable before the
// dedicated tool exists (Task 2). eval returns the value's SchemeString.
func TestPipelineFindDuplicates_EvalSmoke(t *testing.T) {
	mc := inProcessClient(t)
	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Name = "eval"
	req.Params.Arguments = map[string]any{
		"code": `(begin (import (wile goast pipelines))
		  (pipeline-find-duplicates "` + dupclusterPkg + `" (list)))`,
	}
	res, err := mc.CallTool(ctx, req)
	qt.Assert(t, err, qt.IsNil)
	qt.Assert(t, res.IsError, qt.IsFalse)
	tc, ok := mcp.AsTextContent(res.Content[0])
	qt.Assert(t, ok, qt.IsTrue)
	// The projected shape uses kebab-case in Scheme (marshaller snake-cases at
	// the JSON boundary, which this eval path does not cross).
	for _, want := range []string{"functions", "equiv-tier", "SumSlice", "TotalSlice"} {
		qt.Assert(t, strings.Contains(tc.Text, want), qt.IsTrue,
			qt.Commentf("eval output missing %q:\n%s", want, tc.Text))
	}
}

// find_duplicates surfaces the SSA/AST-verified clone pair from the dupcluster
// fixture: SumSlice/TotalSlice, both located, tier proven or structural.
func TestFindDuplicates_DupclusterFixture(t *testing.T) {
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_duplicates", map[string]any{"target": dupclusterPkg})
	envelopeOK(t, env, 1.0)

	prov := env["provenance"].(map[string]any)
	qt.Assert(t, prov["verdict_included"], qt.Equals, false)
	qt.Assert(t, prov["candidate_count"].(float64) > 0, qt.IsTrue)

	results := env["result"].([]any)
	clone := findCandidateWithPair(results, "SumSlice", "TotalSlice")
	qt.Assert(t, clone, qt.Not(qt.IsNil), qt.Commentf("clone pair not surfaced: %v", results))

	tier := clone["equiv_tier"].(string)
	qt.Assert(t, tier == "proven" || tier == "structural", qt.IsTrue,
		qt.Commentf("tier=%s", tier))

	funcs := clone["functions"].([]any)
	qt.Assert(t, len(funcs), qt.Equals, 2)
	for _, f := range funcs {
		fm := f.(map[string]any)
		pos, ok := fm["position"].(string)
		qt.Assert(t, ok, qt.IsTrue, qt.Commentf("position not a string: %v", fm["position"]))
		qt.Assert(t, strings.Contains(pos, "clones.go:"), qt.IsTrue, qt.Commentf("pos=%s", pos))
	}
	// Marshalling fidelity: measures is a nested object of numbers; equiv_tier
	// is a string (symbol round-trip). No verdict field by default.
	measures := clone["measures"].(map[string]any)
	_, hasBenefit := measures["benefit"]
	qt.Assert(t, hasBenefit, qt.IsTrue)
	_, hasVerdict := clone["verdict"]
	qt.Assert(t, hasVerdict, qt.IsFalse)
}

// verdict is opt-in: absent by default (asserted in Task 2), present and
// well-formed when verdict:true. The proven/structural clone pair maps to
// duplicate or likely-duplicate.
func TestFindDuplicates_VerdictOptIn(t *testing.T) {
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_duplicates", map[string]any{
		"target":  dupclusterPkg,
		"verdict": true,
	})
	envelopeOK(t, env, 1.0)

	prov := env["provenance"].(map[string]any)
	qt.Assert(t, prov["verdict_included"], qt.Equals, true)

	results := env["result"].([]any)
	valid := map[string]bool{"duplicate": true, "likely-duplicate": true, "distinct": true}
	for _, r := range results {
		cand := r.(map[string]any)
		v, ok := cand["verdict"].(string)
		qt.Assert(t, ok, qt.IsTrue, qt.Commentf("verdict missing/not string: %v", cand))
		qt.Assert(t, valid[v], qt.IsTrue, qt.Commentf("bad verdict: %q", v))
	}

	clone := findCandidateWithPair(results, "SumSlice", "TotalSlice")
	qt.Assert(t, clone, qt.Not(qt.IsNil))
	cv := clone["verdict"].(string)
	qt.Assert(t, cv == "duplicate" || cv == "likely-duplicate", qt.IsTrue,
		qt.Commentf("clone verdict=%s", cv))
}

// True-negative / precision guard: unrelated functions in different reference
// clusters (SumSlice in the "sort" cluster, LogX in the "log" cluster) must
// never be paired as a candidate. Guards against a clustering regression that
// would lump unrelated functions together.
func TestFindDuplicates_TrueNegative(t *testing.T) {
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_duplicates", map[string]any{"target": dupclusterPkg})
	results := env["result"].([]any)
	qt.Assert(t, findCandidateWithPair(results, "SumSlice", "LogX"), qt.IsNil,
		qt.Commentf("unrelated functions paired: %v", results))
}

// Determinism: identical (target, threshold, verdict) -> structurally equal
// result. reflect.DeepEqual over the parsed result is the practical form of the
// design's "byte-identical" criterion.
func TestFindDuplicates_Deterministic(t *testing.T) {
	mc := inProcessClient(t)
	args := map[string]any{"target": dupclusterPkg}
	a := callTool(t, mc, "find_duplicates", args)
	b := callTool(t, mc, "find_duplicates", args)
	qt.Assert(t, reflect.DeepEqual(a["result"], b["result"]), qt.IsTrue,
		qt.Commentf("non-deterministic result:\n a=%v\n b=%v", a["result"], b["result"]))
}

// findCandidateWithPair returns the first candidate whose functions[].name set
// contains both a and b, or nil. Used by the find_duplicates tests.
func findCandidateWithPair(results []any, a, b string) map[string]any {
	for _, r := range results {
		cand, ok := r.(map[string]any)
		if !ok {
			continue
		}
		funcs, ok := cand["functions"].([]any)
		if !ok {
			continue
		}
		names := map[string]bool{}
		for _, f := range funcs {
			if fm, ok := f.(map[string]any); ok {
				if n, ok := fm["name"].(string); ok {
					names[n] = true
				}
			}
		}
		if names[a] && names[b] {
			return cand
		}
	}
	return nil
}

// All six Phase 1 tools must be advertised by the server (alongside the
// always-present eval tool), over the same registration path stdio and
// HTTP use.
func TestPhase1ToolsRegistered(t *testing.T) {
	mc := inProcessClient(t)

	res, err := mc.ListTools(context.Background(), mcp.ListToolsRequest{})
	qt.Assert(t, err, qt.IsNil)

	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{
		"check_beliefs", "discover_beliefs",
		"recommend_split", "recommend_boundaries", "find_false_boundaries",
		"find_duplicates",
	} {
		qt.Assert(t, names[want], qt.IsTrue, qt.Commentf("missing tool: %s", want))
	}
}
