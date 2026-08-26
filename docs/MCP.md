# MCP Server

`wile-goast` exposes its analysis surface over the Model Context Protocol. Two
transports share the same ten tools and five prompts.

- `wile-goast --mcp` — stdio MCP server (JSON-RPC). One session keyed `"stdio"`.
- `wile-goast --http[=ADDR]` — Streamable HTTP MCP server. `--http` alone binds
  `127.0.0.1:8080` (loopback); `--http=:9000` binds all interfaces. Endpoint is
  `/mcp`. Stateful sessions; graceful shutdown on SIGINT/SIGTERM.
- `--http-idle-ttl=<dur>` tunes the idle-session sweeper (default `30m`; `0`
  disables). Requires `--http`.

**Engine model:** one `*wile.Engine` per MCP session, keyed by `SessionID()`,
built lazily on first tool call and closed on session unregister (clean DELETE
or idle sweep). This isolates each HTTP client's state (`go-load` sessions,
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

Eight coarse-grained tools wrap already-implemented analyses, returning a
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
| `find_false_boundaries` | `target`, `mode?`, `min_extent?`, `min_intent?`, `min_types?`, `scope?`, `symbol?`, `detail?`, `limit?`, `offset?` | cross-boundary report |
| `find_duplicates` | `target`, `threshold?`, `verdict?`, `scope?`, `symbol?`, `detail?`, `limit?`, `offset?` | the scored, located pairs at or above `threshold` similarity, each labelled with its `equiv_tier` (`proven`/`structural`/`divergent`); opt-in `verdict` |
| `scheme_defs` | `symbol`, `roots` | definition sites (file/line/col) for `symbol` across comma-separated `roots`; exploded `define-record-type` names, resolved `.sld` exports; lexical, pre-macro-expansion |
| `scheme_symbols` | `file` | one `.scm`/`.sld`'s top-level definitions ordered by line, plus its library export list if a `.sld` |

- Envelope `version` is a per-tool integer, bumped on any breaking change to
  what the tool returns: the row *shape* (a field renamed, retyped or removed)
  or the row *population* (a selection rule that admits rows a prior
  consumer's case analysis does not cover). Additive provenance changes do not
  bump. Errors surface via MCP's `isError`, not the envelope. JSON keys are
  snake_case; Scheme alists stay kebab-case (converted by
  `cmd/wile-goast/marshal.go`). `find_duplicates` is at `version` 3,
  `find_false_boundaries` at 2.
- `mode` is the FCA field-access mode: `write-only` (default),
  `read-write`, or `type-only`. Any other value is a tool error.
- Optional-parameter defaults: `idf_threshold` 0.36, `max_attributes` 30,
  `refine` false, `threshold` 0.6, `verdict` false, `min_extent` /
  `min_intent` / `min_types` 2. `find_duplicates`'s `threshold` is a floor on
  effective similarity and is what selects the rows returned (see below).
  `discover_beliefs` with `committed_path` omitted or `""` disables
  suppression and returns raw discovery output.
- `find_duplicates` returns every scored pair at or above `threshold`, each
  carrying its own `equiv_tier`. FCA clustering is candidate *generation*,
  C(k,2) per concept; the tier is the adjudication that follows, and it is
  **reported, not imposed**. Provenance carries `tiers`
  (`proven`/`structural`/`divergent`), `scored_pair_count`, and `total_count`
  (post-floor, pre-paging).

  The tier used to be the filter, and that was wrong in a way no threshold
  could work around. `unifiable?` is categorical in its second clause — the
  effective similarity must clear `threshold` **and** no remaining diff may
  fall outside `{type-name, derived-type, identifier, register}`. The second
  clause has no knob, so a pair can sit at 0.94 similarity and still be
  `divergent` at threshold 0.90, 0.60 *and* 0.10. Measured on wile's quasi
  cluster at its pre-refactor commit, `quasiquoteNeedsRuntime` vs
  `quasisyntaxNeedsRuntimeGuarded` scored 0.94 and the tool reported
  `total_count` 0 — indistinguishable from a clean tree. That pair's four
  `structural` diffs were the three latent bugs a human review found in the
  same file hours later. The informative divergence *is* the payload.

  Volume is bounded by the floor the caller sets rather than by a gate they
  cannot reach: on the wile quasi cluster, `threshold` 0.6 keeps 1258 of 5855
  scored pairs. Raise `threshold` to narrow; filter the reported `equiv_tier`
  to recover the old "only pairs that unify by substitution alone" view. An
  empty `result` with a non-zero `divergent` count is still a real answer: the
  pairs were examined and none cleared the floor.
- `find_duplicates` runs discovery first and errors if the target yields more
  than `max-candidate-pairs` (25000) pairs. That is a latency budget, not a
  projection ceiling: scoring runs 280-340 pairs/s and the cost is sunk before
  any result exists, so the guard reads the concept lattice only and refuses in
  ~1s rather than after minutes (`./...`, 58551 pairs, 3m31s). Provenance
  carries `candidate_pair_count` and `max_candidate_pairs`. No single package
  in this repo exceeds the budget; `goast`, the largest, yields 20404 and is
  answered in ~60s.
- Prefer a pipeline tool for its known structural query; use `eval` for
  open-ended exploration. Design: `plans/2026-04-19-mcp-tool-surface-{design,impl}.md`.

### Result views (`find_duplicates`, `find_false_boundaries`)

Both tools take the same five optional parameters, with the same meaning.
Defaults live in the Scheme pipeline, so a client that passes none of them
still gets a usable response.

| Param | Type | Default | Meaning |
|-------|------|---------|---------|
| `scope` | glob | unset | file path the element must match |
| `symbol` | glob | unset | element name must match |
| `detail` | `summary` \| `full` | `summary` | row width |
| `limit` | integer ≥ 0 | 100 | maximum rows returned |
| `offset` | integer ≥ 0 | 0 | rows skipped before `limit` applies |

`*` is the only metacharacter and matches any run of characters, separators
included. A glob matches if it matches the whole string **or the whole of any
suffix beginning at a separator boundary** — `/` for `scope`, `.` for
`symbol`. So `scope=marshal.go`, `scope=goast/marshal.go` and
`scope=*/goast/*.go` all work against the absolute position path with no
relativization, and `symbol=cloneTypeExpr` works on both tools despite one
reporting `Type.Method` and the other the package-qualified SSA form. Unset
matches everything; both set is a conjunction on the same element. There is no
negation or disjunction.

A glob matching nothing is an **empty success**, not an error. An invalid
`detail`, or a negative or non-integer `limit`/`offset`, is a tool error.

Provenance gains `total_count` (rows after filtering, before paging),
`returned_count`, `offset`, `limit`, `detail`, `truncated`, and an echo of
`scope`/`symbol` when set. **`truncated` is the field to read**, but it says
whether *rows* were withheld, not whether every field is present: false at the
default `detail=summary` still means each row is narrower than `full` (on
`find_false_boundaries`, 90.5% of the bytes narrower). True means narrow or
page.

The two tools differ in *where* the filter applies, and this is deliberate:

- `find_duplicates` **pushes down**. The prune lands on the flat candidate-pair
  list before scoring, so a scoped query is cheaper, not merely smaller — and
  the `max-candidate-pairs` guard reads the count *after* the prune, so a
  narrow query on a target that would otherwise be refused is answered.
- `find_false_boundaries` **post-filters** finished concepts. Restricting the
  FCA objects would recompute the lattice over a different context and report
  absence where findings exist: on `./goast`, `symbol=*restructure*` pushed
  into the context finds 0 cross-boundary concepts against the 2 that actually
  match. The whole call is ~1.7s over an 89-concept lattice, so there is no
  cost argument on the other side.

`detail=summary` is where the second tool's real saving is: on `./goast` its
three lattice-relationship fields are 90.5% of the result bytes, and `summary`
skips computing them rather than computing and discarding them. `types_count`
survives both detail levels, since it is `fb-rank-key`'s primary sort
component and a `full` result should expose the key it is ordered by.
`find_duplicates`' `summary` drops the `measures` sub-object, whose
`similarity` merely repeats the top-level `score`; an opt-in `verdict`
survives both detail levels.

Design: `plans/2026-07-28-goast-result-view-{design,impl}.md`.

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
