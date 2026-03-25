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

	if containsGoto(block) {
		mc.SetValue(values.FalseValue)
		return nil
	}

	stmts := block.List
	changed := false

	// Phase 1: Rewrite returns inside loops (Case 2).
	if hasLoopReturns(stmts) {
		counter := 0
		newStmts, ok := rewriteLoopReturns(stmts, &counter)
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
// Returns (nil, false) if any loop contains unrewritable returns
// (inside switch/select).
func rewriteLoopReturns(stmts []ast.Stmt, counter *int) ([]ast.Stmt, bool) {
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
		innerStmts, ok := rewriteLoopReturns(body.List, counter)
		if !ok {
			return nil, false
		}

		// Allocate a control variable for this loop.
		ctlName := fmt.Sprintf("_ctl%d", *counter)
		*counter++

		// Replace returns in the (now inner-processed) body.
		retIdx := 0
		var collected []*ast.ReturnStmt
		newBodyStmts, ok := replaceReturnsInStmts(innerStmts, ctlName, &retIdx, &collected)
		if !ok {
			return nil, false
		}

		// Rebuild the loop with the rewritten body.
		newBody := &ast.BlockStmt{List: newBodyStmts}
		result = append(result, makeVarDeclInt(ctlName))
		switch s := stmt.(type) {
		case *ast.ForStmt:
			result = append(result, &ast.ForStmt{
				Init: s.Init, Cond: s.Cond, Post: s.Post, Body: newBody,
			})
		case *ast.RangeStmt:
			result = append(result, &ast.RangeStmt{
				Key: s.Key, Value: s.Value, Tok: s.Tok, X: s.X, Body: newBody,
			})
		}

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
// Returns (nil, false) if a return is found inside switch/select.
func replaceReturnsInStmts(
	stmts []ast.Stmt,
	ctlName string,
	retIdx *int,
	collected *[]*ast.ReturnStmt,
) ([]ast.Stmt, bool) {
	var result []ast.Stmt
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ReturnStmt:
			*retIdx++
			*collected = append(*collected, s)
			result = append(result,
				makeIntAssign(ctlName, *retIdx),
				makeBreakStmt(),
			)
		case *ast.IfStmt:
			newIf, ok := replaceReturnsInIf(s, ctlName, retIdx, collected)
			if !ok {
				return nil, false
			}
			result = append(result, newIf)
		case *ast.BlockStmt:
			newList, ok := replaceReturnsInStmts(s.List, ctlName, retIdx, collected)
			if !ok {
				return nil, false
			}
			result = append(result, &ast.BlockStmt{List: newList})
		case *ast.SwitchStmt:
			if bodyContainsReturn(s.Body) {
				return nil, false
			}
			result = append(result, stmt)
		case *ast.TypeSwitchStmt:
			if bodyContainsReturn(s.Body) {
				return nil, false
			}
			result = append(result, stmt)
		case *ast.SelectStmt:
			if bodyContainsReturn(s.Body) {
				return nil, false
			}
			result = append(result, stmt)
		default:
			// ForStmt, RangeStmt (already processed), FuncLit,
			// and all other statements pass through unchanged.
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
) (*ast.IfStmt, bool) {
	newBodyList, ok := replaceReturnsInStmts(ifStmt.Body.List, ctlName, retIdx, collected)
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
			newList, ok := replaceReturnsInStmts(e.List, ctlName, retIdx, collected)
			if !ok {
				return nil, false
			}
			newIf.Else = &ast.BlockStmt{List: newList}
		case *ast.IfStmt:
			newElse, ok := replaceReturnsInIf(e, ctlName, retIdx, collected)
			if !ok {
				return nil, false
			}
			newIf.Else = newElse
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
