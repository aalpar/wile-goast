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

package goastcg

import (
	"go/token"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/ssa"
)

// preciseCallGraph refines a CHA call graph by statically resolving the subset
// of indirect calls whose callee is decidable: a constant index into a literal
// []func() (or array of func). CHA soundly over-approximates every indirect
// func() call to ALL address-taken functions of matching signature; for the
// decidable subset the precise callee is recoverable from SSA, so we drop the
// spurious edges. Calls we cannot resolve keep their CHA edges, so the result
// is never less sound than CHA — only more precise where SSA permits.
func preciseCallGraph(prog *ssa.Program) *callgraph.Graph {
	cg := cha.CallGraph(prog)

	var toDelete []*callgraph.Edge
	for fn, node := range cg.Nodes {
		if fn == nil {
			continue
		}
		for _, e := range node.Out {
			precise := resolvePreciseCallee(e.Site)
			if precise == nil {
				continue // unresolvable → keep CHA's (sound) edge
			}
			if e.Callee.Func != precise {
				toDelete = append(toDelete, e)
			}
		}
	}
	for _, e := range toDelete {
		deleteEdge(e)
	}
	return cg
}

// resolvePreciseCallee returns the single function a call site invokes when
// that is statically decidable from SSA, or nil when it is not (in which case
// the caller must fall back to the sound over-approximation). It currently
// decides one pattern: a load of a constant index into a literal slice/array
// of functions — `t := []func(){...}; t[k]()` with k a constant.
func resolvePreciseCallee(site ssa.CallInstruction) *ssa.Function {
	if site == nil {
		return nil
	}
	common := site.Common()
	if common == nil || common.IsInvoke() {
		return nil // interface dispatch is a separate problem
	}
	if common.StaticCallee() != nil {
		return nil // already a direct call; CHA is exact, nothing to prune
	}

	// Called value must be a load (*addr) of a constant-index address.
	load, ok := common.Value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	ia, ok := load.X.(*ssa.IndexAddr)
	if !ok {
		return nil
	}
	k, ok := constInt(ia.Index)
	if !ok {
		return nil
	}

	// Resolve the backing array allocation (possibly via a slice view).
	base := ia.X
	if sl, ok := base.(*ssa.Slice); ok {
		base = sl.X
	}
	alloc, ok := base.(*ssa.Alloc)
	if !ok {
		return nil
	}

	return storedFuncAt(alloc, k)
}

// storedFuncAt returns the unique *ssa.Function stored at constant index k of
// the array allocation, or nil if the contents are not statically pinned: the
// allocation escapes, the index is written dynamically, or index k receives a
// value that is not a single function literal.
func storedFuncAt(alloc *ssa.Alloc, k int64) *ssa.Function {
	var found *ssa.Function
	refs := alloc.Referrers()
	if refs == nil {
		return nil
	}
	for _, r := range *refs {
		switch v := r.(type) {
		case *ssa.IndexAddr:
			j, ok := constInt(v.Index)
			if !ok {
				return nil // dynamic index could overwrite k → unsound to pin
			}
			if j != k {
				continue
			}
			fn := funcStoredVia(v)
			if fn == nil {
				return nil // index k holds a non-literal/ambiguous value
			}
			if found != nil && found != fn {
				return nil // two different functions at index k
			}
			found = fn
		case *ssa.Slice:
			// A slice view of the array. Reads through it are fine; writes or
			// escape through it are not yet accounted for, so be conservative.
			if escapesVia(v) {
				return nil
			}
		default:
			return nil // any other use (Call, Store, Return, …) → may escape
		}
	}
	return found
}

// funcStoredVia returns the function stored through an IndexAddr, if exactly
// one Store targets it and its value is a function literal.
func funcStoredVia(ia *ssa.IndexAddr) *ssa.Function {
	refs := ia.Referrers()
	if refs == nil {
		return nil
	}
	var found *ssa.Function
	for _, r := range *refs {
		st, ok := r.(*ssa.Store)
		if !ok || st.Addr != ia {
			continue
		}
		fn, ok := st.Val.(*ssa.Function)
		if !ok {
			return nil
		}
		if found != nil && found != fn {
			return nil
		}
		found = fn
	}
	return found
}

// escapesVia reports whether a slice value is used in a way that could mutate
// or alias the backing array beyond constant-index reads.
func escapesVia(sl *ssa.Slice) bool {
	refs := sl.Referrers()
	if refs == nil {
		return false
	}
	for _, r := range *refs {
		switch v := r.(type) {
		case *ssa.IndexAddr:
			if _, ok := constInt(v.Index); !ok {
				return true // dynamic index through the slice
			}
		default:
			return true // passed to a call, stored, returned, ranged, …
		}
	}
	return false
}

// constInt extracts a constant integer value from an SSA value.
func constInt(v ssa.Value) (int64, bool) {
	c, ok := v.(*ssa.Const)
	if !ok || c.Value == nil {
		return 0, false
	}
	return c.Int64(), true
}

// deleteEdge removes an edge from both its caller's Out and callee's In lists.
func deleteEdge(e *callgraph.Edge) {
	e.Caller.Out = removeEdge(e.Caller.Out, e)
	e.Callee.In = removeEdge(e.Callee.In, e)
}

func removeEdge(edges []*callgraph.Edge, target *callgraph.Edge) []*callgraph.Edge {
	out := edges[:0]
	for _, e := range edges {
		if e != target {
			out = append(out, e)
		}
	}
	return out
}
