package main

import (
	"fmt"
	"go/types"
	"os"
	"runtime"
	"sort"
	"time"

	"golang.org/x/tools/container/intsets"
	"golang.org/x/tools/go/ssa"
)

// solveDiff computes lfp(F) with DIFFERENCE PROPAGATION, exactly as the map-backed
// version did. The ONLY change is the carrier's REPRESENTATION: every set is an
// intsets.Sparse (a sparse bit vector) instead of a map[int]bool.
//
// Nothing about the algebra moves. Same six constraint forms, same transfer functions,
// same lfp, same permitted schedule. Cycle elimination and HVN are still NOT used: they
// rewrite the constraint graph, and the design bars them.
//
// The point of the swap: with a map, propagating a delta along a copy edge costs one
// hash lookup + one hash insert PER OBJECT. With a sparse bitset it costs one 64-bit OR
// per 64 objects, block-aligned. The lattice element is the same set either way.
// intsets.Sparse is also precisely what go/pointer stores its own points-to sets in
// (pointer/solve.go: `pts nodeset`, `prevPTS nodeset`).
func solveDiff(b *builder, prog *ssa.Program, budget time.Duration) {
	n := b.nNodes
	pts := make([]intsets.Sparse, n)
	delta := make([]intsets.Sparse, n)
	edge := make([]intsets.Sparse, n)
	doneComplex := make([]intsets.Sparse, n)

	base := make([]int, n)
	for i := range base {
		base[i] = -1
	}
	for bs, size := range b.objSize {
		for k := 0; k < size; k++ {
			if bs+k < n {
				base[bs+k] = bs
			}
		}
	}

	geps := make([][][2]int, n)
	loads := make([][]int, n)
	stores := make([][]int, n)
	calls := make([][]int, n) // invoke + icall constraint indices, keyed on the pointer

	// props counts elements newly inserted into some pts(). SAME DEFINITION as the map
	// version, so it is directly comparable to E30's 151.1M.
	// setops counts SET-level union operations: the work unit a bitset carrier actually
	// performs, and the one a Scheme solver would have to perform.
	// elemops counts the element-at-a-time operations that survive.
	var props, setops, elemops int64

	onWL := make([]bool, n)
	wl := []int{}

	push := func(p int) {
		if !onWL[p] {
			onWL[p] = true
			wl = append(wl, p)
		}
	}

	add := func(p, o int) {
		if p < 0 || o < 0 {
			return
		}
		elemops++
		if !pts[p].Insert(o) {
			return
		}
		delta[p].Insert(o)
		props++
		push(p)
	}

	// unionInto is `add` lifted from one element to a whole set. It adds every element
	// of s to pts(p), routes the NEWLY ADDED ones into delta(p), and wakes p. Semantics
	// are identical to looping add() over s; the work is block-wise instead of
	// per-element.
	var scratch intsets.Sparse
	unionInto := func(p int, s *intsets.Sparse) {
		if p < 0 || s.IsEmpty() {
			return
		}
		setops++
		scratch.Difference(s, &pts[p]) // exactly the elements add() would have inserted
		if scratch.IsEmpty() {
			return
		}
		props += int64(scratch.Len())
		pts[p].UnionWith(&scratch)
		delta[p].UnionWith(&scratch)
		push(p)
	}

	addEdge := func(src, dst int) {
		if src < 0 || dst < 0 || src == dst {
			return
		}
		if !edge[src].Insert(dst) {
			return // edge already present
		}
		unionInto(dst, &pts[src]) // a new edge carries the source's full set once
	}

	for i, c := range b.cs {
		switch c.f {
		case cAddr:
			add(c.p, c.q)
		case cCopy:
			addEdge(c.q, c.p)
		case cGep:
			if c.q >= 0 {
				geps[c.q] = append(geps[c.q], [2]int{c.p, c.off})
			}
		case cLoad:
			if c.q >= 0 {
				loads[c.q] = append(loads[c.q], c.p)
			}
		case cStore:
			if c.p >= 0 {
				stores[c.p] = append(stores[c.p], c.q)
			}
		case cInvoke, cICall:
			if c.p >= 0 {
				calls[c.p] = append(calls[c.p], i)
			}
		}
	}

	methodCache := map[[2]interface{}]*ssa.Function{}
	methodOf := func(t types.Type, name string) *ssa.Function {
		k := [2]interface{}{t, name}
		if f, ok := methodCache[k]; ok {
			return f
		}
		var out *ssa.Function
		for _, tt := range []types.Type{t, types.NewPointer(t)} {
			ms := prog.MethodSets.MethodSet(tt)
			for i := 0; i < ms.Len(); i++ {
				if ms.At(i).Obj().Name() == name {
					out = prog.MethodValue(ms.At(i))
					break
				}
			}
			if out != nil {
				break
			}
		}
		methodCache[k] = out
		return out
	}

	dispatch := map[[2]int]bool{}
	callees := map[string]bool{}

	// wire binds a resolved callee's params to args and results to rets.
	// recvObj >= 0 means the receiver object must also flow into param 0.
	wire := func(fn *ssa.Function, c constraint, recvObj int) {
		ps := b.fnParams[fn]
		rr := b.fnResults[fn]
		off := 0
		if recvObj >= 0 && len(ps) > 0 {
			add(ps[0], recvObj)
			off = 1
		}
		for i, a := range c.args {
			if i+off < len(ps) {
				addEdge(a, ps[i+off])
			}
		}
		for i, r := range c.rets {
			if i < len(rr) {
				addEdge(rr[i], r)
			}
		}
		callees[fn.String()] = true
	}

	start := time.Now()
	pops := int64(0)
	timedOut := false

	var d intsets.Sparse // snapshot of delta[v]
	var objs []int       // reusable element buffer
	var dsts []int       // reusable edge buffer

	for len(wl) > 0 {
		pops++
		if pops%1024 == 0 && time.Since(start) > budget {
			timedOut = true
			break
		}
		v := wl[len(wl)-1]
		wl = wl[:len(wl)-1]
		onWL[v] = false

		if delta[v].IsEmpty() {
			continue
		}
		d.Copy(&delta[v])
		delta[v].Clear()

		if len(loads[v]) > 0 || len(stores[v]) > 0 || len(geps[v]) > 0 || len(calls[v]) > 0 {
			// Complex constraints are irreducibly per-object: each object in the delta
			// names a distinct callee / field / cell. No set operation fuses them.
			objs = d.AppendTo(objs[:0])
			for _, o := range objs {
				if !doneComplex[v].Insert(o) {
					continue
				}

				for _, p := range loads[v] {
					addEdge(o, p)
				}
				for _, q := range stores[v] {
					addEdge(q, o)
				}
				for _, g := range geps[v] {
					if ob := base[o]; ob >= 0 && g[1] < b.objSize[ob] {
						add(g[0], ob+g[1])
					}
				}
				for _, ci := range calls[v] {
					c := b.cs[ci]
					ob := base[o]
					if ob < 0 {
						continue
					}
					switch c.f {
					case cInvoke:
						t := b.objType[ob]
						if t == nil {
							continue
						}
						fn := methodOf(t, c.method)
						if fn == nil {
							continue
						}
						dispatch[[2]int{ci, ob}] = true
						wire(fn, c, o) // receiver flows into param 0
					case cICall:
						fn := b.objFunc[ob]
						if fn == nil {
							continue
						}
						dispatch[[2]int{ci, ob}] = true
						wire(fn, c, -1)
					}
				}
			}
		}

		// THE HOT LOOP. The map version ran
		//     for dst := range edge[v] { for o := range d { add(dst, o) } }
		// which is one hash lookup + one hash insert per (edge, object) pair. Here each
		// edge costs ONE set union, whatever the delta's size.
		dsts = edge[v].AppendTo(dsts[:0])
		for _, dst := range dsts {
			unionInto(dst, &d)
		}
	}
	elapsed := time.Since(start)

	nonEmpty, total, nEdges := 0, 0, 0
	for i := range pts {
		if l := pts[i].Len(); l > 0 {
			nonEmpty++
			total += l
		}
	}
	for i := range edge {
		nEdges += edge[i].Len()
	}

	// CARRIER DENSITY. A sparse bit vector only beats a hash map when the elements
	// that co-occur in a set are NUMERICALLY ADJACENT: one 56-byte block covers 256
	// consecutive ids. If the ids in a points-to set are scattered, each block carries
	// a handful of set bits, the word-parallelism evaporates, and the structure
	// degenerates into a linked list with worse constants than the map.
	//
	// The suspicion: main.go draws value nodes and object nodes from ONE counter, so
	// object ids are sprayed across the whole node id space, and a points-to set (which
	// contains only object ids) is sparse in the index it is indexed by.
	var blocks, pairs int
	seen := map[int]bool{}
	buf := []int{}
	for i := range pts {
		if pts[i].IsEmpty() {
			continue
		}
		buf = pts[i].AppendTo(buf[:0])
		pairs += len(buf)
		for k := range seen {
			delete(seen, k)
		}
		for _, o := range buf {
			seen[o/256] = true
		}
		blocks += len(seen)
	}
	objSpan := 0
	for bs, size := range b.objSize {
		if bs+size > objSpan {
			objSpan = bs + size
		}
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	fmt.Printf("\n─── SOLVE (difference propagation; SPARSE BITSET carrier; no HVN, no cycle elim) ───\n")
	if timedOut {
		fmt.Printf("status            TIMED OUT after %v\n", budget)
	} else {
		fmt.Printf("status            CONVERGED\n")
	}
	fmt.Printf("wall              %v\n", elapsed.Round(time.Millisecond))
	fmt.Printf("propagations      %d\n", props)
	fmt.Printf("SET UNIONS        %d\n", setops)
	fmt.Printf("element ops       %d\n", elemops)
	fmt.Printf("worklist pops     %d\n", pops)
	fmt.Printf("copy edges        %d\n", nEdges)
	fmt.Printf("nodes w/ pts      %d\n", nonEmpty)
	fmt.Printf("total pts size    %d\n", total)
	if nonEmpty > 0 {
		fmt.Printf("MEAN |pts|        %.2f\n", float64(total)/float64(nonEmpty))
	}
	fmt.Printf("dispatch resolved %d\n", len(dispatch))
	fmt.Printf("heap              %d MB\n", ms.HeapAlloc/(1<<20))
	fmt.Printf("\n─── CARRIER DENSITY (is the bit vector actually a bit vector?) ───\n")
	fmt.Printf("id space (nodes)  %d\n", n)
	fmt.Printf("object id span    %d\n", objSpan)
	fmt.Printf("pts blocks used   %d\n", blocks)
	if blocks > 0 {
		fmt.Printf("BITS SET / BLOCK  %.2f  (out of 256)\n", float64(pairs)/float64(blocks))
		fmt.Printf("bytes / pair      %.1f  (56B per block)\n", 56*float64(blocks)/float64(pairs))
	}
	if os.Getenv("CALLEES") != "" {
		var cs []string
		for c := range callees {
			cs = append(cs, c)
		}
		sort.Strings(cs)
		for _, c := range cs {
			fmt.Printf("CALLEE %s\n", c)
		}
	}
}
