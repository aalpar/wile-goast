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

// solveDiff computes lfp(F) with DIFFERENCE PROPAGATION over a SPARSE BITSET carrier,
// like csize-bitset, but with one further REPRESENTATION change and nothing else:
//
//	POINTS-TO SETS ARE INDEXED BY A DENSE OBJECT-ELEMENT ID, NOT BY NODE ID.
//
// Why. main.go draws value nodes and object nodes from ONE counter, so object ids are
// sprayed across the entire node id space (measured: object id span 850,293 out of
// 850,295 nodes). A points-to set contains ONLY object ids, so as a bit vector indexed
// by node id it is ~90% structurally empty, and csize-bitset measured the consequence:
// 4.84 bits set per 256-bit block, 0.76 IPC, memory-stalled.
//
// A bit vector only beats a hash map when co-occurring elements are NUMERICALLY
// ADJACENT. So compact the element space: every object sub-node (an object base, or a
// field within one) gets a contiguous index in [0, M). This is a BIJECTION on the
// element set. Same lattice, same transfer functions, same lfp, same schedule. Still no
// HVN and no cycle elimination. Only the encoding of a set element moves.
func solveDiff(b *builder, prog *ssa.Program, budget time.Duration) {
	n := b.nNodes

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

	// THE COMPACTION. denseOf maps an object sub-node id -> its dense element index;
	// sparseOf inverts it. Non-object nodes map to -1: they can never appear in a
	// points-to set, and under the old numbering they were the ~90% of dead index space
	// that hollowed out every block.
	denseOf := make([]int32, n)
	for i := range denseOf {
		denseOf[i] = -1
	}
	sparseOf := make([]int32, 0, n)
	for i := 0; i < n; i++ {
		if base[i] >= 0 {
			denseOf[i] = int32(len(sparseOf))
			sparseOf = append(sparseOf, int32(i))
		}
	}
	M := len(sparseOf) // the element space, vs n for the node space

	pts := make([]intsets.Sparse, n)         // indexed by node; ELEMENTS are dense ids
	delta := make([]intsets.Sparse, n)       // ditto
	edge := make([]intsets.Sparse, n)        // elements are NODE ids (a different space)
	doneComplex := make([]intsets.Sparse, n) // elements are dense ids

	geps := make([][][2]int, n)
	loads := make([][]int, n)
	stores := make([][]int, n)
	calls := make([][]int, n)

	var props, setops, elemops int64

	onWL := make([]bool, n)
	wl := []int{}

	push := func(p int) {
		if !onWL[p] {
			onWL[p] = true
			wl = append(wl, p)
		}
	}

	// add still takes a NODE id for the object, exactly as before, and compacts on the
	// way in. Every call site is unchanged.
	add := func(p, o int) {
		if p < 0 || o < 0 {
			return
		}
		di := denseOf[o]
		if di < 0 {
			return // not an object sub-node; cannot be a points-to element
		}
		elemops++
		if !pts[p].Insert(int(di)) {
			return
		}
		delta[p].Insert(int(di))
		props++
		push(p)
	}

	var scratch intsets.Sparse
	unionInto := func(p int, s *intsets.Sparse) {
		if p < 0 || s.IsEmpty() {
			return
		}
		setops++
		scratch.Difference(s, &pts[p])
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
			return
		}
		unionInto(dst, &pts[src])
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

	var d intsets.Sparse
	var dis []int // dense element ids drawn from the delta
	var dsts []int

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
			dis = d.AppendTo(dis[:0])
			for _, di := range dis {
				if !doneComplex[v].Insert(di) {
					continue
				}
				o := int(sparseOf[di]) // back to a NODE id for the complex constraints

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
						wire(fn, c, o)
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

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	fmt.Printf("\n─── SOLVE (difference propagation; SPARSE BITSET over a DENSE OBJECT ID SPACE) ───\n")
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

	fmt.Printf("\n─── CARRIER DENSITY ───\n")
	fmt.Printf("node id space     %d\n", n)
	fmt.Printf("ELEMENT SPACE (M) %d   (compaction %.2fx)\n", M, float64(n)/float64(M))
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
