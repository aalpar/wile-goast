package main

import (
	"fmt"
	"go/types"
	"os"
	"sort"
	"time"

	"golang.org/x/tools/go/ssa"
)

// solveDiff computes lfp(F) with DIFFERENCE PROPAGATION: on pop, only the
// objects added since a node was last processed are pushed along its edges. A
// newly created edge carries the source's full set once.
//
// Cycle elimination and HVN are NOT used: they rewrite the constraint graph, and
// the design bars them so the result speaks to the algebraic bet rather than to
// solver engineering.
// solveDiff is the frozen E31 difference-propagation solver, modified only to
// RETURN pts and the god object so the HVN probe can identify the flood
// carriers. Provenance: verbatim from ../E31-csize/diff-map-v2-typefilter.go
// except the signature and the trailing return.
func solveDiff(b *builder, prog *ssa.Program, budget time.Duration) (pts []map[int]bool, godObj int, converged bool) {
	n := b.nNodes
	pts = make([]map[int]bool, n)
	delta := make([]map[int]bool, n)
	edge := make([]map[int]bool, n)

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
	calls := make([][]int, n)   // invoke + icall constraint indices, keyed on the pointer
	asserts := make([][]int, n) // tfilter + untag constraint indices, keyed on the interface pointer
	doneComplex := make([]map[int]bool, n)

	var tests, props int64
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
		tests++
		if pts[p] == nil {
			pts[p] = map[int]bool{}
			delta[p] = map[int]bool{}
		}
		if pts[p][o] {
			return
		}
		pts[p][o] = true
		delta[p][o] = true
		props++
		push(p)
	}
	addEdge := func(src, dst int) {
		if src < 0 || dst < 0 || src == dst {
			return
		}
		if edge[src] == nil {
			edge[src] = map[int]bool{}
		}
		if edge[src][dst] {
			return
		}
		edge[src][dst] = true
		for o := range pts[src] {
			add(dst, o)
		}
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
		case cTFilter, cUntag:
			if c.q >= 0 {
				asserts[c.q] = append(asserts[c.q], i) // keyed on src, the interface pointer
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

	for len(wl) > 0 {
		pops++
		if pops%1024 == 0 && time.Since(start) > budget {
			timedOut = true
			break
		}
		v := wl[len(wl)-1]
		wl = wl[:len(wl)-1]
		onWL[v] = false

		if delta[v] == nil || len(delta[v]) == 0 {
			continue
		}
		d := delta[v]
		delta[v] = map[int]bool{}

		if len(loads[v]) > 0 || len(stores[v]) > 0 || len(geps[v]) > 0 || len(calls[v]) > 0 || len(asserts[v]) > 0 {
			if doneComplex[v] == nil {
				doneComplex[v] = map[int]bool{}
			}
			for o := range d {
				if doneComplex[v][o] {
					continue
				}
				doneComplex[v][o] = true

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
				// Type assertions: the object o (a tagged interface object) passes
				// only if its concrete type satisfies the assertion. This is the
				// filter the v1 solver was missing.
				for _, ci := range asserts[v] {
					c := b.cs[ci]
					ob := base[o]
					if ob < 0 {
						continue
					}
					t := b.objType[ob]
					if t == nil {
						continue
					}
					switch c.f {
					case cTFilter: // dst = src.(Iface): keep the tagged object itself
						if types.AssignableTo(t, c.typ) {
							add(c.p, o)
						}
					case cUntag: // dst = src.(Concrete): copy the payload of a type-exact match
						if types.Identical(t, c.typ) {
							addEdge(o, c.p)
						}
					}
				}
			}
		}

		for dst := range edge[v] {
			for o := range d {
				add(dst, o)
			}
		}
	}
	elapsed := time.Since(start)

	nonEmpty, total, nEdges := 0, 0, 0
	maxPts, maxObj := 0, 0 // which OBJECT is in the most points-to sets ("god object")
	inHowMany := map[int]int{}
	var bucket [24]int // |pts| histogram by power of two
	for i, s := range pts {
		if len(s) > 0 {
			nonEmpty++
			total += len(s)
			l := len(s)
			b := 0
			for (1 << b) < l {
				b++
			}
			if b < len(bucket) {
				bucket[b]++
			}
			if l > maxPts {
				maxPts, maxObj = l, i
			}
			for o := range s {
				inHowMany[o]++
			}
		}
	}
	// which single object is pointed to by the most nodes?
	godObj = -1
	godCount := 0
	for o, c := range inHowMany {
		if c > godCount {
			godObj, godCount = o, c
		}
	}
	for _, e := range edge {
		nEdges += len(e)
	}

	fmt.Printf("\n─── SOLVE (difference propagation; no HVN, no cycle elimination) ───\n")
	if timedOut {
		fmt.Printf("status            TIMED OUT after %v\n", budget)
	} else {
		fmt.Printf("status            CONVERGED\n")
	}
	fmt.Printf("wall              %v\n", elapsed.Round(time.Millisecond))
	fmt.Printf("propagations      %d\n", props)
	fmt.Printf("MEMBERSHIP TESTS  %d\n", tests)
	fmt.Printf("worklist pops     %d\n", pops)
	fmt.Printf("copy edges        %d\n", nEdges)
	fmt.Printf("nodes w/ pts      %d\n", nonEmpty)
	fmt.Printf("total pts size    %d\n", total)
	if nonEmpty > 0 {
		fmt.Printf("MEAN |pts|        %.2f\n", float64(total)/float64(nonEmpty))
	}
	fmt.Printf("dispatch resolved %d\n", len(dispatch))
	if maxObj >= 0 && maxObj < len(b.cs)+n {
		var mt types.Type
		if ob := base[maxObj]; ob >= 0 {
			mt = b.objType[ob]
		}
		fmt.Printf("MAX |pts|         %d  (at node %d)\n", maxPts, maxObj)
		if god := base[godObj]; god >= 0 {
			fmt.Printf("GOD OBJECT        node %d, in pts of %d nodes, type %v\n", godObj, godCount, b.objType[god])
		}
		_ = mt
	}
	fmt.Printf("|pts| histogram (log2 buckets):\n")
	for bi, c := range bucket {
		if c > 0 {
			fmt.Printf("  <=2^%-2d  %8d nodes\n", bi, c)
		}
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
	return pts, godObj, !timedOut
}
