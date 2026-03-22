# Claude Code Integration — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make wile-goast a first-class tool in Claude Code's workflow for any Go project.

**Architecture:** CLI + CLAUDE.md + skills + agent + hooks. wile-goast stays a
self-contained binary (embedded libs via `go:embed`). Claude discovers it through
global CLAUDE.md, invokes it via Bash, and uses skills for guided workflows.
Hooks enforce beliefs at commit/PR boundaries.

**Tech Stack:** Go (`embed`, `io/fs`), Claude Code (settings.json hooks, commands,
agents), Scheme (belief DSL, analysis scripts)

**Design doc:** `plans/CLAUDE-CODE-INTEGRATION.md`

---

## Task 1: Embed library files in the binary

The `.sld` and `.scm` files in `lib/` must ship inside the binary so
`(import (wile goast belief))` works after `go install` from any directory.

**Files:**
- Create: `cmd/wile-goast/embed.go`
- Modify: `cmd/wile-goast/main.go`

**Step 1: Create the embed file**

Create `cmd/wile-goast/embed.go`:

```go
package main

import "embed"

// embeddedLib contains the Scheme library files (.sld, .scm) from lib/.
// These are passed to the Wile engine via WithSourceFS so that
// (import (wile goast belief)) works without files on disk.
//
//go:embed lib
var embeddedLib embed.FS
```

**Step 2: Copy lib/ into cmd/wile-goast/**

The `go:embed` directive is relative to the package directory. The `lib/` tree
must be accessible from `cmd/wile-goast/`. Two options:

- **Option A**: Symlink `cmd/wile-goast/lib -> ../../lib` (works for local dev
  and `go install` from source checkout, but `go install` from module proxy
  doesn't follow symlinks)
- **Option B**: Move the canonical `lib/` directory into `cmd/wile-goast/lib/`
  and update any references

**Recommended: Option B** — move `lib/` to `cmd/wile-goast/lib/`. This ensures
`go install github.com/aalpar/wile-goast/cmd/wile-goast@latest` embeds the files.
Update the `Usage:` comments in example scripts and any test that references
`lib/` directly.

Run: `git mv lib cmd/wile-goast/lib`

Verify the embed compiles:

```bash
cd cmd/wile-goast && go build -o /dev/null .
```

Expected: builds without errors.

**Step 3: Wire WithSourceFS into engine construction**

Modify `cmd/wile-goast/main.go` — replace the current engine construction:

```go
engine, err := wile.NewEngine(ctx,
    wile.WithSafeExtensions(),
    wile.WithSourceFS(embeddedLib),
    wile.WithLibraryPaths("lib"),
    wile.WithExtension(goast.Extension),
    wile.WithExtension(goastssa.Extension),
    wile.WithExtension(goastcg.Extension),
    wile.WithExtension(goastcfg.Extension),
    wile.WithExtension(goastlint.Extension),
)
```

Key changes:
- Add `wile.WithSourceFS(embeddedLib)` — tells the engine to resolve files from
  the embedded FS
- Change `wile.WithLibraryPaths()` to `wile.WithLibraryPaths("lib")` — the
  search path is now relative to the embedded FS root

**Step 4: Verify the belief DSL loads from the embedded binary**

```bash
make build
./dist/darwin/arm64/wile-goast '(begin (import (wile goast belief)) (display "belief loaded") (newline))'
```

Expected output: `belief loaded`

Run from a directory that does NOT contain a `lib/` subdirectory to confirm it
uses the embedded files:

```bash
cd /tmp && /path/to/wile-goast '(begin (import (wile goast belief)) (display "belief loaded") (newline))'
```

Expected: `belief loaded` (not a file-not-found error).

**Step 5: Commit**

```bash
git add cmd/wile-goast/embed.go cmd/wile-goast/lib/ cmd/wile-goast/main.go
git commit -m "feat(embed): embed Scheme library files in binary

WithSourceFS + WithLibraryPaths wire the embedded lib/ into the engine
so (import (wile goast belief)) works after go install from any directory."
```

---

## Task 2: Embed example scripts + `--list-scripts` / `--run` flags

**Files:**
- Modify: `cmd/wile-goast/embed.go`
- Modify: `cmd/wile-goast/main.go`

**Step 1: Add script embedding**

Move or copy `examples/goast-query/` into `cmd/wile-goast/scripts/` and add an
embed directive to `cmd/wile-goast/embed.go`:

```go
// embeddedScripts contains the built-in analysis scripts.
//
//go:embed scripts
var embeddedScripts embed.FS
```

Run: `cp -r examples/goast-query cmd/wile-goast/scripts`

Keep the original `examples/` directory as-is — it serves as documentation.

**Step 2: Implement `--list-scripts`**

In `main.go`, before the engine construction, parse the arguments for flags.
The binary currently treats all args as a Scheme expression. Add minimal flag
handling:

```go
func main() {
    ctx := context.Background()

    if len(os.Args) > 1 {
        switch os.Args[1] {
        case "--list-scripts":
            listScripts()
            return
        case "--run":
            if len(os.Args) < 3 {
                fmt.Fprintln(os.Stderr, "Usage: wile-goast --run <script-name>")
                os.Exit(1)
            }
            runScript(ctx, os.Args[2])
            return
        }
    }

    // ... existing engine construction and eval logic ...
}
```

**Step 3: Implement `listScripts()`**

```go
func listScripts() {
    entries, err := fs.ReadDir(embeddedScripts, "scripts")
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error reading scripts: %v\n", err)
        os.Exit(1)
    }
    fmt.Println("Available scripts:")
    for _, e := range entries {
        if !e.IsDir() && strings.HasSuffix(e.Name(), ".scm") {
            name := strings.TrimSuffix(e.Name(), ".scm")
            fmt.Printf("  %s\n", name)
        }
    }
}
```

**Step 4: Implement `runScript()`**

```go
func runScript(ctx context.Context, name string) {
    path := "scripts/" + name + ".scm"
    data, err := fs.ReadFile(embeddedScripts, path)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Script %q not found. Use --list-scripts to see available scripts.\n", name)
        os.Exit(1)
    }

    engine := buildEngine(ctx) // extract engine construction to a helper
    defer func() { _ = engine.Close() }()

    val, err := engine.Eval(ctx, string(data))
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
    if val != nil {
        fmt.Println(val)
    }
}
```

Extract engine construction into `buildEngine(ctx)` to avoid duplication between
`runScript` and the existing eval path.

**Step 5: Test the flags**

```bash
make build
./dist/darwin/arm64/wile-goast --list-scripts
```

Expected:
```
Available scripts:
  goast-query
  state-trace-detect
  state-trace-full
  unify-detect
  unify-detect-pkg
  consistency-comutation
  dead-field-detect
  belief-example
  belief-comutation
```

```bash
./dist/darwin/arm64/wile-goast --run belief-example
```

Expected: belief output (may fail if not run from the wile-goast directory, since
the belief targets `github.com/aalpar/wile-goast/goast` — that's expected; the
point is verifying the script loads and the belief DSL imports successfully).

**Step 6: Commit**

```bash
git add cmd/wile-goast/
git commit -m "feat(cli): --list-scripts and --run for embedded analysis scripts

Embeds examples/goast-query/*.scm as built-in scripts accessible from
any directory. Scripts can reference the belief DSL since both are embedded."
```

---

## Task 3: Global CLAUDE.md section

Add a wile-goast section to `~/.claude/CLAUDE.md` that teaches Claude when and
how to use the tool.

**Files:**
- Modify: `~/.claude/CLAUDE.md`

**Step 1: Add the section**

Append after the `## Tips` section:

```markdown
## Go Static Analysis — wile-goast

When working on Go code, use `wile-goast` for cross-function and package-wide
structural analysis instead of reading code and guessing at patterns. For reading
a single short function, direct file reading is fine.

### When to use it

- Finding similar/duplicate functions — AST diff
- Checking consistency patterns — belief DSL
- Understanding call relationships — call graph queries
- Analyzing control flow — CFG paths, dominance, ordering
- Running lint passes — go/analysis framework
- Examining SSA form — data flow, field stores

### Invocation

    wile-goast '<scheme-expression>'
    wile-goast --run <script-name>
    wile-goast --list-scripts

### The five layers

| Layer | Import | Key primitives |
|-------|--------|---------------|
| AST | `(wile goast)` | `go-parse-file`, `go-parse-string`, `go-format`, `go-typecheck-package` |
| SSA | `(wile goast ssa)` | `go-ssa-build` |
| CFG | `(wile goast cfg)` | `go-cfg`, `go-cfg-dominators`, `go-cfg-paths` |
| Call Graph | `(wile goast callgraph)` | `go-callgraph`, `go-callgraph-callers`, `go-callgraph-callees` |
| Lint | `(wile goast lint)` | `go-analyze`, `go-analyze-list` |
| Belief DSL | `(wile goast belief)` | `define-belief`, `run-beliefs` |

### Core heuristic

Don't read Go source to determine structure — query it.
- "Are these functions similar?" -> AST diff, not eyeballing
- "Does every Lock have an Unlock?" -> belief DSL, not grep
- "What calls this function?" -> call graph, not searching for the name
- "Is this value checked before use?" -> SSA + CFG, not tracing by hand

Use /goast-analyze, /goast-beliefs, or /goast-refactor for guided workflows.
```

**Step 2: Verify**

Start a new Claude Code session in a Go project and confirm the section appears
in context.

---

## Task 4: `goast-analyzer` agent definition

A specialized subagent that knows the full primitive set and can compose
multi-layer analyses.

**Files:**
- Create: `~/.claude/agents/goast-analyzer.md`

**Step 1: Write the agent definition**

Create `~/.claude/agents/goast-analyzer.md`:

```markdown
---
name: goast-analyzer
description: Deep Go static analysis using wile-goast. Use when analyzing Go code structure, finding duplicates, checking consistency patterns, tracing call graphs, or verifying refactoring correctness across functions or packages.
tools: Bash, Read, Glob, Grep
model: inherit
---

You are a Go static analysis specialist. You use `wile-goast`, a CLI tool that
exposes Go's compiler infrastructure (go/ast, go/types, go/ssa, go/callgraph,
go/cfg, go/analysis) as Scheme primitives.

## Invocation

```
wile-goast '<scheme-expression>'
```

All analysis runs inside wile-goast's process. Do not read Go source files
into your context to analyze structure — query them through wile-goast instead.

## Primitives

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

## AST Representation

Go AST nodes are tagged alists: `(tag (key . val) ...)`.
Access fields: `(assoc 'key (cdr node))` or use `nf` from utils:
`(nf node 'key)` returns the value or `#f`.

## Common Patterns

Parse and extract functions:
```scheme
(let* ((file (go-parse-file "path/to/file.go"))
       (decls (cdr (assoc 'decls (cdr file)))))
  (filter (lambda (d) (eq? (car d) 'func-decl)) decls))
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

## Layer Selection

- Structure/shape questions -> AST
- Data flow / field stores / value tracking -> SSA
- Ordering / reachability within a function -> CFG
- Cross-function call relationships -> Call Graph
- Known anti-patterns -> Lint
- Statistical consistency patterns -> Belief DSL

## Reporting

When reporting results, include:
- The Scheme expression you ran (for reproducibility)
- The structured output
- Your interpretation of the results
- Specific file:line references when positions are available
```

**Step 2: Verify the agent is discoverable**

Start a new Claude Code session and check the agent appears in the Agent tool's
available types.

---

## Task 5: `/goast-analyze` skill

General-purpose entry point for structural queries about Go code.

**Files:**
- Create: `~/.claude/commands/goast-analyze.md`

**Step 1: Write the skill**

```markdown
# Go Static Analysis

Analyze Go code structure using wile-goast. This skill determines the right
analysis layer and invokes wile-goast to answer structural questions.

## Arguments

- `$ARGUMENTS` - The question or analysis request (e.g., "what calls FuncName",
  "find functions similar to X", "analyze the CFG of method Y")

## Instructions

### Step 1: Determine the analysis layer

Based on the user's question, select the appropriate layer:

| Question type | Layer | Import |
|--------------|-------|--------|
| Function structure, AST shape, parsing | AST | `(wile goast)` |
| Data flow, field stores, value tracking | SSA | `(wile goast ssa)` |
| Statement ordering, path enumeration | CFG | `(wile goast cfg)` |
| Who calls what, reachability | Call Graph | `(wile goast callgraph)` |
| Known anti-patterns, standard checks | Lint | `(wile goast lint)` |
| Statistical consistency patterns | Belief DSL | `(wile goast belief)` |

If the question spans multiple layers, compose them.

### Step 2: Identify target packages/files

Determine the Go package pattern or file path to analyze. If not specified by
the user, infer from the current working directory or ask.

### Step 3: Compose and run the analysis

For simple single-layer queries, generate the Scheme expression and run it
directly via Bash:

```bash
wile-goast '<scheme-expression>'
```

For complex multi-layer queries, spawn the `goast-analyzer` agent with a clear
description of what to analyze.

### Step 4: Interpret and report results

- Translate s-expression output into human-readable findings
- Reference specific file:line locations when position data is available
- Highlight actionable items vs. informational findings

## Rules

- Do NOT read Go source files to answer structural questions — query wile-goast
- Always show the Scheme expression you ran (for reproducibility)
- If wile-goast is not installed, tell the user:
  `go install github.com/aalpar/wile-goast/cmd/wile-goast@latest`
```

---

## Task 6: `/goast-beliefs` skill

For defining, running, and interpreting consistency beliefs.

**Files:**
- Create: `~/.claude/commands/goast-beliefs.md`

**Step 1: Write the skill**

```markdown
# Belief-Based Consistency Checking

Define and run consistency beliefs against Go packages using wile-goast's belief
DSL. Beliefs detect deviations from statistical patterns (Engler et al., "Bugs
as Deviant Behavior").

## Arguments

- `$ARGUMENTS` - Optional: "run" to run existing beliefs, "define" to create new
  ones, or a description of the pattern to check

## Instructions

### Step 1: Check for existing beliefs

Look for `.goast-beliefs/` directory in the project root:

```bash
ls .goast-beliefs/*.scm 2>/dev/null
```

### Step 2: Based on the request

**If running existing beliefs:**

```bash
wile-goast '(begin
  (import (wile goast belief))
  (for-each (lambda (f) (load f))
    (list ".goast-beliefs/file1.scm" ".goast-beliefs/file2.scm"))
  (run-beliefs "./..."))'
```

**If defining a new belief:**

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

Write the belief to `.goast-beliefs/<name>.scm`.

### Step 3: Interpret results

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

---

## Task 7: `/goast-refactor` skill

For finding unification candidates and verifying refactoring correctness.

**Files:**
- Create: `~/.claude/commands/goast-refactor.md`

**Step 1: Write the skill**

```markdown
# Analysis-Backed Refactoring

Use wile-goast to find unification candidates (duplicate/near-duplicate functions)
and verify refactoring correctness via structural analysis.

## Arguments

- `$ARGUMENTS` - The refactoring goal: "find duplicates in package X",
  "verify refactoring of Y", "unify functions A and B"

## Instructions

### Phase 1: Detect unification candidates

Use the built-in unification detection script or compose a custom query:

```bash
wile-goast --run unify-detect-pkg
```

Or for specific function pairs, spawn the `goast-analyzer` agent to:
1. Parse both functions to s-expression ASTs
2. Run recursive AST diff
3. Score similarity (shared nodes, diff categories, weighted cost)
4. Identify parameterizable differences (type params, value params)

**Interpreting scores:**
- `similarity > 0.80` with `weighted-cost < 10` — strong unification candidate
- `param-count <= 3` — feasible to parameterize
- `structural` diffs — functions have different control flow, unlikely to unify
- `identifier` diffs — free renames, don't count against unification
- `type-name` diffs — become type parameters or interface constraints
- `literal-value` diffs — become value parameters

### Phase 2: Plan the unification

If unification is feasible:
1. Identify the unified function signature (original + type/value params)
2. Use call graph to find ALL call sites of both functions:
   ```bash
   wile-goast '(let ((cg (go-callgraph "package" (quote cha))))
     (append (go-callgraph-callers cg "FuncA")
             (go-callgraph-callers cg "FuncB")))'
   ```
3. Determine if unification reduces total complexity (fewer lines, fewer
   concepts) or merely compresses code at the cost of indirection

### Phase 3: Verify after refactoring

After the refactoring is applied:
1. Run call graph analysis to confirm all call sites reference the unified
   function
2. Run beliefs (if `.goast-beliefs/` exists) to confirm no consistency patterns
   were broken
3. Run lint passes on changed packages:
   ```bash
   wile-goast '(go-analyze "package/..." "nilness" "unusedresult" "shadow")'
   ```

## Rules

- Always show the diff scores and your interpretation before suggesting a merge
- Don't suggest unification when the weighted cost is high (structural diffs)
- Don't suggest unification when it would add more parameters than it removes
  lines — that's compression, not simplification
- Verify by substitution: can every call site use the unified function?
```

---

## Task 8: Pre-commit hook

Fires on commit to check beliefs against changed Go packages.

**Files:**
- Modify: `~/.claude/settings.json`
- Create: `~/.claude/hooks/goast-pre-commit.sh`

**Step 1: Write the hook script**

Create `~/.claude/hooks/goast-pre-commit.sh`:

```bash
#!/bin/bash
# goast-pre-commit: Run .goast-beliefs/ checks against changed Go packages.
# Exits 0 (silent) if no beliefs directory or no Go files changed.

set -euo pipefail

# Skip if wile-goast isn't installed
if ! command -v wile-goast &>/dev/null; then
  exit 0
fi

# Skip if no .goast-beliefs/ directory
if [ ! -d ".goast-beliefs" ]; then
  exit 0
fi

# Find belief files
belief_files=$(find .goast-beliefs -name '*.scm' 2>/dev/null)
if [ -z "$belief_files" ]; then
  exit 0
fi

# Build load expressions for all belief files
loads=""
for f in $belief_files; do
  loads="${loads}(load \"${f}\")"
done

# Run beliefs against ./...
output=$(wile-goast "(begin (import (wile goast belief)) ${loads} (run-beliefs \"./...\"))" 2>&1) || true

# If there's output (deviations found), print it
if [ -n "$output" ]; then
  echo "goast-beliefs: deviations detected"
  echo "$output"
fi

# Always exit 0 — deviations are informational, not blocking
# Change to exit 1 to make deviations block the commit
exit 0
```

```bash
chmod +x ~/.claude/hooks/goast-pre-commit.sh
```

**Step 2: Register the hook in settings.json**

Add to the `hooks` section of `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreCommit": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "/Users/aalpar/.claude/hooks/goast-pre-commit.sh",
            "timeout": 60,
            "statusMessage": "Checking beliefs..."
          }
        ]
      }
    ]
  }
}
```

Note: merge with existing hooks, don't overwrite them.

**Step 3: Test the hook**

In a Go project with a `.goast-beliefs/` directory, make a change and commit via
Claude Code. Verify the hook runs and outputs findings (or runs silently if no
deviations).

**Step 4: Commit the hook script**

The hook script lives in `~/.claude/hooks/` (user config, not project). No git
commit needed for the script itself.

---

## Task 9: Pre-PR hook

Fires on PR creation to run the full analysis suite.

**Files:**
- Create: `~/.claude/hooks/goast-pre-pr.sh`
- Modify: `~/.claude/settings.json`

**Step 1: Write the hook script**

Create `~/.claude/hooks/goast-pre-pr.sh`:

```bash
#!/bin/bash
# goast-pre-pr: Run beliefs + lint on changed packages before PR creation.

set -euo pipefail

# Skip if wile-goast isn't installed
if ! command -v wile-goast &>/dev/null; then
  exit 0
fi

echo "Running pre-PR analysis..."

# 1. Run beliefs if .goast-beliefs/ exists
if [ -d ".goast-beliefs" ]; then
  belief_files=$(find .goast-beliefs -name '*.scm' 2>/dev/null)
  if [ -n "$belief_files" ]; then
    loads=""
    for f in $belief_files; do
      loads="${loads}(load \"${f}\")"
    done
    echo "=== Belief checks ==="
    wile-goast "(begin (import (wile goast belief)) ${loads} (run-beliefs \"./...\"))" 2>&1 || true
  fi
fi

# 2. Run lint passes on changed packages
echo "=== Lint checks ==="
wile-goast '(go-analyze "./..." "nilness" "unusedresult" "shadow")' 2>&1 || true

exit 0
```

```bash
chmod +x ~/.claude/hooks/goast-pre-pr.sh
```

**Step 2: Register in settings.json**

Claude Code does not have a built-in `PrePR` hook event. Instead, this should be
wired into the `/pr` skill or the `github-branch-commit-pr` agent. Add a note to
the PR skill or agent to invoke the script before creating the PR.

Alternative: add it as a `PreToolUse` hook matching the `gh pr create` pattern.
The exact wiring depends on which hook events Claude Code supports — check the
current hook event types and wire accordingly.

**Step 3: Commit**

No git commit needed — user config files.

---

## Summary: Build Order

| Task | Phase | Description | Depends on |
|------|-------|-------------|------------|
| 1 | Foundation | Embed lib files (`go:embed` + `WithSourceFS`) | — |
| 2 | Foundation | Embed scripts + `--list-scripts` / `--run` | Task 1 |
| 3 | Knowledge | Global CLAUDE.md section | Task 2 (for invocation docs) |
| 4 | Knowledge | `goast-analyzer` agent definition | — |
| 5 | Workflows | `/goast-analyze` skill | Task 4 |
| 6 | Workflows | `/goast-beliefs` skill | Task 4 |
| 7 | Workflows | `/goast-refactor` skill | Task 4 |
| 8 | Automation | Pre-commit hook | Task 6 |
| 9 | Automation | Pre-PR hook | Task 6 |

Tasks 1-2 are wile-goast code changes (need branch + commit).
Tasks 3-9 are Claude Code configuration (user-level files, no project commits).
Tasks 3, 4 can be done in parallel.
Tasks 5, 6, 7 can be done in parallel after Task 4.
Tasks 8, 9 can be done in parallel after Task 6.
