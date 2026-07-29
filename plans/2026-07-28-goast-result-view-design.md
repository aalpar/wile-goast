# Result views for MCP pipeline tools: filter, rank, trim

**Status:** design complete, impl pending.
**Scope:** `find_duplicates` and `find_false_boundaries` only. The other six
pipeline tools follow in a second pass; see *Out of scope*.

## Problem

The pipeline tools return everything the analysis produced. An LLM client cannot
narrow the question before paying for it, cannot get the interesting results
first, and cannot avoid receiving fields it has no use for. Measured on this
repo, the two tools in scope fail in two *different* ways, which is why they were
chosen together:

| tool | target | rows | result bytes | failure mode |
|------|--------|-----:|-------------:|--------------|
| `find_duplicates` | `./goast` | 68 | 30515 | 20404 pairs scored (60s) to produce 68 rows; no way to ask about one file |
| `find_false_boundaries` | `./goast` | 5 | 13307 | only 5 rows, but 90% of the bytes are lattice context |

`find_false_boundaries`'s byte breakdown, measured per field across all 5 rows.
Shares are of 13301, the sum of the rows; the 13307 above is the whole-result
write, which is how `find_duplicates`'s 30515 was measured. The 6-byte gap is the
list delimiters.

| field | bytes | share |
|-------|------:|------:|
| `incomparable-with` | 8056 | 60.6% |
| `superconcept-of` | 3801 | 28.6% |
| `subconcept-of` | 178 | 1.3% |
| `extent` + `intent` (the finding itself) | 991 | 7.5% |

Row-count paging is irrelevant to that tool. Row *width* is the whole problem.
A design that only pages would leave it exactly as unusable as it is today.

## Decisions

Locked in discussion, recorded so the impl does not relitigate them.

1. **The need is server-side narrowing plus ranking.** Cursor paging is a
   fallback the caller will rarely reach, not the primary mechanism.
2. **Filters push down into the analysis wherever push-down is sound.** A scoped
   query should cost less, not merely return less. But push-down is sound only
   where the filter applies *after* the step that forms the structure being
   reported. It does for `find_duplicates` (the prune lands on the flat pair list,
   after concept formation, and a surviving pair scores identically). It does not
   for `find_false_boundaries`, where the filter would restrict the FCA objects and
   the lattice would be recomputed over a different context. Measured on `./goast`,
   `write-only`, 178 objects, 5 cross-boundary concepts:

   | filter | objects kept | concepts found |
   |--------|-------------:|---------------:|
   | none | 178 | 5 |
   | `*map*` | 104 | **0** |
   | `*unmap*` | 49 | **0** |
   | `*Prim*` | 8 | **0** |

   A caller asking about `map*` would be told there are no false boundaries when
   there are five. Reporting absence is worse than declining, so
   `find_false_boundaries` post-filters. It costs nothing to do so: the whole call
   is 1.7s over a lattice of 89 concepts.
3. **Two orthogonal glob parameters**, `scope` (path) and `symbol` (name), with
   identical names and semantics on both tools. No query grammar.
4. **Row width is in scope**, as a `detail` level rather than caller-named field
   lists: the caller should not need to know field names before it has seen a
   result.
5. **Defaults are on**: `limit` 50, `detail` `summary`. A client that knows
   nothing about these parameters must get a usable response on the first call,
   which is the failure being fixed. Provenance reports what was withheld.

## Surface

Five optional parameters, same meaning on both tools. These defaults are the
tool's defaults, in the Scheme pipeline. The Go handler passes values through and
adds none of its own; there is exactly one place a default is written.

| param | type | default | meaning |
|-------|------|---------|---------|
| `scope` | glob | unset | file or package path the element must match |
| `symbol` | glob | unset | element name must match |
| `detail` | `summary` \| `full` | `summary` | row width |
| `limit` | integer ≥ 0 | 50 | maximum rows returned |
| `offset` | integer ≥ 0 | 0 | rows skipped before `limit` applies |

### What the globs match against

Globs support `*` only. Naively anchoring them to the whole string would make
both parameters nearly unusable, because the strings the tools actually carry are
long and inconsistent:

| | `find_duplicates` | `find_false_boundaries` |
|---|---|---|
| position | `/Users/aalpar/projects/wile-workspace/wile-goast/goastssa/mapper.go:568:1` | same absolute form |
| method name | `ssaMapper.mapChangeInterface` | `(*github.com/aalpar/wile-goast/goast.loopRewriter).replaceInStmts` |
| function name | `SumSlice` | `github.com/aalpar/wile-goast/goast.cloneTypeExpr` |

Positions are **absolute**, so `goast/*.go` would match nothing. Worse, the two
tools name the same kind of thing differently: `find_duplicates` reports
`Type.Method` (its objects come from `go-func-refs`), `find_false_boundaries`
reports the fully package-qualified SSA form (its objects come from
`go-ssa-field-index`). A single anchored glob would mean different things on each
tool, which defeats decision 3.

One rule fixes both: **a glob matches if it matches the whole string, or any
suffix of it beginning at a separator boundary** — `/` for `scope`, `.` for
`symbol`. Matching a suffix at *any* boundary, not just the last one, is what
makes the common inputs work:

- `scope=marshal.go`, `scope=goast/marshal.go`, and `scope=*/goast/*.go` all
  work. Absolute paths need no special handling and no relativization.
- `symbol=cloneTypeExpr` works on both tools despite the different qualification.
- `symbol=ssaMapper.*` still selects by receiver where the caller wants that.

A last-segment-only rule was considered and rejected: it fails on
`scope=goast/marshal.go`, which is exactly what a caller with a file path in hand
will write.

The trade-off is that a glob can match via several candidate strings, so a caller
cannot demand the fully qualified form specifically, and a short `symbol` can
match same-named methods on different receivers. That is the right trade for a
caller that has a name in hand and does not know which tool qualifies it how; the
qualified name is in every returned row, so a second, narrower call is available.

`glob-match?` is new and belongs in `(wile goast utils)` beside `filter`
(`utils.scm:34`); nothing in the tree does glob matching today, only
`string-contains` and SRFI-13 `string-suffix?`, and wile exposes no regex or
`filepath.Match` primitive.

An unset `scope`/`symbol` matches everything. Both set means both must match
(conjunction). There is no negation or disjunction; if that need appears, it
argues for the `select=` grammar rejected in decision 3, not for more parameters.

## Shared view layer

One combinator in `pipelines.scm` owns everything that is not tool-specific:

```scheme
;; apply-view: rows -> (values shown view-provenance)
;; KEEP? admits a row (#f = admit everything, for a tool that already pruned);
;; RANK-KEY yields the sort key; PROJECT narrows a row for detail=summary.
(define (apply-view rows opts keep? rank-key project) ...)
```

It filters, sorts, applies `offset`/`limit`, projects when `detail` is `summary`,
and emits the provenance those operations imply:

| key | meaning |
|-----|---------|
| `total-count` | rows after filtering, before offset/limit |
| `returned-count` | rows in `result` |
| `offset`, `limit`, `detail` | echoed, so the response is self-describing |
| `truncated` | `total-count > offset + returned-count` |
| `scope`, `symbol` | echoed when set |

`truncated` is the load-bearing field. A client seeing `total_count: 186,
returned_count: 50, truncated: true` knows to narrow or page; one seeing
`truncated: false` knows it has the complete answer. Silent truncation with no
such signal would be worse than no limit at all.

`keep?` is where the two tools diverge. `find_false_boundaries` passes one,
because filtering finished rows is the only correct place for it (decision 2).
`find_duplicates` passes `#f`, because by the time rows exist it has already
pruned, and re-applying the predicate here would be a no-op that hides where the
work was actually saved. Either way `total-count` counts rows after filtering, so
`truncated` means the same thing on both.

## Per-tool hooks

Each tool supplies three things: where `scope`/`symbol` apply, a rank key, and a
summary projection. They differ on the first, per decision 2.

### `find_duplicates` — push-down

Push-down point: `find-scored-candidates` (`dup-detect.scm:269`) builds
`pos-index` from the cheap `go-func-refs` pass *before* the expensive
`canon-index` (`build-func-canon-index` over `go-ssa-build`). The work list is
the flat `candidate-pairs` list (`dup-detect.scm:257`), consumed by
`scored-candidates` (`dup-detect.scm:260`). Filtering `candidate-pairs` there
means both name and file are already available, and the surviving function set
can also restrict what `canon-index` canonicalizes.

This is sound because the FCA concepts are formed from the whole session, before
the prune, and `score-candidate-pair` depends only on the two functions and the
threshold. A scoped result is therefore a genuine subset of the unscoped one.

A pair survives if *either* function matches (asking about `marshalValue` should
surface its duplicate wherever that lives).

- **Rank:** `score` descending, `proven` before `structural` on ties.
- **Summary keeps:** `functions`, `score`, `equiv_tier`.
- **Summary drops:** the `measures` sub-object (`benefit`, `type_params`,
  `value_params`, and `similarity`, which duplicates the top-level `score`).

**Interaction with the scoring guard.** `pipeline-find-duplicates`
(`pipelines.scm:295`) refuses a target over `max-candidate-pairs`
(`pipelines.scm:245`, currently 25000) using a pre-scoring pair count. That count
must be taken **after** the scope/symbol prune, so that a narrow query against a
wide target is answered rather than refused. This is the single largest win in
the design: it converts the guard from a hard wall into a wall the caller can
walk around by being specific.

### `find_false_boundaries` — post-filter

`pipeline-find-false-boundaries` (`pipelines.scm:151`) builds a field-access
context, then a lattice, then `cross-boundary-concepts` (`fca.scm:277`), then
`annotated-boundary-report` (`fca-algebra.scm:45`). The filter applies to the
**finished rows**, keeping a concept whose extent contains at least one matching
function, for the reason recorded in decision 2: restricting the objects would
recompute the lattice and report absence where findings exist.

There is no cost argument against post-filtering here. The whole call is 1.7s and
the lattice has 89 concepts.

`detail=summary` can skip `annotated-boundary-report` rather than computing and
discarding it, but the saving is **bytes, not time**: that pass relates 5
cross-boundary concepts to a lattice of 89, roughly 445 comparisons inside a 1.7s
call. Skip it because the output is unusable otherwise, not for speed.

- **Rank:** types spanned descending, then intent size, then extent size. A
  concept that ties more struct types together across more shared fields is the
  stronger false-boundary signal. *This is inference, not something the existing
  code states* — `cross-boundary-concepts` takes `min-types` as a floor but
  expresses no preference among concepts above it.
- **Summary keeps:** `extent`, `intent`, `extent_size`, `summary`, and a new
  `types_count`. `cross-boundary-concepts` computes the type set at `fca.scm:283`
  to enforce `min-types` and then discards it, returning the bare concept, and the
  pipeline calls `annotated-boundary-report` rather than the `boundary-report`
  that recomputes it. So this is a one-line recompute from the intent
  (`(unique (map attr-struct-name intent))`), not an exposed value being plumbed.
- **Summary drops:** `subconcept_of`, `superconcept_of`, `incomparable_with`.

## Ordering

Offset paging is meaningless without a stable total order, so each rank key ends
in a deterministic tie-break on the row's primary name (joined and
string-compared). Two calls with the same arguments must return the same rows in
the same order, and page 2 must not contain a row already seen on page 1.
`TestFindDuplicates_Deterministic` pins the ordering half for one tool, though by
`reflect.DeepEqual` over the parsed result rather than over bytes (its own comment
calls that "the practical form" of byte-identity). The other tool gets the
equivalent.

## Errors

Validated in the Go handler, matching how `mode` is already checked
(`mcp_tools.go:282`, and a second identical site at `:297`):

- `detail` outside `summary`/`full` → tool error naming both legal values.
- `limit` or `offset` negative or non-integer → tool error.

A `scope`/`symbol` that matches nothing is an **empty success**, not an error,
consistent with the existing rule that an empty `find_duplicates` result means
"no duplicates" rather than "something went wrong".

## Testing

1. **`find_duplicates` push-down is a real prune.** A scoped call returns a
   subset of the unscoped call's rows *and* completes measurably faster on a
   target where scoring dominates.
2. **`find_false_boundaries` post-filter preserves findings.** A scoped call
   returns exactly those unscoped rows whose extent matches, and in particular
   does **not** return zero where the unscoped call returned rows. This is the
   regression pin for decision 2's counterexample: `symbol=*map*` on `./goast`
   must return the matching subset of the 5 concepts, not the 0 that pushing the
   filter into the context would produce.
3. **Guard interaction.** A narrow `scope` against a target that would otherwise
   exceed `max-candidate-pairs` is answered, not refused.
4. **Rank stability.** Repeated identical calls return the same rows in the same
   order; page 2 and page 1 are disjoint and jointly equal an unpaged call.
5. **`truncated` accuracy.** True exactly when rows were withheld; false on a
   complete result, including the empty one.
6. **Field sets.** `summary` and `full` each return exactly their documented
   fields; `full` reproduces today's shape byte-for-byte.
7. **Glob semantics.** `*` matching; whole-string *or* separator-aligned-suffix
   match against both an absolute position path and a qualified name, including
   the multi-segment `goast/marshal.go` case; unset means everything; both set
   means conjunction.

## Interactions with shipped code

- **Nothing outside the MCP handlers calls these pipelines.** `grep -rn
  "pipeline-"` over `examples/` and `cmd/wile-goast/scripts/` returns nothing; the
  only callers are the five handlers in `mcp_tools.go` and one integration test at
  `mcp_tools_integration_test.go:270`. So the defaults can live in the Scheme
  pipeline, where they are testable without an MCP round trip, and no
  compatibility split between the two surfaces is needed.
- **`tools/legibility/probe.go`** derives its expected buckets from the tool's
  own output and maps `divergent → distinct` (`probe.go:144`). Since the
  equiv-tier filter shipped, no `divergent` pair reaches the output, so the
  `distinct` bucket is unreachable while the prompt (`probe.go:183`) still offers
  it. The probe does not fail, but it no longer discriminates on that bucket and
  now presents a choice no correct answer can use. Fix the prompt when this lands.
- **`docs/MCP.md`** gains the parameter table and the provenance keys.

## Out of scope

- The other six tools. `recommend_split` returns a single proposal with no rows;
  `recommend_boundaries` returns three named Pareto frontiers rather than a list,
  so `limit`/`offset` do not type-check against it without a per-frontier
  decision; the belief tools' result shape has not been examined for this.
- **Cross-call result caching.** The engine is per-session and long-lived
  (`mcp.go:67`), so caching a result set and paging over it is architecturally
  available. It is not needed here: only `find_duplicates` has a cost problem, and
  push-down answers it directly. Revisit if a tool appears that is both expensive
  and unprunable, which is exactly the shape `find_false_boundaries` would have
  had if it were slow.
- Any query grammar beyond `*` globs.
- A hard byte cap on pipeline responses. `eval` has one
  (`maxEvalResultBytes`, `mcp.go:317`); the pipeline tools do not. `limit` plus
  `detail` addresses the same risk from the other end, and a byte cap that cuts
  mid-structure produces unparseable JSON. Worth revisiting as a backstop.
