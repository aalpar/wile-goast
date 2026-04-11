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

package goast

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

// PrimGoCFGToStructured implements (go-cfg-to-structured block [func-type]).
// Takes a block s-expression. Returns a restructured block where early
// returns and gotos are eliminated (single exit point), or the block
// unchanged if no early returns or gotos. Raises errGoRestructureError
// if the block contains control flow it cannot restructure.
//
// Optional second argument: a func-type s-expression for result variable
// synthesis (loop-local returns assigned to _r<N> variables).
//
// Four phases run in sequence:
//   - Phase 0a: Backward gotos → for-loops.
//   - Phase 0b: Forward gotos → if !cond { ... } wrapping.
//   - Phase 1: Returns inside for/range → _ctl<N> = K; break + guard-ifs.
//   - Phase 2: Guard-if-return patterns → nested if/else via right-fold.
func PrimGoCFGToStructured(mc machine.CallContext) error {
	blockVal := mc.Arg(0)

	node, err := unmapNode(blockVal)
	if err != nil {
		return werr.WrapForeignErrorf(errGoRestructureError,
			"go-cfg-to-structured: %s", err)
	}

	block, ok := node.(*ast.BlockStmt)
	if !ok {
		return werr.WrapForeignErrorf(errGoRestructureError,
			"go-cfg-to-structured: expected block, got %T", node)
	}

	// Extract result types from optional func-type argument.
	var resultTypes []*ast.Field
	tuple, ok := mc.Arg(1).(values.Tuple)
	if ok {
		pair, pok := tuple.(*values.Pair)
		if pok {
			ftVal := pair.Car()
			if ftVal != values.FalseValue {
				ftNode, err := unmapNode(ftVal)
				if err != nil {
					return werr.WrapForeignErrorf(errGoRestructureError,
						"go-cfg-to-structured: func-type: %s", err)
				}
				ft, ftOk := ftNode.(*ast.FuncType)
				if !ftOk {
					return werr.WrapForeignErrorf(errGoRestructureError,
						"go-cfg-to-structured: expected func-type, got %T", ftNode)
				}
				if ft.Results != nil {
					resultTypes = ft.Results.List
				}
			}
			// Reject extra arguments.
			cdr, cdrok := pair.Cdr().(values.Tuple)
			if cdrok && !values.IsEmptyList(cdr) {
				return werr.WrapForeignErrorf(errGoRestructureError,
					"go-cfg-to-structured: expected at most 1 optional argument, got extra")
			}
		}
	}

	gc := classifyGotos(block)
	if gc == gotoCrossBranch {
		return werr.WrapForeignErrorf(errGoRestructureError,
			"go-cfg-to-structured: cross-branch goto (target label inside nested block)")
	}

	stmts := block.List
	changed := false

	// Phase 0a: Backward gotos -> for loops.
	if gc == gotoBackwardOnly || gc == gotoMixed {
		stmts = restructureBackwardGotos(stmts)
		changed = true
	}

	// Phase 0b: Forward gotos -> if !cond { ... }.
	if gc == gotoForwardOnly || gc == gotoMixed {
		stmts = restructureForwardGotos(stmts)
		changed = true
	}

	// Post-restructuring validation: if any gotos survived the pattern
	// matchers, bail honestly rather than returning an AST with gotos.
	if gc != gotoNone && containsGoto(&ast.BlockStmt{List: stmts}) {
		return werr.WrapForeignErrorf(errGoRestructureError,
			"go-cfg-to-structured: goto pattern not recognized after restructuring")
	}

	// Phase 1: Rewrite returns inside loops (Case 2).
	if hasLoopReturns(stmts) {
		lw := loopRewriter{resultTypes: resultTypes}
		newStmts, ok := lw.rewriteLoops(stmts)
		if !ok {
			return werr.WrapForeignErrorf(errGoRestructureError,
				"go-cfg-to-structured: unrewritable return in loop (naked return or multi-value call)")
		}
		stmts = newStmts
		changed = true
	}

	// Phase 2: Fold linear guard-if-return into if/else (Case 1).
	if hasEarlyReturns(stmts) {
		stmts = restructureStmts(stmts)
		changed = true
	}

	if !changed {
		mc.SetValue(blockVal)
		return nil
	}

	opts := &mapperOpts{}
	mc.SetValue(mapNode(&ast.BlockStmt{List: stmts}, opts))
	return nil
}

// hasEarlyReturns checks whether the statement list contains return
// statements before the final position (inside if-bodies counts).
func hasEarlyReturns(stmts []ast.Stmt) bool {
	for i, stmt := range stmts {
		if i == len(stmts)-1 {
			break
		}
		if isGuardIf(stmt) {
			return true
		}
	}
	return false
}

// isGuardIf returns true if the statement is an if-stmt with no else
// branch whose body contains a return-stmt.
func isGuardIf(stmt ast.Stmt) bool {
	ifStmt, ok := stmt.(*ast.IfStmt)
	if !ok || ifStmt.Else != nil {
		return false
	}
	return bodyContainsReturn(ifStmt.Body)
}

// bodyContainsReturn walks a block looking for any return-stmt.
// Does NOT recurse into nested function literals (their returns
// belong to the literal, not the enclosing function).
func bodyContainsReturn(block *ast.BlockStmt) bool {
	found := false
	ast.Inspect(block, func(n ast.Node) bool {
		if found {
			return false
		}
		switch n.(type) {
		case *ast.ReturnStmt:
			found = true
			return false
		case *ast.FuncLit:
			return false
		}
		return true
	})
	return found
}

// containsGoto walks the block for goto statements or labeled
// statements. Returns true if any are found.
func containsGoto(block *ast.BlockStmt) bool {
	found := false
	ast.Inspect(block, func(n ast.Node) bool {
		if found {
			return false
		}
		switch v := n.(type) {
		case *ast.BranchStmt:
			if v.Tok == token.GOTO {
				found = true
				return false
			}
		case *ast.LabeledStmt:
			found = true
			return false
		case *ast.FuncLit:
			return false
		}
		return true
	})
	return found
}

type gotoClass int

const (
	gotoNone         gotoClass = iota // no gotos
	gotoForwardOnly                   // all gotos jump to labels later in same block
	gotoBackwardOnly                  // all gotos jump to labels earlier in same block
	gotoMixed                         // mix of forward and backward
	gotoCrossBranch                   // goto into/out of if/switch branches
)

// classifyGotos analyzes goto statements in a block.
//
// Classification is sound for flat function bodies where gotos and their
// target labels are top-level statements. A goto nested inside an if/switch
// at statement index N uses N as its source position (structural index, not
// execution order). Gotos targeting labels not in the top-level list are
// classified as gotoCrossBranch (conservative).
//
// Only labels that are actual goto targets are considered. Labels used
// solely by break/continue (e.g., `outer: for ...` with `break outer`)
// are ignored — stripping them would break the output AST.
func classifyGotos(block *ast.BlockStmt) gotoClass {
	// First pass: collect all goto target names.
	gotoTargets := map[string]bool{}
	for _, stmt := range block.List {
		ast.Inspect(stmt, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.BranchStmt:
				if v.Tok == token.GOTO && v.Label != nil {
					gotoTargets[v.Label.Name] = true
				}
			case *ast.FuncLit:
				return false
			}
			return true
		})
	}
	if len(gotoTargets) == 0 {
		return gotoNone
	}

	// Second pass: collect label positions, but only for labels that are
	// actual goto targets. This excludes break/continue loop labels.
	labelPos := map[string]int{}
	for i, stmt := range block.List {
		ls, ok := stmt.(*ast.LabeledStmt)
		if ok && gotoTargets[ls.Label.Name] {
			labelPos[ls.Label.Name] = i
		}
	}

	// Collect goto source positions.
	type gotoInfo struct {
		stmtIdx int
		label   string
	}
	var gotos []gotoInfo
	for i, stmt := range block.List {
		ast.Inspect(stmt, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.BranchStmt:
				if v.Tok == token.GOTO && v.Label != nil {
					gotos = append(gotos, gotoInfo{i, v.Label.Name})
				}
			case *ast.FuncLit:
				return false
			}
			return true
		})
	}

	hasForward := false
	hasBackward := false
	for _, g := range gotos {
		target, ok := labelPos[g.label]
		if !ok {
			// Label not at top level — can't restructure.
			return gotoCrossBranch
		}
		switch {
		case target > g.stmtIdx:
			hasForward = true
		case target < g.stmtIdx:
			hasBackward = true
		default:
			// Self-goto (label and goto at same statement index).
			// Cannot be restructured — classify as cross-branch.
			return gotoCrossBranch
		}
	}

	switch {
	case hasForward && hasBackward:
		return gotoMixed
	case hasForward:
		return gotoForwardOnly
	case hasBackward:
		return gotoBackwardOnly
	default:
		return gotoNone
	}
}

// singleGotoTarget returns the label name if the block contains exactly
// one statement which is a goto. Returns "" otherwise.
func singleGotoTarget(block *ast.BlockStmt) string {
	if len(block.List) != 1 {
		return ""
	}
	bs, ok := block.List[0].(*ast.BranchStmt)
	if !ok || bs.Tok != token.GOTO || bs.Label == nil {
		return ""
	}
	return bs.Label.Name
}

// negateExpr wraps an expression in !(...). Unwraps double negation.
func negateExpr(expr ast.Expr) ast.Expr {
	unary, ok := expr.(*ast.UnaryExpr)
	if ok && unary.Op == token.NOT {
		return unary.X
	}
	return &ast.UnaryExpr{Op: token.NOT, X: &ast.ParenExpr{X: expr}}
}

// restructureForwardGotos rewrites forward gotos in a statement list.
// Pattern: if cond { goto L } ... L: stmt
// Becomes: if !cond { ... } stmt
//
// Runs to fixpoint: each pass rewrites one goto, because wrapping
// statements shifts indices and may expose or relocate other gotos.
func restructureForwardGotos(stmts []ast.Stmt) []ast.Stmt {
	result := make([]ast.Stmt, len(stmts))
	copy(result, stmts)

	for {
		rewritten := false

		// Build label index.
		labelPos := map[string]int{}
		for j, s := range result {
			ls, ok := s.(*ast.LabeledStmt)
			if ok {
				labelPos[ls.Label.Name] = j
			}
		}

		// Find the last forward goto and rewrite it. Processing from the
		// bottom avoids burying unprocessed gotos inside wrapped blocks.
		for i := len(result) - 1; i >= 0; i-- {
			stmt := result[i]
			ifStmt, ok := stmt.(*ast.IfStmt)
			if !ok || ifStmt.Else != nil {
				continue
			}
			gotoTarget := singleGotoTarget(ifStmt.Body)
			if gotoTarget == "" {
				continue
			}
			targetIdx, ok := labelPos[gotoTarget]
			if !ok || targetIdx <= i {
				continue
			}

			// Wrap stmts[i+1..targetIdx-1] in if !cond { ... }.
			skipped := make([]ast.Stmt, targetIdx-i-1)
			copy(skipped, result[i+1:targetIdx])
			negated := negateExpr(ifStmt.Cond)
			wrapped := &ast.IfStmt{
				Cond: negated,
				Body: &ast.BlockStmt{List: skipped},
			}

			// Only strip the label if no other goto targets it.
			targetStmt := result[targetIdx]
			if !hasOtherGoto(result, gotoTarget, i) {
				ls, lsOk := targetStmt.(*ast.LabeledStmt)
				if lsOk {
					targetStmt = ls.Stmt
				}
			}

			// Rebuild: result[:i] + wrapped + targetStmt + result[targetIdx+1:]
			var newStmts []ast.Stmt
			newStmts = append(newStmts, result[:i]...)
			newStmts = append(newStmts, wrapped, targetStmt)
			newStmts = append(newStmts, result[targetIdx+1:]...)
			result = newStmts
			rewritten = true
			break // restart from the top
		}

		if !rewritten {
			break
		}
	}

	return result
}

// hasOtherGoto returns true if any top-level if-goto in stmts (other than
// the one at excludeIdx) targets the given label.
func hasOtherGoto(stmts []ast.Stmt, label string, excludeIdx int) bool {
	for i, stmt := range stmts {
		if i == excludeIdx {
			continue
		}
		ifStmt, ok := stmt.(*ast.IfStmt)
		if !ok || ifStmt.Else != nil {
			continue
		}
		if singleGotoTarget(ifStmt.Body) == label {
			return true
		}
	}
	return false
}

// restructureBackwardGotos rewrites backward gotos as for-loops.
// Handles two patterns:
//   - Conditional: label: stmt ... if cond { goto label }
//     Becomes:     for { stmt ... if !cond { break } }
//   - Unconditional: label: stmt ... goto label
//     Becomes:       for { stmt ... }
func restructureBackwardGotos(stmts []ast.Stmt) []ast.Stmt {
	var result []ast.Stmt

	for i := 0; i < len(stmts); i++ {
		ls, ok := stmts[i].(*ast.LabeledStmt)
		if !ok {
			result = append(result, stmts[i])
			continue
		}

		label := ls.Label.Name
		gotoIdx := -1
		isBareGoto := false
		for j := i + 1; j < len(stmts); j++ {
			if ifHasGoto(stmts[j], label) {
				gotoIdx = j
				break
			}
			if bareGotoTarget(stmts[j]) == label {
				gotoIdx = j
				isBareGoto = true
				break
			}
		}

		if gotoIdx == -1 {
			result = append(result, stmts[i])
			continue
		}

		// Build loop body: labeled stmt's inner + stmts between label and goto.
		var loopBody []ast.Stmt
		loopBody = append(loopBody, ls.Stmt)
		loopBody = append(loopBody, stmts[i+1:gotoIdx]...)

		if !isBareGoto {
			// Conditional: replace goto-if with break-if (negated condition).
			gotoIf, ok := stmts[gotoIdx].(*ast.IfStmt)
			if !ok {
				// ifHasGoto guarantees *ast.IfStmt; defend against future refactors.
				result = append(result, stmts[i])
				continue
			}
			loopBody = append(loopBody, &ast.IfStmt{
				Cond: negateExpr(gotoIf.Cond),
				Body: &ast.BlockStmt{List: []ast.Stmt{makeBreakStmt()}},
			})
		}
		// Bare goto: infinite loop with no break condition.

		result = append(result, &ast.ForStmt{
			Body: &ast.BlockStmt{List: loopBody},
		})

		i = gotoIdx // skip past the goto
	}

	return result
}

// ifHasGoto checks if a statement is `if cond { goto label }`.
func ifHasGoto(stmt ast.Stmt, label string) bool {
	ifStmt, ok := stmt.(*ast.IfStmt)
	if !ok || ifStmt.Else != nil {
		return false
	}
	return singleGotoTarget(ifStmt.Body) == label
}

// bareGotoTarget returns the label if stmt is a bare `goto label`. Returns "" otherwise.
func bareGotoTarget(stmt ast.Stmt) string {
	bs, ok := stmt.(*ast.BranchStmt)
	if !ok || bs.Tok != token.GOTO || bs.Label == nil {
		return ""
	}
	return bs.Label.Name
}

// restructureStmts right-folds the statement list. Guard-if statements
// get the accumulated "rest" as their else branch. Non-if statements
// are prepended to "rest."
func restructureStmts(stmts []ast.Stmt) []ast.Stmt {
	if len(stmts) == 0 {
		return stmts
	}

	rest := []ast.Stmt{stmts[len(stmts)-1]}

	for i := len(stmts) - 2; i >= 0; i-- {
		stmt := stmts[i]
		if isGuardIf(stmt) {
			ifStmt := stmt.(*ast.IfStmt) // Safe: isGuardIf guarantees *ast.IfStmt
			ifStmt = &ast.IfStmt{
				Init: ifStmt.Init,
				Cond: ifStmt.Cond,
				Body: ifStmt.Body,
				Else: wrapBlock(rest),
			}
			rest = []ast.Stmt{ifStmt}
		} else {
			rest = append([]ast.Stmt{stmt}, rest...)
		}
	}

	return rest
}

// wrapBlock wraps a statement list in a BlockStmt. If the list is a
// single IfStmt, returns it directly (Go allows if-stmt as else
// branch without wrapping).
func wrapBlock(stmts []ast.Stmt) ast.Stmt {
	if len(stmts) == 1 {
		switch stmts[0].(type) {
		case *ast.BlockStmt, *ast.IfStmt:
			return stmts[0]
		}
	}
	return &ast.BlockStmt{List: stmts}
}

// branchingBody returns the Body block from switch/type-switch/select
// statements. Returns nil, false for all other node types.
func branchingBody(n ast.Node) (*ast.BlockStmt, bool) {
	switch s := n.(type) {
	case *ast.SwitchStmt:
		return s.Body, true
	case *ast.TypeSwitchStmt:
		return s.Body, true
	case *ast.SelectStmt:
		return s.Body, true
	}
	return nil, false
}

// rebuildBranching reconstructs a switch/type-switch/select statement
// with a new body, preserving all other fields.
func rebuildBranching(stmt ast.Stmt, newBody *ast.BlockStmt) ast.Stmt {
	switch s := stmt.(type) {
	case *ast.SwitchStmt:
		return &ast.SwitchStmt{Init: s.Init, Tag: s.Tag, Body: newBody}
	case *ast.TypeSwitchStmt:
		return &ast.TypeSwitchStmt{Init: s.Init, Assign: s.Assign, Body: newBody}
	case *ast.SelectStmt:
		return &ast.SelectStmt{Body: newBody}
	}
	return stmt
}

// hasReturnInSwitch checks whether any switch/select/type-switch
// statement in the list contains a return in its body.
func hasReturnInSwitch(stmts []ast.Stmt) bool {
	found := false
	for _, stmt := range stmts {
		ast.Inspect(stmt, func(n ast.Node) bool {
			if found {
				return false
			}
			body, isBranching := branchingBody(n)
			if isBranching {
				if bodyContainsReturn(body) {
					found = true
				}
				return false
			}
			switch n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.ForStmt, *ast.RangeStmt:
				// Don't look inside nested loops — they get their own labels.
				return false
			}
			return true
		})
	}
	return found
}

// --- Case 2: Loop return rewriting ---

// hasLoopReturns checks whether any top-level for/range statement
// contains a return in its body.
func hasLoopReturns(stmts []ast.Stmt) bool {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ForStmt:
			if bodyContainsReturn(s.Body) {
				return true
			}
		case *ast.RangeStmt:
			if bodyContainsReturn(s.Body) {
				return true
			}
		}
	}
	return false
}

// loopRewriter holds state for rewriting returns inside for/range loops.
// Shared counters are incremented across sibling loops to produce unique
// names. Per-loop fields are reset by rewriteLoops for each loop.
type loopRewriter struct {
	// ctlCounter allocates unique _ctl<N> control variable names.
	ctlCounter int
	// labelCounter allocates unique _loop<N> labeled-break labels.
	labelCounter int
	// resultVarCounter allocates unique _r<N> result variable names
	// across sibling loops.
	resultVarCounter int
	// resultTypes holds the function's result field list for
	// synthesizing loop-local result variables. Nil when the
	// function has no results.
	resultTypes []*ast.Field

	// ctlName is the control variable name for the current loop.
	ctlName string
	// retIdx counts return sites within the current loop (1-based
	// after increment; used as the control variable value).
	retIdx int
	// collected gathers the original (or synthetic) ReturnStmts
	// replaced in the current loop, emitted as guard-ifs afterward.
	collected []*ast.ReturnStmt
	// loopLabel is the labeled-break label for the current loop.
	// Empty when no switch/select requires a labeled break.
	loopLabel string
	// resultVarCount is the number of result variables for the
	// current loop (len of expanded result types).
	resultVarCount int
	// resultVarBase is the starting index into the _r<N> namespace
	// for the current loop's result variables.
	resultVarBase int
}

// rewriteLoops processes a statement list, rewriting for/range loops
// that contain returns. Each loop gets a _ctl<N> int variable; returns
// inside the loop become _ctl<N> = K; break. Guard-if-return statements
// are emitted after the loop.
//
// When a loop body contains returns inside switch/select, a labeled
// break is used instead (break <label>), and the loop is wrapped
// in a LabeledStmt.
//
// Returns (nil, false) if any loop contains unrewritable returns.
func (lw *loopRewriter) rewriteLoops(stmts []ast.Stmt) ([]ast.Stmt, bool) {
	var result []ast.Stmt
	for _, stmt := range stmts {
		var body *ast.BlockStmt
		switch s := stmt.(type) {
		case *ast.ForStmt:
			body = s.Body
		case *ast.RangeStmt:
			body = s.Body
		}
		if body == nil || !bodyContainsReturn(body) {
			result = append(result, stmt)
			continue
		}

		// Bottom-up: process inner loops first.
		innerStmts, ok := lw.rewriteLoops(body.List)
		if !ok {
			return nil, false
		}

		// Allocate control variable and set per-loop state.
		lw.ctlName = fmt.Sprintf("_ctl%d", lw.ctlCounter)
		lw.ctlCounter++

		lw.loopLabel = ""
		if hasReturnInSwitch(innerStmts) {
			lw.loopLabel = fmt.Sprintf("_loop%d", lw.labelCounter)
			lw.labelCounter++
		}

		lw.retIdx = 0
		lw.collected = nil
		lw.resultVarCount = 0
		lw.resultVarBase = 0
		var types []ast.Expr
		if lw.resultTypes != nil {
			types = expandResultTypes(lw.resultTypes)
			lw.resultVarCount = len(types)
			lw.resultVarBase = lw.resultVarCounter
			lw.resultVarCounter += lw.resultVarCount
		}

		newBodyStmts, ok := lw.replaceInStmts(innerStmts)
		if !ok {
			return nil, false
		}

		// Rebuild the loop with the rewritten body.
		newBody := &ast.BlockStmt{List: newBodyStmts}
		result = append(result, makeVarDeclTyped(lw.ctlName, ast.NewIdent("int")))
		for i, ty := range types {
			result = append(result, makeVarDeclTyped(fmt.Sprintf("_r%d", lw.resultVarBase+i), ty))
		}
		var loopStmt ast.Stmt
		switch s := stmt.(type) {
		case *ast.ForStmt:
			loopStmt = &ast.ForStmt{Init: s.Init, Cond: s.Cond, Post: s.Post, Body: newBody}
		case *ast.RangeStmt:
			loopStmt = &ast.RangeStmt{Key: s.Key, Value: s.Value, Tok: s.Tok, X: s.X, Body: newBody}
		}
		if lw.loopLabel != "" {
			loopStmt = &ast.LabeledStmt{Label: ast.NewIdent(lw.loopLabel), Stmt: loopStmt}
		}
		result = append(result, loopStmt)

		// Emit guard-ifs for each collected return.
		for i, ret := range lw.collected {
			result = append(result, makeCtlGuard(lw.ctlName, i+1, ret))
		}
	}
	return result, true
}

// replaceInStmts walks a statement list replacing return-stmts with
// control variable assignment + break. Recurses into IfStmt and
// BlockStmt. Skips FuncLit and already-processed ForStmt/RangeStmt.
// Returns (nil, false) if a return is found inside switch/select
// (unless loopLabel is set, in which case switch/select are handled
// via labeled break). When resultVarCount > 0, return values are
// assigned to _r<resultVarBase+i> before the break.
func (lw *loopRewriter) replaceInStmts(stmts []ast.Stmt) ([]ast.Stmt, bool) {
	var result []ast.Stmt
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ReturnStmt:
			lw.retIdx++
			if lw.resultVarCount > 0 {
				if len(s.Results) == 0 {
					// Naked return — cannot synthesize loop-local vars.
					return nil, false
				}
				if len(s.Results) != lw.resultVarCount {
					// Multi-value call or count mismatch — bail.
					return nil, false
				}
				for i, expr := range s.Results {
					result = append(result, &ast.AssignStmt{
						Lhs: []ast.Expr{ast.NewIdent(fmt.Sprintf("_r%d", lw.resultVarBase+i))},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{expr},
					})
				}
				syntheticResults := make([]ast.Expr, lw.resultVarCount)
				for i := range lw.resultVarCount {
					syntheticResults[i] = ast.NewIdent(fmt.Sprintf("_r%d", lw.resultVarBase+i))
				}
				lw.collected = append(lw.collected, &ast.ReturnStmt{Results: syntheticResults})
			} else {
				lw.collected = append(lw.collected, s)
			}
			var brk ast.Stmt
			if lw.loopLabel != "" {
				brk = makeLabeledBreak(lw.loopLabel)
			} else {
				brk = makeBreakStmt()
			}
			result = append(result,
				makeIntAssign(lw.ctlName, lw.retIdx),
				brk,
			)
		case *ast.IfStmt:
			newIf, ok := lw.replaceInIf(s)
			if !ok {
				return nil, false
			}
			result = append(result, newIf)
		case *ast.BlockStmt:
			newList, ok := lw.replaceInStmts(s.List)
			if !ok {
				return nil, false
			}
			result = append(result, &ast.BlockStmt{List: newList})
		default:
			body, isBranching := branchingBody(stmt)
			if isBranching && bodyContainsReturn(body) {
				if lw.loopLabel == "" {
					return nil, false
				}
				newClauses, ok := lw.replaceInClauses(body.List)
				if !ok {
					return nil, false
				}
				result = append(result, rebuildBranching(stmt, &ast.BlockStmt{List: newClauses}))
			} else {
				// ForStmt, RangeStmt (already processed), FuncLit,
				// switch/select without returns, and all other
				// statements pass through unchanged.
				result = append(result, stmt)
			}
		}
	}
	return result, true
}

// replaceInClauses processes case/comm clauses, replacing returns
// in their bodies with control variable assignment + labeled break.
func (lw *loopRewriter) replaceInClauses(stmts []ast.Stmt) ([]ast.Stmt, bool) {
	var result []ast.Stmt
	for _, stmt := range stmts {
		switch cc := stmt.(type) {
		case *ast.CaseClause:
			newBody, ok := lw.replaceInStmts(cc.Body)
			if !ok {
				return nil, false
			}
			result = append(result, &ast.CaseClause{List: cc.List, Body: newBody})
		case *ast.CommClause:
			newBody, ok := lw.replaceInStmts(cc.Body)
			if !ok {
				return nil, false
			}
			result = append(result, &ast.CommClause{Comm: cc.Comm, Body: newBody})
		default:
			if bodyContainsReturn(&ast.BlockStmt{List: []ast.Stmt{stmt}}) {
				return nil, false
			}
			result = append(result, stmt)
		}
	}
	return result, true
}

// replaceInIf processes an IfStmt, replacing returns in body
// and else branches.
func (lw *loopRewriter) replaceInIf(ifStmt *ast.IfStmt) (*ast.IfStmt, bool) {
	newBodyList, ok := lw.replaceInStmts(ifStmt.Body.List)
	if !ok {
		return nil, false
	}

	newIf := &ast.IfStmt{
		Init: ifStmt.Init,
		Cond: ifStmt.Cond,
		Body: &ast.BlockStmt{List: newBodyList},
	}

	if ifStmt.Else != nil {
		switch e := ifStmt.Else.(type) {
		case *ast.BlockStmt:
			newList, ok := lw.replaceInStmts(e.List)
			if !ok {
				return nil, false
			}
			newIf.Else = &ast.BlockStmt{List: newList}
		case *ast.IfStmt:
			newElse, ok := lw.replaceInIf(e)
			if !ok {
				return nil, false
			}
			newIf.Else = newElse
		default:
			return nil, false
		}
	}

	return newIf, true
}

// --- AST constructors for loop rewriting ---

// makeIntAssign creates: <name> = <val>
func makeIntAssign(name string, val int) ast.Stmt {
	return &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(name)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", val)}},
	}
}

// makeBreakStmt creates: break
func makeBreakStmt() ast.Stmt {
	return &ast.BranchStmt{Tok: token.BREAK}
}

// makeLabeledBreak creates: break <label>
func makeLabeledBreak(label string) ast.Stmt {
	return &ast.BranchStmt{Tok: token.BREAK, Label: ast.NewIdent(label)}
}

// makeVarDeclTyped creates: var <name> <typeExpr>
func makeVarDeclTyped(name string, typeExpr ast.Expr) ast.Stmt {
	return &ast.DeclStmt{
		Decl: &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{
				&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent(name)},
					Type:  typeExpr,
				},
			},
		},
	}
}

// expandResultTypes expands a field list into one type expression per
// result position. For (x, y int, err error) this returns [int, int, error].
// Each position gets a cloned type expression to avoid AST aliasing when
// fields group multiple names (e.g., x, y int shares one ast.Expr).
func expandResultTypes(fields []*ast.Field) []ast.Expr {
	var types []ast.Expr
	for _, f := range fields {
		n := len(f.Names)
		if n == 0 {
			n = 1 // unnamed result
		}
		for i := range n {
			if i == 0 {
				types = append(types, f.Type)
			} else {
				types = append(types, cloneTypeExpr(f.Type))
			}
		}
	}
	return types
}

// cloneTypeExpr shallow-clones a type expression to avoid AST aliasing
// when fields group multiple names (e.g., x, y int shares one ast.Expr).
func cloneTypeExpr(expr ast.Expr) ast.Expr {
	switch e := expr.(type) {
	case *ast.Ident:
		return ast.NewIdent(e.Name)
	case *ast.SelectorExpr:
		return &ast.SelectorExpr{X: cloneTypeExpr(e.X), Sel: ast.NewIdent(e.Sel.Name)}
	case *ast.StarExpr:
		return &ast.StarExpr{X: cloneTypeExpr(e.X)}
	case *ast.ArrayType:
		return &ast.ArrayType{Len: e.Len, Elt: cloneTypeExpr(e.Elt)}
	case *ast.MapType:
		return &ast.MapType{Key: cloneTypeExpr(e.Key), Value: cloneTypeExpr(e.Value)}
	case *ast.ChanType:
		return &ast.ChanType{Dir: e.Dir, Value: cloneTypeExpr(e.Value)}
	case *ast.FuncType:
		return &ast.FuncType{Params: e.Params, Results: e.Results}
	case *ast.InterfaceType:
		return &ast.InterfaceType{Methods: e.Methods}
	case *ast.StructType:
		return &ast.StructType{Fields: e.Fields}
	case *ast.Ellipsis:
		return &ast.Ellipsis{Elt: cloneTypeExpr(e.Elt)}
	case *ast.IndexExpr:
		return &ast.IndexExpr{X: cloneTypeExpr(e.X), Index: cloneTypeExpr(e.Index)}
	default:
		return expr // fallback: share reference (unknown expression type)
	}
}

// makeCtlGuard creates: if <name> == <val> { <retStmt> }
func makeCtlGuard(name string, val int, retStmt *ast.ReturnStmt) ast.Stmt {
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X:  ast.NewIdent(name),
			Op: token.EQL,
			Y:  &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", val)},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{retStmt}},
	}
}
