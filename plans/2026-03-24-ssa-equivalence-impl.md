# SSA Equivalence Pass — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add SSA-level comparison as a refinement pass on AST unification candidates, producing "unifiable" verdicts when all remaining differences are type substitutions.

**Architecture:** Go primitive for block/register canonicalization (`go-ssa-canonicalize`), Scheme library for extensible algebraic normalization (`(wile goast ssa-normalize)`), shared diff/scoring library (`(wile goast unify)`), validation script with three test cases.

**Tech Stack:** Go (values.Value s-expression manipulation), R7RS Scheme (ast-transform rules), existing goast helpers (Node, Field, GetField, ValueList).

---

### Task 1: `go-ssa-canonicalize` — s-expression field helpers

The canonicalizer works on `values.Value` s-expressions (not raw Go SSA). It needs to extract and rebuild SSA s-expression alists. The existing `goast.GetField` and `goast.Node` helpers work for this. This task builds the Go functions that parse an SSA function s-expression into an intermediate Go struct, making subsequent canonicalization code readable.

**Files:**
- Create: `goastssa/canonicalize.go`
- Test: `goastssa/canonicalize_test.go`

**Step 1: Write the failing test**

Create `goastssa/canonicalize_test.go` with a test that builds SSA for a simple two-block function and calls `go-ssa-canonicalize` on it. The primitive doesn't exist yet, so it should fail.

```go
package goastssa_test

import (
	"testing"

	"github.com/aalpar/wile/values"

	qt "github.com/frankban/quicktest"
)

func TestGoSSACanonicalize_ReturnsSSAFunc(t *testing.T) {
	engine := newEngine(t)

	// Build SSA, grab one function, canonicalize it.
	evl(t, engine, `(define funcs (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)
	evl(t, engine, `(define fn (car funcs))`)
	result := evl(t, engine, `(eq? (car (go-ssa-canonicalize fn)) 'ssa-func)`)
	qt.New(t).Assert(result.Internal(), qt.Equals, values.TrueValue)
}
```

Note: uses the existing test helper `eval` defined in `goastssa/prim_ssa_test.go:39`. References as `eval` throughout — DO NOT create duplicates.

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goastssa/ -run TestGoSSACanonicalize_ReturnsSSAFunc -v`
Expected: FAIL — `go-ssa-canonicalize` is not defined.

**Step 3: Create `canonicalize.go` with the primitive stub and s-expression parsing**

Create `goastssa/canonicalize.go`. This file contains:
- `PrimGoSSACanonicalize` — the primitive entry point
- `parseSSAFunc` — extracts blocks, params, name, signature from the s-expression into a Go struct
- `parseSSABlock` — extracts index, idom, preds, succs, instrs from a block s-expression
- `rebuildSSAFunc` — converts the Go struct back to an s-expression

For this step, implement the parse/rebuild round-trip (canonicalize = identity). The actual canonicalization logic comes in Tasks 2 and 3.

Key types:

```go
type ssaFuncData struct {
	name      string
	signature string
	pkg       string // may be empty
	params    []ssaParamData
	freeVars  values.Value // kept opaque
	blocks    []ssaBlockData
}

type ssaParamData struct {
	name     string
	typStr   string
	original values.Value
}

type ssaBlockData struct {
	index   int64
	idom    int64 // -1 if none (entry block)
	preds   []int64
	succs   []int64
	comment string
	instrs  []values.Value // kept opaque for now
}
```

The primitive validates the input is `(ssa-func ...)`, parses into `ssaFuncData`, calls `canonicalizeBlockOrder` and `renameRegisters` (both stubs), and rebuilds.

Use `goast.GetField`, `goast.RequireString`, `goast.Node`, `goast.Field`, `goast.ValueList` for s-expression access and construction.

Add a `replaceField` helper that rebuilds a tagged alist with one field value replaced — needed by Tasks 2 and 3.

Error sentinel: `errSSACanonicalizeError = werr.NewStaticError("ssa canonicalize error")`

**Step 4: Register the primitive**

Add to `goastssa/register.go` in the `addPrimitives` function:

```go
{Name: "go-ssa-canonicalize", ParamCount: 1, Impl: PrimGoSSACanonicalize,
	Doc:        "Canonicalizes an SSA function s-expression: dominator-order blocks, alpha-renamed registers.",
	ParamNames: []string{"ssa-func"}, Category: "goast-ssa"},
```

**Step 5: Run test to verify it passes**

Run: `go test ./goastssa/ -run TestGoSSACanonicalize_ReturnsSSAFunc -v`
Expected: PASS — round-trip parse/rebuild returns an ssa-func-tagged node.

**Step 6: Commit**

```
feat(ssa): go-ssa-canonicalize stub with s-expression parse/rebuild

Identity transform: parses ssa-func s-expression into Go structs,
rebuilds without modification. Block canonicalization and register
renaming are stubs for subsequent tasks.
```

---

### Task 2: Block canonicalization — dominator tree pre-order

Implement `canonicalizeBlockOrder` to reorder blocks by pre-order DFS of the dominator tree and reindex all cross-references.

**Files:**
- Modify: `goastssa/canonicalize.go` (replace stub)
- Test: `goastssa/canonicalize_test.go`

**Step 1: Write the failing test**

Add a test that verifies block reordering. Build SSA for a function with known control flow, verify that after canonicalization block 0 is the entry, indices are sequential, and idom references use the new indices (parent index < child index in pre-order).

```go
func TestGoSSACanonicalize_BlockOrder(t *testing.T) {
	engine := newEngine(t)

	evl(t, engine, `(define funcs (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)
	evl(t, engine, `
		(define multi-fn
			(let loop ((fs funcs))
				(if (null? fs) #f
					(let* ((fn (car fs))
						   (blocks (cdr (assoc 'blocks (cdr fn)))))
						(if (> (length blocks) 2) fn (loop (cdr fs)))))))`)
	evl(t, engine, `(define canon (go-ssa-canonicalize multi-fn))`)

	t.Run("block 0 is entry (no idom)", func(t *testing.T) {
		// Entry block should have index 0 and no idom field.
	})

	t.Run("block indices are sequential", func(t *testing.T) {
		// Walk blocks, verify index == position in list.
	})

	t.Run("idom references use new indices", func(t *testing.T) {
		// Every non-entry block's idom should be < its own index
		// (dominator tree pre-order: parent before children).
	})
}
```

**Step 2: Run test to verify it fails**

Expected: FAIL — blocks not reordered (stub is identity).

**Step 3: Implement `canonicalizeBlockOrder`**

Algorithm:
1. Build dominator tree from `idom` fields: `children[parentIdx] = [...childIndices]`
2. Find entry block (idom == -1)
3. Pre-order DFS from entry → produces `order []int64` and `oldToNew map[int64]int64`
4. Build `blockByOldIdx` map
5. Reorder blocks, update: `index`, `idom`, `preds`, `succs`
6. Walk instructions, update block index references in: `ssa-phi` edges, `ssa-if` then/else, `ssa-jump` target

Implement helper functions:
- `reindexInstr(instr values.Value, oldToNew map[int64]int64) values.Value` — dispatches on tag
- `reindexPhi` — remaps block indices in `(block-index . register-name)` edge pairs
- `reindexIf` — remaps `then` and `else` integer fields
- `reindexJump` — remaps `target` integer field
- `replaceField(node *values.Pair, key string, newVal values.Value) values.Value` — rebuilds tagged alist with one field replaced (utility for reindex helpers)

**Step 4: Run test to verify it passes**

Run: `go test ./goastssa/ -run TestGoSSACanonicalize_BlockOrder -v`
Expected: PASS

**Step 5: Commit**

```
feat(ssa): block canonicalization via dominator tree pre-order

go-ssa-canonicalize reorders blocks by pre-order DFS of the
dominator tree and reindexes all cross-references (preds, succs,
idom, phi edges, jump/if targets).
```

---

### Task 3: Register alpha-renaming

Implement `renameRegisters` to assign canonical register names in first-use order within the canonical block sequence.

**Files:**
- Modify: `goastssa/canonicalize.go` (replace stub)
- Test: `goastssa/canonicalize_test.go`

**Step 1: Write the failing test**

```go
func TestGoSSACanonicalize_RegisterRenaming(t *testing.T) {
	engine := newEngine(t)

	evl(t, engine, `(define funcs (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)
	evl(t, engine, `(define fn (car funcs))`)
	evl(t, engine, `(define canon (go-ssa-canonicalize fn))`)

	t.Run("params are p0 p1 etc", func(t *testing.T) {
		// First param name should be "p0".
	})

	t.Run("instruction names start with r", func(t *testing.T) {
		// First named instruction in first block should be "r0".
	})
}
```

**Step 2: Run test to verify it fails**

Expected: FAIL — registers still have original SSA names.

**Step 3: Implement `renameRegisters`**

Algorithm:
1. Build `nameMap map[string]string`
2. Rename params: `p0`, `p1`, ... Update `ssaParamData.name` and rebuild `original` s-expression.
3. First pass over blocks (canonical order): collect all instruction `name` fields, assign `r0`, `r1`, ... to `nameMap`.
4. Second pass: apply `renameInstrStrings` to all instructions.

`renameInstrStrings(v values.Value, nameMap map[string]string) values.Value`:
- `*values.String`: if in nameMap, replace
- `*values.Pair`: recurse car and cdr, reuse if unchanged
- Other: return as-is

This handles all operand positions because register references are always strings in the SSA s-expression format, and the recursive walk hits every string in the tree.

**Step 4: Run test to verify it passes**

Run: `go test ./goastssa/ -run TestGoSSACanonicalize_RegisterRenaming -v`
Expected: PASS

**Step 5: Run all canonicalize tests**

Run: `go test ./goastssa/ -run TestGoSSACanonicalize -v`
Expected: All pass.

**Step 6: Commit**

```
feat(ssa): register alpha-renaming in go-ssa-canonicalize

Parameters become p0, p1, ...; instruction definitions become
r0, r1, ... in canonical block order. All operand references
updated via recursive string replacement.
```

---

### Task 4: Error handling and edge cases

**Files:**
- Modify: `goastssa/canonicalize_test.go`
- Modify: `goastssa/canonicalize.go` (if needed)

**Step 1: Write edge case tests**

- Wrong arg type (integer instead of s-expression) → error
- Wrong tag (not `ssa-func`) → error
- Single-block function → passes through cleanly

**Step 2: Run tests**

Run: `go test ./goastssa/ -run "TestGoSSACanonicalize" -v`
Expected: All pass.

**Step 3: Commit**

```
test(ssa): error handling and edge cases for go-ssa-canonicalize
```

---

### Task 5: `(wile goast ssa-normalize)` — library skeleton and commutative rule

**Files:**
- Create: `cmd/wile-goast/lib/wile/goast/ssa-normalize.sld`
- Create: `cmd/wile-goast/lib/wile/goast/ssa-normalize.scm`
- Test: `goast/ssa_normalize_test.go`

**Step 1: Write the failing test**

Tests use `newBeliefEngine(t)` (which sets up library paths for embedded Scheme libraries).

Test the commutative rule: an `ssa-binop` with `(x . "r5") (y . "r2")` and `(op . +)` should swap to `(x . "r2") (y . "r5")`. Also test that subtraction (non-commutative) preserves order.

**Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run TestSSANormalize -v`
Expected: FAIL — library not found.

**Step 3: Create the library files**

`cmd/wile-goast/lib/wile/goast/ssa-normalize.sld` — exports: `ssa-normalize`, `ssa-rule-set`, `ssa-rule-identity`, `ssa-rule-commutative`, `ssa-rule-annihilation`, `ssa-rule-double-neg`. Imports `(wile goast utils)`.

`cmd/wile-goast/lib/wile/goast/ssa-normalize.scm` — implements:
- `commutative-ops` list: `'(+ * & | ^ == !=)`
- `(ssa-rule-commutative)`: returns lambda that swaps `x`/`y` when `(string>? x y)` for commutative ops
- `(constant-zero? s)`: matches `"0"` or `"0:..."` strings
- `(constant-one? s)`: matches `"1"` or `"1:..."` strings
- `(ssa-rule-identity)`: `x + 0 → x`, `x * 1 → x`, etc.
- `(ssa-rule-annihilation)`: `x * 0 → 0`, `x & 0 → 0`
- `(ssa-rule-double-neg)`: placeholder returning `#f` (needs cross-instruction context)
- `(ssa-rule-set . rules)`: composes rules, first non-`#f` wins
- `default-rules`: identity + annihilation + commutative
- `ssa-normalize`: `case-lambda` — `(node)` uses default-rules, `(node rules)` uses custom

All rules are `(lambda (node) ...)` compatible with `ast-transform`.

**Step 4: Run test to verify it passes**

Run: `go test ./goast/ -run TestSSANormalize -v`
Expected: PASS

**Step 5: Commit**

```
feat(ssa): (wile goast ssa-normalize) library with algebraic rules

Commutative sort, identity elimination, and annihilation rules
for SSA binops. Extensible via ssa-rule-set and ast-transform.
```

---

### Task 6: Identity elimination tests

**Files:**
- Modify: `goast/ssa_normalize_test.go`

**Step 1: Write tests for identity and annihilation rules**

- `x + 0` becomes `"r1"` (the x operand string)
- `x * 1` becomes `"r3"`
- `x * 0` becomes `"0:int"`
- Non-binop node (e.g., `ssa-alloc`) passes through unchanged

**Step 2: Run tests**

Run: `go test ./goast/ -run TestSSANormalize -v`
Expected: All pass.

**Step 3: Commit**

```
test(ssa): identity elimination and annihilation rule tests
```

---

### Task 7: `(wile goast unify)` — shared diff/scoring library

Extract the reusable diff engine and scoring logic from `unify-detect-pkg.scm` into a library, adapted for SSA nodes.

**Files:**
- Create: `cmd/wile-goast/lib/wile/goast/unify.sld`
- Create: `cmd/wile-goast/lib/wile/goast/unify.scm`
- Test: `goast/unify_test.go`

**Step 1: Write the failing test**

Test `ssa-diff` on identical nodes (similarity 1.0) and on nodes differing only in type string (type-name diff detected).

**Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run TestUnify -v`
Expected: FAIL — library not found.

**Step 3: Create the library**

`cmd/wile-goast/lib/wile/goast/unify.sld` — exports: `ssa-diff`, `diff-result-similarity`, `diff-result-diffs`, `diff-result-shared`, `diff-result-diff-count`, `score-ssa-diffs`, `find-root-substitutions`, `collapse-diffs`, `unifiable?`

`cmd/wile-goast/lib/wile/goast/unify.scm` — adapted from `unify-detect-pkg.scm`:

Core diff engine (same algorithms, ~200 lines):
- `merge-results`, `shared-result`, `diff-result`
- `fields-diff`, `list-diff`, `ssa-diff` (dispatch)
- SSA-aware `classify-ssa-diff`: register fields → weight 0, type → weight 1, op → weight 2, call-target → weight 3

Result format: `(diff-result (shared . N) (diff-count . N) (entries . (...)))`
Accessors: `diff-result-similarity` computes `shared / (shared + diff-count)`

Scoring and substitution collapsing:
- `find-root-substitutions` — same algorithm as existing script
- `collapse-diffs` — reclassify derived type diffs
- `score-ssa-diffs` — effective similarity with derived promotion

Verdict:
- `(unifiable? result threshold)` — `#t` when effective similarity >= threshold AND all remaining diffs are type-name or derived-type

Key SSA fields classification:

```scheme
(define ssa-register-fields '(name))
(define ssa-type-fields '(type asserted-type))
(define ssa-operator-fields '(op))
(define ssa-target-fields '(func method))
(define ssa-structural-fields '(index preds succs idom then else target))
```

**Step 4: Run test to verify it passes**

Run: `go test ./goast/ -run TestUnify -v`
Expected: PASS

**Step 5: Commit**

```
feat(ssa): (wile goast unify) shared diff/scoring library

Extracted from unify-detect-pkg.scm with SSA-aware classification.
Provides ssa-diff, score-ssa-diffs, substitution collapsing, and
unifiable? verdict predicate.
```

---

### Task 8: Validation script — pncounter/gcounter test case

Write the validation script that runs the full pipeline on the crdt test case and prints the comparison table answering the plan's open question.

**Files:**
- Create: `examples/goast-query/testdata/pncounter/pncounter.go`
- Create: `examples/goast-query/testdata/gcounter/gcounter.go`
- Create: `examples/goast-query/testdata/identity/identity.go`
- Create: `examples/goast-query/ssa-unify-detect.scm`
- Test: manual run

**Step 1: Create testdata packages**

`go-ssa-build` requires real Go packages (it loads via `packages.Load`). Put the test Go code in `examples/goast-query/testdata/` as real packages.

`pncounter/pncounter.go` and `gcounter/gcounter.go` — same code from the existing `unify-detect.scm` inline sources, as proper `.go` files.

`identity/identity.go` — synthetic test case with two functions: one has `x + 0`, the other just uses `x`. Same structure otherwise.

**Step 2: Write the script**

`examples/goast-query/ssa-unify-detect.scm`:

1. Import `(wile goast utils)`, `(wile goast ssa-normalize)`, `(wile goast unify)`
2. Define helper to run the full pipeline on a pair of package paths + function names
3. For each test case:
   a. `go-typecheck-package` both packages → AST diff → AST similarity
   b. `go-ssa-build` both packages → find function by name → `go-ssa-canonicalize` → `ssa-normalize` → `ssa-diff` → SSA similarity + verdict
4. Print comparison table

The script must handle the SSA function lookup: `go-ssa-build` returns a flat list; find the function whose `name` field matches.

**Step 3: Run the script**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go run ./cmd/wile-goast -f examples/goast-query/ssa-unify-detect.scm`

Observe the output. Record AST sim, SSA sim, and verdict. This answers the open question.

**Step 4: Commit**

```
feat(ssa): validation script for SSA equivalence pass

Runs AST diff and SSA diff on pncounter/gcounter, comparing
similarity scores. Answers the open question from
plans/UNIFICATION-DETECTION.md.
```

---

### Task 9: Update plans and docs

**Files:**
- Modify: `plans/UNIFICATION-DETECTION.md` — add results section
- Modify: `docs/PRIMITIVES.md` — add `go-ssa-canonicalize` entry
- Modify: `CLAUDE.md` — add `ssa-normalize` and `unify` to library listing, add `go-ssa-canonicalize` to primitive table

**Step 1: Update UNIFICATION-DETECTION.md**

Add a "Results" section with the validation output. State whether SSA normalization adds value or not.

**Step 2: Update PRIMITIVES.md**

Add `go-ssa-canonicalize` with signature, description, and example.

**Step 3: Update CLAUDE.md**

Add the new Scheme libraries to the library listing. Add `go-ssa-canonicalize` to the goastssa primitive table.

**Step 4: Commit**

```
docs: SSA equivalence validation results and primitive reference
```

---

### Task 10: Full test suite verification

**Step 1: Run all tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: All pass, coverage >= 80%.

**Step 2: If coverage is below threshold, add targeted tests**

Focus on untested branches in `canonicalize.go` (edge cases, error paths).

**Step 3: Final commit if needed**

```
test: coverage for SSA equivalence pass
```
