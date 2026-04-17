# Package Splitting — Phase 3 & 4 Design

Aggregate belief mode for package cohesion monitoring, and an interactive
MCP prompt for one-shot split analysis.

**Status:** Design approved.

**Depends on:** Phase 1-2 complete (`go-func-refs`, `(wile goast split)`).

## Phase 3: Aggregate Belief Mode

### Problem

The belief DSL evaluates checkers per-site: each function gets a category,
deviations are the minority. Package cohesion is not a per-site property —
it's a whole-package verdict. Forcing it into the per-site model produces
misleading framing (what is the "adherent" category for a function in a
package that should be split?).

### New Form: `define-aggregate-belief`

```scheme
(define-aggregate-belief "package-cohesion"
  (sites (all-functions-in "my/pkg"))
  (analyze (single-cluster 'idf-threshold 0.36)))
```

Syntactically distinct from `define-belief`. No `threshold` clause — the
analyzer produces its own verdict.

### Registration

`define-aggregate-belief` calls `register-aggregate-belief!`, which stores
a record with:
- `name` — string
- `site-selector` — `(lambda (ctx) -> list-of-sites)`
- `analyzer` — `(lambda (sites ctx) -> result-alist)`

Stored in a separate list from per-site beliefs. `reset-beliefs!` clears
both lists.

### Evaluation

When `run-beliefs` processes an aggregate belief:

1. Resolve sites via the selector (same as per-site: load packages, run
   selector, get list of func-decls).
2. Pass the **entire site list and context** to the analyzer in one call.
3. Return the analyzer's result directly — no per-site iteration, no
   threshold logic, no adherence/deviation classification.

### Result Shape

```scheme
("package-cohesion"
  (type . aggregate)
  (verdict . SPLIT)
  (confidence . HIGH)
  (functions . 47)
  (report . <recommend-split output>))
```

The `(type . aggregate)` tag distinguishes from per-site results. Per-site
beliefs do not carry this tag (or carry `(type . per-site)` if uniform
tagging is preferred — decide during implementation).

### New Site Selector: `all-functions-in`

```scheme
(all-functions-in "my/pkg/...")
```

Returns all func-decl AST nodes from the matched packages. Same shape as
other selectors (`(lambda (ctx) -> list-of-func-decls)`).

Implementation: loads packages via `go-typecheck-package` through the
context, returns `(all-func-decls (ctx-pkgs ctx))`.

### New Aggregate Analyzer: `single-cluster`

```scheme
(single-cluster 'idf-threshold 0.36)
(single-cluster 'idf-threshold 0.36 'refine)
```

Returns `(lambda (sites ctx) -> result-alist)`.

Steps:
1. Extract the GoSession from context's loaded packages.
2. Call `go-func-refs` with the session (avoids double-load).
3. Call `recommend-split` with IDF threshold and optional `'refine` flag.
4. Map confidence to verdict: `HIGH`/`MEDIUM` → `SPLIT`,
   `LOW`/`NONE` → `COHESIVE`.
5. Return the result alist.

### Cross-Layer Note

The belief context already carries loaded packages (for SSA, call graph,
etc.). `single-cluster` reuses this session via `go-func-refs` session
mode. No redundant `packages.Load` calls.

## Phase 4: Interactive Split Planner

### MCP Prompt: `goast-split`

A Markdown template at `cmd/wile-goast/prompts/goast-split.md`.

**Arguments:**
- `package` (required) — Go package pattern
- `goal` (optional) — motivation for the split

**Three stages:**

**Stage 1: Analyze.** Run `recommend-split` on the package. Display function
count, high-IDF dependencies, group assignments, cut ratio, acyclicity,
confidence.

**Stage 2: Refine.** If confidence is LOW or finer detail is wanted, re-run
with `'refine` (API surface mode). Compare results.

**Stage 3: Plan.** If the split is viable (MEDIUM/HIGH, acyclic), outline:
which functions move, what interface boundary is needed, which bridge
functions need attention.

The prompt contains Scheme snippets for the LLM to eval — a recipe, not a
programmatic workflow. Same pattern as `goast-beliefs` and `goast-refactor`.

### Relationship to Phase 3

- **Prompt (Phase 4):** one-shot analysis — "should I split this package?"
- **Belief (Phase 3):** ongoing monitoring — "alert when cohesion degrades."

The prompt calls `recommend-split` directly. The belief wraps it in the
DSL for batch evaluation via `run-beliefs`.

### Registration

Add to the `promptDef` slice in `cmd/wile-goast/mcp.go`:

```go
{
    name:        "goast-split",
    description: "Analyze package cohesion and recommend splits",
    file:        "prompts/goast-split.md",
    args: []mcp.PromptOption{
        mcp.WithArgument("package",
            mcp.RequiredArgument(),
            mcp.ArgumentDescription("Go package pattern"),
        ),
        mcp.WithArgument("goal",
            mcp.ArgumentDescription("Motivation for the split"),
        ),
    },
}
```

## Scope Boundaries

**In scope:**
- `define-aggregate-belief` / `register-aggregate-belief!` in belief.scm
- `all-functions-in` selector in belief.scm
- `single-cluster` analyzer (new file or in split.scm — TBD during impl)
- `run-beliefs` modification to handle aggregate beliefs
- `goast-split.md` prompt
- `mcp.go` prompt registration
- Tests for all of the above

**Out of scope:**
- Multi-way splits (two-way only, per Phase 1-2)
- Graduation / emit mode for aggregate beliefs
- Weighted min-cut refinements
- `goast-scheme-ref.md` update (deferred to a docs pass)

## Open Questions

1. **Where does `single-cluster` live?** It bridges `(wile goast belief)`
   and `(wile goast split)`. Options: in belief.scm (importing split), in
   split.scm (importing belief), or a new bridge file. Decide during
   implementation based on import direction.

2. **Per-site type tag?** Should per-site results also carry
   `(type . per-site)` for uniform consumer dispatch? Low cost, minor
   convenience. Decide during implementation.
