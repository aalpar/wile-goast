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

- **Reader preserves LINE and COLUMN, via `read-syntax` (not `read`).** `read`
  (`wile/pkg/extensions/io/register.go:57`) returns bare datums with no location.
  `read-syntax` (`register.go:64`, impl `prim_read_write.go:269`) returns *syntax objects*
  carrying `SourceContext{File, Start/End SourceIndexes{Index,Column,Line}}`
  (`wile/pkg/values/source_context.go:48`). Scheme accessors `syntax-line` (1-based) /
  `syntax-column` (0-based) / `syntax->list` (`wile/pkg/registry/core/syntax_loc.go:26`).
  **`syntax->datum` strips location** — the classifier must navigate with `syntax->list`
  and call `syntax->datum` only on leaf identifiers to get the name symbol.
- **CORRECTION (found during implementation): `syntax-source` is a DEAD FIELD over file
  ports — the `file` does NOT come from the reader.** The accessor exists and the Go
  `SourceContext` has a `File`, but `PrimReadSyntax` constructs the parser with
  `NewParser(…)`, which hardcodes the filename to `""` (`wile/pkg/parser/parser.go:66`; a
  `NewParserWithFile` exists and is unused). Verified:
  `(syntax-source (read-syntax (open-input-file "…")))` → `#f`. Line and column are
  unaffected. **The reading code must inject the path it already knows**, via
  `with-source-file` (exported from `(wile goast scmnav)`), which REPLACES the stale
  `(file . "")` entry. `build-index` does this; `pipeline-scheme-symbols` bypasses
  `build-index` and does it itself.
  *This design's original §4 claimed `syntax-source` supplied the file, citing real source
  lines. The grounding verified the accessor EXISTED, never that it was POPULATED from a
  file port. A citation that disarms the next reader's skepticism is worse than an
  unsourced guess — recorded here rather than quietly patched.*
- **Never leave a duplicate key in an alist that gets marshalled.**
  `cmd/wile-goast/marshal.go`'s `marshalPair` builds the JSON object with `obj[key] = val`
  iterating front-to-back (**last write wins**), while Scheme's `assq` returns the **first**
  match. A shadowing prepend — idiomatic in Scheme — therefore INVERTS at the JSON boundary
  and ships the stale value. Replace entries; don't shadow them. A Scheme-side `assq` test
  cannot catch this; assert the key count is 1.
- **`directory-files` is UNSORTED.** wile's confined path uses `(*os.File).ReadDir(-1)`
  (`wile/extensions/files/confined.go:154-168`), which — unlike package-level `os.ReadDir` —
  Go's stdlib does not sort. `scm-source-files` sorts its final path list so every
  downstream consumer inherits a stable order (§3 determinism).
- **UPSTREAM OPPORTUNITY (wile repo, out of scope here):** wiring `PrimReadSyntax` to
  `NewParserWithFile` with the port's filename would make `syntax-source` real and let
  wile-goast drop the `with-source-file` injection entirely.
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
- **Impl-time risk — RESOLVED:** the `files` extension has a "confined" mode
  (`wile/extensions/files/confined.go`), but directory reads are NOT sandboxed for this
  engine. Verified: a repo-root-cwd probe returned `(2 48)`. `directory-files` works.

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

**AS SHIPPED** (corrected against the implementation; the original draft of this section
overclaimed in four ways, all noted inline):

```scheme
;; A resolved definition row:
(scm-def
  (name    . "nf")                          ; STRING
  (kind    . "procedure")                   ; STRING, §5 taxonomy
  (file    . "lib/wile/goast/utils.scm")    ; injected by with-source-file, NOT syntax-source
                                            ;   (§4 correction). Caller-relative pass-through
                                            ;   — paths are NOT absolutized.
  (line    . 22)                            ; syntax-line   (1-based)
  (col     . 9)                             ; syntax-column (0-based)
  (library . "(wile goast utils)"))         ; present only when the file resolves to a library

;; An UNRESOLVED-EXPORT row (positionless — no file/line/col keys at all):
(scm-def
  (name       . "ghost")
  (kind       . "unresolved-export")
  (library    . "(test orphan)")
  (via-import . "(test elsewhere)")         ; ALL of that library's imports, space-joined —
                                            ;   NOT the single import supplying the symbol
  (resolved   . #f))
```

Corrections vs. the original draft:
1. `file` does **not** come from `syntax-source` (see the §4 correction) and is **not**
   absolute — it is the caller-relative path passed in.
2. `resolved` is **absent** on normal rows, not `#t`. It appears only on unresolved-export
   rows, always `#f`. A JSON consumer testing `resolved` gets `undefined` on every real hit
   — test for `kind == "unresolved-export"` instead.
3. `via-import` is the **space-joined list of all** the library's imports, not the one that
   supplies the symbol (resolving *which* import would need the full import graph — out of
   scope for v1).
4. Unresolved rows carry **no `file`/`line`/`col` keys at all** — they are omitted, not
   empty.

Result is a list sorted by `(file, line)` for resolved rows, followed by unresolved rows
sorted by `library` name (positionless rows cannot use the `(file, line)` key). An
**unresolved re-export** still emits a row, so "define isn't in these roots" is a legible
partial, never a silent miss. `[]` is a valid answer ("no binding form named X in these
roots"), distinct from an error.

Fields are structured (separate `file`/`line`/`col`), a deliberate divergence from
`go-func-refs`' combined `"file:line:col"` string — JSON consumers prefer structured over
re-splitting a string.

### Library record (for `scheme_symbols` on a `.sld`, and internal linking)

```scheme
(scm-library
  (name     . "(wile goast utils)")           ; paren-wrapped STRING
  (file     . "lib/wile/goast/utils.sld")     ; caller-relative, injected (not absolutized)
  (line     . 15)
  (exports  . ("nf" "tag?" …))                ; STRINGS; `(rename …)` sublists skipped (v1)
  (imports  . ("(srfi 13)" …))                ; STRINGS, base library names
  (includes . ("utils.scm")))                 ; STRINGS, as written in the clause
```

`includes` are resolved against **the library's own directory** and compared as full paths,
never by basename: `(wile goast fca)` and `(wile algebra fca)` both `(include "fca.scm")`,
so basename matching attributes `(wile algebra fca)`'s definitions to `(wile goast fca)` —
a confident wrong answer on exactly the cross-repo case §2 exists to serve.
Repeated clauses are honored: R7RS permits multiple `export`/`import`/`include`, and this
repo uses them (`belief.sld` has two `include`s; `srfi/14.sld` has six). Taking only the
first silently drops the rest.

### Tools (flat envelope `{version, provenance, result}`)

| Tool | Args | Returns |
|------|------|---------|
| `scheme_defs` | `symbol` (req), `roots` (req, comma-separated dir paths) | all `scm-def` rows whose name = `symbol` across roots; `[]` = genuinely not found |
| `scheme_symbols` | `file` (req, one `.scm`/`.sld`) | every def in that file ordered by line; for a `.sld`, also the `scm-library` record |

`roots` is a comma-separated string (one `string-split`), keeping the tool aligned with the
existing string-typed pipeline surface rather than introducing the first array-typed MCP arg.

### Provenance carries the soundness boundary

**AS SHIPPED** (`scheme_defs`):
```
provenance: { method: "lexical read-syntax scan (pre-macro-expansion)",
              roots: [...], files_scanned: N,
              parse_errors: [{file, message} …],   // §7 soft failures — message ONLY;
                                                   //   line/col extraction from a parse
                                                   //   condition is out of scope for v1
              limits: ["cond-expand not evaluated",
                       "macro-generated bindings invisible",
                       "references/callers out of scope (v1)"] }
```
`scheme_symbols`' provenance carries `method`, `file`, `limits` — and **no `parse_errors`
key**, because it hard-errors on a malformed file rather than soft-collecting (see §7).

## 7. Error handling & testing

### Error handling — a hard/soft split

Takes its cue from two open TODO bugs (`check_beliefs` silently loading 0 beliefs; a belief
file silently dropped on failure). The lesson both encode: never let a failed input vanish.

**AS SHIPPED:**

| Condition | Handling |
|-----------|----------|
| Root path doesn't exist **or is unreadable** | **hard error** naming the path. Validated with `try-directory-files`, which subsumes both — `file-exists?` alone would let an unreadable root pass, and the walk's guard would then silently yield zero files |
| Malformed `.scm`/`.sld` in `scheme_defs` (read-syntax raises) | **soft**: record `{file, message}` in `provenance.parse_errors`, keep indexing the rest, return partial |
| Malformed / nonexistent file in `scheme_symbols` | **hard error** (deviation from the original draft, deliberate: for a one-file tool the single input being bad IS a caller error; loud beats a useless empty envelope) |
| Symbol not found | valid `[]` (not an error) |
| Unreadable **sub**directory during the walk | skipped silently — v1 scope; only ROOTS hard-fail |

One broken file in a multi-root scan degrades to a named diagnostic + partial answer; a bad
*root*, or a bad single-file argument, fails loudly.

**Port safety:** `read-file-forms` closes its port via `dynamic-wind`, not after the read
loop. `read-syntax` *raises* on malformed source and the caller's `guard` escapes past the
close — so the naive form leaks one port per malformed file, on precisely the path the soft
handling exists to serve.

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

### 8.1 The hygiene gate is NOT open — Wile's expander is Scheme-visible (verified 2026-07-16)

**This section originally framed "stay lexical vs. reuse Wile's expander" as an open
decision to be made later. That framing is wrong: the expander is already exposed, and it
consumes exactly the artifact this design's v1 produces.** Recorded here for the v2 track;
not built (v1 shipped lexical, and its `provenance.limits` accurately describe v1).

**The primitives** (`wile/extensions/eval/register.go:72`, extension `eval`, live in goast's
KitchenSink engine — `(procedure? expand)` → `#t`):

| Primitive | Arity | Contract |
|---|---|---|
| `expand` | 1 | *"Fully macro-expands a syntax object or datum. Returns the expanded syntax."* Impl `PrimExpand` (`wile/extensions/eval/prim_eval.go:400`). **Requires a `syntax.SyntaxValue`** — passing a plain quoted datum raises `expand: expected syntax object` (`prim_eval.go:407-410`). |
| `expand-once` | 1 | `PrimExpandOnce` (`prim_eval.go:439`). Returns MULTIPLE values; the registration notes `ReturnType` annotates only the primary. |

**Verified behavior** (probes run against the built `cmd/wile-goast`):

```scheme
;; 1. Full expansion works on read-syntax output — the exact artifact scmnav produces.
(define-syntax swap! (syntax-rules () ((_ a b) (let ((tmp a)) (set! a b) (set! b tmp)))))
(syntax->datum (expand (read-syntax (open-input-string "(swap! p q)"))))
  ;=> (let ((tmp p)) (set! p q) (set! q tmp))

;; 2. *** Expansion PRESERVES the user-written source position of a macro-generated name. ***
(define-syntax mydef (syntax-rules () ((_ n v) (define n v))))
;; source "(mydef foo 42)" — `foo` sits at 0-based col 7
(let* ((x     (expand (read-syntax (open-input-string "(mydef foo 42)"))))
       (name  (cadr (syntax->list x))))
  (list (syntax->datum name) (syntax-line name) (syntax-column name)))
  ;=> (foo 1 7)      <- the ORIGINAL col 7, NOT a synthetic position
;; control: plain "(define foo 42)" -> foo at col 8. Both correct, so positions are real.
```

**Why this matters.** The expanded value is the same syntax-object shape `classify-def-form`
already consumes, so the pipeline is `read-syntax → expand → <existing classifier>`. That
would:
- **delete `"macro-generated bindings invisible"` from `scmnav-limits`** — probe 2 IS that
  case: a macro-generated `define` becomes visible AND points at the source the user wrote;
- likely delete `"cond-expand not evaluated"` (the compiler has
  `wile/pkg/machine/compilation/compile_cond_expand.go`; expansion should resolve it) —
  **unverified, check before claiming**;
- make `scheme_refs` **hygiene-accurate by construction** rather than a lexical
  approximation, because it would walk expanded code. This is the whole reason the gate
  existed, and it is answerable in the affirmative today.

**The real cost — expansion is ENVIRONMENT-SENSITIVE.** The probes worked because the macro
was defined first. Expanding `belief-checkers.scm`'s forms requires `(wile goast belief)`'s
macros in scope. The `.sld`'s `import` clause names what to load (v1 already parses it into
the library-record's `imports`), so this is tractable — but it is the actual build:
1. per-library environment setup (load its imports before expanding its forms);
2. soft-error discipline for expansion failures (unbound macro / missing import), mirroring
   §7's `parse-errors` — a failed expansion must not vanish silently;
3. filtering hygienic macro-INTERNAL bindings (`swap!`'s `tmp`) so they don't pollute the
   index — user-written names keep real positions, introduced ones presumably do not, which
   may itself be the filter;
4. `PrimExpand` branches on `mc.ExpanderContext()` (`prim_eval.go:413`) — non-nil uses the
   current expansion context, else a fallback path. **Not fully read; verify which branch a
   goast query takes and whether it matters.**

**Method note.** This design's §4 originally asserted `syntax-source` supplied the file
(false — see the §4 correction), and this §8 asserted the expander was an open question
(false — it is exposed). Both errors have the same shape: *verifying that a capability
exists in Go, without verifying it is reachable and populated from Scheme.* The reader was
checked that way and the expander was not checked at all.

### 8.2 Still genuinely deferred

- **`scheme_refs` (references / callers).** The natural v2. v1's def-index + library-index
  is the substrate it extends, not replaces. Per §8.1 it should be built on expanded syntax
  (hygiene-accurate by construction), not as a lexical approximation.
- **ctags/etags stopgap** (considered, rejected as the target): cheap external index, no
  export-map, dead-end for later structure queries. Only if something is needed sooner than
  this can ship.

## 9. Approach alternatives considered

- **A — pure Scheme, per-query scan (chosen).** Reuses Wile's reader; the positions come
  for free from `read-syntax`. Substrate for the deferred refs track.
- **B — Go-backed parser (rejected).** Re-implements Wile's reader in Go and discards the
  positions Wile already computes. Pure liability.
- **C — ctags/etags stopgap (rejected).** §8.
