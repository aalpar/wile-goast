# Refinement prototype report: context-insensitive goal-directed caller-discovery does NOT collapse the footprint

**Question (E35 v2 hypothesis):** does context-INSENSITIVE goal-directed
caller-discovery collapse the per-query footprint toward the optimal slice while
keeping the answer exact?

**Verdict: REFUTED — no collapse.** Forward-tracking the dispatch inflates the
footprint in every case measured (clean corpora 6→12, invokestress 13→21,
soundfix 22791→28161, fixtures 22731→27914), and additionally sacrifices
single-seed exactness on god-object-connected receivers (soundfix/fixtures PROBE
4→5, dropping a real `io/fs.dirInfo` caller). The refinement is committed
flag-gated and default OFF; it is a measured negative result, not a shipped
feature.

## The refinement design

The over-activation E37 diagnosed is `activate`'s eager caller-discovery
(rsm.go, invoke + icall branches): backward from a param `v` of function `fn`,
it activates the receiver of every *type-compatible* dispatch site and,
recursively, that receiver's whole backward cone. The answer stays exact because
forward `wire` only fires where `fn` actually dispatches, but the footprint
keeps every candidate receiver + cone.

The prototype (behind `REFINE`, default OFF) inverts this into forward-flow
discovery, the dispatch analog of Task 2's heap-object forward-tracking:

- `buildObjsForFn` (built once, only when REFINE is on) indexes, per function,
  the object bases that dispatch to it. Resolution mirrors `fireCall` exactly:
  - **icall:** bases `o` with `objFunc[o] == fn` (the func-value objects);
  - **invoke:** bases whose concrete type resolves a *demanded* method name to
    `fn` (`methodOf(objType, name) == fn`).
- Caller-discovery for a param of `fn` becomes `for ob := range objsForFn[fn]:
  demandContents(ob)` — reuse the existing forward primitive. `demandContents`
  sets `fwdDone[ob]`, seeds `ob` at its birth (`&ob`), and lets `relay` carry it
  forward, markActive-ing only the nodes on its forward path. A `cInvoke`/`cICall`
  arm fires only where a dispatching object arrives, so `wire` feeds `v` from
  exactly the real callers. Context-insensitive: no call strings, no cloning.

The change is additive and flag-gated; `extract.go`/`solve.go`/`slice.go` are
untouched; no HVN.

## REFINE OFF — shipped behavior preserved (byte-identical gates)

| Gate | Shipped HEAD | REFINE off (this build) |
|------|-------------|-------------------------|
| soundfix EQUIV | PASS (89303 nodes) | PASS (89303 nodes) |
| soundfix DISPATCH | 513/513 | 513/513 |
| soundfix PROBE | 4/263 (missing 246) | 4/263 (missing 246) |
| soundfix footprint median | 22790 | 22791 |
| 126-corpus EQUIV / DISPATCH / PROBE | 126/126 each | 126/126 each |

(The 22790→22791 and the `pushes` count are the known Go-map-order drift; every
gate verdict is identical. The default-OFF path is the shipped code, unmodified.)

## REFINE ON — correctness and footprint

### Correctness: exact when co-seeded; drops one caller on an isolated god-object query

| Target | EQUIV | DISPATCH | PROBE (single-seed) |
|--------|-------|----------|---------------------|
| 126 iface-decoy corpora | 126/126 | 126/126 | **126/126** |
| invokestress (clean, 119 nodes) | PASS | 6/6 | **0/6** |
| soundfix | PASS | 513/513 | **5/263** (was 4) |
| fixtures (cyclic/*, deepchain) | PASS | 510/510 | **5/259** (was 4) |

`DISPATCH` and `EQUIV` co-seed every receiver, so a caller missed by one query's
forward-track is supplied by another's — they stay exact everywhere. Only the
isolated single-seed `PROBE` exposes the drop, and only on the god-object-connected
targets. The new soundfix mismatch is:

```
recv n54567 batch|2| demand|1| (missing 1, extra 0) sample-type io/fs.dirInfo
```

The pre-existing 4/263 misses are `runtime`/`os` baggage from `import "fmt"`
(E37, Claim 1): forward-tracking a *merged* runtime object cannot reconstruct all
its writers on an isolated query. The refinement now routes **caller-discovery**
through that same forward-tracking, so it inherits the incompleteness and extends
it to one more receiver whose caller flows through a merged object. This is
intrinsic to "discover callers by forward-tracking dispatch objects" combined
with the unsolved merged-object forward-tracking gap (E34 established the god-set
is not pointer-equivalent, so there is no cheap completion). **Per the gate this
is a real answer change; it is reported, not shipped — the flag stays default OFF.**

### Footprint: inflates in every case — the opposite of collapse

| Target | optimal slice (median) | eager OFF (median / p90 / max) | REFINE ON (median / p90 / max) |
|--------|-----------------------|-------------------------------|--------------------------------|
| ifdecoy-d15k2-00x (clean) | 6 | **6 / 6 / 6** | **12 / 12 / 12** |
| invokestress (clean) | — | 13 / 13 / 13 | 21 / 21 / 21 |
| soundfix | 6 | 22791 / 23310 / 24637 | **28161 / 28185 / 30420** |
| cyclic/selffield | — | 22731 / 23221 / 24490 | 27914 / 28108 / 30354 |

The headline the spec hoped for — "clean receivers collapse 22790→N" — does not
exist, for a reason the measurement makes concrete:

1. **On clean corpora the eager engine is ALREADY optimal.** The iface-decoy
   footprint is median 6 = the optimal slice. The ~3800× gap the spec cites is
   *not* a general property of the eager engine; it is specific to soundfix,
   whose `import "fmt"` connects every dispatch receiver to the `runtime`
   god-graph. There is no clean subpopulation inside soundfix to collapse — the
   footprint distribution is tight (min≈median≈max), so *every* receiver pays the
   ~22-28k runtime floor regardless of direction.

2. **Forward-tracking is strictly more expensive than the backward slice.**
   Discovering callers backward pulls the receiver pointers' cones — which, for a
   clean receiver, *is* the optimal slice (hence OFF = 6). Forward-tracking a
   dispatch object instead sets `fwdDone[ob]`, which (a) propagates `ob` to *all*
   its uses, not just dispatch sites, and (b) demands `ob`'s CONTENTS via the
   heap-containment machinery (the `fwdDone` flag couples propagation to
   contents-tracking). That touched set is a superset of the backward slice, so
   the footprint doubles even where eager was optimal (6→12).

3. **The forward-cost caveat fired as predicted.** For ubiquitous methods
   (`String`/`Error`/`Read`) `objsForFn[fn]` is large and its objects flow widely
   through the same runtime graph; forward-tracking each floods worse than
   enumerating the sites (soundfix 22791→28161).

## Conclusion

Context-insensitive goal-directed caller-discovery **does not collapse the
footprint**. It inflates it in every case (2× on already-optimal clean receivers;
+24% on the god-object-flooded soundfix), and it trades away single-seed
exactness on god-object-connected receivers (PROBE 4→5). The premise — that
forward-flow discovery would touch less than backward site-enumeration — is
false: the backward slice IS the optimal for clean receivers, and forward-tracking
a dispatch object touches its entire forward reach plus its contents, a strictly
larger set. The soundfix over-activation is a `runtime` god-graph floor that
neither direction escapes context-insensitively; escaping it needs either
call/return context sensitivity (E35 v3) or a solved merged-object representation
(the HVN/god-set problem E34 left open), not a change of discovery direction.

The prototype stays committed, flag-gated, default OFF, as the negative-result
artifact.
