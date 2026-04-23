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
//   - Invoke-mode Call -> one of "dispatch-*" (see reason constants)
//   - Builtin / closure call with unresolvable callee -> "unresolvable-callee"
//   - Unrecognized instruction -> "unhandled:<T>"
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

// confidence classifies a narrowResult. Typed enum over string literals
// so typos fail to compile and exhaustive switches over outcomes stay
// tractable. Scheme wire format (see prim_narrow.go) is the String()
// output — that mapping is the only place the string spelling lives.
//
// The zero value (confUnknown) is reserved as a safety net for
// uninitialized results and intentionally NOT part of the user-visible
// wire format; production constructors always set one of the three
// defined states.
type confidence int

const (
	confUnknown confidence = iota
	confNarrow
	confWidened
	confNoPaths
)

// String returns the Scheme-visible symbol name for c. The switch is
// exhaustive over the declared states; confUnknown (and any unexpected
// value) returns "" so mis-construction surfaces loudly at the bridge
// instead of producing a plausible-looking symbol.
func (c confidence) String() string {
	switch c {
	case confNarrow:
		return "narrow"
	case confWidened:
		return "widened"
	case confNoPaths:
		return "no-paths"
	default:
		return ""
	}
}

// Reason constants used for widened and no-paths results. Scheme callers
// see these as symbols; Go callers should reference them by name so
// typos fail to compile and grep-for-all-reasons works. Tests assert
// against these identifiers rather than string literals.
//
// Widened reasons — classify WHY the narrowing gave up.
const (
	// Structural / traversal reasons.
	reasonCycle              = "cycle"
	reasonChannelReceive     = "channel-receive"
	reasonUnexpectedUnop     = "unexpected-unop"
	reasonFieldLoad          = "field-load"
	reasonFieldAddr          = "field-addr"
	reasonMapLookup          = "map-lookup"
	reasonNilConstant        = "nil-constant"
	reasonParameter          = "parameter"
	reasonFreeVar            = "free-var"
	reasonGlobalLoad         = "global-load"
	reasonUnresolvableCallee = "unresolvable-callee"
	reasonInterfaceResult    = "interface-result"

	// Pointer-load classification (narrowPointerLoad).
	reasonSliceDerefLoad = "slice-deref-load"
	reasonPointerLoad    = "pointer-load"

	// Field-store backward search.
	reasonFieldNoProg       = "field-no-prog"
	reasonFieldNoStructType = "field-no-struct-type"
	reasonFieldNoStores     = "field-no-stores"

	// Global-init backward search.
	reasonGlobalNoPkg    = "global-no-pkg"
	reasonGlobalNoInit   = "global-no-init"
	reasonGlobalNoStores = "global-no-stores"

	// Alloc-store backward search.
	reasonAllocOrphan   = "alloc-orphan"
	reasonAllocNoStores = "alloc-no-stores"

	// Interface-method invoke dispatch. These distinguish legitimate
	// "no implementors" cases from ill-formed invokes (SSA-build
	// invariant violations) — both produced the same tag previously.
	reasonDispatchNoProg           = "dispatch-no-prog"
	reasonDispatchNilMethod        = "dispatch-nil-method"
	reasonDispatchBadSignature     = "dispatch-bad-signature"
	reasonDispatchNonInterfaceRecv = "dispatch-non-interface-recv"
	reasonDispatchAllSynthetic     = "dispatch-all-synthetic"
	reasonDispatchNoImplementors   = "dispatch-no-implementors"
)

// No-paths reasons — classify WHY the input/callee had nothing to walk.
const (
	reasonNilInput        = "nil-input"
	reasonNilType         = "nil-type"
	reasonNilTuple        = "nil-tuple"
	reasonCalleeNoReturns = "callee-no-returns"
)

// narrowResult is the Go-side narrowing output. Converted to Scheme by
// buildNarrowResult in prim_narrow.go (Confidence is serialized as the
// symbol returned by Confidence.String()).
//
// Fields are populated according to Confidence:
//
//	confNarrow    : Types non-empty; Reasons empty.
//	confWidened   : Reasons non-empty; Types may be empty.
//	confNoPaths   : Types empty; Reasons optionally explains why.
//
// Every construction site routes through widened / newNarrow / newNoPaths
// so the invariants stay local to this file.
type narrowResult struct {
	Types      []string
	Confidence confidence
	Reasons    []string
}

// widened constructs a confWidened result carrying a single reason tag.
// Every "I can't narrow further, here's why" exit in this file flows
// through this constructor.
func widened(reason string) *narrowResult {
	return &narrowResult{Confidence: confWidened, Reasons: []string{reason}}
}

// newNarrow constructs a confNarrow result for a single concrete Go
// type. Callers stringify via types.TypeString before the call.
func newNarrow(typeStr string) *narrowResult {
	return &narrowResult{Confidence: confNarrow, Types: []string{typeStr}}
}

// newNoPaths constructs a confNoPaths result. If reason is empty the
// Reasons slice is left nil (legitimate "no sites matched" has no
// tag); otherwise the reason is recorded for debuggers.
func newNoPaths(reason string) *narrowResult {
	if reason == "" {
		return &narrowResult{Confidence: confNoPaths}
	}
	return &narrowResult{Confidence: confNoPaths, Reasons: []string{reason}}
}

// narrowCtx carries the shared state for a single narrow() invocation.
// Replaces a hand-threaded visited map and ambient prog lookups:
//
//   - visited tracks the current descent stack for cycle detection.
//     Callers use enter/leave to maintain the invariant; the DFS
//     discipline is hidden behind methods instead of scattered
//     `defer delete(visited, v)` calls.
//
//   - prog is cached from the initial function's Program and used by
//     helpers that enumerate packages (field stores, invoke dispatch).
//
//   - allPackages caches prog.AllPackages() on first access. An invoke
//     dispatch can trigger N field-store searches, each of which would
//     otherwise re-enumerate the package list; lazy caching flattens
//     that cost.
//
// A fresh ctx is created per narrow() call so caches don't persist
// between queries on the same program.
type narrowCtx struct {
	prog        *ssa.Program
	visited     map[ssa.Value]bool
	allPackages []*ssa.Package
}

// newNarrowCtx constructs a ctx from the entry function. fn may carry
// a nil Prog (isolated SSA fragments in tests); helpers check prog
// nullability before dereferencing.
func newNarrowCtx(fn *ssa.Function) *narrowCtx {
	c := &narrowCtx{visited: make(map[ssa.Value]bool)}
	if fn != nil {
		c.prog = fn.Prog
	}
	return c
}

// enter records v as being on the descent stack. Returns false when v
// is already present (cycle detected); callers should widen with
// reasonCycle in that case. A successful enter MUST be paired with a
// deferred leave so DAG reconvergence (same value reached via two phi
// edges at different stack depths) is not misclassified as a cycle.
func (c *narrowCtx) enter(v ssa.Value) bool {
	if c.visited[v] {
		return false
	}
	c.visited[v] = true
	return true
}

// leave removes v from the descent stack.
func (c *narrowCtx) leave(v ssa.Value) {
	delete(c.visited, v)
}

// packages returns prog.AllPackages() with first-call caching. Safe to
// call when prog is nil; returns a nil slice in that case.
func (c *narrowCtx) packages() []*ssa.Package {
	if c.allPackages != nil || c.prog == nil {
		return c.allPackages
	}
	c.allPackages = c.prog.AllPackages()
	return c.allPackages
}

// narrow is the public entry point. Wraps narrowWalk with a fresh ctx.
func narrow(fn *ssa.Function, v ssa.Value) *narrowResult {
	return narrowWalk(newNarrowCtx(fn), v)
}

// narrowWalk performs the backward SSA traversal.
//
// Cycle detection uses ctx.enter/leave so entries are removed on
// return; DAG reconvergence is not misclassified as a cycle.
func narrowWalk(ctx *narrowCtx, v ssa.Value) *narrowResult {
	if v == nil {
		return newNoPaths(reasonNilInput)
	}
	if !ctx.enter(v) {
		return widened(reasonCycle)
	}
	defer ctx.leave(v)

	switch x := v.(type) {
	case *ssa.Alloc:
		return narrowFromConcreteType(x.Type())
	case *ssa.MakeInterface:
		return narrowWalk(ctx, x.X)
	case *ssa.ChangeType:
		return narrowWalk(ctx, x.X)
	case *ssa.ChangeInterface:
		return narrowWalk(ctx, x.X)
	case *ssa.Convert:
		return narrowWalk(ctx, x.X)
	case *ssa.MultiConvert:
		return narrowWalk(ctx, x.X)
	case *ssa.SliceToArrayPointer:
		return narrowFromConcreteType(x.Type())
	case *ssa.TypeAssert:
		return narrowFromConcreteType(x.AssertedType)
	case *ssa.Phi:
		return narrowFromPhi(ctx, x)
	case *ssa.Extract:
		return narrowFromExtract(ctx, x)
	case *ssa.Call:
		return narrowFromCall(ctx, x)
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
				return narrowPointerLoad(ctx, x)
			case token.ARROW:
				return widened(reasonChannelReceive)
			}
			return widened(reasonUnexpectedUnop)
		}
		return narrowFromConcreteType(x.Type())
	case *ssa.Field:
		if isInterfaceResult(x.Type()) {
			return widened(reasonFieldLoad)
		}
		return narrowFromConcreteType(x.Type())
	case *ssa.FieldAddr:
		if isInterfaceResult(x.Type()) {
			return widened(reasonFieldAddr)
		}
		return narrowFromConcreteType(x.Type())
	case *ssa.Index:
		return narrowFromConcreteType(x.Type())
	case *ssa.IndexAddr:
		return narrowFromConcreteType(x.Type())
	case *ssa.Lookup:
		if isInterfaceResult(x.Type()) {
			return widened(reasonMapLookup)
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
			return widened(reasonNilConstant)
		}
		return narrowFromConcreteType(x.Type())
	case *ssa.Parameter:
		// Concrete-typed parameters narrow from type alone. The historical
		// "parameter" widening was motivated by interface-typed parameters
		// whose runtime value depends on call-site context; concrete-typed
		// parameters carry no such ambiguity. Receiver parameters of methods
		// are the dominant concrete case and matter for narrowFromInvokeDispatch.
		if !isInterfaceResult(x.Type()) {
			return narrowFromConcreteType(x.Type())
		}
		return widened(reasonParameter)
	case *ssa.FreeVar:
		return widened(reasonFreeVar)
	case *ssa.Global:
		return widened(reasonGlobalLoad)
	case *ssa.Function:
		return narrowFromConcreteType(x.Type())
	case *ssa.Builtin:
		return widened(reasonUnresolvableCallee)
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
//	Global                  -> narrowFromGlobalInit   (package-level var)
//	Alloc (local var)       -> narrowFromAllocStores  (recovers stored types)
//	FieldAddr (struct field)-> narrowFromFieldStores
//	IndexAddr (slice/array) -> "slice-deref-load"
//	other                   -> "pointer-load"         (residual bucket)
//
// The Alloc case is the only one that attempts real narrowing; the
// others widen with an informative reason.
func narrowPointerLoad(ctx *narrowCtx, x *ssa.UnOp) *narrowResult {
	switch src := x.X.(type) {
	case *ssa.Global:
		return narrowFromGlobalInit(ctx, src)
	case *ssa.Alloc:
		return narrowFromAllocStores(ctx, src)
	case *ssa.FieldAddr:
		return narrowFromFieldStores(ctx, src)
	case *ssa.IndexAddr:
		return widened(reasonSliceDerefLoad)
	}
	return widened(reasonPointerLoad)
}

// narrowFromFieldStores enumerates every Store instruction in the program
// whose Addr is a FieldAddr referencing the same (struct type, field index)
// as the target, and unions the narrowed types of the stored values.
//
// Matching is per-(struct-type, field-index) rather than per-FieldAddr:
// `p1.Car = v1` and `p2.Car = v2` both count toward narrowing any subsequent
// `_ = _.Car` load on *Pair. This is the correct aliasing assumption for
// fields — per-instance distinction isn't available statically.
//
// Unlike narrowFromGlobalInit (which searches a single Init function), this
// searches every function in every package reachable through ctx.packages().
// Store sites for struct fields can appear anywhere in the program, not just
// package initialization. This is O(functions × instructions) per call; for
// typical programs the traversal completes quickly but the cost scales with
// program size.
//
// Widening reasons produced:
//   - "field-no-prog"        -> FieldAddr's parent function has no program
//   - "field-no-struct-type" -> couldn't extract the struct type from FieldAddr.X
//   - "field-no-stores"      -> no matching Store found anywhere in the program
func narrowFromFieldStores(ctx *narrowCtx, fa *ssa.FieldAddr) *narrowResult {
	fn := fa.Parent()
	if fn == nil || fn.Prog == nil {
		return widened(reasonFieldNoProg)
	}
	targetStruct := fieldAddrStructType(fa)
	if targetStruct == nil {
		return widened(reasonFieldNoStructType)
	}

	results := make([]*narrowResult, 0, 4)
	for _, pkg := range ctx.packages() {
		if pkg == nil {
			continue
		}
		for _, mem := range pkg.Members {
			memFn, ok := mem.(*ssa.Function)
			if !ok {
				continue
			}
			results = appendFieldStoreResults(ctx, results, memFn, targetStruct, fa.Field)
			for _, anon := range memFn.AnonFuncs {
				results = appendFieldStoreResults(ctx, results, anon, targetStruct, fa.Field)
			}
		}
	}

	if len(results) == 0 {
		return widened(reasonFieldNoStores)
	}
	return mergeResults(results)
}

// appendFieldStoreResults scans fn.Blocks for Store instructions whose Addr
// is a FieldAddr matching (targetStruct, fieldIndex) and appends the
// narrowed stored-value result for each.
func appendFieldStoreResults(ctx *narrowCtx, results []*narrowResult, fn *ssa.Function, targetStruct types.Type, fieldIndex int) []*narrowResult {
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			store, ok := instr.(*ssa.Store)
			if !ok {
				continue
			}
			storeFA, ok := store.Addr.(*ssa.FieldAddr)
			if !ok {
				continue
			}
			if storeFA.Field != fieldIndex {
				continue
			}
			if !sameStructType(fieldAddrStructType(storeFA), targetStruct) {
				continue
			}
			results = append(results, narrowWalk(ctx, store.Val))
		}
	}
	return results
}

// fieldAddrStructType unwraps FieldAddr.X's type to the *types.Named struct
// being indexed. FieldAddr.X is always a pointer to a struct; this returns
// the pointee's named type (or the underlying struct if unnamed).
func fieldAddrStructType(fa *ssa.FieldAddr) types.Type {
	t := fa.X.Type()
	if t == nil {
		return nil
	}
	ptr, ok := t.Underlying().(*types.Pointer)
	if !ok {
		return nil
	}
	return ptr.Elem()
}

// sameStructType compares two struct types for matching identity.
// types.Identical handles both named and unnamed (anonymous) struct
// cases correctly: named types compare by declaration identity,
// unnamed types compare by structural shape. No separate Underlying
// fallback is needed.
func sameStructType(a, b types.Type) bool {
	if a == nil || b == nil {
		return false
	}
	return types.Identical(a, b)
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
func narrowFromGlobalInit(ctx *narrowCtx, g *ssa.Global) *narrowResult {
	if g.Pkg == nil {
		return widened(reasonGlobalNoPkg)
	}
	initFn := g.Pkg.Func("init")
	if initFn == nil {
		return widened(reasonGlobalNoInit)
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
			results = append(results, narrowWalk(ctx, store.Val))
		}
	}
	if len(results) == 0 {
		return widened(reasonGlobalNoStores)
	}
	return mergeResults(results)
}

// narrowFromAllocStores finds every Store instruction in alloc.Parent()
// whose Addr is the given alloc, unions the narrowed types of the
// stored values, and returns the merged result. If no stores are found
// (defensive — Go's zero-value init may skip explicit Store ops),
// widens with "alloc-no-stores".
func narrowFromAllocStores(ctx *narrowCtx, alloc *ssa.Alloc) *narrowResult {
	fn := alloc.Parent()
	if fn == nil {
		return widened(reasonAllocOrphan)
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
			results = append(results, narrowWalk(ctx, store.Val))
		}
	}
	if len(results) == 0 {
		return widened(reasonAllocNoStores)
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
		return newNoPaths(reasonNilType)
	}
	if isInterfaceResult(t) {
		return widened(reasonInterfaceResult)
	}
	return newNarrow(types.TypeString(t, nil))
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
func narrowFromPhi(ctx *narrowCtx, phi *ssa.Phi) *narrowResult {
	results := make([]*narrowResult, 0, len(phi.Edges))
	for _, e := range phi.Edges {
		if e == nil {
			continue
		}
		results = append(results, narrowWalk(ctx, e))
	}
	return mergeResults(results)
}

// narrowFromExtract narrows the tuple operand. Extract at a given index cannot
// refine beyond what the tuple as a whole yields, so the merge is conservative.
func narrowFromExtract(ctx *narrowCtx, ex *ssa.Extract) *narrowResult {
	if ex.Tuple == nil {
		return newNoPaths(reasonNilTuple)
	}
	tupleType := ex.Tuple.Type()
	tup, ok := tupleType.(*types.Tuple)
	if ok && ex.Index < tup.Len() {
		elem := tup.At(ex.Index).Type()
		inner := narrowFromConcreteType(elem)
		if inner.Confidence == confNarrow {
			return inner
		}
	}
	return narrowWalk(ctx, ex.Tuple)
}

// narrowFromCall handles static calls (may recurse into callee's return paths
// when the return type is an interface) and interface-method invokes
// (resolved via narrowFromInvokeDispatch).
//
// Concrete-return short-circuit: if the call result type is already concrete,
// the callee's internals tell us nothing narrower — the declared return type
// IS the narrowing. Applies uniformly to static and invoke calls.
func narrowFromCall(ctx *narrowCtx, call *ssa.Call) *narrowResult {
	if !isInterfaceResult(call.Type()) {
		return narrowFromConcreteType(call.Type())
	}
	if call.Call.IsInvoke() {
		return narrowFromInvokeDispatch(ctx, call)
	}
	callee := staticCallee(call)
	if callee == nil {
		return widened(reasonUnresolvableCallee)
	}
	return narrowFromCalleeReturns(ctx, callee)
}

// narrowFromInvokeDispatch resolves an interface-method call by enumerating
// every concrete type in the program that implements the interface,
// resolving the specific method on each, and unioning the narrowed return
// types across all implementations.
//
// The interface type is extracted from the method's receiver signature.
// Both value-receiver and pointer-receiver forms of each concrete type are
// checked; a type can satisfy an interface through either, and Go's method
// set rules (pointer receiver methods are in the method set of the pointer
// type, not the value type) mean we must check both to avoid missing
// implementations.
//
// Synthetic functions (wrappers, bound methods) are skipped — they forward
// to the underlying implementation, which we reach directly via the non-
// synthetic candidate. Including both would double-count.
//
// Widening reasons — distinct tags so debuggers can separate ill-formed
// invokes (SSA invariant violations) from legitimate "empty interface"
// situations:
//   - "dispatch-no-prog"            -> call has no parent function or program (defensive)
//   - "dispatch-nil-method"         -> Call.Method is nil (defensive)
//   - "dispatch-bad-signature"      -> method's type is not *types.Signature or lacks a receiver
//   - "dispatch-non-interface-recv" -> receiver's underlying type is not an interface
//   - "dispatch-all-synthetic"      -> implementations exist but every candidate was synthetic
//   - "dispatch-no-implementors"    -> no concrete type in the program implements the interface
func narrowFromInvokeDispatch(ctx *narrowCtx, call *ssa.Call) *narrowResult {
	fn := call.Parent()
	if fn == nil || fn.Prog == nil {
		return widened(reasonDispatchNoProg)
	}
	method := call.Call.Method
	if method == nil {
		return widened(reasonDispatchNilMethod)
	}
	sig, ok := method.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return widened(reasonDispatchBadSignature)
	}
	iface, ok := sig.Recv().Type().Underlying().(*types.Interface)
	if !ok {
		return widened(reasonDispatchNonInterfaceRecv)
	}

	// Track the two failure modes separately: sawImplementor distinguishes
	// "no candidate implements" from "candidates implement but all were
	// synthetic". Debuggers reach very different conclusions from each.
	sawImplementor := false
	results := make([]*narrowResult, 0, 8)
	for _, pkg := range ctx.packages() {
		if pkg == nil {
			continue
		}
		for _, mem := range pkg.Members {
			typ, ok := mem.(*ssa.Type)
			if !ok {
				continue
			}
			namedType := typ.Type()
			for _, candidate := range []types.Type{namedType, types.NewPointer(namedType)} {
				if !types.Implements(candidate, iface) {
					continue
				}
				mset := fn.Prog.MethodSets.MethodSet(candidate)
				sel := mset.Lookup(method.Pkg(), method.Name())
				if sel == nil {
					continue
				}
				impl := fn.Prog.MethodValue(sel)
				if impl == nil {
					continue
				}
				sawImplementor = true
				if impl.Synthetic != "" {
					continue
				}
				results = append(results, narrowFromCalleeReturns(ctx, impl))
			}
		}
	}

	if len(results) == 0 {
		if sawImplementor {
			return widened(reasonDispatchAllSynthetic)
		}
		return widened(reasonDispatchNoImplementors)
	}
	return mergeResults(results)
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
func narrowFromCalleeReturns(ctx *narrowCtx, callee *ssa.Function) *narrowResult {
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
			results = append(results, narrowWalk(ctx, ret.Results[0]))
		}
	}
	if len(results) == 0 {
		// Callee has no Return instructions (panic-only, infinite loop,
		// etc.). Distinct from "I couldn't find paths" or "input was nil".
		return newNoPaths(reasonCalleeNoReturns)
	}
	return mergeResults(results)
}

// mergeResults combines multiple narrow results into one, unioning types and
// reasons and taking the weakest confidence.
func mergeResults(rs []*narrowResult) *narrowResult {
	if len(rs) == 0 {
		return newNoPaths("")
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
		switch r.Confidence {
		case confWidened:
			anyWidened = true
			allNoPaths = false
		case confNoPaths:
			// unchanged
		default:
			allNoPaths = false
		}
	}
	conf := confNarrow
	switch {
	case anyWidened:
		conf = confWidened
	case allNoPaths:
		conf = confNoPaths
	}
	return &narrowResult{
		Types:      keysSorted(typeSet),
		Confidence: conf,
		Reasons:    keysSorted(reasonSet),
	}
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
