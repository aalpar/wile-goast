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
//   - activating a node installs its BACKWARD feeders (the constraints whose
//     destination it is), so its pts can be computed;
//   - a load `v = *q` must, for each object o in pts(q), also gather o's
//     CONTENTS -- so it activates o and FORWARD-activates from &o to reach the
//     store-pointers that write o (the demand form of the heap match).
package main

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

type rsm struct {
	b    *builder
	prog *ssa.Program
	n    int

	pts  []map[int]bool
	base []int

	// SEMI-NAIVE tabling (mirrors solve.go:27,47 exactly)
	delta       []map[int]bool // node -> unpropagated additions
	doneComplex []map[int]bool // node -> objects whose complex edges fired

	// termination tripwire (§7/§9)
	pushes   int64 // total worklist pushes this query
	nDemands int64 // activate() calls this query (the "#demands" term)

	// backward indices: constraints keyed by DESTINATION node
	addrOf   [][]int       // dst -> objects (cAddr)
	copyBack [][]int       // dst -> srcs (cCopy)
	gepBack  [][][2]int    // dst -> (src, off) (cGep)
	loadBack [][]int       // dst -> ptr (cLoad: dst = *ptr)
	asrtBack [][]int       // dst -> constraint idx (cTFilter/cUntag, keyed by dst)
	callRet  [][]int       // dst -> constraint idx (cInvoke/cICall where dst in rets)
	objAddr  map[int][]int // object -> nodes a with cAddr{a, object}  (for &o)

	// forward indices (mirror batch): keyed by the POINTER node
	copyFwd [][]int    // src -> dsts (cCopy, STATIC), for forward-activation from &o
	dynFwd  [][]int    // src -> dsts added DURING a query (load/store/wire/untag); reset() clears these per query
	gepFwd  [][][2]int // src -> (dst, off)
	loads   [][]int    // ptr -> dst nodes that load *ptr
	stores  [][]int    // ptr -> stored value nodes (*ptr = val)
	calls   [][]int    // ptr(recv/fv) -> constraint idx (cInvoke/cICall)
	asserts [][]int    // ptr(iface src) -> constraint idx (cTFilter/cUntag)

	// REACTIVATION reverse indices (§ gated-rule reentrancy). A gated arm is
	// skipped when its monotonic gate is closed; the fire-once table then blocks
	// the retry. These map a gate variable back to the loop nodes whose gated arm
	// checks it, so a false->true transition can re-fire the skipped arm.
	loadPtrsInto  [][]int       // load-dst -> pointer nodes whose load-out arm targets it (active[dst] gate; STATIC)
	gepPtrsInto   [][]int       // gep-result -> base-pointer nodes whose gep arm result is it (active[g0] gate; STATIC)
	holdersOfBase map[int][]int // base -> loop nodes currently holding an object of that base AND owning a gated arm (fwdDone[b] gate; DYNAMIC, reset per query)

	// dynamic-caller discovery: a param is fed by wire() only when a call that
	// targets its function fires. Backward from a param we must activate those
	// calls' receivers. Callers are dynamic, so we over-approximate soundly:
	// invoke callers by method name, icall callers for any address-taken func.
	paramOwner      map[int]*ssa.Function
	fnMethod        map[*ssa.Function]string // method name, if fn is a method
	fnAddrTaken     map[*ssa.Function]bool   // fn can be an icall target
	invokesByMethod map[string][]int         // method name -> cInvoke ptr nodes
	icallRecvs      []int                    // all cICall ptr nodes

	active  []bool // node may hold+relay pts (emit gate)
	ptDone  []bool // backward feeders installed (was bwDone)
	fwdDone []bool // per-BASE: contents demanded / object forward-tracked (was fwdVisited)

	storesWhereValue [][]int // val node -> store-pointer nodes {p : cStore(p,val)}

	// subscribed makes subscribe() idempotent: a re-fired gated arm (via repush)
	// re-calls loadPop -> subscribe, which would otherwise append a duplicate
	// dynamic edge. src -> set of dsts already subscribed. Reset per touched.
	subscribed []map[int]bool

	fwdBases []int // BASES set fwdDone this query (may not be in touched); for reset

	wl      []int
	onWL    []bool
	touched []int // nodes activated this query, for cheap reset() between queries

	methodCache map[[2]interface{}]*ssa.Function

	// instrumentation
	nActivated int
}

func newRSM(b *builder, prog *ssa.Program) *rsm {
	n := b.nNodes
	d := &rsm{
		b: b, prog: prog, n: n,
		pts:              make([]map[int]bool, n),
		base:             make([]int, n),
		delta:            make([]map[int]bool, n),
		doneComplex:      make([]map[int]bool, n),
		addrOf:           make([][]int, n),
		copyBack:         make([][]int, n),
		gepBack:          make([][][2]int, n),
		loadBack:         make([][]int, n),
		asrtBack:         make([][]int, n),
		callRet:          make([][]int, n),
		objAddr:          map[int][]int{},
		copyFwd:          make([][]int, n),
		gepFwd:           make([][][2]int, n),
		dynFwd:           make([][]int, n),
		loads:            make([][]int, n),
		stores:           make([][]int, n),
		calls:            make([][]int, n),
		asserts:          make([][]int, n),
		loadPtrsInto:     make([][]int, n),
		gepPtrsInto:      make([][]int, n),
		holdersOfBase:    map[int][]int{},
		subscribed:       make([]map[int]bool, n),
		active:           make([]bool, n),
		ptDone:           make([]bool, n),
		fwdDone:          make([]bool, n),
		storesWhereValue: make([][]int, n),
		onWL:             make([]bool, n),
		methodCache:      map[[2]interface{}]*ssa.Function{},
		paramOwner:       map[int]*ssa.Function{},
		fnMethod:         map[*ssa.Function]string{},
		fnAddrTaken:      map[*ssa.Function]bool{},
		invokesByMethod:  map[string][]int{},
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
				d.gepPtrsInto[c.p] = append(d.gepPtrsInto[c.p], c.q) // result -> base-pointer nodes
			}
		case cLoad:
			if c.p >= 0 && c.q >= 0 {
				d.loadBack[c.p] = append(d.loadBack[c.p], c.q)
				d.loads[c.q] = append(d.loads[c.q], c.p)
				d.loadPtrsInto[c.p] = append(d.loadPtrsInto[c.p], c.q) // load-dst -> pointer nodes
			}
		case cStore:
			if c.p >= 0 && c.q >= 0 {
				d.stores[c.p] = append(d.stores[c.p], c.q)
				d.storesWhereValue[c.q] = append(d.storesWhereValue[c.q], c.p)
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

func (d *rsm) push(p int) {
	if !d.onWL[p] {
		d.onWL[p] = true
		d.wl = append(d.wl, p)
		d.pushes++
	}
}

// repush re-injects v's current objects into its delta and re-enqueues it, so
// run() re-evaluates v's GATED arms (load-out / gep / field-push) with the gate
// now open. The ungated arms stay fire-once (doneComplex), so re-firing is cheap
// and cannot bloat dynFwd via wire(). The gated arms are idempotent (markActive /
// emit dedupe, subscribe is guarded), so re-evaluation while a gate is still
// closed is a no-op. Called from the two gate-open triggers; both gates are
// monotonic, so a node is repushed a bounded number of times (termination).
func (d *rsm) repush(v int) {
	if len(d.pts[v]) == 0 {
		return
	}
	if d.delta[v] == nil {
		d.delta[v] = map[int]bool{}
	}
	for o := range d.pts[v] {
		d.delta[v][o] = true
	}
	d.push(v)
}

func (d *rsm) emit(p, o int) { // == solve.go add() :59, demand-gated
	if p < 0 || o < 0 || !d.active[p] { // THE DEMAND GATE (only diff from batch)
		return
	}
	if d.pts[p] == nil {
		d.pts[p] = map[int]bool{}
		d.delta[p] = map[int]bool{}
	}
	if d.pts[p][o] {
		return
	}
	d.pts[p][o] = true
	d.delta[p][o] = true
	// If p owns a gated arm, record it as a holder of o's base so a later
	// fwdDone[base] transition can re-fire the arm (trigger 2). Dups are fine
	// (repush is idempotent). Register only for gated-arm owners to keep the index small.
	if len(d.loads[p]) > 0 || len(d.gepFwd[p]) > 0 || len(d.storesWhereValue[p]) > 0 {
		b := d.baseOf(o)
		d.holdersOfBase[b] = append(d.holdersOfBase[b], p)
	}
	d.push(p)
}

// markActive makes a node able to hold and relay pts (so add() works and run()
// propagates through it), WITHOUT installing its backward feeders. Used for
// nodes on the forward path to a writer: they only relay a loaded object onward,
// so pulling their full backward cone would over-activate massively.
func (d *rsm) markActive(v int) {
	if v < 0 || d.active[v] {
		return
	}
	d.active[v] = true
	d.nActivated++
	d.touched = append(d.touched, v)
	// TRIGGER 1 (target activation): v just became active. Any load-out arm whose
	// dst is v, or gep arm whose result is v, was skipped for objects already
	// present because active[v] was false. Re-fire them now.
	for _, p := range d.loadPtrsInto[v] {
		d.repush(p)
	}
	for _, p := range d.gepPtrsInto[v] {
		d.repush(p)
	}
}

// activate makes v relay-active AND installs its backward feeders so its FULL
// pts is computed. Idempotent in the backward install via ptDone, so a node
// first seen forward can be upgraded to full backward later.
func (d *rsm) activate(v int) {
	if v < 0 {
		return
	}
	d.markActive(v)
	if d.ptDone[v] {
		return
	}
	d.ptDone[v] = true
	d.nDemands++

	for _, o := range d.addrOf[v] {
		d.emit(v, o)
	}
	for _, q := range d.copyBack[v] {
		d.activate(q)
		d.replay(q, v, func(dst, o int) { d.emit(dst, o) }) // pts(v) ⊇ pts(q)
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
func (d *rsm) replay(src, dst int, f func(dst, o int)) {
	for o := range d.pts[src] {
		f(dst, o)
	}
}
func (d *rsm) replayGep(src, dst, off int) {
	for o := range d.pts[src] {
		if ob := d.base[o]; ob >= 0 && off < d.b.objSize[ob] {
			d.emit(dst, ob+off)
		}
	}
}
func (d *rsm) replayLoad(ptr, dst int) {
	for o := range d.pts[ptr] {
		d.loadPop(o, dst)
	}
}
func (d *rsm) replayAssert(ci int) {
	c := d.b.cs[ci]
	for o := range d.pts[c.q] {
		d.fireAssert(c, o)
	}
}
func (d *rsm) replayCall(ci int) {
	c := d.b.cs[ci]
	for o := range d.pts[c.p] {
		d.fireCall(c, o)
	}
}

func (d *rsm) baseOf(o int) int {
	if d.base[o] >= 0 {
		return d.base[o]
	}
	return o
}

// subscribe installs a dynamic subset edge src ⊇→ dst and replays src's set.
// == solve.go addEdge :76.
func (d *rsm) subscribe(src, dst int) {
	if d.subscribed[src] == nil {
		d.subscribed[src] = map[int]bool{}
	}
	if d.subscribed[src][dst] { // idempotent: repush re-fires the arm that calls this
		return
	}
	d.subscribed[src][dst] = true
	d.dynFwd[src] = append(d.dynFwd[src], dst)
	for o := range d.pts[src] {
		d.relay(src, dst, o)
	}
}

// relay forwards one object along a subset edge. THE FOOTPRINT FIX (§3): a
// forward-cone node gets markActive (relay-only), never full activate, so the
// cone is not multiplied by each node's backward cone. Only a node holding a
// TRACKED object escalates.
func (d *rsm) relay(src, dst, o int) {
	if d.fwdDone[d.baseOf(o)] {
		d.markActive(dst)
	}
	d.emit(dst, o)
}

// loadPop: dst = *ptr, o ∈ pts(ptr): dst ⊇ contents(o).
func (d *rsm) loadPop(o, dst int) {
	d.demandContents(o) // fill contents(o) from o's writers
	d.subscribe(o, dst) // contents(o) relays to dst (== addEdge(o,dst))
}

// demandContents finds the writers of o's contents by forward-tracking o's BASE:
// seed &base and let relay() carry it to every store-pointer that writes it.
// Keying off base (not o) is essential (v1 cfl.go:343): a sub-node base+off has
// no &(base+off); its writers are reached from &base via the gep that made &base.off.
func (d *rsm) demandContents(o int) {
	d.markActive(o) // o must be active to HOLD contents (attack-2 secondary; v1 did this at :335)
	b := d.baseOf(o)
	if d.fwdDone[b] {
		return
	}
	d.fwdDone[b] = true
	d.fwdBases = append(d.fwdBases, b) // b may not be markActive'd; track for reset (per-base keying)
	// TRIGGER 2 (base tracked): fwdDone[b] just opened. Every gated arm (load-out,
	// gep, field-push) that checks fwdDone[baseOf(o)] was skipped for the objects
	// of base b already present at their holders. Re-fire those holders.
	for _, p := range d.holdersOfBase[b] {
		d.repush(p)
	}
	for _, a := range d.objAddr[b] {
		d.markActive(a)
		d.emit(a, b)
	}
}

func (d *rsm) fireAssert(c constraint, o int) {
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
			d.emit(c.p, o)
		}
	case cUntag:
		if types.Identical(t, c.typ) {
			d.markActive(c.p)
			d.loadPop(o, c.p) // payload = contents(o), same as a load
		}
	}
}

func (d *rsm) methodOf(t types.Type, name string) *ssa.Function {
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

func (d *rsm) fireCall(c constraint, o int) {
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
func (d *rsm) wire(fn *ssa.Function, c constraint, recvObj int) {
	ps := d.b.fnParams[fn]
	rr := d.b.fnResults[fn]
	off := 0
	if recvObj >= 0 && len(ps) > 0 {
		d.activate(ps[0])
		d.emit(ps[0], recvObj)
		off = 1
	}
	for i, a := range c.args {
		if i+off < len(ps) {
			p := ps[i+off]
			d.activate(p)
			d.activate(a)
			d.dynFwd[a] = append(d.dynFwd[a], p)
			d.replay(a, p, func(dst, o int) { d.emit(dst, o) })
		}
	}
	for i, r := range c.rets {
		if i < len(rr) && r >= 0 {
			d.activate(rr[i])
			d.dynFwd[rr[i]] = append(d.dynFwd[rr[i]], r)
			d.replay(rr[i], r, func(dst, o int) { d.emit(dst, o) })
		}
	}
}

// fireGatedArms fires the three GATE-CONDITIONED complex arms at (v,o): load-out
// (p=*v), gep-forward (p=&v.off), and field-push (o at a store value). Each is
// gated on a monotonic variable -- the target's active bit or o's base being
// tracked (fwdDone). Extracted from run() so the reactivation triggers can
// re-fire them (via repush) once a gate opens; the ungated arms stay fire-once.
// Idempotent: markActive/emit/activate dedupe and subscribe is guarded, so a
// re-evaluation while a gate is still closed (or already fired) is a no-op.
func (d *rsm) fireGatedArms(v, o int) {
	ob := d.baseOf(o)
	for _, dst := range d.loads[v] { // p = *v : LOAD-OUT GATE (§5.3)
		if d.active[dst] || d.fwdDone[ob] {
			d.markActive(dst)
			d.loadPop(o, dst)
		}
	}
	for _, g := range d.gepFwd[v] { // p = &v.off
		if ob >= 0 && g[1] < d.b.objSize[ob] {
			if d.active[g[0]] || d.fwdDone[ob] {
				d.markActive(g[0])
				d.emit(g[0], ob+g[1])
			}
		}
	}
	if d.fwdDone[ob] { // FIELD-PUSH (§5.1): tracked o at a store VALUE
		for _, p := range d.storesWhereValue[v] {
			d.activate(p)
		}
	}
}

// run drives the demand fixpoint over the activated frontier, SEMI-NAIVE: the
// worklist carries only the delta since a node was last processed. The UNGATED
// complex arms (store/call/assert) fire each (v,o) exactly once via doneComplex
// (mirrors solve.go's difference-propagation loop); the GATED arms re-evaluate on
// every delta pass so a reactivation repush can re-fire a gate that has opened.
func (d *rsm) run() {
	for len(d.wl) > 0 {
		v := d.wl[len(d.wl)-1]
		d.wl = d.wl[:len(d.wl)-1]
		d.onWL[v] = false

		dl := d.delta[v]
		if len(dl) == 0 {
			continue
		}
		d.delta[v] = map[int]bool{} // consume delta -> never rescan pts[v]

		// containment-tracking (§5.2): a tracked object landing in a cell tracks the cell
		if d.base[v] >= 0 {
			for o := range dl {
				if d.fwdDone[d.baseOf(o)] {
					d.demandContents(v)
					break
				}
			}
		}

		if len(d.loads[v]) > 0 || len(d.stores[v]) > 0 || len(d.gepFwd[v]) > 0 ||
			len(d.calls[v]) > 0 || len(d.asserts[v]) > 0 {
			dc := d.doneComplex[v]
			if dc == nil {
				dc = map[int]bool{}
				d.doneComplex[v] = dc
			}
			for o := range dl {
				// GATED arms re-evaluate on every delta pass: a reactivation repush
				// re-injects o so a now-open gate fires the previously-skipped arm.
				d.fireGatedArms(v, o)
				if dc[o] {
					continue
				}
				dc[o] = true                      // UNGATED arms fire each (v,o) once (== solve.go:195)
				for _, val := range d.stores[v] { // *v=val, o container: contents(o) ⊇ pts(val)
					d.activate(val)
					d.markActive(o)
					d.subscribe(val, o)
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
		for _, w := range d.copyFwd[v] {
			for o := range dl {
				d.relay(v, w, o)
			}
		}
		for _, w := range d.dynFwd[v] {
			for o := range dl {
				d.relay(v, w, o)
			}
		}
	}
}

// query computes pts for the given seed nodes (and everything they transitively
// require) and returns the number of nodes activated.
func (d *rsm) query(seeds []int) {
	for _, s := range seeds {
		d.activate(s)
	}
	d.run()
}

// reset clears per-query state (pts, activation, worklist) but keeps the indices,
// so a fresh query can run cheaply. Only touched nodes are cleared, so the cost is
// proportional to the previous query's footprint, not to N.
func (d *rsm) reset() {
	for _, i := range d.touched {
		d.pts[i] = nil
		d.delta[i] = nil
		d.doneComplex[i] = nil
		d.active[i] = false
		d.ptDone[i] = false
		d.fwdDone[i] = false
		d.onWL[i] = false
		d.dynFwd[i] = nil
		d.subscribed[i] = nil // subscribe sources are always touched (active)
	}
	for _, b := range d.fwdBases { // per-base fwdDone: a base may not be in touched
		d.fwdDone[b] = false
	}
	d.holdersOfBase = map[int][]int{} // dynamic holder index: rebuilt per query
	d.fwdBases = d.fwdBases[:0]
	d.touched = d.touched[:0]
	d.wl = d.wl[:0]
	d.nActivated = 0
	d.pushes = 0
	d.nDemands = 0
}
