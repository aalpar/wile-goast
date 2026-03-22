# Call-Convention Mining — Design

**Date:** 2026-03-22
**Status:** Draft
**Target:** `github.com/aalpar/wile/machine` (configurable)

## Problem

The `machine/` package has ~19,600 lines across 48 files. Major types like `CompileTimeContinuation` (97 methods) and `MachineContext` (79 methods) encode implicit conventions — methods that share calling patterns — but nothing makes these conventions visible. When conventions drift, bugs hide.

## Approach

A Scheme script that discovers call conventions statistically. It parses a Go package, groups methods by receiver type, computes per-type call frequency, and reports deviations from the majority pattern. The code tells you what the patterns are; you decide which deviations matter.

This is the Engler "bugs as deviant behavior" thesis applied to call patterns: extract what the majority does, flag the minority.

## Algorithm

Four passes, all AST layer:

**Pass 0 — Inventory.** Parse the package with `go-typecheck-package`. Walk all `func-decl` nodes. Group methods by receiver type. Skip types with fewer than `min-type-methods` methods (default: 5).

**Pass 1 — Call Extraction.** For each method, walk its body and collect every callee:
- Direct calls: `functionName(...)` — an `ident` in `call-expr` position
- Selector calls: `receiver.Method(...)` — a `selector-expr` in `call-expr` position

Output: for each method, its call set — the callee names it invokes.

**Pass 2 — Convention Discovery.** For each receiver type, compute frequency of each callee across all methods in the group. A callee is a **convention** when:
- At least `min-frequency` of methods call it (default: 60%)
- At least `min-sites` methods call it (default: 5)

Sort conventions by frequency, strongest first.

**Pass 3 — Deviation Report.** For each convention, list the methods that don't follow it.

## Output Format

Per-type summary, then convention details:

```
══ CompileTimeContinuation (97 methods) ══
  Conventions discovered: 4
  Total deviations: 31

── Convention: CompileTimeContinuation → pushOperation (74%, 72/97) ──
  Deviations (25):
    String, validate, Reset, ...
```

## Configuration

Four knobs at the top of the script:

| Parameter | Default | Purpose |
|-----------|---------|---------|
| `target` | `"github.com/aalpar/wile/machine"` | Go package pattern |
| `min-frequency` | `0.60` | Minimum % to qualify as convention |
| `min-sites` | `5` | Minimum absolute call count |
| `min-type-methods` | `5` | Skip types with fewer methods |

## Script Structure

Single file: `examples/goast-query/call-convention-mine.scm`

```
Configuration
Utilities — import (wile goast utils), no hand-rolled copies
Pass 0: Inventory — parse, group by receiver type
Pass 1: Call Extraction — walk body, collect callees
Pass 2: Convention Discovery — frequency, threshold
Pass 3: Deviation Report — list non-adherents per convention
Summary — types analyzed, conventions found, total deviations
```

Invocation:
```bash
wile-goast -f examples/goast-query/call-convention-mine.scm
```

Override target inline:
```bash
wile-goast '(begin
  (define target "github.com/aalpar/wile/registry/core")
  (include "examples/goast-query/call-convention-mine.scm"))'
```

## Scope

**In scope:**
- Single script, AST layer, any Go package pattern
- Call conventions per receiver type, deviation report
- Configurable thresholds
- Uses `(wile goast utils)` — no duplicated utility code

**Out of scope (v1):**
- SSA/CFG signals (field access, control flow shape) — future layer
- Belief generation from discovered patterns — manual for now
- Cross-package convention mining
- Automatic remediation
- Free function conventions (no natural grouping key)

## Success Criteria

- Run against `machine/`, discover at least 2-3 real conventions
- At least one surprising deviation
- Runs in under 30 seconds (AST-only, no SSA build)
