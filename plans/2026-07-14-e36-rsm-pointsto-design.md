# E36: RSM demand-driven points-to, final design

Notebook `2026-07-14-pre-composition`, entry E36. Successor to E35 (v1, `cfl.go`).
Synthesizes the three candidate designs (boomerang / magicsets / tabling) into one
implementable spec, grafts the runners-up where they are stronger, and folds in every
fix the adversarial pass found. Substrate is frozen: `extract.go` (object model + 8
forms), `solve.go` (batch oracle = the equivalence spec), `slice.go` (optimal footprint),
`cfl.go` (v1, the diff base). Target file: `E36-rsm/rsm.go`.

Paths are absolute to `.../notebooks/2026-07-14-pre-composition.d/E35-cflreach/` unless noted;
`E36-rsm/` already carries byte-identical `extract.go`/`solve.go`/`slice.go`.

## Thesis

E31 already exploded the field stack into object nodes: an allocation is a flat span
`[base, base+size)` (`extract.go:134`), a "field f" is the integer `base+off`
(`fieldOffset`, `extract.go:153`), and the field-Dyck bracket `store[f] … load[f]` is
matched by **object-node identity**: a store `*p=val` and a load `r=*q` act on the same
cell `o` iff `o ∈ pts[p] ∩ pts[q]`. There is no field stack to `k`-limit; its role is
played by the `pts` relation, and its unbounded depth (cyclic fields) is bounded by the
finiteness of object nodes plus memoization, not truncation. Consequence: **context-
insensitive field-sensitive points-to over E31 is exactly Andersen** (`solve.go`), and a
demand engine that runs the batch rules over an activated frontier computes the identical
relation. E36 changes nothing in propagation; it changes only **activation policy**. v1's
two defects (uniform flood, 7/513 multi-hop gaps) are both activation-policy bugs, fixed
here without touching the propagation arithmetic, so `pt == batch` is preserved by
construction.

`pts[v]` has one table with two readings by node kind (`cfl.go:32`): for a pointer,
points-to; for an object/cell node `o`, `pts[o]` **is** `contents(o)` (what was stored
into that field). This is why `contents` needs no separate storage: the store rule fills
`pts[o]` via `addEdge(val,o)` (`solve.go:204`), the load rule reads it via `addEdge(o,dst)`
(`solve.go:201`).

## Adversarial fixes: crosswalk

The verification pass ran four attacks. One found no flaw (hardening only); three found
real under/over-approximations in the candidate designs. Every one is folded in and
located here:

| Attack | Verdict | Where the final design handles it |
|--------|---------|-----------------------------------|
| **1. Non-termination** (copy/gep/field/container cycles) | No flaw; §8 sound | §7 termination argument (finite Herbrand base + monotone + memoized guards). Hardening: §9 adds three cyclic EQUIV fixtures + a `pushes ≤ n·nObjs + #demands` tripwire. |
| **2. Under-approximation at the heap match** (multi-hop store→load→store; intermediate load dest `w` never activated; `active[dst]` gate blocks it) | Flaw in boomerang/tabling as written | §5 **containment-tracking rule** + **load-out gate fix** (`active[dst] ‖ fwdDone[base(o)]`). Plus the secondary note: `demandContents` must `markActive` the object node itself (v1 does this at `cfl.go:335`; the candidate designs dropped it). |
| **3. Over-activation / demand-leak** (caller-discovery activates *all* icall receivers program-wide, dragging errorString into domain queries) | Flaw in v1 + all candidates (all inherit `cfl.go:287-291`) | §6 **signature-filtered caller discovery** (`icallRecvsBySig`), tightened `fnAddrTaken`, and the invoke branch left method-name-scoped (floods only on `Error`/`String`, the allowed tail). |
| **4. Multi-hop-heap completeness** (field-push never fires from a value-only node; load-out blocked by the same gate) | Flaw in tabling; latent in boomerang | §5 **field-push from the value side** (`storesWhereValue` → `demandPT(storePtr)`) *and* the containment-tracking rule, which together thread the object through arbitrary container depth. Closes the 7/513. |

Attacks 2 and 4 are the same omission from two angles; §5 fixes both with one coherent
mechanism.

---

# Phase 1: context-insensitive RSM (fully specified)

## 1. Data structures (over v1's `newDemand`, `cfl.go:76`)

Reuse verbatim from `cfl.go`: `base[]`; the backward indices `addrOf, copyBack, gepBack,
loadBack, asrtBack, callRet, objAddr`; the forward indices `copyFwd, gepFwd, dynFwd, loads,
stores, calls, asserts`; caller-discovery `paramOwner, fnMethod, fnAddrTaken,
invokesByMethod, icallRecvs`; and `reset()` over `touched` (`cfl.go:530`). Changes are
additive and localized:

```go
type rsm struct {
    // ... all of v1's indices, verbatim ...

    pts  []map[int]bool   // node -> objects. pointer: points-to; cell: contents. == batch pts.
    base []int

    // SEMI-NAIVE tabling (grafted from tabling/magicsets; mirrors solve.go exactly)
    delta       []map[int]bool  // node -> unpropagated additions            (solve.go:27)
    doneComplex []map[int]bool  // node -> objects whose complex edges fired  (solve.go:47)

    // ACTIVATION discipline
    active  []bool  // node may hold+relay pts (emit gate)                     (cfl.go:63)
    ptDone  []bool  // backward feeders installed (v1's bwDone)                (cfl.go:64)
    fwdDone []bool  // per-BASE: contents demanded / object forward-tracked (replaces v1 fwdVisited)

    // NEW static indices
    storesWhereValue [][]int          // val node -> store-pointer nodes {p : cStore(p,val)}
    icallRecvsBySig  map[string][]int // canonical func signature -> cICall receiver nodes

    wl, onWL, touched, methodCache ... // as v1
}
```

Deleted from v1: `fwdVisited` and `forwardActivateObject` (`cfl.go:65,345-375`); the
whole-cone activation is gone. `pts`, `delta`, `doneComplex` are exactly `solve.go`'s
state. No reverse points-to index (`PB`) is introduced: writer-finding is done by forward
object tracking, so the tabling/magicsets `PB` table (and its awkward per-query reset) is
avoided entirely.

`storesWhereValue` is built once in the preload loop (`cfl.go:116`): for each `cStore(p,q)`,
append `p` to `storesWhereValue[q]`. `icallRecvsBySig[k] += c.p` for each `cICall`, where
`k = types.TypeString(sig, qualifier)` on the func-value's underlying `*types.Signature`
(§6). Both are `O(|constraints|)`.

Helper: `baseOf(o) = base[o] if base[o] >= 0 else o`.

## 2. The 8 forms → derivation rules (base+offset model)

`pts(N,O)` is the single relation; for a cell `N`, `pts(N,·)` is `contents`. Each rule below
is the lazy image of exactly one `solve.go` primitive (site pinned); the whole propagation
is `solveDiff`'s transfer function, gated by `active`. The only Dyck axis is field-balance,
realized by the shared cell: the `putfield[f] alias getfield[f]` bracket is matched not by
a separate alias search but by the store-pointer and load-pointer carrying the **same cell**
`o` in their `pts` (OOPSLA05 Def.1 cond.2, made structural).

| Form (`extract.go`) | shape | batch rule | backward feeder (`activate`) | forward firing (`run`) |
|---|---|---|---|---|
| `cAddr` | `p = &o` | `add(p,o)` `:95` | `addrOf[p]` → `emit(p,o)` | (none: seeds only) |
| `cCopy` | `p ⊇ q` | `addEdge(q,p)` `:97,265` | `copyBack[p]` → `activate(q)`, replay | `copyFwd[q]` → relay |
| `cGep` | `p = &q.off` | `add(dst,base+off)` `:206` | `gepBack[p]` → `activate(q)`, replayGep | `gepFwd[q]` → re-based relay |
| `cLoad` | `p = *q` | `addEdge(o,p)` `:201` | `loadBack[p]` → `activate(q)`; for `o∈pts[q]`: `loadPop(o,p)` | `loads[q]` → load rule (§5) |
| `cStore` | `*p = q` | `addEdge(q,o)` `:204` | (value side) `storesWhereValue` → `demandPT(p)` (§5) | `stores[p]` → writer-harvest (§5) |
| `cInvoke` | dispatch | `wire(methodOf(t,m),c,o)` `:228` | `callRet[dst]` → `activate(recv)`; param → caller-disc (§6) | `calls[recv]` → `fireCall` (§6) |
| `cICall` | fn-value call | `wire(objFunc[o],c,-1)` `:235` | `callRet` / caller-disc (§6) | `calls[fv]` → `fireCall` |
| `cTFilter` | `dst = src.(I)` | `add(dst,o)` if `Assignable` `:254` | `asrtBack[dst]` → `activate(src)`, replayAssert | `asserts[src]` → filtered copy |
| `cUntag` | `dst = src.(C)` | `addEdge(o,dst)` if `Identical` `:258` | `asrtBack[dst]` → `activate(src)`, replayAssert | `asserts[src]` → filtered load (payload = contents) |

`replayGep` and the gep firing carry the `off < objSize[base]` guard (`solve.go:207`,
`cfl.go:305`); an over-wide gep is dropped. `cTFilter` is object-preserving (a filtered
copy); `cUntag` routes the payload through the **same contents machinery as load** (`loadPop`).

## 3. Core primitives (semi-naive; identical arithmetic to `solve.go`)

```go
func (d *rsm) push(p int) { if !d.onWL[p] { d.onWL[p]=true; d.wl=append(d.wl,p) } }

func (d *rsm) emit(p, o int) {                     // == solve.go add() :59, demand-gated
    if p<0 || o<0 || !d.active[p] { return }        // THE DEMAND GATE (only diff from batch)
    if d.pts[p]==nil { d.pts[p]=map[int]bool{}; d.delta[p]=map[int]bool{} }
    if d.pts[p][o] { return }                        // membership guard -> termination
    d.pts[p][o]=true; d.delta[p][o]=true
    d.push(p)
}

func (d *rsm) subscribe(src, dst int) {            // dynamic subset edge, == addEdge :76
    d.dynFwd[src]=append(d.dynFwd[src], dst)
    for o := range d.pts[src] { d.relay(src, dst, o) }   // replay existing set (cfl.go:298)
}

func (d *rsm) relay(src, dst, o int) {             // forward-relay one object
    if d.fwdDone[d.baseOf(o)] { d.markActive(dst) }  // TIGHT WRITER-FINDING: grow a
    d.emit(dst, o)                                   // tracked object's forward cone,
}                                                     // relay-only (no backward feeders)
```

`emit` is the sole writer of `pts`. `markActive`/`activate` are v1's, unchanged in spirit
(`cfl.go:232,244`): `markActive` = relay-only; `activate` = relay **plus** install backward
feeders (`ptDone`-guarded, upgrades a forward-seen node later).

**The `markActive`-on-relay in `relay()` is the whole of Failure-A's fix.** v1's
`forwardActivateObject` called `activate(nd)` on every copy/gep-cone node (`cfl.go:358`),
multiplying the forward cone by each node's backward cone, the uniform 12-19%-of-N flood.
Here, a forward-cone node gets `markActive` (relay only); the only nodes that get full
`activate` are genuine backward pivots: the query seed, a **store value** (writer), a
**load pointer**, and **wired** params/args/rets (§5, §6). Footprint collapses to the
`slice.go` backward slice plus the relay-only forward surcharge (§9).

## 4. Worklist + subscription (no O(N) scan)

The worklist **is** the delta. A pop processes only a node's *new* objects (`delta[v]`,
consumed on pop), fires only that node's statically-indexed adjacency plus its dynamic
`dynFwd[v]`, and applies each complex `(v,o)` once (`doneComplex[v][o]`). Nothing iterates
all `n`. This replaces v1's `run()` full-`pts[v]` rescan on every pop (`cfl.go:481`) with
`solve.go`'s difference discipline (`solve.go:184-269`).

```go
func (d *rsm) run() {
  for len(d.wl) > 0 {
    v := pop(d.wl); d.onWL[v]=false
    dl := d.delta[v]; if len(dl)==0 { continue }
    d.delta[v] = map[int]bool{}                       // consume delta -> never rescan pts[v]

    // containment-tracking (§5): a tracked object landing in a cell tracks the cell
    if d.base[v] >= 0 {
      for o := range dl { if d.fwdDone[d.baseOf(o)] { d.demandContents(v); break } }
    }

    if d.hasComplex(v) {                               // loads/stores/geps/calls/asserts[v] nonempty
      dc := d.doneComplex[v]; if dc==nil { dc=map[int]bool{}; d.doneComplex[v]=dc }
      for o := range dl {
        if dc[o] { continue }; dc[o]=true              // fire each (v,o) once (== solve.go :195)
        for _,dst := range d.loads[v] {                // p = *v : LOAD-OUT GATE FIX (§5)
          if d.active[dst] || d.fwdDone[d.baseOf(o)] { d.markActive(dst); d.loadPop(o, dst) }
        }
        for _,val := range d.stores[v] {               // *v=val, o container: contents(o) ⊇ pts(val)
          d.activate(val); d.markActive(o); d.subscribe(val, o)   // writer-harvest (solve.go:204)
        }
        for _,g := range d.gepFwd[v] {                 // p = &v.off
          if ob:=d.baseOf(o); ob>=0 && g[1]<d.b.objSize[ob] {
            if d.active[g[0]] || d.fwdDone[ob] { d.markActive(g[0]); d.emit(g[0], ob+g[1]) }
          }
        }
        for _,ci := range d.calls[v]   { d.fireCall(d.b.cs[ci], o) }    // §6
        for _,ci := range d.asserts[v] { d.fireAssert(d.b.cs[ci], o) }
        if d.fwdDone[d.baseOf(o)] {                     // FIELD-PUSH (§5): tracked o at a store VALUE
          for _,p := range d.storesWhereValue[v] { d.activate(p) }      // demand the store pointer(s)
        }
      }
    }
    for _,w := range d.copyFwd[v] { for o := range dl { d.relay(v, w, o) } }
    for _,w := range d.dynFwd[v]  { for o := range dl { d.relay(v, w, o) } }
  }
}
```

Every reaction indexes into a per-node adjacency list built once from `b.cs`. Work per pop
is proportional to that node's degree over its delta; total per query is the footprint, not
`n`. `reset()` clears only `touched` (`cfl.go:530`), extended to clear `delta`,
`doneComplex`, `ptDone`, `fwdDone` for touched entries, all per-query, so cost stays
proportional to the previous query's footprint. `storesWhereValue`/`icallRecvsBySig` are
static, never reset.

## 5. The heap match: writer-finding and multi-hop completeness (folds attacks 2 & 4)

To fill `contents(o) = pts[o]` for a loaded cell `o`, the engine needs every store `*p=val`
with `o ∈ pts[p]`. Those writer-pointers are found by **forward-tracking `o`**: seed `o` at
its births `&o` and let it flow forward; wherever it lands in a store-pointer's pts, the
writer-harvest fires. The load, untag, and dispatch-return backward feeders are the demand
side; the run() rules above are the forward side.

```go
func (d *rsm) loadPop(o, dst int) {                   // dst = *ptr, o ∈ pts[ptr]: dst ⊇ contents(o)
    d.demandContents(o)                                // fill contents(o) from o's writers
    d.subscribe(o, dst)                                // contents(o) relays to dst (== addEdge(o,dst))
}

func (d *rsm) demandContents(o int) {                 // find writers of o's contents
    d.markActive(o)                                    // o must be active to HOLD contents
    b := d.baseOf(o)                                   //   (attack 2 secondary: v1 does this at :335)
    if d.fwdDone[b] { return }
    d.fwdDone[b] = true
    for _, a := range d.objAddr[b] { d.markActive(a); d.emit(a, b) }  // seed base at its births
}
```

Keying the seed off `base` (not `o`) is essential and inherited from v1 (`cfl.go:343`): a
sub-node `base+off` has no `&(base+off)`; its writers are reached from `&base` via the gep
that produced `&base.off`, which `relay()` follows forward.

**Two coupled mechanisms close the multi-hop chain** (the failing shape: `o` stored into
container `C`, `C` loaded back out into writer-pointer `p`, `*p=val`; `p` acquired `o`
through a store→load round-trip, so it is not on the query's backward load chain):

1. **Field-push, value side** (`run` above): when a **tracked** object `o` (`fwdDone[base(o)]`)
   lands in a store-**value** node `v` (`storesWhereValue[v] ∋ p` for `cStore(p,v)`), it must
   flow into `p`'s containers. But `p` may never have been demanded. `activate(p)` demands
   it, computing `pts[p] = the containers`. This is the store-**pointer** discovery v1
   lacked; without it the container-holder is never active and the store never fires.

2. **Containment-tracking** (`run` above): when a tracked object `o` lands in a **cell** `v`
   (`base[v] >= 0`, i.e. `o ∈ contents(v)`), the cell `v` is itself a container carrying a
   tracked object, so `demandContents(v)` tracks it. This propagates tracking **up the
   containment chain**: `track(o)` → `o ∈ contents(C)` → `track(C)` → `C ∈ contents(C')` →
   `track(C')` … Each newly-tracked container gets its births seeded, so it flows forward and
   its **load-out** fires.

3. **Load-out gate fix** (`run` load rule): the load `w = *q` with tracked container `o ∈ pts[q]`
   must carry `o`'s contents out into `w` even though `w` was never backward-demanded. v1
   gated on `active[dst]` alone (`cfl.go:495`), so the intermediate `w` was skipped. The fix
   is `active[dst] || fwdDone[base(o)]`: a tracked container's load-out `markActive`s the
   destination and relays contents through, escalating **only** along store→load brackets
   whose container currently holds a tracked object, so Failure A is not reintroduced.

Worked chain (4 heap constraints; the `&`-births `pc0,pc1→obj_c`, `pH,pH2→obj_H`,
`tPtr→obj_T` elided; batch: `pts[r]={obj_T}`):

```
cStore(pH, pc0)  *H=&c    obj_c ∈ contents(obj_H)
cLoad (p,  pH2)  p=*H     obj_c ∈ pts[p]            (obj_c out of obj_H)
cStore(p,  tPtr) *p=&T    obj_T ∈ contents(obj_c)   THE WRITER
cLoad (r,  pc1)  r=*c     obj_T ∈ pts[r]            the demanded read (dispatch receiver)
```

- `demandPT(r)` → `loadPop(obj_c, r)` → `demandContents(obj_c)` seeds `obj_c` at `pc0,pc1`.
- `obj_c` at `pc0` (store value) → **field-push** `activate(pH)` → `pts[pH]={obj_H}`.
- `obj_H` at `pH` (store ptr) → writer-harvest replays `pts[pc0]={obj_c}` into `obj_H` →
  `emit(obj_H, obj_c)`.
- `emit(obj_H, obj_c)` into cell `obj_H` with `obj_c` tracked → **containment-tracking**
  `demandContents(obj_H)` seeds `obj_H` at `pH,pH2`.
- `obj_H` at `pH2` (load ptr), `active[p]` false but `fwdDone[obj_H]` true → **load-out**
  `markActive(p)`, `loadPop(obj_H, p)` relays `contents(obj_H)={obj_c}` → `emit(p, obj_c)`.
- `obj_c` at `p` (store ptr) → writer-harvest `activate(tPtr)`, `contents(obj_c) ⊇ pts[tPtr]={obj_T}`.
- `contents(obj_c)` gains `obj_T`; the `subscribe(obj_c, r)` from step 1 relays → `emit(r, obj_T)`. Matches batch.

Depth is unbounded: each hop is the same `demandContents → field-push/containment-track →
writer-harvest → load-out` cycle, memoized by `fwdDone[base]`. `cUntag` is load-like and
rides the same machinery (`fireAssert` calls `loadPop`), so multi-hop through type-asserted
payloads closes too.

## 6. Interface dispatch woven in (folds attack 3)

Dispatch resolves as `pts[receiver]` grows, in both directions, with no precomputed call
graph. Forward and `wire` are v1's, unchanged (`cfl.go:423,447`; mirrors `solve.go:149,211`):
`fireCall` resolves `methodOf(objType[base[o]], method)` (invoke) or `objFunc[base[o]]`
(icall), then `wire` injects the receiver **object** into param0 (offsetting args by 1) and
subscribes args→params and callee-results→call-site-rets. Wired nodes are genuine pivots, so
they `activate`. The receiver-object-into-param0 quirk (`solve.go:154`, not a copy edge) and
icall's no-injection (`solve.go:230`) are preserved; getting these wrong breaks `pt==batch`.

**Backward, demanded results:** `callRet[dst]` (`activate`) demands the receiver so
resolution wires the callee result forward into `dst`.

**Backward, demanded params (caller discovery): the demand-leak fix.** A param `ps` is fed
by `wire` only when a call resolving to `ps`'s function fires; backward from `ps` the engine
must demand the receivers of calls that *could* target the function. v1 (`cfl.go:281-292`)
did this two ways:

- invoke: `invokesByMethod[method]`, already method-name scoped. Left as-is. It over-
  activates only for common method names (`Error`, `String`), which are exactly the error-
  typed tail the flood lives in; `slice.go`'s bimodality already treats those as flooded, so
  the over-activation matches the allowed cost, not a leak.
- icall: `if fnAddrTaken[fn] { for recv := range icallRecvs { activate(recv) } }`, method-
  name **independent**, so it activated *every* indirect-call receiver in the program, each
  with its full backward slice. When a domain query's slice merely crosses a param of any
  address-taken function (pervasive: every closure, every synthetic thunk), this dragged in
  the `*errors.errorString` flood. **This is the leak.**

Primary fix, the **signature filter**: a `cICall` `fv(args)` can resolve to `fn` only if
`fn`'s type is assignable to `fv`'s static type, and Go func assignability is signature
identity. So index icall receivers by canonical signature and demand only the compatible
bucket:

```go
// caller discovery for a demanded param of fn (replaces cfl.go:287-291)
if d.fnAddrTaken[fn] {
    for _, recv := range d.icallRecvsBySig[sigKey(fn)] { d.activate(recv) }
}
```

Sound (superset of real callers, since a real caller's func-value type is identical to
`fn`'s signature) and tight: a domain `fn func(Widget) int` no longer activates a
`func(error)` handler's receiver, so errorString is excluded, *unless* `fn`'s own signature
is error-adjacent, in which case the flood is genuinely in the slice (the allowed tail,
matching `slice.go`). `wire`'s `methodOf`/`objFunc` filter still prunes non-`fn` callers, so
the result stays exact; the filter only narrows the *footprint*.

Secondary fix, **tighten `fnAddrTaken`** (`cfl.go:194`): a closure whose `MakeClosure`
value flows only into a direct `Call` at its own site is not an icall target. A cheap
escape check (value node flows into a store/arg/return/copy that outlives the site, not just
an immediate direct call) drops most closures out of the address-taken set, closing the door
before the signature filter even runs. Belt-and-suspenders; the signature filter alone
already cuts the flood.

Not adopted: the candidate designs' "exact-probe" option (demand each candidate receiver's
pts, discard if it does not resolve to `fn`). The verifier showed it is *strictly worse*
than the leak, since `demandPT(receiver)` is full activation of every candidate's backward
slice *before* discarding, touching every icall receiver's slice including the error-typed
ones. Signature-filtering the candidate set before any activation is the correct move.

## 7. Termination (folds attack 1)

Pure monotone-Datalog / finite-Herbrand argument (grafted from magicsets, the cleanest of
the three), plus the operational guards:

1. **Finite domain.** Nodes and objects are fixed integer ranges (`b.nNodes`, `b.objSize`),
   minted only during extraction: `newDemand` reads `b.nNodes` once (`cfl.go:77`) and never
   mints. All relations range over finite domains: `pts, dynFwd ⊆ V×V`; `active, ptDone,
   fwdDone ⊆ V`. `base+off` is a finite lookup, never a constructor, so no term grows.
2. **Monotone, one-shot.** `emit` inserts each `(p,o)` at most once (membership guard);
   `activate`/`markActive`/`demandContents` are `ptDone`/`fwdDone`-idempotent; each complex
   `(v,o)` fires once (`doneComplex`); each subscription is a monotone add. Total `emit`s ≤
   `n·nObjs`; the worklist drains because every push follows a real insertion into some
   `delta[v]`, and `delta[v]` is consumed on pop.
3. **Cycles do not diverge.** A copy/gep cycle circulates until membership yields nothing new,
   then quiesces. A field cycle (`x=x.next`) is an ordinary Datalog cycle closed by the
   membership guard. A container cycle (`A ∈ contents(B)`, `B ∈ contents(A)`) is cut by
   `fwdDone`: `demandContents(A)` → track `B` → `demandContents(B)` → track `A` is a no-op
   the second time. The Sridharan-Bodik self-reference hazard (their `checkingAlias` guard,
   PLDI06 §4) **does not arise**: E36 never runs the exact `alias → flowsTo̅ flowsTo` search
   that recurses on itself; aliasing is resolved by shared-cell membership, which is
   monotone. No self-edge-removal approximation, hence no precision loss on cyclic fields
   (unlike Sridharan's `R_FT`).

Hardening (the attack's recommendation, since the corpora are dispatch-shaped and never
exercise deep cyclic fields): add three EQUIV regression fixtures (`x = x.next`; the two-node
field cycle `b1.next=b2; b2.next=b1` under a traversal phi; and mutually-recursive containers
`A ⊂ B, B ⊂ A`), plus a seed-reproducible termination tripwire asserting total worklist
pushes ≤ `n·nObjs + #demands`.

## 8. Equivalence: `pt(v) == batch-pt(v)`, demand-scoping loses nothing

Two composed facts.

**Propagation identity (soundness, `demand ⊆ batch`).** Every rule that writes `pts` (`emit`,
`relay`, `subscribe`, writer-harvest, gep, `wire`, `fireCall`, `fireAssert`) is character-
identical to a `solve.go` primitive (§2 pins each site), *except* `emit`'s `!active[p]` gate.
So on the active set, E36 computes the least fixpoint of exactly `solveDiff`'s transfer
function; `active` only withholds, never fabricates. By induction on insertion order every
demand fact is a batch fact.

**Activation = contribution closure (completeness, `batch ⊆ demand` on demanded `v`).**
`slice.go`'s `buildFeeds` (`slice.go:15`) *defines* the batch dependency relation: `pts[v]`
is a pure function of the backward-reachable set of `v` over `feeds`. Each `feeds` edge has a
realizer in the demand closure, checked per form:

- `cCopy/cGep p←q` (`slice.go:25`): `copyBack/gepBack` → `activate(q)` + relay.
- `cLoad p←q` and `p←o` for `o∈pts[q]` (`slice.go:27-31`): `loadBack` → `activate(q)`;
  `loadPop(o,p)` → `demandContents(o)` + `subscribe(o,p)`.
- `cStore o←q` for `o∈pts[p]` (`slice.go:32-35`): writer-harvest `subscribe(q,o)`; the
  container `o` is reached by forward-tracking, incl. multi-hop via §5: **the edges v1
  omitted, restored here.**
- `cInvoke/cICall` param←arg/recv, ret←callee-result (`slice.go:36-70`): `fireCall`/`wire`
  forward + `callRet`/signature-filtered caller-discovery backward.
- `cTFilter/cUntag dst←o` (`slice.go:71-87`): `asrtBack`/`asserts`.

Every `feeds` edge on `v`'s backward slice has a realizer, and no rule activates a node
outside the closure (each activation is triggered by a `feeds` predecessor). A node's LFP
value depends only on its contribution closure; E36 activates exactly that closure and runs
the identical transfer, so `pts[v]` reaches the same fixpoint as batch for every demanded
`v`. **Demand-scoping removes only nodes outside the closure, which by definition contribute
nothing.** Querying *all* nodes activates everything and reduces to `solveDiff` verbatim;
that is the EQUIV gate. (The magic-sets soundness+completeness meta-theorem, Beeri-
Ramakrishnan 1991, is the textbook backing for this: the demand transform is a complete
rewrite, not an approximation.)

The obligation this pins: the transform must be *faithful*, i.e. the forward flow must invert
**every** `pts`-rule, including the load and store inversions that are the multi-hop heap
edges. That fidelity is exactly what §5 supplies and v1 lacked; v1 was an unfaithful
transform (missing the store-value/load-out inversions), which is why its EQUIV-all passed
(all nodes active, the gate never bites) but scoped DISPATCH missed 7/513.

## 9. Complexity: tight for domain, allowed-large for flooded

Per query, work = `footprint(v) = backwardSlice(v) ∪ forwardSurcharge`, with `reset()`
proportional to footprint, not `n`.

- **`backwardSlice(v)`**: the `feeds` backward-reachable set (`slice.go`). Domain receivers:
  median 11 (wile), 6-37 across targets.
- **`forwardSurcharge`**: for each demanded cell, the forward flow of the tracked object out
  to its writer-pointers (`relay`'s `markActive`). This is the honest overhead vs the
  theoretical optimum: `slice.go` teleports to writers via resolved `pts`; the engine must
  traverse there (§5's `pH/obj_H/p` chain). It is **relay-only** (`markActive`, no backward
  cones), so it adds cone-sized, not cone×backward-cone, work. The design does not claim to
  hit the exact median-11; it claims footprint ≈ slice + a relay surcharge that stays small
  for domain objects.

Bimodal by construction, matching the measured `slice.go` footprint:

- **Domain query**: loaded objects have small forward cones → surcharge small → footprint ≈
  optimal slice. Not v1's uniform 12-19% of N.
- **Error-typed query**: `demandContents(*errors.errorString)` genuinely reaches its ~18k
  holders; the flood is *in the answer* (p90 = 28k-40k, 10-19% of receivers). Large here is
  correct, and paid only on genuinely error-adjacent queries: relay-cost, not v1's 18k
  activate-cone on every query that grazed a common cell.

Worst case (all nodes active) = `solveDiff`'s bound. E36 never exceeds batch.

## 10. Validation plan

Reuse `main.go`'s drivers (`EQUIV`, `DISPATCH`, `FOOTPRINT`).

- **EQUIV**: seed all nodes; `pts[v] == batch pts[v]` for every `v`. Must stay **126/126** on
  the dispatch corpora and **PASS** on soundfix-converged (89,303 nodes). This is §8 as a
  tripwire: any faithfulness bug under-approximates and it fires (drove v1 40→7→0). Add the
  three cyclic fixtures from §7.
- **DISPATCH**: seed only invoke/icall receivers; per-receiver `pts == batch`. Target
  **513/513 on soundfix** (v1: 506/513). The 7 fixalloc/scavenger/timer/callCleanup cases are
  the acceptance test for §5 (field-push + containment-tracking + load-out gate).
- **FOOTPRINT**: instrument `|touched|` per query; assert the median tracks `slice.go`'s
  optimal backward slice (≈11 on wile, not v1's uniform 12-19% of N) plus a bounded relay
  surcharge, and that flooded footprints occur **only** on receivers whose optimal slice
  reaches the god object (`sliceSize`'s `reachesGod`, `slice.go:94`). Engine footprint ≈
  optimal slice is the requirement-(b) acceptance test.
- **DEMAND-LEAK regression** (new, from attack 3): the 126 corpora are dispatch-shaped and
  never trip the caller-discovery door, so add an **adversarial corpus**: a domain interface
  with a domain method, whose receiver's backward slice crosses a param of an address-taken
  closure, embedded in a program that also has error-typed indirect calls (`cb(err)`,
  `cb func(error)`). Assert the domain query's footprint stays domain-sized (does **not**
  pull errorString), i.e. the signature filter holds. Without this corpus the footprint gate
  passes v1 unchanged and the leak is invisible.

## 11. Build order (minimal, testable delta from `cfl.go`)

1. Add `delta`/`doneComplex` and recast `run()` to the semi-naive loop (§4), replacing the
   full-`pts[v]` rescan (`cfl.go:481`). Queried over all nodes this alone must reproduce
   `solveDiff`; regression: EQUIV 126/126.
2. Replace `forwardActivateObject` (`cfl.go:345`) with `demandContents` (§5) and the
   `relay()` `markActive`-on-tracked discipline. Add `storesWhereValue`; add the field-push,
   containment-tracking, and load-out-gate rules to `run()`. Regression: DISPATCH 513/513 on
   soundfix (closes the 7) and the FOOTPRINT collapse.
3. Add `icallRecvsBySig` + `sigKey`; swap the icall caller-discovery branch (`cfl.go:287-291`)
   for the signature-filtered version; tighten `fnAddrTaken`. Regression: the adversarial
   demand-leak corpus.

Keep `methodOf`, `wire`, `fireCall`, forward `fireAssert` (minus its `forwardActivateObject`
call, replaced by `loadPop` for `cUntag`), and `reset()` verbatim.

## What Phase 1 excludes by design

No call/return Dyck (context sensitivity). Field-balance alone is exact and terminating
without hitting the `D_call ∩ D_field` undecidability (Reps 2000), because only one Dyck axis
is live and E31 pre-explodes it. The error-typed tail (the god object) is removable *only* by
context sensitivity at the source (E34), so it stays in Phase 1's footprint; that is Phase 2.

---

# Phase 2: on-demand context sensitivity (concrete outline)

## Goal and the shape of the win

Phase 1 relocates the errorString flood to the error-adjacent queries (10-19% of receivers)
but does not remove it: context-insensitive Andersen merges every `errors.New`/`fmt.Errorf`
allocation into one abstract `*errors.errorString` object, so any dispatch that can hold an
error pulls the program-wide union (E33/E34). Only context sensitivity removes it, by
splitting that one object per allocation context. Phase 2 adds context **on demand**, scoped
to the query, so the median query stays 11 nodes and only the flooded tail shrinks.

## The choice: k-bounded call-context, NOT the SPDS split

**Recommend k-bounded call-string context (with SCC-collapse for recursion). Do not build
the SPDS synchronized two-PDS.**

Rationale: E31 has already collapsed the field axis. SPDS's whole reason for two
synchronized pushdown systems (POPL19) is to keep the **field** axis exact without
`k`-limiting when fields are a live symbolic stack (`x.f₀.f₁…`), as in a variable-level PAG.
E31 pre-explodes the heap into flat `base+off` object nodes, so **there is no field-PDS to
synchronize**: the field axis is already exact set-membership. That collapses SPDS to just
its call-PDS `P_𝕊`, which is precisely call-string context. So the only pushdown dimension
Phase 2 needs is the call/return stack: a single stack, decidable, no `D_call ∩ D_field`
intersection (the field side is not a stack here). Reps' undecidability does not bite.

Trade-off named:
- **k-bounded call-string** (chosen): simple; one stack; the field axis needs no PDS at all.
  `k`-truncation loses precision on call chains deeper than `k`; recursion is handled not by
  truncation but by **SCC-collapse** (relabel `param`/`return` edges within a call-graph SCC
  as plain copies, Sridharan-Bodik PLDI06 §recursion), which keeps precision off-cycle and,
  because the CG is built on demand, collapses only the query-relevant SCCs.
- **SPDS two-PDS split** (rejected here): exact on both axes with no `k`, but its field-PDS is
  dead weight given the flat heap, and its per-allocation-site instances + alias-injection
  (Fig 8) are heavier machinery than the marginal precision (over `k` + SCC-collapse) buys on
  a representation that is already field-exact.

## Heap cloning gives `errors.New` per-call-site identity

Context sensitivity removes the flood only if the **heap abstraction** is also context-
sensitive. Extend the object model: `obj(key, t)` (`extract.go:134`) becomes
`obj(key, t, ctx)`, so the object allocated inside `errors.New` under caller-context `c₁` is
`(errorString@errors.New, c₁)`, distinct from `(…, c₂)`. A dispatch on an error at one site
then sees only the errors reachable in *its* context, not the program-wide union: the ~18k-
holder god object splits into per-context slivers, and a domain-ish site that merely grazed
one shared error object no longer pulls the whole union.

## Query-scoped context introduction

Contexts are introduced only along the query's backward call chain: this is the on-demand
context-sensitive call graph (Sridharan-Bodik Fig 7's `FINDPOINTSTO(x, c, visited)` with `c`
the call stack). From the demanded receiver, walk backward; at each `wire` boundary push the
call-site onto the context (bounded by `k`, or SCC-collapsed inside recursion); clone the
object model lazily per `(alloc-site, context-suffix)` as contexts are discovered. Nodes and
cells become `(node, ctx)` pairs, but only for the contexts the query traverses, so a domain
query that never enters `errors.New` clones nothing there, and the footprint stays Phase-1-
sized. The context threads through exactly `wire` and the dispatch resolution (`fireCall`),
the two sites Phase 1 already localizes.

## Validation: SOUNDNESS, not `== batch`

Context-sensitive `pts_cs(v, c)` is a **subset-refinement** of context-insensitive
`pts_ci(v)`: cloning splits one merged object into several, and any single context sees a
subset. So `pts_cs ≠ batch`, and the EQUIV `== batch` gate does **not** apply. Replace it:

1. **Soundness tripwire (no real callee dropped).** Merging all contexts back must recover
   Phase 1 exactly: `⊔_c pts_cs(v, c) == pts_ci(v)` for every `v`. If cloning ever drops a
   real flow, the collapse ≠ Phase-1 and the tripwire fires. This is the context-sensitive
   analogue of the EQUIV theorem: a mechanical check that refinement lost no edge. On the
   126 corpora it doubles as "no real dispatch callee dropped" (the callee set resolved
   context-sensitively, unioned over contexts, equals Phase 1's). Feasible on the corpora and
   soundfix; on wile/etcd the collapse check is whole-program and may only be sampled (see
   risks).
2. **Precision gain (the flooded tail shrinks).** On the 10-19% error-adjacent receivers of
   wile/etcd/ollama, measure the backward slice / receiver `|pts|` under context sensitivity.
   The metric of success: p90 slice size and the count of receivers whose slice reaches the
   god object drop materially, the flood splitting per context so that error-adjacent domain
   sites fall out of the flooded set. The median (already 11) must not regress.

---

# Open risks / where this could still fail (honest)

- **Forward surcharge can exceed the slice for shared containers.** §9's relay-only forward
  cone is small for domain objects, but a domain object stored into a *hot* container (one
  read by many loads) makes the containment-tracking + load-out `markActive` a wide set. The
  footprint is still sound and exact-in-result, but the `|touched|` gate could show a fat tail
  even on some domain queries. Only the FOOTPRINT run on wile/etcd/ollama settles whether the
  surcharge stays near-optimal; it is measured, not proven.
- **Multi-hop depth beyond the known 7.** The 7 soundfix failures are 2-hop. The §5 mechanism
  is depth-unbounded by construction (memoized recursion), but no corpus exhibits a 3+-hop
  chain, so completeness at greater depth is argued, not observed. A synthetic deep-chain
  fixture should be added to DISPATCH; until then a latent gap at depth ≥ 3 is possible.
- **Containment-tracking over-tracks in cyclic/aliased heaps.** Tracking every container that
  receives a tracked object is correct but can, on heavily-aliased structures, track a large
  set of containers (each `markActive` + seeded). Sound, but a surcharge amplifier distinct
  from the shared-container case above.
- **Signature filter is loose for generic signatures.** `func()`, `func(...interface{})`, and
  other very-general func types bucket many icall sites together, so a demanded param of such
  a function still activates a large candidate set. Signature identity is the tightest *sound*
  filter available in Go, so this residual is intrinsic; whether the wide buckets are rare is
  an empirical question the adversarial corpus only partly answers.
- **`fnAddrTaken` tightening is heuristic.** The escape check (§6 secondary) is a cheap
  approximation of "can this closure be indirectly called"; a mis-tightening would *under*-
  activate and fire EQUIV. It must be gated behind EQUIV before it is trusted, and if EQUIV is
  fragile against it, ship only the signature filter (which is sound on its own).
- **Phase 2 context explosion.** Heap cloning per call-site can blow up the object domain if
  contexts proliferate; `k` must be small (1-2) and SCC-collapse aggressive. E34 established
  that *only* context sensitivity removes the flood but did not measure the cost; whether
  small-`k` demand context shrinks the errorString tail *without* exploding the cloned domain
  is the central unproven Phase-2 bet.
- **Phase 2 soundness collapse is expensive at scale.** The `⊔_c pts_cs == pts_ci` tripwire is
  whole-program; with SCC-collapsed/unbounded-in-practice contexts it is finite but costly on
  wile/etcd, so it may only be sampled there, leaving a window where a refinement soundness
  bug shows on the big targets but not the corpora.
