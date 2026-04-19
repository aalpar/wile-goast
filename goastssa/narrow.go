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
// Stub implementation. Task 4 ships the primitive with a no-paths result;
// subsequent tasks (5-12) add Alloc, Call, Phi, TypeAssert, Extract, and
// widening paths with reason tags.
//
// See plans/2026-04-19-axis-b-analyzer-impl-design.md §6.

package goastssa

import (
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

// narrowWalk performs the backward SSA traversal. Stub: returns no-paths
// for every value; real cases arrive in later tasks.
func narrowWalk(fn *ssa.Function, v ssa.Value, visited map[ssa.Value]bool) *narrowResult {
	if visited[v] {
		return &narrowResult{Confidence: "widened", Reasons: []string{"cycle"}}
	}
	visited[v] = true
	_ = fn
	return &narrowResult{Confidence: "no-paths"}
}
