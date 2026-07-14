// Offline Hash-Value Numbering (HVN), a faithful port of go/pointer's hvn.go
// (Hardekopf & Lin, SAS'07) over E31's 8-form constraint representation.
//
// This is a DIAGNOSTIC, not a production optimization: it computes the
// pointer-equivalence (PE) labels HVN would assign, then the probe counts how
// many PE classes the god-set flood nodes collapse into. It never renumbers
// constraints or re-solves; it only labels.
//
// The reference (go/pointer/hvn.go) uses online SCC coalescing (Tarjan's visit
// with find/union). This port is a semantically-equivalent TWO-PASS: plain
// iterative Tarjan to get SCCs, then a topological sweep assigning each SCC one
// PE label from its indirect flag and the labels flowing in along REAL edges.
// The label of an SCC depends only on (a) whether any member is indirect and
// (b) its real-edge predecessors' labels; both are order-independent given
// dependency-first processing, which Tarjan's completion order provides.
package main

import (
	"sort"
	"strconv"
	"strings"
)

const nonPointer = -1 // PE sentinel: an SCC with no incoming pointer labels

type peResult struct {
	label    []int  // PE label per main node id (0..N-1); nonPointer for non-pointers
	indirect []bool // main node marked indirect (fresh label, HVN-unmergeable)
	nLabels  int    // distinct PE labels among main nodes (the HVN node count)
}

func runHVN(b *builder) peResult {
	N := b.nNodes
	M := 2 * N // offline nodes: [0,N) main graph, [N,2N) ref(X) = X+N
	ref := func(id int) int { return id + N }

	realEdge := make([][]int, M) // dependency edges dst-->src (label-gathering)
	implEdge := make([][]int, M) // implicit edges (SCC computation only)
	indirect := make([]bool, M)
	seed := make([][]int, M) // PE labels seeded by addr / offsetAddr

	// ref(X) nodes are indirect by construction: they denote the unknown
	// points-to set of a dereferenced pointer (hvn.go:271).
	for x := N; x < M; x++ {
		indirect[x] = true
	}

	// Label allocation. addr labels ARE object ids (< N); offsetAddr and SCC
	// labels are allocated >= N. Distinctness is all that matters here (we
	// count classes, we do not renumber), so the exact values are arbitrary
	// so long as the three ranges never collide.
	nextLabel := N
	newLabel := func() int { l := nextLabel; nextLabel++; return l }
	offLabel := map[[2]int]int{} // (src,offset) -> memoized offsetAddr label

	// ---- markIndirectNodes ----
	// (a) Every address-taken object span. In E31 every object is born from an
	//     addr constraint, so this marks every object's whole flat span. It
	//     subsumes go/pointer's (c) map-value and (d) array-elem cases, which
	//     are sub-nodes inside those spans.
	for _, c := range b.cs {
		if c.f == cAddr {
			size := b.objSize[c.q]
			if size <= 0 {
				size = 1
			}
			for k := 0; k < size && c.q+k < N; k++ {
				indirect[c.q+k] = true
			}
		}
	}
	// (b) P-blocks of address-taken functions. A function is address-taken if
	//     it is a method (in a method set, hence dynamically invocable) or its
	//     func-value node flows as a value (a possible icall target). Both are
	//     structural; neither needs the solve.
	valueUsed := map[int]bool{}
	for _, c := range b.cs {
		switch c.f {
		case cCopy, cStore, cLoad, cGep:
			if c.q >= 0 {
				valueUsed[c.q] = true
			}
		}
		for _, a := range c.args {
			if a >= 0 {
				valueUsed[a] = true
			}
		}
	}
	for fn, ps := range b.fnParams {
		// Address-taken (params wired dynamically, hence not captured offline)
		// unless the function is ONLY ever a static callee. A static call emits
		// explicit cCopy param<-arg edges, which the offline graph sees. Every
		// other case wires params dynamically:
		//   Recv()!=nil     a method — invocable via interface
		//   Parent()!=nil   a closure / nested func — entered via MakeClosure
		//   Synthetic!=""   a bound-method or wrapper thunk — a taken method value
		//   valueUsed       a top-level func whose value flows to an icall
		at := fn.Signature.Recv() != nil || fn.Parent() != nil || fn.Synthetic != ""
		if !at {
			if id, ok := b.nodeOf[fn]; ok && valueUsed[id] {
				at = true
			}
		}
		if at {
			for _, p := range ps {
				if p >= 0 && p < N {
					indirect[p] = true
				}
			}
		}
	}

	// ---- presolve: offline edges + seed labels, per constraint form ----
	for _, c := range b.cs {
		switch c.f {
		case cAddr: // dst = &obj : dst seeds the object's own label; *dst ~~> obj
			seed[c.p] = append(seed[c.p], c.q)
			implEdge[ref(c.p)] = append(implEdge[ref(c.p)], c.q)
		case cCopy: // dst = src : dst-->src ; *dst ~~> *src
			realEdge[c.p] = append(realEdge[c.p], c.q)
			implEdge[ref(c.p)] = append(implEdge[ref(c.p)], ref(c.q))
		case cGep: // dst = &src.off : fresh memoized label per (src,off)
			key := [2]int{c.q, c.off}
			l, ok := offLabel[key]
			if !ok {
				l = newLabel()
				offLabel[key] = l
			}
			seed[c.p] = append(seed[c.p], l)
		case cLoad: // dst = *src : dst --> ref(src)  (E31 loads are offset 0)
			realEdge[c.p] = append(realEdge[c.p], ref(c.q))
		case cStore: // *dst = src : ref(dst) --> src
			realEdge[ref(c.p)] = append(realEdge[ref(c.p)], c.q)
		case cTFilter: // dst = src.(Iface) : result not captured offline
			if c.p >= 0 && c.p < N {
				indirect[c.p] = true
			}
		case cUntag: // dst = src.(Concrete) : payload not captured offline
			if c.p >= 0 && c.p < N {
				indirect[c.p] = true
			}
		case cInvoke, cICall: // R-block (rets) receives values via dynamic wiring
			for _, r := range c.rets {
				if r >= 0 && r < N {
					indirect[r] = true
				}
			}
		}
	}

	// ---- pass 1: SCCs of the offline graph (real + implicit edges) ----
	comp, nc := tarjanSCC(M, realEdge, implEdge)

	// ---- pass 2: topological labeling ----
	// comp ids are assigned in Tarjan completion order = dependency-first, so a
	// real edge dst-->src has comp[src] labeled before comp[dst] is.
	compIndirect := make([]bool, nc)
	compSeed := make([][]int, nc)
	for id := 0; id < M; id++ {
		c := comp[id]
		if indirect[id] {
			compIndirect[c] = true
		}
		compSeed[c] = append(compSeed[c], seed[id]...)
	}
	// members per component, to gather real-edge predecessors
	members := make([][]int, nc)
	for id := 0; id < M; id++ {
		members[comp[id]] = append(members[comp[id]], id)
	}

	compLabel := make([]int, nc)
	hvnLabel := map[string]int{} // canonical label per input label-set
	set := map[int]bool{}
	for c := 0; c < nc; c++ {
		if compIndirect[c] {
			compLabel[c] = newLabel()
			continue
		}
		for k := range set {
			delete(set, k)
		}
		for _, s := range compSeed[c] {
			set[s] = true
		}
		for _, m := range members[c] {
			for _, dep := range realEdge[m] {
				dc := comp[dep]
				if dc == c {
					continue // intra-SCC edge
				}
				dl := compLabel[dc]
				if dl == nonPointer {
					continue // predecessor is a non-pointer
				}
				set[dl] = true
			}
		}
		switch len(set) {
		case 0:
			compLabel[c] = nonPointer
		case 1:
			for k := range set {
				compLabel[c] = k
			}
		default:
			ks := make([]int, 0, len(set))
			for k := range set {
				ks = append(ks, k)
			}
			sort.Ints(ks)
			var sb strings.Builder
			for i, k := range ks {
				if i > 0 {
					sb.WriteByte(',')
				}
				sb.WriteString(strconv.Itoa(k))
			}
			key := sb.String()
			l, ok := hvnLabel[key]
			if !ok {
				l = newLabel()
				hvnLabel[key] = l
			}
			compLabel[c] = l
		}
	}

	// project onto the N main nodes
	res := peResult{
		label:    make([]int, N),
		indirect: make([]bool, N),
	}
	distinct := map[int]bool{}
	for id := 0; id < N; id++ {
		res.label[id] = compLabel[comp[id]]
		res.indirect[id] = indirect[id]
		distinct[res.label[id]] = true
	}
	res.nLabels = len(distinct)
	return res
}

// tarjanSCC returns a component id per node (in completion order: a node's
// dependencies get lower comp ids) and the component count. Iterative, to
// survive ~1.5M offline nodes.
func tarjanSCC(n int, adj1, adj2 [][]int) ([]int, int) {
	const unvisited = -1
	index := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	comp := make([]int, n)
	for i := range index {
		index[i] = unvisited
		comp[i] = unvisited
	}
	var stack []int
	idx, nc := 0, 0

	type frame struct {
		v, e int // node, edge cursor (0..len(adj1) spans adj1 then adj2)
	}
	for root := 0; root < n; root++ {
		if index[root] != unvisited {
			continue
		}
		st := []frame{{root, 0}}
		index[root] = idx
		low[root] = idx
		idx++
		stack = append(stack, root)
		onStack[root] = true
		for len(st) > 0 {
			f := &st[len(st)-1]
			v := f.v
			// neighbours = adj1[v] then adj2[v], addressed by a single cursor
			d1 := adj1[v]
			var w = -1
			if f.e < len(d1) {
				w = d1[f.e]
			} else if f.e-len(d1) < len(adj2[v]) {
				w = adj2[v][f.e-len(d1)]
			}
			if w >= 0 {
				f.e++
				if index[w] == unvisited {
					index[w] = idx
					low[w] = idx
					idx++
					stack = append(stack, w)
					onStack[w] = true
					st = append(st, frame{w, 0})
				} else if onStack[w] && index[w] < low[v] {
					low[v] = index[w]
				}
				continue
			}
			// done with v
			if low[v] == index[v] {
				for {
					x := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					onStack[x] = false
					comp[x] = nc
					if x == v {
						break
					}
				}
				nc++
			}
			st = st[:len(st)-1]
			if len(st) > 0 {
				p := st[len(st)-1].v
				if low[v] < low[p] {
					low[p] = low[v]
				}
			}
		}
	}
	return comp, nc
}
