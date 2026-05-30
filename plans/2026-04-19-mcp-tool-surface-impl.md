# MCP Tool Surface — Phase 1 Implementation Plan

> **Status:** Draft (2026-04-23). Scoped to Phase 1 of
> `plans/2026-04-19-mcp-tool-surface-design.md`. Phases 2-4 pair with
> their own impl plans when their design prerequisites ship.
>
> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development` or
> `superpowers:executing-plans` to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose the five already-implemented Phase 1 pipelines as MCP
tools on the `wile-goast --mcp` server: `check_beliefs`,
`discover_beliefs`, `recommend_split`, `recommend_boundaries`,
`find_false_boundaries`. Each tool returns a structured report
(Scheme s-expression text) wrapped in a common envelope carrying
version + provenance. The existing `eval` tool remains unchanged; the
new tools are additive.

**Architecture:**

- A new Scheme module `lib/wile/goast/pipelines.scm`
  (library `(wile goast pipelines)`) encapsulates each pipeline as a
  single top-level procedure. Each returns an alist with keys
  `(tool, version, provenance, result)`. Encapsulation keeps Go
  handlers uniform and makes pipelines reachable from `eval` for
  `eval`-mode composition (the design doc §171 requirement).
- Go-side changes live in a new `cmd/wile-goast/mcp_tools.go`
  containing one handler per tool plus a shared `invokePipeline` helper
  that formats the Scheme call, invokes the engine, and wraps errors as
  `mcp.NewToolResultError`. The existing `handleEval` and prompt
  registration in `mcp.go` are unchanged.
- Integration tests in `cmd/wile-goast/mcp_tools_integration_test.go`
  use `client.NewInProcessClient(server)` from `mcp-go v0.45.0`
  against a small test package in
  `cmd/wile-goast/testdata/phase1/` to exercise the full call path.

**Tech Stack:** Go (handlers, tests), Wile R7RS Scheme (pipelines).
Wile primitives used: existing goast/ssa/cfg/callgraph primitives plus
belief-suppression procedures shipped 2026-04-23 in commit `846a5dd`.
Go test harness: `quicktest` + `mcp-go`'s `client.NewInProcessClient`.

**Parent design:** `plans/2026-04-19-mcp-tool-surface-design.md`.

**Adjacent plans:**

- `plans/2026-04-17-belief-suppression-design.md` —
  `with-belief-scope`, `load-committed-beliefs`, `suppress-known`
  (shipped 2026-04-23, consumed by `check_beliefs` and
  `discover_beliefs`).
- `plans/2026-04-17-fca-duplicate-detection-design.md` —
  `find_duplicates` (Phase 3, not in scope).
- `plans/2026-04-19-llm-concept-filter-design.md` —
  `filter_concepts` (Phase 2, not in scope).

**Project conventions observed:**

- Direct push to master (per `CLAUDE.local.md`).
- No Co-Authored-By lines in commit messages.
- `.sld` exports every symbol explicitly; tests import
  `(wile goast pipelines)`.
- Go tests use quicktest (`qt.New(t)`, `qt.Assert`, `qt.Equals`).
- Scheme-runner helpers live in `goast/prim_goast_test.go:42`
  (`eval`) and `cmd/wile-goast/session_integration_test.go`
  (`testutil.RunScheme`). MCP-level tests use a new
  `inProcessClient(t)` helper (see Task 1).
- `make lint` + `make test` must pass before claiming done.
- `VERSION` is auto-bumped by pre-commit hook — do not touch.

---

## Locked decisions (resolved 2026-05-29)

The five impl-plan decision points were resolved in a design pass on
2026-05-29. Rationales recorded in
`memory/mcp-tool-surface-decisions.md`. Summary here for plan readers.

1. **Output format — JSON via `NewToolResultJSON`.** Marshals into both
   text content (JSON string) and `structuredContent` field. Matches
   conventional MCP servers. Forces a Wile→JSON marshaller (Task 1.5).
   Rationale: Postel — be conservative in what you emit; every peer
   MCP server emits JSON.
2. **`discover_beliefs` parameterization — three parameters.**
   `target` (required Go package pattern), `discovery_path` (required
   `.scm` file or directory), `committed_path` (optional; omit for no
   suppression). Suppression defaults to off — raw discovery is the
   additive baseline. Asymmetry with `check_beliefs` (one belief
   parameter) is intentional: the pipelines have different belief-set
   roles.
3. **Envelope shape — flat `{version, provenance, result}`.** No `tool`
   field — peer protocols (JSON-RPC, MCP `CallToolResult`, GraphQL,
   REST) don't echo the call name in responses. Per-tool integer
   versioning (`"version": 1`), bumped only on breaking changes to
   that tool's `result` shape. Errors via MCP's `isError` flag, not
   an envelope field. JSON keys snake_case at the boundary;
   Scheme alists stay kebab-case internally.
4. **Session reuse — deferred indefinitely.** No `session_id`
   parameter on any Phase 1 tool. No `go_load` MCP tool. Every tool
   takes `target: string`. If package-load performance becomes
   load-bearing, the conventional response is server-side internal
   caching keyed on `(target_pattern, fs-state)` — below the MCP
   surface, no client-visible handles. Convention check: LSP,
   JSON-RPC, REST, MCP itself all keep state server-side, addressed
   by stable identifiers.
5. **Built-in discovery beliefs — deferred.** `discovery_path` is
   required in Phase 1. No bundled default belief set — curating one
   is a separate research/stability commitment that wile-goast
   (v0.5.189) is too early to take on. Forward-compatible path: if a
   curated set ships later, `discovery_path` becomes optional with
   the built-in as default (strictly additive). Phase 1 follow-up:
   ship 2-3 example discovery beliefs in `examples/discovery-beliefs/`
   as documentation templates (not loaded by default).

### Wile→JSON marshaller mapping (consequence of #1)

Each Wile value type maps to JSON via the following table. Built once
in Task 1.5, reused across all Phase 1 tools.

| Wile type | JSON output |
|---|---|
| Rational `9/10` | string `"9/10"` (exact value preserved) |
| Symbol `'strong` | string `"strong"` (loses symbol/string distinction — fine for enum fields) |
| Quoted s-expr (`sites-expr`, `expect-expr`, `analyze-expr`) | string of Scheme code |
| Alist `((k . v) ...)` (keys are symbols) | object `{"k": v}` with kebab→snake key conversion |
| Proper list `(a b c)` | array `[a, b, c]` |
| Dotted pair in non-alist position | `{"car": ..., "cdr": ...}` fallback |
| Boolean `#t` / `#f` | `true` / `false` |
| String | string |
| Integer / float | number |
| `#!void` | omit field (caller decides via `if val.IsVoid()`) |

No invented tagging schemes — keep emission conventional.

---

## File Structure

> **Path note (post-`7ee305d` refactor):** Scheme libraries live at the
> project root under `lib/wile/goast/`, embedded via top-level
> `package wilegoast` (`embed.go:24` → `wilegoast.Lib`). The CLI binary
> picks them up via `wile.WithSourceFS(wilegoast.Lib)` in
> `cmd/wile-goast/main.go:175`.

**Create:**

- `lib/wile/goast/pipelines.scm` — five pipeline procedures + shared
  envelope helper.
- `lib/wile/goast/pipelines.sld` — R7RS library definition, exports
  the five procedures and the envelope constructor.
- `cmd/wile-goast/marshal.go` — Wile value → Go `any` walker
  producing JSON-marshallable maps/slices per the locked mapping
  table. Reused across all tool handlers.
- `cmd/wile-goast/marshal_test.go` — type-coverage tests for every
  Wile value kind in the mapping table.
- `cmd/wile-goast/mcp_tools.go` — Go tool handlers + shared
  `invokePipeline` helper.
- `cmd/wile-goast/mcp_tools_integration_test.go` — integration tests
  driving the MCP server via `client.NewInProcessClient`.
- `cmd/wile-goast/testdata/phase1/` — minimal Go package used by
  integration tests (two structs + a handful of methods — enough
  to exercise each pipeline).

**Modify:**

- `cmd/wile-goast/mcp.go` — register the five new tools in `doMCP`
  via a new `registerPhase1Tools(s)` method on `mcpServer`.
- `cmd/wile-goast/main.go` — no changes (the new pipelines library is
  picked up via existing `wile.WithSourceFS(wilegoast.Lib)` +
  `WithLibraryPaths("lib")`).
- `CLAUDE.md` — add MCP tool surface section documenting the five
  tools.
- `CHANGELOG.md` — prepend entry.
- `plans/CLAUDE.md` — flip row status for the design doc; add impl
  row.

**Do not modify:**

- `cmd/wile-goast/mcp.go`'s `handleEval`, prompt registration, or
  `mcpServer` struct fields beyond tool registration.
- Any file in `goast/`, `goastssa/`, `goastcfg/`, `goastcg/`,
  `goastlint/` — all pipeline code lives in Scheme.
- `VERSION`.

---

## Task 1: Envelope helper + pipelines library skeleton

**Files:**

- Create: `lib/wile/goast/pipelines.scm`
- Create: `lib/wile/goast/pipelines.sld`
- Create: `cmd/wile-goast/mcp_tools_integration_test.go`

- [ ] **Step 1: Create `pipelines.sld`**

```scheme
(define-library (wile goast pipelines)
  (export
    ;; Shared envelope constructor (tool-specific version supplied per call)
    pipeline-envelope
    ;; Tool procedures
    pipeline-check-beliefs
    pipeline-discover-beliefs
    pipeline-recommend-split
    pipeline-recommend-boundaries
    pipeline-find-false-boundaries)
  (import
    (wile goast)
    (wile goast ssa)
    (wile goast belief)
    (wile goast fca)
    (wile goast fca-algebra)
    (wile goast fca-recommend)
    (wile goast split)
    (wile goast utils))
  (include "pipelines.scm"))
```

- [ ] **Step 2: Create `pipelines.scm` with the envelope helper only**

```scheme
;; ── Shared envelope ─────────────────────────────────────
;;
;; Every Phase 1 tool returns:
;;   ((version    . <integer>)
;;    (provenance . <alist>)
;;    (result     . <alist-or-list>))
;;
;; The version is a per-tool integer, supplied by each pipeline. It is
;; bumped only on breaking changes to that tool's result shape;
;; additive provenance changes do not bump.
;;
;; The Go handler converts this alist to JSON via the marshaller in
;; cmd/wile-goast/marshal.go. Alist keys are emitted in kebab-case
;; here; the marshaller normalises to snake_case at the JSON boundary.
;;
;; No `tool` field: peer protocols (JSON-RPC, MCP, GraphQL, REST) do
;; not echo the call name in responses.

(define (pipeline-envelope version provenance result)
  (list (cons 'version version)
        (cons 'provenance provenance)
        (cons 'result result)))
```

- [ ] **Step 3: Add the in-process MCP client test helper**

Create `cmd/wile-goast/mcp_tools_integration_test.go` with:

```go
package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	qt "github.com/frankban/quicktest"
)

// inProcessClient returns a ready-to-use in-process MCP client and the
// underlying server. The server has all Phase 1 tools registered.
func inProcessClient(t *testing.T) *client.Client {
	t.Helper()
	ms := &mcpServer{}
	s := server.NewMCPServer("wile-goast-test", "test",
		server.WithToolCapabilities(true))
	ms.registerPhase1Tools(s)
	c, err := client.NewInProcessClient(s)
	if err != nil {
		t.Fatalf("new in-process client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := c.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return c
}

// callTool calls a tool and returns its structured content as a
// generic map. Tools return JSON via NewToolResultJSON, which
// populates both the text content (JSON string) and structuredContent;
// callers can assert against either, but structuredContent is the
// canonical form for typed inspection.
func callTool(t *testing.T, c *client.Client, name string, args map[string]any) map[string]any {
	t.Helper()
	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := c.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if res.IsError {
		for _, c := range res.Content {
			if tc, ok := c.(mcp.TextContent); ok {
				t.Fatalf("tool %s reported error: %s", name, tc.Text)
			}
		}
		t.Fatalf("tool %s reported error (no text content)", name)
	}
	m, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("tool %s returned non-map structured content: %T", name, res.StructuredContent)
	}
	return m
}

// envelopeOK fails the test if the returned envelope is missing the
// version or provenance keys, or if version differs from expected.
// Leaves result-shape assertions to per-tool tests.
func envelopeOK(t *testing.T, envelope map[string]any, expectedVersion float64) {
	t.Helper()
	c := qt.New(t)
	c.Assert(envelope["version"], qt.Equals, expectedVersion)
	c.Assert(envelope["provenance"], qt.Not(qt.IsNil))
	c.Assert(envelope["result"], qt.Not(qt.IsNil))
}
```

Note: `expectedVersion` is `float64` because `encoding/json` decodes
numbers into `float64` by default when unmarshalling into `any`. The
JSON output is `"version": 1`; the test parses it back as `1.0`.

`registerPhase1Tools` does not exist yet; Task 2 adds it. Referencing
it here is deliberate — this step creates a compile error that Task 2
resolves.

- [ ] **Step 4: Confirm compile fails at this step**

```
cd /Users/aalpar/projects/wile-workspace/wile-goast
go build ./cmd/wile-goast/
```

Expected: FAIL — undefined `registerPhase1Tools`. Proceed to Task 2.

- [ ] **Step 5: Commit (ask first)**

> "Task 1: envelope helper + in-process test harness. Commit
> intentionally-broken state so Task 2 can show the fix?"

On approval:

```bash
git add lib/wile/goast/pipelines.scm \
        lib/wile/goast/pipelines.sld \
        cmd/wile-goast/mcp_tools_integration_test.go && \
  git commit -m "feat(mcp): pipelines library skeleton + test harness"
```

Alternative if the broken-state commit feels wrong: hold the commit
until Task 2 compiles.

---

## Task 1.5: Wile→JSON marshaller

**Files:**

- Create: `cmd/wile-goast/marshal.go`
- Create: `cmd/wile-goast/marshal_test.go`

The pipelines library produces Scheme alists. The MCP tool handlers
must emit JSON via `mcp.NewToolResultJSON(data)`, which calls
`json.Marshal` on a Go `any`. Bridging requires a Wile-value walker
that produces `map[string]any` / `[]any` / scalars per the locked
mapping table (see "Locked decisions" §1).

Build this once, here, before any tool handler is written. Every
tool handler invokes `marshalToJSON(val) (any, error)` on the
pipeline's returned Scheme value, then passes the result to
`mcp.NewToolResultJSON`.

- [ ] **Step 1: Declare the marshaller error sentinel and signature**

```go
package main

import (
	"fmt"
	"strings"

	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

var errMarshalUnsupported = werr.NewStaticError("marshal: unsupported wile value kind")

// marshalToJSON converts a Wile value into a Go any suitable for
// json.Marshal, following the Phase 1 mapping table. Returns
// errMarshalUnsupported if a value kind is not covered.
func marshalToJSON(v values.Value) (any, error) {
	// implementation per Step 2 below
}
```

- [ ] **Step 2: Implement the type dispatch**

The implementation switches on `v.Kind()` and handles each kind. Key
rules from the mapping table:

```go
func marshalToJSON(v values.Value) (any, error) {
	switch {
	case v == nil || v.IsVoid():
		return nil, nil
	case v.IsBool():
		return v.Bool(), nil
	case v.IsString():
		return v.String(), nil
	case v.IsInteger():
		return v.Integer(), nil
	case v.IsFloat():
		return v.Float(), nil
	case v.IsRational():
		// Exact preservation — string per locked mapping.
		return v.SchemeString(), nil  // prints as "9/10"
	case v.IsSymbol():
		// Loses symbol/string distinction; acceptable for enum-shaped
		// fields. The Scheme side must not use symbols where the
		// string/symbol distinction is semantically load-bearing in
		// the JSON output.
		return v.Symbol(), nil
	case v.IsPair():
		return marshalPair(v)
	case v.IsNull():
		return []any{}, nil
	case v.IsVector():
		return marshalVector(v)
	}
	return nil, werr.WrapForeignErrorf(errMarshalUnsupported,
		"kind=%s", v.Kind())
}
```

`marshalPair` is the tricky one. Two cases:

1. **Alist of symbol-keyed pairs** — emit as JSON object. Detect by
   walking the list and checking that every car is a pair whose car
   is a symbol. Convert each key from kebab-case to snake_case.
2. **Anything else** — emit as JSON array (proper list) or
   `{"car":..., "cdr":...}` fallback (dotted pair in non-alist
   position).

```go
func marshalPair(v values.Value) (any, error) {
	if isAlist(v) {
		obj := map[string]any{}
		for cur := v; !cur.IsNull(); cur = cur.Cdr() {
			entry := cur.Car()
			key := kebabToSnake(entry.Car().Symbol())
			val, err := marshalToJSON(entry.Cdr())
			if err != nil {
				return nil, err
			}
			obj[key] = val
		}
		return obj, nil
	}
	if isProperList(v) {
		var arr []any
		for cur := v; !cur.IsNull(); cur = cur.Cdr() {
			elem, err := marshalToJSON(cur.Car())
			if err != nil {
				return nil, err
			}
			arr = append(arr, elem)
		}
		return arr, nil
	}
	// Dotted pair fallback
	car, err := marshalToJSON(v.Car())
	if err != nil {
		return nil, err
	}
	cdr, err := marshalToJSON(v.Cdr())
	if err != nil {
		return nil, err
	}
	return map[string]any{"car": car, "cdr": cdr}, nil
}

func kebabToSnake(s string) string {
	return strings.ReplaceAll(s, "-", "_")
}
```

Verify the Wile values API (`v.Car()`, `v.Cdr()`, `v.Symbol()`,
predicates) against `github.com/aalpar/wile/values` before
committing — function names may differ in v1.16.0.

- [ ] **Step 3: Write the type-coverage test**

`marshal_test.go` must exercise every row of the mapping table. Use
table-driven tests, one case per Wile value kind. Verify by parsing
the JSON output back with `encoding/json`:

```go
func TestMarshalToJSON(t *testing.T) {
	cases := []struct {
		name   string
		scheme string  // Scheme source producing the value
		expect string  // expected JSON output (after re-marshalling)
	}{
		{"integer", "42", `42`},
		{"float", "3.14", `3.14`},
		{"rational", "9/10", `"9/10"`},
		{"symbol", "'strong", `"strong"`},
		{"string", `"hello"`, `"hello"`},
		{"true", "#t", `true`},
		{"false", "#f", `false`},
		{"null", "'()", `[]`},
		{"proper list", "'(1 2 3)", `[1,2,3]`},
		{"alist", `'((a . 1) (b . 2))`, `{"a":1,"b":2}`},
		{"alist kebab→snake", `'((sites-expr . "x"))`, `{"sites_expr":"x"}`},
		{"nested alist", `'((outer . ((inner . 1))))`, `{"outer":{"inner":1}}`},
		{"dotted pair", `'(1 . 2)`, `{"car":1,"cdr":2}`},
	}
	// ... run each via the test engine, marshal, assert ...
}
```

- [ ] **Step 4: Run the tests**

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast
go test ./cmd/wile-goast/ -run TestMarshalToJSON
```

Expected: PASS. If any kind fails, fix the marshaller before moving
on. The marshaller is on the hot path of every Phase 1 tool — every
miscoded type mapping is a JSON corruption across the surface.

- [ ] **Step 5: Commit**

```bash
git add cmd/wile-goast/marshal.go cmd/wile-goast/marshal_test.go && \
  git commit -m "feat(mcp): Wile→JSON marshaller for pipeline tool output"
```

**Decision points exercised in this task:**

- Symbol/string collapse: confirmed lossy for enum fields; if any
  pipeline ever needs symbol-typed output round-trippable through
  JSON, the marshaller adds a tagged form (`{"_sym": "x"}`) at that
  point. Not now.
- Rational rendering: `"9/10"` is the locked emit; consumers parse if
  arithmetic is needed.
- Key conversion: applied at every alist key, not just top-level.
  Nested objects also get snake_case keys.

---

## Task 2: `check_beliefs` — run committed beliefs against a target

The simplest pipeline: user provides `beliefs_path` (file or
directory) and `package`; tool loads beliefs, runs them, returns the
result alist.

**Files:**

- Modify: `lib/wile/goast/pipelines.scm`
- Modify: `lib/wile/goast/pipelines.sld` (already
  exports `pipeline-check-beliefs` from Task 1)
- Create: `cmd/wile-goast/mcp_tools.go`
- Create: `cmd/wile-goast/testdata/phase1/` (minimal Go package)

- [ ] **Step 1: Build the test fixture package**

Create `cmd/wile-goast/testdata/phase1/phase1.go`:

```go
// Package phase1 is a tiny Go package used only by MCP phase-1
// integration tests. It contains two simple structs with a handful
// of methods exercising FCA, split, and belief pipelines.
package phase1

import "sync"

type Counter struct {
	mu    sync.Mutex
	value int
}

func (c *Counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value++
}

func (c *Counter) Read() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

type Cache struct {
	data map[string]int
}

func (c *Cache) Get(k string) (int, bool) {
	v, ok := c.data[k]
	return v, ok
}

func (c *Cache) Put(k string, v int) {
	c.data[k] = v
}
```

Minimal, deterministic, non-trivial — enough to exercise
struct-field FCA and a lock-pairing belief.

- [ ] **Step 2: Write the first failing test**

Append to `cmd/wile-goast/mcp_tools_integration_test.go`:

```go
func TestCheckBeliefs_LockPairing(t *testing.T) {
	c := qt.New(t)
	mc := inProcessClient(t)

	dir := t.TempDir()
	belief := filepath.Join(dir, "lock.scm")
	mustWriteFile(t, belief, `
		(import (wile goast belief))
		(define-belief "lock-unlock"
		  (sites (functions-matching (contains-call "Lock")))
		  (expect (paired-with "Lock" "Unlock"))
		  (threshold 0.9 1))
	`)

	env := callTool(t, mc, "check_beliefs", map[string]any{
		"target":       "github.com/aalpar/wile-goast/cmd/wile-goast/testdata/phase1",
		"beliefs_path": dir,
	})
	envelopeOK(t, env, 1.0)
	// Both Counter methods pair Lock with Unlock — belief should be strong.
	result := env["result"]
	c.Assert(result, qt.Not(qt.IsNil))
	// Assertions on result shape are tool-specific; the marshaller
	// emits result as []any (list of belief result objects). Drill
	// into specific keys here.
}
```

`mustWriteFile` + `schemeStr` are available via the existing
`goast/belief_integration_test.go` helpers — re-declare them in this
file's helper section or factor to `testutil` (Resolved Ambiguities
§1).

- [ ] **Step 3: Run, confirm failure (tool does not exist yet)**

```
go test ./cmd/wile-goast/ -run TestCheckBeliefs_LockPairing -v
```

Expected: FAIL.

- [ ] **Step 4: Implement `pipeline-check-beliefs` in Scheme**

Append to `lib/wile/goast/pipelines.scm`:

```scheme
;; ── check_beliefs ────────────────────────────────────────
;;
;; Load committed/candidate beliefs from BELIEFS-PATH, run them
;; against TARGET, return the per-belief result list under result.
;; Provenance records belief count and paths probed.

(define (pipeline-check-beliefs target beliefs-path)
  (with-belief-scope
    (lambda ()
      (let* ((committed (load-committed-beliefs beliefs-path))
             ;; load-committed-beliefs registers beliefs inside the
             ;; scope then returns a snapshot — but we want them active
             ;; for run-beliefs. Re-register from the snapshot.
             (per-site (car committed))
             (_ (for-each register-belief! per-site))
             (results (run-beliefs target)))
        (pipeline-envelope 1
          (list (cons 'target target)
                (cons 'beliefs-path beliefs-path)
                (cons 'belief-count (length per-site)))
          results)))))
```

**Implementation note for the worker:** `register-belief!` may not
be a public procedure in the belief module. Two options:

  1. If the belief module exports an internal registration primitive,
     use it.
  2. Otherwise, load the `.scm` files directly inside the scope (not
     via `load-committed-beliefs`, which isolates them):

     ```scheme
     (define (pipeline-check-beliefs target beliefs-path)
       (with-belief-scope
         (lambda ()
           (let ((count 0))
             (for-each
               (lambda (f)
                 (guard (exn (#t #f))
                   (load f)
                   (set! count (+ count 1))))
               (if (file-exists? beliefs-path)
                 (list-scheme-files-in-dir-or-single beliefs-path)
                 (error "path does not exist" beliefs-path)))
             (pipeline-envelope 1
               (list (cons 'target target)
                     (cons 'beliefs-path beliefs-path)
                     (cons 'belief-count count))
               (run-beliefs target))))))
     ```

Decide based on what the belief module exports. **Decision point:**
if option 2, the pipeline needs to list `.scm` files under a path.
This is a filesystem concern, not a belief concern — see §Upstream
(wile) work at the bottom of this plan. Short-term: inline the
directory walk with a local `substring`-based suffix check and mark
it `;; TODO: replace with (string-suffix? ".scm" ...)  when wile
ships it`. Long-term: consume the wile-side helper.

- [ ] **Step 5: Implement the Go handler**

Create `cmd/wile-goast/mcp_tools.go`:

```go
// Copyright 2026 Aaron Alpar
// Licensed under the Apache License, Version 2.0 ...
// [boilerplate header as in mcp.go]

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerPhase1Tools registers the five Phase 1 pipeline tools on s.
func (ms *mcpServer) registerPhase1Tools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("check_beliefs",
			mcp.WithDescription("Run committed beliefs against a Go package. "+
				"Returns adherence/deviation report per belief. "+
				"Use when you have a directory of .scm belief files and want a "+
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

// invokePipeline builds the Scheme call from fmt+args, evaluates it on
// the engine, marshals the resulting Wile value to JSON-compatible
// Go any via marshalToJSON, and returns a tool result with both text
// (JSON string) and structured content populated. Tool-level errors
// become mcp.NewToolResultError; engine init failure becomes the same.
func (ms *mcpServer) invokePipeline(ctx context.Context, code string) (*mcp.CallToolResult, error) {
	engine, err := ms.getEngine(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("engine init: %v", err)), nil
	}
	val, err := engine.EvalMultiple(ctx, code)
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

// schemeStringLiteral quotes s as a Scheme string literal. Used to
// pass user-supplied paths safely into Scheme source code.
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
```

- [ ] **Step 6: Wire Phase 1 registration into `doMCP`**

In `cmd/wile-goast/mcp.go`, after the `s.AddTool("eval", ...)` call
and before `registerPrompts`, add:

```go
ms.registerPhase1Tools(s)
```

- [ ] **Step 7: Run the test, expect pass**

```
go test ./cmd/wile-goast/ -run TestCheckBeliefs_LockPairing -v
```

Expected: PASS. If it fails with "no sites found", the test fixture's
`sync.Lock` is on `sync.Mutex`; the belief's `(contains-call "Lock")`
should still match (`c.mu.Lock`). If it fails with a Scheme error,
probe the belief-loading branch decision from Step 4.

- [ ] **Step 8: Commit (ask first)**

> "Task 2: `check_beliefs` tool + test + registration. Commit?"

On approval:

```bash
git add lib/wile/goast/pipelines.scm \
        cmd/wile-goast/mcp_tools.go \
        cmd/wile-goast/mcp.go \
        cmd/wile-goast/mcp_tools_integration_test.go \
        cmd/wile-goast/testdata/phase1/phase1.go && \
  git commit -m "feat(mcp): add check_beliefs tool"
```

---

## Task 3: `discover_beliefs` — run discovery beliefs, suppress committed, emit source

**Files:**

- Modify: `lib/wile/goast/pipelines.scm`
- Modify: `cmd/wile-goast/mcp_tools.go`
- Modify: `cmd/wile-goast/mcp_tools_integration_test.go`

- [ ] **Step 1: Write the test**

```go
func TestDiscoverBeliefs_EmitsFiltered(t *testing.T) {
	c := qt.New(t)
	mc := inProcessClient(t)

	discoveryDir := t.TempDir()
	mustWriteFile(t, filepath.Join(discoveryDir, "discovery.scm"), `
		(import (wile goast belief))
		(define-belief "methods-have-body"
		  (sites (functions-matching (name-matches "")))
		  (expect (custom (lambda (site ctx)
		    (if (nf site 'body) 'has-body 'no-body))))
		  (threshold 0.9 1))
	`)
	committedDir := t.TempDir()
	// No committed beliefs — everything the discovery turns up should
	// appear in the emitted source.

	env := callTool(t, mc, "discover_beliefs", map[string]any{
		"target":         "github.com/aalpar/wile-goast/cmd/wile-goast/testdata/phase1",
		"discovery_path": discoveryDir,
		"committed_path": committedDir,
	})
	envelopeOK(t, env, 1.0)
	// emit-beliefs writes define-belief forms into the result; the
	// result is a map with emitted_source as a JSON string of Scheme.
	result := env["result"].(map[string]any)
	emitted := result["emitted_source"].(string)
	c.Assert(emitted, qt.Contains, "define-belief")
	c.Assert(emitted, qt.Contains, "methods-have-body")
}
```

- [ ] **Step 2: Run, confirm failure**

- [ ] **Step 3: Implement the Scheme pipeline**

Append to `pipelines.scm`:

```scheme
;; ── discover_beliefs ─────────────────────────────────────
;;
;; Run DISCOVERY-PATH beliefs against TARGET, suppress any result
;; whose expressions match a belief in COMMITTED-PATH, emit the
;; survivors as Scheme source text suitable for commit.
;;
;; COMMITTED-PATH may be #f or a path to an empty directory — in
;; either case no suppression is applied.

(define (pipeline-discover-beliefs target discovery-path committed-path)
  (let* ((results
           (with-belief-scope
             (lambda ()
               ;; Load the discovery beliefs into this scope, then run.
               (for-each
                 (lambda (f) (guard (exn (#t #f)) (load f)))
                 (scheme-files-under discovery-path))
               (run-beliefs target))))
         (committed
           (if (or (not committed-path) (equal? committed-path ""))
             (cons '() '())
             (load-committed-beliefs committed-path)))
         (filtered (suppress-known results committed))
         (emitted (emit-beliefs filtered)))
    (pipeline-envelope 1
      (list (cons 'target target)
            (cons 'discovery-path discovery-path)
            (cons 'committed-path (or committed-path ""))
            (cons 'raw-count (length results))
            (cons 'filtered-count (length filtered)))
      (list (cons 'emitted-source emitted)
            (cons 'filtered-results filtered)))))

;; Local helper — list .scm files directly under PATH. If PATH is a
;; single file ending in .scm, returns just (list PATH).
(define (scheme-files-under path)
  (guard (exn (#t (list path)))
    (list-scheme-files-in-dir path)))
```

**Locked per DP #2:** `discovery_path` required; `committed_path`
optional (empty string treated as "no committed beliefs, no
suppression — raw discovery returned").

- [ ] **Step 4: Register the tool**

Append to `registerPhase1Tools`:

```go
s.AddTool(
	mcp.NewTool("discover_beliefs",
		mcp.WithDescription("Run a directory of discovery beliefs against a Go package, "+
			"suppress any that match a committed belief, return survivors as "+
			"Scheme source ready to commit."),
		mcp.WithString("target", mcp.Required(),
			mcp.Description("Go package pattern")),
		mcp.WithString("discovery_path", mcp.Required(),
			mcp.Description("Path to discovery .scm file or directory")),
		mcp.WithString("committed_path",
			mcp.Description("Path to committed beliefs (optional). Empty string disables suppression.")),
	),
	ms.handleDiscoverBeliefs,
)
```

Add the handler (mirror of `handleCheckBeliefs`):

```go
func (ms *mcpServer) handleDiscoverBeliefs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target := req.GetString("target", "")
	discovery := req.GetString("discovery_path", "")
	committed := req.GetString("committed_path", "")
	if target == "" {
		return mcp.NewToolResultError("target parameter is required"), nil
	}
	if discovery == "" {
		return mcp.NewToolResultError("discovery_path parameter is required"), nil
	}
	code := `(import (wile goast pipelines))
(pipeline-discover-beliefs ` +
		schemeStringLiteral(target) + ` ` +
		schemeStringLiteral(discovery) + ` ` +
		schemeStringLiteral(committed) + `)`
	return ms.invokePipeline(ctx, code)
}
```

- [ ] **Step 5: Run, pass, commit**

```
go test ./cmd/wile-goast/ -run TestDiscoverBeliefs -v
```

On pass, ask and commit:

```bash
git add lib/wile/goast/pipelines.scm \
        cmd/wile-goast/mcp_tools.go \
        cmd/wile-goast/mcp_tools_integration_test.go && \
  git commit -m "feat(mcp): add discover_beliefs tool"
```

---

## Task 4: `recommend_split` — package cohesion analysis

**Files:**

- Modify: `lib/wile/goast/pipelines.scm`
- Modify: `cmd/wile-goast/mcp_tools.go`
- Modify: `cmd/wile-goast/mcp_tools_integration_test.go`

- [ ] **Step 1: Write the test**

```go
func TestRecommendSplit_Phase1Fixture(t *testing.T) {
	c := qt.New(t)
	mc := inProcessClient(t)

	env := callTool(t, mc, "recommend_split", map[string]any{
		"target": "github.com/aalpar/wile-goast/cmd/wile-goast/testdata/phase1",
	})
	envelopeOK(t, env, 1.0)
	// The fixture is too small to split meaningfully — expect NONE.
	result := env["result"].(map[string]any)
	c.Assert(result["confidence"], qt.Equals, "NONE")
}
```

Small fixture = NONE confidence; bigger fixture would give
MEDIUM/HIGH but is not needed for phase-1 wiring verification. To
stress the confidence path, add a second test against a known-split
candidate (e.g., `goast` itself — careful with runtime cost).

- [ ] **Step 2: Run, confirm failure**

- [ ] **Step 3: Implement the Scheme pipeline**

```scheme
;; ── recommend_split ──────────────────────────────────────
;;
;; Apply IDF-weighted FCA + min-cut to TARGET's per-function
;; import signatures. OPTS is an alist of option overrides.

(define (pipeline-recommend-split target opts)
  (let* ((session (go-load target))
         (refs (go-func-refs session))
         (kw-opts
           (append
             (maybe-kw opts 'idf-threshold)
             (if (assoc 'refine opts) '(refine) '())
             (maybe-kw opts 'max-attributes)))
         (report (apply recommend-split refs kw-opts)))
    (pipeline-envelope 1
      (list (cons 'target target)
            (cons 'options kw-opts)
            (cons 'function-count (length refs)))
      report)))

;; Translate an alist option into the two-element list form recommend-split
;; expects: 'idf-threshold N appears as ('idf-threshold N) in the opts list.
(define (maybe-kw opts key)
  (let ((e (assoc key opts)))
    (if e (list key (cdr e)) '())))
```

`go-func-refs` and `recommend-split` are both already implemented —
no new analysis code.

- [ ] **Step 4: Register the tool + handler**

```go
s.AddTool(
	mcp.NewTool("recommend_split",
		mcp.WithDescription("Analyze a Go package's cohesion and recommend a two-way split "+
			"via IDF-weighted FCA + min-cut. Returns split proposal with confidence."),
		mcp.WithString("target", mcp.Required(),
			mcp.Description("Go package pattern")),
		mcp.WithNumber("idf_threshold",
			mcp.Description("Minimum IDF to keep a package as a signature attribute (default 0.36)")),
		mcp.WithBoolean("refine",
			mcp.Description("Refine context by (package, object) granularity")),
		mcp.WithNumber("max_attributes",
			mcp.Description("Fail fast if attribute count exceeds this (default 30)")),
	),
	ms.handleRecommendSplit,
)
```

Handler:

```go
func (ms *mcpServer) handleRecommendSplit(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target := req.GetString("target", "")
	if target == "" {
		return mcp.NewToolResultError("target parameter is required"), nil
	}
	var optsParts []string
	if t, ok := req.GetArguments()["idf_threshold"]; ok {
		optsParts = append(optsParts, fmt.Sprintf("(idf-threshold . %v)", t))
	}
	if r, ok := req.GetArguments()["refine"]; ok {
		if b, _ := r.(bool); b {
			optsParts = append(optsParts, "(refine . #t)")
		}
	}
	if m, ok := req.GetArguments()["max_attributes"]; ok {
		optsParts = append(optsParts, fmt.Sprintf("(max-attributes . %v)", m))
	}
	code := `(import (wile goast pipelines))
(pipeline-recommend-split ` + schemeStringLiteral(target) +
		` (list ` + strings.Join(optsParts, " ") + `))`
	return ms.invokePipeline(ctx, code)
}
```

- [ ] **Step 5: Run, pass, commit**

---

## Task 5: `recommend_boundaries` — function split/merge/extract frontiers

**Files:**

- Modify: `lib/wile/goast/pipelines.scm`
- Modify: `cmd/wile-goast/mcp_tools.go`
- Modify: `cmd/wile-goast/mcp_tools_integration_test.go`

- [ ] **Step 1: Write the test**

```go
func TestRecommendBoundaries_Phase1Fixture(t *testing.T) {
	c := qt.New(t)
	mc := inProcessClient(t)

	env := callTool(t, mc, "recommend_boundaries", map[string]any{
		"target": "github.com/aalpar/wile-goast/cmd/wile-goast/testdata/phase1",
	})
	envelopeOK(t, env, 1.0)
	// Three frontier keys must appear, even if empty.
	result := env["result"].(map[string]any)
	c.Assert(result["splits"], qt.Not(qt.IsNil))
	c.Assert(result["merges"], qt.Not(qt.IsNil))
	c.Assert(result["extracts"], qt.Not(qt.IsNil))
}
```

- [ ] **Step 2: Implement the Scheme pipeline**

```scheme
;; ── recommend_boundaries ─────────────────────────────────
;;
;; Build a (package, function → struct-field) FCA context from
;; SSA data, compute the concept lattice, and ask fca-recommend
;; for the three Pareto frontiers.

(define (pipeline-recommend-boundaries target mode)
  (let* ((session (go-load session-or-target-probe target))
         (field-idx (go-ssa-field-index session))
         (ctx (field-index->context field-idx (or mode 'write-only)))
         (lattice (concept-lattice ctx))
         (ssa-funcs (go-ssa-build session))
         (rec (boundary-recommendations lattice ssa-funcs)))
    (pipeline-envelope 1
      (list (cons 'target target)
            (cons 'mode (or mode 'write-only))
            (cons 'concept-count (length lattice)))
      rec)))
```

The `session-or-target-probe` pseudo-name is placeholder — `go-load`
already accepts a pattern string. Use it directly.

- [ ] **Step 3: Register + handler + test + commit**

Pattern as Task 4.

---

## Task 6: `find_false_boundaries` — cross-struct concepts + lattice annotations

**Files:**

- Modify: `lib/wile/goast/pipelines.scm`
- Modify: `cmd/wile-goast/mcp_tools.go`
- Modify: `cmd/wile-goast/mcp_tools_integration_test.go`

- [ ] **Step 1: Write the test**

```go
func TestFindFalseBoundaries_Phase1Fixture(t *testing.T) {
	c := qt.New(t)
	mc := inProcessClient(t)

	env := callTool(t, mc, "find_false_boundaries", map[string]any{
		"target": "github.com/aalpar/wile-goast/cmd/wile-goast/testdata/phase1",
	})
	envelopeOK(t, env, 1.0)
	// Even an empty result must round-trip a valid envelope.
	c.Assert(env["result"], qt.Not(qt.IsNil))
}
```

The fixture's `Counter` and `Cache` share no fields, so no
cross-boundary concept is expected — the test just verifies the
envelope.

- [ ] **Step 2: Implement the Scheme pipeline**

```scheme
;; ── find_false_boundaries ────────────────────────────────
;;
;; Build a write-mode FCA context from struct fields, filter for
;; cross-boundary concepts, annotate with lattice relationships.

(define (pipeline-find-false-boundaries target opts)
  (let* ((session (go-load target))
         (field-idx (go-ssa-field-index session))
         (mode (or (assoc-default opts 'mode) 'write-only))
         (min-ext (or (assoc-default opts 'min-extent) 2))
         (min-int (or (assoc-default opts 'min-intent) 2))
         (min-typ (or (assoc-default opts 'min-types) 2))
         (ctx (field-index->context field-idx mode))
         (lattice (concept-lattice ctx))
         (cross (cross-boundary-concepts lattice
                  'min-extent min-ext
                  'min-intent min-int
                  'min-types min-typ))
         (annotated (annotated-boundary-report cross lattice)))
    (pipeline-envelope 1
      (list (cons 'target target)
            (cons 'mode mode)
            (cons 'lattice-size (length lattice))
            (cons 'cross-boundary-count (length cross)))
      annotated)))

(define (assoc-default alist key)
  (let ((e (assoc key alist))) (if e (cdr e) #f)))
```

- [ ] **Step 3: Register + handler + test + commit**

Pattern as Task 4. The handler passes `mode`, `min_extent`,
`min_intent`, `min_types` as optional parameters.

---

## Task 7: Full-surface sanity check — all tools listed, initialize round-trips

**Files:**

- Modify: `cmd/wile-goast/mcp_tools_integration_test.go`

- [ ] **Step 1: Write the test**

```go
func TestPhase1ToolsRegistered(t *testing.T) {
	c := qt.New(t)
	mc := inProcessClient(t)

	ctx := context.Background()
	res, err := mc.ListTools(ctx, mcp.ListToolsRequest{})
	c.Assert(err, qt.IsNil)

	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{
		"check_beliefs", "discover_beliefs",
		"recommend_split", "recommend_boundaries",
		"find_false_boundaries",
	} {
		c.Assert(names[want], qt.IsTrue,
			qt.Commentf("missing tool: %s", want))
	}
}
```

- [ ] **Step 2: Run, expect pass (no code changes needed if Tasks 2-6 all registered)**

- [ ] **Step 3: Commit**

```bash
git add cmd/wile-goast/mcp_tools_integration_test.go && \
  git commit -m "test(mcp): verify Phase 1 tools are registered"
```

---

## Task 8: Update mcp.go instructions string

**Files:**

- Modify: `cmd/wile-goast/mcp.go`

- [ ] **Step 1: Expand the `WithInstructions` block**

The existing instructions describe only the `eval` tool and the five
prompts. Add a section:

```
"## Pipeline tools\n\n"+
"Five pipeline tools return structured reports without LLM orchestration:\n"+
"- `check_beliefs` — run .scm beliefs against a Go package\n"+
"- `discover_beliefs` — run discovery beliefs, suppress committed ones, emit source\n"+
"- `recommend_split` — IDF-FCA + min-cut package split recommendation\n"+
"- `recommend_boundaries` — function-level split/merge/extract Pareto frontiers\n"+
"- `find_false_boundaries` — cross-struct concepts via FCA + lattice annotations\n\n"+
"Prefer pipeline tools for known structural queries; use `eval` for exploration.\n\n"+
```

- [ ] **Step 2: Commit**

Ask first, then:

```bash
git add cmd/wile-goast/mcp.go && \
  git commit -m "docs(mcp): describe pipeline tools in server instructions"
```

---

## Task 9: `make lint` + `make test` green gate

- [ ] **Step 1: Run lint**

```
cd /Users/aalpar/projects/wile-workspace/wile-goast && make lint
```

Expected: 0 issues.

- [ ] **Step 2: Run tests**

```
cd /Users/aalpar/projects/wile-workspace/wile-goast && make test
```

Expected: all packages PASS. The new integration tests add ~3-5 s
to the `cmd/wile-goast` package (dominated by package loads).

- [ ] **Step 3: Run `make ci`**

Expected: lint + build + test + coverage + `go mod verify` all PASS.

- [ ] **Step 4: No commit (validation only).**

---

## Task 10: Documentation — CHANGELOG + CLAUDE.md + plans/CLAUDE.md

**Files:**

- Modify: `CHANGELOG.md`
- Modify: `CLAUDE.md`
- Modify: `plans/CLAUDE.md`

- [ ] **Step 1: Prepend CHANGELOG entry**

```markdown
## Unreleased — MCP Pipeline Tools (Phase 1)

Five pipeline-shaped MCP tools expose already-implemented analyses as
first-class tool calls, complementing the existing `eval` tool. Each
returns a `(tool, version, provenance, result)` envelope.

- `check_beliefs` — run committed beliefs against a Go package.
- `discover_beliefs` — run discovery beliefs, suppress committed
  matches, emit survivors as Scheme source.
- `recommend_split` — IDF-weighted FCA + min-cut package split
  recommendation.
- `recommend_boundaries` — function-level split/merge/extract
  Pareto frontiers.
- `find_false_boundaries` — FCA cross-struct concepts with algebraic
  annotations.

Phases 2-4 of the tool surface (`filter_concepts`, `find_duplicates`,
`explain_function`, `restructure_block`, `trace_path`) will ship under
their own plans.
```

- [ ] **Step 2: Add `## MCP Pipeline Tools` section to `CLAUDE.md`**

Insert after the existing `## MCP Server` section with a tool table
mirroring the one in the design doc §84-97 but filtered to the five
Phase 1 tools and with their actual parameter names.

- [ ] **Step 3: Update `plans/CLAUDE.md`**

Find the row for `2026-04-19-mcp-tool-surface-design.md` in the Active
Plan Files table. Change status to:

```
Phase 1 shipped — see 2026-04-19-mcp-tool-surface-impl.md. Phases 2-4 pending.
```

Add a new row for this impl plan.

- [ ] **Step 4: Commit**

Ask first, then:

```bash
git add CHANGELOG.md CLAUDE.md plans/CLAUDE.md && \
  git commit -m "docs(mcp): document Phase 1 pipeline tools"
```

---

## Self-review checklist (plan author)

- [ ] Every Step has an exact file path.
- [ ] Every code Step shows actual code (no "implement the function"
      stubs) or says "mirror of Task N".
- [ ] Every test Step says how to run and what to expect.
- [ ] Commits are asked-for, not auto-taken.
- [ ] `make lint` + `make test` run at Task 9.
- [ ] Every design-doc §297-308 Phase 1 tool maps to a task:
      `check_beliefs`→T2, `discover_beliefs`→T3, `recommend_split`→T4,
      `recommend_boundaries`→T5, `find_false_boundaries`→T6.
- [ ] Every design-doc cross-cutting choice §98-122 is honored:
      coarse-grained (one tool per pipeline), structured output
      (JSON envelope via `NewToolResultJSON`), provenance included
      (envelope field), parameter composition (no LLM orchestration),
      `eval` as peer (untouched), session as handle (deferred
      indefinitely per locked DP #4 — server-side caching is the
      eventual perf response, not exposed session handles).

---

## Resolved ambiguities

| # | Ambiguity | Resolution |
|---|-----------|------------|
| 1 | `mustWriteFile` / `schemeStr` location | Duplicate in `cmd/wile-goast/mcp_tools_integration_test.go`. If both tests grow coupled, later move to `testutil/`. |
| 2 | Envelope version bump policy | Per-tool integer (`1`, `2`, ...); bumped only on breaking changes to that tool's `result` shape (renamed key, changed value type, removed key). Adding a new key is non-breaking. Per-tool — `find_false_boundaries` can reach v3 while `recommend_split` stays v1. |
| 3 | `committed_path` empty-string semantics | Empty string = no suppression. `#f` would be cleaner in Scheme but MCP params are stringly-typed; empty-string is the pragmatic bridge. |
| 4 | `recommend_boundaries` `mode` default | `'write-only` — matches the existing default inherited from `field-index->context` and is the mode used in all existing FCA examples. |
| 5 | Tool parameter naming convention | Snake-case at the JSON boundary (`target`, `beliefs_path`, `discovery_path`, `committed_path`, `idf_threshold`). Scheme-side uses kebab-case (`sites-expr`, `idf-threshold`); the marshaller normalises kebab→snake at every alist key. |
| 6 | Output format | JSON via `NewToolResultJSON` (locked DP #1). Populates both text content (JSON string) and `structuredContent`. See marshaller table in "Locked decisions" §1. |

---

## Open Scheme-side work depending on implementation probing

These items may require small extensions during Task 2/3 implementation:

- **`register-belief!`** — does the belief module export a
  registration primitive callable from user code? If not, the
  "load .scm in scope" branch of Task 2 Step 4 applies.
- **`emit-beliefs` return type** — Task 3 assumes it returns a
  string suitable for display; verify by reading
  `belief.scm:798-...` during Task 3 Step 3 implementation.

Probe these before writing the final Scheme in Task 2/3; the plan
structure remains the same either way — only the internal pipeline
implementation changes.

## Upstream (wile) work surfaced by this plan

Listing `.scm` files in a directory is a filesystem operation, not a
Go-static-analysis operation. It belongs in wile. Probing this one
case exposed a broader pattern: wile has `(scheme base)` R7RS strings
(`substring`, `string=?`, `string-length`, etc.) but **does not have
SRFI-13** (the extended string library with `string-prefix?`,
`string-suffix?`, `string-contains`, `string-join`, `string-split`,
etc.).

wile-goast has compensated by hand-rolling these primitives in four
places:

| Workaround | Location | SRFI-13 replacement |
|------------|----------|---------------------|
| `string-contains` | `lib/wile/goast/utils.scm:95` | `string-contains` |
| `string-join` | `lib/wile/goast/utils.scm:147` | `string-join` |
| `string-suffix?` | `lib/wile/goast/fca-recommend.scm:24` | `string-suffix?` |
| `list-scheme-files-in-dir` (inline substring check) | `lib/wile/goast/belief.scm:862` (shipped 2026-04-23, commit `846a5dd`) | `string-suffix?` + `directory-files` + `filter` |

Three independent workarounds for three SRFI-13 procedures, plus a
fourth inline instance. That is not a coincidence — it is a signal
that SRFI-13 (or the commonly-needed subset) needs to land in wile.

The right factoring, in order:

1. Add SRFI-13 (or at minimum `string-prefix?`, `string-suffix?`,
   `string-contains`, `string-join`, `string-split`) to wile's
   stdlib. Track in `../../wile/plans/WORKSPACE-ROADMAP.md` per the
   workspace-coordination rule.
2. In wile-goast, consume the new wile primitives: retire the three
   hand-rolls in `utils.scm` / `fca-recommend.scm` and remove
   `list-scheme-files-in-dir` from `belief.scm`. `(wile goast utils)`
   then stops hosting generic string plumbing (a category-mistake
   similar to the `list-scheme-files-in-dir` placement).
3. For new code in Tasks 2-3 of this plan: if SRFI-13 has shipped,
   use it directly. If not, inline a `substring`-based workaround
   with `;; TODO: replace with (import (srfi 13)) once wile ships
   it`, and migrate later.

Steps 1-2 are **out of scope** for this plan — they live in wile (or
in a follow-up wile-goast cleanup commit). Flagging them here so the
upstream dependency is visible and the sequencing is explicit. This
plan's Tasks 2-3 do not block on them.

---

## Phase 1 follow-ups

Work surfaced by the 2026-05-29 design pass that is intentionally
**not in Phase 1 scope** but should ship soon after Phase 1 lands.
Each item resolves a deferred decision; none block Phase 1.

### 1. Example discovery beliefs (DP #5 follow-up)

Per the locked DP #5 resolution, `discover_beliefs` requires
`discovery_path`. To make first-run usable without committing to a
curated default set, ship 2-3 example discovery belief files as
documentation templates.

**Location:** `examples/discovery-beliefs/` (project root).

**Suggested templates:**

- `lock-unlock-pairing.scm` — `(paired-with "Lock" "Unlock")` over
  functions that call `Lock`. The canonical Engler-style belief.
- `defer-close-after-open.scm` — `(paired-with "Open" "Close")` over
  functions that call `Open`. Different shape: pairs across the
  return value's method set.
- `error-checked-before-use.scm` — `(checked-before-use err)` over
  functions returning `error` values to subsequent callers.

**Constraints:**

- Each file is a single self-contained `(define-belief ...)` form.
- No `register-belief!` runtime side effects from loading — loading
  the file MUST define the belief and nothing else.
- Documented in `examples/discovery-beliefs/README.md`: what each
  belief looks for, what false-positive shapes to expect, how to
  adapt the threshold for your codebase.
- **Distinct from defaults.** These are not auto-loaded by
  `discover_beliefs`. They are copy-paste-ready entry points the
  user passes explicitly via `--discovery_path`.

Effort: ~3 hours (templates + README). Can be a separate commit.

### 2. Internal package-load caching (DP #4 follow-up)

Per the locked DP #4 resolution, Phase 1 tools load packages fresh
per call. If aggregate performance becomes load-bearing (multiple
tools hit the same target in sequence within one session), the
correct response is **server-side caching keyed on
`(target_pattern, fs-state)`**, not client-visible session handles.

**Where it lives:** `cmd/wile-goast/cache.go` (new). A `sync.Map`
of `target → loaded GoSession` keyed by the target string. The
`mcpServer` struct gains a `cache *targetCache` field; tool handlers
call `ms.cache.GetOrLoad(target)` instead of `(go-load target)`
directly.

**Eviction strategy:** invalidate cache entry when `fs.WalkDir` over
the target's source tree shows any mtime newer than the cached
load timestamp. Cheap enough to check on every request; correct.

**Provenance addition (additive, non-breaking):** envelope provenance
gains a `cache_hit: bool` field. Consumers tolerate missing fields,
so existing v1 tools don't bump.

**When to do it:** profile-driven. Don't speculate; measure when a
real workflow becomes painful. Likely never needed for single-tool
calls; relevant only for multi-call orchestrations the LLM strings
together.

### 3. Phase 2 entry point — `filter_concepts`

Phase 2 of the design doc (`2026-04-19-llm-concept-filter-design.md`)
introduces `filter_concepts` as the canonical post-filter. Its design
is independent; its impl plan is its own document. Phase 1 does not
depend on it.

The relevant Phase 1 hook: any tool producing a concept-shaped result
(`find_false_boundaries`, future `find_duplicates`) should make sure
its `result` shape is consumable by `filter_concepts`' future input
contract. Specifically, each concept-result object should carry an
opaque `id` field that `filter_concepts` can reference back in its
verdict output. Adding `id` to the locked-v1 result shape now is
additive (non-breaking) and saves a version bump later.
