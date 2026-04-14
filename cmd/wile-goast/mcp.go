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
	"io/fs"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/aalpar/wile"
	"github.com/aalpar/wile/werr"
)

// BuildVersion is set by -ldflags at build time.
var BuildVersion string

// BuildSHA is set by -ldflags at build time.
var BuildSHA string

var errMCPServer = werr.NewStaticError("mcp server error")

type mcpServer struct {
	engine     *wile.Engine
	engineOnce sync.Once
	engineErr  error
}

func doMCP(ctx context.Context) error {
	ms := &mcpServer{}

	v := BuildVersion
	if v == "" {
		v = "dev"
	}

	s := server.NewMCPServer(
		"wile-goast",
		v,
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(true),
		server.WithInstructions(
			"# The wile-goast MCP server\n\n"+
				"Go static analysis via Scheme. Use this server for structural queries about Go code "+
				"that go beyond what grep or file reading can answer reliably.\n\n"+
				"## When to use\n\n"+
				"- Finding similar or duplicate functions → AST diff via `(wile goast)`\n"+
				"- Checking consistency patterns (every Lock has Unlock, etc.) → belief DSL via `(wile goast belief)`\n"+
				"- Understanding call relationships → call graph queries via `(wile goast callgraph)`\n"+
				"- Analyzing control flow (dominance, reachability, paths) → CFG via `(wile goast cfg)`\n"+
				"- Running lint/analysis passes → `(wile goast lint)`\n"+
				"- Examining SSA form and data flow → `(wile goast ssa)`\n\n"+
				"The `eval` tool accepts Scheme expressions with these libraries pre-loaded. "+
				"Parse Go packages with `go-parse-file` or `go-typecheck-package`, then query the result.\n\n"+
				"## When NOT to use\n\n"+
				"- Scheme runtime behavior, primitive signatures, library docs → use wile instead\n"+
				"- Go symbol navigation, references, diagnostics, renaming → use gopls instead\n"+
				"- Reading a single short function → direct file reading is faster\n\n"+
				"## Prompts\n\n"+
				"Four prompts are available:\n"+
				"- `goast-analyze` — selects the right analysis layer for a structural question\n"+
				"- `goast-beliefs` — defines and runs consistency checks via the belief DSL\n"+
				"- `goast-refactor` — finds unification candidates and verifies refactoring correctness\n"+
				"- `goast-scheme-ref` — Wile Scheme reference: available/missing primitives, idioms, exports, gotchas. **Load before writing non-trivial Scheme.**\n",
		),
	)

	s.AddTool(
		mcp.NewTool("eval",
			mcp.WithDescription("Evaluate a Scheme expression with Go static analysis primitives loaded. "+
				"Available libraries: (wile goast), (wile goast ssa), (wile goast cfg), "+
				"(wile goast callgraph), (wile goast lint), (wile goast belief), (wile goast utils)."),
			mcp.WithString("code",
				mcp.Required(),
				mcp.Description("Scheme expression to evaluate"),
			),
		),
		ms.handleEval,
	)

	err := ms.registerPrompts(s)
	if err != nil {
		return werr.WrapForeignErrorf(errMCPServer, "registering prompts: %s", err)
	}

	return server.ServeStdio(s)
}

func (ms *mcpServer) getEngine(ctx context.Context) (*wile.Engine, error) {
	ms.engineOnce.Do(func() {
		ms.engine, ms.engineErr = buildEngineOrError(ctx)
	})
	return ms.engine, ms.engineErr
}

func (ms *mcpServer) handleEval(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	code := req.GetString("code", "")
	if code == "" {
		return mcp.NewToolResultError("code parameter is required"), nil
	}

	engine, err := ms.getEngine(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("engine init failed: %v", err)), nil
	}

	val, err := engine.EvalMultiple(ctx, code)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if val == nil || val.IsVoid() {
		return mcp.NewToolResultText(""), nil
	}
	return mcp.NewToolResultText(val.SchemeString()), nil
}

func (ms *mcpServer) registerPrompts(s *server.MCPServer) error {
	type promptDef struct {
		name        string
		description string
		file        string
		args        []mcp.PromptOption
	}

	prompts := []promptDef{
		{
			name:        "goast-analyze",
			description: "Analyze Go code structure — selects the right analysis layer and composes Scheme queries",
			file:        "prompts/goast-analyze.md",
			args: []mcp.PromptOption{
				mcp.WithArgument("question",
					mcp.RequiredArgument(),
					mcp.ArgumentDescription("The analysis question (e.g., 'what calls FuncName', 'find functions similar to X')"),
				),
				mcp.WithArgument("package",
					mcp.ArgumentDescription("Go package pattern to analyze (e.g., './...', 'my/package')"),
				),
			},
		},
		{
			name:        "goast-beliefs",
			description: "Define and run consistency beliefs against Go packages using the belief DSL",
			file:        "prompts/goast-beliefs.md",
			args: []mcp.PromptOption{
				mcp.WithArgument("action",
					mcp.ArgumentDescription("'run' to run existing beliefs, 'define' to create new ones, or describe the pattern to check"),
				),
				mcp.WithArgument("package",
					mcp.ArgumentDescription("Go package pattern to analyze (e.g., './...', 'my/package')"),
				),
			},
		},
		{
			name:        "goast-refactor",
			description: "Find unification candidates and verify refactoring correctness via structural analysis",
			file:        "prompts/goast-refactor.md",
			args: []mcp.PromptOption{
				mcp.WithArgument("goal",
					mcp.RequiredArgument(),
					mcp.ArgumentDescription("The refactoring goal (e.g., 'find duplicates in package X', 'unify functions A and B')"),
				),
				mcp.WithArgument("package",
					mcp.ArgumentDescription("Go package pattern to analyze (e.g., './...', 'my/package')"),
				),
			},
		},
		{
			name:        "goast-scheme-ref",
			description: "Wile Scheme reference — available primitives, missing builtins, idioms, library exports, and gotchas. Load before writing non-trivial Scheme.",
			file:        "prompts/goast-scheme-ref.md",
		},
		{
			name:        "goast-split",
			description: "Analyze package cohesion and recommend splits via IDF-weighted Formal Concept Analysis",
			file:        "prompts/goast-split.md",
			args: []mcp.PromptOption{
				mcp.WithArgument("package",
					mcp.RequiredArgument(),
					mcp.ArgumentDescription("Go package pattern to analyze (e.g., 'my/package', './internal/...')"),
				),
				mcp.WithArgument("goal",
					mcp.ArgumentDescription("Motivation for the split (e.g., 'reduce coupling', 'package too large')"),
				),
			},
		},
	}

	for _, p := range prompts {
		content, err := fs.ReadFile(embeddedPrompts, p.file)
		if err != nil {
			return werr.WrapForeignErrorf(errMCPServer, "reading %s: %s", p.file, err)
		}
		text := string(content)
		promptOpts := append([]mcp.PromptOption{mcp.WithPromptDescription(p.description)}, p.args...)

		s.AddPrompt(
			mcp.NewPrompt(p.name, promptOpts...),
			ms.makePromptHandler(text),
		)
	}
	return nil
}

func (ms *mcpServer) makePromptHandler(template string) server.PromptHandlerFunc {
	return func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		text := template
		for k, v := range req.Params.Arguments {
			text = strings.ReplaceAll(text, "{{"+k+"}}", v)
		}
		for _, placeholder := range []string{"{{question}}", "{{package}}", "{{action}}", "{{goal}}"} {
			text = strings.ReplaceAll(text, placeholder, "(not specified)")
		}
		return &mcp.GetPromptResult{
			Messages: []mcp.PromptMessage{
				mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text)),
			},
		}, nil
	}
}
