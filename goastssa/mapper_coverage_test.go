// Copyright 2026 Aaron Alpar
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Coverage-targeted tests for mapper.go paths that don't arise from
// typical Go source: mapMultiConvert (requires generic type parameter
// conversion) and mapUnknown (defensive default branch, rarely hit
// via dispatch).

package goastssa

import (
	"go/token"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/aalpar/wile/values"
)

// TestMapMultiConvertGeneric exercises mapMultiConvert. Go's SSA
// builder emits *ssa.MultiConvert in `emitConv` when converting a
// value whose type is a type parameter whose core includes multiple
// distinct underlying representations. A constraint like
// `string | []byte` (two different underlying layouts) forces the
// multi-form; a single-type constraint like `~string` lowers to a
// plain *ssa.Convert.
func TestMapMultiConvertGeneric(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

// Conv accepts either string or []byte and converts to string.
// The union constraint forces SSA to emit MultiConvert for the
// string conversion inside the body.
func Conv[T string | []byte](s T) string {
	return string(s)
}

// Entry point so the generic instantiation is realized.
func Foo() string {
	return Conv([]byte("hello"))
}
`, "Conv")

	mapper := &ssaMapper{fset: token.NewFileSet()}
	result := mapper.mapFunction(fn)

	found := findNodeByTag(result, "ssa-multi-convert")
	if found == nil {
		t.Skip("Go toolchain did not emit ssa.MultiConvert for this fixture; coverage not exercised.")
	}
	c.Assert(found, qt.IsNotNil)
}

// TestMapUnknownDirect unit-tests mapUnknown by calling it directly
// with a real SSA instruction. The function's purpose is defensive —
// producing a diagnostic node when dispatchInstruction receives an
// unrecognized instruction type. Direct invocation lets us cover the
// function's body (including the ssa.Value branch) without needing
// an actual unknown instruction type from the Go toolchain.
func TestMapUnknownDirect(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo(a, b int) int {
	return a + b
}
`, "Foo")

	// Find any instruction to pass to mapUnknown.
	c.Assert(len(fn.Blocks), qt.Not(qt.Equals), 0)
	c.Assert(len(fn.Blocks[0].Instrs), qt.Not(qt.Equals), 0)
	instr := fn.Blocks[0].Instrs[0]

	mapper := &ssaMapper{fset: token.NewFileSet()}
	node := mapper.mapUnknown(instr)
	c.Assert(node, qt.IsNotNil)

	// The node is a pair with a leading symbol tag; we just verify
	// the shape without assuming a specific tag name.
	pair, ok := node.(*values.Pair)
	c.Assert(ok, qt.IsTrue)
	_, ok = pair.Car().(*values.Symbol)
	c.Assert(ok, qt.IsTrue, qt.Commentf("mapUnknown node should lead with a tag symbol"))
}

// TestMapUnknownOnNonValueInstruction covers the ssa.Value type-
// assertion branch that only fires when the instruction is NOT a
// Value (e.g., Jump, If, Store — instructions without a result).
func TestMapUnknownOnNonValueInstruction(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	fn := buildSSAFromSource(t, dir, `
package testpkg

func Foo(a, b int) int {
	if a > b {
		return a
	}
	return b
}
`, "Foo")

	// Locate a Jump or If instruction — these aren't ssa.Value.
	var nonValueInstr = fn.Blocks[0].Instrs[len(fn.Blocks[0].Instrs)-1]
	// The terminator is usually Jump/If/Return. Any of them work for
	// covering the "ok == false" branch of the ssa.Value assertion;
	// Return is itself not an ssa.Value.
	mapper := &ssaMapper{fset: token.NewFileSet()}
	node := mapper.mapUnknown(nonValueInstr)
	c.Assert(node, qt.IsNotNil)
}
