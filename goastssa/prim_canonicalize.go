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

package goastssa

import (
	"strconv"

	"github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

// ssaFuncData is the intermediate representation of an SSA function s-expression.
type ssaFuncData struct {
	name      string
	signature string
	pkg       string
	params    []ssaParamData
	freeVars  values.Value
	blocks    []ssaBlockData
}

// ssaParamData holds a single SSA parameter.
type ssaParamData struct {
	name     string
	typStr   string
	original values.Value
}

// ssaBlockData holds a single SSA basic block.
type ssaBlockData struct {
	index   int64
	idom    int64
	preds   []int64
	succs   []int64
	comment string
	instrs  []values.Value
}

// PrimGoSSACanonicalize canonicalizes an SSA function s-expression:
// dominator-order blocks, alpha-renamed registers.
func PrimGoSSACanonicalize(mc *machine.MachineContext) error {
	arg := mc.Arg(0)
	node, ok := arg.(*values.Pair)
	if !ok {
		return werr.WrapForeignErrorf(errSSACanonicalizeError,
			"go-ssa-canonicalize: expected pair, got %T", arg)
	}

	tag, ok := node.Car().(*values.Symbol)
	if !ok || tag.Key != "ssa-func" {
		return werr.WrapForeignErrorf(errSSACanonicalizeError,
			"go-ssa-canonicalize: expected ssa-func node, got %v", node.Car())
	}

	fd, err := parseSSAFunc(node.Cdr())
	if err != nil {
		return err
	}

	if err := canonicalizeBlockOrder(&fd); err != nil {
		return err
	}
	renameRegisters(&fd)

	mc.SetValue(rebuildSSAFunc(&fd))
	return nil
}

// parseSSAFunc extracts fields from an ssa-func s-expression into ssaFuncData.
func parseSSAFunc(fields values.Value) (ssaFuncData, error) {
	var fd ssaFuncData

	nameVal, err := goast.RequireField(fields, "ssa-func", "name")
	if err != nil {
		return fd, err
	}
	fd.name, err = goast.RequireString(nameVal, "ssa-func", "name")
	if err != nil {
		return fd, err
	}

	sigVal, err := goast.RequireField(fields, "ssa-func", "signature")
	if err != nil {
		return fd, err
	}
	fd.signature, err = goast.RequireString(sigVal, "ssa-func", "signature")
	if err != nil {
		return fd, err
	}

	pkgVal, found := goast.GetField(fields, "pkg")
	if found {
		fd.pkg, err = goast.RequireString(pkgVal, "ssa-func", "pkg")
		if err != nil {
			return fd, err
		}
	}

	paramsVal, err := goast.RequireField(fields, "ssa-func", "params")
	if err != nil {
		return fd, err
	}
	fd.params, err = parseSSAParams(paramsVal)
	if err != nil {
		return fd, err
	}

	freeVarsVal, err := goast.RequireField(fields, "ssa-func", "free-vars")
	if err != nil {
		return fd, err
	}
	fd.freeVars = freeVarsVal

	blocksVal, err := goast.RequireField(fields, "ssa-func", "blocks")
	if err != nil {
		return fd, err
	}
	fd.blocks, err = parseSSABlocks(blocksVal)
	if err != nil {
		return fd, err
	}

	return fd, nil
}

// parseSSAParams extracts parameter data from a list of ssa-param nodes.
func parseSSAParams(paramsList values.Value) ([]ssaParamData, error) {
	var params []ssaParamData
	cur := paramsList
	for !values.IsEmptyList(cur) {
		pair, ok := cur.(*values.Pair)
		if !ok {
			return nil, werr.WrapForeignErrorf(errSSACanonicalizeError,
				"go-ssa-canonicalize: malformed params list")
		}
		paramNode, ok := pair.Car().(*values.Pair)
		if !ok {
			return nil, werr.WrapForeignErrorf(errSSACanonicalizeError,
				"go-ssa-canonicalize: expected pair in params list")
		}

		nameVal, err := goast.RequireField(paramNode.Cdr(), "ssa-param", "name")
		if err != nil {
			return nil, err
		}
		name, err := goast.RequireString(nameVal, "ssa-param", "name")
		if err != nil {
			return nil, err
		}

		typVal, err := goast.RequireField(paramNode.Cdr(), "ssa-param", "type")
		if err != nil {
			return nil, err
		}
		typStr, err := goast.RequireString(typVal, "ssa-param", "type")
		if err != nil {
			return nil, err
		}

		params = append(params, ssaParamData{
			name:     name,
			typStr:   typStr,
			original: pair.Car(),
		})
		cur = pair.Cdr()
	}
	return params, nil
}

// parseSSABlocks extracts block data from a list of ssa-block nodes.
func parseSSABlocks(blocksList values.Value) ([]ssaBlockData, error) {
	var blocks []ssaBlockData
	cur := blocksList
	for !values.IsEmptyList(cur) {
		pair, ok := cur.(*values.Pair)
		if !ok {
			return nil, werr.WrapForeignErrorf(errSSACanonicalizeError,
				"go-ssa-canonicalize: malformed blocks list")
		}
		blockNode, ok := pair.Car().(*values.Pair)
		if !ok {
			return nil, werr.WrapForeignErrorf(errSSACanonicalizeError,
				"go-ssa-canonicalize: expected pair in blocks list")
		}

		bd, err := parseSSABlock(blockNode.Cdr())
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, bd)
		cur = pair.Cdr()
	}
	return blocks, nil
}

// parseSSABlock extracts fields from an ssa-block s-expression.
func parseSSABlock(fields values.Value) (ssaBlockData, error) {
	var bd ssaBlockData

	idxVal, err := goast.RequireField(fields, "ssa-block", "index")
	if err != nil {
		return bd, err
	}
	idx, ok := idxVal.(*values.Integer)
	if !ok {
		return bd, werr.WrapForeignErrorf(errSSACanonicalizeError,
			"go-ssa-canonicalize: block index expected integer")
	}
	bd.index = idx.Value

	// idom is optional (entry block has none).
	idomVal, found := goast.GetField(fields, "idom")
	if found {
		idom, ok := idomVal.(*values.Integer)
		if !ok {
			return bd, werr.WrapForeignErrorf(errSSACanonicalizeError,
				"go-ssa-canonicalize: block idom expected integer")
		}
		bd.idom = idom.Value
	} else {
		bd.idom = -1
	}

	bd.preds, err = parseIntList(fields, "preds")
	if err != nil {
		return bd, err
	}
	bd.succs, err = parseIntList(fields, "succs")
	if err != nil {
		return bd, err
	}

	// comment is optional.
	commentVal, found := goast.GetField(fields, "comment")
	if found {
		s, ok := commentVal.(*values.String)
		if !ok {
			return bd, werr.WrapForeignErrorf(errSSACanonicalizeError,
				"go-ssa-canonicalize: block comment expected string")
		}
		bd.comment = s.Value
	}

	instrsVal, err := goast.RequireField(fields, "ssa-block", "instrs")
	if err != nil {
		return bd, err
	}
	bd.instrs = collectList(instrsVal)

	return bd, nil
}

// parseIntList extracts a list of integers from a field.
func parseIntList(fields values.Value, key string) ([]int64, error) {
	listVal, err := goast.RequireField(fields, "ssa-block", key)
	if err != nil {
		return nil, err
	}
	var result []int64
	cur := listVal
	for !values.IsEmptyList(cur) {
		pair, ok := cur.(*values.Pair)
		if !ok {
			return nil, werr.WrapForeignErrorf(errSSACanonicalizeError,
				"go-ssa-canonicalize: malformed %s list", key)
		}
		n, ok := pair.Car().(*values.Integer)
		if !ok {
			return nil, werr.WrapForeignErrorf(errSSACanonicalizeError,
				"go-ssa-canonicalize: %s expected integer element", key)
		}
		result = append(result, n.Value)
		cur = pair.Cdr()
	}
	return result, nil
}

// collectList gathers all elements from a Scheme list into a slice.
func collectList(list values.Value) []values.Value {
	var result []values.Value
	cur := list
	for !values.IsEmptyList(cur) {
		pair, ok := cur.(*values.Pair)
		if !ok {
			break
		}
		result = append(result, pair.Car())
		cur = pair.Cdr()
	}
	return result
}

// canonicalizeBlockOrder reorders blocks by pre-order DFS of the dominator tree
// and reindexes all cross-references (preds, succs, idom, phi edges, jump/if targets).
func canonicalizeBlockOrder(fd *ssaFuncData) error {
	if len(fd.blocks) <= 1 {
		return nil
	}

	// Build dominator tree: children[parentIdx] = [...childIndices]
	children := make(map[int64][]int64)
	entryIdx := int64(-1)
	for _, b := range fd.blocks {
		if b.idom == -1 {
			entryIdx = b.index
		} else {
			children[b.idom] = append(children[b.idom], b.index)
		}
	}
	if entryIdx == -1 {
		return werr.WrapForeignErrorf(errSSACanonicalizeError,
			"go-ssa-canonicalize: no entry block (idom == -1) found")
	}

	// Pre-order DFS from entry.
	var order []int64
	var dfs func(int64)
	dfs = func(idx int64) {
		order = append(order, idx)
		for _, child := range children[idx] {
			dfs(child)
		}
	}
	dfs(entryIdx)

	if len(order) != len(fd.blocks) {
		return werr.WrapForeignErrorf(errSSACanonicalizeError,
			"go-ssa-canonicalize: dominator tree covers %d of %d blocks",
			len(order), len(fd.blocks))
	}

	// Build old→new index mapping.
	oldToNew := make(map[int64]int64, len(order))
	for newIdx, oldIdx := range order {
		oldToNew[oldIdx] = int64(newIdx)
	}

	// Index blocks by old index for lookup.
	blockByOldIdx := make(map[int64]*ssaBlockData, len(fd.blocks))
	for i := range fd.blocks {
		blockByOldIdx[fd.blocks[i].index] = &fd.blocks[i]
	}

	// Reorder and reindex.
	newBlocks := make([]ssaBlockData, len(order))
	for i, oldIdx := range order {
		b := *blockByOldIdx[oldIdx]
		b.index = int64(i)
		if b.idom != -1 {
			b.idom = oldToNew[b.idom]
		}
		b.preds = remapIndices(b.preds, oldToNew)
		b.succs = remapIndices(b.succs, oldToNew)
		b.instrs = reindexInstrs(b.instrs, oldToNew)
		newBlocks[i] = b
	}
	fd.blocks = newBlocks
	return nil
}

// remapIndices applies oldToNew mapping to a slice of block indices.
func remapIndices(indices []int64, oldToNew map[int64]int64) []int64 {
	if len(indices) == 0 {
		return indices
	}
	result := make([]int64, len(indices))
	for i, idx := range indices {
		if newIdx, ok := oldToNew[idx]; ok {
			result[i] = newIdx
		} else {
			result[i] = idx
		}
	}
	return result
}

// reindexInstrs updates block index references in instructions.
func reindexInstrs(instrs []values.Value, oldToNew map[int64]int64) []values.Value {
	result := make([]values.Value, len(instrs))
	for i, instr := range instrs {
		result[i] = reindexInstr(instr, oldToNew)
	}
	return result
}

// reindexInstr dispatches on instruction tag to remap block indices.
func reindexInstr(instr values.Value, oldToNew map[int64]int64) values.Value {
	node, ok := instr.(*values.Pair)
	if !ok {
		return instr
	}
	tag, ok := node.Car().(*values.Symbol)
	if !ok {
		return instr
	}
	switch tag.Key {
	case "ssa-phi":
		return reindexPhi(node, oldToNew)
	case "ssa-if":
		return reindexIf(node, oldToNew)
	case "ssa-jump":
		return reindexJump(node, oldToNew)
	default:
		return instr
	}
}

// reindexPhi remaps block indices in phi edge pairs (block-index . register-name).
func reindexPhi(node *values.Pair, oldToNew map[int64]int64) values.Value {
	edgesVal, found := goast.GetField(node.Cdr(), "edges")
	if !found {
		return node
	}
	var newEdges []values.Value
	cur := edgesVal
	for !values.IsEmptyList(cur) {
		pair, ok := cur.(*values.Pair)
		if !ok {
			break
		}
		edge, ok := pair.Car().(*values.Pair)
		if ok {
			blockIdx, ok := edge.Car().(*values.Integer)
			if ok {
				if newIdx, mapped := oldToNew[blockIdx.Value]; mapped {
					newEdges = append(newEdges, values.NewCons(
						values.NewInteger(newIdx), edge.Cdr()))
				} else {
					newEdges = append(newEdges, pair.Car())
				}
			} else {
				newEdges = append(newEdges, pair.Car())
			}
		} else {
			newEdges = append(newEdges, pair.Car())
		}
		cur = pair.Cdr()
	}
	return replaceField(node, "edges", goast.ValueList(newEdges))
}

// reindexIf remaps then/else block indices.
func reindexIf(node *values.Pair, oldToNew map[int64]int64) values.Value {
	result := remapIntField(node, "then", oldToNew)
	return remapIntField(result.(*values.Pair), "else", oldToNew)
}

// reindexJump remaps the target block index.
func reindexJump(node *values.Pair, oldToNew map[int64]int64) values.Value {
	return remapIntField(node, "target", oldToNew)
}

// remapIntField replaces an integer field value using oldToNew mapping.
func remapIntField(node *values.Pair, key string, oldToNew map[int64]int64) values.Value {
	val, found := goast.GetField(node.Cdr(), key)
	if !found {
		return node
	}
	intVal, ok := val.(*values.Integer)
	if !ok {
		return node
	}
	if newIdx, mapped := oldToNew[intVal.Value]; mapped {
		return replaceField(node, key, values.NewInteger(newIdx))
	}
	return node
}

// renameRegisters assigns canonical register names in first-use order:
// free variables → fv0, fv1, ...; params → p0, p1, ...; instructions → r0, r1, ...
func renameRegisters(fd *ssaFuncData) {
	nameMap := make(map[string]string)

	// Rename free variables: fv0, fv1, ...
	fd.freeVars = renameFreeVars(fd.freeVars, nameMap)

	// Rename params: p0, p1, ...
	for i := range fd.params {
		canonical := "p" + strconv.FormatInt(int64(i), 10)
		nameMap[fd.params[i].name] = canonical
		fd.params[i].name = canonical
	}

	// First pass: collect instruction names, assign r0, r1, ...
	rIdx := 0
	for _, b := range fd.blocks {
		for _, instr := range b.instrs {
			name := instrName(instr)
			if name != "" {
				if _, exists := nameMap[name]; !exists {
					nameMap[name] = "r" + strconv.FormatInt(int64(rIdx), 10)
					rIdx++
				}
			}
		}
	}

	// Second pass: apply renaming to all instructions.
	for i := range fd.blocks {
		for j, instr := range fd.blocks[i].instrs {
			fd.blocks[i].instrs[j] = renameInstrStrings(instr, nameMap)
		}
	}
}

// renameFreeVars renames free variable nodes (fv0, fv1, ...) and updates nameMap.
func renameFreeVars(freeVars values.Value, nameMap map[string]string) values.Value {
	var result []values.Value
	idx := 0
	cur := freeVars
	for !values.IsEmptyList(cur) {
		pair, ok := cur.(*values.Pair)
		if !ok {
			break
		}
		node, ok := pair.Car().(*values.Pair)
		if ok {
			oldName := ""
			nameVal, found := goast.GetField(node.Cdr(), "name")
			if found {
				if s, ok := nameVal.(*values.String); ok {
					oldName = s.Value
				}
			}
			canonical := "fv" + strconv.FormatInt(int64(idx), 10)
			if oldName != "" {
				nameMap[oldName] = canonical
			}
			result = append(result, replaceField(node, "name", goast.Str(canonical)))
			idx++
		} else {
			result = append(result, pair.Car())
		}
		cur = pair.Cdr()
	}
	return goast.ValueList(result)
}

// instrName extracts the name field from an instruction s-expression, if present.
func instrName(instr values.Value) string {
	node, ok := instr.(*values.Pair)
	if !ok {
		return ""
	}
	nameVal, found := goast.GetField(node.Cdr(), "name")
	if !found {
		return ""
	}
	s, ok := nameVal.(*values.String)
	if !ok {
		return ""
	}
	return s.Value
}

// renameInstrStrings recursively replaces all string values using nameMap.
func renameInstrStrings(v values.Value, nameMap map[string]string) values.Value {
	switch val := v.(type) {
	case *values.String:
		if replacement, ok := nameMap[val.Value]; ok {
			return goast.Str(replacement)
		}
		return v
	case *values.Pair:
		newCar := renameInstrStrings(val.Car(), nameMap)
		newCdr := renameInstrStrings(val.Cdr(), nameMap)
		if newCar == val.Car() && newCdr == val.Cdr() {
			return v
		}
		return values.NewCons(newCar, newCdr)
	default:
		return v
	}
}

// rebuildSSAFunc converts ssaFuncData back to an s-expression.
func rebuildSSAFunc(fd *ssaFuncData) values.Value {
	params := make([]values.Value, len(fd.params))
	for i, p := range fd.params {
		params[i] = goast.Node("ssa-param",
			goast.Field("name", goast.Str(p.name)),
			goast.Field("type", goast.Str(p.typStr)),
		)
	}

	blocks := make([]values.Value, len(fd.blocks))
	for i, b := range fd.blocks {
		blocks[i] = rebuildSSABlock(&b)
	}

	fields := []values.Value{
		goast.Field("name", goast.Str(fd.name)),
		goast.Field("signature", goast.Str(fd.signature)),
		goast.Field("params", goast.ValueList(params)),
		goast.Field("free-vars", fd.freeVars),
		goast.Field("blocks", goast.ValueList(blocks)),
	}
	if fd.pkg != "" {
		fields = append(fields, goast.Field("pkg", goast.Str(fd.pkg)))
	}
	return goast.Node("ssa-func", fields...)
}

// rebuildSSABlock converts ssaBlockData back to an s-expression.
func rebuildSSABlock(bd *ssaBlockData) values.Value {
	preds := make([]values.Value, len(bd.preds))
	for i, p := range bd.preds {
		preds[i] = values.NewInteger(p)
	}
	succs := make([]values.Value, len(bd.succs))
	for i, s := range bd.succs {
		succs[i] = values.NewInteger(s)
	}

	fields := []values.Value{
		goast.Field("index", values.NewInteger(bd.index)),
		goast.Field("preds", goast.ValueList(preds)),
		goast.Field("succs", goast.ValueList(succs)),
		goast.Field("instrs", goast.ValueList(bd.instrs)),
	}
	if bd.comment != "" {
		fields = append(fields, goast.Field("comment", goast.Str(bd.comment)))
	}
	if bd.idom != -1 {
		fields = append(fields, goast.Field("idom", values.NewInteger(bd.idom)))
	}
	return goast.Node("ssa-block", fields...)
}

// replaceField rebuilds a tagged alist with one field value replaced.
func replaceField(node *values.Pair, key string, newVal values.Value) values.Value {
	tag := node.Car()
	fields := node.Cdr()
	var result []values.Value
	cur := fields
	for !values.IsEmptyList(cur) {
		pair, ok := cur.(*values.Pair)
		if !ok {
			break
		}
		entry, ok := pair.Car().(*values.Pair)
		if ok {
			sym, ok := entry.Car().(*values.Symbol)
			if ok && sym.Key == key {
				result = append(result, goast.Field(key, newVal))
				cur = pair.Cdr()
				continue
			}
		}
		result = append(result, pair.Car())
		cur = pair.Cdr()
	}
	return values.NewCons(tag, values.List(result...))
}
