// E35 — demand-driven points-to: does querying only dispatch sites sidestep the
// errorString flood that makes batch Andersen intractable (E33/E34)?
//
// Pipeline: extract (frozen E31) -> batch solve (frozen; the equivalence oracle)
// -> demand engine. Two checks:
//
//	EQUIV (converged targets): query ALL nodes; demand pts must equal batch pts
//	  node-for-node. The equivalence theorem (context-insensitive field-sensitive
//	  CFL-reachability == Andersen) makes any mismatch a bug.
//	DISPATCH (all targets): query only the invoke/icall receiver nodes; report how
//	  many nodes the demand engine activates (its footprint) and whether the god
//	  object was pulled in.
package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"golang.org/x/tools/go/ssa"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cflreach <target-dir>")
		os.Exit(1)
	}
	dir := os.Args[1]
	budget := 120 * time.Second
	switch os.Getenv("BUDGET") {
	case "short":
		budget = 60 * time.Second
	case "long":
		budget = 600 * time.Second
	}

	b, prog := extract(dir)
	ptsBatch, godObj, converged := solveDiff(b, prog, budget)
	N := b.nNodes

	// ---- Task 0 flood-taxonomy dump (E36 Phase 2 gate; env-gated, additive) ----
	// Classifies the top-K god objects by holder count as GLOBAL (uncloneable
	// singleton) vs an allocation-site object (cloneable per calling context by
	// Phase 2's heap-cloning design). Does NOT touch extract.go/solve.go/slice.go.
	if os.Getenv("TAXONOMY") != "" {
		taxonomyDump(b, ptsBatch, 15)
	}

	// dispatch receiver nodes: the pointer of every invoke/icall constraint
	recvSet := map[int]bool{}
	for _, c := range b.cs {
		if (c.f == cInvoke || c.f == cICall) && c.p >= 0 {
			recvSet[c.p] = true
		}
	}
	var recvs []int
	for r := range recvSet {
		recvs = append(recvs, r)
	}
	sort.Ints(recvs)

	// ---- EQUIV: demand querying ALL nodes must reproduce batch exactly ----
	fmt.Printf("\n─── EQUIV (demand queries all nodes == batch) ───\n")
	if !converged {
		fmt.Printf("solve TIMED OUT — skipping full equivalence (batch pts is partial).\n")
		fmt.Printf("EQUIV: skipped\n")
	} else {
		d := newRSM(b, prog)
		all := make([]int, N)
		for i := range all {
			all[i] = i
		}
		t0 := time.Now()
		d.query(all)
		bound := int64(b.nNodes)*int64(b.nObjs) + d.nDemands
		if d.pushes > bound {
			fmt.Printf("TERMINATION: TRIPWIRE FIRED  pushes %d > bound %d (n*nObjs %d + demands %d)\n",
				d.pushes, bound, int64(b.nNodes)*int64(b.nObjs), d.nDemands)
		} else {
			fmt.Printf("TERMINATION: OK  pushes %d <= bound %d\n", d.pushes, bound)
		}
		agree, differ, missing, extra := 0, 0, 0, 0
		var firstDiffs []string
		for v := 0; v < N; v++ {
			bset, dset := ptsBatch[v], d.pts[v]
			if sameSet(bset, dset) {
				if len(bset) > 0 {
					agree++
				}
				continue
			}
			differ++
			m, e := diffCount(bset, dset)
			missing += m
			extra += e
			if len(firstDiffs) < 8 {
				firstDiffs = append(firstDiffs, fmt.Sprintf("n%d batch|%d| demand|%d| (missing %d, extra %d)",
					v, len(bset), len(dset), m, e))
			}
		}
		fmt.Printf("wall              %v\n", time.Since(t0).Round(time.Millisecond))
		fmt.Printf("non-empty agree   %d\n", agree)
		fmt.Printf("differ            %d  (demand-missing %d obj, demand-extra %d obj)\n", differ, missing, extra)
		for _, s := range firstDiffs {
			fmt.Printf("  DIFF %s\n", s)
		}
		if differ == 0 {
			fmt.Printf("EQUIV: PASS  (demand == batch on all %d non-empty nodes)\n", agree)
		} else {
			fmt.Printf("EQUIV: FAIL  (%d nodes differ)\n", differ)
		}
	}

	// ---- FLOOD ON DISPATCH SITES: how many receivers carry the god object ----
	// The crux, robust to any footprint looseness: demand can only avoid the flood
	// at the dispatch sites that are not error-typed. Computed from batch pts.
	fmt.Printf("\n─── FLOOD ON DISPATCH SITES (batch ground truth%s) ───\n", partialTag(converged))
	if godObj >= 0 {
		flooded, total, nonEmpty, mx := 0, 0, 0, 0
		for _, r := range recvs {
			s := ptsBatch[r]
			if len(s) == 0 {
				continue
			}
			nonEmpty++
			total += len(s)
			if len(s) > mx {
				mx = len(s)
			}
			if s[godObj] {
				flooded++
			}
		}
		mean := 0.0
		if nonEmpty > 0 {
			mean = float64(total) / float64(nonEmpty)
		}
		fmt.Printf("receivers         %d total, %d resolved\n", len(recvs), nonEmpty)
		fmt.Printf("flooded receivers %d / %d  (god object n%d in their pts)\n", flooded, nonEmpty, godObj)
		fmt.Printf("receiver |pts|    mean %.1f  max %d\n", mean, mx)
	}

	// ---- OPTIMAL DEMAND FOOTPRINT: the resolved-graph backward slice ----
	// The exact set of nodes whose pts feeds a receiver — what an optimal demand
	// engine touches, with no over-activation. Computed from batch's own pts.
	fmt.Printf("\n─── OPTIMAL DEMAND FOOTPRINT (resolved-graph backward slice%s) ───\n", partialTag(converged))
	dd := newRSM(b, prog)
	t1 := time.Now()
	feeds := dd.buildFeeds(ptsBatch)
	mark := make([]int, N)
	for i := range mark {
		mark[i] = -1
	}
	var foots []int
	pullGod := 0
	for i, r := range recvs {
		if len(ptsBatch[r]) == 0 {
			continue // unresolved receiver, nothing to slice
		}
		sz, god := sliceSize(feeds, r, godObj, mark, i)
		foots = append(foots, sz)
		if god {
			pullGod++
		}
	}
	sort.Ints(foots)
	mx := 0
	if len(foots) > 0 {
		mx = foots[len(foots)-1]
	}
	fmt.Printf("wall              %v\n", time.Since(t1).Round(time.Millisecond))
	fmt.Printf("resolved recvs    %d\n", len(foots))
	fmt.Printf("slice (nodes)     median %d  p90 %d  max %d   (of %d)\n", pctile(foots, 50), pctile(foots, 90), mx, N)
	fmt.Printf("slice (%% of N)    median %.3f  p90 %.3f  max %.3f\n",
		100*float64(pctile(foots, 50))/float64(N), 100*float64(pctile(foots, 90))/float64(N), 100*float64(mx)/float64(N))
	if godObj >= 0 {
		fmt.Printf("reach the flood   %d / %d receivers' slices contain god object n%d\n", pullGod, len(foots), godObj)
	}

	// ---- ENGINE FOOTPRINT: the engine's actual per-receiver activation ----
	// PROBE (single-seed correctness gate): each receiver is queried ALONE, which
	// IS the demand engine's real use case. Compare de.pts[r] to batch pts[r]; a
	// mismatch means backward caller-discovery under- (missing) or over-approx
	// (extra) on a single seed. EQUIV/DISPATCH co-seed every node and cannot see
	// this. Gated on converged: a partial batch makes the comparison meaningless.
	fmt.Printf("\n─── ENGINE FOOTPRINT (rsm |touched| per receiver) ───\n")
	de := newRSM(b, prog)
	var efoots []int
	probeN, probeMiss, probeMissing, probeExtra := 0, 0, 0, 0
	var probeFails []string
	// ---- Task 0 follow-up: PROBEDIAG (E36 Phase 2 gate; env-gated, additive) ----
	// For every PROBE-mismatching receiver, classify EVERY missed object
	// (batch_pts(r) \ demand_pts(r), collapsed node->object) by ORIGIN KIND, to
	// settle whether the plan's §2.2 "context-collapsed, cloneable via per-
	// context heap cloning" claim actually holds for soundfix's 4/263 residual.
	probeDiag := os.Getenv("PROBEDIAG") != ""
	var diagBase []int
	var diagKeyOf map[int]ssa.Value
	diagTotals := map[string]int{}
	if probeDiag {
		diagBase, diagKeyOf = objectBaseMaps(b)
	}
	for _, r := range recvs {
		if len(ptsBatch[r]) == 0 {
			continue
		}
		de.query([]int{r})
		efoots = append(efoots, len(de.touched))
		if converged {
			probeN++
			if !sameSet(ptsBatch[r], de.pts[r]) {
				probeMiss++
				m, e := diffCount(ptsBatch[r], de.pts[r])
				probeMissing += m
				probeExtra += e
				if os.Getenv("DUMPPROBE") != "" && len(probeFails) < 16 {
					var ty string
					for o := range ptsBatch[r] {
						if ob := de.base[o]; ob >= 0 && de.b.objType[ob] != nil {
							ty = de.b.objType[ob].String()
							break
						}
					}
					probeFails = append(probeFails, fmt.Sprintf("recv n%d batch|%d| demand|%d| (missing %d, extra %d) sample-type %s",
						r, len(ptsBatch[r]), len(de.pts[r]), m, e, ty))
				}
				if probeDiag {
					dumpProbeDiag(b, r, ptsBatch[r], de.pts[r], diagBase, diagKeyOf, diagTotals)
				}
			}
		}
		de.reset()
	}
	sort.Ints(efoots)
	emx := 0
	if len(efoots) > 0 {
		emx = efoots[len(efoots)-1]
	}
	fmt.Printf("engine footprint  median %d  p90 %d  max %d   (of %d)\n",
		pctile(efoots, 50), pctile(efoots, 90), emx, N)
	if converged {
		for _, s := range probeFails {
			fmt.Printf("  PROBE-DIFF %s\n", s)
		}
		fmt.Printf("PROBE: %d / %d isolated single-seed queries MISMATCH batch  (missing %d, extra %d)\n",
			probeMiss, probeN, probeMissing, probeExtra)
		if probeDiag {
			fmt.Printf("\n─── PROBEDIAG TOTAL (missed-object origin kinds, all mismatching receivers) ───\n")
			var kinds []string
			for k := range diagTotals {
				kinds = append(kinds, k)
			}
			sort.Strings(kinds)
			total := 0
			for _, k := range kinds {
				fmt.Printf("  %-14s %d\n", k, diagTotals[k])
				total += diagTotals[k]
			}
			fmt.Printf("  %-14s %d\n", "TOTAL", total)
		}
	}

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
}

func partialTag(converged bool) string {
	if converged {
		return ""
	}
	return " — PARTIAL solve, indicative"
}

func pctile(sorted []int, p int) int {
	if len(sorted) == 0 {
		return 0
	}
	i := p * len(sorted) / 100
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}

func diffCount(a, b map[int]bool) (aMinusB, bMinusA int) {
	for k := range a {
		if !b[k] {
			aMinusB++
		}
	}
	for k := range b {
		if !a[k] {
			bMinusA++
		}
	}
	return
}

func sameSet(a, b map[int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// taxonomyDump is Task 0 of the E36 Phase 2 flood-taxonomy gate. It reports
// the top-K god objects by holder count and classifies each by ORIGIN KIND,
// derived from the dynamic type of the ssa.Value that keyed its allocation in
// b.objBase (extract.go's obj(key, t)):
//
//	GLOBAL        *ssa.Global        — one object, no allocation context, UNCLONEABLE
//	ALLOC         *ssa.Alloc         — allocation site, cloneable per calling context
//	MAKEINTERFACE *ssa.MakeInterface — allocation site, cloneable per calling context
//	FUNC          *ssa.Function      — a function/closure object
//	<other>       anything else      — printed by concrete Go type, not silently bucketed
//
// This is the cloneability signal Phase 2's heap-cloning needs: an ALLOC or
// MAKEINTERFACE inside errors.New/fmt.Errorf, shared by many callers, is
// exactly the merge Phase 2 splits. A GLOBAL has no allocation context and
// cannot be split by per-context cloning.
//
// Reads only b.objBase/objType (already retained by the frozen extractor) and
// the ptsBatch returned by solveDiff; does not edit or reach into
// extract.go/solve.go internals. Sorted for PRESENTATION only — the analysis
// itself (holder-count accumulation) iterates ptsBatch/objBase in whatever
// order Go gives, same as the rest of the (unsorted-by-design) engine.
func taxonomyDump(b *builder, ptsBatch []map[int]bool, k int) {
	base, keyOf := objectBaseMaps(b)

	// Holder counts, aggregated to object base: how many pts sets (of ALL N
	// nodes, batch ground truth) contain some node of this object.
	holders := map[int]int{}
	for _, s := range ptsBatch {
		for o := range s {
			ob := base[o]
			if ob < 0 {
				ob = o
			}
			holders[ob]++
		}
	}

	type row struct {
		base  int
		count int
	}
	rows := make([]row, 0, len(holders))
	for ob, c := range holders {
		rows = append(rows, row{ob, c})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count != rows[j].count {
			return rows[i].count > rows[j].count
		}
		return rows[i].base < rows[j].base // deterministic tie-break, presentation only
	})
	if k > len(rows) {
		k = len(rows)
	}

	fmt.Printf("\n─── FLOOD TAXONOMY (top %d god objects by holder count) ───\n", k)
	for i := 0; i < k; i++ {
		ob, cnt := rows[i].base, rows[i].count
		t := b.objType[ob]
		kind, detail := classifyOrigin(keyOf[ob])
		fmt.Printf("  #%-2d base=n%-8d holders=%-8d kind=%-14s type=%-40v %s\n", i+1, ob, cnt, kind, t, detail)
	}
}

// classifyOrigin reports the ORIGIN KIND of an object's key and a detail
// string: for GLOBAL, the global's full (package-qualified) name; for
// ALLOC/MAKEINTERFACE, the enclosing function (the cloneability signal — an
// alloc inside errors.New/fmt.Errorf shared by many callers is exactly the
// multi-allocation merge Phase 2 splits); for FUNC, the function's full name.
// Anything unexpected is reported by its concrete Go type, never silently
// bucketed, plus its enclosing function if it has one.
func classifyOrigin(key ssa.Value) (kind, detail string) {
	switch v := key.(type) {
	case *ssa.Global:
		return "GLOBAL", fmt.Sprintf("global=%s (UNCLONEABLE singleton)", v.String())
	case *ssa.Alloc:
		return "ALLOC", fmt.Sprintf("enclosing-func=%s", parentName(v))
	case *ssa.MakeInterface:
		return "MAKEINTERFACE", fmt.Sprintf("enclosing-func=%s", parentName(v))
	case *ssa.Function:
		return "FUNC", fmt.Sprintf("func=%s", v.String())
	case nil:
		return "UNKNOWN", "no key found in objBase (unexpected)"
	default:
		detail := fmt.Sprintf("go-type=%T", key)
		if p, ok := key.(interface{ Parent() *ssa.Function }); ok {
			detail += fmt.Sprintf(" enclosing-func=%s", parentName(p))
		}
		return "OTHER", detail
	}
}

func parentName(v interface{ Parent() *ssa.Function }) string {
	fn := v.Parent()
	if fn == nil {
		return "<nil>"
	}
	return fn.String()
}

// objectBaseMaps recomputes, from the frozen extractor's retained
// b.objBase/b.objSize, (1) node -> owning-object-base (any node inside an
// object's flat [base, base+size) range collapses to that object's base,
// same as solve.go/rsm.go/taxonomyDump) and (2) object-base -> the ssa.Value
// key that allocated it (the inverse of b.objBase; injective, since
// obj(key, t) allocates a fresh base the first time a given key is seen).
// Shared by taxonomyDump and dumpProbeDiag so the base/keyOf derivation is
// defined exactly once.
func objectBaseMaps(b *builder) (base []int, keyOf map[int]ssa.Value) {
	n := b.nNodes
	base = make([]int, n)
	for i := range base {
		base[i] = -1
	}
	for bs, size := range b.objSize {
		for off := 0; off < size; off++ {
			if bs+off < n {
				base[bs+off] = bs
			}
		}
	}
	keyOf = make(map[int]ssa.Value, len(b.objBase))
	for key, bse := range b.objBase {
		keyOf[bse] = key
	}
	return base, keyOf
}

// dumpProbeDiag is the PROBEDIAG=1 follow-up to Task 0: for ONE PROBE-
// mismatching receiver r, it computes the set difference batch_pts(r) \
// demand_pts(r) — the objects an isolated single-seed query MISSES relative
// to the batch oracle — collapses each missing NODE to its owning OBJECT
// base (a struct object may contribute several tracked field nodes; the
// cloneability question in plan §2.2/§6.3 is per-OBJECT, not per-node), and
// classifies every distinct missed object by ORIGIN KIND via classifyOrigin.
//
// This is the data Task 0's follow-up needs: plan §2.2 claims the residual
// is "context-collapsed merged runtime objects" fixable by per-context heap
// cloning. Per Task 0's own taxonomy, heap cloning can only split an ALLOC /
// MAKEINTERFACE (an allocation site, cloneable per calling context) — never
// a GLOBAL (one object, no allocation context to clone). If the missed set
// here is GLOBAL-dominated, that mechanism claim is false for this residual.
//
// diagTotals accumulates GLOBAL/ALLOC/MAKEINTERFACE/FUNC/OTHER/UNKNOWN
// counts across all mismatching receivers in the caller's loop, so the
// caller can print one TOTAL summary after processing every receiver.
func dumpProbeDiag(b *builder, r int, batchSet, demandSet map[int]bool, base []int, keyOf map[int]ssa.Value, diagTotals map[string]int) {
	missingObjs := map[int]bool{}
	missingNodes := 0
	for o := range batchSet {
		if demandSet[o] {
			continue
		}
		missingNodes++
		ob := base[o]
		if ob < 0 {
			ob = o
		}
		missingObjs[ob] = true
	}
	var bases []int
	for ob := range missingObjs {
		bases = append(bases, ob)
	}
	sort.Ints(bases) // presentation only; classification itself is order-independent

	fmt.Printf("  PROBEDIAG n%-8d %d missing nodes -> %d distinct missed objects\n", r, missingNodes, len(bases))
	for _, ob := range bases {
		t := b.objType[ob]
		kind, detail := classifyOrigin(keyOf[ob])
		diagTotals[kind]++
		fmt.Printf("    base=n%-8d kind=%-14s type=%-40v %s\n", ob, kind, t, detail)
	}
}
