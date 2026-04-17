# MCP Server Design

**Status:** COMPLETED. `--mcp` flag, eval tool, 3 prompts all shipped.

## Summary

Add `--mcp` flag to `wile-goast` that starts a stdio JSON-RPC server using
`mark3labs/mcp-go`. Exposes the Scheme evaluator as a single MCP tool and
replaces the current Claude Code skill files with MCP prompts.

## Motivation

1. **Discoverability** — MCP servers self-describe their capabilities. Any
   MCP-compatible client discovers available tools and prompts at startup
   without manual skill/agent installation.
2. **Broader reach** — Cursor, Windsurf, Continue, Zed, and other MCP clients
   gain access. Currently locked to Claude Code's skill system.
3. **Richer interaction model** — MCP prompts replace external skill files,
   keeping analysis guidance co-located with the tool that executes it.

## Architecture

```
wile-goast --mcp
  ┌──────────┐    ┌────────────────────────┐
  │ mcp-go   │───→│ Wile Engine            │
  │ stdio    │    │  + goast extensions    │
  │ server   │←───│  + embedded libs       │
  └──────────┘    └────────────────────────┘
       ↕
   stdin/stdout (JSON-RPC)
```

One persistent Wile engine serves all tool calls within the session.

## Tool

Single tool: **`eval`**

| Parameter | Type   | Required | Description                    |
|-----------|--------|----------|--------------------------------|
| `code`    | string | yes      | Scheme expression to evaluate  |

Returns the evaluation result as text. Scheme errors (parse failures, runtime
exceptions) surface as MCP tool errors (`IsError: true`), not transport errors.

## Prompts

Three prompts replace the current Claude Code skill files:

| Prompt           | Arguments                                          | Replaces             |
|------------------|----------------------------------------------------|----------------------|
| `goast-analyze`  | `question` (required), `package` (optional)        | `/goast-analyze`     |
| `goast-beliefs`  | `action` (optional), `package` (optional)           | `/goast-beliefs`     |
| `goast-refactor` | `goal` (required), `package` (optional)             | `/goast-refactor`    |

Each prompt returns instructional content as prompt messages: layer selection
guidance, DSL reference, interpretation rules. The client's LLM receives these
instructions and uses the `eval` tool to execute analysis.

### Prompt content source

The prompt message text is the same content currently in the skill `.md` files,
adapted for MCP prompt format. The `goast-analyzer` agent's primitive reference
and AST conventions fold into the `goast-analyze` prompt.

## Engine Lifecycle

- Engine boots on first `eval` tool call (lazy init), not on MCP `initialize`
- Engine persists for the session lifetime
- No explicit shutdown — engine closes when the process exits (stdio EOF)

## Flag Behavior

`--mcp` is mutually exclusive with `-e`, `-f`, `--run`, `--list-scripts`.
If `--mcp` is set alongside any of these, exit with a usage error.

## File Changes

### New files

- `cmd/wile-goast/mcp.go` — MCP server setup: tool handler, prompt handlers,
  `doMCP(ctx)` entry point

### Modified files

- `cmd/wile-goast/main.go` — Add `--mcp` flag to `Options`, dispatch to
  `doMCP` before other modes
- `go.mod` — Add `github.com/mark3labs/mcp-go` dependency

## Deletions (post-validation)

After the MCP server is working and tested with a client:

- `~/.claude/commands/goast-analyze.md`
- `~/.claude/commands/goast-beliefs.md`
- `~/.claude/commands/goast-refactor.md`
- `~/.claude/agents/goast-analyzer.md`
- CLAUDE.md wile-goast skill section (replaced by MCP self-description)

## Dependencies

- `github.com/mark3labs/mcp-go` (new)
- No other new dependencies

## MCP Library

`mark3labs/mcp-go` chosen over `modelcontextprotocol/go-sdk` for:
- Lighter dependency footprint (no JWT/OAuth)
- No Go version bump required (project is on 1.24)
- Builder pattern fits our simple surface (one string tool, three prompts)
- Most adopted Go MCP library

## Alternatives Considered

### Tool granularity

- **One tool per primitive (~18 tools)**: Maximum discoverability but rigid —
  adding a primitive means updating the MCP surface. Rejected.
- **Layer-oriented tools (~6 tools)**: Middle ground. Deferred — can add later
  if `eval` proves too opaque for some clients.
- **Single `eval` tool**: Chosen. Maximum flexibility, thinnest wrapper.
  Clients that know Scheme get full power. Prompts provide guidance for
  clients that don't.

### Binary architecture

- **Separate binary (`cmd/wile-goast-mcp/`)**: Clean separation but doubles
  install friction. Rejected — the shared code is one function.
- **Flag on existing binary (`--mcp`)**: Chosen. One binary, one install.

### MCP resources

Deferred to a future version. Candidates: embedded scripts, primitive docs,
analyzer list. Not needed for v1 — prompts carry the guidance.
