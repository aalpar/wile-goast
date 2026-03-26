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


// PrimGoCFGToStructured implements (go-cfg-to-structured block-sexpr).
// Takes a block s-expression. Returns a restructured block where early
// returns are eliminated (single exit point), or the block unchanged if
// no early returns, or #f if the block contains control flow it cannot
// restructure (goto, labeled branches, returns inside switch/select
// within loops).
//
// Two phases run in sequence:
//   - Case 2: Returns inside for/range → _ctl<N> = K; break + guard-ifs.
//   - Case 1: Guard-if-return patterns → nested if/else via right-fold.
func PrimGoCFGToStructured(mc *machine.MachineContext) error {
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
	if rest, ok := mc.Arg(1).(values.Tuple); ok {
		if pair, ok := rest.(*values.Pair); ok {
			ftVal := pair.Car()
			if ftVal != values.FalseValue {
				ftNode, err := unmapNode(ftVal)
				if err != nil {
					return werr.WrapForeignErrorf(errGoRestructureError,
						"go-cfg-to-structured: func-type: %s", err)
				}
				ft, ok := ftNode.(*ast.FuncType)
				if !ok {
					return werr.WrapForeignErrorf(errGoRestructureError,
						"go-cfg-to-structured: expected func-type, got %T", ftNode)
				}
				if ft.Results != nil {
					resultTypes = ft.Results.List
				}
			}
		}
	}

	gc := classifyGotos(block)
	if gc == gotoCrossBranch {
		mc.SetValue(values.FalseValue)
		return nil
	}

	stmts := block.List
	changed := false

	// Phase 0a: Backward gotos -> for loops.
	if gc == gotoBackwardOnly || gc == gotoMixed {
		// TODO: Task 10
		mc.SetValue(values.FalseValue)
		return nil
	}

	// Phase 0b: Forward gotos -> if !cond { ... }.
	if gc == gotoForwardOnly || gc == gotoMixed {
		stmts = restructureForwardGotos(stmts)
		changed = true
	}

	// Post-restructuring validation: if any gotos survived the pattern
	// matchers, bail honestly rather than returning an AST with gotos.
	if gc != gotoNone && containsGoto(&ast.BlockStmt{List: stmts}) {
		mc.SetValue(values.FalseValue)
		return nil
	}

	// Phase 1: Rewrite returns inside loops (Case 2).
	if hasLoopReturns(stmts) {
		ctlCounter := 0
		labelCounter := 0
		newStmts, ok := rewriteLoopReturns(stmts, &ctlCounter, &labelCounter, resultTypes)
		if !ok {
			mc.SetValue(values.FalseValue)
			return nil
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
	gotoNone        gotoClass = iota // no gotos
	gotoForwardOnly                  // all gotos jump to labels later in same block
	gotoBackwardOnly                 // all gotos jump to labels earlier in same block
	gotoMixed                        // mix of forward and backward
	gotoCrossBranch                  // goto into/out of if/switch branches
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
		if ls, ok := stmt.(*ast.LabeledStmt); ok {
			if gotoTargets[ls.Label.Name] {
				labelPos[ls.Label.Name] = i
			}
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
		if target > g.stmtIdx {
			hasForward = true
		} else {
			hasBackward = true
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
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
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
			if ls, ok := s.(*ast.LabeledStmt); ok {
				labelPos[ls.Label.Name] = j
			}
		}

		// Find the first forward goto and rewrite it.
		for i, stmt := range result {
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
				if ls, ok := targetStmt.(*ast.LabeledStmt); ok {
					targetStmt = ls.Stmt
				}
			}

			// Rebuild: result[:i] + wrapped + targetStmt + result[targetIdx+1:]
			var newStmts []ast.Stmt
			newStmts = append(newStmts, result[:i]...)
			newStmts = append(newStmts, wrapped)
			newStmts = append(newStmts, targetStmt)
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

// hasReturnInSwitch checks whether any switch/select/type-switch
// statement in the list contains a return in its body.
func hasReturnInSwitch(stmts []ast.Stmt) bool {
	found := false
	for _, stmt := range stmts {
		ast.Inspect(stmt, func(n ast.Node) bool {
			if found {
				return false
			}
			switch s := n.(type) {
			case *ast.SwitchStmt:
				if bodyContainsReturn(s.Body) {
					found = true
					return false
				}
			case *ast.TypeSwitchStmt:
				if bodyContainsReturn(s.Body) {
					found = true
					return false
				}
			case *ast.SelectStmt:
				if bodyContainsReturn(s.Body) {
					found = true
					return false
				}
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

// rewriteLoopReturns processes a statement list, rewriting for/range
// loops that contain returns. Each loop gets a _ctl<N> int variable;
// returns inside the loop become _ctl<N> = K; break. Guard-if-return
// statements are emitted after the loop.
//
// When a loop body contains returns inside switch/select, a labeled
// break is used instead (break <label>), and the loop is wrapped
// in a LabeledStmt.
//
// Returns (nil, false) if any loop contains unrewritable returns.
func rewriteLoopReturns(stmts []ast.Stmt, counter *int, labelCounter *int, resultTypes []*ast.Field) ([]ast.Stmt, bool) {
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
		innerStmts, ok := rewriteLoopReturns(body.List, counter, labelCounter, resultTypes)
		if !ok {
			return nil, false
		}

		// Allocate a control variable for this loop.
		ctlName := fmt.Sprintf("_ctl%d", *counter)
		*counter++

		// Determine if a labeled break is needed (returns inside switch/select).
		loopLabel := ""
		if hasReturnInSwitch(innerStmts) {
			loopLabel = fmt.Sprintf("_loop%d", *labelCounter)
			*labelCounter++
		}

		// Replace returns in the (now inner-processed) body.
		retIdx := 0
		var collected []*ast.ReturnStmt
		resultVarCount := 0
		var types []ast.Expr
		if resultTypes != nil {
			types = expandResultTypes(resultTypes)
			resultVarCount = len(types)
		}

		newBodyStmts, ok := replaceReturnsInStmtsLabeled(innerStmts, ctlName, &retIdx, &collected, loopLabel, resultVarCount)
		if !ok {
			return nil, false
		}

		// Rebuild the loop with the rewritten body.
		newBody := &ast.BlockStmt{List: newBodyStmts}
		result = append(result, makeVarDeclInt(ctlName))
		for i, ty := range types {
			result = append(result, makeVarDeclTyped(fmt.Sprintf("_r%d", i), ty))
		}
		var loopStmt ast.Stmt
		switch s := stmt.(type) {
		case *ast.ForStmt:
			loopStmt = &ast.ForStmt{Init: s.Init, Cond: s.Cond, Post: s.Post, Body: newBody}
		case *ast.RangeStmt:
			loopStmt = &ast.RangeStmt{Key: s.Key, Value: s.Value, Tok: s.Tok, X: s.X, Body: newBody}
		}
		if loopLabel != "" {
			loopStmt = &ast.LabeledStmt{Label: ast.NewIdent(loopLabel), Stmt: loopStmt}
		}
		result = append(result, loopStmt)

		// Emit guard-ifs for each collected return.
		for i, ret := range collected {
			result = append(result, makeCtlGuard(ctlName, i+1, ret))
		}
	}
	return result, true
}

// replaceReturnsInStmts walks a statement list replacing return-stmts
// with control variable assignment + break. Recurses into IfStmt and
// BlockStmt. Skips FuncLit and already-processed ForStmt/RangeStmt.
// Returns (nil, false) if a return is found inside switch/select
// (unless loopLabel is set, in which case switch/select are handled
// via labeled break).
func replaceReturnsInStmts(
	stmts []ast.Stmt,
	ctlName string,
	retIdx *int,
	collected *[]*ast.ReturnStmt,
) ([]ast.Stmt, bool) {
	return replaceReturnsInStmtsLabeled(stmts, ctlName, retIdx, collected, "", 0)
}

func replaceReturnsInStmtsLabeled(
	stmts []ast.Stmt,
	ctlName string,
	retIdx *int,
	collected *[]*ast.ReturnStmt,
	loopLabel string,
	resultVarCount int,
) ([]ast.Stmt, bool) {
	var result []ast.Stmt
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ReturnStmt:
			*retIdx++
			if resultVarCount > 0 {
				if len(s.Results) == 0 {
					// Naked return — cannot synthesize loop-local vars.
					return nil, false
				}
				if len(s.Results) != resultVarCount {
					// Multi-value call or count mismatch — bail.
					return nil, false
				}
				for i, expr := range s.Results {
					result = append(result, &ast.AssignStmt{
						Lhs: []ast.Expr{ast.NewIdent(fmt.Sprintf("_r%d", i))},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{expr},
					})
				}
				syntheticResults := make([]ast.Expr, resultVarCount)
				for i := range resultVarCount {
					syntheticResults[i] = ast.NewIdent(fmt.Sprintf("_r%d", i))
				}
				*collected = append(*collected, &ast.ReturnStmt{Results: syntheticResults})
			} else {
				*collected = append(*collected, s)
			}
			var brk ast.Stmt
			if loopLabel != "" {
				brk = makeLabeledBreak(loopLabel)
			} else {
				brk = makeBreakStmt()
			}
			result = append(result,
				makeIntAssign(ctlName, *retIdx),
				brk,
			)
		case *ast.IfStmt:
			newIf, ok := replaceReturnsInIf(s, ctlName, retIdx, collected, loopLabel, resultVarCount)
			if !ok {
				return nil, false
			}
			result = append(result, newIf)
		case *ast.BlockStmt:
			newList, ok := replaceReturnsInStmtsLabeled(s.List, ctlName, retIdx, collected, loopLabel, resultVarCount)
			if !ok {
				return nil, false
			}
			result = append(result, &ast.BlockStmt{List: newList})
		case *ast.SwitchStmt:
			if bodyContainsReturn(s.Body) {
				if loopLabel == "" {
					return nil, false
				}
				newClauses, ok := replaceReturnsInClauses(s.Body.List, ctlName, retIdx, collected, loopLabel, resultVarCount)
				if !ok {
					return nil, false
				}
				result = append(result, &ast.SwitchStmt{Init: s.Init, Tag: s.Tag, Body: &ast.BlockStmt{List: newClauses}})
			} else {
				result = append(result, stmt)
			}
		case *ast.TypeSwitchStmt:
			if bodyContainsReturn(s.Body) {
				if loopLabel == "" {
					return nil, false
				}
				newClauses, ok := replaceReturnsInClauses(s.Body.List, ctlName, retIdx, collected, loopLabel, resultVarCount)
				if !ok {
					return nil, false
				}
				result = append(result, &ast.TypeSwitchStmt{Init: s.Init, Assign: s.Assign, Body: &ast.BlockStmt{List: newClauses}})
			} else {
				result = append(result, stmt)
			}
		case *ast.SelectStmt:
			if bodyContainsReturn(s.Body) {
				if loopLabel == "" {
					return nil, false
				}
				newClauses, ok := replaceReturnsInClauses(s.Body.List, ctlName, retIdx, collected, loopLabel, resultVarCount)
				if !ok {
					return nil, false
				}
				result = append(result, &ast.SelectStmt{Body: &ast.BlockStmt{List: newClauses}})
			} else {
				result = append(result, stmt)
			}
		default:
			// ForStmt, RangeStmt (already processed), FuncLit,
			// and all other statements pass through unchanged.
			result = append(result, stmt)
		}
	}
	return result, true
}

// replaceReturnsInClauses processes case/comm clauses, replacing returns
// in their bodies with control variable assignment + labeled break.
func replaceReturnsInClauses(
	stmts []ast.Stmt,
	ctlName string,
	retIdx *int,
	collected *[]*ast.ReturnStmt,
	loopLabel string,
	resultVarCount int,
) ([]ast.Stmt, bool) {
	var result []ast.Stmt
	for _, stmt := range stmts {
		switch cc := stmt.(type) {
		case *ast.CaseClause:
			newBody, ok := replaceReturnsInStmtsLabeled(cc.Body, ctlName, retIdx, collected, loopLabel, resultVarCount)
			if !ok {
				return nil, false
			}
			result = append(result, &ast.CaseClause{List: cc.List, Body: newBody})
		case *ast.CommClause:
			newBody, ok := replaceReturnsInStmtsLabeled(cc.Body, ctlName, retIdx, collected, loopLabel, resultVarCount)
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

// replaceReturnsInIf processes an IfStmt, replacing returns in body
// and else branches.
func replaceReturnsInIf(
	ifStmt *ast.IfStmt,
	ctlName string,
	retIdx *int,
	collected *[]*ast.ReturnStmt,
	loopLabel string,
	resultVarCount int,
) (*ast.IfStmt, bool) {
	newBodyList, ok := replaceReturnsInStmtsLabeled(ifStmt.Body.List, ctlName, retIdx, collected, loopLabel, resultVarCount)
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
			newList, ok := replaceReturnsInStmtsLabeled(e.List, ctlName, retIdx, collected, loopLabel, resultVarCount)
			if !ok {
				return nil, false
			}
			newIf.Else = &ast.BlockStmt{List: newList}
		case *ast.IfStmt:
			newElse, ok := replaceReturnsInIf(e, ctlName, retIdx, collected, loopLabel, resultVarCount)
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

// makeVarDeclInt creates: var <name> int
func makeVarDeclInt(name string) ast.Stmt {
	return &ast.DeclStmt{
		Decl: &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{
				&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent(name)},
					Type:  ast.NewIdent("int"),
				},
			},
		},
	}
}

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

// cloneTypeExpr shallow-clones a type expression. Handles the common
// cases: idents, selectors, stars, arrays, maps.
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
	default:
		return expr // fallback: share reference
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
