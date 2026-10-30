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
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
	envelopeOK(t, env, 2.0)

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
	envelopeOK(t, env, 2.0)
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
// fixture: SumSlice/TotalSlice, both located. The pair is returned because it
// clears the similarity floor; that this particular fixture's pair also lands
// in tier proven or structural is a property of the fixture, asserted below,
// not the reason it appears.
func TestFindDuplicates_DupclusterFixture(t *testing.T) {
	mc := inProcessClient(t)
	// detail:"full" because this test asserts on the measures sub-object below,
	// which detail=summary (the default) drops.
	env := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg, "detail": "full"})
	envelopeOK(t, env, 4.0)

	prov := env["provenance"].(map[string]any)
	qt.Assert(t, prov["verdict_included"], qt.Equals, false)
	qt.Assert(t, prov["total_count"].(float64) > 0, qt.IsTrue)

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

	// The BigFloat->float64 score projection: score is a JSON number in (0,1],
	// and the top-level score, measures.similarity, and each function's score
	// are the same effective-similarity value.
	score, ok := clone["score"].(float64)
	qt.Assert(t, ok, qt.IsTrue, qt.Commentf("score not a float64: %v", clone["score"]))
	qt.Assert(t, score > 0 && score <= 1, qt.IsTrue, qt.Commentf("score out of range: %v", score))
	sim, ok := measures["similarity"].(float64)
	qt.Assert(t, ok, qt.IsTrue, qt.Commentf("similarity not a float64: %v", measures["similarity"]))
	qt.Assert(t, sim, qt.Equals, score)
	for _, f := range funcs {
		fm := f.(map[string]any)
		fs, ok := fm["score"].(float64)
		qt.Assert(t, ok, qt.IsTrue, qt.Commentf("function score not a float64: %v", fm["score"]))
		qt.Assert(t, fs, qt.Equals, score)
	}
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
	envelopeOK(t, env, 4.0)

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
			fm, ok := f.(map[string]any)
			if ok {
				n, ok := fm["name"].(string)
				if ok {
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

const nodupsPkg = "github.com/aalpar/wile-goast/cmd/wile-goast/testdata/nodups"

// Empty success: a package with no clusterable pairs returns a valid envelope
// with total_count 0 and an empty result array — not an error. "No
// duplicates found" is a useful answer.
func TestFindDuplicates_EmptySuccess(t *testing.T) {
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_duplicates", map[string]any{"target": nodupsPkg})
	envelopeOK(t, env, 4.0)
	prov := env["provenance"].(map[string]any)
	qt.Assert(t, prov["total_count"], qt.Equals, float64(0))
	qt.Assert(t, env["result"], qt.DeepEquals, []any{})
}

// Error path: a bad target propagates the Scheme load failure as an MCP tool
// error (isError), not a malformed envelope. Uses raw CallTool because the
// callTool helper fatals on isError.
func TestFindDuplicates_BadTarget(t *testing.T) {
	mc := inProcessClient(t)
	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Name = "find_duplicates"
	req.Params.Arguments = map[string]any{"target": "github.com/aalpar/wile-goast/NONEXISTENT"}
	res, err := mc.CallTool(ctx, req)
	qt.Assert(t, err, qt.IsNil) // transport ok; tool-level error is in the result
	qt.Assert(t, res.IsError, qt.IsTrue)
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

// wholeTreePkg yields 36474 candidate pairs, comfortably over the scoring
// budget, so it exercises the guard on real (not fixture) input. Note the
// budget is NOT exceeded by any single package here: goast, the largest,
// yields 16011 and is answered rather than refused.
const wholeTreePkg = "github.com/aalpar/wile-goast/..."

// find_duplicates must refuse a target past the scoring budget before paying
// the cost, and say so in terms the caller can act on. Discovery alone sizes
// the target, so the refusal lands in about a second against the minutes the
// scoring would take.
func TestFindDuplicates_PairCountGuard(t *testing.T) {
	mc := inProcessClient(t)

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Name = "find_duplicates"
	req.Params.Arguments = map[string]any{"target": wholeTreePkg}
	res, err := mc.CallTool(ctx, req)
	qt.Assert(t, err, qt.IsNil)
	qt.Assert(t, res.IsError, qt.IsTrue)

	tc, ok := mcp.AsTextContent(res.Content[0])
	qt.Assert(t, ok, qt.IsTrue)
	// The count is not pinned (it tracks the tree's own churn); the budget
	// and the remedy are, since those are what make the error actionable.
	for _, want := range []string{"candidate pairs", "25000", "narrow a '/...' tree"} {
		qt.Assert(t, strings.Contains(tc.Text, want), qt.IsTrue,
			qt.Commentf("guard message not actionable: %s", tc.Text))
	}
}

// goastPkg (16011 candidate pairs) is over the old 9992 projection ceiling
// and under the 25000 scoring budget. It is the regression pin for the
// ceiling having been lifted rather than merely raised — a real package the
// tool used to refuse and now answers. (goastssa served until candidate-pairs
// was deduplicated, which took it from 13941 pairs to 4457.)

// A target between the old ceiling and the current budget must be answered,
// not refused, and every returned pair must clear the similarity floor.
func TestFindDuplicates_AnswersPastOldCeiling(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: scores 16011 pairs, ~50s")
	}
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_duplicates", map[string]any{"target": goastPkg})
	envelopeOK(t, env, 4.0)

	prov := env["provenance"].(map[string]any)
	qt.Assert(t, prov["scored_pair_count"].(float64) > 9992, qt.IsTrue,
		qt.Commentf("target no longer exercises the old ceiling: %v", prov["scored_pair_count"]))

	// total_count is what cleared the similarity floor, so it is bounded by
	// the pairs scored and not derivable from the tier tally: threshold
	// selects in similarity units, tier is reported alongside.
	total := prov["total_count"].(float64)
	qt.Assert(t, total <= prov["scored_pair_count"].(float64), qt.IsTrue,
		qt.Commentf("more rows kept than scored: %v > %v", total, prov["scored_pair_count"]))
	// The result is the first page of `total`, sized by whatever limit the
	// envelope itself reports -- this asserts the relationship between
	// total_count, limit, returned_count and truncated, not a pinned default.
	limit := prov["limit"].(float64)
	qt.Assert(t, prov["returned_count"].(float64), qt.Equals, math.Min(total, limit))
	qt.Assert(t, float64(len(env["result"].([]any))), qt.Equals, prov["returned_count"])
	qt.Assert(t, prov["truncated"], qt.Equals, total > limit)

	threshold := prov["threshold"].(float64)
	for _, r := range env["result"].([]any) {
		row := r.(map[string]any)
		qt.Assert(t, row["score"].(float64) >= threshold, qt.IsTrue,
			qt.Commentf("row below the floor: score=%v threshold=%v", row["score"], threshold))
		// Every row still carries its adjudication; it just no longer gates.
		tier := row["equiv_tier"].(string)
		qt.Assert(t, tier == "proven" || tier == "structural" || tier == "divergent",
			qt.IsTrue, qt.Commentf("unknown tier: %s", tier))
	}
}

// goastcfgDupsPkg is a real package whose candidate pairs are ALL divergent:
// FCA clusters them by shared references, the unifier accepts none of them.
const goastcfgDupsPkg = "github.com/aalpar/wile-goast/goastcfg"

// A divergent pair above the similarity floor is a ROW, labelled with the
// verdict that rejected it -- not a count the caller never sees.
//
// unifiable? is categorical in its second clause (no remaining diff outside
// type/register), so a pair can sit at 0.87 similarity and still be
// divergent, and no threshold can retrieve it. Gating on the tier therefore
// reported total_count 0 on a package holding near-identical functions, which
// is indistinguishable from a package that never clustered. It also deleted
// the most useful rows the tool produces: a high-similarity divergent pair's
// structural diffs are where the two implementations actually disagree.
//
// The tier tally stays in provenance, so the adjudication is still legible;
// it just no longer decides what the caller is allowed to see.
func TestFindDuplicates_DivergentPairsAreReturnedAndLabelled(t *testing.T) {
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_duplicates", map[string]any{"target": goastcfgDupsPkg})
	envelopeOK(t, env, 4.0)

	prov := env["provenance"].(map[string]any)
	tiers := prov["tiers"].(map[string]any)
	qt.Assert(t, tiers["proven"], qt.Equals, float64(0))
	qt.Assert(t, tiers["structural"], qt.Equals, float64(0))
	qt.Assert(t, tiers["divergent"].(float64) > 0, qt.IsTrue,
		qt.Commentf("no rejected pairs recorded: %v", tiers))
	qt.Assert(t, prov["scored_pair_count"], qt.Equals, tiers["divergent"])

	// Every pair here is divergent, so anything returned is a divergent pair
	// that cleared the floor -- exactly the population the tier gate deleted.
	rows := env["result"].([]any)
	qt.Assert(t, len(rows) > 0, qt.IsTrue,
		qt.Commentf("divergent pairs above the floor were dropped: total=%v", prov["total_count"]))
	qt.Assert(t, prov["total_count"].(float64) > 0, qt.IsTrue)

	threshold := prov["threshold"].(float64)
	for _, r := range rows {
		row := r.(map[string]any)
		qt.Assert(t, row["equiv_tier"], qt.Equals, "divergent")
		qt.Assert(t, row["score"].(float64) >= threshold, qt.IsTrue,
			qt.Commentf("row below the floor: score=%v threshold=%v", row["score"], threshold))
	}
}

// The floor is the knob, and it must actually move: raising the threshold
// past every pair's similarity empties the result. This is what the tier gate
// could not do in reverse -- lowering it never admitted a divergent pair.
func TestFindDuplicates_ThresholdIsASimilarityFloor(t *testing.T) {
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_duplicates",
		map[string]any{"target": goastcfgDupsPkg, "threshold": 0.999})
	envelopeOK(t, env, 4.0)

	prov := env["provenance"].(map[string]any)
	qt.Assert(t, prov["total_count"], qt.Equals, float64(0))
	qt.Assert(t, env["result"], qt.DeepEquals, []any{})
	// The work still happened and is still reported.
	qt.Assert(t, prov["scored_pair_count"].(float64) > 0, qt.IsTrue)
}

// The guard must not fire on a target inside the ceiling: the dupcluster
// fixture still returns its clone pair.
func TestFindDuplicates_GuardAllowsSmallTarget(t *testing.T) {
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_duplicates", map[string]any{"target": dupclusterPkg})
	envelopeOK(t, env, 4.0)
	qt.Assert(t, len(env["result"].([]any)) > 0, qt.IsTrue)
}

// goastcfgPkg is a real package (unlike the phase1 fixture): 14 functions, so
// compute-idf's log(14/df) is a BigFloat for every df that does not divide 14.
const goastcfgPkg = "github.com/aalpar/wile-goast/goastcfg/..."

// containsFloat64 reports whether any leaf of a marshalled envelope is a
// float64. Shape-agnostic: high-idf keys are package-path strings, not
// symbols, so the alist marshals as dotted pairs rather than an object.
func containsFloat64(v any) bool {
	switch x := v.(type) {
	case float64:
		return true
	case map[string]any:
		for _, e := range x {
			if containsFloat64(e) {
				return true
			}
		}
	case []any:
		return slices.ContainsFunc(x, containsFloat64)
	}
	return false
}

// recommend_split must marshal a real package's report. The phase1 fixture
// passes by arithmetic accident (N=4, one high-IDF package at df=2, so 4/2 is
// an Integer and log returns a Float); any package with a non-dividing df
// produces a BigFloat and used to fail the whole envelope with
// "marshal: type=*values.BigFloat: unsupported wile value type".
func TestRecommendSplit_RealPackageBigFloatIDF(t *testing.T) {
	mc := inProcessClient(t)
	env := callTool(t, mc, "recommend_split", map[string]any{"target": goastcfgPkg})
	envelopeOK(t, env, 2.0)

	result := env["result"]
	qt.Assert(t, result, qt.Not(qt.IsNil))
	qt.Assert(t, containsFloat64(result), qt.IsTrue,
		qt.Commentf("no float64 leaf in result: %v", result))
}

// goastPkg is a real package (the phase1 fixture has no cross-boundary
// concept at all). Measured at plan time: 178 FCA objects, an 89-concept
// lattice, 5 cross-boundary concepts, ~1.7s.
const goastPkg = "github.com/aalpar/wile-goast/goast"

// Decision 2's regression pin. Pushing a symbol filter into the FCA context
// restricts the objects, recomputes the lattice over a different context, and
// reports absence where findings exist. Measured on ./goast, write-only:
// *restructure* keeps 3 objects and finds 0 concepts pushed down, against the
// 2 finished concepts whose extent contains restructureBackwardGotos. Telling
// a caller there are no false boundaries when there are is worse than
// declining, so this tool post-filters.
//
// Asserted as an invariant rather than a pinned count, because the extent
// names track goast's own churn: non-empty, a subset of the unfiltered rows,
// and every returned row genuinely matches. If goast changes such that no
// concept contains "restructure", replace the glob with another one that
// separates the two approaches (*attach* and *loopRewriter* also did at plan
// time) -- do not "fix" it by pushing the filter down.
func TestFindFalseBoundaries_PostFilterPreservesFindings(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: builds an 89-concept lattice over ./goast")
	}
	mc := inProcessClient(t)

	all := callTool(t, mc, "find_false_boundaries", map[string]any{"target": goastPkg})
	envelopeOK(t, all, 2.0)
	allRows := all["result"].([]any)
	qt.Assert(t, len(allRows) > 0, qt.IsTrue,
		qt.Commentf("fixture no longer produces cross-boundary concepts"))

	scoped := callTool(t, mc, "find_false_boundaries", map[string]any{
		"target": goastPkg,
		"symbol": "*restructure*",
	})
	envelopeOK(t, scoped, 2.0)
	rows := scoped["result"].([]any)

	qt.Assert(t, len(rows) > 0, qt.IsTrue,
		qt.Commentf("post-filter reported absence where findings exist: %v", rows))
	qt.Assert(t, len(rows) < len(allRows), qt.IsTrue,
		qt.Commentf("filter did not narrow: %d of %d", len(rows), len(allRows)))

	// Every returned row really matches, and is one of the unfiltered rows.
	unfiltered := map[string]bool{}
	for _, r := range allRows {
		unfiltered[extentKey(r)] = true
	}
	for _, r := range rows {
		qt.Assert(t, unfiltered[extentKey(r)], qt.IsTrue,
			qt.Commentf("row absent from the unfiltered result: %v", r))
		matched := false
		for _, n := range r.(map[string]any)["extent"].([]any) {
			if strings.Contains(n.(string), "restructure") {
				matched = true
			}
		}
		qt.Assert(t, matched, qt.IsTrue,
			qt.Commentf("row has no matching extent member: %v", r))
	}

	// The provenance is self-describing: cross_boundary_count is the
	// pre-filter population, total_count the post-filter one.
	prov := scoped["provenance"].(map[string]any)
	qt.Assert(t, prov["symbol"], qt.Equals, "*restructure*")
	qt.Assert(t, prov["total_count"].(float64), qt.Equals, float64(len(rows)))
	qt.Assert(t, prov["cross_boundary_count"].(float64) > prov["total_count"].(float64),
		qt.IsTrue, qt.Commentf("prov=%v", prov))
}

// extentKey joins a boundary row's extent into a stable identity string.
func extentKey(row any) string {
	ext, ok := row.(map[string]any)["extent"].([]any)
	if !ok {
		return ""
	}
	names := make([]string, 0, len(ext))
	for _, n := range ext {
		s, _ := n.(string)
		names = append(names, s)
	}
	return strings.Join(names, "|")
}

// Row WIDTH, not row count, is this tool's failure mode: measured on ./goast
// the three lattice-relationship lists are 90.5% of the result bytes
// (incomparable_with alone is 60.6%). Both levels carry types_count; full
// additionally carries the three lattice-relationship lists that summary
// drops.
func TestFindFalseBoundaries_DetailFieldSets(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: builds an 89-concept lattice over ./goast")
	}
	mc := inProcessClient(t)

	summary := callTool(t, mc, "find_false_boundaries", map[string]any{"target": goastPkg})
	full := callTool(t, mc, "find_false_boundaries", map[string]any{
		"target": goastPkg,
		"detail": "full",
	})

	wantSummary := []string{"extent", "intent", "extent_size", "summary", "types_count"}
	wantFull := []string{"extent", "intent", "extent_size", "summary", "types_count",
		"subconcept_of", "superconcept_of", "incomparable_with"}

	assertFieldSet(t, summary["result"].([]any), wantSummary)
	assertFieldSet(t, full["result"].([]any), wantFull)

	// types_count agrees with the summary string, which is the '+'-joined
	// type list, so the recompute from the intent is not drifting. fb-types
	// and concept-summary derive the struct set independently from the same
	// intent, so this catches drift between them at both detail levels.
	for _, rows := range [][]any{summary["result"].([]any), full["result"].([]any)} {
		for _, r := range rows {
			row := r.(map[string]any)
			n := int(row["types_count"].(float64))
			qt.Assert(t, n, qt.Equals, strings.Count(row["summary"].(string), "+")+1,
				qt.Commentf("types_count vs summary: %v", row))
		}
	}
}

// assertFieldSet fails unless every row has exactly the wanted keys.
func assertFieldSet(t *testing.T, rows []any, want []string) {
	t.Helper()
	qt.Assert(t, len(rows) > 0, qt.IsTrue, qt.Commentf("no rows to check"))
	for _, r := range rows {
		row := r.(map[string]any)
		got := make([]string, 0, len(row))
		for k := range row {
			got = append(got, k)
		}
		slices.Sort(got)
		w := slices.Clone(want)
		slices.Sort(w)
		qt.Assert(t, got, qt.DeepEquals, w, qt.Commentf("row=%v", row))
	}
}

// Ranking: more struct types tied together first, then a larger shared field
// set, then a larger extent. Repeated identical calls agree, and pages
// partition the unpaged result.
func TestFindFalseBoundaries_RankStabilityAndPaging(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: builds an 89-concept lattice over ./goast")
	}
	mc := inProcessClient(t)
	args := map[string]any{"target": goastPkg}

	a := callTool(t, mc, "find_false_boundaries", args)
	b := callTool(t, mc, "find_false_boundaries", args)
	qt.Assert(t, reflect.DeepEqual(a["result"], b["result"]), qt.IsTrue,
		qt.Commentf("non-deterministic order"))

	all := a["result"].([]any)
	qt.Assert(t, len(all) >= 2, qt.IsTrue, qt.Commentf("need >=2 rows to page"))

	// Rank key is non-increasing across the result.
	prev := -1
	for _, r := range all {
		n := int(r.(map[string]any)["types_count"].(float64))
		if prev >= 0 {
			qt.Assert(t, n <= prev, qt.IsTrue,
				qt.Commentf("types_count not descending: %d after %d", n, prev))
		}
		prev = n
	}

	p1 := callTool(t, mc, "find_false_boundaries", map[string]any{
		"target": goastPkg, "limit": float64(1), "offset": float64(0)})
	p2 := callTool(t, mc, "find_false_boundaries", map[string]any{
		"target": goastPkg, "limit": float64(len(all) - 1), "offset": float64(1)})

	qt.Assert(t, p1["provenance"].(map[string]any)["truncated"], qt.Equals, true)
	qt.Assert(t, p2["provenance"].(map[string]any)["truncated"], qt.Equals, false)

	joined := append(slices.Clone(p1["result"].([]any)), p2["result"].([]any)...)
	qt.Assert(t, reflect.DeepEqual(joined, all), qt.IsTrue,
		qt.Commentf("pages do not partition the unpaged result"))
}

// A glob matching nothing is an empty SUCCESS, not an error -- consistent
// with an empty find_duplicates result meaning "nothing cleared the floor".
func TestFindFalseBoundaries_NoMatchIsEmptySuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: builds an 89-concept lattice over ./goast")
	}
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_false_boundaries", map[string]any{
		"target": goastPkg,
		"symbol": "NoSuchFunctionAnywhere",
	})
	envelopeOK(t, env, 2.0)
	qt.Assert(t, env["result"], qt.DeepEquals, []any{})
	prov := env["provenance"].(map[string]any)
	qt.Assert(t, prov["total_count"], qt.Equals, float64(0))
	qt.Assert(t, prov["truncated"], qt.Equals, false)
}

// detail / limit / offset are validated in the Go handler, matching how mode
// already is. A bad value is a tool error naming what is legal.
func TestViewParams_Validation(t *testing.T) {
	mc := inProcessClient(t)
	ctx := context.Background()

	cases := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"find_false_boundaries", map[string]any{"target": phase1Pkg, "detail": "brief"},
			"detail must be summary or full"},
		{"find_false_boundaries", map[string]any{"target": phase1Pkg, "limit": float64(-1)},
			"limit must be a non-negative integer"},
		{"find_false_boundaries", map[string]any{"target": phase1Pkg, "offset": float64(-3)},
			"offset must be a non-negative integer"},
		{"find_false_boundaries", map[string]any{"target": phase1Pkg, "limit": 2.5},
			"limit must be a non-negative integer"},
		{"find_false_boundaries", map[string]any{"target": phase1Pkg, "limit": 1e30},
			"limit must be a non-negative integer"},
	}
	for _, tc := range cases {
		req := mcp.CallToolRequest{}
		req.Params.Name = tc.tool
		req.Params.Arguments = tc.args
		res, err := mc.CallTool(ctx, req)
		qt.Assert(t, err, qt.IsNil)
		qt.Assert(t, res.IsError, qt.IsTrue, qt.Commentf("args=%v", tc.args))
		tcText, ok := mcp.AsTextContent(res.Content[0])
		qt.Assert(t, ok, qt.IsTrue)
		qt.Assert(t, tcText.Text, qt.Equals, tc.want)
	}
}

// The push-down is a real prune, not a post-filter: a scoped call scores
// strictly fewer pairs and returns a subset of the unscoped rows.
//
// Asserted on the pair counts rather than on wall-clock. A timing assertion
// would smuggle machine-dependent nondeterminism into the suite; the pair
// count IS the work, and it is deterministic.
func TestFindDuplicates_ScopeIsAPrune(t *testing.T) {
	mc := inProcessClient(t)

	all := callTool(t, mc, "find_duplicates", map[string]any{"target": dupclusterPkg})
	envelopeOK(t, all, 4.0)
	allProv := all["provenance"].(map[string]any)

	scoped := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg,
		"symbol": "*SumSlice*",
	})
	envelopeOK(t, scoped, 4.0)
	prov := scoped["provenance"].(map[string]any)

	// Fewer pairs reach the scorer, and the count is taken AFTER the prune.
	qt.Assert(t, prov["candidate_pair_count"].(float64) <
		allProv["candidate_pair_count"].(float64), qt.IsTrue,
		qt.Commentf("pair count not pruned: %v vs %v",
			prov["candidate_pair_count"], allProv["candidate_pair_count"]))
	qt.Assert(t, prov["scored_pair_count"].(float64) <
		allProv["scored_pair_count"].(float64), qt.IsTrue)

	// Every scoped row is an unscoped row, and mentions the queried symbol.
	unscoped := map[string]bool{}
	for _, r := range all["result"].([]any) {
		unscoped[pairKey(r)] = true
	}
	rows := scoped["result"].([]any)
	qt.Assert(t, len(rows) > 0, qt.IsTrue, qt.Commentf("scoped call returned nothing"))
	for _, r := range rows {
		qt.Assert(t, unscoped[pairKey(r)], qt.IsTrue,
			qt.Commentf("scoped row absent from unscoped result: %v", r))
		qt.Assert(t, strings.Contains(pairKey(r), "SumSlice"), qt.IsTrue,
			qt.Commentf("row does not mention the queried symbol: %v", r))
	}
	// A pair survives if EITHER function matches: asking about SumSlice
	// surfaces its duplicate, TotalSlice, which does not match the glob.
	qt.Assert(t, findCandidateWithPair(rows, "SumSlice", "TotalSlice"),
		qt.Not(qt.IsNil))
}

// pairKey joins a candidate's function names into a stable identity string.
func pairKey(row any) string {
	funcs, ok := row.(map[string]any)["functions"].([]any)
	if !ok {
		return ""
	}
	names := make([]string, 0, len(funcs))
	for _, f := range funcs {
		n, _ := f.(map[string]any)["name"].(string)
		names = append(names, n)
	}
	return strings.Join(names, "|")
}

// scope narrows by file, against the absolute position the tool carries.
func TestFindDuplicates_ScopeByFile(t *testing.T) {
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg,
		"scope":  "clones.go",
	})
	envelopeOK(t, env, 4.0)
	rows := env["result"].([]any)
	qt.Assert(t, findCandidateWithPair(rows, "SumSlice", "TotalSlice"), qt.Not(qt.IsNil),
		qt.Commentf("scope=clones.go did not surface the clone pair: %v", rows))
	qt.Assert(t, env["provenance"].(map[string]any)["scope"], qt.Equals, "clones.go")
}

// The single largest win in the design: the pair-count guard reads the count
// AFTER the prune, so a narrow query against a target that would otherwise be
// refused is answered instead. The guard becomes a wall the caller can walk
// around by being specific.
func TestFindDuplicates_ScopeWalksAroundTheGuard(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: loads and SSA-builds the whole tree")
	}
	mc := inProcessClient(t)

	// Unscoped, this target is refused (36474 pairs, over the 25000 budget).
	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Name = "find_duplicates"
	req.Params.Arguments = map[string]any{"target": wholeTreePkg}
	res, err := mc.CallTool(ctx, req)
	qt.Assert(t, err, qt.IsNil)
	qt.Assert(t, res.IsError, qt.IsTrue, qt.Commentf("target no longer exceeds the budget"))

	// Scoped to one file, the same target is answered.
	env := callTool(t, mc, "find_duplicates", map[string]any{
		"target": wholeTreePkg,
		"scope":  "goastcfg/prim_cfg.go",
	})
	envelopeOK(t, env, 4.0)
	prov := env["provenance"].(map[string]any)
	qt.Assert(t, prov["candidate_pair_count"].(float64) <= 25000, qt.IsTrue,
		qt.Commentf("pruned pair count still over budget: %v", prov["candidate_pair_count"]))
	qt.Assert(t, prov["scope"], qt.Equals, "goastcfg/prim_cfg.go")
}

// summary drops the measures sub-object (whose similarity merely repeats the
// top-level score); full is today's shape. An opt-in verdict survives both:
// a caller who asked for it asked for it.
func TestFindDuplicates_DetailFieldSets(t *testing.T) {
	mc := inProcessClient(t)

	summary := callTool(t, mc, "find_duplicates", map[string]any{"target": dupclusterPkg})
	full := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg, "detail": "full"})
	withVerdict := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg, "verdict": true})

	assertFieldSet(t, summary["result"].([]any),
		[]string{"functions", "score", "equiv_tier"})
	assertFieldSet(t, full["result"].([]any),
		[]string{"functions", "score", "equiv_tier", "measures"})
	assertFieldSet(t, withVerdict["result"].([]any),
		[]string{"functions", "score", "equiv_tier", "verdict"})
}

// Ranking: score descending, proven before structural on ties, deterministic
// on repeat, and pages partition the unpaged result.
func TestFindDuplicates_RankStabilityAndPaging(t *testing.T) {
	mc := inProcessClient(t)
	args := map[string]any{"target": dupclusterPkg}
	a := callTool(t, mc, "find_duplicates", args)
	b := callTool(t, mc, "find_duplicates", args)
	qt.Assert(t, reflect.DeepEqual(a["result"], b["result"]), qt.IsTrue,
		qt.Commentf("non-deterministic order"))

	all := a["result"].([]any)
	qt.Assert(t, len(all) >= 2, qt.IsTrue, qt.Commentf("need >=2 rows to page"))

	prev := 2.0
	for _, r := range all {
		s := r.(map[string]any)["score"].(float64)
		qt.Assert(t, s <= prev, qt.IsTrue,
			qt.Commentf("score not descending: %v after %v", s, prev))
		prev = s
	}

	p1 := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg, "limit": float64(1), "offset": float64(0)})
	p2 := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg, "limit": float64(len(all) - 1), "offset": float64(1)})

	qt.Assert(t, p1["provenance"].(map[string]any)["truncated"], qt.Equals, true)
	qt.Assert(t, p1["provenance"].(map[string]any)["total_count"].(float64),
		qt.Equals, float64(len(all)))
	qt.Assert(t, p2["provenance"].(map[string]any)["truncated"], qt.Equals, false)

	joined := append(slices.Clone(p1["result"].([]any)), p2["result"].([]any)...)
	qt.Assert(t, reflect.DeepEqual(joined, all), qt.IsTrue,
		qt.Commentf("pages do not partition the unpaged result"))
}

// A glob matching nothing is an empty success, not an error.
func TestFindDuplicates_NoMatchIsEmptySuccess(t *testing.T) {
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg,
		"symbol": "NoSuchFunctionAnywhere",
	})
	envelopeOK(t, env, 4.0)
	qt.Assert(t, env["result"], qt.DeepEquals, []any{})
	prov := env["provenance"].(map[string]any)
	qt.Assert(t, prov["total_count"], qt.Equals, float64(0))
	qt.Assert(t, prov["candidate_pair_count"], qt.Equals, float64(0))
	qt.Assert(t, prov["truncated"], qt.Equals, false)
}

// Same-named functions in different packages are distinct rows: `symbol'
// matches the short name both share, and `pkg' tells them apart.
func TestFindDuplicates_SameNameAcrossPackages(t *testing.T) {
	mc := inProcessClient(t)
	root := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcross"
	env := callTool(t, mc, "find_duplicates", map[string]any{
		// The import-path wildcard skips testdata; the relative pattern does not.
		"target":    "../../examples/goast-query/testdata/dupcross/...",
		"symbol":    "Parse",
		"threshold": 0.0,
	})
	envelopeOK(t, env, 4.0)
	// One row must pair Parse with Parse. Rows pairing Parse with an Encode
	// would cover both packages without the cross-package pair existing.
	var pkgs map[string]bool
	for _, row := range env["result"].([]any) {
		funcs := row.(map[string]any)["functions"].([]any)
		a, b := funcs[0].(map[string]any), funcs[1].(map[string]any)
		if a["name"] == "Parse" && b["name"] == "Parse" {
			pkgs = map[string]bool{a["pkg"].(string): true, b["pkg"].(string): true}
		}
	}
	qt.Assert(t, pkgs, qt.DeepEquals, map[string]bool{root + "/a": true, root + "/b": true})
}
