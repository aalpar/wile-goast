# Package Split Analysis

Analyze a Go package's function dependencies to discover natural package
boundaries. Uses IDF-weighted Formal Concept Analysis on import signatures.

## Target Package

{{package}}

## Goal

{{goal}}

## Instructions

### Step 1: Run the analysis

```scheme
(import (wile goast split))
(import (wile goast utils))

(define refs (go-func-refs "{{package}}"))
(define report (recommend-split refs))
report
```

### Step 2: Interpret the result

The report contains:

- **functions** — total function count
- **high-idf** — informative dependencies (high IDF = referenced by few functions)
- **groups** — two function groups from the min-cut partition
  - **group-a**, **group-b** — function names in each group
  - **cut** — functions that bridge both groups (the coupling cost)
  - **cut-ratio** — cut size / total (lower is better; < 0.15 is excellent)
- **acyclic** — whether the split avoids Go import cycles
  - **a-refs-b**, **b-refs-a** — cross-group reference counts (cycle exists iff both > 0)
- **confidence** — overall verdict:
  - **HIGH** — acyclic, cut-ratio <= 0.15
  - **MEDIUM** — acyclic, cut-ratio <= 0.30
  - **LOW** — cyclic or high cut-ratio
  - **NONE** — no meaningful split found

### Step 3: Refine if needed

If confidence is LOW or you want finer-grained grouping, re-run with API
surface refinement. This replaces package-level attributes with
(package, object-name) pairs:

```scheme
(define report-refined (recommend-split refs 'refine))
report-refined
```

Compare the two reports — refinement often reveals sub-clusters within a
group that share the same package import but use different API surfaces.

You can also adjust the IDF threshold (default 0.36, which excludes
packages referenced by >70% of functions):

```scheme
(define report-strict (recommend-split refs 'idf-threshold 0.5))
```

### Step 4: Plan the split

If confidence is MEDIUM or HIGH and the split is acyclic:

1. **Identify the groups.** Name them by their dominant dependency
   (e.g., "ast-parsing" vs "type-checking").

2. **Examine the cut.** Functions in the cut bridge both groups. For each:
   - Can it be moved entirely to one group?
   - Does it need an interface or accessor to bridge the boundary?

3. **Check the dependency direction.** Only one group may import the other
   (or neither imports the other). The `a-refs-b` / `b-refs-a` counts
   show which direction is dominant — the referencing group imports the
   referenced group.

4. **List what moves.** The smaller group typically becomes the new package.
   List the files and functions that would move.

## Rules

- Always run the basic analysis before refinement
- A cut-ratio above 0.30 usually means the package is cohesive — don't force a split
- Acyclic is a hard requirement for Go packages — if both directions have references, the split needs restructuring (interface extraction or function moves)
- The analysis shows _where_ boundaries naturally fall, not _whether_ to split — that decision depends on package size, team structure, and maintenance goals
