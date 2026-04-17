# run-beliefs Return Value

## Problem

`run-beliefs` prints a formatted report to stdout and returns void. The MCP
`eval` tool checks `val.IsVoid()` and returns an empty string, so agents calling
`run-beliefs` get no data back. The structured results already exist inside
`evaluate-belief` and `evaluate-aggregate-beliefs` but are discarded after
printing.

## Decision

Return only, no printing. `run-beliefs` returns a flat list of self-describing
alists — one per registered belief (per-site and aggregate). Agents consume the
list directly; nothing is printed.

## Return Shape

### Per-site belief (status: strong | weak)

```scheme
((name . "lock-unlock")
 (type . per-site)
 (status . strong)
 (pattern . paired-defer)
 (ratio . 9/10)
 (total . 10)
 (adherence . ("pkg.Foo" "pkg.Bar"))
 (deviations . (("pkg.Baz" . unpaired))))
```

### Per-site belief (status: no-sites)

```scheme
((name . "no-match")
 (type . per-site)
 (status . no-sites))
```

### Per-site belief (status: error)

```scheme
((name . "broken")
 (type . per-site)
 (status . error)
 (message . "no such package: foo/bar"))
```

### Aggregate belief (status: ok)

```scheme
((name . "pkg-cohesion")
 (type . aggregate)
 (status . ok)
 (verdict . SPLIT)
 (confidence . HIGH)
 (functions . 47)
 (report . ...))
```

Analyzer-produced keys (verdict, confidence, etc.) are passed through from the
analyzer result alist.

### Aggregate belief (status: error)

```scheme
((name . "broken-agg")
 (type . aggregate)
 (status . error)
 (message . "analysis failed"))
```

## What Changes

- `run-beliefs`: stops printing, accumulates alists, returns list
- `evaluate-aggregate-beliefs`: stops printing, returns list of alists
- `print-header`, `print-belief-result`, `print-aggregate-result`: removed (dead code)
- `site-display-name`, `display-to-string`: kept (used to build display names in return alists)

## What Doesn't Change

- `define-belief`, `define-aggregate-belief`, `reset-beliefs!`
- `evaluate-belief` (still returns 6-element positional list internally)
- `ctx-store-result!` bootstrapping for `sites-from`
- All site selectors and property checkers
- No Go code changes — `handleEval` already serializes via `SchemeString()`

## Scope

`cmd/wile-goast/lib/wile/goast/belief.scm` only.
