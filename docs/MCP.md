# MCP Server

`wile-goast` exposes its analysis surface over the Model Context Protocol. Two
transports share the same tool, pipeline tools, and prompts.

- `wile-goast --mcp` — stdio MCP server (JSON-RPC). One session keyed `"stdio"`.
- `wile-goast --http[=ADDR]` — Streamable HTTP MCP server. `--http` alone binds
  `127.0.0.1:8080` (loopback); `--http=:9000` binds all interfaces. Endpoint is
  `/mcp`. Stateful sessions; graceful shutdown on SIGINT/SIGTERM.

**Engine model:** one `*wile.Engine` per MCP session, keyed by `SessionID()`,
built lazily on first `eval` and closed on session unregister (clean DELETE or
30-min idle sweep). This isolates each HTTP client's state (`go-load` sessions,
defined beliefs) and serializes concurrent `eval`s within a session via a
per-engine mutex. stdio is the degenerate one-session case. See
`plans/2026-05-30-http-mcp-server-design.md`.

## Tools

- `eval`: takes a `code` string (Scheme expression), returns the result.
  Results over 16 KB (`maxEvalResultBytes`) are truncated on a UTF-8 boundary
  and annotated with a projection hint; on a Scheme error the result carries the
  distilled cheatsheet (`cmd/wile-goast/reference/cheatsheet.md`) so the caller
  can self-correct. Task-indexed description: duplicates, call paths,
  checked-before-use, cross-site beliefs.
- `reference`: returns the distilled Scheme cheatsheet (exact primitive
  arities, the parse→query→project pattern, missing builtins, small-output
  idioms). Single source of truth, also appended to `eval` errors. Load it
  before writing a non-trivial `eval`.

## Pipeline Tools (Phase 1)

Five coarse-grained tools wrap already-implemented analyses, returning a
`{version, provenance, result}` JSON envelope (via `NewToolResultJSON`,
both text + `structuredContent`) instead of requiring `eval`-driven
orchestration. Registered in `newServer` (`registerPhase1Tools`), so they
appear on both stdio and HTTP. Implemented in `lib/wile/goast/pipelines.scm`
(library `(wile goast pipelines)`) with handlers in `cmd/wile-goast/mcp_tools.go`.

| Tool | Parameters | Result |
|------|------------|--------|
| `check_beliefs` | `target`, `beliefs_path` | per-belief adherence/deviation list |
| `discover_beliefs` | `target`, `discovery_path`, `committed_path?` | `{emitted_source, filtered_results}` |
| `recommend_split` | `target`, `idf_threshold?`, `refine?`, `max_attributes?` | split proposal + `confidence` |
| `recommend_boundaries` | `target`, `mode?` | `{splits, merges, extracts}` Pareto frontiers |
| `find_false_boundaries` | `target`, `mode?`, `min_extent?`, `min_intent?`, `min_types?` | cross-boundary report |

- Envelope `version` is a per-tool integer (bumped only on breaking
  `result`-shape changes); errors surface via MCP's `isError`, not the
  envelope. JSON keys are snake_case; Scheme alists stay kebab-case
  (converted by `cmd/wile-goast/marshal.go`).
- `mode` is the FCA field-access mode: `write-only` (default),
  `read-write`, or `type-only`.
- Prefer a pipeline tool for its known structural query; use `eval` for
  open-ended exploration. Design: `plans/2026-04-19-mcp-tool-surface-{design,impl}.md`.

## Prompts

| Prompt | Description |
|--------|-------------|
| `goast-analyze` | Structural analysis — layer selection, primitive reference, examples |
| `goast-beliefs` | Belief DSL — define and run consistency beliefs |
| `goast-refactor` | Unification detection — find duplicates, verify refactoring |
| `goast-scheme-ref` | Wile Scheme reference — primitives, idioms, exports, gotchas |
| `goast-split` | Package cohesion analysis and split recommendations |

Prompt content lives in `cmd/wile-goast/prompts/*.md` (embedded in binary).

The `goast-scheme-ref` prompt is the long-form Scheme reference; the `reference`
tool serves the distilled short form from `cmd/wile-goast/reference/cheatsheet.md`,
which is also appended to `eval` error results.

## Client Config

stdio:

```json
{"mcpServers": {"wile-goast": {"command": "wile-goast", "args": ["--mcp"]}}}
```

Streamable HTTP (server started separately via `wile-goast --http`):

```json
{"mcpServers": {"wile-goast": {"type": "streamable-http", "url": "http://127.0.0.1:8080/mcp"}}}
```
