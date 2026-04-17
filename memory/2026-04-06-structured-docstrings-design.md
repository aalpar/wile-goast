# Structured Docstrings for wile-goast — Design

**Date**: 2026-04-06
**Status**: Approved
**Motivation**: Conform to Wile's REPL documentation system (`,doc`, `,topics`,
`,apropos`). Go primitives have single-line `Doc` strings with no examples or
cross-references. Scheme procedures have zero docstrings — all documentation is
in comments. The goal: every exported symbol gets structured metadata so the REPL
tools produce rich, navigable output.

## Format

### Go Primitives (PrimitiveSpec)

Structured metadata uses two channels:

**PrimitiveSpec fields** (rendered by `formatPrimitiveDoc`):
- `ReturnType` — `values.TypeList`, `values.TypeBoolean`, etc. Use `values.TypeAny`
  for polymorphic returns (e.g., `go-node-type` returns a symbol but there's no
  `TypeSymbol` in the enum that maps cleanly).
- `ParamTypes` — add where unambiguous. Omit for polymorphic params (string-or-GoSession).

**Doc string sections** (preserved in output by `formatPrimitiveDoc`):
- Prose description (existing, enriched with option documentation)
- `Examples:` — 1-2 short usage examples
- `See also:` — cross-references to related primitives

Category and ParamNames already present on all primitives. No changes needed.

### Scheme Procedures (Guile-style docstrings)

String literal immediately after parameter list. Parsed by `internal/docparse/`.

```
"Brief description.\n\nParameters:\n  arg : type\nReturns: type\nCategory: cat\n\nExamples:\n  (func 'arg)\n\nSee also: `related'."
```

Sections: `Parameters:`, `Returns:`, `Category:` are extracted into structured
fields. `Examples:` and `See also:` are preserved in prose. All sections optional.

## Categories

| Category | Scope |
|----------|-------|
| `goast` | Core AST: parse, format, type-check, load, sessions |
| `goast-ssa` | SSA construction and canonicalization |
| `goast-cfg` | Control flow graph, dominators, paths |
| `goast-callgraph` | Call graph construction and queries |
| `goast-lint` | go/analysis framework |
| `goast-belief` | Belief DSL: selectors, checkers, combinators |
| `goast-dataflow` | Worklist analysis and def-use reachability |
| `goast-utils` | AST traversal, list utilities, tree rewriters |
| `goast-ssa-normalize` | SSA algebraic normalization rules |
| `goast-unify` | Structural diff and unification detection |
| `goast-fca` | Formal Concept Analysis: context, lattice, boundary detection |
| `goast-boolean` | Boolean normalization for AST conditions and belief selectors |
| `goast-path` | Semiring path algebra over call graphs |

## Inventory

### Go Primitives (24)

#### goast/register.go (11)

| Primitive | ReturnType | ParamTypes | See also |
|-----------|-----------|------------|----------|
| `go-parse-file` | list | string, symbol | `go-parse-string`, `go-format` |
| `go-parse-string` | list | string, symbol | `go-parse-file`, `go-format` |
| `go-parse-expr` | list | string | `go-parse-file`, `go-node-type` |
| `go-format` | string | list | `go-parse-file`, `go-parse-string` |
| `go-node-type` | any | list | `go-parse-file`, `nf` |
| `go-typecheck-package` | list | (polymorphic), symbol | `go-load`, `go-ssa-build` |
| `go-interface-implementors` | list | string, (polymorphic) | `go-typecheck-package` |
| `go-load` | any | string, symbol | `go-session?`, `go-typecheck-package` |
| `go-session?` | boolean | any | `go-load` |
| `go-list-deps` | list | string | `go-load` |
| `go-cfg-to-structured` | list | list | `go-cfg`, `go-format` |

#### goastssa/register.go (3)

| Primitive | ReturnType | ParamTypes | See also |
|-----------|-----------|------------|----------|
| `go-ssa-build` | list | (polymorphic), symbol | `go-load`, `go-ssa-canonicalize` |
| `go-ssa-field-index` | list | (polymorphic) | `go-ssa-build`, `stores-to-fields` |
| `go-ssa-canonicalize` | list | list | `go-ssa-build`, `ssa-normalize` |

#### goastcfg/register.go (4)

| Primitive | ReturnType | ParamTypes | See also |
|-----------|-----------|------------|----------|
| `go-cfg` | list | (polymorphic), string, symbol | `go-cfg-dominators`, `go-cfg-paths` |
| `go-cfg-dominators` | list | list | `go-cfg`, `go-cfg-dominates?` |
| `go-cfg-dominates?` | boolean | list, any, any | `go-cfg-dominators` |
| `go-cfg-paths` | list | list, any, any | `go-cfg`, `go-cfg-dominators` |

#### goastcg/register.go (4)

| Primitive | ReturnType | ParamTypes | See also |
|-----------|-----------|------------|----------|
| `go-callgraph` | list | (polymorphic), symbol | `go-callgraph-callers`, `go-callgraph-callees` |
| `go-callgraph-callers` | any | list, string | `go-callgraph`, `go-callgraph-callees` |
| `go-callgraph-callees` | any | list, string | `go-callgraph`, `go-callgraph-callers` |
| `go-callgraph-reachable` | list | list, string | `go-callgraph` |

#### goastlint/register.go (2)

| Primitive | ReturnType | ParamTypes | See also |
|-----------|-----------|------------|----------|
| `go-analyze` | list | (polymorphic), string | `go-analyze-list` |
| `go-analyze-list` | list | (none) | `go-analyze` |

### Scheme Procedures

#### utils.scm — 13 exports, Category: goast-utils

| Procedure | Signature | Returns |
|-----------|-----------|---------|
| `nf` | `(nf node key)` | any |
| `tag?` | `(tag? node t)` | boolean |
| `walk` | `(walk val visitor)` | any |
| `filter-map` | `(filter-map f lst)` | list |
| `flat-map` | `(flat-map f lst)` | list |
| `member?` | `(member? x lst)` | boolean |
| `unique` | `(unique lst)` | list |
| `has-char?` | `(has-char? s c)` | boolean |
| `ordered-pairs` | `(ordered-pairs lst)` | list |
| `take` | `(take lst n)` | list |
| `drop` | `(drop lst n)` | list |
| `ast-transform` | `(ast-transform node f)` | list |
| `ast-splice` | `(ast-splice lst f)` | list |

#### dataflow.scm — 10 exports, Category: goast-dataflow

| Procedure | Signature | Returns | Notes |
|-----------|-----------|---------|-------|
| `boolean-lattice` | `(boolean-lattice)` | list | Constructor; returns lattice alist |
| `ssa-all-instrs` | `(ssa-all-instrs ssa-fn)` | list | |
| `ssa-instruction-names` | `(ssa-instruction-names ssa-fn)` | list | |
| `make-reachability-transfer` | `(make-reachability-transfer all-instrs found? names-lat)` | procedure | |
| `defuse-reachable?` | `(defuse-reachable? ssa-fn start-names found? fuel)` | boolean | |
| `block-instrs` | `(block-instrs block)` | list | |
| `run-analysis` | `(run-analysis direction lattice transfer ssa-fn . args)` | list | Variadic: [initial-state] ['check-monotone] |
| `analysis-in` | `(analysis-in result block-idx)` | any | |
| `analysis-out` | `(analysis-out result block-idx)` | any | |
| `analysis-states` | `(analysis-states result)` | list | |

#### belief.scm — 30 exported procedures, Category: goast-belief

Excludes: `define-belief` (macro), `*beliefs*` (variable), 7 re-exports from utils.

| Procedure | Signature | Returns | Subcategory |
|-----------|-----------|---------|-------------|
| `run-beliefs` | `(run-beliefs target)` | any | Core |
| `reset-beliefs!` | `(reset-beliefs!)` | any | Core |
| `make-context` | `(make-context target)` | list | Context |
| `ctx-pkgs` | `(ctx-pkgs ctx)` | list | Context |
| `ctx-ssa` | `(ctx-ssa ctx)` | list | Context |
| `ctx-callgraph` | `(ctx-callgraph ctx)` | list | Context |
| `ctx-find-ssa-func` | `(ctx-find-ssa-func ctx pkg-path name)` | any | Context |
| `ctx-field-index` | `(ctx-field-index ctx)` | list | Context |
| `all-func-decls` | `(all-func-decls pkgs)` | list | Site selectors |
| `functions-matching` | `(functions-matching . preds)` | procedure | Site selectors |
| `callers-of` | `(callers-of func-name)` | procedure | Site selectors |
| `methods-of` | `(methods-of type-name)` | procedure | Site selectors |
| `implementors-of` | `(implementors-of iface-name)` | procedure | Site selectors |
| `interface-methods` | `(interface-methods iface-name . args)` | procedure | Site selectors |
| `sites-from` | `(sites-from belief-name . opts)` | procedure | Site selectors |
| `has-params` | `(has-params . type-strings)` | procedure | Predicates |
| `has-receiver` | `(has-receiver type-str)` | procedure | Predicates |
| `name-matches` | `(name-matches pattern)` | procedure | Predicates |
| `contains-call` | `(contains-call . func-names)` | procedure | Pred/Checker |
| `stores-to-fields` | `(stores-to-fields struct-name . field-names)` | procedure | Predicates |
| `all-of` | `(all-of . preds)` | procedure | Combinators |
| `any-of` | `(any-of . preds)` | procedure | Combinators |
| `none-of` | `(none-of . preds)` | procedure | Combinators |
| `paired-with` | `(paired-with op-a op-b)` | procedure | Property checkers |
| `ordered` | `(ordered op-a op-b)` | procedure | Property checkers |
| `co-mutated` | `(co-mutated . field-names)` | procedure | Property checkers |
| `checked-before-use` | `(checked-before-use value-pattern)` | procedure | Property checkers |
| `custom` | `(custom proc)` | procedure | Property checkers |

Note: `contains-call` is exported as both a predicate (for `functions-matching`)
and a property checker (for `expect`). Same procedure, dual role — document both
uses in the docstring.

#### ssa-normalize.scm — 5 exports, Category: goast-ssa-normalize

| Procedure | Signature | Returns |
|-----------|-----------|---------|
| `ssa-normalize` | case-lambda: `(node)` or `(node rules)` | any |
| `ssa-rule-set` | `(ssa-rule-set . rules)` | procedure |
| `ssa-rule-identity` | `(ssa-rule-identity)` | procedure |
| `ssa-rule-commutative` | `(ssa-rule-commutative)` | procedure |
| `ssa-rule-annihilation` | `(ssa-rule-annihilation)` | procedure |

#### unify.scm — 13 exports, Category: goast-unify

| Procedure | Signature | Returns |
|-----------|-----------|---------|
| `tree-diff` | `(tree-diff node-a node-b classifier)` | list |
| `ast-diff` | `(ast-diff node-a node-b)` | list |
| `ssa-diff` | `(ssa-diff node-a node-b)` | list |
| `classify-ast-diff` | `(classify-ast-diff tag field str-a str-b path)` | symbol |
| `classify-ssa-diff` | `(classify-ssa-diff tag field str-a str-b path)` | symbol |
| `diff-result-similarity` | `(diff-result-similarity r)` | number |
| `diff-result-diffs` | `(diff-result-diffs r)` | list |
| `diff-result-shared` | `(diff-result-shared r)` | number |
| `diff-result-diff-count` | `(diff-result-diff-count r)` | number |
| `score-diffs` | `(score-diffs shared-count diff-count diffs)` | number |
| `find-root-substitutions` | `(find-root-substitutions pairs)` | list |
| `collapse-diffs` | `(collapse-diffs diffs roots)` | list |
| `unifiable?` | `(unifiable? result threshold)` | boolean |

## Totals

| Source | Docstrings to write |
|--------|---------------------|
| Go primitives (5 files) | 24 |
| utils.scm | 13 |
| dataflow.scm | 10 |
| belief.scm | 28 |
| ssa-normalize.scm | 5 |
| unify.scm | 13 |
| **Total** | **93** |

Excluded: 1 macro (`define-belief`), 1 variable (`*beliefs*`), 7 re-exports
(belief.sld re-exports from utils — docstring propagates via shared closure).

## Go Import Changes

Files that need `values` import for `ReturnType`/`ParamTypes`:

| File | Currently imports values? | Change |
|------|--------------------------|--------|
| `goast/register.go` | No | Add `"github.com/aalpar/wile/values"` |
| `goastssa/register.go` | No | Add `"github.com/aalpar/wile/values"` |
| `goastcfg/register.go` | No | Add `"github.com/aalpar/wile/values"` |
| `goastcg/register.go` | No | Add `"github.com/aalpar/wile/values"` |
| `goastlint/register.go` | No | Add `"github.com/aalpar/wile/values"` |

## Phasing

### Phase 1: Go Primitives (24 docstrings)

Update all 5 register.go files: enrich `Doc` strings with `Examples:` and
`See also:` sections, add `ReturnType`, add `ParamTypes` where unambiguous.
Add `values` import.

### Phase 2: Scheme Utilities (13 docstrings)

Add docstrings to utils.scm. These are leaf dependencies — no cross-library
`See also:` complications.

### Phase 3: Scheme Dataflow (10 docstrings)

Add docstrings to dataflow.scm. Depends on utils having docstrings for
cross-references.

### Phase 4: Scheme SSA Normalize (5 docstrings)

Add docstrings to ssa-normalize.scm. Small, self-contained.

### Phase 5: Scheme Unify (13 docstrings)

Add docstrings to unify.scm. Self-contained diff/scoring library.

### Phase 6: Scheme Belief DSL (28 docstrings)

Add docstrings to belief.scm. Largest file, most cross-references. Done last
because it re-exports from utils and imports from dataflow — all `See also:`
targets are documented by this point.

## Out of Scope

- `define-belief` macro docstring (no Wile mechanism for `define-syntax` docs)
- `*beliefs*` variable (no parameter list, no docstring slot)
- Changes to Wile's docparse or REPL rendering
- `boolean-lattice` in dataflow.scm is a zero-arg constructor returning a lattice
  alist — document it as a procedure, not a variable
