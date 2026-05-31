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
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerPhase1Tools registers the Phase 1 pipeline tools on s. Called
// from newServer (the shared stdio+HTTP construction site), so both
// transports expose the same tools.
func (ms *mcpServer) registerPhase1Tools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("check_beliefs",
			mcp.WithDescription("Run committed beliefs against a Go package. "+
				"Returns an adherence/deviation report per belief. Use when you "+
				"have a .scm file or directory of belief files and want a "+
				"structural consistency report."),
			mcp.WithString("target", mcp.Required(),
				mcp.Description("Go package pattern (e.g., 'my/pkg/...')")),
			mcp.WithString("beliefs_path", mcp.Required(),
				mcp.Description("Path to a .scm file or directory of .scm files")),
		),
		ms.handleCheckBeliefs,
	)
	// Tasks 3-6 register the other four tools here.
}

// invokePipeline evaluates a pipeline call on the session's engine,
// marshals the resulting Wile value to JSON-compatible Go via
// marshalToJSON, and returns a tool result with both text (JSON string)
// and structured content populated. Tool-level errors become
// mcp.NewToolResultError.
//
// Mirrors handleEval's per-session-engine access (mcp.go): resolve the
// entry by session key, hold evalMu across EvalMultiple so concurrent
// pipelined requests on one client don't race the single engine.
func (ms *mcpServer) invokePipeline(ctx context.Context, code string) (*mcp.CallToolResult, error) {
	entry, err := ms.builtEntry(ctx, sessionKey(ctx))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("engine init failed: %v", err)), nil
	}

	entry.evalMu.Lock()
	defer entry.evalMu.Unlock()

	val, err := entry.engine.EvalMultiple(ctx, code)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := marshalToJSON(val)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal: %v", err)), nil
	}
	res, err := mcp.NewToolResultJSON(data)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("encode: %v", err)), nil
	}
	return res, nil
}

// schemeStringLiteral quotes s as a Scheme string literal, escaping
// backslashes and double quotes. Used to pass user-supplied paths and
// patterns safely into pipeline call source.
func schemeStringLiteral(s string) string {
	r := strings.ReplaceAll(s, `\`, `\\`)
	r = strings.ReplaceAll(r, `"`, `\"`)
	return `"` + r + `"`
}

func (ms *mcpServer) handleCheckBeliefs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target := req.GetString("target", "")
	beliefsPath := req.GetString("beliefs_path", "")
	if target == "" {
		return mcp.NewToolResultError("target parameter is required"), nil
	}
	if beliefsPath == "" {
		return mcp.NewToolResultError("beliefs_path parameter is required"), nil
	}
	code := `(import (wile goast pipelines))
(pipeline-check-beliefs ` + schemeStringLiteral(target) + ` ` + schemeStringLiteral(beliefsPath) + `)`
	return ms.invokePipeline(ctx, code)
}
