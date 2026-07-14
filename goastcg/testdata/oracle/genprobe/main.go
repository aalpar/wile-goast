// Print every dynamic-dispatch call site and the callees Andersen resolves for it.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
	"vendprobe/pointer"
)

func main() {
	dir := os.Args[1]
	mode := ssa.BuilderMode(0)
	if len(os.Args) > 2 && os.Args[2] == "instantiate" {
		mode |= ssa.InstantiateGenerics
	}
	cfg := &packages.Config{Mode: packages.LoadAllSyntax, Dir: dir}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil { fmt.Fprintln(os.Stderr, "load:", err); os.Exit(1) }
	prog, _ := ssautil.AllPackages(pkgs, mode)
	prog.Build()

	var mains []*ssa.Package
	for _, p := range prog.AllPackages() {
		if p.Pkg.Name() == "main" && p.Func("main") != nil { mains = append(mains, p) }
	}
	res, err := pointer.Analyze(&pointer.Config{Mains: mains, BuildCallGraph: true})
	if err != nil { fmt.Println("andersen FAILED:", err); os.Exit(1) }

	var out []string
	callgraph.GraphVisitEdges(res.CallGraph, func(e *callgraph.Edge) error {
		if e.Site == nil { return nil }
		if !e.Site.Common().IsInvoke() { return nil }
		out = append(out, fmt.Sprintf("  INVOKE %-28s -> %s",
			e.Site.Common().Value.Name()+"."+e.Site.Common().Method.Name(), e.Callee.Func.String()))
		return nil
	})
	sort.Strings(out)
	if len(out) == 0 { fmt.Println("  (no interface-dispatch edges resolved at all)") }
	fmt.Println(strings.Join(out, "\n"))
}
