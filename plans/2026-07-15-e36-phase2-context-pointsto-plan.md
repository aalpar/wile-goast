# E36 Phase 2: on-demand context-sensitive points-to — full plan + resume point

> **Purpose of this doc:** a self-contained resume point after a context clear. A fresh session
> should be able to read THIS file (plus, optionally, notebook entries E36-E39 and the design doc)
> and continue Phase 2 with no other context. Phase 1 is shipped and committed. **Start at Task 0**
> (the flood-taxonomy experiment) — it is a GATE that picks the validation target before any
> object-model code is written.

---

## 0. How to resume (fresh session)

1. Read this file end to end.
2. Read notebook entries **E36-E39** in `notebooks/2026-07-14-pre-composition.md` (the demand-engine
   arc and why the refinement line is closed) and the **Phase 2 design** at
   `plans/2026-07-14-e36-rsm-pointsto-design.md:500-609`.
3. Execute **Task 0** (flood-taxonomy experiment). It is a gate: its result picks/confirms the
   Phase 2 validation target and confirms Phase 2 is validatable. Do NOT write object-model code
   before Task 0 reports.
4. Recommended execution model: **superpowers:subagent-driven-development** (fresh subagent per task,
   review after each, PROBE + the collapse tripwire as gates), same as Phase 1. The user steers at
   decision points (see §3); surface genuine design forks, don't guess.

Engine dir: `notebooks/2026-07-14-pre-composition.d/E36-rsm/`. **All build/run: `GOWORK=off`**
(module `csize`, not in any go.work). Build: `GOWORK=off go build -o rsm .`. Gates:
`./run-corpora.sh ./rsm | tail` (EQUIV/DISPATCH/PROBE 126/126) and
`GOWORK=off ./rsm testdata/soundfix` (EQUIV/DISPATCH/PROBE/engine-footprint).

---

## 1. Current state (Phase 1 SHIPPED, HEAD `b42e5d5`)

The v2 RSM demand-driven, field-sensitive, context-INsensitive points-to engine
(`E36-rsm/rsm.go`) computes points-to and dispatch that equal batch Andersen when co-seeded, and
is single-seed-sound except a small intrinsic residual. Verified gates:
- **EQUIV** (demand == batch on all nodes): 126/126 corpora + soundfix (89,303 nodes).
- **DISPATCH** (per-receiver == batch): 513/513 soundfix + 126/126 corpora.
- **PROBE** (isolated single-seed query == batch — the gate that catches demand-correctness bugs
  the co-seeded EQUIV/DISPATCH are blind to): 0 on all 126 corpora + fixtures' own receivers;
  **4/263 on soundfix** (deterministic; context-collapsed merged runtime objects).
- **TERMINATION**: OK (pushes ~2.25e5 ≪ bound 2.19e9).
- **Footprint**: engine `|touched|` per query is near-optimal on clean code (clean corpus optimal
  slice median 4 vs engine footprint median 6). On soundfix it is median **22,791** vs optimal 6
  — the runtime god-graph floor (see §2).

Frozen equivalence oracle (byte-identical to E35, do NOT edit in Phase 2): `extract.go`, `solve.go`;
`slice.go` (only the `buildFeeds` receiver retyped). The `== batch` EQUIV theorem is Phase 1's
oracle and does NOT apply to Phase 2 (see §6 validation).

### Commit arc (all on master, `1f08d30..b42e5d5`)
- `ce5de57`,`cea3ee6` Task 0 scaffold + substrate. `4ebb198` Task 1 semi-naive engine.
- `1bb59a1` Task 2 forward object-tracking + gated-rule reactivation (DISPATCH 7→0).
- `c3ca6c9`,`c8e25e6`,`7ad6f4f`,`8fb0150` Task 3 (signature filter; name+alias-independent sigKey via
  `typeutil.Hasher`; receiver-type-scoped invoke discovery; isolated single-seed PROBE gate added;
  reset() confirmed complete; unsound Step-5 fnAddrTaken reverted).
- `2773c18` Task 4 cyclic fixtures + push tripwire. `b2a982e` Task 5 deep-chain fixture.
- `7384fde` Task 6 notebook E36. `e424dcd`+`4b9d89c` E37 (correct footprint claim + artifact).
- `0acdade`+`8799751` E38 (forward-contents refinement refuted). `295a529`+`b42e5d5` E39
  (resolve-then-prune refinement refuted).

### Notebook entries (`notebooks/2026-07-14-pre-composition.md`)
- **E36** shipped the engine; the PROBE gate caught 3 latent single-seed bugs the co-seeded gates hid
  (gated-rule re-entrancy, sigKey name+alias, fnAddrTaken).
- **E37** Corrects E36's footprint claim: not a genuine flood, it is over-activation; also fixes
  misremembered merged-object figures.
- **E38** forward-CONTENTS refinement (`REFINE=1`) refuted — floods worse (`demandContents` demanded
  the token's contents = god-graph).
- **E39** forward-FLOW resolve-then-prune (`REFINE=prune`) ALSO refuted — floods worse (22,791→26,538),
  drops a caller. `PRUNE_NOHEAP=1` gives the same footprint ⇒ the cost is forward COPY-FLOW BREADTH of
  a SHARED dispatch token, not the heap. **Both forward-refinement directions are closed.**
- Committed diagnostics: `E36-rsm/E36-rsm-diagnostic.txt`, `E37-probe-detail.txt`,
  `E38-refine-diagnostic.txt`, `E39-prune-diagnostic.txt`.

### The `REFINE` prototype (committed, flag-gated, DEFAULT OFF — do not ship, do not re-try)
`rsm.go` carries `REFINE=1` (forward-contents, E38) and `REFINE=prune`/`PRUNE_NOHEAP=1` (forward-flow,
E39), both refuted. Default (flags unset) is byte-behavior-identical to the shipped engine. The
refinement line is CLOSED: a dispatch token is SHARED, so forward-from-token is broad while
backward-from-query is targeted; the shipped backward engine is the tightest context-INsensitive
option. **Do not re-attempt forward refinement.** The only lever left for the footprint floor is
context sensitivity = this Phase 2.

---

## 2. The two Phase-1 residuals Phase 2 targets

1. **Footprint floor** on stdlib-heavy targets: soundfix engine footprint median 22,791 vs optimal 6.
   This is the runtime god-graph reached via UNRESOLVED type-compatible dispatch candidates (the
   optimal slice uses batch's RESOLVED edges). Intrinsic to context-insensitive analysis; only
   context (splitting the merged object) can lower it.
2. **4/263 soundfix single-seed misses** (PROBE): receivers n49048/n49074/n81355/n81362 miss 80/6/80/80
   objects of runtime/os/unsafe type — context-collapsed merged runtime objects that need per-context
   heap cloning to reconstruct on a lone query.

---

## 3. Decisions already made this session (do not relitigate)

- Pursue full single-seed soundness → drove the sigKey fix + PROBE gate to the Phase-1 ceiling (4/263).
- Correct E36's footprint overstatement in E37 (append-only Corrects, not a rewrite).
- Prototype the refinement (E38) → refuted. Then resolve-then-prune (E39) → refuted. Refinement closed.
- **Pursue Phase 2 (context sensitivity), and DIAGNOSE THE FLOOD TAXONOMY FIRST** (this plan's Task 0)
  — because soundfix's god object is a global singleton, not the errorString merge Phase 2 is designed
  for (see §4).

---

## 4. THE KEY NUANCE that makes Task 0 necessary (do not skip)

The design's Phase 2 (`2026-07-14-e36-rsm-pointsto-design.md:500-609`) splits the **errorString
MULTI-ALLOCATION merge**: context-insensitive Andersen merges every `errors.New`/`fmt.Errorf`
allocation into one abstract `*errors.errorString`; per-context heap cloning gives each site its own
object, fragmenting the union (E33/E34; wile's god object was `*errors.errorString`).

BUT **soundfix's god object is `runtime.mheap`** — the Go GLOBAL `runtime.mheap_`, ONE object with no
allocation context. Heap cloning splits objects *per allocation context*; **it cannot split a global
singleton.** Its 4,781 holders are genuine references to one global, not a mergeable union. So Phase 2
as designed likely does NOT help soundfix's #1 flood; its designed win lands on errorString-heavy
targets (wile/etcd), and wile is where the batch footprint loop OOM'd earlier. Building Phase 2 before
confirming a validatable target risks a large object-model change against an unhelpable flood. Hence
Task 0.

---

## 5. Task 0 — THE FLOOD-TAXONOMY EXPERIMENT (do FIRST; it is a GATE)

**Goal:** determine with data which god objects are cloneable (multi-allocation merges = Phase 2's
target) vs uncloneable (global singletons), and pick/confirm a validation target with a genuine
multi-allocation merge.

**Method (cheap; extend the batch solver's existing GOD OBJECT dump in `main.go`/`solve.go`):**
- For soundfix, and for at least one real errorString-heavy target (wile/etcd if the batch solve can
  run — it may need a longer budget or only the extraction+partial-solve; else a candidate found by
  grepping for heavy `errors.New`/`fmt.Errorf` use), dump the **top-K god objects by holder count**,
  each with: `objType`, holder count, and crucially whether it is a **GLOBAL** (one object,
  uncloneable) or an **allocation-site** object (cloneable). The extractor distinguishes these at
  object creation (`extract.go` ~134, `obj(key,t)`); expose the key origin (global var vs
  `*ssa.Alloc`/`MakeInterface`/`New` site).
- Answer three questions:
  1. Is soundfix's flood ONLY `runtime.mheap` (a global, Phase 2 can't help), or does soundfix ALSO
     have a cloneable errorString-style multi-allocation merge with many holders?
  2. On a real errorString-heavy target, is the god object a genuine multi-allocation merge (many
     `errors.New` sites collapsed into one object)? Confirm the E33/E34 premise on the current engine.
  3. **Pick the Phase 2 validation target:** a program that (a) converges under batch solve (so
     `⊔_c pts_cs == pts_ci` is checkable) AND (b) has a real multi-allocation merge god object that
     cloning splits. Candidates: a real target (wile/etcd) if it converges; else a **synthetic
     errorString-heavy fixture** (N `errors.New` sites → shared interface dispatch; converges + is
     cloneable by construction) built as a new `testdata/` target.

**STOP-and-report gate:** Task 0's output picks the target and confirms Phase 2 is validatable. If NO
cloneable target is available/buildable, reassess whether Phase 2 is worth the build before writing
object-model tasks. Record the taxonomy as **notebook entry E40**.

---

## 6. Phase 2 design (from `2026-07-14-e36-rsm-pointsto-design.md:500-609`)

- **Context model:** k-bounded call-string (k=1-2) with SCC-collapse for recursion (Sridharan-Bodik
  PLDI06 §recursion: relabel param/return edges within a call-graph SCC as plain copies). NOT SPDS —
  E31 already flattened the field axis to `base+off` set-membership, so there is no field-PDS to
  synchronize; only the call/return stack needs a PDS (decidable, single stack).
- **Heap cloning:** `obj(key, t)` → `obj(key, t, ctx)`. Cloned lazily per `(alloc-site, context-suffix)`,
  only for contexts the query traverses. The merged god object splits into per-context slivers; a
  domain site that merely grazed one shared error object no longer pulls the whole union.
- **Query-scoped context introduction:** contexts introduced only along the query's backward call
  chain (Sridharan-Bodik Fig 7 `FINDPOINTSTO(x, c, visited)`); push the call-site onto ctx at each
  `wire` boundary (bounded by k / SCC-collapsed in recursion). Context threads through exactly `wire`
  and the dispatch resolution `fireCall` — the two sites Phase 1 already localizes. A domain query that
  never enters `errors.New` clones nothing there; footprint stays Phase-1-sized.
- **Validation — SOUNDNESS, not `== batch`** (the EQUIV theorem does NOT apply, since `pts_cs ⊆ pts_ci`):
  1. **Soundness-collapse tripwire:** `⊔_c pts_cs(v, c) == pts_ci(v)` for every v (merging all contexts
     recovers Phase 1 exactly — the context-sensitive EQUIV analogue; a mechanical check that
     refinement dropped no real edge / callee). Whole-program on corpora + the Task-0 target; may be
     sampled on wile/etcd.
  2. **Precision gain:** on the error-adjacent receivers, p90 slice size + the count of receivers whose
     slice reaches the god object drop materially (the flood splits per context); the median (already
     ~6-11) must NOT regress.
  3. **PROBE carries forward:** isolated single-seed correctness stays a gate; Phase 2 should also drive
     the soundfix 4/263 residual toward 0 IF those merged objects are cloneable (Task 0 tells us).
- **Central unproven Phase-2 bet:** whether small-k demand context shrinks the errorString tail WITHOUT
  exploding the cloned object domain. `k` small (1-2); SCC-collapse aggressive. E34 established that
  ONLY context removes the flood but did not measure the cost; this is the bet.

---

## 7. Provisional task breakdown (finalize the full breakdown AFTER Task 0 picks the target)

- **T0** flood-taxonomy experiment + target selection (§5) — GATE. → notebook E40.
- **T1** object model `obj(key,t)` → `obj(key,t,ctx)`; ctx representation (k-bounded call-string).
  **Likely needs a NEW extractor/solver/engine variant (a Phase-2 dir), NOT edits to the frozen
  Phase-1 `extract.go`/`solve.go`/`slice.go`** — the Phase-1 files are the `== batch` oracle and must
  stay intact for the collapse tripwire to compare against. Decide the exact split in T1.
- **T2** context threading through `wire` + `fireCall`; SCC-collapse in the on-demand call graph.
- **T3** query-scoped lazy context introduction + per-`(site,ctx)` heap cloning.
- **T4** the `⊔_c pts_cs == pts_ci` soundness-collapse gate (the new oracle; replaces EQUIV).
- **T5** precision measurement on the Task-0 target (tail shrink; median non-regression; PROBE toward 0).
- **T6** notebook entry (Phase 2 result: does small-k context shrink the tail without domain explosion?).
- Each task independently gated on the collapse tripwire + PROBE, the way Phase 1 tasks gated on EQUIV.

---

## 8. Constraints & gotchas (carried from Phase 1)

- **Frozen Phase-1 oracle:** `extract.go`/`solve.go` byte-identical to E35; `slice.go` body frozen.
  Phase 2 changes the object model → work in a NEW variant, keep Phase-1 intact as the collapse
  reference. No HVN, no cycle elimination (speaks to the algebraic bet).
- **Build/run:** `GOWORK=off` always. Gates: `run-corpora.sh` (EQUIV/DISPATCH/PROBE 126/126) + soundfix.
- **VERSION does not bump** on commit (retired hook). Notebook is committed; plans/ is gitignored →
  new plan files need `git add -f`.
- **Determinism:** the engine uses `map[int]bool` throughout; do NOT force sorted iteration to mask
  nondeterminism (it hides completeness gaps — a Phase-1 lesson). A complete analysis is order-
  independent by construction; PROBE `extra=0` is the corroborating signal.
- **Housekeeping:** the engine dir has two stray untracked build binaries (`rsm`, `csize`); harmless,
  uncommitted. Consider a `.gitignore` for them. Pre-existing untracked/modified non-E36 files
  (`plans/2026-07-14-cflreach-demand-pointsto-design.md` modified, `notes.txt`) are NOT ours — leave.

## 9. Phase-1 rolled-up Minors (from the SDD ledger; none block Phase 2, note for later)
- `main.go` DISPATCH/PROBE sample-type dump uses map-iteration order (diagnostic-only, env-gated).
- `rsm.go` `holdersOfBase[b]` keeps dead appends after its gate opens (perf-only; guard `if !fwdDone[b]`).
- `rsm.go` invoke-narrowing fallback branches (`rt==nil`, `!IsInterface`) are dead code given invariants.
- The `testdata/invokestress` corpus doesn't cover embedded/promoted-method receivers (verified externally).

## 10. Pointers
- Phase 1 impl plan (shipped): `plans/2026-07-14-e36-rsm-pointsto-impl.md`.
- Design (Phase 1 + Phase 2 outline): `plans/2026-07-14-e36-rsm-pointsto-design.md`.
- Notebook: `notebooks/2026-07-14-pre-composition.md` (thesis = pre-composition; this line serves E26).
- SDD scratch ledger (recovery detail, not authoritative): `.superpowers/sdd/progress.md`.
