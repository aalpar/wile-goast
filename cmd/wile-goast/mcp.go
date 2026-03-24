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
)

// BuildVersion is set by -ldflags at build time.
var BuildVersion string

// BuildSHA is set by -ldflags at build time.
var BuildSHA string

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

	if err := ms.registerPrompts(s); err != nil {
		return fmt.Errorf("registering prompts: %w", err)
	}

	return server.ServeStdio(s)
}

func (ms *mcpServer) getEngine(ctx context.Context) (*wile.Engine, error) {
	ms.engineOnce.Do(func() {
		ms.engine = buildEngine(ctx)
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
	}

	for _, p := range prompts {
		content, err := fs.ReadFile(embeddedPrompts, p.file)
		if err != nil {
			return fmt.Errorf("reading %s: %w", p.file, err)
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
