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
