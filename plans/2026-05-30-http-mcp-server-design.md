# Streamable HTTP MCP transport for wile-goast

**Date:** 2026-05-30
**Status:** Implemented

## Goal

Add a Streamable HTTP transport to the wile-goast MCP server, alongside the
existing stdio transport. Same tool (`eval`) and prompts; the only new concern
is concurrency, because HTTP serves multiple clients/requests in parallel while
stdio is serial.

## Background

The protocol server (tools + prompts) is transport-agnostic in mcp-go. Today
`doMCP` (`cmd/wile-goast/mcp.go`) builds a `*server.MCPServer` and calls
`server.ServeStdio`. The server holds a single shared `*wile.Engine`, lazily
built via `sync.Once`. stdio processes requests serially, so concurrent access
to the engine never happens.

A `*wile.Engine` has a shared mutable global environment and is **not**
concurrency-safe. Under HTTP, two problems appear:

1. **Cross-client state leakage** — a single global engine means one client's
   `(go-load ...)` sessions and `define-belief` state become visible to all
   clients.
2. **Data races** — concurrent `EvalMultiple` on one engine.

## Decisions (locked)

- **Engine model:** per-session engine. Each MCP session gets its own engine,
  keyed by `ClientSession.SessionID()`. Isolation across clients; cross-call
  state preserved per client.
- **Transport:** Streamable HTTP only (`server.NewStreamableHTTPServer`). SSE is
  legacy; not exposed.
- **CLI:** new `--http[=ADDR]` flag, default `127.0.0.1:8080` (loopback).
  Mutually exclusive with `--mcp`, `-e`, `-f`, `--run`, `--list-scripts`.

## Architecture

```
   stdio ───────▶ newServer() ─┐
   http  ───────▶ newServer() ─┴─▶ *server.MCPServer  (eval tool + 5 prompts)
                                         │ ClientSessionFromContext(ctx)
                                         ▼
                                 engineManager: sessionID → *engineEntry
                                 OnUnregisterSession → Close + evict
```

The per-session model subsumes the single-engine model. stdio is the degenerate
case: exactly one session with the constant ID `"stdio"`
(`mcp-go/server/stdio.go:127`), registered at `Listen` startup and unregistered
at shutdown. No transport special-casing.

## Components

### `engineEntry`
Holds one engine, built lazily, plus an eval mutex.

```go
type engineEntry struct {
    once   sync.Once
    engine *wile.Engine
    err    error
    evalMu sync.Mutex   // serializes EvalMultiple within a session
}
```

- Lazy build reuses `buildEngineOrError`. A session that only fetches prompts
  never pays the `KitchenSink` build cost.
- `evalMu` handles intra-session concurrency: one client may pipeline concurrent
  requests onto its single engine; the mutex serializes them. Per-session engine
  gives cross-session isolation; `evalMu` gives within-session safety.

### `mcpServer` (revised)
Replace the `engine` / `engineOnce` / `engineErr` fields with:

```go
type mcpServer struct {
    mu      sync.Mutex
    engines map[string]*engineEntry   // keyed by session ID
}
```

- `getEngine(ctx)`: resolve session via `server.ClientSessionFromContext(ctx)`
  (fall back to key `""` if nil), get-or-create the entry under `mu`, then build
  under the entry's `once`. The manager lock is **not** held during the build.
- `handleEval`: `entry := getEngine(ctx)`; `entry.evalMu.Lock()`;
  `entry.engine.EvalMultiple(...)`; unlock. Build failure returns a tool error,
  not a crash (mirrors current behavior).

### Lifecycle hook
`OnUnregisterSession`: pop the entry under `mu`, then `Close()` its engine if
built. Registered via `server.WithHooks`. Only the unregister hook is needed —
build is lazy, so no register hook.

Idle/abandoned HTTP sessions are reaped by the library sweeper: its
`cleanupSessionState` calls `UnregisterSession` (`streamable_http.go:892`),
firing the hook. Enabled via `WithSessionIdleTTL`.

### `newServer()`
Shared construction extracted from the current `doMCP`: instructions, `eval`
tool, prompt registration, `WithHooks`. Called by both `doMCP` and `doHTTP`.

### `doMCP` (revised)
`s := ms.newServer(); return server.ServeStdio(s)`.

### `doHTTP(ctx, addr)`
```go
s := ms.newServer()
httpSrv := server.NewStreamableHTTPServer(s,
    server.WithStateful(true),                 // persist Mcp-Session-Id across requests
    server.WithSessionIdleTTL(httpIdleTTL),    // reap abandoned sessions → frees engines
)
// Start in goroutine; graceful Shutdown on SIGINT/ctx cancel via signal.NotifyContext.
```

- `WithStateful(true)` swaps in `InsecureStatefulSessionIdManager` so the same
  `Mcp-Session-Id` maps to the same engine across requests. "Insecure" = no
  per-request crypto validation; acceptable for a loopback dev tool.
- Logs the listen address and `/mcp` endpoint to stderr on startup.
- `httpIdleTTL` is a package constant (30 min). Not a flag — YAGNI until tuning
  is actually needed.

## CLI surface

`main.go` `Options`:

```go
HTTP string `long:"http" optional:"yes" optional-value:"127.0.0.1:8080" description:"Start MCP server over Streamable HTTP at ADDR (default 127.0.0.1:8080)"`
```

- `--http` alone → `127.0.0.1:8080`.
- `--http=:9000` → all interfaces, port 9000.
- Guard in `main()`: `--http` set ⇒ reject combination with `--mcp`, `-e`,
  `-f`, `--run`, `--list-scripts` (mirror the existing `--mcp` guard at
  `main.go:94`). go-flags reports a set optional-value flag via a non-empty
  default; detect "flag present" by comparing against an unset sentinel —
  implement by checking `opts.HTTP != ""` after parse is insufficient because
  the default only applies when the flag is given. Confirmed go-flags only
  populates `optional-value` when the flag appears, so `opts.HTTP != ""` is a
  valid "HTTP mode requested" test.

## Error handling

- Reuse the `errMCPServer` sentinel + `werr.WrapForeignErrorf`.
- Per-session engine build failure → tool error (`mcp.NewToolResultError`).
- Listen / shutdown failure → wrapped error returned from `doHTTP`.

## Testing

### Unit — engine manager
- Same session key → same `*wile.Engine` instance across `getEngine` calls.
- Distinct keys → distinct engines.
- `onUnregister(key)` closes and removes the entry; a subsequent `getEngine` for
  that key builds a fresh engine.

### Integration — HTTP transport
Following the existing `*_integration_test.go` style:
- Start `doHTTP` on an ephemeral port (`127.0.0.1:0`); drive with mcp-go's
  `client.NewStreamableHttpClient`.
- `eval` returns correct results (e.g. `(+ 1 2)` → `3`).
- **State isolation:** define a binding (e.g. `(define x 42)`) in session A,
  confirm `x` is unbound in a second client session B.

## Trade-offs

- Optimizing for **client isolation** over memory: N concurrent clients = N
  engines, each paying the `KitchenSink` build cost on first eval. Acceptable
  for an analysis tool with few simultaneous clients.
- Not optimizing for horizontal scaling — `InsecureStatefulSessionIdManager`
  needs sticky sessions; irrelevant for a single-process loopback tool.

## Out of scope

- Authentication / TLS (loopback default sidesteps it; `WithTLSCert` exists if
  needed later).
- SSE transport.
- Configurable idle TTL flag.
