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
	eval(t, engine, `(define funcs (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)
	eval(t, engine, `(define fn (car funcs))`)
	result := eval(t, engine, `(eq? (car (go-ssa-canonicalize fn)) 'ssa-func)`)
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

**Optional field handling:** The SSA mapper conditionally emits `idom` and `comment` — entry blocks have no `idom` field, and blocks without comments have no `comment` field.

- `parseSSABlock`: when `GetField` returns `found=false` for `idom`, store `-1`. When `comment` is absent, store `""`.
- `parseSSAFunc`: when `GetField` returns `found=false` for `pkg`, store `""`.
- `rebuildSSAFunc`: suppress `idom` when `-1`, `comment` when `""`, `pkg` when `""`. This preserves round-trip fidelity with the original s-expression.

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

	eval(t, engine, `(define funcs (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)
	eval(t, engine, `
		(define multi-fn
			(let loop ((fs funcs))
				(if (null? fs) #f
					(let* ((fn (car fs))
						   (blocks (cdr (assoc 'blocks (cdr fn)))))
						(if (> (length blocks) 2) fn (loop (cdr fs)))))))`)
	eval(t, engine, `(define canon (go-ssa-canonicalize multi-fn))`)

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
4. Drop unreachable blocks — any block not visited by the DFS is omitted from the result (dead code, irrelevant for comparison)
5. Build `blockByOldIdx` map
6. Reorder blocks, update: `index`, `idom`, `preds`, `succs`
7. Walk instructions, update block index references in: `ssa-phi` edges, `ssa-if` then/else, `ssa-jump` target

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

	eval(t, engine, `(define funcs (go-ssa-build "github.com/aalpar/wile-goast/goast"))`)
	eval(t, engine, `(define fn (car funcs))`)
	eval(t, engine, `(define canon (go-ssa-canonicalize fn))`)

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
2. Rename free variables: `fv0`, `fv1`, ... Extract names from `freeVars` s-expression (list of `ssa-free-var` nodes, each with a `name` field). Add to `nameMap`. Rebuild the `freeVars` list with updated names.
3. Rename params: `p0`, `p1`, ... Update `ssaParamData.name` and rebuild `original` s-expression. Add to `nameMap`.
4. First pass over blocks (canonical order): collect all instruction `name` fields, assign `r0`, `r1`, ... to `nameMap`.
5. Second pass: apply `renameInstrStrings` to all instructions.

All three namespaces (free variables, params, instructions) go into the same `nameMap`, ensuring no collisions — even if SSA happens to reuse a name across scopes, each original name maps to exactly one canonical name.

`renameInstrStrings(v values.Value, nameMap map[string]string) values.Value`:
- `*values.String`: if in nameMap, replace
- `*values.Pair`: recurse car and cdr, reuse if unchanged
- Other: return as-is

The recursive walk hits every string in the tree, covering all operand positions. Non-string values (`#f` for nil operands, integers for block indices) pass through unchanged.

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

`cmd/wile-goast/lib/wile/goast/ssa-normalize.sld` — exports: `ssa-normalize`, `ssa-rule-set`, `ssa-rule-identity`, `ssa-rule-commutative`, `ssa-rule-annihilation`. Imports `(wile goast utils)`.

`cmd/wile-goast/lib/wile/goast/ssa-normalize.scm` — implements:
- `commutative-ops` list: `'(+ * & | ^ == !=)`
- `(ssa-rule-commutative)`: returns lambda that swaps `x`/`y` when `(string>? x y)` for commutative ops
- `(integer-type? s)`: matches type strings starting with `"int"` or `"uint"` (e.g. `"int"`, `"int64"`, `"uint32"`)
- `(constant-zero? s)`: matches `"0"` or `"0:..."` strings
- `(constant-one? s)`: matches `"1"` or `"1:..."` strings
- `(ssa-rule-identity)`: `x + 0 → x`, `x * 1 → x`, etc. — **scoped to integer types only** (checks the node's `type` field via `integer-type?`). Float identity is unsafe due to IEEE 754 `-0.0` and `NaN` semantics.
- `(ssa-rule-annihilation)`: `x * 0 → 0`, `x & 0 → 0` — **scoped to integer types only** (`NaN * 0 ≠ 0`)
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

Test both `ast-diff` and `ssa-diff`:
- `ast-diff` on identical AST nodes → similarity 1.0
- `ast-diff` on AST nodes differing in type annotation → `type-name` diff detected
- `ssa-diff` on identical SSA nodes → similarity 1.0
- `ssa-diff` on SSA nodes differing only in `type` field → `type-name` diff detected
- `ssa-diff` on SSA nodes differing in `name` field (instruction) → `register` diff (weight 0)

**Step 2: Run test to verify it fails**

Run: `go test ./goast/ -run TestUnify -v`
Expected: FAIL — library not found.

**Step 3: Create the library**

`cmd/wile-goast/lib/wile/goast/unify.sld` — exports: `tree-diff`, `ast-diff`, `ssa-diff`, `classify-ast-diff`, `classify-ssa-diff`, `diff-result-similarity`, `diff-result-diffs`, `diff-result-shared`, `diff-result-diff-count`, `score-diffs`, `find-root-substitutions`, `collapse-diffs`, `unifiable?`

`cmd/wile-goast/lib/wile/goast/unify.scm` — adapted from `unify-detect-pkg.scm`:

**Pluggable classifier design.** The core diff engine is generic over tagged alists. It always tracks both `parent-tag` (the enclosing node's tag) and `path` (accumulated field keys) through the recursion. A **classifier function** receives `(tag field str-a str-b path)` and returns a category symbol. Two classifiers are provided:

- `classify-ast-diff` — path-based, ported from `unify-detect-pkg.scm`'s `classify-string-diff`. Uses `path` to detect type positions. Ignores `tag`.
- `classify-ssa-diff` — tag-based, uses `(tag, field)` pairs. Ignores `path` for classification (path still tracked for reporting).

Convenience wrappers bind the classifier:
- `(ast-diff node-a node-b)` — calls `tree-diff` with `classify-ast-diff`
- `(ssa-diff node-a node-b)` — calls `tree-diff` with `classify-ssa-diff`
- `(tree-diff node-a node-b classifier)` — generic entry point

Core diff engine:
- `merge-results`, `shared-result`, `diff-result`
- `fields-diff` — takes `parent-tag`, `path`, and `classifier`; passes all three to leaf classification
- `list-diff` — passes through `parent-tag` and `classifier` unchanged
- `tree-diff-walk` — extracts tag from `(car node)` at each tagged-alist level, forwards to `fields-diff`

Result format: `(diff-result (shared . N) (diff-count . N) (entries . (...)))`
Accessors: `diff-result-similarity` computes `shared / (shared + diff-count)`

Scoring and substitution collapsing:
- `find-root-substitutions` — same algorithm as existing script
- `collapse-diffs` — reclassify derived type diffs
- `score-diffs` — effective similarity with derived promotion (works with either classifier's output)

Verdict:
- `(unifiable? result threshold)` — `#t` when effective similarity >= threshold AND all remaining diffs are type-name or derived-type

**AST diff classification.** Ported from `classify-string-diff` in `unify-detect-pkg.scm`. Uses path position to classify:

```scheme
(define ast-type-fields '(type inferred-type asserted-type obj-pkg signature))
(define ast-identifier-fields '(name sel label))

(define (classify-ast-diff tag field str-a str-b path)
  (cond
    ((memq field ast-type-fields)                        'type-name)
    ((and (eq? field 'name) (in-type-position? path))    'type-name)
    ((memq field ast-identifier-fields)                  'identifier)
    ((eq? field 'value)                                  'literal-value)
    ((eq? field 'tok)                                    'operator)
    (else                                                'identifier)))
```

**SSA diff classification.** Uses `(tag, field)` pairs. The same field name means different things in different node types:

```scheme
;; Tags where 'name' is NOT a register — it's semantic identity.
;; Everything else (instruction nodes) uses 'name' as a register.
(define ssa-identity-name-tags '(ssa-func ssa-param))

(define (classify-ssa-diff tag field str-a str-b path)
  (cond
    ;; Type annotations on any node.
    ((memq field '(type asserted-type))                  'type-name)
    ;; Operators.
    ((eq? field 'op)                                     'operator)
    ;; Call targets.
    ((memq field '(func method))                         'call-target)
    ;; Structural (block indices, preds/succs) — should match after canonicalization.
    ((memq field '(index preds succs idom then else target)) 'structural)
    ;; 'name' field: register in instructions, identity in ssa-func/ssa-param.
    ((and (eq? field 'name) (memq tag ssa-identity-name-tags)) 'identifier)
    ((eq? field 'name)                                   'register)
    ;; Everything else.
    (else                                                'identifier)))
```

Weight map: `register → 0`, `type-name → 1`, `identifier → 0`, `operator → 2`, `call-target → 3`, `structural → 100`.

**Step 4: Run test to verify it passes**

Run: `go test ./goast/ -run TestUnify -v`
Expected: PASS

**Step 5: Commit**

```
feat(ssa): (wile goast unify) shared diff/scoring library

Extracted from unify-detect-pkg.scm with SSA-aware classification.
Provides ast-diff, ssa-diff, score-diffs, substitution collapsing,
and unifiable? verdict predicate.
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

`go-ssa-build` requires real Go packages (it loads via `packages.Load`). Put the test Go code in `examples/goast-query/testdata/` as real packages. These are under the module root and resolve as standard import paths:

- `github.com/aalpar/wile-goast/examples/goast-query/testdata/pncounter`
- `github.com/aalpar/wile-goast/examples/goast-query/testdata/gcounter`
- `github.com/aalpar/wile-goast/examples/goast-query/testdata/identity`

`pncounter/pncounter.go` (`package pncounter`) and `gcounter/gcounter.go` (`package gcounter`) — same code from the existing `unify-detect.scm` inline sources, as proper `.go` files.

`identity/identity.go` (`package identity`) — synthetic test case with two functions: one has `x + 0`, the other just uses `x`. Same structure otherwise.

**Step 2: Write the script**

`examples/goast-query/ssa-unify-detect.scm`:

1. Import `(wile goast utils)`, `(wile goast ssa-normalize)`, `(wile goast unify)`
2. Define helper to run the full pipeline on a pair of package paths + function names
3. For each test case, measure at **three stages** to isolate each layer's contribution:
   a. **AST diff** — `go-typecheck-package` both packages → `ast-diff` on AST func-decls → similarity
   b. **SSA canonicalized only** — `go-ssa-build` → find function by name → `go-ssa-canonicalize` → `ssa-diff` → similarity
   c. **SSA canonicalized + normalized** — same as (b) but apply `ssa-normalize` before diffing → similarity + verdict
4. Print comparison table with all three measurements per test case:

```
  Test case                  AST     SSA-canon  SSA-canon+norm  Verdict
  pncounter/gcounter Incr    0.72    0.85       0.91            unifiable
  ...
```

This directly answers whether Scheme normalization adds value beyond what Go's SSA builder and block/register canonicalization already provide.

The script must handle the SSA function lookup: `go-ssa-build` returns a flat list; find the function whose `name` field matches.

**Step 3: Run the script**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go run ./cmd/wile-goast -f examples/goast-query/ssa-unify-detect.scm`

Observe the output. Record all three similarity columns and verdicts. This answers the open question from `plans/UNIFICATION-DETECTION.md`. If the SSA-canon and SSA-canon+norm columns are identical, the normalization layer is redundant and can be removed.

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
