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
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	qt "github.com/frankban/quicktest"
)

// evalReq builds an eval CallToolRequest for the given Scheme code.
func evalReq(code string) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Name = "eval"
	req.Params.Arguments = map[string]any{"code": code}
	return req
}

// resultText extracts the first text content from a tool result.
func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("result content is not text: %T", res.Content[0])
	}
	return tc.Text
}

// A large eval result must be truncated to at most maxEvalResultBytes + the
// appended hint, and must carry the projection hint that teaches the caller to
// return less or use a pipeline tool. Parsing this source file's own AST
// reliably exceeds 16 KB.
func TestHandleEval_TruncatesOversizeResult(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ms := &mcpServer{}
	defer ms.closeAll()

	res, err := ms.handleEval(ctx, evalReq(`(go-parse-file "mcp.go")`))
	c.Assert(err, qt.IsNil)
	c.Assert(res.IsError, qt.IsFalse)

	text := resultText(t, res)
	c.Assert(strings.Contains(text, "truncated"), qt.IsTrue)
	c.Assert(strings.Contains(text, "reference"), qt.IsTrue)
	// Body is capped; total = maxEvalResultBytes body + a short hint.
	c.Assert(len(text) < maxEvalResultBytes+512, qt.IsTrue)
}

// A small eval result must pass through unchanged (no hint, no truncation).
func TestHandleEval_SmallResultUnchanged(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ms := &mcpServer{}
	defer ms.closeAll()

	res, err := ms.handleEval(ctx, evalReq(`(+ 1 2)`))
	c.Assert(err, qt.IsNil)
	c.Assert(res.IsError, qt.IsFalse)
	c.Assert(resultText(t, res), qt.Equals, "3")
}

// The reference tool must be advertised so Claude can pull the cheatsheet
// before its first eval.
func TestReferenceTool_Registered(t *testing.T) {
	mc := inProcessClient(t)
	res, err := mc.ListTools(context.Background(), mcp.ListToolsRequest{})
	qt.Assert(t, err, qt.IsNil)

	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	qt.Assert(t, names["reference"], qt.IsTrue)
}

// The reference tool must return the distilled cheatsheet content.
func TestReferenceTool_ReturnsCheatsheet(t *testing.T) {
	mc := inProcessClient(t)
	req := mcp.CallToolRequest{}
	req.Params.Name = "reference"
	res, err := mc.CallTool(context.Background(), req)
	qt.Assert(t, err, qt.IsNil)
	qt.Assert(t, res.IsError, qt.IsFalse)
	qt.Assert(t, len(res.Content) > 0, qt.IsTrue)

	tc, ok := mcp.AsTextContent(res.Content[0])
	qt.Assert(t, ok, qt.IsTrue)
	qt.Assert(t, strings.Contains(tc.Text, "go-parse-file"), qt.IsTrue)
	qt.Assert(t, strings.Contains(tc.Text, "go-callgraph"), qt.IsTrue)
}

// An eval that errors on an undefined symbol must return an error result that
// carries the cheatsheet, so the caller can self-correct without a human
// invoking a prompt.
func TestHandleEval_ErrorAppendsCheatsheet(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ms := &mcpServer{}
	defer ms.closeAll()

	res, err := ms.handleEval(ctx, evalReq(`(this-symbol-does-not-exist 1 2)`))
	c.Assert(err, qt.IsNil)
	c.Assert(res.IsError, qt.IsTrue)

	text := resultText(t, res)
	// The original error is preserved AND the cheatsheet is appended.
	c.Assert(strings.Contains(text, "go-callgraph"), qt.IsTrue)
	c.Assert(strings.Contains(text, "parse → query → project"), qt.IsTrue)
}

// The server instructions must lead with differentiation from the tools Claude
// already reaches for. These keywords are the contract; a future wording edit
// that drops them should fail here.
func TestInstructions_ContainDifferentiation(t *testing.T) {
	c := qt.New(t)
	ms := &mcpServer{}
	t.Cleanup(ms.closeAll)

	s, err := ms.newServer()
	c.Assert(err, qt.IsNil)
	cl, err := client.NewInProcessClient(s)
	c.Assert(err, qt.IsNil)
	t.Cleanup(func() { _ = cl.Close() })

	ctx := context.Background()
	c.Assert(cl.Start(ctx), qt.IsNil)
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "1.0.0"}
	initRes, err := cl.Initialize(ctx, initReq)
	c.Assert(err, qt.IsNil)

	for _, kw := range []string{"grep", "gopls", "can't"} {
		c.Assert(strings.Contains(initRes.Instructions, kw), qt.IsTrue,
			qt.Commentf("instructions missing differentiation keyword %q", kw))
	}
}

// capEvalResult must never split a multibyte rune: it backs up to a rune
// boundary before truncating. "中" is 3 bytes, and 16384 is not a multiple of
// 3, so the cut lands mid-rune and the back-up loop must engage. The truncated
// body must remain valid UTF-8.
func TestCapEvalResult_TruncatesOnRuneBoundary(t *testing.T) {
	c := qt.New(t)
	// 6000 * 3 = 18000 bytes, over the 16384 cap.
	s := strings.Repeat("中", 6000)
	out := capEvalResult(s)
	c.Assert(len(out) < len(s), qt.IsTrue)
	c.Assert(strings.Contains(out, "truncated"), qt.IsTrue)
	// The body before the appended hint must be valid UTF-8 (no split rune).
	body := out
	prefix, _, found := strings.Cut(out, "\n\n;;")
	if found {
		body = prefix
	}
	c.Assert(utf8.ValidString(body), qt.IsTrue)
}

// The eval description must be task-indexed — leading with the concrete
// questions it answers, not "evaluate a Scheme expression".
func TestEvalDescription_TaskIndexed(t *testing.T) {
	c := qt.New(t)
	mc := inProcessClient(t)
	res, err := mc.ListTools(context.Background(), mcp.ListToolsRequest{})
	c.Assert(err, qt.IsNil)

	var desc string
	for _, tool := range res.Tools {
		if tool.Name == "eval" {
			desc = tool.Description
		}
	}
	for _, kw := range []string{"duplicate", "call path", "checked"} {
		c.Assert(strings.Contains(desc, kw), qt.IsTrue,
			qt.Commentf("eval description missing task phrase %q", kw))
	}
}
