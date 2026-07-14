// csize v2: extract Andersen points-to constraints, solve lfp(F), count the
// membership tests the fixpoint performs.
//
// v2 fixes the precision confound E29 recorded:
//   - tuple results are PER-COMPONENT. Go is saturated with (v, err); merging
//     every multi-return into one node poisons the sets downstream.
//   - functions and closures are OBJECTS, so a func value has provenance.
//   - indirect calls get a real `icall` constraint that wires params to args,
//     instead of the bogus load v1 emitted.
//   - closure bindings flow to the callee's FREE VARS.
package main

import (
	"fmt"
	"go/types"
	"os"
	"sort"
	"time"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type form int

const (
	cAddr form = iota
	cCopy
	cGep
	cLoad
	cStore
	cInvoke  // ∀o ∈ pts(recv): impl(type(o), m)
	cICall   // ∀o ∈ pts(fv):  the function o denotes
	cTFilter // dst = src.(Iface):    ∀o ∈ pts(src) with AssignableTo(type(o), Iface): add(dst, o)
	cUntag   // dst = src.(Concrete): ∀o ∈ pts(src) with Identical(type(o), Concrete): dst ← pts(o)
	nForms
)

var formName = [nForms]string{"addr", "copy", "gep", "load", "store", "invoke", "icall", "tfilter", "untag"}

type constraint struct {
	f      form
	p, q   int
	off    int
	method string
	typ    types.Type // asserted type, for cTFilter / cUntag
	args   []int
	rets   []int
}

type builder struct {
	nodeOf map[ssa.Value]int
	nNodes int

	objBase map[ssa.Value]int
	objType map[int]types.Type
	objSize map[int]int
	objFunc map[int]*ssa.Function
	nObjs   int

	tupleBase map[ssa.Value]int
	fnParams  map[*ssa.Function][]int
	fnResults map[*ssa.Function][]int
	cs        []constraint
}

func tracked(t types.Type) bool {
	switch t := t.Underlying().(type) {
	case *types.Pointer, *types.Interface, *types.Signature, *types.Map, *types.Chan, *types.Slice:
		return true
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			if tracked(t.Field(i).Type()) {
				return true
			}
		}
	case *types.Array:
		return tracked(t.Elem())
	case *types.Basic:
		return t.Kind() == types.UnsafePointer
	case *types.Tuple:
		for i := 0; i < t.Len(); i++ {
			if tracked(t.At(i).Type()) {
				return true
			}
		}
	}
	return false
}

func flatSize(t types.Type, depth int) int {
	if depth > 6 {
		return 1
	}
	switch u := t.Underlying().(type) {
	case *types.Pointer, *types.Interface, *types.Signature:
		return 1
	case *types.Basic:
		if u.Kind() == types.UnsafePointer {
			return 1
		}
		return 0
	case *types.Struct:
		n := 0
		for i := 0; i < u.NumFields(); i++ {
			n += flatSize(u.Field(i).Type(), depth+1)
		}
		return n
	case *types.Array:
		return flatSize(u.Elem(), depth+1)
	case *types.Slice, *types.Chan:
		return 1
	case *types.Map:
		return 2
	}
	return 0
}

func (b *builder) fresh() int { n := b.nNodes; b.nNodes++; return n }

func (b *builder) node(v ssa.Value) int {
	if v == nil {
		return -1
	}
	if n, ok := b.nodeOf[v]; ok {
		return n
	}
	n := b.fresh()
	b.nodeOf[v] = n
	return n
}

func (b *builder) obj(key ssa.Value, t types.Type) int {
	if base, ok := b.objBase[key]; ok {
		return base
	}
	size := flatSize(t, 0)
	if size == 0 {
		size = 1
	}
	base := b.nNodes
	b.nNodes += size
	b.objBase[key] = base
	b.objType[base] = t
	b.objSize[base] = size
	b.nObjs++
	return base
}

func (b *builder) add(c constraint) { b.cs = append(b.cs, c) }

func fieldOffset(st *types.Struct, i int) int {
	off := 0
	for k := 0; k < i; k++ {
		off += flatSize(st.Field(k).Type(), 0)
	}
	return off
}

// resultNodes: a tuple-typed value gets one node PER COMPONENT.
func (b *builder) resultNodes(v ssa.Value) []int {
	if v == nil {
		return nil
	}
	tup, ok := v.Type().Underlying().(*types.Tuple)
	if !ok {
		return []int{b.node(v)}
	}
	base, ok := b.tupleBase[v]
	if !ok {
		base = b.nNodes
		b.nNodes += tup.Len()
		b.tupleBase[v] = base
	}
	out := make([]int, tup.Len())
	for i := range out {
		out[i] = base + i
	}
	return out
}

func (b *builder) addGlobalsAndFuncs(prog *ssa.Program, fns []*ssa.Function) {
	for _, pkg := range prog.AllPackages() {
		for _, m := range pkg.Members {
			if g, ok := m.(*ssa.Global); ok {
				o := b.obj(g, g.Type().(*types.Pointer).Elem())
				b.add(constraint{f: cAddr, p: b.node(g), q: o})
			}
		}
	}
	for _, fn := range fns {
		o := b.obj(fn, fn.Signature)
		b.objFunc[o] = fn
		b.add(constraint{f: cAddr, p: b.node(fn), q: o})
	}
}

func (b *builder) instr(fn *ssa.Function, ins ssa.Instruction) {
	switch v := ins.(type) {
	case *ssa.Alloc:
		b.add(constraint{f: cAddr, p: b.node(v), q: b.obj(v, v.Type().(*types.Pointer).Elem())})
	case *ssa.MakeSlice:
		b.add(constraint{f: cAddr, p: b.node(v), q: b.obj(v, v.Type())})
	case *ssa.MakeMap:
		b.add(constraint{f: cAddr, p: b.node(v), q: b.obj(v, v.Type())})
	case *ssa.MakeChan:
		b.add(constraint{f: cAddr, p: b.node(v), q: b.obj(v, v.Type())})

	case *ssa.MakeInterface:
		o := b.obj(v, v.X.Type())
		b.add(constraint{f: cAddr, p: b.node(v), q: o})
		if tracked(v.X.Type()) {
			b.add(constraint{f: cStore, p: b.node(v), q: b.node(v.X)})
		}

	case *ssa.MakeClosure:
		cf, _ := v.Fn.(*ssa.Function)
		o := b.obj(v, v.Type())
		if cf != nil {
			b.objFunc[o] = cf
		}
		b.add(constraint{f: cAddr, p: b.node(v), q: o})
		if cf != nil {
			for i, bv := range v.Bindings {
				if i < len(cf.FreeVars) && tracked(bv.Type()) {
					b.add(constraint{f: cCopy, p: b.node(cf.FreeVars[i]), q: b.node(bv)})
				}
			}
		}

	case *ssa.FieldAddr:
		pt, ok := v.X.Type().Underlying().(*types.Pointer)
		if !ok {
			return
		}
		st, ok := pt.Elem().Underlying().(*types.Struct)
		if !ok {
			return
		}
		b.add(constraint{f: cGep, p: b.node(v), q: b.node(v.X), off: fieldOffset(st, v.Field)})

	case *ssa.IndexAddr:
		b.add(constraint{f: cGep, p: b.node(v), q: b.node(v.X), off: 0})

	case *ssa.Field:
		if tracked(v.Type()) {
			b.add(constraint{f: cCopy, p: b.node(v), q: b.node(v.X)})
		}
	case *ssa.Index:
		if tracked(v.Type()) {
			b.add(constraint{f: cCopy, p: b.node(v), q: b.node(v.X)})
		}

	case *ssa.UnOp:
		if (v.Op.String() == "*" || v.Op.String() == "<-") && tracked(v.Type()) {
			b.add(constraint{f: cLoad, p: b.node(v), q: b.node(v.X)})
		}

	case *ssa.Store:
		if tracked(v.Val.Type()) {
			b.add(constraint{f: cStore, p: b.node(v.Addr), q: b.node(v.Val)})
		}
	case *ssa.MapUpdate:
		if tracked(v.Value.Type()) {
			b.add(constraint{f: cStore, p: b.node(v.Map), q: b.node(v.Value)})
		}
	case *ssa.Lookup:
		if tracked(v.Type()) {
			b.add(constraint{f: cLoad, p: b.node(v), q: b.node(v.X)})
		}
	case *ssa.Send:
		if tracked(v.X.Type()) {
			b.add(constraint{f: cStore, p: b.node(v.Chan), q: b.node(v.X)})
		}

	case *ssa.Phi:
		if tracked(v.Type()) {
			for _, e := range v.Edges {
				b.add(constraint{f: cCopy, p: b.node(v), q: b.node(e)})
			}
		}

	case *ssa.ChangeType:
		if tracked(v.Type()) {
			b.add(constraint{f: cCopy, p: b.node(v), q: b.node(v.X)})
		}
	case *ssa.Convert:
		if tracked(v.Type()) && tracked(v.X.Type()) {
			b.add(constraint{f: cCopy, p: b.node(v), q: b.node(v.X)})
		}
	case *ssa.ChangeInterface:
		b.add(constraint{f: cCopy, p: b.node(v), q: b.node(v.X)})
	case *ssa.Slice:
		if tracked(v.Type()) {
			b.add(constraint{f: cCopy, p: b.node(v), q: b.node(v.X)})
		}

	case *ssa.TypeAssert:
		// Mirrors go/pointer's typeAssert(T, dst, src, exact=true) (gen.go:388).
		// The v1 unconditional cCopy passed the WHOLE interface points-to set
		// through every assertion, unfiltered -- the source of the mean-|pts| blowup
		// (E32/E33). An assertion is a FILTER, not a copy.
		rs := b.resultNodes(v)
		if len(rs) > 0 && tracked(v.X.Type()) {
			if types.IsInterface(v.AssertedType) {
				// dst keeps each tagged object o assignable to the asserted iface.
				b.add(constraint{f: cTFilter, p: rs[0], q: b.node(v.X), typ: v.AssertedType})
			} else {
				// dst gets the PAYLOAD of each tagged object whose concrete type is
				// exactly the asserted type.
				b.add(constraint{f: cUntag, p: rs[0], q: b.node(v.X), typ: v.AssertedType})
			}
		}

	case *ssa.Extract:
		// PER-COMPONENT: the v1 whole-tuple copy is what merged every (v, err).
		if !tracked(v.Type()) {
			return
		}
		src := b.resultNodes(v.Tuple)
		if v.Index < len(src) {
			b.add(constraint{f: cCopy, p: b.node(v), q: src[v.Index]})
		}

	case *ssa.Range:
		b.add(constraint{f: cCopy, p: b.node(v), q: b.node(v.X)})
	case *ssa.Next:
		rs := b.resultNodes(v)
		for i := 1; i < len(rs); i++ {
			b.add(constraint{f: cLoad, p: rs[i], q: b.node(v.Iter)})
		}

	case *ssa.Return:
		rr := b.fnResults[fn]
		for i, r := range v.Results {
			if i < len(rr) && tracked(r.Type()) {
				b.add(constraint{f: cCopy, p: rr[i], q: b.node(r)})
			}
		}

	case *ssa.Call:
		b.call(v, v.Common())
	case *ssa.Go:
		b.call(nil, v.Common())
	case *ssa.Defer:
		b.call(nil, v.Common())
	}
}

func (b *builder) call(res ssa.Value, cc *ssa.CallCommon) {
	if bi, ok := cc.Value.(*ssa.Builtin); ok {
		b.builtin(bi, res, cc)
		return
	}
	var rets []int
	if res != nil {
		rets = b.resultNodes(res)
	}

	if cc.IsInvoke() {
		var args []int
		for _, a := range cc.Args {
			args = append(args, b.node(a))
		}
		b.add(constraint{f: cInvoke, p: b.node(cc.Value), method: cc.Method.Name(), args: args, rets: rets})
		return
	}

	if callee := cc.StaticCallee(); callee != nil {
		ps := b.fnParams[callee]
		for i, a := range cc.Args {
			if i < len(ps) && tracked(a.Type()) {
				b.add(constraint{f: cCopy, p: ps[i], q: b.node(a)})
			}
		}
		rr := b.fnResults[callee]
		for i := range rets {
			if i < len(rr) {
				b.add(constraint{f: cCopy, p: rets[i], q: rr[i]})
			}
		}
		return
	}

	// INDIRECT CALL through a func value: a real dynamic constraint, resolved
	// against the function objects in pts(fv). v1 emitted a bogus load here.
	var args []int
	for _, a := range cc.Args {
		args = append(args, b.node(a))
	}
	b.add(constraint{f: cICall, p: b.node(cc.Value), args: args, rets: rets})
}

func (b *builder) builtin(bi *ssa.Builtin, res ssa.Value, cc *ssa.CallCommon) {
	switch bi.Name() {
	case "append":
		if res == nil || len(cc.Args) == 0 {
			return
		}
		o := b.obj(res, res.Type())
		b.add(constraint{f: cAddr, p: b.node(res), q: o})
		b.add(constraint{f: cCopy, p: b.node(res), q: b.node(cc.Args[0])})
		if len(cc.Args) >= 2 && tracked(cc.Args[1].Type()) {
			tmp := b.fresh()
			b.add(constraint{f: cLoad, p: tmp, q: b.node(cc.Args[1])})
			b.add(constraint{f: cStore, p: b.node(res), q: tmp})
		}
	case "copy":
		if len(cc.Args) >= 2 && tracked(cc.Args[1].Type()) {
			tmp := b.fresh()
			b.add(constraint{f: cLoad, p: tmp, q: b.node(cc.Args[1])})
			b.add(constraint{f: cStore, p: b.node(cc.Args[0]), q: tmp})
		}
	}
}

func main() {
	dir := os.Args[1]
	t0 := time.Now()
	cfg := &packages.Config{Mode: packages.LoadAllSyntax, Dir: dir}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	tLoad := time.Since(t0)

	b := &builder{
		nodeOf:    map[ssa.Value]int{},
		objBase:   map[ssa.Value]int{},
		objType:   map[int]types.Type{},
		objSize:   map[int]int{},
		objFunc:   map[int]*ssa.Function{},
		tupleBase: map[ssa.Value]int{},
		fnParams:  map[*ssa.Function][]int{},
		fnResults: map[*ssa.Function][]int{},
	}

	all := ssautil.AllFunctions(prog)
	fns := make([]*ssa.Function, 0, len(all))
	for fn := range all {
		if fn.Blocks != nil {
			fns = append(fns, fn)
		}
	}
	sort.Slice(fns, func(i, j int) bool { return fns[i].String() < fns[j].String() })

	t1 := time.Now()
	for _, fn := range fns {
		var ps []int
		for _, p := range fn.Params {
			ps = append(ps, b.node(p))
		}
		b.fnParams[fn] = ps
		rr := make([]int, fn.Signature.Results().Len())
		for i := range rr {
			rr[i] = b.fresh()
		}
		b.fnResults[fn] = rr
	}
	b.addGlobalsAndFuncs(prog, fns)
	for _, fn := range fns {
		for _, blk := range fn.Blocks {
			for _, ins := range blk.Instrs {
				b.instr(fn, ins)
			}
		}
	}
	tExtract := time.Since(t1)

	var byForm [nForms]int
	for _, c := range b.cs {
		byForm[c.f]++
	}
	fmt.Printf("TARGET            %s\n", dir)
	fmt.Printf("load+ssa          %v\n", tLoad.Round(time.Millisecond))
	fmt.Printf("extract           %v\n\n", tExtract.Round(time.Millisecond))
	fmt.Printf("functions         %d\n", len(fns))
	fmt.Printf("nodes             %d\n", b.nNodes)
	fmt.Printf("objects           %d\n", b.nObjs)
	fmt.Printf("constraints       %d\n", len(b.cs))
	for f := form(0); f < nForms; f++ {
		fmt.Printf("  %-8s        %8d\n", formName[f], byForm[f])
	}

	budget := 600 * time.Second
	if os.Getenv("BUDGET") == "short" {
		budget = 60 * time.Second
	}
	solveDiff(b, prog, budget)
}
