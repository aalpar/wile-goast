// E35 — demand-driven points-to: does querying only dispatch sites sidestep the
// errorString flood that makes batch Andersen intractable (E33/E34)?
//
// Pipeline: extract (frozen E31) -> batch solve (frozen; the equivalence oracle)
// -> demand engine. Two checks:
//   EQUIV (converged targets): query ALL nodes; demand pts must equal batch pts
//     node-for-node. The equivalence theorem (context-insensitive field-sensitive
//     CFL-reachability == Andersen) makes any mismatch a bug.
//   DISPATCH (all targets): query only the invoke/icall receiver nodes; report how
//     many nodes the demand engine activates (its footprint) and whether the god
//     object was pulled in.
package main

import (
	"fmt"
	"os"
	"sort"
	"time"
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
		d := newDemand(b, prog)
		all := make([]int, N)
		for i := range all {
			all[i] = i
		}
		t0 := time.Now()
		d.query(all)
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
	dd := newDemand(b, prog)
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

	// ---- corpora correctness gate (converged only): full receiver equivalence ----
	if converged {
		dd2 := newDemand(b, prog)
		dd2.query(recvs)
		diff := 0
		for _, r := range recvs {
			if !sameSet(ptsBatch[r], dd2.pts[r]) {
				diff++
			}
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
