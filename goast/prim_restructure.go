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
	"go/ast"
	"go/token"

	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)


// PrimGoCFGToStructured implements (go-cfg-to-structured block-sexpr).
// Takes a block s-expression. Returns a restructured block where early
// returns are nested into if/else chains (single exit point), or the
// block unchanged if no early returns, or #f if the block contains
// control flow it cannot restructure (goto, labeled branches).
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

	if !hasEarlyReturns(block.List) {
		mc.SetValue(blockVal)
		return nil
	}

	result := restructureStmts(block.List)

	opts := &mapperOpts{}
	mc.SetValue(mapNode(&ast.BlockStmt{List: result}, opts))
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
