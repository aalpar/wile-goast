// Scale probe: does the vendored Andersen points-to analysis survive a real Go program?
// Andersen needs only Mains, so we avoid ssautil.AllFunctions (which trips a generics
// panic in RuntimeTypes on x/tools v0.46).
package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
	"vendprobe/pointer"
)

func mem() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc / (1 << 20)
}

func edges(cg *callgraph.Graph) int {
	n := 0
	for _, node := range cg.Nodes {
		n += len(node.Out)
	}
	return n
}

func main() {
	dir, pattern := os.Args[1], os.Args[2]
	t0 := time.Now()
	cfg := &packages.Config{Mode: packages.LoadAllSyntax, Dir: dir}
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	prog, _ := ssautil.AllPackages(pkgs, 0)
	prog.Build()
	fmt.Printf("  load+ssa  : %-9v  pkgs=%d  ssa_pkgs=%d  heap=%dMB\n",
		time.Since(t0).Round(time.Millisecond), len(pkgs), len(prog.AllPackages()), mem())

	var mains []*ssa.Package
	for _, p := range prog.AllPackages() {
		if p.Pkg.Name() == "main" && p.Func("main") != nil {
			mains = append(mains, p)
		}
	}
	if len(mains) == 0 {
		fmt.Println("  andersen  : SKIPPED (needs a main package)")
		return
	}
	t2 := time.Now()
	res, err := pointer.Analyze(&pointer.Config{Mains: mains, BuildCallGraph: true})
	if err != nil {
		fmt.Printf("  andersen  : FAILED after %v: %v\n", time.Since(t2).Round(time.Millisecond), err)
		os.Exit(1)
	}
	fmt.Printf("  andersen  : %-9v  nodes=%d  edges=%d  heap=%dMB\n",
		time.Since(t2).Round(time.Millisecond), len(res.CallGraph.Nodes), edges(res.CallGraph), mem())
}
