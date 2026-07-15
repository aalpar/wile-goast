# E36 RSM demand-driven points-to — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `E35-cflreach/cfl.go` (v1 demand engine) into `E36-rsm/rsm.go`, a demand-driven field-sensitive context-insensitive points-to engine that fixes v1's two defects — the uniform-flood footprint and the 7/513 multi-hop-heap DISPATCH gaps — while preserving `pt == batch` by construction.

**Architecture:** A *policy* diff, not an *arithmetic* diff. Every `pts`-writing rule stays character-identical to a `solve.go` primitive (`add`/`addEdge`/`wire`); the only new byte is `emit`'s `!active[p]` gate. So the three semantic changes are independently gated: (1) swap the propagation engine to `solve.go`'s semi-naive `delta`/`doneComplex` discipline with activation policy *frozen* — EQUIV stays 126/126; (2) replace v1's whole-cone `forwardActivateObject` writer-finding with forward object-tracking (`demandContents` + `storesWhereValue` field-push + containment-tracking + load-out-gate fix) — DISPATCH closes 7→0 and the footprint collapses; (3) replace program-wide icall caller-discovery with a signature-filtered version — the demand-leak corpus stays domain-sized. Then harden termination and multi-hop depth with fixtures.

**Tech Stack:** Go 1.24, module `csize` (standalone, `GOWORK=off` mandatory), `golang.org/x/tools/go/ssa` + `go/types`. No new deps. This is notebook experiment code under `notebooks/2026-07-14-pre-composition.d/E36-rsm/`, outside the wile-goast library's coverage/lint gates.

## Global Constraints

- **Module & build:** `csize`, Go 1.24. Build and run **only** with `GOWORK=off` (the notebook module is not in any go.work). Build: `GOWORK=off go build -o rsm .` from inside `E36-rsm/`.
- **Frozen substrate — never edit:** `extract.go`, `solve.go` must stay byte-identical to E35 (they are the equivalence oracle; changing them invalidates every gate). `slice.go` body stays byte-identical; only its `buildFeeds` receiver type is retyped `*demand`→`*rsm` (Task 0).
- **Propagation arithmetic is character-identical to `solve.go`.** Every rule that writes `pts` (`emit`, `relay`, `subscribe`, writer-harvest, gep, `wire`, `fireCall`, `fireAssert`) must mirror a pinned `solve.go` site. The *only* deviation from batch is `emit`'s `!active[p]` demand gate. Getting the receiver-object-into-param0 quirk (`solve.go:154`, not a copy edge) or icall's no-injection (`solve.go:235`) wrong breaks `pt == batch`.
- **No HVN, no cycle elimination.** The design bars both so the result speaks to the algebraic bet, not solver engineering. Do not add them.
- **The EQUIV theorem is the test oracle.** Context-insensitive field-sensitive CFL-reachability over E31's flat `base+off` object model *is* Andersen. Any faithfulness bug under-approximates, so EQUIV (`demand == batch` on all nodes) fires. Keep EQUIV green after every task.
- **Paths (absolute, used verbatim in commands below):**
  - Engine dir: `/Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm`
  - v1 diff base: `/Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E35-cflreach/cfl.go`
  - 126 corpora: `$HOME/ClaudeProjects/LLMAccuracy/code-restructuring/packages_iface_decoy` (dirs `ifdecoy-*`)
  - soundfix (ephemeral, snapshot in Task 0): `/private/tmp/claude-501/-Users-aalpar-projects-wile-workspace-wile-goast/83c4587d-97da-4fe1-9d07-be39f684dd07/scratchpad/soundfix`

## Design source

Full spec: [`plans/2026-07-14-e36-rsm-pointsto-design.md`](2026-07-14-e36-rsm-pointsto-design.md). Section references below (§1–§11) are to that document. Every line reference in the design (`cfl.go:NN`, `solve.go:NN`, `slice.go:NN`) was verified accurate against the E35 substrate before this plan was written.

---

## File Structure

All files live in `E36-rsm/` (module `csize`, `package main`).

| File | Status | Responsibility |
|------|--------|----------------|
| `extract.go` | **frozen** (present) | E31 extractor: `builder`, `constraint`, 8 forms, object model. Untouched. |
| `solve.go` | **frozen** (present) | E31 batch difference-propagation solver = the equivalence oracle. Untouched. |
| `slice.go` | **1-token retype** (present) | Optimal-footprint backward slice (`buildFeeds`, `sliceSize`). Only the `buildFeeds` receiver becomes `*rsm`. |
| `rsm.go` | **create** (Task 0, then edited by Tasks 1–3) | The demand engine. Copied from `cfl.go`, `demand`→`rsm`. This is the whole variable surface. |
| `main.go` | **create + instrument** (Task 0, +Task 2) | Driver: extract → batch → EQUIV + DISPATCH + FOOTPRINT. Copied from E35; `newDemand`→`newRSM`; DISPATCH diff-dump + engine-footprint added. |
| `run-corpora.sh` | **create** (Task 0) | 126-corpus gate runner: loops `ifdecoy-*`, greps `EQUIV: PASS` / `DISPATCH: PASS`. |
| `testdata/soundfix/` | **snapshot** (Task 0) | Stable copy of the ephemeral soundfix target (8K source, 153k SSA nodes). |
| `testdata/cyclic/{selffield,twonode,mutual}/` | **create** (Task 4) | Three cyclic-heap EQUIV regression fixtures (§7). |
| `testdata/deepchain/` | **create** (Task 5) | ≥3-hop store→load→store DISPATCH fixture (open-risk: multi-hop beyond the known 2-hop 7). |
| `testdata/demandleak/` | **create** (Task 3) | Adversarial demand-leak corpus (§10): domain query must not pull errorString. |

**Naming carried across tasks (single source of truth):**

- Struct `rsm`; constructor `func newRSM(b *builder, prog *ssa.Program) *rsm`.
- Core writers: `func (d *rsm) emit(p, o int)` (was `add`), `func (d *rsm) subscribe(src, dst int)` (dynamic subset edge), `func (d *rsm) relay(src, dst, o int)` (forward-relay one object, `markActive`-on-tracked).
- Activation: `func (d *rsm) markActive(v int)` (relay-only), `func (d *rsm) activate(v int)` (relay + backward feeders). `demandPT(x)` in the design **is** `activate(x)` — use `activate`.
- Heap: `func (d *rsm) loadPop(o, dst int)`, `func (d *rsm) demandContents(o int)`, `func (d *rsm) baseOf(o int) int`.
- Fields added over v1: `pts, delta, doneComplex []map[int]bool`; `active, ptDone, fwdDone []bool` (`ptDone` replaces v1 `bwDone`; `fwdDone` replaces v1 `fwdVisited`, and is keyed per **base**); `storesWhereValue [][]int`; `icallRecvsBySig map[string][]int`; `pushes, nDemands int64`.
- Signature key: `func sigKey(sig *types.Signature) string` = `types.TypeString(sig, nil)`.

---

## Task 0: Scaffold the diff base (v1, renamed, building, green)

Bring v1 into `E36-rsm` unchanged in behavior, renamed `demand`→`rsm`, and establish the baseline the three semantic tasks improve on: EQUIV 126/126, DISPATCH 126/126, soundfix EQUIV PASS + DISPATCH **FAIL (7)**, footprint uniform. No semantic change here — a green build and a recorded baseline are the deliverable.

**Files:**
- Create: `E36-rsm/rsm.go` (from `E35-cflreach/cfl.go`, symbols renamed)
- Create: `E36-rsm/main.go` (from `E35-cflreach/main.go`, `newDemand`→`newRSM`, DISPATCH diff-dump added)
- Modify: `E36-rsm/slice.go:15` (`func (d *demand) buildFeeds` → `func (d *rsm) buildFeeds`)
- Create: `E36-rsm/run-corpora.sh`
- Snapshot: `E36-rsm/testdata/soundfix/` (copy of the ephemeral target)

**Interfaces:**
- Produces: `type rsm`, `newRSM`, and every v1 method under the new receiver — consumed by all later tasks and by `main.go`/`slice.go`.

- [ ] **Step 1: Snapshot the soundfix target before it evaporates**

`soundfix` is 8K of source (a tiny `main.go` whose imports expand to 153k SSA nodes) sitting in an ephemeral `/private/tmp` scratchpad. Copy it into the notebook so the DISPATCH gate survives temp cleanup.

Run:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
mkdir -p testdata
cp -R "/private/tmp/claude-501/-Users-aalpar-projects-wile-workspace-wile-goast/83c4587d-97da-4fe1-9d07-be39f684dd07/scratchpad/soundfix" testdata/soundfix
ls testdata/soundfix
```
Expected: `go.mod  main.go`.

If the source path is already gone, STOP and ask: soundfix must be regenerated (it is the DISPATCH 513 gate and the acceptance test for §5). Do not proceed to Tasks 2/5 without it.

- [ ] **Step 2: Create `rsm.go` from `cfl.go` with the struct rename**

Copy `E35-cflreach/cfl.go` to `E36-rsm/rsm.go`, then rename exactly these symbols (leave all logic identical):
- `type demand struct` → `type rsm struct`
- `func newDemand(` → `func newRSM(`
- every `(d *demand)` receiver → `(d *rsm)`
- the local `d := &demand{` in the constructor → `d := &rsm{`

Run (mechanical; the constructor line carries both `newDemand` and its `*demand` return type, so rewrite it in one pattern). Comment prose keeps the word "demand" — this *is* a demand engine — so verify only that no `demand` **code identifier** remains:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d
sed -e 's/type demand struct/type rsm struct/' \
    -e 's/func newDemand(\(.*\)) \*demand {/func newRSM(\1) *rsm {/' \
    -e 's/(d \*demand)/(d *rsm)/g' \
    -e 's/d := &demand{/d := \&rsm{/' \
    E35-cflreach/cfl.go > E36-rsm/rsm.go
grep -nE '\*demand|&demand|type demand|newDemand|\(d \*demand\)' E36-rsm/rsm.go && echo "STOP: code identifier 'demand' remains" || echo "clean: no 'demand' code identifier left"
```
Expected: `clean: no 'demand' code identifier left`. (Prose occurrences of "demand" in comments are correct and stay.)

- [ ] **Step 3: Create `main.go` with `newDemand`→`newRSM` and a DISPATCH diff-dump**

Copy `E35-cflreach/main.go` to `E36-rsm/main.go` and swap the three `newDemand(` calls (portable sed):
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d
sed 's/newDemand(/newRSM(/g' E35-cflreach/main.go > E36-rsm/main.go
grep -nE 'newDemand' E36-rsm/main.go && echo "STOP: newDemand remains" || echo "main.go: newRSM swap clean"
```
Then extend the converged DISPATCH gate (now at the tail of `E36-rsm/main.go`, from `E35-cflreach/main.go:164-178`) to name the failing receivers under `DUMPDISPATCH=1`. Replace that block with:

```go
	// ---- corpora correctness gate (converged only): full receiver equivalence ----
	if converged {
		dd2 := newRSM(b, prog)
		dd2.query(recvs)
		diff := 0
		var firstFails []string
		for _, r := range recvs {
			if !sameSet(ptsBatch[r], dd2.pts[r]) {
				diff++
				if os.Getenv("DUMPDISPATCH") != "" && len(firstFails) < 16 {
					m, e := diffCount(ptsBatch[r], dd2.pts[r])
					var ty string
					for o := range ptsBatch[r] {
						if ob := dd2.base[o]; ob >= 0 && dd2.b.objType[ob] != nil {
							ty = dd2.b.objType[ob].String()
							break
						}
					}
					firstFails = append(firstFails, fmt.Sprintf("recv n%d batch|%d| demand|%d| (missing %d, extra %d) sample-type %s",
						r, len(ptsBatch[r]), len(dd2.pts[r]), m, e, ty))
				}
			}
		}
		for _, s := range firstFails {
			fmt.Printf("  DISPATCH-DIFF %s\n", s)
		}
		if diff == 0 {
			fmt.Printf("DISPATCH: PASS  (all %d receivers match batch)\n", len(recvs))
		} else {
			fmt.Printf("DISPATCH: FAIL  (%d receivers differ)\n", diff)
		}
	}
```

- [ ] **Step 4: Retype `slice.go`'s `buildFeeds` receiver**

`slice.go:15` is `func (d *demand) buildFeeds(...)`. Its body references `d.n, d.b, d.base, d.methodOf` — all present on `rsm`. Change only the receiver:

Run (portable redirect+`mv` form — this environment's `sed` is **GNU sed**, so `sed -i ''` fails, reading `''` as an empty script; comment prose keeps "demand"):
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
sed 's/func (d \*demand) buildFeeds/func (d *rsm) buildFeeds/' slice.go > slice.go.tmp && mv slice.go.tmp slice.go
grep -nE '\*demand' slice.go && echo "STOP: code identifier remains" || echo "slice.go clean (no *demand identifier)"
```
Expected: `slice.go clean (no *demand identifier)`. (Verified: this rename + the `newDemand`→`newRSM` swap below produce a binary that builds and runs `EQUIV: PASS` on the corpora.)

- [ ] **Step 5: Build (the "failing test" is that it did not build before)**

Run:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
GOWORK=off go build -o rsm . && echo BUILD_OK
```
Expected: `BUILD_OK`. (Before this task, `GOWORK=off go build ./...` fails with `./slice.go:15:10: undefined: demand`.)

- [ ] **Step 6: Baseline — one corpus and soundfix**

Run:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
GOWORK=off ./rsm "$HOME/ClaudeProjects/LLMAccuracy/code-restructuring/packages_iface_decoy/ifdecoy-d15k2-000" 2>/dev/null | grep -E 'EQUIV:|DISPATCH:'
DUMPDISPATCH=1 GOWORK=off ./rsm testdata/soundfix 2>/dev/null | grep -E 'EQUIV:|DISPATCH'
```
Expected (this is v1's known state — the FAIL is the defect Task 2 fixes):
```
EQUIV: PASS  (demand == batch on all ... non-empty nodes)
DISPATCH: PASS  (all ... receivers match batch)
EQUIV: PASS  (demand == batch on all 89303 non-empty nodes)
  DISPATCH-DIFF recv n... (missing ...) sample-type runtime.fixalloc...   [and 6 more, fixalloc/scavenger/timer/callCleanup]
DISPATCH: FAIL  (7 receivers differ)
```

- [ ] **Step 7: Create the 126-corpus gate runner**

Create `E36-rsm/run-corpora.sh`:
```bash
#!/bin/bash
# 126-corpus EQUIV+DISPATCH gate. Usage: ./run-corpora.sh ./rsm
set -u
SOLVER="${1:-./rsm}"
CORPORA="$HOME/ClaudeProjects/LLMAccuracy/code-restructuring/packages_iface_decoy"
ep=0; ef=0; dp=0; df=0
for c in "$CORPORA"/ifdecoy-*/; do
  out=$(GOWORK=off "$SOLVER" "$c" 2>/dev/null)
  if echo "$out" | grep -q "EQUIV: PASS"; then ep=$((ep+1)); else ef=$((ef+1)); echo "EQUIV FAIL  $(basename "$c")"; fi
  if echo "$out" | grep -q "DISPATCH: PASS"; then dp=$((dp+1)); else df=$((df+1)); echo "DISPATCH FAIL  $(basename "$c")"; fi
done
echo "-----------------------------------------"
echo "EQUIV    $ep pass / $ef fail"
echo "DISPATCH $dp pass / $df fail"
```
Make it executable and run it:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
chmod +x run-corpora.sh
./run-corpora.sh ./rsm | tail -3
```
Expected:
```
-----------------------------------------
EQUIV    126 pass / 0 fail
DISPATCH 126 pass / 0 fail
```

- [ ] **Step 8: Commit**

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast
git add -f notebooks/2026-07-14-pre-composition.d/E36-rsm/rsm.go \
           notebooks/2026-07-14-pre-composition.d/E36-rsm/main.go \
           notebooks/2026-07-14-pre-composition.d/E36-rsm/slice.go \
           notebooks/2026-07-14-pre-composition.d/E36-rsm/run-corpora.sh \
           notebooks/2026-07-14-pre-composition.d/E36-rsm/testdata/soundfix
git commit -m "E36 Task 0: rsm.go diff-base (v1 renamed), drivers, soundfix snapshot; EQUIV 126/126, DISPATCH 506/513"
```

---

## Task 1: Semi-naive propagation engine (§4, §11.1)

Recast the engine to `solve.go`'s difference discipline: the worklist **is** the delta, each complex `(v,o)` fires once (`doneComplex`), nothing rescans `pts[v]`. Activation policy is **unchanged** — v1's whole-cone `forwardActivateObject` writer-finding stays. Because all-nodes EQUIV activates everything, this alone must still reproduce `solveDiff` (EQUIV 126/126, soundfix EQUIV PASS). DISPATCH stays FAIL(7) and the footprint stays fat: those are Task 2.

**Files:**
- Modify: `E36-rsm/rsm.go` (struct fields, `newRSM` init, `add`→`emit`, `run()` recast, `reset()`, add `pushes`/`nDemands` tripwire)

**Interfaces:**
- Consumes: Task 0's `rsm`.
- Produces: `emit(p,o)` (delta-writing), the semi-naive `run()`, `delta`/`doneComplex`/`pushes` fields — consumed by Task 2's rule edits.

- [ ] **Step 1: Failing test — semi-naive not yet in place**

There is no unit-level failing test; the guard is the invariant test at Step 6 (EQUIV must stay 126/126 *after* the recast). Before editing, re-record the pass so a regression is visible:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
GOWORK=off ./rsm testdata/soundfix 2>/dev/null | grep 'EQUIV:'
```
Expected: `EQUIV: PASS  (demand == batch on all 89303 non-empty nodes)`.

- [ ] **Step 2: Add semi-naive + tripwire fields to the struct**

In `rsm.go`, inside `type rsm struct`, add after the `pts []map[int]bool` / `base []int` fields:
```go
	// SEMI-NAIVE tabling (mirrors solve.go:27,47 exactly)
	delta       []map[int]bool // node -> unpropagated additions
	doneComplex []map[int]bool // node -> objects whose complex edges fired

	// termination tripwire (§7/§9)
	pushes    int64 // total worklist pushes this query
	nDemands  int64 // activate() calls this query (the "#demands" term)
```

- [ ] **Step 3: Initialize the new slices in `newRSM`**

In `newRSM`, alongside `pts: make([]map[int]bool, n)`, add:
```go
		delta:       make([]map[int]bool, n),
		doneComplex: make([]map[int]bool, n),
```

- [ ] **Step 4: Rename `add`→`emit` and write the delta**

Replace v1's `add` (`cfl.go:214-226`) with `emit`, and rename **every** call site `d.add(` → `d.emit(` across `rsm.go` (in `activate`, `replay*`, `loadObject`, `fireAssert`, `wire`, and the old `run`). New body:
```go
func (d *rsm) emit(p, o int) { // == solve.go add() :59, demand-gated
	if p < 0 || o < 0 || !d.active[p] { // THE DEMAND GATE (only diff from batch)
		return
	}
	if d.pts[p] == nil {
		d.pts[p] = map[int]bool{}
		d.delta[p] = map[int]bool{}
	}
	if d.pts[p][o] {
		return
	}
	d.pts[p][o] = true
	d.delta[p][o] = true
	d.push(p)
}
```
And in `push`, count it:
```go
func (d *rsm) push(p int) {
	if !d.onWL[p] {
		d.onWL[p] = true
		d.wl = append(d.wl, p)
		d.pushes++
	}
}
```

- [ ] **Step 5: Recast `run()` to the delta loop (keeping v1 writer-finding)**

Replace v1's `run()` (`cfl.go:475-516`, the `for o := range d.pts[v]` rescan) with the semi-naive loop. This is the §4 skeleton but the load/store rules still call v1's `loadObject`/writer-finding (Task 2 swaps those). Consume `delta[v]`, fire each complex `(v,o)` once:
```go
func (d *rsm) run() {
	for len(d.wl) > 0 {
		v := d.wl[len(d.wl)-1]
		d.wl = d.wl[:len(d.wl)-1]
		d.onWL[v] = false

		dl := d.delta[v]
		if len(dl) == 0 {
			continue
		}
		d.delta[v] = map[int]bool{} // consume delta -> never rescan pts[v]

		if len(d.loads[v]) > 0 || len(d.stores[v]) > 0 || len(d.gepFwd[v]) > 0 ||
			len(d.calls[v]) > 0 || len(d.asserts[v]) > 0 {
			dc := d.doneComplex[v]
			if dc == nil {
				dc = map[int]bool{}
				d.doneComplex[v] = dc
			}
			for o := range dl {
				if dc[o] {
					continue
				}
				dc[o] = true // fire each (v,o) once (== solve.go:195)
				for _, dst := range d.loads[v] {
					if d.active[dst] {
						d.loadObject(o, dst) // v1 writer-finding; Task 2 replaces
					}
				}
				for _, val := range d.stores[v] {
					d.activate(val)
					d.dynFwd[val] = append(d.dynFwd[val], o)
					d.activate(o)
					d.replay(val, o, func(dst, oo int) { d.emit(dst, oo) })
				}
				for _, g := range d.gepFwd[v] {
					if ob := d.base[o]; ob >= 0 && g[1] < d.b.objSize[ob] {
						d.emit(g[0], ob+g[1])
					}
				}
				for _, ci := range d.calls[v] {
					d.fireCall(d.b.cs[ci], o)
				}
				for _, ci := range d.asserts[v] {
					if d.active[d.b.cs[ci].p] || d.b.cs[ci].f == cTFilter {
						d.fireAssert(d.b.cs[ci], o)
					}
				}
			}
		}
		for _, w := range d.copyFwd[v] {
			for o := range dl {
				d.emit(w, o)
			}
		}
		for _, w := range d.dynFwd[v] {
			for o := range dl {
				d.emit(w, o)
			}
		}
	}
}
```
Note: v1 keyed loads as `loadBack`/`loads` and stores as `stores` (no `storesWhereValue` yet). This step introduces no new index.

- [ ] **Step 6: Extend `reset()` to clear the per-query semi-naive state**

v1's `reset()` (`cfl.go:530`) clears `touched` entries. Extend it to also clear `delta`, `doneComplex`, and reset the counters:
```go
func (d *rsm) reset() {
	for _, i := range d.touched {
		d.pts[i] = nil
		d.delta[i] = nil
		d.doneComplex[i] = nil
		d.active[i] = false
		d.bwDone[i] = false
		d.fwdVisited[i] = false
		d.onWL[i] = false
		d.dynFwd[i] = nil
	}
	d.touched = d.touched[:0]
	d.wl = d.wl[:0]
	d.nActivated = 0
	d.pushes = 0
	d.nDemands = 0
}
```
(`bwDone`/`fwdVisited` are still v1's here; Task 2 renames them to `ptDone`/`fwdDone`.)

- [ ] **Step 7: Count demands in `activate`**

At the top of `activate` (after the `markActive(v)` call), add `d.nDemands++` once per genuine backward install, so the tripwire's `#demands` term is populated. Place it right after the `d.bwDone[v]` idempotence guard returns:
```go
	if d.bwDone[v] {
		return
	}
	d.bwDone[v] = true
	d.nDemands++
```

- [ ] **Step 8: Run EQUIV — must still be 126/126 and soundfix PASS**

Run:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
GOWORK=off go build -o rsm . && echo BUILD_OK
GOWORK=off ./rsm testdata/soundfix 2>/dev/null | grep -E 'EQUIV:|DISPATCH:'
./run-corpora.sh ./rsm | tail -3
```
Expected:
```
BUILD_OK
EQUIV: PASS  (demand == batch on all 89303 non-empty nodes)
DISPATCH: FAIL  (7 receivers differ)
EQUIV    126 pass / 0 fail
DISPATCH 126 pass / 0 fail
```
(DISPATCH FAIL(7) on soundfix is expected and unchanged — activation policy is untouched.)

- [ ] **Step 9: Commit**

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast
git add -f notebooks/2026-07-14-pre-composition.d/E36-rsm/rsm.go
git commit -m "E36 Task 1: semi-naive delta/doneComplex engine; EQUIV 126/126 preserved"
```

---

## Task 2: Forward object-tracking writer-finding (§5, §11.2) — closes the 7/513 and collapses the footprint

Replace v1's whole-cone `forwardActivateObject` (which `activate`s every copy/gep-cone node, the uniform 12-19%-of-N flood) with forward object-tracking: seed a tracked object at its births, `relay` it forward `markActive`-only, and fire the writer-harvest / load-out / field-push / containment-tracking rules where a *tracked* object lands. Three coupled mechanisms thread an object through arbitrary container depth, closing the 7 multi-hop-heap gaps. `pt == batch` is preserved because every new rule mirrors a `solve.go` primitive; only the *activation trigger* changes.

**Files:**
- Modify: `E36-rsm/rsm.go` (rename `bwDone`→`ptDone`, `fwdVisited`→`fwdDone` (per-base); delete `forwardActivateObject`; add `baseOf`, `subscribe`, `relay`, `loadPop`, `demandContents`; add `storesWhereValue` index; rewrite the `run()` load/store/gep rules with the gates)
- Modify: `E36-rsm/main.go` (add engine-footprint instrumentation to the DISPATCH loop)

**Interfaces:**
- Consumes: Task 1's `emit`, `run()`, `delta`, `doneComplex`.
- Produces: `baseOf`, `subscribe`, `relay`, `loadPop`, `demandContents`, `fwdDone` (per-base), `storesWhereValue`, `ptDone` — the writer-finding surface. `sigKey`/caller-discovery is Task 3.

- [ ] **Step 1: Failing test — the 7 gaps are the target**

Confirm the 7 you must drive to 0, and capture which objects they are:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
DUMPDISPATCH=1 GOWORK=off ./rsm testdata/soundfix 2>/dev/null | grep -E 'DISPATCH-DIFF|DISPATCH:'
```
Expected: 7 `DISPATCH-DIFF` lines (fixalloc/scavenger/timer/callCleanup) then `DISPATCH: FAIL  (7 receivers differ)`. These are 2-hop heap chains: object stored into one container, loaded, stored into another, whose writers v1's copy/gep forward-activation never chases.

- [ ] **Step 2: Rename activation-discipline fields**

In `type rsm struct`, rename `bwDone []bool` → `ptDone []bool` and `fwdVisited []bool` → `fwdDone []bool`, and add the comment that `fwdDone` is keyed **per base** (`fwdDone[baseOf(o)]`), and add the `storesWhereValue` index:
```go
	active  []bool // node may hold+relay pts (emit gate)
	ptDone  []bool // backward feeders installed (was bwDone)
	fwdDone []bool // per-BASE: contents demanded / object forward-tracked (was fwdVisited)

	storesWhereValue [][]int // val node -> store-pointer nodes {p : cStore(p,val)}
```
Update `newRSM` init: `ptDone: make([]bool, n)`, `fwdDone: make([]bool, n)`, `storesWhereValue: make([][]int, n)`, and delete the old `bwDone`/`fwdVisited` init lines. Rename every `d.bwDone`→`d.ptDone` and `d.fwdVisited`→`d.fwdDone` in `activate`/`reset` (and delete `fwdVisited` uses in the now-deleted `forwardActivateObject`).

- [ ] **Step 3: Build `storesWhereValue` in the preload loop**

In `newRSM`'s constraint preload (the `for i, c := range b.cs` loop, `cfl.go:116`), in the `case cStore` arm (currently only fills `d.stores[c.p]`), also append the pointer to the value's list:
```go
		case cStore:
			if c.p >= 0 && c.q >= 0 {
				d.stores[c.p] = append(d.stores[c.p], c.q)
				d.storesWhereValue[c.q] = append(d.storesWhereValue[c.q], c.p)
			}
```

- [ ] **Step 4: Add `baseOf`, `subscribe`, `relay`; delete `loadObject`/`forwardActivateObject`; add `loadPop`/`demandContents`**

Delete v1's `loadObject` (`cfl.go:331-337`) and `forwardActivateObject` (`cfl.go:345-375`). Add:
```go
func (d *rsm) baseOf(o int) int {
	if d.base[o] >= 0 {
		return d.base[o]
	}
	return o
}

// subscribe installs a dynamic subset edge src ⊇→ dst and replays src's set.
// == solve.go addEdge :76.
func (d *rsm) subscribe(src, dst int) {
	d.dynFwd[src] = append(d.dynFwd[src], dst)
	for o := range d.pts[src] {
		d.relay(src, dst, o)
	}
}

// relay forwards one object along a subset edge. THE FOOTPRINT FIX (§3): a
// forward-cone node gets markActive (relay-only), never full activate, so the
// cone is not multiplied by each node's backward cone. Only a node holding a
// TRACKED object escalates.
func (d *rsm) relay(src, dst, o int) {
	if d.fwdDone[d.baseOf(o)] {
		d.markActive(dst)
	}
	d.emit(dst, o)
}

// loadPop: dst = *ptr, o ∈ pts(ptr): dst ⊇ contents(o).
func (d *rsm) loadPop(o, dst int) {
	d.demandContents(o)  // fill contents(o) from o's writers
	d.subscribe(o, dst)  // contents(o) relays to dst (== addEdge(o,dst))
}

// demandContents finds the writers of o's contents by forward-tracking o's BASE:
// seed &base and let relay() carry it to every store-pointer that writes it.
// Keying off base (not o) is essential (v1 cfl.go:343): a sub-node base+off has
// no &(base+off); its writers are reached from &base via the gep that made &base.off.
func (d *rsm) demandContents(o int) {
	d.markActive(o) // o must be active to HOLD contents (attack-2 secondary; v1 did this at :335)
	b := d.baseOf(o)
	if d.fwdDone[b] {
		return
	}
	d.fwdDone[b] = true
	for _, a := range d.objAddr[b] {
		d.markActive(a)
		d.emit(a, b)
	}
}
```

- [ ] **Step 5: Rewrite the `run()` complex rules with the §5 gates**

Replace the load/store/gep arms inside `run()`'s per-`(v,o)` block, add the containment-tracking pre-check and the field-push post-check. The full `run()` complex section becomes (copy/dyn tail with `relay`, unchanged names):
```go
		// containment-tracking (§5.2): a tracked object landing in a cell tracks the cell
		if d.base[v] >= 0 {
			for o := range dl {
				if d.fwdDone[d.baseOf(o)] {
					d.demandContents(v)
					break
				}
			}
		}

		if len(d.loads[v]) > 0 || len(d.stores[v]) > 0 || len(d.gepFwd[v]) > 0 ||
			len(d.calls[v]) > 0 || len(d.asserts[v]) > 0 {
			dc := d.doneComplex[v]
			if dc == nil {
				dc = map[int]bool{}
				d.doneComplex[v] = dc
			}
			for o := range dl {
				if dc[o] {
					continue
				}
				dc[o] = true
				for _, dst := range d.loads[v] { // p = *v : LOAD-OUT GATE FIX (§5.3)
					if d.active[dst] || d.fwdDone[d.baseOf(o)] {
						d.markActive(dst)
						d.loadPop(o, dst)
					}
				}
				for _, val := range d.stores[v] { // *v=val, o container: contents(o) ⊇ pts(val)
					d.activate(val)
					d.markActive(o)
					d.subscribe(val, o)
				}
				for _, g := range d.gepFwd[v] { // p = &v.off
					if ob := d.baseOf(o); ob >= 0 && g[1] < d.b.objSize[ob] {
						if d.active[g[0]] || d.fwdDone[ob] {
							d.markActive(g[0])
							d.emit(g[0], ob+g[1])
						}
					}
				}
				for _, ci := range d.calls[v] {
					d.fireCall(d.b.cs[ci], o)
				}
				for _, ci := range d.asserts[v] {
					if d.active[d.b.cs[ci].p] || d.b.cs[ci].f == cTFilter {
						d.fireAssert(d.b.cs[ci], o)
					}
				}
				if d.fwdDone[d.baseOf(o)] { // FIELD-PUSH (§5.1): tracked o at a store VALUE
					for _, p := range d.storesWhereValue[v] {
						d.activate(p)
					}
				}
			}
		}
		for _, w := range d.copyFwd[v] {
			for o := range dl {
				d.relay(v, w, o)
			}
		}
		for _, w := range d.dynFwd[v] {
			for o := range dl {
				d.relay(v, w, o)
			}
		}
```

- [ ] **Step 6: Route `cUntag` through `loadPop`**

v1's `fireAssert` `cUntag` arm (`cfl.go:391-397`) called `forwardActivateObject` (now deleted). Replace it to ride the same contents machinery as load (§5, §11 "minus its forwardActivateObject call, replaced by loadPop for cUntag"):
```go
	case cUntag:
		if types.Identical(t, c.typ) {
			d.markActive(c.p)
			d.loadPop(o, c.p) // payload = contents(o), same as a load
		}
```

- [ ] **Step 7: Add engine-footprint instrumentation to `main.go`'s DISPATCH loop**

The design's FOOTPRINT gate (§10) wants "engine footprint ≈ optimal slice". `main.go` already prints the *optimal* slice (`slice.go`); add the *engine's* per-receiver `|touched|` beside it. After the OPTIMAL DEMAND FOOTPRINT block (`main.go:161`), insert:
```go
	// ---- ENGINE FOOTPRINT: the engine's actual per-receiver activation ----
	fmt.Printf("\n─── ENGINE FOOTPRINT (rsm |touched| per receiver) ───\n")
	de := newRSM(b, prog)
	var efoots []int
	for _, r := range recvs {
		if len(ptsBatch[r]) == 0 {
			continue
		}
		de.query([]int{r})
		efoots = append(efoots, len(de.touched))
		de.reset()
	}
	sort.Ints(efoots)
	emx := 0
	if len(efoots) > 0 {
		emx = efoots[len(efoots)-1]
	}
	fmt.Printf("engine footprint  median %d  p90 %d  max %d   (of %d)\n",
		pctile(efoots, 50), pctile(efoots, 90), emx, N)
```

- [ ] **Step 8: Run the gates — DISPATCH 7→0, EQUIV still green, footprint collapsed**

Run:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
GOWORK=off go build -o rsm . && echo BUILD_OK
GOWORK=off ./rsm testdata/soundfix 2>/dev/null | grep -E 'EQUIV:|DISPATCH:|engine footprint'
./run-corpora.sh ./rsm | tail -3
```
Expected:
```
BUILD_OK
EQUIV: PASS  (demand == batch on all 89303 non-empty nodes)
engine footprint  median <~10>  p90 ...  max ...   (of 153502)
DISPATCH: PASS  (all ... receivers match batch)
EQUIV    126 pass / 0 fail
DISPATCH 126 pass / 0 fail
```
Acceptance: soundfix DISPATCH goes to **PASS** (the 7 closed); EQUIV stays 126/126 + soundfix PASS; engine-footprint median tracks the optimal slice (soundfix optimal median was 6 at partial budget) rather than v1's uniform 12-19% of N. If EQUIV regresses, a rule stopped mirroring `solve.go` — bisect the run() arms against the pinned `solve.go` sites (§2 table).

- [ ] **Step 9: Footprint sanity on a real target (wile)**

The footprint claim is *measured, not proven* (open risk: hot shared containers). Run one real target and eyeball that the engine footprint stays near the optimal slice for the domain median and only balloons on god-object-reaching receivers:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
BUDGET=short GOWORK=off ./rsm /Users/aalpar/projects/wile-workspace/wile-goast 2>/dev/null | grep -E 'slice \(nodes\)|engine footprint|reach the flood'
```
Expected: optimal `slice (nodes) median 11 ...`; `engine footprint median` in the same order of magnitude (a small relay surcharge over 11, not thousands). Record the numbers in the notebook entry; a fat median here is the open-risk "hot shared container" surcharge, to be reported, not silently accepted.

- [ ] **Step 10: Commit**

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast
git add -f notebooks/2026-07-14-pre-composition.d/E36-rsm/rsm.go \
           notebooks/2026-07-14-pre-composition.d/E36-rsm/main.go
git commit -m "E36 Task 2: forward object-tracking writer-finding; DISPATCH 513/513 on soundfix, footprint collapse; EQUIV 126/126"
```

---

## Task 3: Signature-filtered caller discovery (§6, §11.3) — closes the demand-leak

v1's icall caller-discovery (`cfl.go:287-291`) activates **every** indirect-call receiver program-wide when a demanded param belongs to any address-taken function, dragging the `*errors.errorString` flood into unrelated domain queries. Replace it with a signature-filtered version: index icall receivers by canonical func signature, demand only the compatible bucket. Sound (a real caller's func-value type is signature-identical) and tight. `pt == batch` unaffected — the filter narrows the *footprint*, and `wire`'s `objFunc` filter still prunes non-`fn` callers so the result stays exact.

**Files:**
- Modify: `E36-rsm/rsm.go` (`icallRecvsBySig` index + `valOf` reverse map in `newRSM`; `sigKey`; the caller-discovery branch in `activate`; tighten `fnAddrTaken`)
- Create: `E36-rsm/testdata/demandleak/` (adversarial corpus, §10)

**Interfaces:**
- Consumes: Task 2's engine + `activate`.
- Produces: `sigKey(sig)`, `icallRecvsBySig` — no downstream consumer (last engine task).

**Note (design under-specification resolved here):** §6 keys the bucket on "the func-value's underlying `*types.Signature`", but the `builder` exposes only `nodeOf map[ssa.Value]int` (value→node), no node→type. So `newRSM` must invert `b.nodeOf` to recover each icall func-value's `ssa.Value` and read `.Type().Underlying().(*types.Signature)`. Both the bucket key and `sigKey(fn.Signature)` use `types.TypeString(sig, nil)` (nil qualifier canonicalizes both sides).

- [ ] **Step 1: Failing test — build the adversarial demand-leak corpus**

The 126 corpora never trip caller-discovery, so the leak is invisible without a dedicated corpus. Create `E36-rsm/testdata/demandleak/go.mod`:
```
module demandleak

go 1.24.0
```
Create `E36-rsm/testdata/demandleak/main.go` — a domain interface+method whose receiver's backward slice crosses a param of an address-taken closure, embedded with error-typed indirect calls:
```go
package main

import (
	"errors"
	"fmt"
)

// Domain interface + method: the query target.
type Widget interface{ Area() int }

type Box struct{ w, h int }

func (b *Box) Area() int { return b.w * b.h }

// An address-taken closure whose param the domain receiver's slice crosses.
func apply(f func(Widget) int, wg Widget) int { return f(wg) }

// Error-typed indirect call: the flood that must NOT be pulled into the domain query.
func onErr(cb func(error), e error) { cb(e) }

func main() {
	var wg Widget = &Box{3, 4}
	measure := func(w Widget) int { return w.Area() } // Area() dispatch: the domain receiver
	fmt.Println(apply(measure, wg))

	handler := func(e error) { fmt.Println(e.Error()) } // Error() dispatch on errorString
	onErr(handler, errors.New("boom"))
	onErr(handler, fmt.Errorf("wrapped: %w", errors.New("inner")))
}
```
Confirm the corpus loads and resolves under the current (leaky) engine, and capture the leaky footprint for the domain receiver so the fix is measurable:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
GOWORK=off ./rsm testdata/demandleak 2>/dev/null | grep -E 'EQUIV:|DISPATCH:|engine footprint'
```
Expected: `EQUIV: PASS`, `DISPATCH: PASS`. The engine-footprint max here reflects the leak (the `Area()` query crosses `apply`'s address-taken param and, pre-fix, activates the `onErr` receiver bucket too). Record the number; Task 3's acceptance is that it drops after the filter.

- [ ] **Step 2: Add `sigKey` and the reverse-map-built `icallRecvsBySig`**

Add the helper:
```go
func sigKey(sig *types.Signature) string { return types.TypeString(sig, nil) }
```
In `type rsm struct`, add `icallRecvsBySig map[string][]int` next to `icallRecvs`. In `newRSM`, init it `icallRecvsBySig: map[string][]int{}`, and after the constraint preload loop, build it from an inverted `nodeOf`:
```go
	// invert nodeOf so we can recover an icall func-value's static signature
	valOf := make(map[int]ssa.Value, len(b.nodeOf))
	for v, id := range b.nodeOf {
		valOf[id] = v
	}
	for _, c := range b.cs {
		if c.f == cICall && c.p >= 0 {
			if fv, ok := valOf[c.p]; ok {
				if sig, ok := fv.Type().Underlying().(*types.Signature); ok {
					k := sigKey(sig)
					d.icallRecvsBySig[k] = append(d.icallRecvsBySig[k], c.p)
				}
			}
		}
	}
```

- [ ] **Step 3: Swap the caller-discovery branch in `activate`**

Replace v1's leaky icall branch (`cfl.go:287-291`) inside `activate`'s `if fn, ok := d.paramOwner[v]; ok { ... }` block:
```go
		if d.fnAddrTaken[fn] {
			for _, recv := range d.icallRecvsBySig[sigKey(fn.Signature)] {
				d.activate(recv)
			}
		}
```
Leave the invoke branch (`invokesByMethod[m]`) as-is — §6 keeps it method-name-scoped (it over-activates only on `Error`/`String`, the allowed error tail).

- [ ] **Step 4: Run the leak gate — domain footprint must drop, EQUIV stays green**

Run:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
GOWORK=off go build -o rsm . && echo BUILD_OK
GOWORK=off ./rsm testdata/demandleak 2>/dev/null | grep -E 'EQUIV:|DISPATCH:|engine footprint'
GOWORK=off ./rsm testdata/soundfix 2>/dev/null | grep -E 'EQUIV:|DISPATCH:'
./run-corpora.sh ./rsm | tail -3
```
Expected:
```
BUILD_OK
EQUIV: PASS ...
DISPATCH: PASS ...
engine footprint  median ...  (max dropped vs Step 1 — the Area() query no longer pulls the onErr bucket)
EQUIV: PASS ...
DISPATCH: PASS  (all ... receivers match batch)
EQUIV    126 pass / 0 fail
DISPATCH 126 pass / 0 fail
```
Acceptance: the demandleak engine-footprint max is smaller than Step 1's, EQUIV/DISPATCH unchanged everywhere. If DISPATCH regresses on the corpus, the signature filter dropped a real caller — verify `sigKey(fn.Signature)` equals the bucket key computed from the func-value's `ssa.Value` type (the two must canonicalize identically under nil qualifier).

- [ ] **Step 5: Tighten `fnAddrTaken` (belt-and-suspenders; gate behind EQUIV)**

§6 secondary: a closure whose `MakeClosure` value flows only into a direct `Call` at its own site is not an icall target. v1's test (`cfl.go:194`) is `at := fn.Parent() != nil || fn.Synthetic != ""` — over-broad (every closure). Refine the `valueUsed`-based upgrade so a closure counts as address-taken only if its value escapes (flows into a store/arg/return/copy that outlives the site), not merely into an immediate direct call. In `newRSM`, the existing loop already computes `valueUsed[c.q]` for `cCopy/cStore/cLoad/cGep` and args. Change the closure default so `fn.Parent() != nil` alone does NOT set `at` — require the `valueUsed` escape:
```go
		at := fn.Synthetic != "" // synthetic thunks stay address-taken
		if id, ok := b.nodeOf[fn]; ok && valueUsed[id] {
			at = true // the closure's value node escapes -> genuine icall target
		}
		d.fnAddrTaken[fn] = at
```
This is a heuristic that could *under*-activate and fire EQUIV. It MUST be gated:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
GOWORK=off go build -o rsm . && GOWORK=off ./rsm testdata/soundfix 2>/dev/null | grep 'EQUIV:'
./run-corpora.sh ./rsm | tail -3
```
Expected: `EQUIV: PASS` on soundfix AND `EQUIV 126 pass / 0 fail`. **If EQUIV fails, revert this step** — the signature filter (Steps 2–3) is sound on its own and is the shippable fix. Do not keep a tightening that breaks EQUIV.

- [ ] **Step 6: Commit**

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast
git add -f notebooks/2026-07-14-pre-composition.d/E36-rsm/rsm.go \
           notebooks/2026-07-14-pre-composition.d/E36-rsm/testdata/demandleak
git commit -m "E36 Task 3: signature-filtered icall caller discovery closes demand-leak; EQUIV 126/126"
```

---

## Task 4: Termination hardening — cyclic fixtures + push tripwire (§7, §9)

The corpora are dispatch-shaped and never exercise deep cyclic fields, so add three cyclic-heap EQUIV fixtures and assert the termination tripwire. All three must EQUIV-PASS (the finite-Herbrand + `fwdDone`-memoized argument holds) and the worklist must stay bounded.

**Files:**
- Create: `E36-rsm/testdata/cyclic/{selffield,twonode,mutual}/{go.mod,main.go}`
- Modify: `E36-rsm/main.go` (assert `pushes ≤ n·nObjs + nDemands` after the EQUIV query)

**Interfaces:**
- Consumes: Task 2's `fwdDone`-memoized `demandContents`; Task 1's `pushes`/`nDemands`.

- [ ] **Step 1: Add the termination tripwire to the EQUIV driver**

In `main.go`, right after the all-nodes `d.query(all)` (`main.go:64`), assert the bound (§7 hardening: "total worklist pushes ≤ n·nObjs + #demands"):
```go
		bound := int64(b.nNodes)*int64(b.nObjs) + d.nDemands
		if d.pushes > bound {
			fmt.Printf("TERMINATION: TRIPWIRE FIRED  pushes %d > bound %d (n*nObjs %d + demands %d)\n",
				d.pushes, bound, int64(b.nNodes)*int64(b.nObjs), d.nDemands)
		} else {
			fmt.Printf("TERMINATION: OK  pushes %d <= bound %d\n", d.pushes, bound)
		}
```

- [ ] **Step 2: Fixture 1 — self-field cycle `x = x.next`**

Create `E36-rsm/testdata/cyclic/selffield/go.mod` (`module cyc\n\ngo 1.24.0`) and `main.go`:
```go
package main

import "fmt"

type Node struct {
	next *Node
	tag  Stringer
}

type Stringer interface{ String() string }
type S struct{ v int }

func (s *S) String() string { return fmt.Sprint(s.v) }

func main() {
	n := &Node{tag: &S{1}}
	n.next = n // self-referential field: x = x.next forever
	for i := 0; i < 3; i++ {
		n = n.next
	}
	fmt.Println(n.tag.String()) // dispatch on the cyclically-loaded tag
}
```

- [ ] **Step 3: Fixture 2 — two-node field cycle under a traversal phi**

Create `E36-rsm/testdata/cyclic/twonode/{go.mod,main.go}` (`module cyc`):
```go
package main

import "fmt"

type Stringer interface{ String() string }
type S struct{ v int }

func (s *S) String() string { return fmt.Sprint(s.v) }

type B struct {
	next *B
	tag  Stringer
}

func main() {
	b1 := &B{tag: &S{1}}
	b2 := &B{tag: &S{2}}
	b1.next = b2
	b2.next = b1 // two-node cycle
	p := b1
	for i := 0; i < 5; i++ { // traversal phi merges b1/b2 into p
		p = p.next
	}
	fmt.Println(p.tag.String())
}
```

- [ ] **Step 4: Fixture 3 — mutually-recursive containers `A ⊂ B, B ⊂ A`**

Create `E36-rsm/testdata/cyclic/mutual/{go.mod,main.go}` (`module cyc`):
```go
package main

import "fmt"

type Stringer interface{ String() string }
type S struct{ v int }

func (s *S) String() string { return fmt.Sprint(s.v) }

type A struct {
	b   *B
	tag Stringer
}
type B struct {
	a   *A
	tag Stringer
}

func main() {
	a := &A{tag: &S{1}}
	b := &B{tag: &S{2}}
	a.b = b
	b.a = a // A contains B contains A
	fmt.Println(a.b.a.b.a.tag.String()) // deep containment chain, dispatch at the end
}
```

- [ ] **Step 5: Run all three — EQUIV PASS + TERMINATION OK**

Run:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
GOWORK=off go build -o rsm .
for f in selffield twonode mutual; do
  echo "== $f =="
  GOWORK=off ./rsm "testdata/cyclic/$f" 2>/dev/null | grep -E 'EQUIV:|TERMINATION:|DISPATCH:'
done
```
Expected, for each: `EQUIV: PASS`, `TERMINATION: OK ...`, `DISPATCH: PASS`. A hang means a cycle escaped the `fwdDone`/membership guard — the containment-tracking or field-push recursion is not memoizing on `baseOf(o)`; re-check that `demandContents` returns early on `fwdDone[b]`.

- [ ] **Step 6: Commit**

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast
git add -f notebooks/2026-07-14-pre-composition.d/E36-rsm/main.go \
           notebooks/2026-07-14-pre-composition.d/E36-rsm/testdata/cyclic
git commit -m "E36 Task 4: cyclic-heap EQUIV fixtures + push tripwire (termination hardening)"
```

---

## Task 5: Deep multi-hop DISPATCH fixture (open-risk: depth ≥ 3)

The 7 closed gaps are 2-hop. §5's mechanism is depth-unbounded *by construction* (memoized recursion), but no corpus exhibits a 3+-hop chain, so completeness at greater depth is argued, not observed (design "Open risks"). Add a synthetic ≥3-hop store→load→store→load→store→load chain feeding a dispatch receiver and assert DISPATCH matches batch.

**Files:**
- Create: `E36-rsm/testdata/deepchain/{go.mod,main.go}`

**Interfaces:**
- Consumes: Task 2's `demandContents`/containment-tracking/load-out recursion.

- [ ] **Step 1: Build the 3-hop fixture**

Create `E36-rsm/testdata/deepchain/go.mod` (`module deep\n\ngo 1.24.0`) and `main.go` — an object threaded through three nested containers before the dispatch reads it back out:
```go
package main

import "fmt"

type Stringer interface{ String() string }
type Impl struct{ v int }

func (i *Impl) String() string { return fmt.Sprint(i.v) }

type L1 struct{ p Stringer }
type L2 struct{ c *L1 }
type L3 struct{ c *L2 }

func main() {
	target := &Impl{42}

	// hop 1: target stored into L1
	a := &L1{}
	a.p = target
	// hop 2: L1 stored into L2
	x := &L2{}
	x.c = a
	// hop 3: L2 stored into L3
	y := &L3{}
	y.c = x

	// read back out through all three containers, then dispatch
	got := y.c.c.p // L3 -> L2 -> L1 -> Stringer
	fmt.Println(got.String())
}
```

- [ ] **Step 2: Run — DISPATCH must match batch (the whole point)**

Run:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
GOWORK=off ./rsm testdata/deepchain 2>/dev/null | grep -E 'EQUIV:|DISPATCH:'
```
Expected: `EQUIV: PASS`, `DISPATCH: PASS  (all ... receivers match batch)`. A DISPATCH FAIL here means the containment-tracking / load-out recursion does not chase depth ≥ 3 — the latent gap the open-risk names. If it fails, the multi-hop mechanism needs the design authors before shipping; STOP and report (do not paper over with a special case).

- [ ] **Step 3: Commit**

```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast
git add -f notebooks/2026-07-14-pre-composition.d/E36-rsm/testdata/deepchain
git commit -m "E36 Task 5: 3-hop deep-chain DISPATCH fixture (multi-hop depth observed, not just argued)"
```

---

## Task 6: Full-target footprint run + notebook entry E36

Reproduce the E35 measurement set on the real targets with the new engine and record the outcome as notebook entry E36. This is the acceptance record, not a code change.

**Files:**
- Create: `E36-rsm/run-{wile,etcd,ollama,soundfix-CONVERGED,corpora-126}.txt` (regenerated artifacts)
- Modify: the notebook (append entry E36 via the lab-notebook skill)

- [ ] **Step 1: Regenerate the corpora + soundfix gate artifacts**

Run:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
./run-corpora.sh ./rsm | tail -3 | tee run-corpora-126.txt
GOWORK=off ./rsm testdata/soundfix 2>/dev/null | tee run-soundfix-CONVERGED.txt | grep -E 'EQUIV:|DISPATCH:|engine footprint|TERMINATION:'
```
Expected: `EQUIV 126 pass / 0 fail`, `DISPATCH 126 pass / 0 fail`; soundfix EQUIV PASS + DISPATCH PASS (513/513) + TERMINATION OK.

- [ ] **Step 2: Footprint on the three real targets**

Run (each is a 60s partial-budget slice + engine footprint):
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast/notebooks/2026-07-14-pre-composition.d/E36-rsm
for t in /Users/aalpar/projects/wile-workspace/wile-goast; do
  BUDGET=short GOWORK=off ./rsm "$t" 2>/dev/null | tee "run-wile.txt" | grep -E 'slice \(nodes\)|engine footprint|reach the flood'
done
```
Repeat for etcd/ollama target dirs if available (paths from the E35 run logs). Record the engine-footprint median vs the optimal-slice median for each. The gate (§10 FOOTPRINT): engine footprint ≈ optimal slice for the domain median; flooded footprints occur only on receivers whose optimal slice reaches the god object. A fat engine median on a real target is the open-risk "hot shared container" surcharge — **report it in the notebook**, do not suppress it.

- [ ] **Step 3: Record notebook entry E36**

Use the lab-notebook skill to append entry E36 to `2026-07-14-pre-composition`: the four gates (EQUIV 126/126 + soundfix, DISPATCH 513/513, engine-footprint vs optimal slice on the real targets, demand-leak corpus), the fixtures added (3 cyclic + deep-chain), and honestly whether the footprint surcharge stayed near-optimal or showed a fat tail. Cross-link the design doc and this plan.

- [ ] **Step 4: Commit + update the plans index**

Mark this plan Shipped in `plans/CLAUDE.local.md`'s Active Plan Files table (the `2026-07-14-e36-rsm-pointsto-design.md` row), then:
```bash
cd /Users/aalpar/projects/wile-workspace/wile-goast
git add -f notebooks/2026-07-14-pre-composition.d/E36-rsm/run-*.txt \
           notebooks/2026-07-14-pre-composition.d/*.md
git add plans/CLAUDE.local.md plans/2026-07-14-e36-rsm-pointsto-impl.md
git commit -m "E36 Task 6: full-target footprint run + notebook entry E36; RSM engine shipped"
```

---

## What this plan excludes by design (= design §"What Phase 1 excludes" + Phase 2)

**Phase 2 (context sensitivity) is NOT in this plan.** k-bounded call-string context + heap cloning (design §Phase 2) removes the errorString flood entirely by splitting the merged `*errors.errorString` object per allocation context. It changes the validation model (`pts_cs ⊑ pts_ci`, so `== batch` no longer holds; the gate becomes the `⊔_c pts_cs == pts_ci` soundness collapse). It is a separate plan, written only after Phase 1's four gates are green and the footprint surcharge is measured on the real targets. Do not start Phase 2 mechanisms (object-model `(key, t)`→`(key, t, ctx)`, context-threaded `wire`) inside this plan.

---

## Self-Review

**Spec coverage** (design §→task):
- §1 data structures → Task 0 (rename), Task 1 (`delta`/`doneComplex`), Task 2 (`storesWhereValue`, `fwdDone` per-base, `ptDone`), Task 3 (`icallRecvsBySig`). `reset()` extension → Task 1 Step 6.
- §2 8-forms→rules → preserved verbatim except the load/store/gep/untag arms rewritten in Task 2 Steps 5–6; caller-discovery in Task 3.
- §3 core primitives (`emit`/`subscribe`/`relay`) → Task 1 Step 4 (`emit`), Task 2 Step 4 (`subscribe`/`relay`/`baseOf`).
- §4 worklist (no O(N) scan) → Task 1 Step 5.
- §5 heap match (writer-finding, field-push, containment-tracking, load-out gate) → Task 2 Steps 4–6; multi-hop depth → Task 5.
- §6 dispatch woven in + demand-leak fix → Task 3.
- §7 termination → Task 4 (fixtures + tripwire).
- §8 equivalence → the EQUIV gate run after every task.
- §9 complexity (footprint) → Task 2 Steps 7–9, Task 6 Step 2.
- §10 validation plan (EQUIV/DISPATCH/FOOTPRINT/demand-leak) → Tasks 0/2/3/6.
- §11 build order → Tasks 1/2/3 map 1:1 to §11 steps 1/2/3; Task 0 is the pre-baseline; Tasks 4/5/6 are the §7/§9/§10 hardening + record.

**Placeholder scan:** every code step shows complete Go; every run step shows the exact command + expected output line. The two "STOP and report" branches (Task 0 Step 1 if soundfix is gone; Task 5 Step 2 if depth ≥ 3 fails) are deliberate escalation gates, not TBDs.

**Type consistency:** `emit(p,o int)`, `subscribe(src,dst int)`, `relay(src,dst,o int)`, `loadPop(o,dst int)`, `demandContents(o int)`, `baseOf(o int) int`, `sigKey(sig *types.Signature) string`, `newRSM(b *builder, prog *ssa.Program) *rsm` — used identically in every task that references them. Field renames (`bwDone`→`ptDone`, `fwdVisited`→`fwdDone`) are applied atomically in Task 2 Step 2 (struct + `newRSM` + `activate` + `reset` together), so no half-renamed intermediate compiles. `fwdDone` is keyed per `baseOf(o)` everywhere it is read/written (relay, run gates, demandContents).

**Known open risks carried from the design (measured, not eliminated by this plan):** forward surcharge on hot shared containers (Task 2 Step 9 / Task 6 Step 2 measure it); multi-hop depth ≥ 3 (Task 5 observes it); signature-filter looseness on very-general func types (`func()`, `func(...interface{})`) — intrinsic, the adversarial corpus only partly probes it; `fnAddrTaken` tightening is heuristic and EQUIV-gated (Task 3 Step 5 reverts on failure).
