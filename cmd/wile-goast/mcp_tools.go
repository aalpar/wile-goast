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
	s.AddTool(
		mcp.NewTool("discover_beliefs",
			mcp.WithDescription("Run a directory of discovery beliefs against a Go "+
				"package, suppress any that match a committed belief, and return the "+
				"survivors as Scheme source ready to commit. Use to mine a codebase for "+
				"new consistency patterns without re-surfacing already-committed ones."),
			mcp.WithString("target", mcp.Required(),
				mcp.Description("Go package pattern (e.g., 'my/pkg/...')")),
			mcp.WithString("discovery_path", mcp.Required(),
				mcp.Description("Path to a discovery .scm file or directory of them")),
			mcp.WithString("committed_path",
				mcp.Description("Path to committed beliefs (optional). Omit or pass "+
					"\"\" to disable suppression and return raw discovery output.")),
		),
		ms.handleDiscoverBeliefs,
	)
	s.AddTool(
		mcp.NewTool("recommend_split",
			mcp.WithDescription("Analyze a Go package's cohesion and recommend a "+
				"two-way split via IDF-weighted FCA + min-cut. Returns the split "+
				"proposal with a confidence verdict (HIGH/MEDIUM/LOW/NONE)."),
			mcp.WithString("target", mcp.Required(),
				mcp.Description("Go package pattern (e.g., 'my/pkg/...')")),
			mcp.WithNumber("idf_threshold",
				mcp.Description("Minimum IDF to keep a package as a signature attribute (default 0.36)")),
			mcp.WithBoolean("refine",
				mcp.Description("Refine the context by (package, object) granularity")),
			mcp.WithNumber("max_attributes",
				mcp.Description("Fail fast if attribute count exceeds this (default 30; lattice is 2^N)")),
		),
		ms.handleRecommendSplit,
	)
	// Tasks 5-6 register the other two tools here.
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

func (ms *mcpServer) handleDiscoverBeliefs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target := req.GetString("target", "")
	discoveryPath := req.GetString("discovery_path", "")
	committedPath := req.GetString("committed_path", "")
	if target == "" {
		return mcp.NewToolResultError("target parameter is required"), nil
	}
	if discoveryPath == "" {
		return mcp.NewToolResultError("discovery_path parameter is required"), nil
	}
	code := `(import (wile goast pipelines))
(pipeline-discover-beliefs ` +
		schemeStringLiteral(target) + ` ` +
		schemeStringLiteral(discoveryPath) + ` ` +
		schemeStringLiteral(committedPath) + `)`
	return ms.invokePipeline(ctx, code)
}

func (ms *mcpServer) handleRecommendSplit(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target := req.GetString("target", "")
	if target == "" {
		return mcp.NewToolResultError("target parameter is required"), nil
	}
	// Build an alist of option overrides from the supplied params; the
	// Scheme pipeline flattens it to recommend-split's plist form.
	args := req.GetArguments()
	var optsParts []string
	if v, ok := args["idf_threshold"]; ok {
		optsParts = append(optsParts, fmt.Sprintf("(idf-threshold . %v)", v))
	}
	if r, ok := args["refine"]; ok {
		if b, _ := r.(bool); b {
			optsParts = append(optsParts, "(refine . #t)")
		}
	}
	if m, ok := args["max_attributes"]; ok {
		optsParts = append(optsParts, fmt.Sprintf("(max-attributes . %v)", m))
	}
	code := `(import (wile goast pipelines))
(pipeline-recommend-split ` + schemeStringLiteral(target) +
		` (list ` + strings.Join(optsParts, " ") + `))`
	return ms.invokePipeline(ctx, code)
}
