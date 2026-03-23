# SSA Field Index Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `go-ssa-field-index` Go primitive that returns pre-correlated field access data per function, eliminating the 4-minute Scheme-side SSA tree walking bottleneck.

**Architecture:** New Go function `PrimGoSSAFieldIndex` in `goastssa/prim_ssa.go` iterates `[]ssa.Instruction` natively, correlates `FieldAddr`+`Store`+`Field` instructions, and returns a flat s-expression index. The belief DSL's `stores-to-fields` and `co-mutated` are rewritten to filter this index instead of walking SSA trees.

**Tech Stack:** Go (`golang.org/x/tools/go/ssa`, `go/types`), Scheme (belief DSL), existing `goastssa` helpers (`typesDeref`, `fieldNameAt`, `valName`)

**Design doc:** `plans/SSA-FIELD-INDEX.md`

---

### Task 1: Register the new primitive (stub)

Create the primitive registration and a stub implementation that returns an empty list.

**Files:**
- Modify: `goastssa/register.go:40-47`
- Modify: `goastssa/prim_ssa.go` (append new function)

**Step 1: Add registration**

In `goastssa/register.go`, add a second entry to the `PrimitiveSpec` slice in `addPrimitives` (line 41-45):

```go
func addPrimitives(r *registry.Registry) error {
	r.AddPrimitives([]registry.PrimitiveSpec{
		{Name: "go-ssa-build", ParamCount: 2, IsVariadic: true, Impl: PrimGoSSABuild,
			Doc:        "Builds SSA for a Go package and returns a list of ssa-func nodes.",
			ParamNames: []string{"pattern", "options"}, Category: "goast-ssa"},
		{Name: "go-ssa-field-index", ParamCount: 1, Impl: PrimGoSSAFieldIndex,
			Doc:        "Returns per-function field access summaries for a Go package.",
			ParamNames: []string{"pattern"}, Category: "goast-ssa"},
	}, registry.PhaseRuntime)
	return nil
}
```

**Step 2: Add stub implementation**

Append to `goastssa/prim_ssa.go`:

```go
// PrimGoSSAFieldIndex implements (go-ssa-field-index pattern).
// Returns a list of ssa-field-summary nodes with per-function field
// access data (struct type, field name, receiver, read/write mode).
func PrimGoSSAFieldIndex(mc *machine.MachineContext) error {
	pattern, err := helpers.RequireArg[*values.String](mc, 0, werr.ErrNotAString, "go-ssa-field-index")
	if err != nil {
		return err
	}

	err = security.CheckWithAuthorizer(mc.Authorizer(), security.AccessRequest{
		Resource: security.ResourceProcess,
		Action:   security.ActionLoad,
		Target:   "go",
	})
	if err != nil {
		return err
	}

	_ = pattern // TODO: implement
	mc.SetValue(values.EmptyList)
	return nil
}
```

**Step 3: Run tests to verify compilation**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goastssa/ -run TestExtension -v`
Expected: PASS

**Step 4: Commit**

```bash
git add goastssa/register.go goastssa/prim_ssa.go
git commit -m "add: go-ssa-field-index primitive stub"
```

---

### Task 2: Write the test for the Go primitive

Write tests that call `go-ssa-field-index` on the `goastssa` package and verify the return shape.

**Files:**
- Modify: `goastssa/prim_ssa_test.go` (append tests)

**Step 1: Write the tests**

```go
func TestSSAFieldIndex(t *testing.T) {
	engine := newEngine(t)

	// go-ssa-field-index should return a list of ssa-field-summary nodes.
	result := eval(t, engine, `
		(import (wile goast ssa))
		(let ((index (go-ssa-field-index "github.com/aalpar/wile-goast/goastssa")))
		  (and (pair? index)
		       (let ((first (car index)))
		         (and (pair? first)
		              (eq? (car first) 'ssa-field-summary)))))
	`)
	c := qt.New(t)
	c.Assert(result, qt.Equals, values.TrueValue)
}

func TestSSAFieldIndexContent(t *testing.T) {
	engine := newEngine(t)

	// Each entry should have func, pkg, and fields keys.
	result := eval(t, engine, `
		(import (wile goast ssa))
		(let* ((index (go-ssa-field-index "github.com/aalpar/wile-goast/goastssa"))
		       (first (car index))
		       (fields (cdr first)))
		  (and (assoc 'func fields)
		       (assoc 'pkg fields)
		       (assoc 'fields fields)
		       #t))
	`)
	c := qt.New(t)
	c.Assert(result, qt.Equals, values.TrueValue)
}

func TestSSAFieldIndexAccessMode(t *testing.T) {
	engine := newEngine(t)

	// Field access entries should have struct, field, recv, mode keys.
	// Mode should be read or write.
	result := eval(t, engine, `
		(import (wile goast ssa))
		(let* ((index (go-ssa-field-index "github.com/aalpar/wile-goast/goastssa"))
		       (entry (let loop ((idx index))
		                (if (null? idx) #f
		                  (let ((fs (cdr (assoc 'fields (cdr (car idx))))))
		                    (if (pair? fs) (car idx) (loop (cdr idx)))))))
		       (access (car (cdr (assoc 'fields (cdr entry))))))
		  (and (assoc 'struct (cdr access))
		       (assoc 'field (cdr access))
		       (assoc 'recv (cdr access))
		       (assoc 'mode (cdr access))
		       (let ((m (cdr (assoc 'mode (cdr access)))))
		         (or (eq? m 'read) (eq? m 'write)))))
	`)
	c := qt.New(t)
	c.Assert(result, qt.Equals, values.TrueValue)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goastssa/ -run TestSSAFieldIndex -v`
Expected: FAIL (stub returns empty list)

**Step 3: Commit**

```bash
git add goastssa/prim_ssa_test.go
git commit -m "test: go-ssa-field-index shape and content tests"
```

---

### Task 3: Implement the Go primitive

Replace the stub with the full implementation.

**Files:**
- Modify: `goastssa/prim_ssa.go` (replace stub)

**Step 1: Check if `goast.Sym` exists**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && grep -n 'func Sym' goast/helpers.go`

If it doesn't exist, use `values.NewSymbol("read")` instead.

**Step 2: Implement**

Replace the stub `PrimGoSSAFieldIndex` with the full implementation. The implementation:

1. Loads packages and builds SSA (same pattern as `PrimGoSSABuild`)
2. Calls `collectFieldSummaries` for each SSA package
3. `collectFieldSummaries` iterates package-level functions and methods (same iteration pattern as `PrimGoSSABuild` lines 131-167)
4. `buildFuncSummary` does one pass per function:
   - Collects `*ssa.FieldAddr` instructions into a map (register name -> field info)
   - Collects `*ssa.Store` instruction addr register names into a set
   - Collects `*ssa.Field` instructions as direct reads
   - Correlates: FieldAddr register in store set = write, otherwise read
5. Returns `nil` for functions with no field accesses (filtered out)
6. `structTypeName` helper extracts struct name and package from `types.Type` via `(*types.Named).Obj()`

Key: reuse existing helpers `typesDeref` (mapper.go:328) and `fieldNameAt` (mapper.go:337). Add `structTypeName` as a new helper.

See the complete implementation in the design doc (`plans/SSA-FIELD-INDEX.md`, "Go Implementation" section) for the full code.

**Step 3: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goastssa/ -run TestSSAFieldIndex -v`
Expected: ALL PASS

**Step 4: Run full goastssa tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goastssa/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add goastssa/prim_ssa.go
git commit -m "add: go-ssa-field-index implementation"
```

---

### Task 4: Add `ctx-field-index` to the belief DSL

Add the context slot, accessor, and Scheme-side helper functions.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm` (lines 57-63 for context, plus new functions)
- Modify: `cmd/wile-goast/lib/wile/goast/belief.sld` (exports)

**Step 1: Add `field-index` slot to `make-context` (line 57-63)**

```scheme
(define (make-context target)
  (list (cons 'target target)
        (cons 'pkgs #f)
        (cons 'ssa #f)
        (cons 'ssa-index #f)
        (cons 'field-index #f)
        (cons 'callgraph #f)
        (cons 'results '())))
```

**Step 2: Add accessor and helpers after `ctx-callgraph` (~line 89)**

```scheme
(define (ctx-field-index ctx)
  (or (ctx-ref ctx 'field-index)
      (let ((idx (go-ssa-field-index (ctx-target ctx))))
        (ctx-set! ctx 'field-index idx)
        idx)))

;; Find the field summary for a function by package path and name.
(define (find-field-summary index pkg-path func-name)
  (let loop ((entries (if (pair? index) index '())))
    (if (null? entries) #f
      (let ((entry (car entries)))
        (if (and (equal? (nf entry 'func) func-name)
                 (equal? (nf entry 'pkg) pkg-path))
          entry
          (loop (cdr entries)))))))

;; Extract written field names for a given struct from a summary.
;; If struct-name is #f, returns writes for all structs.
(define (writes-for-struct summary struct-name)
  (let ((fields (nf summary 'fields)))
    (if (not (pair? fields)) '()
      (filter-map
        (lambda (access)
          (and (eq? (nf access 'mode) 'write)
               (or (not struct-name)
                   (equal? (nf access 'struct) struct-name))
               (nf access 'field)))
        fields))))

;; Check that all names in required are present in available.
(define (all-present? required available)
  (let loop ((rs required))
    (cond ((null? rs) #t)
          ((member? (car rs) available) (loop (cdr rs)))
          (else #f))))
```

**Step 3: Export `ctx-field-index` in `belief.sld`**

Change the context exports line to:

```scheme
    make-context ctx-pkgs ctx-ssa ctx-callgraph ctx-find-ssa-func ctx-field-index
```

**Step 4: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBelief -v`
Expected: ALL PASS (no consumers use the new slot yet)

**Step 5: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/belief.scm cmd/wile-goast/lib/wile/goast/belief.sld
git commit -m "add: ctx-field-index and helpers in belief DSL"
```

---

### Task 5: Rewrite `stores-to-fields` and `co-mutated`

Replace both functions to use the field index instead of SSA tree walking.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm` (two function replacements)

**Step 1: Replace `stores-to-fields` (lines ~396-418)**

```scheme
;; (stores-to-fields struct-name field ...) — SSA: function stores to
;; these fields. Uses the pre-built field index from Go.
(define (stores-to-fields struct-name . field-names)
  (lambda (fn ctx)
    (let* ((fname (nf fn 'name))
           (pkg-path (nf fn 'pkg-path))
           (summary (find-field-summary (ctx-field-index ctx) pkg-path fname)))
      (and summary
           (let ((writes (writes-for-struct summary struct-name)))
             (all-present? field-names writes))))))
```

**Step 2: Replace `co-mutated` (lines ~593-621)**

```scheme
;; (co-mutated field ...) — checks whether all named fields are stored
;; together in the function. Uses the pre-built field index from Go.
;; Returns: 'co-mutated or 'partial
(define (co-mutated . field-names)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (summary (find-field-summary (ctx-field-index ctx) pkg-path fname)))
      (if (not summary) 'partial
        (let ((writes (writes-for-struct summary #f)))
          (if (all-present? field-names writes)
            'co-mutated
            'partial))))))
```

**Step 3: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBelief -v`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/belief.scm
git commit -m "refactor: stores-to-fields and co-mutated use field index"
```

---

### Task 6: Remove parallel-filter-map and threads dependency

The parallel infrastructure was a workaround for the Scheme-side bottleneck. With the Go primitive, it's unnecessary.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm` (remove parallel code, revert functions-matching)
- Modify: `cmd/wile-goast/main.go` (remove threads import)
- Modify: `goast/belief_integration_test.go` (remove threads import)

**Step 1: Remove parallel code from belief.scm**

Remove `*parallel-threads*`, `partition-list`, `*pfm-mutex*`, and `parallel-filter-map` (the block starting around line 325).

**Step 2: Revert `functions-matching` to sequential**

```scheme
(define (functions-matching . preds)
  (lambda (ctx)
    (let ((funcs (all-func-decls (ctx-pkgs ctx))))
      (filter-map
        (lambda (fn)
          (and (let loop ((ps preds))
                 (cond ((null? ps) #t)
                       (((car ps) fn ctx) (loop (cdr ps)))
                       (else #f)))
               fn))
        funcs))))
```

**Step 3: Remove threads from main.go and test**

Remove the `"github.com/aalpar/wile/extensions/threads"` import and `wile.WithExtension(threads.Extension)` from both `cmd/wile-goast/main.go` and `goast/belief_integration_test.go`.

**Step 4: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBelief -v && go test ./...`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/belief.scm cmd/wile-goast/main.go goast/belief_integration_test.go
git commit -m "remove: parallel-filter-map and threads dependency"
```

---

### Task 7: Kubelet validation and CI

**Step 1: Build**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make build`

**Step 2: Run kubelet co-mutation beliefs**

Run: `cd /Users/aalpar/projects/kubernetes && time /path/to/dist/wile-goast -f /path/to/examples/goast-query/k8s-kubelet-comutation.scm`

Expected: Same results (4/5 co-mutated, deviation in `convertToKubeContainerStatus`), under 10 seconds.

**Step 3: Run kubelet AST-only beliefs**

Run: `cd /Users/aalpar/projects/kubernetes && time /path/to/dist/wile-goast -f /path/to/examples/goast-query/k8s-kubelet-beliefs.scm`

Expected: Same results as before, unaffected.

**Step 4: Full CI**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: ALL PASS
