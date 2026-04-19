# TODO

Top-level task: composable API for wile-goast analysis and transformation (Go code inlining).
Two independent tracks — shared sessions and transformation primitives — converge
at the inlining pipeline. See `plans/2026-03-24-transformation-primitives-design.md`.

## Track A: Shared Session API — DONE (v0.5.0)

Completed 2026-03-24. GoSession holds loaded packages, lazy SSA/callgraph.
All 7 package-loading primitives accept GoSession or string. Belief DSL
creates session in `make-context`. See `plans/2026-03-24-shared-session-design.md`.

## Track B: Transformation Primitives — DONE

Scheme-level tree rewriting and Go-level control flow restructuring for
refactoring operations (inlining, extraction, code motion). All sub-items
(B1, B2a, B2b, B3) completed 2026-03-25.

### B1. Scheme utils — DONE

- [x] Implement `ast-transform` in `cmd/wile-goast/lib/wile/goast/utils.scm`
- [x] Implement `ast-splice` in `cmd/wile-goast/lib/wile/goast/utils.scm`
- [x] Add `take` and `drop` to `utils.scm`
- [x] Export new functions from `utils.sld`

### B2a. go-cfg-to-structured — Case 1 (no dependencies) — DONE

- [x] Linear early returns → nested if/else

### B2b. go-cfg-to-structured — Case 2 (depends on B2a) — DONE

Completed 2026-03-25. Returns inside for/range are rewritten as
`_ctl<N> = K; break` with guard-if-return statements after the loop.
Composes with Case 1 (guard folding) in a single call. Supports nested
loops (bottom-up) and multiple return sites per loop.
See `plans/2026-03-25-b3-c2-c6-design.md` (B2b/B3 design).

### B3. go-cfg-to-structured improvements (depends on B2) — DONE

Completed 2026-03-25. Labeled break for switch/select returns in loops,
result variable synthesis for multiple return values, forward and backward
goto elimination (cross-branch still returns `#f`).
See `plans/2026-03-25-b3-c2-c6-design.md` (B3 design).

- [x] Handle goto / labeled branches (forward and backward; cross-branch returns #f)
- [x] Handle switch/select with early returns inside loops (labeled break)
- [x] Handle multiple return values (_r0, _r1, ...)

## Track C: Static Analysis Forms (depends on Wile algebra library)

Wile gets a general-purpose algebra library (`(wile algebra)` or similar).
wile-goast builds static-analysis combinators on top. Items below are
wile-goast consumers — they migrate to or are built on the Wile algebra API
once it exists.

### C1. Migrate existing hand-rolled algebra — DONE

Completed 2026-03-25. `checked-before-use` uses `(wile goast dataflow)` product
lattice fixpoint. `ssa-normalize` uses `(wile algebra rewrite)` axiom declarations.
`score-diffs` left as-is (not algebraic structure).

- [x] `checked-before-use` Kleene iteration → fixpoint over product lattice (powerset x boolean) via `(wile goast dataflow)`
- [x] `ssa-normalize` rewrite rules → migrated to `(wile algebra rewrite)` axiom declarations
- [x] `score-diffs` similarity accumulation — leave as-is (weighted sum + ratio, not semiring structure)

### C2. Dataflow analysis framework — DONE

Completed 2026-03-26. Worklist-based forward/backward analysis over SSA blocks.
`run-analysis` with per-block transfer, `analysis-in`/`analysis-out` queries,
`'check-monotone` flag. See `plans/2026-03-26-c2-dataflow-design.md`.

- [x] Define transfer function interface (per-block)
- [x] Forward/backward analysis combinator over SSA blocks (reverse postorder)
- [x] Worklist algorithm integrated with block ordering
- [x] Per-variable analysis via map lattice (vars → lattice values) — uses existing (wile algebra)
- [x] Product lattice for combining analysis dimensions — uses existing (wile algebra)
- [x] Monotonicity assertion (debug mode) — detect buggy transfer functions

### C3. Pre-built abstract domains — DONE

Completed 2026-04-12. Five domains in `(wile goast domains)` library:
reaching definitions, liveness, constant propagation, sign, interval.
Shared `go-concrete-eval` for integer opcodes. Interval analysis uses
per-block widening in transfer closure.
See `plans/2026-03-26-c3-domains-design.md`.

- [x] Powerset lattice — liveness, reaching definitions
- [x] Flat lattice (⊥ < concrete values < ⊤) — constant propagation
- [x] Sign lattice ({⊥, -, 0, +, ⊤})
- [x] Interval lattice — range analysis with widening

### C4. Path algebra on call graphs

- [x] Boolean semiring — reachability (generalize `go-callgraph-reachable`)
- [x] Tropical semiring — shortest/longest call chains
- [ ] CFL-reachability — context-sensitive analysis

### C5. Galois connections for abstract interpretation

- [ ] Abstraction/concretization pair interface
- [ ] Soundness check (alpha ∘ gamma ⊒ id)
- [ ] Connect Go concrete values to abstract domains

### C6. Belief DSL integration

- [ ] Belief graduation — 100% adherence beliefs become dataflow assertions
- [ ] Belief-defined lattices — express belief checkers as lattice transfer functions

## Track D: Duplicate Detection — Feasibility Probe (2026-04-18)

Probe before committing to full impl. Design doc:
`plans/2026-04-17-fca-duplicate-detection-design.md`.

Hypothesis: the dumbest cross-function SSA block-sequence detector may
catch most real duplicates without FCA prefiltering. If so, simplify the
impl. If not, the miss set informs whether FCA (Track 1) or aggressive
inlining (Track 2) earns its cost.

### D1. Ground truth — hand-labeled corpus

- [ ] Pick target: `wile-goast` itself (~300 funcs), single package or whole repo?
- [ ] Hand-label 15–20 function pairs believed to be duplicates / near-duplicates
- [ ] Classify each pair: structural / algebraic / semantic (same intent, different impl)
- [ ] Save as `plans/2026-04-18-dup-detect-corpus.md` (or similar)

### D2. Minimal detector prototype

- [ ] Flatten each function's SSA to instruction-tag sequence (ignore operands)
- [ ] Find repeated sub-sequences of length ≥ K across any two functions
- [ ] Rank matches by length × inverse-frequency of constituent ops
- [ ] Output: ranked list of (func-a, func-b, match-length, match-location)
- [ ] Prototype in `examples/goast-query/` — not a library yet

### D3. Measure + decide

- [ ] Run prototype on corpus target
- [ ] Compute recall against hand-labels
- [ ] Compute precision on top-N candidates (spot-check)
- [ ] Decide next track based on results:
  - \>80% recall → simplify FCA plan, maybe skip prefilter entirely
  - 30–80% → proceed with FCA duplicate detection impl plan as written
  - <30% → pivot to inlining / boundary-discovery design doc (Track E)

### D4. Open design question (document outcome)

- [ ] If pursuing inlining (Track E): which functions should NOT be inlined?
  Candidates: unbounded loops, interface dispatch sites, cross-cohesion
  boundaries, strongly-named intent functions, exported API. Needs a rule
  that matches human intuition, documented in a design doc before impl.

## MCP / Belief API Mismatch

- [ ] `run-beliefs` returns `#<void>` and writes results via `display` to stdout.
      The MCP eval tool captures only the return value — stdout is discarded
      (or worse, corrupts the JSON-RPC stream). This means beliefs are unusable
      from the MCP interface without the `open-output-string` / `parameterize`
      workaround. Fix: `run-beliefs` should return a structured alist of results
      (belief name, adherence ratio, site count, deviation list) and optionally
      print when called interactively. Same pattern as `go-typecheck-package`
      which returns data, not text.

- [ ] `evaluate-belief` and `make-context` are not exported from `(wile goast belief)`.
      Workaround (`open-output-string` + `parameterize`) works but is fragile —
      LLMs shouldn't need to know Scheme I/O plumbing to use the belief DSL.

## Other

- [ ] Move `stores-to-fields` predicate to Go side
  - Sub-tree matching (fragment detection within functions)
  - CFG isomorphism as a standalone tool
  - Call graph context pre-filtering
  - Integration into the belief DSL
  - --emit mode for the unification detector

- [ ] Previous session friction: `(wile algebra rewrite)` not importable standalone
      (only transitively via `ssa-normalize`), `delete-duplicates` missing from
      Scheme stdlib, SSA requires real packages on disk. See git history for details.

- [ ] Fix `nf` return #f

    ⏺ Good question. The nf returning #f issue is a recurring pattern in this codebase because of a design choice at the foundation layer.

      Why #f keeps being a problem

      nf (the universal field accessor for tagged alists) returns #f for missing fields:

      (define (nf node key)
        (let ((e (assoc key (cdr node))))
          (if e (cdr e) #f)))

      This is Scheme's idiomatic "return sentinel for missing." The problem: #f is also a legitimate Scheme value (boolean false), and — more importantly — it's silently valid in almost every position. It passes
      through list, cons, arithmetic comparisons, and string operations until something finally crashes far from the source. It's the same class of bug as null propagation in Java or nil in Ruby.

      Three factors compound it:
      1. Every nf consumer must check. Hundreds of call sites. Forgetting once creates a silent propagation path.
      2. The "happy path" always works. go-parse-expr produces well-formed AST — op, x, y are always present for binary-expr. So the #f case never triggers in tests. It's only reachable from malformed input or future
      AST structure changes.
      3. The error surfaces far from the cause. (list '#f '#f '#f) is a valid S-expression. It propagates through the normalizer, through sexp-term-protocol, and only crashes when something tries to car on a symbol
      deep inside the algebra library.

      What can be done: The structural fix would be adding nf! (error on missing) alongside nf (return #f), and using nf! at boundaries where fields are structurally required. That's a broader change — worth a separate
       PR. For now, the practical fix is guarding at the boundary in new code.

- [ ] Fix "/staff-engineer" findings below

    ╭─── Claude Code v2.1.97 ────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
    │                                                    │ Recent activity                                                                                                                                               │
    │                 Welcome back Aaron!                │ 11s ago  2                                                                                                                                                    │
    │                                                    │ 23m ago  on false boundary disovery - can we try some functional false boundary discovery?                                                                    │
    │                                                    │ 8h ago   push a release                                                                                                                                       │
    │                       ▐▛███▜▌                      │ /resume for more                                                                                                                                              │
    │                      ▝▜█████▛▘                     │ ───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────── │
    │                        ▘▘ ▝▝                       │ What's new                                                                                                                                                    │
    │                                                    │ Added interactive Google Vertex AI setup wizard accessible from the login screen when selecting "3rd-party platform", guiding you through GCP authentication… │
    │   Opus 4.6 (1M context) with hi… · Claude Max ·    │ Added `CLAUDE_CODE_PERFORCE_MODE` env var: when set, Edit/Write/NotebookEdit fail on read-only files with a `p4 edit` hint instead of silently overwriting t… │
    │   aaron.alpar@gmail.com's Organization             │ Added Monitor tool for streaming events from background scripts                                                                                               │
    │        ~/projects/wile-workspace/wile-goast        │ /release-notes for more                                                                                                                                       │
    ╰────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯

      Searched for 1 pattern, read 1 file (ctrl+o to expand)

    ⏺ I now have a thorough picture. Let me also quickly check the TODO.md for context on known issues.

      Read 1 file (ctrl+o to expand)

    ⏺ Now I have a comprehensive picture. Here's the assessment.

      ---
      Technical Debt Assessment — wile-goast

      [Priority: High] — prim_restructure.go is the complexity magnet (998 lines, 7-arg functions)

      Where: goast/prim_restructure.go, particularly replaceReturnsInStmtsLabeled (lines 711-817) and rewriteLoopReturns (lines 628-702)

      What: replaceReturnsInStmtsLabeled takes 7 parameters and manually threads state through recursive calls. The SwitchStmt/TypeSwitchStmt/SelectStmt branches (lines 771-809) are a hand-unrolled loop — all three do
      the same thing: check bodyContainsReturn, check loopLabel, call replaceReturnsInClauses, then reconstruct the statement with a new body. The only difference is the output constructor. Same pattern in
      hasReturnInSwitch (lines 559-592). Meanwhile rewriteLoopReturns builds up a parameter list (ctlCounter, labelCounter, resultVarCounter, resultTypes) that screams "this should be a struct."

      Why it matters: Any new case support (e.g., for ... select or deferred returns) requires touching multiple hand-unrolled branches. The 7-parameter threading makes it easy to pass the wrong variable at the wrong
      position. At 998 lines this file is 2x the next largest in the package and the primary growth risk.

      Suggested fix: Extract a loopRewriter struct holding the threaded state (ctlName, retIdx, collected, loopLabel, resultVarCount, resultVarBase). Unify the three switch-like cases with a helper that extracts Body
      *ast.BlockStmt from any branching statement.

      Effort: M

      ---
      [Priority: High] — unmapExprList / unmapStmtList duplicate the generic unmapList

      Where: goast/unmapper.go:210-339

      What: unmapList[T] is a perfectly good generic function (line 210). But unmapExprList (line 242) and unmapStmtList (line 276) hand-unroll the same traversal logic. The only semantic difference: the specialized
      versions filter out nil results (when unmapExpr/unmapStmt returns nil for #f values). The generic version doesn't do this filtering. So three implementations of the same list-walk exist, the generic is used once
      (for unmapStringList), and the specialized ones duplicate ~60 lines of boilerplate each.

      Why it matters: Maintenance inconsistency — a fix to list traversal error handling would need to be applied in three places. The generic was clearly intended to replace the others but the migration was never
      completed.

      Suggested fix: Add a nil-filtering option to unmapList or compose with a post-filter. Replace unmapExprList and unmapStmtList with calls to the generic.

      Effort: S

      ---
      [Priority: Medium] — Test helper duplication across all packages

      Where: goast/prim_goast_test.go:42-58, goastcg/prim_callgraph_test.go:39-54, goastcfg/prim_cfg_test.go:39-54, goastlint/prim_lint_test.go:39-54, goastssa/prim_ssa_test.go:39-54

      What: Every package defines identical eval() and evalExpectError() functions that duplicate testutil.RunScheme / testutil.RunSchemeExpectError. Some tests use the local copy, some use testutil, and some files use
       both within the same package. The local newEngine() differs per package (loading different extensions) so it legitimately needs to be local — but eval/evalExpectError are copy-pasted verbatim.

      Why it matters: Five copies of the same function. If EvalMultiple API changes, five files need updating. Worse: the inconsistent usage (some tests call eval(), some call testutil.RunScheme() in the same file)
      makes it unclear which is canonical.

      Suggested fix: Delete the local eval and evalExpectError from each test file. Use testutil.RunScheme / testutil.RunSchemeExpectError everywhere.

      Effort: S

      ---
      [Priority: Medium] — Dead code: CachedValue / SetCachedValue on GoSession

      Where: goast/session.go:124-136

      What: Both methods are defined (with a mutex-protected cache map) but have zero callers anywhere in the codebase. The comment says "Sub-extensions use this to cache derived data" — but none do. The cache field,
      cacheMu mutex, and both methods are dead weight. Coverage confirms: 0.0%.

      Why it matters: Dead code with a mutex is a signal that the design anticipated a pattern that never materialized. It misleads readers into thinking the cache is live infrastructure they need to preserve.

      Suggested fix: Delete both methods, the cache map field, and the cacheMu mutex. If caching is needed later, add it then — the design will be informed by actual usage.

      Effort: S

      ---
      [Priority: Medium] — run-beliefs returns void, unusable via MCP

      Where: cmd/wile-goast/lib/wile/goast/belief.scm (noted in TODO.md:105-116)

      What: run-beliefs writes results to stdout via display and returns #<void>. The MCP eval tool captures the return value, not stdout. This makes the belief DSL — arguably the project's most valuable user-facing
      feature — unusable from the MCP interface without a fragile open-output-string / parameterize workaround.

      Why it matters: The primary consumer of this tool is an LLM agent via MCP. If the core feature doesn't work through the core interface, that's a functional gap. The workaround requires Scheme I/O plumbing
      knowledge that an LLM shouldn't need.

      Suggested fix: Return a structured alist from run-beliefs (belief name, adherence ratio, site count, deviation list). Optionally print when called interactively.

      Effort: M

      ---
      [Priority: Medium] — nf returns #f for missing fields (silent propagation)

      Where: cmd/wile-goast/lib/wile/goast/utils.scm:8-11, documented in TODO.md:131-154

      What: nf is the universal field accessor for tagged alists. It returns #f for missing fields. Since #f is a valid Scheme value, it propagates silently through arithmetic, list operations, and string operations
      until something crashes far from the source. Every nf consumer must check. Hundreds of call sites.

      Why it matters: This is documented as a known issue. It's the same class of bug as null propagation — errors surface far from their cause, making debugging difficult. Any new Scheme code built on nf inherits this
       risk.

      Suggested fix: Add nf! (error on missing) and use it at boundaries where fields are structurally required.

      Effort: S

      ---
      [Priority: Low] — chanDirSymbol panics on unknown input

      Where: goast/mapper.go:681

      What: Only panic in production code (the other one is in a test). chanDirSymbol panics on an unknown ast.ChanDir value. Since ChanDir has only three valid values and go/ast guarantees them, this is theoretically
      unreachable — but a panic is the wrong signal for a library. Every other mapper function returns errors.

      Why it matters: Inconsistency with the rest of the error handling. If a future Go version adds a channel direction (unlikely but possible), this would crash the process instead of returning an error.

      Suggested fix: Return (values.Value, error) like everything else and propagate up.

      Effort: S

      ---
      [Priority: Low] — cloneTypeExpr at 0% coverage

      Where: goast/prim_restructure.go:959-986

      What: cloneTypeExpr handles 11 type expression variants but has zero test coverage. It's called only from expandResultTypes when a result field has multiple names (e.g., (x, y int)) — a case that apparently isn't
       exercised by existing tests.

      Why it matters: If cloning is wrong for any variant (e.g., the FuncType case does a shallow copy of Params/Results, which shares ast.Field pointers), the bug would only surface when formatting multi-named result
      types after loop return rewriting.

      Suggested fix: Add a test for go-cfg-to-structured with a function returning (x, y int, err error).

      Effort: S

      ---
      [Priority: Low] — goast/register.go doesn't wrap Extension

      Where: goast/register.go:23 vs all other register.go files

      What: Base goast uses var Extension = registry.NewExtension(...) directly, while all sub-extensions wrap with a *Extension struct implementing LibraryNamer. This is actually correct by design (goast is the base
      domain, not a subdomain), but undocumented. A reader looking at the four sub-extensions might "fix" goast to match.

      Why it matters: Low risk, but worth a one-line comment.

      Suggested fix: Add a comment on line 23: // No LibraryNamer wrapper — goast is the base domain.

      Effort: S

      ---
      State of the Code

      This is a well-structured codebase for its size (~17k lines Go, ~2.4k lines Scheme). The five-package architecture mirrors the compiler IR layers cleanly, error handling is disciplined (sentinel + wrap pattern,
      zero fmt.Errorf in production), and the bidirectional mapper/unmapper design is the core abstraction that makes everything work. Coverage sits at 81% with no glaring holes outside the noted cloneTypeExpr. The
      main debt is concentrated: prim_restructure.go is the complexity magnet where hand-unrolled patterns and parameter threading make future changes fragile. The rest is minor cleanup.

      Top 3 to tackle first:

      1. prim_restructure.go parameter threading — this is where the next bug will come from. Extracting a struct reduces the 7-arg threading and makes the hand-unrolled switch/typeswitch/select pattern visible as a
      loop.
      2. run-beliefs return value — this blocks the primary use case (MCP agents). It's not complex to fix, and it unblocks the tool's core value proposition.
      3. Test helper consolidation — five copies of eval() is the kind of thing that stays forever if you don't fix it now. Quick win, eliminates a class of future maintenance burden.

      ★ Insight ─────────────────────────────────────
      Two patterns worth noting from this assessment:

      Hand-unrolled patterns as debt indicators. The SwitchStmt/TypeSwitchStmt/SelectStmt triplication in replaceReturnsInStmtsLabeled is a textbook case. These three types all have a Body *ast.BlockStmt field, and the
       code does the same check-call-reconstruct sequence for each. The "loop" is the type dispatch itself — if you extract bodyFromBranchingStmt(stmt) (*ast.BlockStmt, bool) and reconstructBranchingStmt(stmt,
      newBody), the three cases collapse to one. This is the kind of thing wile-goast itself could detect: AST diff of the three branches would show near-identical structure with only the constructor differing.

      Dead code with infrastructure (the cache mutex). The CachedValue/SetCachedValue pattern is interesting because it's not just dead code — it's dead infrastructure (a mutex, a map, initialization in the
      constructor). Dead code is cheap to delete. Dead infrastructure makes people afraid to delete because "maybe something needs it." The answer: git history preserves it. Delete it, and if someone needs session
      caching later, they'll build it for the actual use case rather than guessing at an interface.
      ─────────────────────────────────────────────────

    ✻ Brewed for 6m 31s

    ❯ is the ruleguard tools from from Wile copied into wile-goast?

    ⏺ Let me check what exists.

      Searched for 4 patterns, read 3 files, listed 1 directory (ctrl+o to expand)

    ⏺ No. wile-goast has no .golangci.yml and no ruleguard/ directory. The ruleguard rules from wile haven't been copied over.

      Wile's ruleguard/rules.go enforces four project conventions:

      1. noCompoundIf — no if init; cond {} (separate init from condition)
      2. noErrorsNew — no errors.New in production code (use werr.*)
      3. noFmtErrorf — no fmt.Errorf in production code (use werr.*)
      4. noBareSentinelPanic — no panic(werr.ErrSomething) without wrapping

      wile-goast runs golangci-lint run ./... but with no config file — just the defaults. So none of those rules apply here. The error handling conventions happen to be followed (I checked — zero fmt.Errorf or
      errors.New in production code), but there's no lint enforcing it. The compound-if rule is not checked at all.

      ★ Insight ─────────────────────────────────────
      The question of "copy the file" vs "share it" is a real design choice here. These rules reference werr types specific to the wile ecosystem, so they're not generic — they'd apply directly to wile-goast. Three
      options, increasing in sophistication:

      1. Copy ruleguard/rules.go + .golangci.yml — works, but the rules drift. If wile adds a fifth rule, wile-goast doesn't get it.
      2. Symlink or git submodule — ties them together but adds workspace coupling.
      3. Extract a shared wile-lint-rules module — both repos import it. Overkill at zero consumers.

      At this project stage (v0.5.x, zero consumers, same author for both repos), copying is the pragmatic choice. The drift risk is low because you're the only consumer, and the rules are small enough to eyeball.
      ─────────────────────────────────────────────────

## Staff-Engineer Findings 2026-04-19 — Next to Address

All items below must be fixed. Priority reflects ordering, not optionality.
Effort tags: S (hours), M (day), L (multi-day).

### High

- [x] **Session/pattern dispatch duplicated 4×** — Added
      `goast.DispatchSessionOrPattern(arg, primName, sessionFn, patternFn) error`
      in `goast/session.go`. Replaced dispatch blocks in `goastssa/prim_ssa.go`,
      `goastcfg/prim_cfg.go`, `goastcg/prim_callgraph.go`, `goastlint/prim_lint.go`.
      Closure-per-site carries the extra args (rest/funcName/algo/analyzers).
      TDD: three tests in `goast/session_test.go`. **[S]** — done 2026-04-19

- [ ] **`belief.scm` god-object (1018 lines)** — Extract `(wile goast belief-checkers)`
      library containing property checkers (`paired-with`, `ordered`, `contains-call`,
      `stores-to-fields`, `checked-before-use`, `custom`) + SSA helpers at
      `cmd/wile-goast/lib/wile/goast/belief.scm:571-755`. Leave registry, selectors,
      runner, and emit in `belief.scm`. **[M]**

- [x] **`filter` reimplemented in 4 Scheme libraries** — Added `filter` to
      `utils.scm` (named-let version, correct for #f elements) + exported from
      `utils.sld`. Deleted local copies at `domains.scm:23`, `split.scm:20`,
      `fca-recommend.scm:24`, `unify.scm:33`. Find/position were not duplicated.
      **[S]** — done 2026-04-19

- [ ] **AST tag addition touches 6 files** — Convert mapper/unmapper dispatch in
      `goast/mapper.go` (852 lines) + `goast/unmapper.go` + `unmapper_{decl,stmt,expr,
      types,comments}.go` to a registration table keyed by AST tag
      (`map[reflect.Type]mapperFn`), collocating forward + reverse mapping per
      node type. Reduces per-tag extension cost from 6 files to 1. **[L]**

- [ ] **10 of 12 embedded Scheme libraries have no integration test** — Missing
      dedicated coverage: `boolean-simplify.scm`, `domains.scm`, `fca.scm`,
      `fca-algebra.scm`, `fca-recommend.scm`, `path-algebra.scm`, `split.scm`,
      `ssa-normalize.scm`, `unify.scm`, `utils.scm`. Only `belief.scm` and
      `dataflow.scm` are exercised (via `goast/belief_integration_test.go`). Add
      one 10–20 line smoke test per library exercising a public entry point.
      Do before next wile version bump. **[M]**

### Medium

- [x] **`register.go` LibraryNamer divergence** — Confirmed correct by design
      and documented. `registry.NewExtension("goast", ...)` gets library name
      `(wile goast)` from the default `(wile <ext-name>)` rule in
      `wile/registry/extension.go:27`. Sub-extensions need the wrapper because
      `goast-ssa` would render as `(wile goast-ssa)` not `(wile goast ssa)`.
      Added multi-line comment in `goast/register.go` explaining the rationale.
      **[S]** — done 2026-04-19

- [ ] **`mapper.go` idiom divergence (free fns vs receiver methods)** —
      `goast/mapper.go` uses free functions with `*mapperOpts` struct-passing;
      `goastssa/mapper.go`, `goastcfg/mapper.go`, `goastcg/mapper.go` use receiver
      methods on `*ssaMapper` / `*cfgMapper` / `*cgMapper`. Converge on
      receiver-method style across all four. **[M]**

- [ ] **FCA concept lattice scaling wall (2^|attributes|)** —
      `cmd/wile-goast/lib/wile/goast/fca.scm` (`concept-lattice` via NextClosure)
      enumerates all formal concepts up front. Worst case explodes at >100
      functions × >50 dependency attributes. Consumers: `recommend-split`,
      `single-cluster` aggregate belief. Add bounded variant with `'max-concepts`
      and `'min-extent` keyword arguments; keep exhaustive as default. **[M]**

- [ ] **Hardcoded limits buried in library code** — Expose as keyword arguments
      with documented defaults:
      `cmd/wile-goast/lib/wile/goast/split.scm:95,363` (IDF threshold 0.36 —
      tunable via keyword but default undocumented);
      `cmd/wile-goast/scripts/unify-detect-pkg.scm:547` (similarity 0.60 — not
      tunable, requires source edit);
      `cmd/wile-goast/lib/wile/goast/belief.scm:735` (fuel=5 in
      `checked-before-use` — silently caps reachability). **[S]**

- [ ] **Error-path test gaps in sub-extensions** — Coverage headlines hide
      untested error branches: `goastssa` 67% test/src ratio, `goastlint` 59%,
      `goastcfg` 74%. Primary offender: `goastssa/prim_canonicalize.go` has 10+
      `werr.WrapForeignErrorf` sites with no error-case tests. Add table-driven
      `TestPrim<X>Errors` per package covering documented sentinels. **[S]**

- [ ] **Error sentinel naming inconsistency** — Prefix and suffix drift across
      packages: `errGo*` / `errCFG*` / `errCG*` / `errLint*`, some with `Error`
      suffix, some without. Sites: `goast/helpers.go:24-29`,
      `goastcfg/prim_cfg.go:30-31`, `goastcg/prim_callgraph.go:45-48`,
      `goastlint/prim_lint.go:~55-70`. Pick `err<Pkg><Category>` (no `Error`
      suffix — type name carries it), apply repo-wide, add a ruleguard rule
      once `ruleguard/rules.go` is wired to CI (see prior staff-engineer finding
      above on ruleguard integration). **[S]**

### Low

- [x] **FCA triad import ambiguity** — Verified: works as designed.
      `fca.sld` acts as a facade, explicitly re-exporting 11 symbols from
      `(wile algebra fca)` (see `;; Re-exported from (wile algebra fca)` comment
      on line 17). `fca-algebra.sld` imports `(wile goast fca)` and transitively
      picks up `concept-relationship`, `concept-extent`, etc. All three
      `TestFCAAlgebra_*` test suites pass. The audit agent missed the facade
      comment. No fix required. **[S]** — verified 2026-04-19

- [x] **`path-algebra.scm` exports unused internally** — Verified: exports are
      `make-path-analysis`, `path-analysis?`, `path-query`, `path-query-all`
      (thin wrapper over `(wile algebra graph)`). `goast/path_algebra_test.go`
      (309 lines) provides direct coverage. Documented in
      `cmd/wile-goast/prompts/goast-scheme-ref.md:151`. Role is public-facing
      Scheme API, not internal utility. No action needed.
      **[S]** — verified 2026-04-19

- [ ] **`belief_integration_test.go` lacks parameterization** — 1986 lines, 49
      test functions, each re-initializes a Wile engine and reloads packages.
      Convert to `t.Run` subtests grouped by belief category with shared
      engine setup where safe. Reduces wall time and maintenance burden. **[M]**

