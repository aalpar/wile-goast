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
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/aalpar/wile/pkg/wile"
	"github.com/aalpar/wile/werr"
)

// BuildVersion is set by -ldflags at build time.
var BuildVersion string

// BuildSHA is set by -ldflags at build time.
var BuildSHA string

var errMCPServer = werr.NewStaticError("mcp server error")

// engineEntry holds one session's Wile engine. The engine is built lazily on
// first use so a session that only fetches prompts never pays the KitchenSink
// build cost. evalMu serializes EvalMultiple calls within a session, since a
// single client may pipeline concurrent requests onto its one engine.
type engineEntry struct {
	once   sync.Once
	engine *wile.Engine
	err    error
	evalMu sync.Mutex
}

// closeEngine settles any in-flight build (or precludes a future one) via the
// idempotent once, then closes the engine if it was built. Safe to call
// concurrently with a build: sync.Once serializes the two.
func (e *engineEntry) closeEngine() {
	e.once.Do(func() {})
	if e.engine != nil {
		_ = e.engine.Close()
	}
}

// mcpServer owns one engine per MCP session, keyed by ClientSession.SessionID().
// This gives cross-client isolation (one client's go-load sessions and defined
// beliefs stay invisible to others) while preserving per-client state across
// calls. stdio is the degenerate case: a single session keyed "stdio".
type mcpServer struct {
	mu      sync.Mutex
	engines map[string]*engineEntry

	// idleTTL bounds how long an idle Streamable HTTP session is kept before the
	// library sweeper reaps it, firing OnUnregisterSession to free its engine.
	// Passed straight to WithSessionIdleTTL: zero or negative disables the
	// sweeper. The CLI default lives on the --http-idle-ttl flag; a zero-value
	// mcpServer (stdio, tests) leaves the sweeper off, which is harmless there.
	idleTTL time.Duration
}

// entryForKey returns the engineEntry for a session key, creating an empty one
// if absent. The manager lock is held only for the map operation, never during
// the (expensive) engine build.
func (ms *mcpServer) entryForKey(key string) *engineEntry {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if ms.engines == nil {
		ms.engines = make(map[string]*engineEntry)
	}
	e := ms.engines[key]
	if e == nil {
		e = &engineEntry{}
		ms.engines[key] = e
	}
	return e
}

// builtEntry returns the entry for a key with its engine built (once).
func (ms *mcpServer) builtEntry(ctx context.Context, key string) (*engineEntry, error) {
	e := ms.entryForKey(key)
	e.once.Do(func() {
		e.engine, e.err = buildEngineOrError(ctx)
	})
	return e, e.err
}

// engineForKey returns the built engine for a session key.
func (ms *mcpServer) engineForKey(ctx context.Context, key string) (*wile.Engine, error) {
	e, err := ms.builtEntry(ctx, key)
	return e.engine, err
}

// evict removes a session's engine and closes it (the OnUnregisterSession path).
func (ms *mcpServer) evict(key string) {
	ms.mu.Lock()
	e := ms.engines[key]
	delete(ms.engines, key)
	ms.mu.Unlock()
	if e != nil {
		e.closeEngine()
	}
}

// closeAll closes every managed engine. Used at HTTP server shutdown and in tests.
func (ms *mcpServer) closeAll() {
	ms.mu.Lock()
	entries := ms.engines
	ms.engines = nil
	ms.mu.Unlock()
	for _, e := range entries {
		e.closeEngine()
	}
}

// onUnregister is the OnUnregisterSession hook: free the engine for a session
// that has terminated (clean DELETE, or reaped by the idle sweeper).
func (ms *mcpServer) onUnregister(_ context.Context, s server.ClientSession) {
	if s != nil {
		ms.evict(s.SessionID())
	}
}

// sessionKey resolves the MCP session id from context. The fallback key is
// defensive — both stdio and stateful HTTP supply a session.
func sessionKey(ctx context.Context) string {
	s := server.ClientSessionFromContext(ctx)
	if s != nil {
		return s.SessionID()
	}
	return ""
}

// newServer builds the transport-agnostic protocol server: instructions, the
// eval tool, prompts, and the session-cleanup hook. Shared by stdio and HTTP.
func (ms *mcpServer) newServer() (*server.MCPServer, error) {
	v := BuildVersion
	if v == "" {
		v = "dev"
	}

	hooks := &server.Hooks{}
	hooks.AddOnUnregisterSession(ms.onUnregister)

	s := server.NewMCPServer(
		"wile-goast",
		v,
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(true),
		server.WithHooks(hooks),
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
				"## Pipeline tools\n\n"+
				"Five pipeline tools return structured JSON reports without LLM orchestration. "+
				"Each takes a `target` Go package pattern and returns a `{version, provenance, result}` envelope:\n"+
				"- `check_beliefs` — run a directory of .scm beliefs against a package\n"+
				"- `discover_beliefs` — run discovery beliefs, suppress committed ones, emit survivors as source\n"+
				"- `recommend_split` — IDF-weighted FCA + min-cut package split recommendation\n"+
				"- `recommend_boundaries` — function-level split/merge/extract Pareto frontiers\n"+
				"- `find_false_boundaries` — cross-struct FCA concepts with lattice annotations\n\n"+
				"Prefer a pipeline tool for its known structural query; use `eval` for open-ended exploration.\n\n"+
				"## When NOT to use\n\n"+
				"- Scheme runtime behavior, primitive signatures, library docs → use wile instead\n"+
				"- Go symbol navigation, references, diagnostics, renaming → use gopls instead\n"+
				"- Reading a single short function → direct file reading is faster\n\n"+
				"## Prompts\n\n"+
				"Five prompts are available:\n"+
				"- `goast-analyze` — selects the right analysis layer for a structural question\n"+
				"- `goast-beliefs` — defines and runs consistency checks via the belief DSL\n"+
				"- `goast-refactor` — finds unification candidates and verifies refactoring correctness\n"+
				"- `goast-scheme-ref` — Wile Scheme reference: available/missing primitives, idioms, exports, gotchas. **Load before writing non-trivial Scheme.**\n"+
				"- `goast-split` — analyze package cohesion and recommend splits via IDF-weighted FCA\n",
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

	ms.registerPhase1Tools(s)

	err := ms.registerPrompts(s)
	if err != nil {
		return nil, werr.WrapForeignErrorf(errMCPServer, "registering prompts: %s", err)
	}

	return s, nil
}

// doMCP runs the MCP server over stdio.
func doMCP(ctx context.Context) error {
	ms := &mcpServer{}
	s, err := ms.newServer()
	if err != nil {
		return err
	}
	return server.ServeStdio(s)
}

// newStreamableHTTPServer builds the Streamable HTTP transport over the shared
// protocol server. Stateful so each client's Mcp-Session-Id maps to the same
// engine across requests; idle sessions are reaped (freeing engines) by the
// library sweeper. The returned server is an http.Handler, so tests can mount
// it on httptest.NewServer directly.
func (ms *mcpServer) newStreamableHTTPServer() (*server.StreamableHTTPServer, error) {
	s, err := ms.newServer()
	if err != nil {
		return nil, err
	}
	return server.NewStreamableHTTPServer(s,
		server.WithStateful(true),
		server.WithSessionIdleTTL(ms.idleTTL),
	), nil
}

// doHTTP runs the MCP server over Streamable HTTP at addr, shutting down
// gracefully on SIGINT/SIGTERM or context cancellation. idleTTL bounds how long
// abandoned sessions are retained before their engines are freed; zero or
// negative disables reaping.
func doHTTP(ctx context.Context, addr string, idleTTL time.Duration) error {
	ms := &mcpServer{idleTTL: idleTTL}
	defer ms.closeAll()

	httpSrv, err := ms.newStreamableHTTPServer()
	if err != nil {
		return err
	}

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "wile-goast MCP server listening on http://%s/mcp\n", addr)
		err := httpSrv.Start(addr)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- werr.WrapForeignErrorf(errMCPServer, "http serve: %s", err)
			return
		}
		serveErr <- nil
	}()

	select {
	case <-sigCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	case err := <-serveErr:
		return err
	}
}

func (ms *mcpServer) handleEval(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	code := req.GetString("code", "")
	if code == "" {
		return mcp.NewToolResultError("code parameter is required"), nil
	}

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
