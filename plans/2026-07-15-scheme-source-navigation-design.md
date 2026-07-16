# Scheme Source Navigation — MCP (design)

**Date:** 2026-07-15
**Status:** Design complete, impl pending.
**Target:** new pure-Scheme library `(wile goast scmnav)` + two MCP tools (`scheme_defs`,
`scheme_symbols`).
**Supersedes:** the four-bullet proposal under *Scheme Source Navigation — MCP* in
[`TODO.md`](../TODO.md).

## 1. Motivation

The Wile codebases are Go *and* Scheme, and a lot of Scheme: wile-goast's own
`lib/wile/goast/*.{scm,sld}`, wile's stdlib, the algebra libraries, tests. gopls
(`wile-gopls`) and every `wile-goast` layer navigate the **Go** side only. There is
**no semantic navigation for the Scheme source at all** — Serena's LSP backends are
Gopls + Pyright, and no Scheme language server exists to install. Reverse-engineering
the Scheme half falls back to grep/read.

wile-goast is the natural home for the fix: it is already a Wile-hosted Scheme program,
so it can read Scheme source with **Wile's own reader** (positions preserved) and expose
navigation primitives as MCP tools, mirroring the Go layers.

| Query | Go | Scheme |
|-------|-----|--------|
| symbol nav / jump-to-def | `wile-gopls go_search`, goast `go-func-refs` | **← this track (v1)** |
| find callers / references | `wile-gopls go_symbol_references` | deferred (v2, see §8) |
| structure / library overview | goast callgraph | **← this track (v1)** |

## 2. Functional requirements — scope

### In scope (v1)

- **Jump-to-def** is the driving query: given a symbol name, return every *definition*
  site across caller-supplied roots, filtered to binding forms (not every textual
  mention). This is the pain grep can't solve: of 40 hits on `cfl-solve`, exactly one
  is the `(define (cfl-solve …) …)`.
- **Full binding-form coverage** (§5): `define` / `define-syntax` / `define-values`,
  `define-record-type` exploded into its constructor/predicate/accessor/mutator names,
  and a **library export map** so an exported/imported symbol resolves to the exporting
  library and, when reachable, its underlying `define`.
- **Library/file overview** (`scheme_symbols`) falls out for free as the same classifier
  projected over one file with no name filter.
- **Caller-supplied roots**: a query names the directory root(s) to scan. Cross-repo
  (wile-goast lib + wile stdlib) = pass both roots.

### Out of scope (v1)

- **Reference / caller finding** (`scheme_refs`). This is the expensive query where
  hygiene bites (shadowing, macro-introduced references). Deferred deliberately; v1 stays
  **sound-by-construction** rather than shipping a lexical approximation that is quietly
  wrong. See §8.
- **Macro/hygiene-accurate resolution.** v1 is lexical (pre-macro-expansion). Reusing
  Wile's expander for real binding resolution is a separate, larger track.
- **`cond-expand`** evaluation. Feature-conditional defines are absent in the current
  tree; named as a limitation, not silently skipped.
- **A persistent/incremental index.** v1 re-scans per query (§4).

### Success criteria

- `scheme_defs("cfl-solve", "lib")` returns the single define site in the real goast lib.
- `define-record-type` yields one row per generated name, each at its **own** line/col.
- A re-export whose define lives outside the passed roots returns a *legible partial*
  (`resolved:#f`, `via-import` named), never a false "not found."
- A malformed file in a root produces a named diagnostic and does **not** abort the query.
- Output ordering is deterministic.

## 3. Non-functional requirements — behavior at scale

- **Corpus size.** wile-goast lib + wile stdlib is a few hundred small files. A stateless
  per-query scan (enumerate → read → classify) is trivially fast; no index, no cache, no
  invalidation state. A persistent index would solve a scale problem that does not exist
  and would add exactly the shared mutable state to avoid until forced.
- **Determinism.** Output sorted by `(file, line)`. This repo has a documented history of
  enumeration-order nondeterminism (site-ordering defect `14b2388`; candidate-order bug in
  the dispatch follow-ups). Ordering is asserted up front (§7).
- **Soundness boundary rides in the result.** Provenance carries the lexical/pre-expansion
  caveat and per-run limits (§6), consistent with the provenance-intrinsic principle:
  the caveat is intrinsic to the result, not parked in a README.
- **No new Go primitive.** `read-syntax` + the syntax-location accessors + `directory-files`
  are all Scheme-visible from goast's KitchenSink engine, so the feature is pure Scheme.

## 4. High-level design

A single pure-Scheme library `(wile goast scmnav)`, a straight pipeline of small units.
Only the first two touch the filesystem; the classifier and everything downstream are pure.

```
roots ──▶ enumerate ──▶ read ──▶ classify ──┬─▶ def-index ──▶ query ──▶ project ──▶ (scm-def…) alists
          (dir walk)   (read-   (per top-    │   (name→sites)  (filter    (tagged
                        syntax)   level form) │                 by name)    alist)
                                              └─▶ library-index (export→define link)
```

| Unit | Signature (conceptual) | Does | Depends on |
|------|------------------------|------|------------|
| `scm-source-files` | `roots → list<path>` | recursive walk **built on** `directory-files` (single-level; the unit recurses into subdirs), keep `.scm`/`.sld` | files ext |
| `read-file-forms` | `path → list<syntax>` | `read-syntax` loop over the port; top-level forms | io + files ext |
| `classify-def-form` | `syntax → list<scm-def>` | match binding heads; explode `define-record-type`; **keeps syntax wrappers to read each name's own position** | syntax accessors |
| `classify-library` | `syntax → library-rec` | for `.sld`: library name + export list + included files | syntax accessors |
| `build-index` | `list<scm-def> × list<library-rec> → index` | name→sites map + export→define links | — |
| `defs-query` | `symbol × index → list<scm-def>` | filter by name; annotate exported/imported symbols | — |
| `project-def` | `scm-def → alist` | `(scm-def (name)(kind)(file)(line)(col)(library)…)` via `Node`/`Field`/`Str` | goast helpers |

**Boundaries.** The classifier is the only unit with real logic and depends on *nothing but
syntax accessors*, so it is unit-testable in isolation by feeding it `read-syntax` output —
no filesystem. Filesystem access is quarantined to `scm-source-files` + `read-file-forms`.
Query and projection are pure map/filter.

### Grounding (verified against source, 2026-07-15)

- **Reader preserves positions, via `read-syntax` (not `read`).** `read`
  (`wile/pkg/extensions/io/register.go:57`) returns bare datums with no location.
  `read-syntax` (`register.go:64`, impl `prim_read_write.go:269`) returns *syntax objects*
  carrying `SourceContext{File, Start/End SourceIndexes{Index,Column,Line}}`
  (`wile/pkg/values/source_context.go:48`). Scheme accessors `syntax-source` /
  `syntax-line` (1-based) / `syntax-column` (0-based) / `syntax->list`
  (`wile/pkg/registry/core/syntax_loc.go:26`). **`syntax->datum` strips location** — the
  classifier must navigate with `syntax->list` and call `syntax->datum` only on leaf
  identifiers to get the name symbol.
- **Reachable from goast.** goast builds with `wile.WithProfile(wile.KitchenSink)`
  (`cmd/wile-goast/main.go:221`); `allExtensions` includes `ioext.Extension` and
  `files.Extension` (`wile/pkg/internal/bootstrap/bootstrap.go:69–70`). So
  `read-syntax`, `open-input-file`/`open-input-string`, and `directory-files` are live.
- **Pattern to mirror.** MCP pipeline tool `check_beliefs`:
  `cmd/wile-goast/mcp_tools.go:29` (`AddTool`) → handler builds
  `(pipeline-check-beliefs …)` and runs it through `invokePipeline` (`mcp_tools.go:149`).
  Output-shape analog `go-func-refs` (`goast/prim_funcrefs.go:151`) emits
  `(func-ref (name)(pkg)(refs)(pos))` via `Node`/`Field`/`Str`/`ValueList`
  (`goast/helpers.go:38`).
- **Impl-time risk to verify:** the `files` extension has a "confined" mode
  (`wile/extensions/files/confined.go`); confirm directory reads are not sandboxed for the
  MCP engine before relying on `directory-files`.

## 5. The definition classifier

The only unit with real subtlety. Each emitted name carries *its own* identifier position,
read off the sub-syntax (not the enclosing form's).

| Form | Emits `(name . kind)` |
|------|----------------------|
| `(define x v)` | `x` · variable |
| `(define (f a…) …)` | `f` · procedure |
| `(define-syntax m …)` | `m` · syntax |
| `(define-values (a b) …)` | `a` · variable, `b` · variable |
| `(define-record-type P (mk a) p? (a p-a) (b p-b set-b!))` | `P` · record-type, `mk` · constructor, `p?` · predicate, `p-a`/`p-b` · accessor, `set-b!` · mutator |
| `(define-library N (export …)(import …)(include …))` | one library record (§5.2) |

- Top-level `(begin …)` splicing is descended one level.
- `cond-expand` is out of scope for v1 (limitation, surfaced in provenance).
- Curried `(define ((f a) b) …)`: extract the innermost head name; kind procedure.

### 5.1 Why `syntax->list`, not `syntax->datum`

`syntax->datum` flattens to a plain s-expr and drops every position. `syntax->list`
returns the *children as syntax objects*, so descending `(define-record-type …)` to the
accessor identifier still lets us read `syntax-line`/`-column` off that exact token. The
classifier navigates with `syntax->list` and calls `syntax->datum` only on leaf
identifiers to recover the name symbol. **This is the one accessor behavior the whole
design leans on** — the position-accuracy test (§7) is its tripwire.

### 5.2 Export → define resolution (a trichotomy, reported honestly)

Library layout is uniform: `(define-library NAME (export …) (import …) (include "x.scm" …))`,
defines living in the included `.scm`; some exports are **re-exports** of imported symbols
(`utils.sld` re-exports `string-join` from `(srfi 13)`; `fca.sld` re-exports from
`(wile algebra fca)`). For each exported name, look it up in the def index restricted to the
library's included files:

- **local** — found: link export → its define site; annotate `(library . "(wile goast utils)")`.
- **re-export** — not local, but the library `import`s others: annotate
  `(via-import . "(srfi 13)")`; if that library is within the passed roots, recurse to its
  define; else `(resolved . #f)`.
- **unresolved** — exported, no local define, no import that could supply it: emit as-is (a
  macro-generated binding or an actual bug — surfaced, not hidden).

Caller-supplied roots (§2) is what makes this honest across repos: `fca.sld` re-exports
from `(wile algebra fca)` in the *wile* repo. One root → legible partial
(`resolved:#f`, `via-import` named); both roots → resolves through to the real define.

## 6. Conceptual data model — output shape & tools

### Def record (Scheme alist → JSON via the pipeline marshaller)

```scheme
(scm-def
  (name       . "cfl-solve")
  (kind       . "procedure")            ; §5 taxonomy
  (file       . "/abs/path/cfl.scm")    ; syntax-source
  (line       . 42)                     ; syntax-line   (1-based)
  (col        . 5)                      ; syntax-column (0-based)
  (library    . "(wile goast cfl)")     ; present when exported / local-resolved
  (via-import . "(srfi 13)")            ; re-export case only
  (resolved   . #t))                    ; #f on unresolved re-export (no file/line then)
```

Result is a list sorted by `(file, line)`. An **unresolved re-export** still emits a row
(name + `via-import` + `resolved:#f`, no position), so "define isn't in these roots" is a
legible partial, never a silent miss. `[]` is a valid answer ("no binding form named X in
these roots"), distinct from an error.

Fields are structured (separate `file`/`line`/`col`), a deliberate divergence from
`go-func-refs`' combined `"file:line:col"` string — JSON consumers prefer structured over
re-splitting a string.

### Library record (for `scheme_symbols` on a `.sld`, and internal linking)

```scheme
(scm-library
  (name     . "(wile goast utils)")
  (file     . "/abs/utils.sld")
  (line     . 15)
  (exports  . ("nf" "tag?" …))
  (includes . ("utils.scm")))
```

### Tools (flat envelope `{version, provenance, result}`)

| Tool | Args | Returns |
|------|------|---------|
| `scheme_defs` | `symbol` (req), `roots` (req, comma-separated dir paths) | all `scm-def` rows whose name = `symbol` across roots; `[]` = genuinely not found |
| `scheme_symbols` | `file` (req, one `.scm`/`.sld`) | every def in that file ordered by line; for a `.sld`, also the `scm-library` record |

`roots` is a comma-separated string (one `string-split`), keeping the tool aligned with the
existing string-typed pipeline surface rather than introducing the first array-typed MCP arg.

### Provenance carries the soundness boundary

```
provenance: { method: "lexical read-syntax scan (pre-macro-expansion)",
              roots: [...], files_scanned: N,
              parse_errors: [{file,line,col,message} …],   // §7, soft failures
              limits: ["cond-expand not evaluated",
                       "macro-generated bindings invisible",
                       "references/callers out of scope (v1)"] }
```

## 7. Error handling & testing

### Error handling — a hard/soft split

Takes its cue from two open TODO bugs (`check_beliefs` silently loading 0 beliefs; a belief
file silently dropped on failure). The lesson both encode: never let a failed input vanish.

| Condition | Handling |
|-----------|----------|
| Root path doesn't exist / unreadable | **hard error** naming the path (required arg pointing nowhere = caller mistake) |
| Malformed `.scm`/`.sld` (read-syntax parse error) | **soft**: record `{file,line,col,message}` in `provenance.parse_errors`, keep indexing the rest, return partial |
| `.sld` names a missing `include` file | soft diagnostic, continue |
| Symbol not found | valid `[]` (not an error) |
| Directory read denied (confined mode) | hard error (query can't be honestly fulfilled) |

One broken file degrades to a named diagnostic + partial answer; a bad *root* fails loudly.
The split mirrors `make ci` (fail loud on structural breakage, diagnose-and-continue on
content): a bad root is structural, a bad file is one row of many.

### Testing

The classifier is a pure function over `read-syntax` output, so the core suite is
filesystem-free (parse a string via `open-input-string`, assert emitted rows). Fixtures go
in `examples/goast-query/testdata/scmnav/`, matching the existing `dominance/` and `aggflow/`
belief-checker fixture convention.

- **Classifier unit tests** — one per binding form. Load-bearing: `define-record-type`
  (all five sub-kinds, each at its own line/col) and top-level `begin` splicing.
- **Export-trichotomy tests** — a fixture lib dir exercising local / re-export-resolved /
  re-export-unresolved / no-define-unresolved.
- **Position-accuracy test** — exact line/col against a known fixture; the tripwire for the
  `syntax->list`-preserves-child-positions assumption (§5.1). Breaks loudly if wile's
  accessor behavior drifts.
- **Determinism test** — stable `(file, line)` ordering.
- **Go integration test** (mirrors `belief_integration_test.go`) — `pipeline-scheme-defs`
  over a fixture root asserting envelope shape, plus a real-target smoke:
  `scheme_defs("cfl-solve", "lib")` resolves in the actual goast lib.
- **Error-path tests** — nonexistent root → hard error; malformed file → parse-error
  diagnostic + other files still indexed; unknown symbol → `[]`.

All tests, including the `define-record-type` explosion test, are written as part of impl.

## 8. Deferred / open questions

- **`scheme_refs` (references / callers).** The natural v2. v1's def-index + library-index
  is the substrate it extends, not replaces. First cut would be lexical (application-site
  collection by name); the open gate is whether to stay lexical or reuse Wile's expander
  for hygiene-accurate resolution (shadowing, macro-introduced references). Decide before
  building refs, not now.
- **Hygiene-accurate resolution.** Larger track; overlaps Wile's expander. Out of v1.
- **ctags/etags stopgap** (considered, rejected as the target): cheap external index, no
  export-map, dead-end for later structure queries. Only if something is needed sooner than
  this can ship.

## 9. Approach alternatives considered

- **A — pure Scheme, per-query scan (chosen).** Reuses Wile's reader; the positions come
  for free from `read-syntax`. Substrate for the deferred refs track.
- **B — Go-backed parser (rejected).** Re-implements Wile's reader in Go and discards the
  positions Wile already computes. Pure liability.
- **C — ctags/etags stopgap (rejected).** §8.
