# Claude Code Integration

**Status**: Proposed
**Dependencies**: wile `fs.FS` library loader support, wile-goast `go:embed`
**Scope**: Any Go project — not workspace-specific

## Problem

wile-goast provides deterministic, compiler-backed static analysis of Go source
code. It is designed for AI agents: LLMs write Scheme fluently, and s-expressions
are the natural representation for tree-structured compiler data. But Claude Code
has no awareness of the tool. When working on Go code, Claude reads source files,
traces logic by eye, and relies on subjective pattern matching — exactly the
failure modes wile-goast was built to eliminate.

The integration should achieve three properties:

1. **Context efficiency** — Go source flows through wile-goast's Scheme
   interpreter, not through the conversation window
2. **Objectivity** — structural questions get deterministic answers from compiler
   infrastructure, not LLM judgment
3. **Automation** — consistency checks run at commit and PR boundaries without
   manual invocation

## Architecture

Six layers, bottom to top. Each layer depends only on those below it.

```
┌──────────────────────────────────────────────────┐
│  Hooks (automated gates)                         │
│  • pre-commit: belief check on changed packages  │
│  • pre-PR: full analysis suite                   │
├──────────────────────────────────────────────────┤
│  Skills (guided workflows)                       │
│  • /goast-analyze   — structural queries         │
│  • /goast-beliefs   — define & run beliefs       │
│  • /goast-refactor  — unify + verify refactoring │
├──────────────────────────────────────────────────┤
│  Agent (deep analysis)                           │
│  • goast-analyzer   — multi-layer composition    │
├──────────────────────────────────────────────────┤
│  CLAUDE.md (knowledge layer)                     │
│  • trigger heuristic + layer table               │
│  • "query structure, don't read it"              │
├──────────────────────────────────────────────────┤
│  .goast-beliefs/ (per-project convention)        │
│  • belief definitions, version-controlled        │
├──────────────────────────────────────────────────┤
│  wile-goast CLI (execution layer)                │
│  • go install, self-contained (embedded libs)    │
│  • embedded script library                       │
└──────────────────────────────────────────────────┘
```

## Prerequisites

Three items required before the integration layers can be built. P1 is already
complete; P2 and P3 are new work.

### P1. wile: `fs.FS` support in library loader — DONE

The library loader (`machine/library_loader.go`) now resolves `(import ...)` via
the `FileResolver` interface — the same interface used by `include`/`load`. This
was the key blocker: previously `import` was hardcoded to the OS filesystem.

**Current state** (verified):
- `LoadLibrary` calls `env.FileResolver().(FileResolver)` at line 73
- `WithSourceFS(fs.FS)` engine option sets up `FSFileResolver`
- `FSFileResolver` resolves through load path stack, library search paths, then
  FS root — all within the virtual filesystem
- `ToFSPath()` returns forward-slash paths for `fs.FS` compatibility
- The `WithSourceFS` docstring shows the exact embedding pattern:
  `go:embed` + `WithSourceFS` + `WithLibraryPaths`

No changes to wile are needed.

### P2. wile-goast: embed library files

```go
//go:embed lib
var embeddedLib embed.FS
```

The `cmd/wile-goast/main.go` passes `embeddedLib` to the engine via
`wile.WithSourceFS(embeddedLib)` and `wile.WithLibraryPaths("lib")`. After this,
`go install` produces a self-contained binary. `(import (wile goast belief))`
works from any directory.

### P3. wile-goast: embed script library + CLI flags

The example scripts (`examples/goast-query/*.scm`) are embedded alongside the
library files. The binary gains two flags:

```
wile-goast --list-scripts          # list available built-in scripts
wile-goast --run <script-name>     # run a named embedded script
```

Optionally, a Scheme primitive `(goast-script "name")` returns the script source
as a string for inspection or composition.

## Layer 1: wile-goast CLI

After prerequisites, the binary is installed via:

```bash
go install github.com/aalpar/wile-goast/cmd/wile-goast@latest
```

Invocation patterns:

```bash
# Evaluate a Scheme expression
wile-goast '(go-parse-file "main.go")'

# Run an embedded script
wile-goast --run belief-example

# List available scripts
wile-goast --list-scripts
```

No runtime dependencies beyond the Go module cache (for `go/packages` to resolve
import paths of target packages).

## Layer 2: Per-Project Beliefs — `.goast-beliefs/`

A directory at the project root containing `.scm` files, each defining one or
more beliefs. Files import the belief DSL and define beliefs, but do **not** call
`run-beliefs`. The runner supplies the target package pattern at invocation time.

```
myproject/
├── .goast-beliefs/
│   ├── lock-unlock.scm
│   ├── error-handling.scm
│   └── co-mutation.scm
├── go.mod
└── ...
```

Example file:

```scheme
;; .goast-beliefs/lock-unlock.scm
(import (wile goast belief))

(define-belief "lock-unlock-pairing"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.90 5))
```

**Design choice**: Separate files per concern, not a single monolithic file.
Easier to add/remove in PRs, maps naturally to reviewable units.

**Runner invocation** (used by hooks and skills):

```bash
# Load all beliefs, run against changed packages
wile-goast '(begin
  (import (wile goast belief))
  (load ".goast-beliefs/lock-unlock.scm")
  (load ".goast-beliefs/error-handling.scm")
  (run-beliefs "my/package/..."))'
```

Or, with a helper script that auto-discovers belief files:

```bash
wile-goast --run run-project-beliefs -- "./..."
```

## Layer 3: Global CLAUDE.md

Added to `~/.claude/CLAUDE.md`. Active in every project, concise enough to not
waste context. Detailed patterns and reference material live in skills (loaded on
demand).

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

## Layer 4: Agent — `goast-analyzer`

A specialized subagent that knows wile-goast deeply. Spawned by skills for
complex multi-step analyses, or directly when the main conversation needs
structural insight.

```yaml
name: goast-analyzer
description: >
  Deep Go static analysis using wile-goast. Use when analyzing Go code
  structure, finding duplicates, checking consistency patterns, or
  verifying refactoring correctness.
tools: [Bash, Read, Glob, Grep]
```

The agent's system prompt contains:

- **Full primitive reference** — all 5 layers + belief DSL, with signatures,
  options, return types
- **AST node tag catalog** — 50+ tags and their field structures
- **Scheme idiom library** — common query patterns for each analysis type
- **Layer selection guidance** — "use CFG for ordering questions, SSA for data
  flow, call graph for reachability"
- **Output interpretation** — how to read belief results, diff scores, call
  graph edges

**Key property**: The agent runs analysis in wile-goast's process, not in
Claude's context window. Go source flows through the Scheme interpreter, not
through the conversation. This is how context efficiency is achieved.

## Layer 5: Skills

Three skills, loaded on demand via slash commands.

### `/goast-analyze` — General structural queries

Entry point for any structural question about Go code. The skill:

1. Determines which analysis layer(s) are appropriate for the question
2. Spawns the `goast-analyzer` agent if the query is multi-step
3. For simple queries, generates and runs the Scheme expression directly

**Contains**: full primitive reference, common pattern library, layer selection
decision tree, example queries organized by question type.

### `/goast-beliefs` — Consistency deviation detection

For defining, running, and interpreting beliefs. The skill:

1. Checks for `.goast-beliefs/` directory in the project
2. If defining new beliefs: guides through site selector, property checker,
   threshold selection
3. If running beliefs: loads definitions, runs against target packages, reports
   deviations with file locations

**Contains**: belief DSL reference, all selector predicates and property checkers,
threshold tuning guidance, example beliefs for common patterns (lock/unlock,
close-after-open, error-check-before-use, co-mutation).

### `/goast-refactor` — Unify + verify refactoring

For finding unification candidates and verifying refactoring correctness. The
skill:

1. **Detect phase**: Run AST diff between candidate functions, score similarity,
   identify parameterizable differences
2. **Plan phase**: Determine if unification reduces total complexity — report
   parameter count, weighted cost, type/value params needed
3. **Verify phase**: After refactoring, use call graph to confirm all call sites
   are updated, run beliefs to check consistency patterns are preserved

**Contains**: diff scoring model (weights, categories), unification feasibility
criteria, call graph verification patterns, before/after comparison workflow.

## Layer 6: Hooks

Two Claude Code hooks, configured in `~/.claude/settings.json`.

### Pre-commit: belief check

Fires when Claude creates a git commit containing `.go` files. Detects changed
Go packages, loads `.goast-beliefs/*.scm` if the directory exists, runs beliefs
against changed packages.

```
trigger: pre-commit
condition: staged *.go files AND .goast-beliefs/ exists
action:
  1. git diff --cached --name-only '*.go' | extract package dirs
  2. For each belief file in .goast-beliefs/:
     load definitions
  3. run-beliefs against changed package patterns
  4. Report deviations (non-zero exit blocks commit)
```

On success: silent.
On deviation: prints findings, blocks commit, Claude sees the output and can
address the issues.

### Pre-PR: full analysis suite

Fires when Claude creates a PR. Runs the full analysis battery against all
changed packages since the base branch.

```
trigger: pre-PR
action:
  1. git diff --name-only $(git merge-base HEAD main)..HEAD '*.go'
     | extract package dirs
  2. Load and run all .goast-beliefs/ definitions
  3. Run go-analyze with relevant lint passes
  4. Optionally: unification detection on new/changed functions
  5. Report as structured output
```

## Build Order

The layers can be built incrementally. Each is independently useful.

### Phase 1: Foundation
1. ~~**P1**: wile `fs.FS` support in library loader~~ — DONE
2. **P2**: wile-goast `go:embed` lib files (`WithSourceFS` + `WithLibraryPaths`)
3. **P3**: wile-goast `--list-scripts` / `--run` flags

### Phase 2: Knowledge
4. **CLAUDE.md**: Global section — trigger heuristic, layer table
5. **Agent**: `goast-analyzer` definition with full prompt

### Phase 3: Workflows
6. **Skills**: `/goast-analyze`, `/goast-beliefs`, `/goast-refactor`
7. **Convention**: `.goast-beliefs/` directory, runner script

### Phase 4: Automation
8. **Hooks**: pre-commit belief check, pre-PR full suite

## Trade-offs

**Why not an MCP server?** An MCP server would be a rigid wrapper around
something already designed for LLM consumption. The power is in dynamic
composition — Claude generating novel Scheme queries on the fly. Pre-canned MCP
tools would limit this to a fixed set of operations, and every new analysis
pattern would require server changes.

**Why skills + agent instead of CLAUDE.md alone?** Context budget. The full
primitive reference, pattern library, and AST node catalog are too large for
always-on CLAUDE.md. Skills load on demand; the agent carries the knowledge in
its own context window.

**Why per-project `.goast-beliefs/` instead of global beliefs?** Beliefs are
project-specific — lock/unlock patterns, error handling conventions, co-mutation
requirements vary by codebase. Version-controlling them with the project ensures
they evolve with the code and are reviewable in PRs.

**Why CLI + Bash instead of structured tool calls?** "LLMs write Scheme
fluently, and s-expressions are the natural representation for tree-structured
compiler data." The CLI is already the right interface. Adding a structured
wrapper would add a translation layer between Claude and the tool with no benefit.
