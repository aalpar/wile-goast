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

**Step 3: Run tests to verify helpers compile**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build ./goast/...`
Expected: compile success (helpers are unused but that's fine — they'll be wired in Task 2)

**Step 4: Commit**

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

Update the `BlockStmt` case to call `replaceReturnsInStmtsLabeled` directly (not the
wrapper) so that `loopLabel` is threaded through nested blocks:

```go
		case *ast.BlockStmt:
			newList, ok := replaceReturnsInStmtsLabeled(s.List, ctlName, retIdx, collected, loopLabel)
			if !ok {
				return nil, false
			}
			result = append(result, &ast.BlockStmt{List: newList})
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

Replace the full function with the updated signature. All recursive calls must
thread `loopLabel` — both the body processing and both else branches:

```go
func replaceReturnsInIf(
	ifStmt *ast.IfStmt,
	ctlName string,
	retIdx *int,
	collected *[]*ast.ReturnStmt,
	loopLabel string,
) (*ast.IfStmt, bool) {
	newBodyList, ok := replaceReturnsInStmtsLabeled(ifStmt.Body.List, ctlName, retIdx, collected, loopLabel)
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
			newList, ok := replaceReturnsInStmtsLabeled(e.List, ctlName, retIdx, collected, loopLabel)
			if !ok {
				return nil, false
			}
			newIf.Else = &ast.BlockStmt{List: newList}
		case *ast.IfStmt:
			newElse, ok := replaceReturnsInIf(e, ctlName, retIdx, collected, loopLabel)
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
```

Update the `IfStmt` case in `replaceReturnsInStmtsLabeled` to pass `loopLabel`:

```go
		case *ast.IfStmt:
			newIf, ok := replaceReturnsInIf(s, ctlName, retIdx, collected, loopLabel)
```

**Step 3a: Add `replaceReturnsInClauses` for switch/select case bodies**

This function can now reference `replaceReturnsInStmtsLabeled` which exists in this task.
The `default` case rejects any non-clause statement that contains a return — this
prevents silent passthrough of return-containing statements.

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
			if bodyContainsReturn(&ast.BlockStmt{List: []ast.Stmt{stmt}}) {
				return nil, false
			}
			result = append(result, stmt)
		}
	}
	return result, true
}
```

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

	eval(t, engine, `
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
		result := eval(t, engine, `(eq? (car result) 'block)`)
		qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
	})

	t.Run("has labeled break", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "_loop0"), qt.IsTrue,
			qt.Commentf("expected loop label, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "break _loop0"), qt.IsTrue,
			qt.Commentf("expected labeled break, got:\n%s", s))
	})

	t.Run("has guard after loop", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "return -1"), qt.IsTrue,
			qt.Commentf("expected return in guard, got:\n%s", s))
	})
}
```

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

In `register.go`, change the `go-cfg-to-structured` entry. Use `"rest"` for the
variadic param name, matching the convention in `go-load` and `go-list-deps`:

```go
		{Name: "go-cfg-to-structured", ParamCount: 2, IsVariadic: true, Impl: PrimGoCFGToStructured,
			Doc:        "Restructures a block with early returns into a single-exit if/else tree. Optional second arg: func-type for result variable synthesis.",
			ParamNames: []string{"block", "rest"}, Category: "goast"},
```

**Step 2: Extract result types from optional func-type in `PrimGoCFGToStructured`**

At the top of `PrimGoCFGToStructured`, after parsing the block, extract result types
from the optional rest-args. The variadic arg arrives as a `values.Tuple`; walk it
for the first element (matching the `go-load` / `go-list-deps` convention):

```go
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
callees: `replaceReturnsInIf`, `replaceReturnsInClauses`). Also update the
`BlockStmt` case to thread `resultVarCount` through recursive calls. When > 0,
emit assignments before the ctl assignment.

Guard against mismatched result counts — naked returns (zero results) and
multi-valued function call returns (`return f()` where `f()` returns multiple
values) cannot be rewritten safely:

```go
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
//
// Assumption: classification is sound for flat function bodies where gotos
// and their target labels are top-level statements. A goto nested inside
// an if/switch at statement index N uses N as its source position (structural
// index, not execution order). Gotos targeting labels not in the top-level
// list are classified as gotoCrossBranch (conservative).
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

Uses a fixpoint loop: each pass rewrites one forward goto, then repeats until
no more are found. This correctly handles multiple gotos targeting the same
label — the first pass wraps statements up to the label, the second pass
finds the remaining goto (now inside the wrapped block or still top-level)
and handles it in the next iteration. Label stripping only happens when no
more gotos target that label.

```go
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
```

**Step 3: Wire into `PrimGoCFGToStructured`**

Remove the temporary bail-out for forward gotos. Add a new phase before loop
rewriting, with post-restructuring validation that catches any gotos that
survived the pattern matchers:

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

	// Post-restructuring validation: if any gotos survived the pattern
	// matchers (bare gotos, multi-statement if-bodies, nested gotos),
	// bail honestly rather than returning an AST that still has gotos.
	if gc != gotoNone && containsGoto(&ast.BlockStmt{List: stmts}) {
		mc.SetValue(values.FalseValue)
		return nil
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
			gotoIf, _ := stmts[gotoIdx].(*ast.IfStmt)
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
```

**Step 2: Wire into `PrimGoCFGToStructured`**

Replace the backward goto TODO. Ensure backward runs BEFORE forward when `gotoMixed`.
Ordering invariant: backward rewriting consumes backward gotos (converting them to
`for` loops) and cannot introduce new forward gotos. So the initial classification
remains valid for Phase 0b.

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

	// Post-restructuring validation: if any gotos survived, bail.
	if gc != gotoNone && containsGoto(&ast.BlockStmt{List: stmts}) {
		mc.SetValue(values.FalseValue)
		return nil
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

**Step 2: Test bare unconditional backward goto (infinite loop)**

```go
func TestGoCFGToStructured_BareBackwardGoto(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(define source "
			package p
			func f() {
				start:
				work()
				goto start
			}")
		(define file (go-parse-string source))
		(define decls (cdr (assoc 'decls (cdr file))))
		(define body (cdr (assoc 'body (cdr (car decls)))))
		(define result (go-cfg-to-structured body))`)

	t.Run("becomes infinite for loop", func(t *testing.T) {
		result := eval(t, engine, `(go-format result)`)
		s := result.Internal().(*values.String).Value
		qt.New(t).Assert(strings.Contains(s, "goto"), qt.IsFalse,
			qt.Commentf("should eliminate goto, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "for {"), qt.IsTrue,
			qt.Commentf("expected for loop, got:\n%s", s))
		qt.New(t).Assert(strings.Contains(s, "work()"), qt.IsTrue)
		// No break — this is an intentional infinite loop.
		qt.New(t).Assert(strings.Contains(s, "break"), qt.IsFalse,
			qt.Commentf("should not have break in infinite loop, got:\n%s", s))
	})
}
```

**Step 3: Test mixed forward + backward goto**

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

**Step 4: Run all tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestGoCFGToStructured -v`
Expected: all pass

**Step 5: Commit**

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
