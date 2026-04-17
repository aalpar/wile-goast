# Structured Docstrings Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add structured docstrings to all 93 exported symbols (24 Go primitives + 69 Scheme procedures) so `,doc`, `,topics`, and `,apropos` produce rich output.

**Architecture:** Go primitives get enriched `Doc` strings with `Examples:`/`See also:` sections plus `ReturnType`/`ParamTypes` fields on `PrimitiveSpec`. Scheme procedures get Guile-style docstrings (string literal after parameter list) with `Parameters:`/`Returns:`/`Category:` sections parsed by Wile's `internal/docparse/`.

**Tech Stack:** Go (`registry.PrimitiveSpec`, `values.ValueType`), Scheme (R7RS `define` docstring convention)

**Design doc:** `plans/2026-04-06-structured-docstrings-design.md`

---

## Docstring Conventions

### Go Primitives — Doc String Format

```go
Doc: "Brief description.\n" +
    "Additional detail about options or behavior.\n\n" +
    "Examples:\n" +
    "  (primitive-name \"arg\")\n" +
    "  (primitive-name \"arg\" 'option)\n\n" +
    "See also: `related-a', `related-b'.",
```

### Scheme Procedures — Docstring Format

```scheme
(define (func arg1 arg2)
  "Brief description.\n\nParameters:\n  arg1 : type\n  arg2 : type\nReturns: type\nCategory: category-name\n\nExamples:\n  (func 'a 'b)\n\nSee also: `related'."
  (body ...))
```

For higher-order constructors (site selectors, property checkers) that return procedures:
- `Returns: procedure` with prose explaining what the returned lambda accepts/returns.

For accessors (ctx-pkgs, diff-result-shared, etc.):
- Brief one-line description. No examples needed. `See also:` links to constructor.

---

## Task 1: goast/register.go — Core AST Primitives (11)

**Files:**
- Modify: `goast/register.go`

**Step 1: Add values import and update all 11 PrimitiveSpec entries**

```go
import (
	"github.com/aalpar/wile/registry"
	"github.com/aalpar/wile/values"
)
```

Update each spec with enriched `Doc`, `ReturnType`, and `ParamTypes` where applicable:

```go
{Name: "go-parse-file", ParamCount: 2, IsVariadic: true, Impl: PrimGoParseFile,
    Doc: "Parses a Go source file and returns an s-expression AST.\n" +
        "Options: 'comments includes comment nodes in the AST.\n\n" +
        "Examples:\n" +
        "  (go-parse-file \"main.go\")\n" +
        "  (go-parse-file \"main.go\" 'comments)\n\n" +
        "See also: `go-parse-string', `go-parse-expr', `go-format'.",
    ParamNames: []string{"filename", "options"}, Category: "goast",
    ReturnType: values.TypeList},
{Name: "go-parse-string", ParamCount: 2, IsVariadic: true, Impl: PrimGoParseString,
    Doc: "Parses a Go source string as a complete file and returns an s-expression AST.\n" +
        "Options: 'comments includes comment nodes.\n\n" +
        "Examples:\n" +
        "  (go-parse-string \"package main\\nfunc f() {}\")\n" +
        "  (go-parse-string \"package main\" 'comments)\n\n" +
        "See also: `go-parse-file', `go-parse-expr', `go-format'.",
    ParamNames: []string{"source", "options"}, Category: "goast",
    ReturnType: values.TypeList},
{Name: "go-parse-expr", ParamCount: 1, Impl: PrimGoParseExpr,
    Doc: "Parses a single Go expression and returns an s-expression AST.\n\n" +
        "Examples:\n" +
        "  (go-parse-expr \"x + y\")\n" +
        "  (go-parse-expr \"func() { return 1 }\")\n\n" +
        "See also: `go-parse-file', `go-node-type'.",
    ParamNames: []string{"source"}, Category: "goast",
    ParamTypes: []values.ValueType{values.TypeString},
    ReturnType: values.TypeList},
{Name: "go-format", ParamCount: 1, Impl: PrimGoFormat,
    Doc: "Converts an s-expression AST back to Go source code.\n" +
        "Falls back to unformatted output for partial or invalid ASTs.\n\n" +
        "Examples:\n" +
        "  (go-format (go-parse-expr \"x + 1\"))\n" +
        "  (go-format (go-parse-file \"main.go\"))\n\n" +
        "See also: `go-parse-file', `go-parse-string'.",
    ParamNames: []string{"ast"}, Category: "goast",
    ParamTypes: []values.ValueType{values.TypeList},
    ReturnType: values.TypeString},
{Name: "go-node-type", ParamCount: 1, Impl: PrimGoNodeType,
    Doc: "Returns the tag symbol of an AST node (e.g., 'func-decl, 'binary-expr).\n\n" +
        "Examples:\n" +
        "  (go-node-type (go-parse-expr \"x + 1\"))  ; => binary-expr\n\n" +
        "See also: `go-parse-file', `nf'.",
    ParamNames: []string{"ast"}, Category: "goast",
    ParamTypes: []values.ValueType{values.TypeList}},
{Name: "go-typecheck-package", ParamCount: 2, IsVariadic: true, Impl: PrimGoTypecheckPackage,
    Doc: "Loads a Go package with full type information and returns annotated ASTs.\n" +
        "First arg is a package pattern or GoSession. Options: 'debug.\n\n" +
        "Examples:\n" +
        "  (go-typecheck-package \"./...\")\n" +
        "  (go-typecheck-package (go-load \"./...\"))\n\n" +
        "See also: `go-load', `go-ssa-build', `go-interface-implementors'.",
    ParamNames: []string{"pattern", "options"}, Category: "goast",
    ReturnType: values.TypeList},
{Name: "go-interface-implementors", ParamCount: 2, Impl: PrimInterfaceImplementors,
    Doc: "Finds all concrete types implementing a named interface.\n" +
        "Second arg is a package pattern or GoSession.\n\n" +
        "Examples:\n" +
        "  (go-interface-implementors \"Reader\" \"io/...\")\n" +
        "  (go-interface-implementors \"Handler\" (go-load \"net/http\"))\n\n" +
        "See also: `go-typecheck-package', `implementors-of'.",
    ParamNames: []string{"interface-name", "package-pattern"}, Category: "goast",
    ReturnType: values.TypeList},
{Name: "go-load", ParamCount: 2, IsVariadic: true, Impl: PrimGoLoad,
    Doc: "Loads Go packages and returns a GoSession for reuse across analysis primitives.\n" +
        "Avoids redundant packages.Load calls when multiple primitives share a target.\n\n" +
        "Examples:\n" +
        "  (define s (go-load \"./...\"))\n" +
        "  (go-typecheck-package s)\n" +
        "  (go-ssa-build s)\n\n" +
        "See also: `go-session?', `go-typecheck-package', `go-ssa-build'.",
    ParamNames: []string{"pattern", "rest"}, Category: "goast"},
{Name: "go-session?", ParamCount: 1, Impl: PrimGoSessionP,
    Doc: "Returns #t if the argument is a GoSession value.\n\n" +
        "Examples:\n" +
        "  (go-session? (go-load \"./...\"))  ; => #t\n" +
        "  (go-session? \"./...\")            ; => #f\n\n" +
        "See also: `go-load'.",
    ParamNames: []string{"v"}, Category: "goast",
    ReturnType: values.TypeBoolean},
{Name: "go-list-deps", ParamCount: 2, IsVariadic: true, Impl: PrimGoListDeps,
    Doc: "Returns the transitive closure of import paths for the given package patterns.\n" +
        "Lightweight alternative to go-load for dependency discovery.\n\n" +
        "Examples:\n" +
        "  (go-list-deps \"net/http\")\n\n" +
        "See also: `go-load'.",
    ParamNames: []string{"pattern", "rest"}, Category: "goast",
    ReturnType: values.TypeList},
{Name: "go-cfg-to-structured", ParamCount: 2, IsVariadic: true, Impl: PrimGoCFGToStructured,
    Doc: "Restructures a block containing early returns into a single-exit if/else tree.\n" +
        "Optional second arg: func-type for result variable synthesis.\n" +
        "Phase 1 rewrites returns inside for/range to break+guard. Phase 2 folds\n" +
        "guard-if-return sequences into nested if/else via right-fold.\n\n" +
        "Examples:\n" +
        "  (go-cfg-to-structured block)\n" +
        "  (go-cfg-to-structured block func-type)\n\n" +
        "See also: `go-cfg', `go-format'.",
    ParamNames: []string{"block", "rest"}, Category: "goast",
    ReturnType: values.TypeList},
```

**Step 2: Build and verify**

Run: `make build`
Expected: Clean build, no compilation errors.

**Step 3: Commit**

```
git add goast/register.go
git commit -m "docs(goast): add structured docstrings to core AST primitives"
```

---

## Task 2: goastssa/register.go — SSA Primitives (3)

**Files:**
- Modify: `goastssa/register.go`

**Step 1: Add values import and update all 3 PrimitiveSpec entries**

```go
import (
	"github.com/aalpar/wile/registry"
	"github.com/aalpar/wile/values"
)
```

```go
{Name: "go-ssa-build", ParamCount: 2, IsVariadic: true, Impl: PrimGoSSABuild,
    Doc: "Builds SSA form for a Go package and returns a list of ssa-func nodes.\n" +
        "First arg is a package pattern or GoSession. Options: 'debug.\n\n" +
        "Examples:\n" +
        "  (go-ssa-build \"./...\")\n" +
        "  (go-ssa-build (go-load \"./...\") 'debug)\n\n" +
        "See also: `go-load', `go-ssa-canonicalize', `go-ssa-field-index'.",
    ParamNames: []string{"pattern", "options"}, Category: "goast-ssa",
    ReturnType: values.TypeList},
{Name: "go-ssa-field-index", ParamCount: 1, Impl: PrimGoSSAFieldIndex,
    Doc: "Returns per-function field access summaries for a Go package.\n" +
        "Each entry maps a function to its struct field store/load sites.\n" +
        "Arg is a package pattern or GoSession.\n\n" +
        "Examples:\n" +
        "  (go-ssa-field-index \"./...\")\n\n" +
        "See also: `go-ssa-build', `stores-to-fields'.",
    ParamNames: []string{"pattern"}, Category: "goast-ssa",
    ReturnType: values.TypeList},
{Name: "go-ssa-canonicalize", ParamCount: 1, Impl: PrimGoSSACanonicalize,
    Doc: "Canonicalizes an SSA function: dominator-order blocks, alpha-renamed registers.\n" +
        "Produces deterministic output for structural comparison.\n\n" +
        "Examples:\n" +
        "  (map go-ssa-canonicalize (go-ssa-build \"./...\"))\n\n" +
        "See also: `go-ssa-build', `ssa-normalize'.",
    ParamNames: []string{"ssa-func"}, Category: "goast-ssa",
    ParamTypes: []values.ValueType{values.TypeList},
    ReturnType: values.TypeList},
```

**Step 2: Build and verify**

Run: `make build`
Expected: Clean build.

**Step 3: Commit**

```
git add goastssa/register.go
git commit -m "docs(goastssa): add structured docstrings to SSA primitives"
```

---

## Task 3: goastcfg/register.go — CFG Primitives (4)

**Files:**
- Modify: `goastcfg/register.go`

**Step 1: Add values import and update all 4 PrimitiveSpec entries**

```go
import (
	"github.com/aalpar/wile/registry"
	"github.com/aalpar/wile/values"
)
```

```go
{Name: "go-cfg", ParamCount: 3, IsVariadic: true, Impl: PrimGoCFG,
    Doc: "Builds the control flow graph for a named function in a Go package.\n" +
        "First arg is a package pattern or GoSession.\n\n" +
        "Examples:\n" +
        "  (go-cfg \"./...\" \"MyFunc\")\n" +
        "  (go-cfg (go-load \"./...\") \"MyFunc\")\n\n" +
        "See also: `go-cfg-dominators', `go-cfg-paths', `go-cfg-to-structured'.",
    ParamNames: []string{"pattern", "func-name", "options"}, Category: "goast-cfg",
    ReturnType: values.TypeList},
{Name: "go-cfg-dominators", ParamCount: 1, Impl: PrimGoCFGDominators,
    Doc: "Builds a dominator tree from a cfg-block list returned by go-cfg.\n" +
        "Uses the Lengauer-Tarjan algorithm via golang.org/x/tools.\n\n" +
        "Examples:\n" +
        "  (go-cfg-dominators (go-cfg \"./...\" \"MyFunc\"))\n\n" +
        "See also: `go-cfg', `go-cfg-dominates?'.",
    ParamNames: []string{"cfg"}, Category: "goast-cfg",
    ParamTypes: []values.ValueType{values.TypeList},
    ReturnType: values.TypeList},
{Name: "go-cfg-dominates?", ParamCount: 3, Impl: PrimGoCFGDominates,
    Doc: "Returns #t if block A dominates block B in the dominator tree.\n\n" +
        "Examples:\n" +
        "  (go-cfg-dominates? dom-tree 0 3)\n\n" +
        "See also: `go-cfg-dominators', `go-cfg'.",
    ParamNames: []string{"dom-tree", "a", "b"}, Category: "goast-cfg",
    ReturnType: values.TypeBoolean},
{Name: "go-cfg-paths", ParamCount: 3, Impl: PrimGoCFGPaths,
    Doc: "Enumerates simple paths between two blocks in the CFG.\n" +
        "Capped at 1024 paths to bound computation.\n\n" +
        "Examples:\n" +
        "  (go-cfg-paths cfg 0 5)\n\n" +
        "See also: `go-cfg', `go-cfg-dominators'.",
    ParamNames: []string{"cfg", "from", "to"}, Category: "goast-cfg",
    ParamTypes: []values.ValueType{values.TypeList},
    ReturnType: values.TypeList},
```

**Step 2: Build and verify**

Run: `make build`
Expected: Clean build.

**Step 3: Commit**

```
git add goastcfg/register.go
git commit -m "docs(goastcfg): add structured docstrings to CFG primitives"
```

---

## Task 4: goastcg/register.go — Call Graph Primitives (4)

**Files:**
- Modify: `goastcg/register.go`

**Step 1: Add values import and update all 4 PrimitiveSpec entries**

```go
import (
	"github.com/aalpar/wile/registry"
	"github.com/aalpar/wile/values"
)
```

```go
{Name: "go-callgraph", ParamCount: 2, Impl: PrimGoCallgraph,
    Doc: "Builds a call graph for a Go package using the specified algorithm.\n" +
        "Algorithm is a symbol: 'static, 'cha, 'rta, or 'vta.\n" +
        "First arg is a package pattern or GoSession.\n\n" +
        "Examples:\n" +
        "  (go-callgraph \"./...\" 'rta)\n" +
        "  (go-callgraph (go-load \"./...\") 'cha)\n\n" +
        "See also: `go-callgraph-callers', `go-callgraph-callees', `go-callgraph-reachable'.",
    ParamNames: []string{"pattern", "algorithm"}, Category: "goast-callgraph",
    ReturnType: values.TypeList},
{Name: "go-callgraph-callers", ParamCount: 2, Impl: PrimGoCallgraphCallers,
    Doc: "Returns the incoming edges (callers) of a function in the call graph.\n" +
        "Returns #f if the function is not found. Use qualified names for methods\n" +
        "(e.g., \"(*Type).Method\").\n\n" +
        "Examples:\n" +
        "  (go-callgraph-callers cg \"handleRequest\")\n\n" +
        "See also: `go-callgraph', `go-callgraph-callees', `callers-of'.",
    ParamNames: []string{"graph", "func-name"}, Category: "goast-callgraph"},
{Name: "go-callgraph-callees", ParamCount: 2, Impl: PrimGoCallgraphCallees,
    Doc: "Returns the outgoing edges (callees) of a function in the call graph.\n" +
        "Returns #f if the function is not found.\n\n" +
        "Examples:\n" +
        "  (go-callgraph-callees cg \"main\")\n\n" +
        "See also: `go-callgraph', `go-callgraph-callers'.",
    ParamNames: []string{"graph", "func-name"}, Category: "goast-callgraph"},
{Name: "go-callgraph-reachable", ParamCount: 2, Impl: PrimGoCallgraphReachable,
    Doc: "Returns function names transitively reachable from the root in the call graph.\n\n" +
        "Examples:\n" +
        "  (go-callgraph-reachable cg \"main\")\n\n" +
        "See also: `go-callgraph'.",
    ParamNames: []string{"graph", "root-name"}, Category: "goast-callgraph",
    ReturnType: values.TypeList},
```

**Step 2: Build and verify**

Run: `make build`
Expected: Clean build.

**Step 3: Commit**

```
git add goastcg/register.go
git commit -m "docs(goastcg): add structured docstrings to call graph primitives"
```

---

## Task 5: goastlint/register.go — Lint Primitives (2)

**Files:**
- Modify: `goastlint/register.go`

**Step 1: Add values import and update both PrimitiveSpec entries**

```go
import (
	"github.com/aalpar/wile/registry"
	"github.com/aalpar/wile/values"
)
```

```go
{
    Name: "go-analyze", ParamCount: 2, IsVariadic: true,
    Impl: PrimGoAnalyze,
    Doc: "Runs named go/analysis passes on a Go package and returns diagnostics.\n" +
        "First arg is a package pattern or GoSession. Remaining args are\n" +
        "analyzer names (strings). Use go-analyze-list for available names.\n\n" +
        "Examples:\n" +
        "  (go-analyze \"./...\" \"nilness\" \"shadow\")\n" +
        "  (go-analyze (go-load \"./...\") \"unusedresult\")\n\n" +
        "See also: `go-analyze-list'.",
    ParamNames: []string{"pattern", "analyzer-names"},
    Category:   "goast-lint",
    ReturnType: values.TypeList,
},
{
    Name: "go-analyze-list", ParamCount: 0,
    Impl: PrimGoAnalyzeList,
    Doc: "Returns a sorted list of available analyzer names.\n" +
        "These names are valid arguments to go-analyze.\n\n" +
        "Examples:\n" +
        "  (go-analyze-list)  ; => (\"appends\" \"asmdecl\" ...)\n\n" +
        "See also: `go-analyze'.",
    ParamNames: []string{},
    Category:   "goast-lint",
    ReturnType: values.TypeList,
},
```

**Step 2: Build and verify**

Run: `make build`
Expected: Clean build.

**Step 3: Commit**

```
git add goastlint/register.go
git commit -m "docs(goastlint): add structured docstrings to lint primitives"
```

---

## Task 6: utils.scm — Utility Procedures (13)

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/utils.scm`

**Step 1: Add docstrings to all 13 exported procedures**

Insert a docstring as the first expression after each `define` parameter list.
Line numbers reference the current file.

**Line 8 — nf:**
```scheme
(define (nf node key)
  "Access a field in a tagged-alist AST node by key.\nReturns the associated value, or #f if not found.\n\nParameters:\n  node : list\n  key : symbol\nReturns: any\nCategory: goast-utils\n\nExamples:\n  (nf func-decl 'name)      ; => \"MyFunc\"\n  (nf func-decl 'missing)   ; => #f\n\nSee also: `tag?', `walk'."
  ...)
```

**Line 13 — tag?:**
```scheme
(define (tag? node t)
  "Test whether NODE is a tagged-alist with tag T.\nReturns #t if node is a pair whose car is T.\n\nParameters:\n  node : list\n  t : symbol\nReturns: boolean\nCategory: goast-utils\n\nExamples:\n  (tag? node 'func-decl)    ; => #t or #f\n\nSee also: `nf', `walk'."
  ...)
```

**Line 17 — filter-map:**
```scheme
(define (filter-map f lst)
  "Apply F to each element of LST, keeping only non-#f results.\n\nParameters:\n  f : procedure\n  lst : list\nReturns: list\nCategory: goast-utils\n\nExamples:\n  (filter-map (lambda (x) (and (> x 2) x)) '(1 2 3 4))  ; => (3 4)\n\nSee also: `flat-map'."
  ...)
```

**Line 24 — flat-map:**
```scheme
(define (flat-map f lst)
  "Apply F to each element of LST and concatenate the resulting lists.\n\nParameters:\n  f : procedure\n  lst : list\nReturns: list\nCategory: goast-utils\n\nExamples:\n  (flat-map (lambda (x) (list x x)) '(1 2))  ; => (1 1 2 2)\n\nSee also: `filter-map'."
  ...)
```

**Line 29 — walk:**
```scheme
(define (walk val visitor)
  "Depth-first walk over goast s-expressions.\nCalls (visitor node) on every tagged-alist node in the tree.\nVisitor is called for side effects; return value is ignored.\n\nParameters:\n  val : any\n  visitor : procedure\nReturns: any\nCategory: goast-utils\n\nSee also: `nf', `tag?', `ast-transform'."
  ...)
```

**Line 44 — member?:**
```scheme
(define (member? x lst)
  "Test whether X is a member of LST using equal?.\n\nParameters:\n  x : any\n  lst : list\nReturns: boolean\nCategory: goast-utils\n\nSee also: `unique'."
  ...)
```

**Line 50 — unique:**
```scheme
(define (unique lst)
  "Remove duplicates from LST, preserving first-occurrence order.\n\nParameters:\n  lst : list\nReturns: list\nCategory: goast-utils\n\nSee also: `member?'."
  ...)
```

**Line 57 — has-char?:**
```scheme
(define (has-char? s c)
  "Test whether string S contains character C.\n\nParameters:\n  s : string\n  c : char\nReturns: boolean\nCategory: goast-utils"
  ...)
```

**Line 64 — ordered-pairs:**
```scheme
(define (ordered-pairs lst)
  "Return all ordered pairs from LST. Each pair appears once.\nUseful for pairwise comparison of functions.\n\nParameters:\n  lst : list\nReturns: list\nCategory: goast-utils\n\nExamples:\n  (ordered-pairs '(a b c))  ; => ((a b) (a c) (b c))"
  ...)
```

**Line 71 — take:**
```scheme
(define (take lst n)
  "Return the first N elements of LST.\n\nParameters:\n  lst : list\n  n : integer\nReturns: list\nCategory: goast-utils\n\nSee also: `drop'."
  ...)
```

**Line 76 — drop:**
```scheme
(define (drop lst n)
  "Drop the first N elements of LST and return the rest.\n\nParameters:\n  lst : list\n  n : integer\nReturns: list\nCategory: goast-utils\n\nSee also: `take'."
  ...)
```

**Line 82 — ast-transform:**
```scheme
(define (ast-transform node f)
  "Depth-first pre-order tree rewriter over goast s-expressions.\nF receives each tagged-alist node and returns a replacement node.\nIf F returns #f, the original node is kept and children are visited.\nIf F returns a node, that node replaces the original (children not revisited).\n\nParameters:\n  node : list\n  f : procedure\nReturns: list\nCategory: goast-utils\n\nSee also: `ast-splice', `walk'."
  ...)
```

**Line 103 — ast-splice:**
```scheme
(define (ast-splice lst f)
  "Flat-mapping rewriter for statement/declaration lists.\nF receives each element and returns a list of replacements.\nUseful for inserting or removing statements in a block.\n\nParameters:\n  lst : list\n  f : procedure\nReturns: list\nCategory: goast-utils\n\nSee also: `ast-transform'."
  ...)
```

**Step 2: Build and verify**

Run: `make build && make test`
Expected: Clean build, all tests pass.

**Step 3: Commit**

```
git add cmd/wile-goast/lib/wile/goast/utils.scm
git commit -m "docs(utils): add structured docstrings to all goast-utils exports"
```

---

## Task 7: dataflow.scm — Dataflow Analysis Procedures (10)

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/dataflow.scm`

**Step 1: Add docstrings to all 10 exported procedures**

**Line 8 — boolean-lattice:**
```scheme
(define (boolean-lattice)
  "Construct a boolean lattice: bottom=#f, join=or, equal?=eq?.\nReturns an alist suitable for run-analysis.\n\nReturns: list\nCategory: goast-dataflow\n\nSee also: `run-analysis'."
  ...)
```

**Line 18 — ssa-all-instrs:**
```scheme
(define (ssa-all-instrs ssa-fn)
  "Flatten all instructions from all blocks of an SSA function.\n\nParameters:\n  ssa-fn : list\nReturns: list\nCategory: goast-dataflow\n\nSee also: `ssa-instruction-names', `block-instrs'."
  ...)
```

**Line 28 — ssa-instruction-names:**
```scheme
(define (ssa-instruction-names ssa-fn)
  "Extract all named values (registers) from an SSA function.\n\nParameters:\n  ssa-fn : list\nReturns: list\nCategory: goast-dataflow\n\nSee also: `ssa-all-instrs'."
  ...)
```

**Line 42 — make-reachability-transfer:**
```scheme
(define (make-reachability-transfer all-instrs found? names-lat)
  "Build a transfer function for def-use reachability analysis.\nALL-INSTRS is the flat instruction list. FOUND? is a predicate\nthat recognizes the target instruction. NAMES-LAT is a powerset\nlattice over instruction names.\n\nParameters:\n  all-instrs : list\n  found? : procedure\n  names-lat : list\nReturns: procedure\nCategory: goast-dataflow\n\nSee also: `defuse-reachable?'."
  ...)
```

**Line 77 — block-instrs:**
```scheme
(define (block-instrs block)
  "Extract the instruction list from an SSA block.\n\nParameters:\n  block : list\nReturns: list\nCategory: goast-dataflow\n\nSee also: `ssa-all-instrs'."
  ...)
```

**Line 98 — defuse-reachable?:**
```scheme
(define (defuse-reachable? ssa-fn start-names found? fuel)
  "Test whether any START-NAMES value reaches an instruction matching FOUND?\nvia def-use chains within FUEL iterations. Uses product lattice fixpoint.\n\nParameters:\n  ssa-fn : list\n  start-names : list\n  found? : procedure\n  fuel : integer\nReturns: boolean\nCategory: goast-dataflow\n\nExamples:\n  (defuse-reachable? fn '(\"t0\") (lambda (i) (tag? i 'ssa-if)) 5)\n\nSee also: `run-analysis', `make-reachability-transfer'."
  ...)
```

**Line 111 — analysis-in:**
```scheme
(define (analysis-in result block-idx)
  "Query the in-state at a block from a run-analysis result.\n\nParameters:\n  result : list\n  block-idx : integer\nReturns: any\nCategory: goast-dataflow\n\nSee also: `analysis-out', `analysis-states', `run-analysis'."
  ...)
```

**Line 115 — analysis-out:**
```scheme
(define (analysis-out result block-idx)
  "Query the out-state at a block from a run-analysis result.\n\nParameters:\n  result : list\n  block-idx : integer\nReturns: any\nCategory: goast-dataflow\n\nSee also: `analysis-in', `analysis-states', `run-analysis'."
  ...)
```

**Line 119 — analysis-states:**
```scheme
(define (analysis-states result)
  "Return the full result alist from run-analysis: ((idx in out) ...).\n\nParameters:\n  result : list\nReturns: list\nCategory: goast-dataflow\n\nSee also: `analysis-in', `analysis-out', `run-analysis'."
  ...)
```

**Line 124 — run-analysis:**
```scheme
(define (run-analysis direction lattice transfer ssa-fn . args)
  "Run worklist-based dataflow analysis on an SSA function.\nDIRECTION is 'forward or 'backward. LATTICE is an alist with 'bottom,\n'join, and 'equal? entries. TRANSFER is (lambda (block state) -> state).\nOptional args: initial state value, 'check-monotone flag for debugging.\n\nParameters:\n  direction : symbol\n  lattice : list\n  transfer : procedure\n  ssa-fn : list\nReturns: list\nCategory: goast-dataflow\n\nExamples:\n  (run-analysis 'forward (boolean-lattice) my-transfer fn)\n  (run-analysis 'forward lat xfer fn init-state 'check-monotone)\n\nSee also: `analysis-in', `analysis-out', `analysis-states', `boolean-lattice'."
  ...)
```

**Step 2: Build and verify**

Run: `make build && make test`
Expected: Clean build, all tests pass.

**Step 3: Commit**

```
git add cmd/wile-goast/lib/wile/goast/dataflow.scm
git commit -m "docs(dataflow): add structured docstrings to all goast-dataflow exports"
```

---

## Task 8: ssa-normalize.scm — SSA Normalization Procedures (5)

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/ssa-normalize.scm`

**Step 1: Add docstrings to all 5 exported procedures**

**Line 93 — ssa-rule-identity:**
```scheme
(define (ssa-rule-identity)
  "Construct a normalization rule for identity operations.\nRewrites x+0->x, x*1->x, x|0->x, x^0->x for integer types.\nReturns a rule: (lambda (node) -> node-or-#f).\n\nReturns: procedure\nCategory: goast-ssa-normalize\n\nSee also: `ssa-rule-annihilation', `ssa-rule-set', `ssa-normalize'."
  ...)
```

**Line 99 — ssa-rule-annihilation:**
```scheme
(define (ssa-rule-annihilation)
  "Construct a normalization rule for absorbing operations.\nRewrites x*0->0, x&0->0 for integer types.\nReturns a rule: (lambda (node) -> node-or-#f).\n\nReturns: procedure\nCategory: goast-ssa-normalize\n\nSee also: `ssa-rule-identity', `ssa-rule-set', `ssa-normalize'."
  ...)
```

**Line 105 — ssa-rule-commutative:**
```scheme
(define (ssa-rule-commutative)
  "Construct a normalization rule for commutative operations.\nSorts operands lexicographically so a+b and b+a produce identical output.\nReturns a rule: (lambda (node) -> node-or-#f).\n\nReturns: procedure\nCategory: goast-ssa-normalize\n\nSee also: `ssa-rule-identity', `ssa-rule-set', `ssa-normalize'."
  ...)
```

**Line 111 — ssa-rule-set:**
```scheme
(define (ssa-rule-set . rules)
  "Compose multiple normalization rules into one.\nApplies rules in order; first non-#f result wins.\n\nParameters:\n  rules : procedure\nReturns: procedure\nCategory: goast-ssa-normalize\n\nExamples:\n  (ssa-rule-set (ssa-rule-identity) (ssa-rule-commutative))\n\nSee also: `ssa-normalize'."
  ...)
```

**Line 129 — ssa-normalize (case-lambda):**

This is a case-lambda, so the docstring goes in the first clause body.
Read the exact form to determine placement.

```scheme
(define ssa-normalize
  ;; Docstring in comment form since case-lambda doesn't support leading docstrings
  ;; in Wile's current implementation. The Doc will be in the first clause.
  (case-lambda
    ((node)
     "Normalize an SSA binop node using default algebraic rules.\nWith one arg, applies identity + annihilation + commutativity rules.\nWith two args, applies the given rule set instead.\n\nParameters:\n  node : list\nReturns: any\nCategory: goast-ssa-normalize\n\nExamples:\n  (ssa-normalize binop-node)\n  (ssa-normalize binop-node (ssa-rule-set (ssa-rule-identity)))\n\nSee also: `ssa-rule-set', `ssa-rule-identity', `go-ssa-canonicalize'."
     ...)
    ((node rules) ...)))
```

**Step 2: Build and verify**

Run: `make build && make test`
Expected: Clean build, all tests pass.

**Step 3: Commit**

```
git add cmd/wile-goast/lib/wile/goast/ssa-normalize.scm
git commit -m "docs(ssa-normalize): add structured docstrings to all normalization exports"
```

---

## Task 9: unify.scm — Unification Detection Procedures (13)

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/unify.scm`

**Step 1: Add docstrings to all 13 exported procedures**

**Line 52 — diff-result-shared:**
```scheme
(define (diff-result-shared r)
  "Extract the shared-node count from a diff result.\n\nParameters:\n  r : list\nReturns: integer\nCategory: goast-unify\n\nSee also: `diff-result-diff-count', `diff-result-similarity'."
  ...)
```

**Line 53 — diff-result-diff-count:**
```scheme
(define (diff-result-diff-count r)
  "Extract the diff count from a diff result.\n\nParameters:\n  r : list\nReturns: integer\nCategory: goast-unify\n\nSee also: `diff-result-shared', `diff-result-diffs'."
  ...)
```

**Line 54 — diff-result-diffs:**
```scheme
(define (diff-result-diffs r)
  "Extract the list of classified diffs from a diff result.\nEach diff is (category path val-a val-b).\n\nParameters:\n  r : list\nReturns: list\nCategory: goast-unify\n\nSee also: `diff-result-diff-count', `score-diffs'."
  ...)
```

**Line 56 — diff-result-similarity:**
```scheme
(define (diff-result-similarity r)
  "Extract the raw similarity ratio from a diff result.\nReturns shared/(shared+diffs) as an exact rational.\n\nParameters:\n  r : list\nReturns: number\nCategory: goast-unify\n\nSee also: `score-diffs', `unifiable?'."
  ...)
```

**Line 77 — classify-ast-diff:**
```scheme
(define (classify-ast-diff tag field str-a str-b path)
  "Classify an AST diff by path position.\nReturns a category symbol: 'type, 'identifier, 'literal, or 'structural.\n\nParameters:\n  tag : symbol\n  field : symbol\n  str-a : string\n  str-b : string\n  path : list\nReturns: symbol\nCategory: goast-unify\n\nSee also: `ast-diff', `classify-ssa-diff'."
  ...)
```

**Line 93 — classify-ssa-diff:**
```scheme
(define (classify-ssa-diff tag field str-a str-b path)
  "Classify an SSA diff by node tag.\nReturns a category symbol: 'type, 'register, or 'structural.\n\nParameters:\n  tag : symbol\n  field : symbol\n  str-a : string\n  str-b : string\n  path : list\nReturns: symbol\nCategory: goast-unify\n\nSee also: `ssa-diff', `classify-ast-diff'."
  ...)
```

**Line 206 — tree-diff:**
```scheme
(define (tree-diff node-a node-b classifier)
  "Generic structural diff of two s-expression trees.\nCLASSIFIER categorizes each leaf difference. Returns a diff-result\nwith shared count, diff count, and classified diff list.\n\nParameters:\n  node-a : list\n  node-b : list\n  classifier : procedure\nReturns: list\nCategory: goast-unify\n\nSee also: `ast-diff', `ssa-diff'."
  ...)
```

**Line 209 — ast-diff:**
```scheme
(define (ast-diff node-a node-b)
  "Diff two AST nodes using path-based classification.\nConvenience wrapper around tree-diff with classify-ast-diff.\n\nParameters:\n  node-a : list\n  node-b : list\nReturns: list\nCategory: goast-unify\n\nExamples:\n  (ast-diff func-a func-b)\n\nSee also: `ssa-diff', `tree-diff', `unifiable?'."
  ...)
```

**Line 212 — ssa-diff:**
```scheme
(define (ssa-diff node-a node-b)
  "Diff two SSA nodes using tag-based classification.\nConvenience wrapper around tree-diff with classify-ssa-diff.\n\nParameters:\n  node-a : list\n  node-b : list\nReturns: list\nCategory: goast-unify\n\nExamples:\n  (ssa-diff (go-ssa-canonicalize fn-a) (go-ssa-canonicalize fn-b))\n\nSee also: `ast-diff', `tree-diff', `unifiable?'."
  ...)
```

**Line 261 — find-root-substitutions:**
```scheme
(define (find-root-substitutions pairs)
  "Find root substitution pairs from a list of (val-a . val-b) diffs.\nRoot substitutions are the minimal set from which others are derivable.\n\nParameters:\n  pairs : list\nReturns: list\nCategory: goast-unify\n\nSee also: `collapse-diffs', `score-diffs'."
  ...)
```

**Line 270 — collapse-diffs:**
```scheme
(define (collapse-diffs diffs roots)
  "Remove diffs that are derivable from root substitutions.\nA diff is derivable if applying the root substitutions to val-a\nproduces val-b.\n\nParameters:\n  diffs : list\n  roots : list\nReturns: list\nCategory: goast-unify\n\nSee also: `find-root-substitutions', `score-diffs'."
  ...)
```

**Line 301 — score-diffs:**
```scheme
(define (score-diffs shared-count diff-count diffs)
  "Compute effective similarity with substitution collapsing.\nFinds root substitutions, collapses derivable diffs, and returns\nthe adjusted similarity as a weighted score.\n\nParameters:\n  shared-count : integer\n  diff-count : integer\n  diffs : list\nReturns: number\nCategory: goast-unify\n\nSee also: `find-root-substitutions', `collapse-diffs', `unifiable?'."
  ...)
```

**Line 348 — unifiable?:**
```scheme
(define (unifiable? result threshold)
  "Determine whether two nodes are similar enough to unify.\nReturns #t when effective similarity >= THRESHOLD and all remaining\ndiffs are type/register (not structural).\n\nParameters:\n  result : list\n  threshold : number\nReturns: boolean\nCategory: goast-unify\n\nExamples:\n  (unifiable? (ast-diff fn-a fn-b) 0.85)\n\nSee also: `ast-diff', `ssa-diff', `score-diffs'."
  ...)
```

**Step 2: Build and verify**

Run: `make build && make test`
Expected: Clean build, all tests pass.

**Step 3: Commit**

```
git add cmd/wile-goast/lib/wile/goast/unify.scm
git commit -m "docs(unify): add structured docstrings to all unification exports"
```

---

## Task 10: belief.scm — Belief DSL Procedures (28)

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm`

**Step 1: Add docstrings to core procedures (3)**

**Line 22 — reset-beliefs!:**
```scheme
(define (reset-beliefs!)
  "Clear all registered beliefs.\n\nCategory: goast-belief\n\nSee also: `run-beliefs'."
  ...)
```

**Line 813 — run-beliefs:**
```scheme
(define (run-beliefs target)
  "Evaluate all registered beliefs against the target package pattern.\nPrints results showing adherence and deviation sites per belief.\nBeliefs are registered via define-belief.\n\nParameters:\n  target : string\nReturns: any\nCategory: goast-belief\n\nExamples:\n  (run-beliefs \"my/package/...\")\n\nSee also: `reset-beliefs!'."
  ...)
```

**Step 2: Add docstrings to context procedures (6)**

**Line 60 — make-context:**
```scheme
(define (make-context target)
  "Create a lazy-loading analysis context for TARGET package pattern.\nThe context loads AST, SSA, call graph, and field index on demand.\n\nParameters:\n  target : string\nReturns: list\nCategory: goast-belief\n\nSee also: `ctx-pkgs', `ctx-ssa', `ctx-callgraph'."
  ...)
```

**Line 82 — ctx-pkgs:**
```scheme
(define (ctx-pkgs ctx)
  "Return the type-checked package ASTs from CTX, loading if needed.\n\nParameters:\n  ctx : list\nReturns: list\nCategory: goast-belief\n\nSee also: `make-context', `ctx-ssa'."
  ...)
```

**Line 88 — ctx-ssa:**
```scheme
(define (ctx-ssa ctx)
  "Return the SSA functions from CTX, building if needed.\n\nParameters:\n  ctx : list\nReturns: list\nCategory: goast-belief\n\nSee also: `make-context', `ctx-pkgs', `ctx-find-ssa-func'."
  ...)
```

**Line 94 — ctx-callgraph:**
```scheme
(define (ctx-callgraph ctx)
  "Return the call graph from CTX, building with RTA if needed.\n\nParameters:\n  ctx : list\nReturns: list\nCategory: goast-belief\n\nSee also: `make-context', `callers-of'."
  ...)
```

**Line 100 — ctx-field-index:**
```scheme
(define (ctx-field-index ctx)
  "Return the SSA field access index from CTX, building if needed.\n\nParameters:\n  ctx : list\nReturns: list\nCategory: goast-belief\n\nSee also: `make-context', `stores-to-fields'."
  ...)
```

**Line 189 — ctx-find-ssa-func:**
```scheme
(define (ctx-find-ssa-func ctx pkg-path name)
  "Look up an SSA function by package path and short name.\nBuilds an index on first call for O(1) subsequent lookups.\n\nParameters:\n  ctx : list\n  pkg-path : string\n  name : string\nReturns: any\nCategory: goast-belief\n\nSee also: `ctx-ssa', `make-context'."
  ...)
```

**Step 3: Add docstrings to site selectors (7)**

**Line 216 — all-func-decls:**
```scheme
(define (all-func-decls pkgs)
  "Extract all func-decl nodes from a list of typed package ASTs.\n\nParameters:\n  pkgs : list\nReturns: list\nCategory: goast-belief\n\nSee also: `functions-matching'."
  ...)
```

**Line 236 — functions-matching:**
```scheme
(define (functions-matching . preds)
  "Site selector: functions matching all predicates.\nReturns a procedure (lambda (ctx) -> list-of-func-decls).\n\nParameters:\n  preds : procedure\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (functions-matching (contains-call \"Lock\"))\n  (functions-matching (has-receiver \"*Server\") (contains-call \"Close\"))\n\nSee also: `callers-of', `methods-of', `has-params', `contains-call'."
  ...)
```

**Line 271 — callers-of:**
```scheme
(define (callers-of func-name)
  "Site selector: all callers of a function.\nReturns a procedure (lambda (ctx) -> list-of-func-decls).\nUses the call graph to resolve callers, then maps back to AST func-decls.\n\nParameters:\n  func-name : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (callers-of \"handleRequest\")\n\nSee also: `functions-matching', `go-callgraph-callers'."
  ...)
```

**Line 294 — methods-of:**
```scheme
(define (methods-of type-name)
  "Site selector: all methods on a receiver type.\nShorthand for (functions-matching (has-receiver TYPE-NAME)).\n\nParameters:\n  type-name : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (methods-of \"*Server\")\n\nSee also: `functions-matching', `has-receiver'."
  ...)
```

**Line 299 — implementors-of:**
```scheme
(define (implementors-of iface-name)
  "Site selector: methods of all types implementing an interface.\nFinds concrete implementors via go-interface-implementors, then collects\ntheir methods.\n\nParameters:\n  iface-name : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (implementors-of \"Storage\")\n\nSee also: `interface-methods', `go-interface-implementors'."
  ...)
```

**Line 320 — interface-methods:**
```scheme
(define (interface-methods iface-name . args)
  "Site selector: methods of interface implementors, optionally filtered by name.\nWith one arg, returns all methods. With two, filters to methods matching\nthe given name.\n\nParameters:\n  iface-name : string\n  args : any\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (interface-methods \"Storage\")\n  (interface-methods \"Storage\" \"Save\")\n\nSee also: `implementors-of'."
  ...)
```

**Line 352 — sites-from:**
```scheme
(define (sites-from belief-name . opts)
  "Site selector: reuse results from a previously evaluated belief.\nOPTS control filtering: 'adherence or 'deviation, and optionally\na specific result symbol.\n\nParameters:\n  belief-name : string\n  opts : any\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (sites-from \"lock-unlock\" 'deviation)\n  (sites-from \"lock-unlock\" 'adherence 'paired-defer)\n\nSee also: `run-beliefs'."
  ...)
```

**Step 4: Add docstrings to predicates (5)**

**Line 397 — has-params:**
```scheme
(define (has-params . type-strings)
  "Predicate: function signature contains these parameter types.\nReturns a procedure (lambda (func-decl) -> boolean).\n\nParameters:\n  type-strings : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (has-params \"context.Context\" \"*http.Request\")\n\nSee also: `has-receiver', `name-matches', `functions-matching'."
  ...)
```

**Line 424 — has-receiver:**
```scheme
(define (has-receiver type-str)
  "Predicate: method receiver matches type string.\nMatches against both the type name and pointer variants.\n\nParameters:\n  type-str : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (has-receiver \"*Server\")\n\nSee also: `has-params', `methods-of', `functions-matching'."
  ...)
```

**Line 444 — name-matches:**
```scheme
(define (name-matches pattern)
  "Predicate: function name contains PATTERN as a substring.\n\nParameters:\n  pattern : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (name-matches \"Test\")\n\nSee also: `has-params', `functions-matching'."
  ...)
```

**Line 472 — contains-call:**
```scheme
(define (contains-call . func-names)
  "Predicate and property checker: function body calls any of FUNC-NAMES.\nAs a predicate for functions-matching, returns #t/#f.\nAs a property checker for expect, returns 'present or 'absent.\n\nParameters:\n  func-names : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (contains-call \"Lock\" \"RLock\")\n\nSee also: `functions-matching', `paired-with'."
  ...)
```

**Line 489 — stores-to-fields:**
```scheme
(define (stores-to-fields struct-name . field-names)
  "Predicate: SSA function stores to the named fields of STRUCT-NAME.\nDisambiguates receivers against the full struct field set.\n\nParameters:\n  struct-name : string\n  field-names : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (stores-to-fields \"Config\" \"Host\" \"Port\")\n\nSee also: `co-mutated', `go-ssa-field-index'."
  ...)
```

**Step 5: Add docstrings to combinators (3)**

**Line 501 — all-of:**
```scheme
(define (all-of . preds)
  "Predicate combinator: all predicates must match.\n\nParameters:\n  preds : procedure\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (all-of (has-receiver \"*Server\") (contains-call \"Close\"))\n\nSee also: `any-of', `none-of'."
  ...)
```

**Line 509 — any-of:**
```scheme
(define (any-of . preds)
  "Predicate combinator: at least one predicate must match.\n\nParameters:\n  preds : procedure\nReturns: procedure\nCategory: goast-belief\n\nSee also: `all-of', `none-of'."
  ...)
```

**Line 517 — none-of:**
```scheme
(define (none-of . preds)
  "Predicate combinator: no predicate matches.\n\nParameters:\n  preds : procedure\nReturns: procedure\nCategory: goast-belief\n\nSee also: `all-of', `any-of'."
  ...)
```

**Step 6: Add docstrings to property checkers (5)**

**Line 556 — paired-with:**
```scheme
(define (paired-with op-a op-b)
  "Property checker: verify that calls to OP-A are paired with OP-B.\nReturns 'paired-defer if paired via defer, 'paired-call if paired\nvia regular call, or 'unpaired.\n\nParameters:\n  op-a : string\n  op-b : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (paired-with \"Lock\" \"Unlock\")\n  (paired-with \"Open\" \"Close\")\n\nSee also: `contains-call', `ordered'."
  ...)
```

**Line 587 — ordered:**
```scheme
(define (ordered op-a op-b)
  "Property checker: verify that OP-A's SSA block dominates OP-B's block.\nReturns 'a-dominates-b, 'b-dominates-a, 'same-block, or 'unordered.\n\nParameters:\n  op-a : string\n  op-b : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (ordered \"Validate\" \"Execute\")\n\nSee also: `paired-with', `checked-before-use'."
  ...)
```

**Line 664 — co-mutated:**
```scheme
(define (co-mutated . field-names)
  "Property checker: verify that all named fields are stored together.\nReturns 'co-mutated if all fields written, 'partial otherwise.\nSkips receiver disambiguation — stores-to-fields already filtered.\n\nParameters:\n  field-names : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (co-mutated \"Host\" \"Port\" \"Scheme\")\n\nSee also: `stores-to-fields'."
  ...)
```

**Line 680 — checked-before-use:**
```scheme
(define (checked-before-use value-pattern)
  "Property checker: verify that a value matching VALUE-PATTERN is tested\nbefore use. Uses bounded def-use reachability (fuel=5) to check whether\nthe value flows through a comparison before reaching a non-guard use.\nReturns 'guarded or 'unguarded.\n\nParameters:\n  value-pattern : string\nReturns: procedure\nCategory: goast-belief\n\nExamples:\n  (checked-before-use \"err\")\n\nSee also: `ordered', `defuse-reachable?'."
  ...)
```

**Line 694 — custom:**
```scheme
(define (custom proc)
  "Property checker: escape hatch for user-defined checks.\nPROC receives (site ctx) and returns a symbol categorizing the result.\n\nParameters:\n  proc : procedure\nReturns: procedure\nCategory: goast-belief"
  ...)
```

**Step 7: Build and verify**

Run: `make build && make test`
Expected: Clean build, all tests pass.

**Step 8: Commit**

```
git add cmd/wile-goast/lib/wile/goast/belief.scm
git commit -m "docs(belief): add structured docstrings to all belief DSL exports"
```

---

## Task 11: Final Verification

**Step 1: Full CI**

Run: `make ci`
Expected: lint + build + test + covercheck all pass.

**Step 2: Spot-check REPL output**

Run: `./dist/darwin/arm64/wile-goast -e '(import (wile goast utils)) (procedure-documentation nf)'`
Expected: Returns the docstring for `nf`.

Run: `./dist/darwin/arm64/wile-goast -e '(import (wile goast belief)) (procedure-documentation run-beliefs)'`
Expected: Returns the docstring for `run-beliefs`.

**Step 3: Update plans**

Mark `2026-04-06-structured-docstrings-impl.md` as complete in `plans/CLAUDE.md`.
