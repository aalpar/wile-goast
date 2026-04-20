// Copyright 2026 Aaron Alpar
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Flow-sensitive SSA narrowing: backward-walk the def-use chain of an
// SSA value to determine the set of concrete types that can flow into it.
//
// Handled cases (confidence=narrow):
//   - Alloc / composite literal
//   - MakeInterface (recurses on wrapped value)
//   - ChangeType / Convert / ChangeInterface (recurses on operand)
//   - Call with concrete return
//   - Call with interface return (inter-procedural over static callees)
//   - Phi (unions over edges)
//   - TypeAssert (records AssertedType)
//   - Extract (recurses on tuple operand)
//   - Const (typed constants yield their static type)
//
// Widened with reason tag:
//   - Parameter -> "parameter"
//   - Global / Field / FieldAddr / UnOp load / Lookup -> "global-load" / "field-load"
//   - nil Const -> "nil-constant"
//   - Invoke-mode Call -> "interface-method-dispatch"
//   - Builtin / closure call with unresolvable callee -> "unresolvable-callee"
//   - Unrecognized instruction -> "unhandled"
//   - Cycle in recursion -> "cycle"
//
// See plans/2026-04-19-axis-b-analyzer-impl-design.md §6.

package goastssa

import (
	"fmt"
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/ssa"
)

// widened is a small constructor for the common widened-with-reason result
// shape. Reduces a 4-line struct literal to a one-liner; every reason
// string in this file flows through here.
func widened(reason string) *narrowResult {
	return &narrowResult{Confidence: "widened", Reasons: []string{reason}}
}

// narrowResult is the Go-side narrowing output. Converted to Scheme by
// buildNarrowResult in prim_narrow.go.
type narrowResult struct {
	Types      []string
	Confidence string
	Reasons    []string
}

// narrow is the public entry point. Wraps narrowWalk with a fresh visited set.
func narrow(fn *ssa.Function, v ssa.Value) *narrowResult {
	visited := make(map[ssa.Value]bool)
	return narrowWalk(fn, v, visited)
}

// narrowWalk performs the backward SSA traversal.
//
// visited tracks values currently *on the descent stack* — classic DFS
// cycle detection. Entries are removed on return (see defer below) so
// a DAG reconvergence point (e.g., same value reached via two phi
// edges) is not misclassified as a cycle.
func narrowWalk(fn *ssa.Function, v ssa.Value, visited map[ssa.Value]bool) *narrowResult {
	if v == nil {
		return &narrowResult{Confidence: "no-paths", Reasons: []string{"nil-input"}}
	}
	if visited[v] {
		return widened("cycle")
	}
	visited[v] = true
	defer delete(visited, v)

	switch x := v.(type) {
	case *ssa.Alloc:
		return narrowFromConcreteType(x.Type())
	case *ssa.MakeInterface:
		return narrowWalk(fn, x.X, visited)
	case *ssa.ChangeType:
		return narrowWalk(fn, x.X, visited)
	case *ssa.ChangeInterface:
		return narrowWalk(fn, x.X, visited)
	case *ssa.Convert:
		return narrowWalk(fn, x.X, visited)
	case *ssa.MultiConvert:
		return narrowWalk(fn, x.X, visited)
	case *ssa.SliceToArrayPointer:
		return narrowFromConcreteType(x.Type())
	case *ssa.TypeAssert:
		return narrowFromConcreteType(x.AssertedType)
	case *ssa.Phi:
		return narrowFromPhi(fn, x, visited)
	case *ssa.Extract:
		return narrowFromExtract(fn, x, visited)
	case *ssa.Call:
		return narrowFromCall(x, visited)
	case *ssa.MakeClosure:
		return narrowFromConcreteType(x.Type())
	case *ssa.MakeMap:
		return narrowFromConcreteType(x.Type())
	case *ssa.MakeSlice:
		return narrowFromConcreteType(x.Type())
	case *ssa.MakeChan:
		return narrowFromConcreteType(x.Type())
	case *ssa.BinOp:
		return narrowFromConcreteType(x.Type())
	case *ssa.UnOp:
		// UnOp covers *, !, -, ^, and <-. ! / - / ^ yield concrete types.
		// Channel receive (<-) and pointer deref (*) may produce interface
		// results — classify by load source so the axis-b inventory shows
		// where narrowing can be extended.
		if isInterfaceResult(x.Type()) {
			switch x.Op {
			case token.MUL:
				return narrowPointerLoad(x, visited)
			case token.ARROW:
				return widened("channel-receive")
			}
			return widened("unexpected-unop")
		}
		return narrowFromConcreteType(x.Type())
	case *ssa.Field:
		if isInterfaceResult(x.Type()) {
			return widened("field-load")
		}
		return narrowFromConcreteType(x.Type())
	case *ssa.FieldAddr:
		if isInterfaceResult(x.Type()) {
			return widened("field-addr")
		}
		return narrowFromConcreteType(x.Type())
	case *ssa.Index:
		return narrowFromConcreteType(x.Type())
	case *ssa.IndexAddr:
		return narrowFromConcreteType(x.Type())
	case *ssa.Lookup:
		if isInterfaceResult(x.Type()) {
			return widened("map-lookup")
		}
		return narrowFromConcreteType(x.Type())
	case *ssa.Slice:
		return narrowFromConcreteType(x.Type())
	case *ssa.Range:
		return narrowFromConcreteType(x.Type())
	case *ssa.Next:
		return narrowFromConcreteType(x.Type())
	case *ssa.Const:
		if x.IsNil() {
			return widened("nil-constant")
		}
		return narrowFromConcreteType(x.Type())
	case *ssa.Parameter:
		return widened("parameter")
	case *ssa.FreeVar:
		return widened("free-var")
	case *ssa.Global:
		return widened("global-load")
	case *ssa.Function:
		return narrowFromConcreteType(x.Type())
	case *ssa.Builtin:
		return widened("unresolvable-callee")
	}
	// Unknown SSA opcode. Include the Go type name so future toolchain
	// additions (or forgotten switch cases) produce a traceable reason
	// instead of silently falling into a generic "widened" bucket.
	return widened(fmt.Sprintf("unhandled:%T", v))
}

// narrowPointerLoad classifies an interface-typed UnOp(*) by the kind
// of address being dereferenced. This replaces the previous catch-all
// "pointer-load" reason with specific tags that reveal where narrowing
// could be extended:
//
//	Global                  -> "global-load"       (package-level var)
//	Alloc (local var)       -> narrowFromAllocStores (recovers stored types)
//	FieldAddr (struct field)-> "field-deref-load"
//	IndexAddr (slice/array) -> "slice-deref-load"
//	other                   -> "pointer-load"      (residual bucket)
//
// The Alloc case is the only one that attempts real narrowing; the
// others widen with an informative reason.
func narrowPointerLoad(x *ssa.UnOp, visited map[ssa.Value]bool) *narrowResult {
	switch src := x.X.(type) {
	case *ssa.Global:
		return narrowFromGlobalInit(src, visited)
	case *ssa.Alloc:
		return narrowFromAllocStores(src, visited)
	case *ssa.FieldAddr:
		return widened("field-deref-load")
	case *ssa.IndexAddr:
		return widened("slice-deref-load")
	}
	return widened("pointer-load")
}

// narrowFromGlobalInit finds every Store instruction in g.Pkg.Init
// (the synthetic package init function) whose Addr is the given
// global, unions the narrowed types of the stored values, and returns
// the merged result.
//
// Strategy: init-only. We trust that package-level globals holding
// interface values are assigned at declaration time and walk only the
// single Init function. Runtime reassignments from other functions
// are NOT tracked. This is a conservative-correctness trade-off: it
// may over-narrow globals that are swapped at runtime (e.g., by tests
// or plug-in registration), but catches the dominant Go idiom
// `var Foo Interface = &Concrete{}`.
//
// Widening reasons produced:
//   - "global-no-pkg"    -> Global has no associated package (rare; build issue)
//   - "global-no-init"   -> package has no synthetic Init function
//   - "global-no-stores" -> Init exists but never writes the global
func narrowFromGlobalInit(g *ssa.Global, visited map[ssa.Value]bool) *narrowResult {
	if g.Pkg == nil {
		return widened("global-no-pkg")
	}
	initFn := g.Pkg.Func("init")
	if initFn == nil {
		return widened("global-no-init")
	}
	results := make([]*narrowResult, 0, 4)
	for _, b := range initFn.Blocks {
		for _, instr := range b.Instrs {
			store, ok := instr.(*ssa.Store)
			if !ok {
				continue
			}
			if store.Addr != ssa.Value(g) {
				continue
			}
			results = append(results, narrowWalk(initFn, store.Val, visited))
		}
	}
	if len(results) == 0 {
		return widened("global-no-stores")
	}
	return mergeResults(results)
}

// narrowFromAllocStores finds every Store instruction in alloc.Parent()
// whose Addr is the given alloc, unions the narrowed types of the
// stored values, and returns the merged result. If no stores are found
// (defensive — Go's zero-value init may skip explicit Store ops),
// widens with "alloc-no-stores".
func narrowFromAllocStores(alloc *ssa.Alloc, visited map[ssa.Value]bool) *narrowResult {
	fn := alloc.Parent()
	if fn == nil {
		return widened("alloc-orphan")
	}
	results := make([]*narrowResult, 0, 4)
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			store, ok := instr.(*ssa.Store)
			if !ok {
				continue
			}
			if store.Addr != ssa.Value(alloc) {
				continue
			}
			results = append(results, narrowWalk(fn, store.Val, visited))
		}
	}
	if len(results) == 0 {
		return widened("alloc-no-stores")
	}
	return mergeResults(results)
}

// narrowFromConcreteType returns a narrow result if t is a concrete
// (non-interface) Go type, widened with interface-result otherwise.
func narrowFromConcreteType(t types.Type) *narrowResult {
	if t == nil {
		// Nil types.Type is an invariant violation (SSA values should
		// always carry a type). Tag it distinctly so debuggers can tell
		// this from a legitimate callee-no-returns 'no-paths'.
		return &narrowResult{Confidence: "no-paths", Reasons: []string{"nil-type"}}
	}
	if isInterfaceResult(t) {
		return widened("interface-result")
	}
	return &narrowResult{
		Types:      []string{types.TypeString(t, nil)},
		Confidence: "narrow",
	}
}

// isInterfaceResult reports whether t's underlying type is an interface.
// Pointer-to-interface is NOT detected — callers that want to distinguish
// *io.Reader from io.Reader must unwrap the pointer first. (This matches
// the reality of the call sites: they pass ssa.Value.Type() and treat
// pointer-to-interface as concrete.)
func isInterfaceResult(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Interface)
	return ok
}

// narrowFromPhi unions the narrow results of every incoming edge.
func narrowFromPhi(fn *ssa.Function, phi *ssa.Phi, visited map[ssa.Value]bool) *narrowResult {
	results := make([]*narrowResult, 0, len(phi.Edges))
	for _, e := range phi.Edges {
		if e == nil {
			continue
		}
		results = append(results, narrowWalk(fn, e, visited))
	}
	return mergeResults(results)
}

// narrowFromExtract narrows the tuple operand. Extract at a given index cannot
// refine beyond what the tuple as a whole yields, so the merge is conservative.
func narrowFromExtract(fn *ssa.Function, ex *ssa.Extract, visited map[ssa.Value]bool) *narrowResult {
	if ex.Tuple == nil {
		return &narrowResult{Confidence: "no-paths", Reasons: []string{"nil-tuple"}}
	}
	tupleType := ex.Tuple.Type()
	tup, ok := tupleType.(*types.Tuple)
	if ok && ex.Index < tup.Len() {
		elem := tup.At(ex.Index).Type()
		inner := narrowFromConcreteType(elem)
		if inner.Confidence == "narrow" {
			return inner
		}
	}
	return narrowWalk(fn, ex.Tuple, visited)
}

// narrowFromCall handles static calls (may recurse into callee's return paths
// when the return type is an interface) and interface-method invokes
// (widened with interface-method-dispatch).
func narrowFromCall(call *ssa.Call, visited map[ssa.Value]bool) *narrowResult {
	if call.Call.IsInvoke() {
		return widened("interface-method-dispatch")
	}
	callee := staticCallee(call)
	if callee == nil {
		return widened("unresolvable-callee")
	}
	if !isInterfaceResult(call.Type()) {
		return narrowFromConcreteType(call.Type())
	}
	return narrowFromCalleeReturns(callee, visited)
}

// staticCallee returns the concrete callee *ssa.Function if the call is a static
// function call, nil otherwise (e.g., function-variable or builtin).
func staticCallee(call *ssa.Call) *ssa.Function {
	fn, ok := call.Call.Value.(*ssa.Function)
	if !ok {
		return nil
	}
	return fn
}

// narrowFromCalleeReturns walks every Return instruction in callee and narrows
// the result operand at index 0 (interfaces are always single-return by the
// time we reach here — multi-return functions hit Extract before Call).
func narrowFromCalleeReturns(callee *ssa.Function, visited map[ssa.Value]bool) *narrowResult {
	results := make([]*narrowResult, 0, 4)
	for _, b := range callee.Blocks {
		for _, instr := range b.Instrs {
			ret, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}
			if len(ret.Results) == 0 {
				continue
			}
			results = append(results, narrowWalk(callee, ret.Results[0], visited))
		}
	}
	if len(results) == 0 {
		// Callee has no Return instructions (panic-only, infinite loop,
		// etc.). Distinct from "I couldn't find paths" or "input was nil".
		return &narrowResult{Confidence: "no-paths", Reasons: []string{"callee-no-returns"}}
	}
	return mergeResults(results)
}

// mergeResults combines multiple narrow results into one, unioning types and
// reasons and taking the weakest confidence.
func mergeResults(rs []*narrowResult) *narrowResult {
	if len(rs) == 0 {
		return &narrowResult{Confidence: "no-paths"}
	}
	typeSet := make(map[string]bool)
	reasonSet := make(map[string]bool)
	anyWidened := false
	allNoPaths := true
	for _, r := range rs {
		if r == nil {
			continue
		}
		for _, t := range r.Types {
			typeSet[t] = true
		}
		for _, reason := range r.Reasons {
			reasonSet[reason] = true
		}
		if r.Confidence == "widened" {
			anyWidened = true
			allNoPaths = false
		} else if r.Confidence != "no-paths" {
			allNoPaths = false
		}
	}
	types := keysSorted(typeSet)
	reasons := keysSorted(reasonSet)
	conf := "narrow"
	if anyWidened {
		conf = "widened"
	} else if allNoPaths {
		conf = "no-paths"
	}
	return &narrowResult{Types: types, Confidence: conf, Reasons: reasons}
}

// keysSorted returns the keys of m in deterministic order. Callers rely on
// stability for reproducible output.
func keysSorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
