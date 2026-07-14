// E34 — the offline-HVN diagnostic.
//
// Open question from E33: the copy graph is nearly acyclic, so CYCLE
// elimination collapses nothing. But HVN's pointer-equivalence is broader than
// cycle-equivalence (it also merges DAG-equivalent nodes). Do the ~20,700
// god-set flood nodes collapse into few PE classes under offline HVN, or many?
//
//	Few classes  -> HVN stores the flood set once, not ~20,000 times. HVN is
//	                the lever after all (overturns C7's 1.3x dismissal).
//	Many classes -> HVN cannot help; the only remaining lever is context
//	                sensitivity (attack errors.New's flood at its source).
//
// Pipeline: extract (frozen E31) -> partial solve (identify flood carriers) ->
// offline HVN (label) -> cross-tabulate labels against the carriers. A
// per-class points-to-uniformity check validates HVN soundness on converged
// (corpus) targets.
package main

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/tools/go/ssa"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: hvn <target-dir>")
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
	pts, godObj, converged := solveDiff(b, prog, budget)

	t0 := time.Now()
	res := runHVN(b)
	tHVN := time.Since(t0)

	N := b.nNodes
	nIndirect := 0
	for i := 0; i < N; i++ {
		if res.indirect[i] {
			nIndirect++
		}
	}

	fmt.Printf("\n─── OFFLINE HVN ───\n")
	fmt.Printf("wall              %v\n", tHVN.Round(time.Millisecond))
	fmt.Printf("main nodes        %d\n", N)
	fmt.Printf("PE classes        %d\n", res.nLabels)
	fmt.Printf("node reduction    %.2f%%   (%d -> %d)\n",
		100*(1-float64(res.nLabels)/float64(N)), N, res.nLabels)
	fmt.Printf("indirect nodes    %d  (%.1f%% — fresh label, HVN-unmergeable)\n",
		nIndirect, 100*float64(nIndirect)/float64(N))

	// ---- the flood carriers ----
	fmt.Printf("\n─── GOD-SET COLLAPSE (the E33 question) ───\n")
	if godObj < 0 {
		fmt.Printf("no god object found (empty solve?)\n")
	} else {
		var god []int
		for i := 0; i < N; i++ {
			if pts[i] != nil && pts[i][godObj] {
				god = append(god, i)
			}
		}
		reportSet(fmt.Sprintf("carriers of god object n%d", godObj), god, res)
	}

	// ---- the fat-set, at several thresholds (robustness) ----
	fmt.Printf("\n─── FAT-SET COLLAPSE (by |pts| threshold) ───\n")
	for _, thr := range []int{64, 256, 1024, 2048} {
		var fat []int
		for i := 0; i < N; i++ {
			if len(pts[i]) >= thr {
				fat = append(fat, i)
			}
		}
		if len(fat) == 0 {
			continue
		}
		reportSet(fmt.Sprintf("|pts| >= %d", thr), fat, res)
	}

	// ---- classify the nodes HVN calls non-pointer but the solver floods ----
	if os.Getenv("DUMPNP") != "" {
		dumpNonPointerFlood(b, pts, res, N)
	}

	// ---- soundness: every PE class must have uniform solved pts ----
	fmt.Printf("\n─── SOUNDNESS (PE class <=> identical pts) ───\n")
	soundness(pts, res, N, converged)
}

// reportSet prints how a node set distributes over PE classes: how many members
// are indirect (unmergeable by construction) vs direct, and how many distinct
// PE classes they occupy. The last number is how many times HVN stores the set.
func reportSet(name string, s []int, res peResult) {
	if len(s) == 0 {
		fmt.Printf("%-32s (empty)\n", name)
		return
	}
	labels := map[int]bool{}
	directLabels := map[int]bool{}
	nInd := 0
	for _, i := range s {
		labels[res.label[i]] = true
		if res.indirect[i] {
			nInd++
		} else {
			directLabels[res.label[i]] = true
		}
	}
	fmt.Printf("%-32s nodes %6d   indirect %6d (%.0f%%)   PE classes %6d   (direct-only classes %d)\n",
		name, len(s), nInd, 100*float64(nInd)/float64(len(s)), len(labels), len(directLabels))
}

// soundness checks the defining property of HVN: two nodes share a PE label iff
// they must have identical points-to sets. So every PE class must be uniform in
// the solved pts. A violation = HVN over-merged = a soundness bug (modulo the
// dead-code *p-cycle imprecision noted in hvn.go's CROSS-CHECK). Authoritative
// only when the solve CONVERGED; on a timed-out solve pts is partial.
func soundness(pts []map[int]bool, res peResult, N int, converged bool) {
	byLabel := map[int][]int{}
	npFlood := 0 // nodes HVN called non-pointer that the solver gave a pts
	for i := 0; i < N; i++ {
		if len(pts[i]) == 0 {
			continue // only pointer nodes with a solved set are constrained
		}
		if res.label[i] == nonPointer {
			npFlood++
			continue // not an equivalence class; the claim is "empty pts"
		}
		byLabel[res.label[i]] = append(byLabel[res.label[i]], i)
	}
	// The exact per-class pts comparison is O(class size x |pts|); on a timed-out
	// solve the god-set classes are huge and the pts is partial (non-authoritative
	// anyway), so skip it unless the solve converged or SOUND=1 forces it.
	full := converged || os.Getenv("SOUND") != ""

	classes, multi, bad, badNodes := 0, 0, 0, 0
	chainBad, incompBad := 0, 0 // subset-ordered (sound superset) vs incomparable (over-merge)
	for lbl, ids := range byLabel {
		classes++
		if len(ids) < 2 {
			continue
		}
		multi++
		if !full {
			continue
		}
		ref := pts[ids[0]]
		uniform := true
		for _, j := range ids[1:] {
			if !sameSet(ref, pts[j]) {
				uniform = false
			}
		}
		if !uniform {
			bad++
			badNodes += len(ids)
			// classify: a chain (all sets subset-comparable pairwise) is the
			// documented dead-code *p-cycle imprecision (sound superset). An
			// incomparable pair is a genuine over-merge (a real bug).
			if chainOrdered(pts, ids) {
				chainBad++
			} else {
				incompBad++
			}
			if os.Getenv("DUMPBAD") != "" {
				fmt.Printf("  bad class p%d: %d nodes, |pts| =", lbl, len(ids))
				for _, j := range ids {
					fmt.Printf(" %d", len(pts[j]))
				}
				fmt.Printf("  chain=%v\n", chainOrdered(pts, ids))
			}
		}
	}
	verdict := "CONVERGED — authoritative"
	if !converged {
		verdict = "TIMED OUT — partial pts, indicative only"
	}
	fmt.Printf("solve             %s\n", verdict)
	fmt.Printf("non-ptr w/ pts    %d  (HVN said empty, solver disagrees — must be 0)\n", npFlood)
	fmt.Printf("non-empty classes %d  (multi-member %d)\n", classes, multi)
	if !full {
		fmt.Printf("per-class check   SKIPPED (solve timed out; set SOUND=1 to force). npFlood above is still authoritative.\n")
		if npFlood == 0 {
			fmt.Printf("VERDICT           no non-pointer floods on the partial solve (indicative)\n")
		} else {
			fmt.Printf("VERDICT           %d non-pointer floods — inspect\n", npFlood)
		}
		return
	}
	fmt.Printf("over-merged       %d classes  (%d nodes)\n", bad, badNodes)
	fmt.Printf("  subset-ordered   %d  (sound superset — the documented dead-code *p-cycle case)\n", chainBad)
	fmt.Printf("  incomparable     %d  (genuine over-merge — a real HVN bug)\n", incompBad)
	switch {
	case bad == 0 && npFlood == 0:
		fmt.Printf("VERDICT           HVN is sound on this target (every PE class has uniform pts)\n")
	case incompBad == 0 && npFlood == 0:
		fmt.Printf("VERDICT           SOUND — all non-uniform classes are subset-ordered (known HVN imprecision)\n")
	default:
		fmt.Printf("VERDICT           UNSOUND — %d incomparable over-merges, %d non-ptr floods — HVN BUG, do not trust counts\n", incompBad, npFlood)
	}
}

// chainOrdered reports whether the solved pts of the given nodes form a total
// order under set inclusion. A non-uniform class that is chain-ordered is the
// documented HVN imprecision (a superset from a dead-code *p cycle), which is
// sound. An incomparable pair means two genuinely-different pointers were
// merged — a real over-merge bug.
func chainOrdered(pts []map[int]bool, ids []int) bool {
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			a, b := pts[ids[i]], pts[ids[j]]
			if !subset(a, b) && !subset(b, a) {
				return false
			}
		}
	}
	return true
}

// subset reports a ⊆ b.
func subset(a, b map[int]bool) bool {
	if len(a) > len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// dumpNonPointerFlood classifies nodes HVN labeled non-pointer (nonPointer)
// that nonetheless have a non-empty solved pts — the signature of a missing
// offline edge or indirect mark. It reverse-maps each node against the
// builder's structures to reveal what kind of SSA node the offline model missed.
func dumpNonPointerFlood(b *builder, pts []map[int]bool, res peResult, N int) {
	// reverse maps
	valOf := map[int]ssa.Value{}
	for v, id := range b.nodeOf {
		valOf[id] = v
	}
	objSpan := map[int]int{} // node -> obj base
	for base, size := range b.objSize {
		for k := 0; k < size; k++ {
			objSpan[base+k] = base
		}
	}
	isParam := map[int]bool{}
	for _, ps := range b.fnParams {
		for _, p := range ps {
			isParam[p] = true
		}
	}
	isResult := map[int]bool{}
	for _, rr := range b.fnResults {
		for _, r := range rr {
			isResult[r] = true
		}
	}
	tupleSpan := map[int]int{}
	for _, base := range b.tupleBase {
		tupleSpan[base] = base // just membership; size not tracked here
	}

	fmt.Printf("\n─── NON-POINTER FLOOD (HVN says empty, solver disagrees) ───\n")
	shown := 0
	for id := 0; id < N; id++ {
		if res.label[id] != nonPointer || len(pts[id]) == 0 {
			continue
		}
		kind := "?"
		switch {
		case objSpan[id] != 0 || b.objSize[id] > 0:
			kind = "OBJECT-subnode"
		case isParam[id]:
			kind = "param"
		case isResult[id]:
			kind = "func-result"
		}
		var desc string
		if sv, ok := valOf[id]; ok {
			desc = fmt.Sprintf("%T %s", sv, sv.Name())
			if p := sv.Parent(); p != nil {
				desc += "  in " + p.String()
			}
		}
		fmt.Printf("  n%-8d |pts|=%-4d indirect=%-5v  %-14s  %s\n",
			id, len(pts[id]), res.indirect[id], kind, desc)
		shown++
		if shown >= 40 {
			fmt.Printf("  ... (truncated)\n")
			break
		}
	}
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
