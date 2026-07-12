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

- `opts` are symbols, e.g. `'positions` `'comments`.
- Call-graph names are qualified: `"(*Server).ProcessRequest"`,
  `"command-line-arguments.main"`.

### Call-graph algorithms — the result's soundness IS the algorithm's

`<algo>` is a symbol. **The choice changes what the answer means.** Every one of
these returns the same shape, so the return value cannot tell you whether you are
holding an exact set or a bound. You must know which you asked for.

| algo | soundness | use when |
|------|-----------|----------|
| `'static` | **UNDER-approx** (a lower bound) | resolves only direct calls; **ignores calls through function values**, so it MISSES edges in higher-order code. Rarely what you want. |
| `'cha` | **OVER-approx** (an upper bound) | resolves each indirect call to EVERY signature-compatible function. Sound, but very loose. |
| `'rta` | **OVER-approx**, tighter than `'cha` | prunes `'cha` to types actually instantiated. |
| `'vta` | **OVER-approx**, tightest of the standard three | flow-sensitive type propagation. Still a bound: it can include a function whose constant index is never invoked. |
| `'precise` | **EXACT on constant-index `[]func()`**; `'cha` elsewhere | CHA refined by resolving `t := []func(){...}; t[k]()` (k constant) from SSA. Never less sound than `'cha`. |

**Default to `'precise` for higher-order dispatch** (`[]func()` indexed by a
constant). It is exact there and never worse than `'cha` anywhere else:

    (import (wile goast path-algebra))
    (go-callgraph-reachable (go-callgraph "." 'precise) "p.f0")

**If you use an over-approximating algorithm (`'cha`/`'rta`/`'vta`), you are holding
an UPPER BOUND, not the answer.** Its surplus is yours to discharge: for each
function reached only through an indexed `[]func()` call, check that some *invoked*
constant index actually selects it, and drop the ones none selects. Reporting a
bound as if it were exact is the single most common way to get a wrong answer here.

**`'precise` does NOT resolve interface dispatch** (`x.M()`); it falls back to `'cha`
edges there. For interfaces `'vta` is the tightest available and is still an upper
bound — the discharge obligation above applies.

## Missing builtins (don't reach for these)

- `filter` → `(filter-map (lambda (x) (and (pred x) x)) lst)`
- `fold-left` / `fold-right` / `sort` → `fold` (SRFI-1: `(fold f seed lst)`) or a named-let
- `string-contains` / `string-prefix?` → `substring` + `string=?`
- `hashtable-keys` / `hashtable->alist` → track keys in a separate list

## Prefer a pipeline tool (bounded output)

For a known query, these return small JSON and never blow up the context:
`check_beliefs`, `discover_beliefs`, `recommend_split`, `recommend_boundaries`,
`find_false_boundaries`.
