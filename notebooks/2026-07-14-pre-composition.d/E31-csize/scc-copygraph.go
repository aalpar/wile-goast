// Is the god-set mass explained by copy-cycles that cycle-elimination would collapse?
// Purely structural: SCCs of the COPY (subset) edge graph. No fixpoint, no oracle.
package main

import (
	"fmt"
	"go/types"
	"os"
	"sort"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func main() {
	dir := os.Args[1]
	cfg := &packages.Config{Mode: packages.LoadAllSyntax, Dir: dir}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil { fmt.Fprintln(os.Stderr, "load:", err); os.Exit(1) }
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()

	// Build the copy-edge graph directly from SSA (the cCopy-producing instructions):
	// Phi, ChangeType, Convert, ChangeInterface, Slice, and the param<-arg / ret<-ret
	// bindings of static calls. Node = ssa.Value; edge dst<-src means pts(dst) >= pts(src).
	id := map[ssa.Value]int{}
	nid := func(v ssa.Value) int {
		if v == nil { return -1 }
		if i, ok := id[v]; ok { return i }
		i := len(id); id[v] = i; return i
	}
	type edge struct{ dst, src int }
	var edges []edge
	addE := func(dst, src ssa.Value) {
		if dst == nil || src == nil { return }
		edges = append(edges, edge{nid(dst), nid(src)})
	}
	tracked := func(t types.Type) bool {
		switch t.Underlying().(type) {
		case *types.Pointer, *types.Interface, *types.Signature, *types.Map, *types.Chan, *types.Slice:
			return true
		}
		return false
	}
	for fn := range ssautil.AllFunctions(prog) {
		params := map[*ssa.Function][]*ssa.Parameter{}
		_ = params
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				switch v := ins.(type) {
				case *ssa.Phi:
					for _, e := range v.Edges { addE(v, e) }
				case *ssa.ChangeType:
					addE(v, v.X)
				case *ssa.Convert:
					addE(v, v.X)
				case *ssa.ChangeInterface:
					addE(v, v.X)
				case *ssa.Slice:
					addE(v, v.X)
				case *ssa.Call:
					if callee := v.Common().StaticCallee(); callee != nil {
						for i, a := range v.Common().Args {
							if i < len(callee.Params) && tracked(a.Type()) { addE(callee.Params[i], a) }
						}
					}
				}
			}
		}
	}
	N := len(id)

	// Tarjan SCC.
	g := make([][]int, N)
	for _, e := range edges {
		if e.dst >= 0 && e.src >= 0 { g[e.src] = append(g[e.src], e.dst) } // edge src->dst (flow direction)
	}
	index := make([]int, N); low := make([]int, N); onst := make([]bool, N)
	comp := make([]int, N)
	for i := range index { index[i] = -1; comp[i] = -1 }
	var stack []int; idx, nc := 0, 0
	var dfs func(v int)
	// iterative Tarjan to avoid stack overflow on 1M nodes
	type frame struct{ v, i int }
	dfs = func(root int) {
		st := []frame{{root, 0}}
		index[root] = idx; low[root] = idx; idx++; stack = append(stack, root); onst[root] = true
		for len(st) > 0 {
			f := &st[len(st)-1]
			if f.i < len(g[f.v]) {
				w := g[f.v][f.i]; f.i++
				if index[w] == -1 {
					index[w] = idx; low[w] = idx; idx++; stack = append(stack, w); onst[w] = true
					st = append(st, frame{w, 0})
				} else if onst[w] && index[w] < low[f.v] {
					low[f.v] = index[w]
				}
			} else {
				if low[f.v] == index[f.v] {
					for {
						w := stack[len(stack)-1]; stack = stack[:len(stack)-1]; onst[w] = false
						comp[w] = nc
						if w == f.v { break }
					}
					nc++
				}
				st = st[:len(st)-1]
				if len(st) > 0 && low[f.v] < low[st[len(st)-1].v] { low[st[len(st)-1].v] = low[f.v] }
			}
		}
	}
	_ = dfs
	for i := 0; i < N; i++ { if index[i] == -1 { dfs(i) } }

	size := make([]int, nc)
	for i := 0; i < N; i++ { size[comp[i]]++ }
	sort.Sort(sort.Reverse(sort.IntSlice(size)))
	big, nodesInBig := 0, 0
	for _, s := range size { if s > 1 { big++; nodesInBig += s } }
	fmt.Printf("%-10s copy-graph nodes %d  edges %d  SCCs %d  nontrivial-SCCs %d  nodes-in-cycles %d\n",
		dir[len(dir)-1:], N, len(edges), nc, big, nodesInBig)
	fmt.Printf("  largest SCCs: ")
	for i := 0; i < 8 && i < len(size); i++ { fmt.Printf("%d ", size[i]) }
	fmt.Println()
}
