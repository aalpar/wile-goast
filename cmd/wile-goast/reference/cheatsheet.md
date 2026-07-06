# wile-goast eval quick reference

Write correct Scheme on the first try. Parse a package, then **project small**:
never return a whole AST or package (they can exceed 100K chars and are
truncated).

## Pattern: parse → query → project

    (go-typecheck-package "./...")            ; typed packages (big; summarize it)
    (length (go-typecheck-package "./..."))   ; how many packages
    (map car (go-parse-file "main.go"))       ; top-level structure tags only

`(map car <alist>)` returns the tags of any node (a safe first probe). For field
tags, load the `goast-scheme-ref` prompt or see docs/AST-NODES.md.

## Core primitives (exact arities)

    (go-parse-file <filename> . opts)        -> (file ...) alist
    (go-parse-string <source> . opts)        -> (file ...) alist
    (go-typecheck-package <target> . opts)   -> list of (package ...)
    (go-format <ast>)                        -> Go source string
    (go-ssa-build <pattern> . opts)          -> list of ssa-func
    (go-cfg <pattern> <func-name> . opts)    -> list of cfg-block
    (go-callgraph <pattern> <algo>)          -> list of cg-node
    (go-callgraph-callers <graph> <name>)    -> list of cg-edge or #f
    (go-callgraph-callees <graph> <name>)    -> list of cg-edge or #f
    (go-analyze <pattern> <analyzer> ...)    -> list of diagnostic
    (go-analyze-list)                        -> list of analyzer names

- `<algo>` is a symbol: `'static` `'cha` `'rta` `'vta`. Example:
  `(go-callgraph "." 'static)`.
- `opts` are symbols, e.g. `'positions` `'comments`.
- Call-graph names are qualified: `"(*Server).ProcessRequest"`,
  `"command-line-arguments.main"`.

## Missing builtins (don't reach for these)

- `filter` → `(filter-map (lambda (x) (and (pred x) x)) lst)`
- `fold-left` / `fold-right` / `sort` → `fold` (SRFI-1: `(fold f seed lst)`) or a named-let
- `string-contains` / `string-prefix?` → `substring` + `string=?`
- `hashtable-keys` / `hashtable->alist` → track keys in a separate list

## Prefer a pipeline tool (bounded output)

For a known query, these return small JSON and never blow up the context:
`check_beliefs`, `discover_beliefs`, `recommend_split`, `recommend_boundaries`,
`find_false_boundaries`.
