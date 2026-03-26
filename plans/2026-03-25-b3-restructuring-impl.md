# B3 Restructuring Improvements — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extend `go-cfg-to-structured` to handle labeled breaks (switch/select
returns in loops), multiple return values (loop-local variable scoping), and
forward/backward goto elimination.

**Architecture:** Three independent features added to `goast/prim_restructure.go`.
B3.1 (labeled break) modifies the loop return rewriter to recurse into switch/select
bodies using labeled breaks. B3.2 (multiple return values) adds an optional func-type
argument and synthesizes result variables. B3.3 (goto) replaces the `containsGoto`
bail-out with limited goto elimination for forward and backward patterns.

**Tech Stack:** Go (`go/ast`, `go/token`), tested via Scheme evaluation in `restructure_test.go`.

---

### Task 1: Labeled break — AST helpers

**Files:**
- Modify: `goast/prim_restructure.go`

**Step 1: Add `makeLabeledBreak` helper**

After the existing `makeBreakStmt` at line 407, add:

```go
// makeLabeledBreak creates: break <label>
func makeLabeledBreak(label string) ast.Stmt {
	return &ast.BranchStmt{Tok: token.BREAK, Label: ast.NewIdent(label)}
}
```

**Step 2: Add `hasReturnInSwitch` detection helper**

After `hasLoopReturns`, add:

```go
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
```

**Step 3: Add `replaceReturnsInClauses` for switch/select case bodies**

```go
// replaceReturnsInClauses processes case/comm clauses, replacing returns
// in their bodies with control variable assignment + labeled break.
func replaceReturnsInClauses(
	stmts []ast.Stmt,
	ctlName string,
	retIdx *int,
	collected *[]*ast.ReturnStmt,
	loopLabel string,
) ([]ast.Stmt, bool) {
	var result []ast.Stmt
	for _, stmt := range stmts {
		switch cc := stmt.(type) {
		case *ast.CaseClause:
			newBody, ok := replaceReturnsInStmtsLabeled(cc.Body, ctlName, retIdx, collected, loopLabel)
			if !ok {
				return nil, false
			}
			result = append(result, &ast.CaseClause{List: cc.List, Body: newBody})
		case *ast.CommClause:
			newBody, ok := replaceReturnsInStmtsLabeled(cc.Body, ctlName, retIdx, collected, loopLabel)
			if !ok {
				return nil, false
			}
			result = append(result, &ast.CommClause{Comm: cc.Comm, Body: newBody})
		default:
			result = append(result, stmt)
		}
	}
	return result, true
}
```

**Step 4: Run tests to verify helpers compile**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build ./goast/...`
Expected: compile success (helpers are unused but that's fine — they'll be wired in Task 2)

Note: If the compiler warns about unused functions, that's expected. We wire them in the
next task. If it errors, address the compile error.

**Step 5: Commit**

```
feat(restructure): add labeled break AST helpers
```

---

### Task 2: Labeled break — wire into loop rewriter

**Files:**
- Modify: `goast/prim_restructure.go`

**Step 1: Rename `replaceReturnsInStmts` to `replaceReturnsInStmtsLabeled` and add `loopLabel` parameter**

Change the signature and body:

```go
func replaceReturnsInStmtsLabeled(
	stmts []ast.Stmt,
	ctlName string,
	retIdx *int,
	collected *[]*ast.ReturnStmt,
	loopLabel string,
) ([]ast.Stmt, bool) {
```

In the `ReturnStmt` case, change `makeBreakStmt()` to:

```go
		case *ast.ReturnStmt:
			*retIdx++
			*collected = append(*collected, s)
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
```

In the `SwitchStmt`, `TypeSwitchStmt`, and `SelectStmt` cases, instead of bailing,
recurse when `loopLabel` is set:

```go
		case *ast.SwitchStmt:
			if bodyContainsReturn(s.Body) {
				if loopLabel == "" {
					return nil, false
				}
				newClauses, ok := replaceReturnsInClauses(s.Body.List, ctlName, retIdx, collected, loopLabel)
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
				newClauses, ok := replaceReturnsInClauses(s.Body.List, ctlName, retIdx, collected, loopLabel)
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
				newClauses, ok := replaceReturnsInClauses(s.Body.List, ctlName, retIdx, collected, loopLabel)
				if !ok {
					return nil, false
				}
				result = append(result, &ast.SelectStmt{Body: &ast.BlockStmt{List: newClauses}})
			} else {
				result = append(result, stmt)
			}
```

**Step 2: Add backward-compatible wrapper `replaceReturnsInStmts`**

Keep the old signature as a wrapper:

```go
func replaceReturnsInStmts(
	stmts []ast.Stmt,
	ctlName string,
	retIdx *int,
	collected *[]*ast.ReturnStmt,
) ([]ast.Stmt, bool) {
	return replaceReturnsInStmtsLabeled(stmts, ctlName, retIdx, collected, "")
}
```

**Step 3: Update `replaceReturnsInIf` to accept and pass `loopLabel`**

Add `loopLabel string` parameter, thread it through recursive calls to
`replaceReturnsInStmtsLabeled` and itself. Update the `IfStmt` case in
`replaceReturnsInStmtsLabeled` to call the new signature.

**Step 4: Wire label assignment into `rewriteLoopReturns`**

Add a `labelCounter *int` parameter to `rewriteLoopReturns`. In
`PrimGoCFGToStructured`, initialize it:

```go
	if hasLoopReturns(stmts) {
		ctlCounter := 0
		labelCounter := 0
		newStmts, ok := rewriteLoopReturns(stmts, &ctlCounter, &labelCounter)
```

In `rewriteLoopReturns`, after processing inner loops and before
`replaceReturnsInStmts`, check if a label is needed:

```go
		loopLabel := ""
		if hasReturnInSwitch(innerStmts) {
			loopLabel = fmt.Sprintf("_loop%d", *labelCounter)
			*labelCounter++
		}

		newBodyStmts, ok := replaceReturnsInStmtsLabeled(innerStmts, ctlName, &retIdx, &collected, loopLabel)
```

When emitting the loop, wrap it in a `LabeledStmt` if `loopLabel` is set:

```go
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
```

**Step 5: Run build**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build ./goast/...`
Expected: compile success

**Step 6: Run existing tests to verify no regressions**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestGoCFGToStructured -v`
Expected: all 11 existing tests pass

**Step 7: Commit**

```
refactor(restructure): wire labeled break into loop rewriter
```

---

### Task 3: Labeled break — tests

**Files:**
- Modify: `goast/restructure_test.go`

**Step 1: Change existing `LoopReturnInSwitch` test to expect success**

The test `TestGoCFGToStructured_LoopReturnInSwitch` currently expects `#f`.
Change it to verify the restructured output:

```go
func TestGoCFGToStructured_LoopReturnInSwitch(t *testing.T) {
	engine := newEngine(t)

	scheme_eval(t, engine, `
		(define source "
			package p
			func f(items []int) int {
				for _, v := range items {
					switch {
					case v < 0:
						return -1
					}
				}
				return 0
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("returns a block", func(t *testing.T) {
		result := scheme_eval(t, engine, `(eq? (car result) 'block)`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("has labeled break", func(t *testing.T) {
		result := scheme_eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "_loop0"), qt.IsTrue,
			qt.Commentf("expected loop label, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "break _loop0"), qt.IsTrue,
			qt.Commentf("expected labeled break, got:\n%s", s))
	})

	t.Run("has guard after loop", func(t *testing.T) {
		result := scheme_eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "return -1"), qt.IsTrue,
			qt.Commentf("expected return in guard, got:\n%s", s))
	})
}
```

Note: The test helper is called `eval` in the existing tests. Use the same name
as the existing tests — check `prim_goast_test.go` for the actual helper name.

**Step 2: Add test for type-switch with return in loop**

```go
func TestGoCFGToStructured_LoopReturnInTypeSwitch(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(items []interface{}) int {
				for _, v := range items {
					switch v.(type) {
					case int:
						return 1
					case string:
						return 2
					}
				}
				return 0
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("has labeled break and multiple guards", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "break _loop0"), qt.IsTrue,
			qt.Commentf("expected labeled break, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "return 1"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "return 2"), qt.IsTrue)
	})
}
```

**Step 3: Add test for select with return in loop**

```go
func TestGoCFGToStructured_LoopReturnInSelect(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(ch chan int, done chan bool) int {
				for {
					select {
					case v := <-ch:
						if v < 0 { return v }
					case <-done:
						return 0
					}
				}
				return -1
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("has labeled break", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "break _loop0"), qt.IsTrue,
			qt.Commentf("expected labeled break, got:\n%s", s))
	})
}
```

**Step 4: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestGoCFGToStructured -v`
Expected: all tests pass including the new ones

**Step 5: Commit**

```
test(restructure): labeled break for switch/select returns in loops
```

---

### Task 4: Multiple return values — optional func-type argument

**Files:**
- Modify: `goast/register.go`
- Modify: `goast/prim_restructure.go`

**Step 1: Change primitive registration to variadic**

In `register.go`, change the `go-cfg-to-structured` entry:

```go
		{Name: "go-cfg-to-structured", ParamCount: 2, IsVariadic: true, Impl: PrimGoCFGToStructured,
			Doc:        "Restructures a block with early returns into a single-exit if/else tree. Optional second arg: func-type for result variable synthesis.",
			ParamNames: []string{"block", "func-type"}, Category: "goast"},
```

**Step 2: Extract result types from optional func-type in `PrimGoCFGToStructured`**

At the top of `PrimGoCFGToStructured`, after parsing the block, extract result types
if the second argument is provided:

```go
	var resultTypes []*ast.Field
	if mc.NArgs() > 1 {
		ftVal := mc.Arg(1)
		if !values.IsFalse(ftVal) {
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
```

**Step 3: Thread `resultTypes` into `rewriteLoopReturns`**

Add `resultTypes []*ast.Field` parameter to `rewriteLoopReturns`. Pass it from
`PrimGoCFGToStructured`.

**Step 4: Run build**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build ./goast/...`
Expected: compile success

**Step 5: Run existing tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestGoCFGToStructured -v`
Expected: all pass (second arg is optional, so existing tests are unaffected)

**Step 6: Commit**

```
feat(restructure): accept optional func-type for result variable synthesis
```

---

### Task 5: Multiple return values — result variable synthesis

**Files:**
- Modify: `goast/prim_restructure.go`

**Step 1: Add `makeVarDeclTyped` helper**

```go
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
```

**Step 2: Add `expandResultTypes` to flatten field list into per-variable types**

Go field lists can group names: `(x, y int)` is two fields, one `*ast.Field`.
We need one type expression per return position.

```go
// expandResultTypes expands a field list into one type expression per
// result position. For (x, y int, err error) this returns [int, int, error].
func expandResultTypes(fields []*ast.Field) []ast.Expr {
	var types []ast.Expr
	for _, f := range fields {
		n := len(f.Names)
		if n == 0 {
			n = 1 // unnamed result
		}
		for range n {
			types = append(types, f.Type)
		}
	}
	return types
}
```

**Step 3: Modify `rewriteLoopReturns` to synthesize result variables**

When `resultTypes` is non-nil and a collected return has values, emit result variable
declarations and replace return values with assignments.

In `rewriteLoopReturns`, compute `resultVarCount` from `resultTypes` and pass it
to `replaceReturnsInStmtsLabeled`. Emit `var _r0 T; var _r1 T; ...` declarations
alongside the `var _ctlN int` declaration.

```go
		resultVarCount := 0
		var types []ast.Expr
		if resultTypes != nil {
			types = expandResultTypes(resultTypes)
			resultVarCount = len(types)
		}

		newBodyStmts, ok := replaceReturnsInStmtsLabeled(innerStmts, ctlName, &retIdx, &collected, loopLabel, resultVarCount)
```

After the ctl var declaration, emit result var declarations:

```go
		result = append(result, makeVarDeclInt(ctlName))
		for i, ty := range types {
			result = append(result, makeVarDeclTyped(fmt.Sprintf("_r%d", i), ty))
		}
```

**Step 4: Modify return replacement to emit assignments**

Add `resultVarCount int` parameter to `replaceReturnsInStmtsLabeled` (and its
callees). When > 0, emit assignments before the ctl assignment:

```go
		case *ast.ReturnStmt:
			*retIdx++
			if resultVarCount > 0 {
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
```

**Step 5: Run build**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build ./goast/...`
Expected: compile success

**Step 6: Commit**

```
feat(restructure): synthesize result variables for loop-local returns
```

---

### Task 6: Multiple return values — tests

**Files:**
- Modify: `goast/restructure_test.go`

**Step 1: Test single return value with loop-local variable**

```go
func TestGoCFGToStructured_LoopReturnLocalVar(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(items []int) int {
				for _, v := range items {
					if v < 0 { return v }
				}
				return 0
			}")
		(define file (go-parse-string source))
		(define decl (car (cdr (assoc 'decls (cdr file)))))
		(define body (cdr (assoc 'body (cdr decl))))
		(define ftype (cdr (assoc 'type (cdr decl))))
		(define result (go-cfg-to-structured body ftype))`)

	t.Run("has result variable", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "var _r0 int"), qt.IsTrue,
			qt.Commentf("expected _r0 declaration, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "_r0 = v"), qt.IsTrue,
			qt.Commentf("expected _r0 assignment, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "return _r0"), qt.IsTrue,
			qt.Commentf("expected return _r0 in guard, got:\n%s", s))
	})
}
```

**Step 2: Test multiple return values**

```go
func TestGoCFGToStructured_LoopMultiReturn(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(items []int) (int, error) {
				for _, v := range items {
					if v < 0 { return v, errNeg }
				}
				return 0, nil
			}")
		(define file (go-parse-string source))
		(define decl (car (cdr (assoc 'decls (cdr file)))))
		(define body (cdr (assoc 'body (cdr decl))))
		(define ftype (cdr (assoc 'type (cdr decl))))
		(define result (go-cfg-to-structured body ftype))`)

	t.Run("has two result variables", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "var _r0 int"), qt.IsTrue,
			qt.Commentf("expected _r0 int, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "var _r1 error"), qt.IsTrue,
			qt.Commentf("expected _r1 error, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "return _r0, _r1"), qt.IsTrue,
			qt.Commentf("expected return _r0, _r1 in guard, got:\n%s", s))
	})
}
```

**Step 3: Test backward compatibility (no func-type gives same behavior)**

```go
func TestGoCFGToStructured_LoopReturnNoFuncType(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(items []int) error {
				for _, v := range items {
					if v < 0 { return errNeg }
				}
				return nil
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("still works without func-type", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "errNeg"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "_r0"), qt.IsFalse,
			qt.Commentf("should not synthesize _r vars without func-type, got:\n%s", s))
	})
}
```

**Step 4: Run all tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestGoCFGToStructured -v`
Expected: all tests pass

**Step 5: Commit**

```
test(restructure): multiple return values with func-type
```

---

### Task 7: Forward goto elimination — detection

**Files:**
- Modify: `goast/prim_restructure.go`

**Step 1: Add goto classification types and `classifyGotos`**

```go
type gotoClass int

const (
	gotoNone        gotoClass = iota // no gotos
	gotoForwardOnly                  // all gotos jump to labels later in same block
	gotoBackwardOnly                 // all gotos jump to labels earlier in same block
	gotoMixed                        // mix of forward and backward
	gotoCrossBranch                  // goto into/out of if/switch branches
)

// classifyGotos analyzes goto statements in a block.
func classifyGotos(block *ast.BlockStmt) gotoClass {
	// Collect label positions (index in top-level statement list).
	labelPos := map[string]int{}
	for i, stmt := range block.List {
		if ls, ok := stmt.(*ast.LabeledStmt); ok {
			labelPos[ls.Label.Name] = i
		}
	}
	if len(labelPos) == 0 {
		return gotoNone
	}

	// Collect goto targets with their source position.
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
	if len(gotos) == 0 {
		return gotoNone
	}

	hasForward := false
	hasBackward := false
	for _, g := range gotos {
		target, ok := labelPos[g.label]
		if !ok {
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
```

**Step 2: Update `PrimGoCFGToStructured` to use `classifyGotos`**

Replace:
```go
	if containsGoto(block) {
		mc.SetValue(values.FalseValue)
		return nil
	}
```

With:
```go
	gc := classifyGotos(block)
	if gc == gotoCrossBranch {
		mc.SetValue(values.FalseValue)
		return nil
	}
	// Temporary: still bail on all gotos until forward/backward are implemented.
	if gc != gotoNone {
		mc.SetValue(values.FalseValue)
		return nil
	}
```

**Step 3: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestGoCFGToStructured -v`
Expected: all pass (behavior unchanged)

**Step 4: Commit**

```
refactor(restructure): classify gotos instead of blanket rejection
```

---

### Task 8: Forward goto elimination — restructuring

**Files:**
- Modify: `goast/prim_restructure.go`

**Step 1: Implement forward goto helpers**

```go
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
```

**Step 2: Implement `restructureForwardGotos`**

```go
// restructureForwardGotos rewrites forward gotos in a statement list.
// Pattern: if cond { goto L } ... L: stmt
// Becomes: if !cond { ... } stmt
func restructureForwardGotos(stmts []ast.Stmt) []ast.Stmt {
	result := make([]ast.Stmt, len(stmts))
	copy(result, stmts)

	for i := len(result) - 1; i >= 0; i-- {
		// Rebuild label index each pass (positions shift).
		labelPos := map[string]int{}
		for j, s := range result {
			if ls, ok := s.(*ast.LabeledStmt); ok {
				labelPos[ls.Label.Name] = j
			}
		}

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

		// Unwrap label at targetIdx.
		var labelStmt ast.Stmt
		if ls, ok := result[targetIdx].(*ast.LabeledStmt); ok {
			labelStmt = ls.Stmt
		} else {
			labelStmt = result[targetIdx]
		}

		// Rebuild: result[:i] + wrapped + labelStmt + result[targetIdx+1:]
		var newStmts []ast.Stmt
		newStmts = append(newStmts, result[:i]...)
		newStmts = append(newStmts, wrapped)
		newStmts = append(newStmts, labelStmt)
		newStmts = append(newStmts, result[targetIdx+1:]...)
		result = newStmts
	}

	return result
}
```

**Step 3: Wire into `PrimGoCFGToStructured`**

Remove the temporary bail-out for forward gotos. Add a new phase before loop
rewriting:

```go
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

	// Phase 1: Rewrite returns inside loops (Case 2).
	// ... existing code ...
```

**Step 4: Run build and tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build ./goast/... && go test ./goast/ -run TestGoCFGToStructured -v`
Expected: compile success, all tests pass

**Step 5: Commit**

```
feat(restructure): forward goto elimination via condition negation
```

---

### Task 9: Forward goto — tests

**Files:**
- Modify: `goast/restructure_test.go`

**Step 1: Update `GotoReturnsFalse` to test forward goto success**

Replace the existing test:

```go
func TestGoCFGToStructured_ForwardGoto(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(x int) {
				if x > 0 { goto end }
				println(x)
				end:
				println(0)
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("eliminates goto", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "goto"), qt.IsFalse,
			qt.Commentf("should eliminate goto, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "println(x)"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "println(0)"), qt.IsTrue)
	})
}
```

**Step 2: Add test for multiple forward gotos to same label**

```go
func TestGoCFGToStructured_MultipleForwardGotos(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(x, y int) {
				if x > 0 { goto cleanup }
				if y > 0 { goto cleanup }
				work()
				cleanup:
				close()
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("nests conditions", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "goto"), qt.IsFalse,
			qt.Commentf("should eliminate goto, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "work()"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "close()"), qt.IsTrue)
	})
}
```

**Step 3: Add test for cross-branch goto (still returns #f)**

```go
func TestGoCFGToStructured_CrossBranchGoto(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f(x int) {
				goto inner
				if x > 0 {
					inner:
					println(x)
				}
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("returns false", func(t *testing.T) {
		result := eval(t, engine, `result`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.FalseValue)
	})
}
```

**Step 4: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestGoCFGToStructured -v`
Expected: all pass

**Step 5: Commit**

```
test(restructure): forward goto elimination
```

---

### Task 10: Backward goto elimination

**Files:**
- Modify: `goast/prim_restructure.go`

**Step 1: Implement `restructureBackwardGotos`**

```go
// restructureBackwardGotos rewrites backward gotos as for-loops.
// Pattern: label: stmt ... if cond { goto label }
// Becomes: for { stmt ... if !cond { break } }
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
		for j := i + 1; j < len(stmts); j++ {
			if ifHasGoto(stmts[j], label) {
				gotoIdx = j
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

		// Replace goto-if with break-if (negated condition).
		gotoIf := stmts[gotoIdx].(*ast.IfStmt)
		loopBody = append(loopBody, &ast.IfStmt{
			Cond: negateExpr(gotoIf.Cond),
			Body: &ast.BlockStmt{List: []ast.Stmt{makeBreakStmt()}},
		})

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
```

**Step 2: Wire into `PrimGoCFGToStructured`**

Replace the backward goto TODO. Ensure backward runs BEFORE forward when `gotoMixed`:

```go
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
```

**Step 3: Run build and tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build ./goast/... && go test ./goast/ -run TestGoCFGToStructured -v`
Expected: compile success, all tests pass

**Step 4: Commit**

```
feat(restructure): backward goto elimination as for-loops
```

---

### Task 11: Backward and mixed goto — tests

**Files:**
- Modify: `goast/restructure_test.go`

**Step 1: Test backward goto (do-while pattern)**

```go
func TestGoCFGToStructured_BackwardGoto(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f() {
				start:
				work()
				if shouldContinue() { goto start }
				cleanup()
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("becomes for loop", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "goto"), qt.IsFalse,
			qt.Commentf("should eliminate goto, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "for {"), qt.IsTrue,
			qt.Commentf("expected for loop, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "work()"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "break"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "cleanup()"), qt.IsTrue)
	})
}
```

**Step 2: Test mixed forward + backward goto**

```go
func TestGoCFGToStructured_MixedGoto(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f() int {
				retry:
				result := try()
				if result == nil { goto retry }
				if result.err != nil { goto cleanup }
				process(result)
				cleanup:
				return close()
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("eliminates both gotos", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "goto"), qt.IsFalse,
			qt.Commentf("should eliminate both gotos, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "for {"), qt.IsTrue)
		qt.New(t).Assert(strings.Contains(s, "close()"), qt.IsTrue)
	})
}
```

**Step 3: Run all tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestGoCFGToStructured -v`
Expected: all pass

**Step 4: Commit**

```
test(restructure): backward and mixed goto elimination
```

---

### Task 12: Full test suite + documentation

**Step 1: Run full CI**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: lint + build + test + covercheck + verify-mod all pass

**Step 2: Fix any coverage gaps**

If coverage drops below 80%, add targeted tests for uncovered branches.

**Step 3: Update TODO.md — mark B3 done**

```markdown
### B3. go-cfg-to-structured improvements (depends on B2) — DONE

- [x] Handle goto / labeled branches (forward and backward; cross-branch returns #f)
- [x] Handle switch/select with early returns inside loops (labeled break)
- [x] Handle multiple return values (_r0, _r1, ...)
```

**Step 4: Update CLAUDE.md**

Update the `go-cfg-to-structured` entry to reflect expanded capabilities and
the optional `func-type` argument.

**Step 5: Update docs/PRIMITIVES.md**

Document the new optional second argument and the four restructuring phases.

**Step 6: Commit**

```
chore: complete B3 restructuring improvements — docs and TODO
```
