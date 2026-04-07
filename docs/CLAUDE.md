# docs/ -- Documentation Conventions

## Documentation Notation

| Notation | Meaning | Example |
|----------|---------|---------|
| `<value>` | Required placeholder (user supplies) | `go-parse-file <path>` |
| `[value]` | Optional element | `go-ssa-build <pattern> ['debug]` |
| `<value>...` | One or more of this element | `go-analyze <pattern> <analyzer>...` |
| `[value]...` | Zero or more of this element | `(define-belief <name> [clause]...)` |
| `{a\|b}` | Required choice between alternatives | `go-callgraph <pattern> {static\|cha\|rta}` |
| `[a\|b]` | Optional choice between alternatives | `go-cfg <pattern> <func> [dot\|alist]` |
| `ALLCAPS` | Environment variable or constant | `$GOPATH` |
| `` `literal` `` | Exact text (use as-is) | `` `'debug` `` |
| `->` | Maps to / becomes / produces | `Go AST -> s-expression alist` |

**Escaping**: When angle brackets appear literally in Scheme expressions (rare), escape as `\<` or quote the whole expression.

**Combining**: `[--timeout <ms>]` means the flag is optional, but if provided, requires a value.

## Citing Design and Implementation Influences

When a design choice or implementation technique is drawn from an external source, cite it in code comments or documentation. This includes algorithms, data structure choices, and semantic decisions influenced by other work.

**What to cite**:
- Algorithms from the static analysis literature (SSA construction, dominator algorithms, call graph algorithms, abstract interpretation)
- Academic papers and their specific contributions (e.g., "Engler et al., Bugs as Deviant Behavior")
- Go toolchain internals that informed the design (`go/ast`, `go/types`, `golang.org/x/tools`)
- Pattern sources (e.g., belief DSL inspired by statistical consistency deviation detection)

**Format**: Cite inline in doc comments or documentation text, close to the content the influence applies to. Name the source and what was adopted.

**Examples**:
```
## Dominator tree construction uses the Lengauer-Tarjan algorithm
## via golang.org/x/tools/go/ssa.

## Belief DSL implements statistical consistency deviation detection
## per Engler et al., "Bugs as Deviant Behavior" (SOSP 2001).

## SSA canonicalization: alpha-renaming follows dominator-order
## block traversal for deterministic register numbering.
```

**Why**: Citing sources makes the documentation self-describing about *why* things are done a certain way. It helps users understand the provenance of design decisions and find the original material in `BIBLIOGRAPHY.md`.

## Document Organization

| File | Audience | Purpose |
|------|----------|---------|
| `PRIMITIVES.md` | API consumers | Complete primitive reference for all layers |
| `AST-NODES.md` | API consumers | Field reference for all AST node tags |
| `EXAMPLES.md` | Practitioners | Annotated walkthroughs of example scripts |
| `GO-STATIC-ANALYSIS.md` | New users | Usage guide with architecture overview and layer selection |
