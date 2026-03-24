# MCP Server Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `--mcp` flag to wile-goast that starts a stdio MCP server with one eval tool and three analysis prompts.

**Architecture:** Single `doMCP(ctx)` entry point in new `mcp.go` file. Lazy-init Wile engine on first eval call. Three prompts loaded from embedded `.md` files replace Claude Code skill files.

**Tech Stack:** `mark3labs/mcp-go` for MCP protocol, existing Wile engine + goast extensions.

**Design doc:** `docs/plans/2026-03-23-mcp-server-design.md`

---

### Task 1: Add mcp-go dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Add the dependency**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go get github.com/mark3labs/mcp-go@latest`

**Step 2: Tidy**

Run: `go mod tidy`

**Step 3: Verify build**

Run: `make build`
Expected: SUCCESS (dependency added but not yet used — that's fine, go mod keeps it)

**Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add mark3labs/mcp-go for MCP server support"
```

---

### Task 2: Create prompt content files

The three prompts replace the Claude Code skill files. Each `.md` file contains
the instructional text returned as an MCP prompt message. The `goast-analyze`
prompt merges content from both the `/goast-analyze` skill and the
`goast-analyzer` agent (primitive reference, AST conventions, examples).

**Files:**
- Create: `cmd/wile-goast/prompts/goast-analyze.md`
- Create: `cmd/wile-goast/prompts/goast-beliefs.md`
- Create: `cmd/wile-goast/prompts/goast-refactor.md`

**Step 1: Create `cmd/wile-goast/prompts/goast-analyze.md`**

This is the largest prompt — it merges the analysis skill's workflow with the
analyzer agent's complete primitive reference. Arguments `{{question}}` and
`{{package}}` are interpolated at runtime.

```markdown
# Go Static Analysis

Analyze Go code structure using wile-goast's eval tool. Determine the right
analysis layer and compose Scheme expressions to answer structural questions.

## Your Question

{{question}}

## Target Package

{{package}}

## Instructions

### Step 1: Determine the analysis layer

Based on the question, select the appropriate layer:

| Question type | Layer | Import |
|--------------|-------|--------|
| Function structure, AST shape, parsing | AST | `(wile goast)` |
| Data flow, field stores, value tracking | SSA | `(wile goast ssa)` |
| Statement ordering, path enumeration | CFG | `(wile goast cfg)` |
| Who calls what, reachability | Call Graph | `(wile goast callgraph)` |
| Known anti-patterns, standard checks | Lint | `(wile goast lint)` |
| Statistical consistency patterns | Belief DSL | `(wile goast belief)` |

If the question spans multiple layers, compose them in a single expression.

### Step 2: Compose and run the analysis

Use the `eval` tool with a Scheme expression. All analysis runs inside the
wile-goast process — do not read Go source files to answer structural questions.

### Step 3: Interpret and report results

- Translate s-expression output into human-readable findings
- Reference specific file:line locations when position data is available
- Highlight actionable items vs. informational findings
- Always show the Scheme expression you ran (for reproducibility)

## Primitive Reference

### AST — `(import (wile goast))`
- `(go-parse-file path . options)` — parse .go file to s-expression AST
- `(go-parse-string source . options)` — parse Go source string
- `(go-parse-expr source)` — parse single expression
- `(go-format ast)` — convert s-expression AST back to Go source
- `(go-node-type ast)` — return tag symbol of an AST node
- `(go-typecheck-package pattern . options)` — load with type annotations

Options: `'positions` (include file:line:col), `'comments` (include doc/comments)

### SSA — `(import (wile goast ssa))`
- `(go-ssa-build pattern)` — build SSA for a package
- `(go-ssa-field-index ssa-pkg)` — field access index for all functions

### CFG — `(import (wile goast cfg))`
- `(go-cfg pattern func-name)` — build CFG for a named function
- `(go-cfg-dominators cfg)` — build dominator tree
- `(go-cfg-dominates? dom-tree block-a block-b)` — test dominance
- `(go-cfg-paths cfg from to)` — enumerate simple paths between blocks

### Call Graph — `(import (wile goast callgraph))`
- `(go-callgraph pattern . algorithm)` — build call graph (static, cha, rta)
- `(go-callgraph-callers cg func-name)` — incoming edges
- `(go-callgraph-callees cg func-name)` — outgoing edges
- `(go-callgraph-reachable cg func-name)` — transitive reachability

### Lint — `(import (wile goast lint))`
- `(go-analyze pattern analyzer-names...)` — run analysis passes
- `(go-analyze-list)` — list available analyzer names

### Belief DSL — `(import (wile goast belief))`

```scheme
(define-belief "name"
  (sites <selector>)
  (expect <checker>)
  (threshold <ratio> <min-count>))
(run-beliefs "package/pattern/...")
```

Site selectors: `functions-matching`, `callers-of`, `methods-of`, `sites-from`
Predicates: `has-params`, `has-receiver`, `name-matches`, `contains-call`,
  `stores-to-fields`, `all-of`, `any-of`, `none-of`
Property checkers: `contains-call`, `paired-with`, `ordered`, `co-mutated`,
  `checked-before-use`, `custom`

### Utilities — `(import (wile goast utils))`

```scheme
(nf node 'key)           ; field access -> value or #f
(tag? node 'func-decl)   ; tag predicate -> #t or #f
(walk val visitor)        ; depth-first walk, collects non-#f results
(filter-map f lst)        ; map keeping non-#f results
(flat-map f lst)          ; map then concatenate
```

## AST Representation

Go AST nodes are tagged alists: `(tag (key . val) ...)`.
Access fields: `(assoc 'key (cdr node))` or use `nf` from utils.

Key field type rules:
- `name`, `sel`, `label`, `value` (on lit) -> string
- `type` (on composite-lit, func-decl, etc.) -> node or `#f`
- `decls`, `list`, `elts`, `args` -> list of nodes
- `inferred-type`, `obj-pkg` -> string (only with `go-typecheck-package`)

## Common Patterns

Walk all nodes in a package:
```scheme
(import (wile goast utils))
(let ((pkgs (go-typecheck-package "my/package")))
  (for-each
    (lambda (pkg)
      (for-each
        (lambda (file)
          (walk file
            (lambda (node)
              (when (eq? (car node) 'composite-lit)
                (display (nf (nf node 'type) 'name))
                (newline))
              #f)))
        (cdr (assoc 'files (cdr pkg)))))
    pkgs))
```

Find callers of a function:
```scheme
(let ((cg (go-callgraph "my/package" 'cha)))
  (go-callgraph-callers cg "FuncName"))
```

Check dominance between statements:
```scheme
(let* ((cfg (go-cfg "my/package" "FuncName"))
       (dom (go-cfg-dominators cfg)))
  (go-cfg-dominates? dom 0 3))
```
```

**Step 2: Create `cmd/wile-goast/prompts/goast-beliefs.md`**

```markdown
# Belief-Based Consistency Checking

Define and run consistency beliefs against Go packages using wile-goast's belief
DSL. Beliefs detect deviations from statistical patterns (Engler et al., "Bugs
as Deviant Behavior").

## Request

{{action}}

## Target Package

{{package}}

## Instructions

### Running existing beliefs

Look for `.goast-beliefs/` directory in the project root. If belief files exist,
run them using the eval tool:

```scheme
(begin
  (import (wile goast belief))
  (for-each (lambda (f) (load f))
    (list ".goast-beliefs/file1.scm" ".goast-beliefs/file2.scm"))
  (run-beliefs "./..."))
```

Replace the file list with actual `.goast-beliefs/*.scm` files.

### Defining a new belief

Guide the user through:
1. **Name**: descriptive identifier for the pattern
2. **Sites**: where to look — which functions match?
3. **Expect**: what property to check at each site?
4. **Threshold**: minimum adherence ratio and site count

Available site selectors:
- `(functions-matching pred ...)` — functions matching structural predicates
- `(callers-of "func")` — all callers of a named function
- `(methods-of "Type")` — all methods on a receiver type

Available predicates (for `functions-matching`):
- `(has-params "type" ...)` — signature contains param types
- `(has-receiver "type")` — method receiver matches
- `(name-matches "pattern")` — function name substring
- `(contains-call "func" ...)` — body calls any of these
- `(stores-to-fields "Struct" "field" ...)` — SSA stores to fields
- `(all-of pred ...)` / `(any-of pred ...)` / `(none-of pred ...)`

Available property checkers:
- `(contains-call "func" ...)` — call present/absent
- `(paired-with "A" "B")` — A paired with B (e.g., Lock/Unlock)
- `(ordered "A" "B")` — dominance ordering between calls
- `(co-mutated "field" ...)` — fields always stored together
- `(checked-before-use "val")` — value guarded before use
- `(custom (lambda (site ctx) ...))` — user-defined check

Write the belief to `.goast-beliefs/<name>.scm`. The file should import the
belief DSL but NOT call `run-beliefs` — the runner supplies the target.

### Interpreting results

Belief output reports:
- **Adherence**: what percentage of sites follow the pattern
- **Deviations**: which sites break the pattern (with locations)
- **Classification**: the majority behavior vs. minority deviations

Deviations are potential bugs — they break a pattern that most code follows.
But not all deviations are bugs; some are intentional exceptions. Present both
interpretations.

## Rules

- Belief files should NOT call `run-beliefs` — the runner supplies the target
- Each `.goast-beliefs/*.scm` file should `(import (wile goast belief))`
- Threshold: use 0.90 for strong patterns, 0.66 for weaker ones; minimum 3 sites
```

**Step 3: Create `cmd/wile-goast/prompts/goast-refactor.md`**

```markdown
# Analysis-Backed Refactoring

Use wile-goast to find unification candidates (duplicate/near-duplicate functions)
and verify refactoring correctness via structural analysis.

## Goal

{{goal}}

## Target Package

{{package}}

## Phase 1: Detect unification candidates

Use the eval tool to run the built-in unification detection:

```scheme
;; For package-wide scan:
(begin
  (import (wile goast) (wile goast utils))
  ;; Load and run unify-detect-pkg logic
  )
```

Or for specific function pairs:
1. Parse both functions to s-expression ASTs
2. Run recursive AST diff
3. Score similarity (shared nodes, diff categories, weighted cost)
4. Identify parameterizable differences (type params, value params)

**Interpreting scores:**
- `similarity > 0.80` with `weighted-cost < 10` — strong unification candidate
- `param-count <= 3` — feasible to parameterize
- `structural` diffs — different control flow, unlikely to unify
- `identifier` diffs — free renames, don't count against unification
- `type-name` diffs — become type parameters or interface constraints
- `literal-value` diffs — become value parameters

## Phase 2: Plan the unification

If unification is feasible:
1. Identify the unified function signature (original + type/value params)
2. Use call graph to find ALL call sites of both functions:
   ```scheme
   (let ((cg (go-callgraph "package" 'cha)))
     (append (go-callgraph-callers cg "FuncA")
             (go-callgraph-callers cg "FuncB")))
   ```
3. Determine if unification reduces total complexity (fewer lines, fewer
   concepts) or merely compresses code at the cost of indirection

## Phase 3: Verify after refactoring

After the refactoring is applied:
1. Run call graph analysis to confirm all call sites reference the unified
   function
2. Run beliefs (if `.goast-beliefs/` exists) to confirm no consistency patterns
   were broken
3. Run lint passes on changed packages:
   ```scheme
   (go-analyze "package/..." "nilness" "unusedresult" "shadow")
   ```

## Rules

- Always show the diff scores and your interpretation before suggesting a merge
- Don't suggest unification when the weighted cost is high (structural diffs)
- Don't suggest unification when it would add more parameters than it removes
  lines — that's compression, not simplification
- Verify by substitution: can every call site use the unified function?
```

**Step 4: Verify build**

Run: `make build`
Expected: SUCCESS (new files are not yet referenced from Go code)

**Step 5: Commit**

```bash
git add cmd/wile-goast/prompts/
git commit -m "feat: add MCP prompt content files for analysis, beliefs, refactoring"
```

---

### Task 3: Add prompt embed directive

**Files:**
- Modify: `cmd/wile-goast/embed.go:17-23`

**Step 1: Add embed for prompts**

Add to `embed.go` after the existing embed vars:

```go
//go:embed prompts
var embeddedPrompts embed.FS
```

**Step 2: Verify build**

Run: `make build`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add cmd/wile-goast/embed.go
git commit -m "feat: embed MCP prompt content files"
```

---

### Task 4: Create mcp.go with eval tool

This is the core file. It contains:
- `doMCP(ctx)` — entry point from main
- `evalHandler` — MCP tool handler wrapping `engine.EvalMultiple`
- Lazy engine initialization via `sync.Once`

**Files:**
- Create: `cmd/wile-goast/mcp.go`

**Step 1: Write mcp.go**

```go
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

// mcpServer holds the MCP server state including the lazily-initialized
// Wile engine.
type mcpServer struct {
	engine     *wile.Engine
	engineOnce sync.Once
	engineErr  error
}

func doMCP(ctx context.Context) error {
	ms := &mcpServer{}

	s := server.NewMCPServer(
		"wile-goast",
		version,
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(true),
	)

	// Register the eval tool
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

	// Register prompts
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
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		text := template
		for k, v := range req.Params.Arguments {
			text = strings.ReplaceAll(text, "{{"+k+"}}", v)
		}
		// Clear any unreplaced placeholders
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
```

**Note on `version`:** The `version` variable needs to exist. Check if there's
already a version constant in main.go or embed.go. If not, add to mcp.go:

```go
var version = "0.3.4" // TODO: read from VERSION file or build flag
```

If `VERSION` is already read somewhere, use that. If not, read it from the
embedded FS or hardcode for now.

**Step 2: Check for existing version handling**

Run: `grep -r "version\|VERSION" cmd/wile-goast/ --include="*.go"`

If no version variable exists, add this to mcp.go before `doMCP`:

```go
//go:embed VERSION
var version string
```

But `VERSION` is at the repo root, not in `cmd/wile-goast/`. Embed paths are
relative to the Go file. Options:
- Hardcode `"0.3.4"` (simplest, update on release)
- Read `VERSION` at build time via `-ldflags`

Hardcode for now. The MCP spec version field is informational, not critical.

**Step 3: Verify build**

Run: `make build`
Expected: SUCCESS (mcp.go compiles but doMCP is not yet called from main)

**Step 4: Commit**

```bash
git add cmd/wile-goast/mcp.go
git commit -m "feat: MCP server with eval tool and analysis prompts"
```

---

### Task 5: Wire --mcp flag in main.go

**Files:**
- Modify: `cmd/wile-goast/main.go:49-54` (Options struct)
- Modify: `cmd/wile-goast/main.go:72-84` (dispatch block)
- Modify: `cmd/wile-goast/main.go:25` (doc comment)

**Step 1: Add MCP field to Options**

In the `Options` struct (line 49), add:

```go
MCP         bool     `long:"mcp" description:"Start as MCP server on stdio"`
```

**Step 2: Add mutual exclusivity check and dispatch**

After `ctx := context.Background()` (line 72), before the `--list-scripts`
check, add:

```go
// --mcp: start MCP server
if opts.MCP {
	if len(opts.Eval) > 0 || len(opts.File) > 0 || opts.ListScripts || opts.Run != "" {
		fmt.Fprintln(os.Stderr, "Error: --mcp cannot be combined with -e, -f, --run, or --list-scripts")
		os.Exit(1)
	}
	if err := doMCP(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
	return
}
```

**Step 3: Update doc comment**

Add to the package doc comment (around line 23):

```go
//	wile-goast --mcp                    start MCP server on stdio
```

**Step 4: Verify build**

Run: `make build`
Expected: SUCCESS

**Step 5: Commit**

```bash
git add cmd/wile-goast/main.go
git commit -m "feat: add --mcp flag for MCP server mode"
```

---

### Task 6: End-to-end smoke test

Test the MCP server with raw JSON-RPC messages. The MCP protocol uses
newline-delimited JSON over stdio.

**Files:** None (manual testing)

**Step 1: Test initialize + tool list**

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | dist/$(go env GOOS)/$(go env GOARCH)/wile-goast --mcp 2>/dev/null
```

Expected: JSON response containing `"name":"eval"` in the tools list.

**Step 2: Test eval tool**

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"eval","arguments":{"code":"(+ 1 2)"}}}' | dist/$(go env GOOS)/$(go env GOARCH)/wile-goast --mcp 2>/dev/null
```

Expected: JSON response containing `"text":"3"` in the result content.

**Step 3: Test eval tool error handling**

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"eval","arguments":{"code":"(undefined-function)"}}}' | dist/$(go env GOOS)/$(go env GOARCH)/wile-goast --mcp 2>/dev/null
```

Expected: JSON response with `"isError":true` and an error message.

**Step 4: Test prompt list**

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"prompts/list","params":{}}' | dist/$(go env GOOS)/$(go env GOARCH)/wile-goast --mcp 2>/dev/null
```

Expected: JSON response listing `goast-analyze`, `goast-beliefs`, `goast-refactor`.

**Step 5: Test prompt retrieval**

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"prompts/get","params":{"name":"goast-analyze","arguments":{"question":"what calls buildEngine","package":"./..."}}}' | dist/$(go env GOOS)/$(go env GOARCH)/wile-goast --mcp 2>/dev/null
```

Expected: JSON response with prompt messages containing the interpolated
question and package in the analysis instructions.

**Step 6: Test mutual exclusivity**

```bash
dist/$(go env GOOS)/$(go env GOARCH)/wile-goast --mcp -e '(+ 1 2)' 2>&1
```

Expected: stderr output containing "cannot be combined" and exit code 1.

---

### Task 7: Run CI checks

**Step 1: Full CI**

Run: `make ci`
Expected: SUCCESS (lint, build, test, covercheck, verify-mod all pass)

**Step 2: Fix any issues**

If lint or vet complains about unused imports, missing error checks, etc.,
fix them in `mcp.go` and re-run.

**Step 3: Final commit if any fixes**

```bash
git add -u
git commit -m "fix: address CI findings in MCP server code"
```

---

### Task 8: Version bump

**Files:**
- Modify: `VERSION`

**Step 1: Bump version**

Update `VERSION` from `v0.3.4` to `v0.4.0` (new feature: MCP server mode).

**Step 2: Update version constant in mcp.go**

If hardcoded, update the version string to match.

**Step 3: Commit**

```bash
git add VERSION cmd/wile-goast/mcp.go
git commit -m "release: v0.4.0 — MCP server mode"
```
