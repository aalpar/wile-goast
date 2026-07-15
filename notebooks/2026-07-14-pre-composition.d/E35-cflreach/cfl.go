// Demand-driven, field-sensitive, context-INsensitive points-to.
//
// Context-insensitive field-sensitive CFL-reachability degenerates to the
// Andersen relation (the only Dyck language is field-balance, realized by
// routing load/store through shared object nodes; the call/return Dyck language
// would be v2). So this computes the SAME points-to relation as the E31 batch
// solver (solve.go), but seeded from a query set and propagated only among
// ACTIVATED nodes. Query all nodes -> batch Andersen exactly (the equivalence
// theorem, which main.go checks). Query only dispatch sites -> a subset, and the
// count of activated nodes is the demand footprint the hypothesis is about.
//
// The propagation rules are line-for-line the batch solver's; the only new
// mechanism is activation:
//   * activating a node installs its BACKWARD feeders (the constraints whose
//     destination it is), so its pts can be computed;
//   * a load `v = *q` must, for each object o in pts(q), also gather o's
//     CONTENTS -- so it activates o and FORWARD-activates from &o to reach the
//     store-pointers that write o (the demand form of the heap match).
package main

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

type demand struct {
	b    *builder
	prog *ssa.Program
	n    int

	pts  []map[int]bool
	base []int

	// backward indices: constraints keyed by DESTINATION node
	addrOf   [][]int          // dst -> objects (cAddr)
	copyBack [][]int          // dst -> srcs (cCopy)
	gepBack  [][][2]int       // dst -> (src, off) (cGep)
	loadBack [][]int          // dst -> ptr (cLoad: dst = *ptr)
	asrtBack [][]int          // dst -> constraint idx (cTFilter/cUntag, keyed by dst)
	callRet  [][]int          // dst -> constraint idx (cInvoke/cICall where dst in rets)
	objAddr  map[int][]int    // object -> nodes a with cAddr{a, object}  (for &o)

	// forward indices (mirror batch): keyed by the POINTER node
	copyFwd [][]int    // src -> dsts (cCopy, STATIC), for forward-activation from &o
	dynFwd  [][]int    // src -> dsts added DURING a query (load/store/wire/untag); reset() clears these per query
	gepFwd  [][][2]int // src -> (dst, off)
	loads   [][]int    // ptr -> dst nodes that load *ptr
	stores  [][]int    // ptr -> stored value nodes (*ptr = val)
	calls   [][]int    // ptr(recv/fv) -> constraint idx (cInvoke/cICall)
	asserts [][]int    // ptr(iface src) -> constraint idx (cTFilter/cUntag)

	// dynamic-caller discovery: a param is fed by wire() only when a call that
	// targets its function fires. Backward from a param we must activate those
	// calls' receivers. Callers are dynamic, so we over-approximate soundly:
	// invoke callers by method name, icall callers for any address-taken func.
	paramOwner      map[int]*ssa.Function
	fnMethod        map[*ssa.Function]string // method name, if fn is a method
	fnAddrTaken     map[*ssa.Function]bool   // fn can be an icall target
	invokesByMethod map[string][]int         // method name -> cInvoke ptr nodes
	icallRecvs      []int                    // all cICall ptr nodes

	active     []bool
	bwDone     []bool // backward feeders installed (vs merely relay-active)
	fwdVisited []bool // forward-activation cone of loaded objects
	wl         []int
	onWL       []bool
	touched    []int // nodes activated this query, for cheap reset() between queries

	methodCache map[[2]interface{}]*ssa.Function

	// instrumentation
	nActivated int
}

func newDemand(b *builder, prog *ssa.Program) *demand {
	n := b.nNodes
	d := &demand{
		b: b, prog: prog, n: n,
		pts:         make([]map[int]bool, n),
		base:        make([]int, n),
		addrOf:      make([][]int, n),
		copyBack:    make([][]int, n),
		gepBack:     make([][][2]int, n),
		loadBack:    make([][]int, n),
		asrtBack:    make([][]int, n),
		callRet:     make([][]int, n),
		objAddr:     map[int][]int{},
		copyFwd:     make([][]int, n),
		gepFwd:      make([][][2]int, n),
		dynFwd:      make([][]int, n),
		loads:       make([][]int, n),
		stores:      make([][]int, n),
		calls:       make([][]int, n),
		asserts:     make([][]int, n),
		active:          make([]bool, n),
		bwDone:          make([]bool, n),
		fwdVisited:      make([]bool, n),
		onWL:            make([]bool, n),
		methodCache:     map[[2]interface{}]*ssa.Function{},
		paramOwner:      map[int]*ssa.Function{},
		fnMethod:        map[*ssa.Function]string{},
		fnAddrTaken:     map[*ssa.Function]bool{},
		invokesByMethod: map[string][]int{},
	}
	for i := range d.base {
		d.base[i] = -1
	}
	for bs, size := range b.objSize {
		for k := 0; k < size; k++ {
			if bs+k < n {
				d.base[bs+k] = bs
			}
		}
	}
	for i, c := range b.cs {
		switch c.f {
		case cAddr:
			if c.p >= 0 && c.q >= 0 {
				d.addrOf[c.p] = append(d.addrOf[c.p], c.q)
				d.objAddr[c.q] = append(d.objAddr[c.q], c.p)
			}
		case cCopy:
			if c.p >= 0 && c.q >= 0 {
				d.copyBack[c.p] = append(d.copyBack[c.p], c.q)
				d.copyFwd[c.q] = append(d.copyFwd[c.q], c.p)
			}
		case cGep:
			if c.p >= 0 && c.q >= 0 {
				d.gepBack[c.p] = append(d.gepBack[c.p], [2]int{c.q, c.off})
				d.gepFwd[c.q] = append(d.gepFwd[c.q], [2]int{c.p, c.off})
			}
		case cLoad:
			if c.p >= 0 && c.q >= 0 {
				d.loadBack[c.p] = append(d.loadBack[c.p], c.q)
				d.loads[c.q] = append(d.loads[c.q], c.p)
			}
		case cStore:
			if c.p >= 0 && c.q >= 0 {
				d.stores[c.p] = append(d.stores[c.p], c.q)
			}
		case cInvoke, cICall:
			if c.p >= 0 {
				d.calls[c.p] = append(d.calls[c.p], i)
			}
			for _, r := range c.rets {
				if r >= 0 {
					d.callRet[r] = append(d.callRet[r], i)
				}
			}
		case cTFilter, cUntag:
			if c.q >= 0 {
				d.asserts[c.q] = append(d.asserts[c.q], i)
			}
			if c.p >= 0 {
				d.asrtBack[c.p] = append(d.asrtBack[c.p], i)
			}
		}
		if c.f == cInvoke && c.p >= 0 {
			d.invokesByMethod[c.method] = append(d.invokesByMethod[c.method], c.p)
		}
		if c.f == cICall && c.p >= 0 {
			d.icallRecvs = append(d.icallRecvs, c.p)
		}
	}

	// caller-discovery: params -> owning function; per function, its method name
	// (invoke targets) and whether it is address-taken (icall targets). Mirrors
	// hvn.go's address-taken test: a func is icall-reachable if it is a closure,
	// a synthetic thunk, or its value-node flows.
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
		for _, p := range ps {
			if p >= 0 {
				d.paramOwner[p] = fn
			}
		}
		if recv := fn.Signature.Recv(); recv != nil {
			d.fnMethod[fn] = "" // filled from the SSA method name below
		}
		at := fn.Parent() != nil || fn.Synthetic != ""
		if id, ok := b.nodeOf[fn]; ok && valueUsed[id] {
			at = true
		}
		d.fnAddrTaken[fn] = at
	}
	// a method's dispatched name is its object's name; recover it from the func name.
	for fn := range d.fnMethod {
		d.fnMethod[fn] = fn.Name()
	}
	return d
}

func (d *demand) push(p int) {
	if !d.onWL[p] {
		d.onWL[p] = true
		d.wl = append(d.wl, p)
	}
}

func (d *demand) add(p, o int) {
	if p < 0 || o < 0 || !d.active[p] {
		return
	}
	if d.pts[p] == nil {
		d.pts[p] = map[int]bool{}
	}
	if d.pts[p][o] {
		return
	}
	d.pts[p][o] = true
	d.push(p)
}

// markActive makes a node able to hold and relay pts (so add() works and run()
// propagates through it), WITHOUT installing its backward feeders. Used for
// nodes on the forward path to a writer: they only relay a loaded object onward,
// so pulling their full backward cone would over-activate massively.
func (d *demand) markActive(v int) {
	if v < 0 || d.active[v] {
		return
	}
	d.active[v] = true
	d.nActivated++
	d.touched = append(d.touched, v)
}

// activate makes v relay-active AND installs its backward feeders so its FULL
// pts is computed. Idempotent in the backward install via bwDone, so a node
// first seen forward can be upgraded to full backward later.
func (d *demand) activate(v int) {
	if v < 0 {
		return
	}
	d.markActive(v)
	if d.bwDone[v] {
		return
	}
	d.bwDone[v] = true

	for _, o := range d.addrOf[v] {
		d.add(v, o)
	}
	for _, q := range d.copyBack[v] {
		d.activate(q)
		d.replay(q, v, func(dst, o int) { d.add(dst, o) }) // pts(v) ⊇ pts(q)
	}
	for _, g := range d.gepBack[v] {
		d.activate(g[0])
		d.replayGep(g[0], v, g[1])
	}
	for _, q := range d.loadBack[v] {
		d.activate(q)
		d.replayLoad(q, v)
	}
	for _, ci := range d.asrtBack[v] {
		c := d.b.cs[ci]
		d.activate(c.q)
		d.replayAssert(ci)
	}
	for _, ci := range d.callRet[v] {
		c := d.b.cs[ci]
		d.activate(c.p) // the recv / func-value; resolving it wires rets forward
		d.replayCall(ci)
	}
	// if v is a param, its values arrive via wire() when a call targeting its
	// function fires. Backward from v we must activate those calls' receivers.
	if fn, ok := d.paramOwner[v]; ok {
		if m, isMethod := d.fnMethod[fn]; isMethod {
			for _, recv := range d.invokesByMethod[m] {
				d.activate(recv)
			}
		}
		if d.fnAddrTaken[fn] {
			for _, recv := range d.icallRecvs {
				d.activate(recv)
			}
		}
	}
	d.push(v)
}

// replay re-fires a just-installed edge over the source's EXISTING pts, since the
// source may already have been populated before this edge existed.
func (d *demand) replay(src, dst int, f func(dst, o int)) {
	for o := range d.pts[src] {
		f(dst, o)
	}
}
func (d *demand) replayGep(src, dst, off int) {
	for o := range d.pts[src] {
		if ob := d.base[o]; ob >= 0 && off < d.b.objSize[ob] {
			d.add(dst, ob+off)
		}
	}
}
func (d *demand) replayLoad(ptr, dst int) {
	for o := range d.pts[ptr] {
		d.loadObject(o, dst)
	}
}
func (d *demand) replayAssert(ci int) {
	c := d.b.cs[ci]
	for o := range d.pts[c.q] {
		d.fireAssert(c, o)
	}
}
func (d *demand) replayCall(ci int) {
	c := d.b.cs[ci]
	for o := range d.pts[c.p] {
		d.fireCall(c, o)
	}
}

// loadObject handles `dst = *ptr` for one object o in pts(ptr): dst gets o's
// contents. Contents come from stores into o, so we activate o and
// FORWARD-activate from &o to reach the store-pointers that write it.
func (d *demand) loadObject(o, dst int) {
	d.forwardActivateObject(o)
	// dst ⊇ pts(o): install edge o->dst and replay
	d.dynFwd[o] = append(d.dynFwd[o], dst) // dynamic copy edge, like batch's addEdge(o,dst)
	d.activate(o)
	d.replay(o, dst, func(dst, oo int) { d.add(dst, oo) })
}

// forwardActivateObject activates the forward flow of an object's BASE: &base
// and every pointer it reaches through copy/gep, so that the store-pointers
// holding the base (or its fields, via gep) fire their stores and fill the
// object's contents. This is the demand form of the heap match. Keying off the
// base (not o) is essential: a sub-node box+off has no &(box+off); its writers
// are reached from &box via the gep that produced &box.off.
func (d *demand) forwardActivateObject(o int) {
	base := d.base[o]
	if base < 0 {
		base = o
	}
	stack := append([]int(nil), d.objAddr[base]...)
	for len(stack) > 0 {
		nd := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if d.fwdVisited[nd] {
			continue
		}
		d.fwdVisited[nd] = true
		d.activate(nd) // holder must be active for run() to propagate the object into it
		for _, w := range d.copyFwd[nd] {
			if !d.fwdVisited[w] {
				stack = append(stack, w)
			}
		}
		for _, w := range d.dynFwd[nd] {
			if !d.fwdVisited[w] {
				stack = append(stack, w)
			}
		}
		for _, g := range d.gepFwd[nd] {
			if !d.fwdVisited[g[0]] {
				stack = append(stack, g[0])
			}
		}
	}
}

func (d *demand) fireAssert(c constraint, o int) {
	ob := d.base[o]
	if ob < 0 {
		return
	}
	t := d.b.objType[ob]
	if t == nil {
		return
	}
	switch c.f {
	case cTFilter:
		if types.AssignableTo(t, c.typ) {
			d.add(c.p, o)
		}
	case cUntag:
		if types.Identical(t, c.typ) {
			d.dynFwd[o] = append(d.dynFwd[o], c.p)
			d.forwardActivateObject(o)
			d.activate(o)
			d.replay(o, c.p, func(dst, oo int) { d.add(dst, oo) })
		}
	}
}

func (d *demand) methodOf(t types.Type, name string) *ssa.Function {
	k := [2]interface{}{t, name}
	if f, ok := d.methodCache[k]; ok {
		return f
	}
	var out *ssa.Function
	for _, tt := range []types.Type{t, types.NewPointer(t)} {
		ms := d.prog.MethodSets.MethodSet(tt)
		for i := 0; i < ms.Len(); i++ {
			if ms.At(i).Obj().Name() == name {
				out = d.prog.MethodValue(ms.At(i))
				break
			}
		}
		if out != nil {
			break
		}
	}
	d.methodCache[k] = out
	return out
}

func (d *demand) fireCall(c constraint, o int) {
	ob := d.base[o]
	if ob < 0 {
		return
	}
	switch c.f {
	case cInvoke:
		t := d.b.objType[ob]
		if t == nil {
			return
		}
		fn := d.methodOf(t, c.method)
		if fn != nil {
			d.wire(fn, c, o)
		}
	case cICall:
		if fn := d.b.objFunc[ob]; fn != nil {
			d.wire(fn, c, -1)
		}
	}
}

// wire binds a resolved callee's params to args and results to rets, mirroring
// solve.go's wire. All wired nodes are activated (they are now relevant).
func (d *demand) wire(fn *ssa.Function, c constraint, recvObj int) {
	ps := d.b.fnParams[fn]
	rr := d.b.fnResults[fn]
	off := 0
	if recvObj >= 0 && len(ps) > 0 {
		d.activate(ps[0])
		d.add(ps[0], recvObj)
		off = 1
	}
	for i, a := range c.args {
		if i+off < len(ps) {
			p := ps[i+off]
			d.activate(p)
			d.activate(a)
			d.dynFwd[a] = append(d.dynFwd[a], p)
			d.replay(a, p, func(dst, o int) { d.add(dst, o) })
		}
	}
	for i, r := range c.rets {
		if i < len(rr) && r >= 0 {
			d.activate(rr[i])
			d.dynFwd[rr[i]] = append(d.dynFwd[rr[i]], r)
			d.replay(rr[i], r, func(dst, o int) { d.add(dst, o) })
		}
	}
}

// run drives the demand fixpoint over the activated frontier.
func (d *demand) run() {
	for len(d.wl) > 0 {
		v := d.wl[len(d.wl)-1]
		d.wl = d.wl[:len(d.wl)-1]
		d.onWL[v] = false

		for o := range d.pts[v] {
			// forward copy edges (static + dynamic load/untag/wire edges)
			for _, w := range d.copyFwd[v] {
				d.add(w, o)
			}
			for _, w := range d.dynFwd[v] {
				d.add(w, o)
			}
			for _, g := range d.gepFwd[v] {
				if ob := d.base[o]; ob >= 0 && g[1] < d.b.objSize[ob] {
					d.add(g[0], ob+g[1])
				}
			}
			for _, dst := range d.loads[v] {
				if d.active[dst] {
					d.loadObject(o, dst)
				}
			}
			for _, val := range d.stores[v] {
				// *v = val, and o ∈ pts(v): pts(o) ⊇ pts(val)
				d.activate(val)
				d.dynFwd[val] = append(d.dynFwd[val], o)
				d.activate(o)
				d.replay(val, o, func(dst, oo int) { d.add(dst, oo) })
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
}

// query computes pts for the given seed nodes (and everything they transitively
// require) and returns the number of nodes activated.
func (d *demand) query(seeds []int) {
	for _, s := range seeds {
		d.activate(s)
	}
	d.run()
}

// reset clears per-query state (pts, activation, worklist) but keeps the indices,
// so a fresh query can run cheaply. Only touched nodes are cleared, so the cost is
// proportional to the previous query's footprint, not to N.
func (d *demand) reset() {
	for _, i := range d.touched {
		d.pts[i] = nil
		d.active[i] = false
		d.bwDone[i] = false
		d.fwdVisited[i] = false
		d.onWL[i] = false
		d.dynFwd[i] = nil
	}
	d.touched = d.touched[:0]
	d.wl = d.wl[:0]
	d.nActivated = 0
}
