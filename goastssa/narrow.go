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
	"go/types"

	"golang.org/x/tools/go/ssa"
)

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
func narrowWalk(fn *ssa.Function, v ssa.Value, visited map[ssa.Value]bool) *narrowResult {
	if v == nil {
		return &narrowResult{Confidence: "no-paths"}
	}
	if visited[v] {
		return &narrowResult{Confidence: "widened", Reasons: []string{"cycle"}}
	}
	visited[v] = true

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
		// Dereference (op == *): reading from a pointer cell. Widen with field-load
		// unless the result type is itself concrete and we can use it.
		if isInterfaceResult(x.Type()) {
			return &narrowResult{Confidence: "widened", Reasons: []string{"field-load"}}
		}
		return narrowFromConcreteType(x.Type())
	case *ssa.Field:
		if isInterfaceResult(x.Type()) {
			return &narrowResult{Confidence: "widened", Reasons: []string{"field-load"}}
		}
		return narrowFromConcreteType(x.Type())
	case *ssa.FieldAddr:
		if isInterfaceResult(x.Type()) {
			return &narrowResult{Confidence: "widened", Reasons: []string{"field-load"}}
		}
		return narrowFromConcreteType(x.Type())
	case *ssa.Index:
		return narrowFromConcreteType(x.Type())
	case *ssa.IndexAddr:
		return narrowFromConcreteType(x.Type())
	case *ssa.Lookup:
		if isInterfaceResult(x.Type()) {
			return &narrowResult{Confidence: "widened", Reasons: []string{"field-load"}}
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
			return &narrowResult{Confidence: "widened", Reasons: []string{"nil-constant"}}
		}
		return narrowFromConcreteType(x.Type())
	case *ssa.Parameter:
		return &narrowResult{Confidence: "widened", Reasons: []string{"parameter"}}
	case *ssa.FreeVar:
		return &narrowResult{Confidence: "widened", Reasons: []string{"free-var"}}
	case *ssa.Global:
		return &narrowResult{Confidence: "widened", Reasons: []string{"global-load"}}
	case *ssa.Function:
		return narrowFromConcreteType(x.Type())
	case *ssa.Builtin:
		return &narrowResult{Confidence: "widened", Reasons: []string{"unresolvable-callee"}}
	}
	return &narrowResult{Confidence: "widened", Reasons: []string{"unhandled"}}
}

// narrowFromConcreteType returns a narrow result if t is a concrete
// (non-interface) Go type, widened with field-load otherwise.
func narrowFromConcreteType(t types.Type) *narrowResult {
	if t == nil {
		return &narrowResult{Confidence: "no-paths"}
	}
	if isInterfaceResult(t) {
		return &narrowResult{Confidence: "widened", Reasons: []string{"field-load"}}
	}
	return &narrowResult{
		Types:      []string{types.TypeString(t, nil)},
		Confidence: "narrow",
	}
}

// isInterfaceResult reports whether t is an interface type (possibly pointer-to).
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
		return &narrowResult{Confidence: "no-paths"}
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
		return &narrowResult{Confidence: "widened", Reasons: []string{"interface-method-dispatch"}}
	}
	callee := staticCallee(call)
	if callee == nil {
		return &narrowResult{Confidence: "widened", Reasons: []string{"unresolvable-callee"}}
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
		return &narrowResult{Confidence: "no-paths"}
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
	sortStrings(out)
	return out
}

// sortStrings sorts s in place (standard-library sort, extracted so the
// import list stays small).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
