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

package goastcfg

import (
	"go/token"

	"golang.org/x/tools/go/ssa"

	"github.com/aalpar/wile/pkg/values"

	"github.com/aalpar/wile-goast/goast"
)

type cfgMapper struct {
	fset      *token.FileSet
	positions bool
}

// mapFunction maps all basic blocks of an SSA function to cfg-block s-expressions.
// fn.Recover, if present, is appended last and tagged with (recover . #t).
func (p *cfgMapper) mapFunction(fn *ssa.Function) values.Value {
	closures := closureBlocks(fn)
	blocks := make([]values.Value, len(fn.Blocks))
	for i, b := range fn.Blocks {
		blocks[i] = p.mapBlock(b, false, closures[b])
	}
	if fn.Recover != nil {
		blocks = append(blocks, p.mapBlock(fn.Recover, true, closures[fn.Recover]))
	}
	return goast.ValueList(blocks)
}

// closureBlocks attributes every func literal of fn to one of its blocks, so
// that the union over the blocks is exactly fn.AnonFuncs, once each, in go/ssa's
// own $N order. Returns nil when fn has no literals.
//
// The enumeration must start from fn.AnonFuncs rather than from the instruction
// stream, because go/ssa emits a MakeClosure only for a literal that captures
// something: one that captures nothing is emitted as no instruction at all and
// merely appears as an *ssa.Function operand wherever its value is used --
// `Two`'s two literals are both operands of a single Return. Enumerating
// MakeClosure would silently drop every non-capturing literal, which is the
// same silent omission this field exists to end.
//
// Starting from AnonFuncs is also what keeps a name that cannot round-trip out,
// with no Synthetic string test: a bound-method wrapper ("M$bound", minted for
// `return t.M`) has no parent, is in neither ssaPkg.Members nor any method set,
// and findFunction cannot resolve it -- and it is in no AnonFuncs list, so it
// can never be listed. Conversely a Go 1.23 range-over-func body is Synthetic
// yet is an ordinary AnonFuncs entry carrying the user's loop body, resolves as
// "parent$N", and is listed.
//
// The name emitted is String(), the qualified form, not Name(). Name() is
// parent$N over the *short* name, so two same-named methods on different types
// in one package both yield "Same$1" -- and findFunction's short-name predicate
// (fn.Name() == name && fn.Pkg == ssaPkg) is satisfied by both, resolving by Go
// map order over ssaPkg.Members. Measured on x/tools v0.47.0:
// wile/pkg/values' "schemeStringWithVisited$1" is 5 different literals and 12
// process runs of go-cfg returned the 4-block one 3 times and a 1-block one 9
// times; "(*.../values.Pair).schemeStringWithVisited$1" returned the same
// literal 400/400. Since the field exists precisely so a name can be fed back
// in, an ambiguous name would defeat it silently -- the same failure the
// AnonFuncs restriction above rules out, reached by collision rather than by
// synthesis.
func closureBlocks(fn *ssa.Function) map[*ssa.BasicBlock][]values.Value {
	if len(fn.AnonFuncs) == 0 {
		return nil
	}
	q := make(map[*ssa.BasicBlock][]values.Value, len(fn.AnonFuncs))
	for _, anon := range fn.AnonFuncs {
		b := closureBlock(fn, anon)
		if b == nil {
			continue
		}
		q[b] = append(q[b], goast.Str(anon.String()))
	}
	return q
}

// closureBlock returns the block of fn that anon is attributed to: the block of
// its first referring instruction, or fn's entry block when it has none.
//
// Function.Referrers is non-nil exactly when Parent() != nil, i.e. exactly for
// func literals, and its first entry is the MakeClosure whenever go/ssa emitted
// one -- so for a capturing literal this is the construction site exactly. For
// a non-capturing one there is no construction instruction to point at, so the
// first use stands in; that is the same block in ordinary code, but a literal
// written before a loop and called inside it lands on the loop body. A literal
// referred to only from a nested literal, or not at all (`_ = func() {}`),
// falls to the entry block.
func closureBlock(fn, anon *ssa.Function) *ssa.BasicBlock {
	refs := anon.Referrers()
	if refs != nil {
		for _, instr := range *refs {
			b := instr.Block()
			if b != nil && b.Parent() == fn {
				return b
			}
		}
	}
	if len(fn.Blocks) == 0 {
		return nil
	}
	return fn.Blocks[0]
}

// mapBlock maps a single SSA basic block to a cfg-block s-expression.
// isRecover must be true when b is fn.Recover; it adds (recover . #t) to the
// field list to distinguish it from the entry block (both have idom=#f).
// closures, when non-empty, becomes the block's (closures "Name$1" ...) field.
func (p *cfgMapper) mapBlock(b *ssa.BasicBlock, isRecover bool, closures []values.Value) values.Value {
	preds := make([]values.Value, len(b.Preds))
	for i, pred := range b.Preds {
		preds[i] = values.NewInteger(int64(pred.Index))
	}
	succs := make([]values.Value, len(b.Succs))
	for i, succ := range b.Succs {
		succs[i] = values.NewInteger(int64(succ.Index))
	}

	var idom values.Value
	if idomBlock := b.Idom(); idomBlock != nil {
		idom = values.NewInteger(int64(idomBlock.Index))
	} else {
		idom = values.FalseValue
	}

	fields := []values.Value{
		goast.Field("index", values.NewInteger(int64(b.Index))),
		goast.Field("preds", goast.ValueList(preds)),
		goast.Field("succs", goast.ValueList(succs)),
		goast.Field("idom", idom),
	}
	if isRecover {
		fields = append(fields, goast.Field("recover", values.TrueValue))
	}
	if b.Comment != "" {
		fields = append(fields, goast.Field("comment", goast.Str(b.Comment)))
	}
	if len(closures) > 0 {
		fields = append(fields, goast.Field("closures", goast.ValueList(closures)))
	}
	if p.positions && b.Instrs != nil && len(b.Instrs) > 0 {
		pos := b.Instrs[0].Pos()
		if pos.IsValid() {
			fields = append(fields, goast.Field("pos", goast.Str(p.fset.Position(pos).String())))
		}
	}
	return goast.Node("cfg-block", fields...)
}
