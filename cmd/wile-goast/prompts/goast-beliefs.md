# Belief-Based Consistency Checking

Define and run consistency beliefs against Go packages using wile-goast's belief
DSL. Beliefs detect deviations from statistical patterns (Engler et al., "Bugs
as Deviant Behavior").

## Request

{{action}}

## Target Package

{{package}}

## Instructions

### Running existing beliefs

Look for `.goast-beliefs/` directory in the project root. If belief files exist,
run them using the eval tool:

```scheme
(begin
  (import (wile goast belief))
  (for-each (lambda (f) (load f))
    (list ".goast-beliefs/file1.scm" ".goast-beliefs/file2.scm"))
  (run-beliefs "./..."))
```

Replace the file list with actual `.goast-beliefs/*.scm` files.

### Defining a new belief

Guide the user through:
1. **Name**: descriptive identifier for the pattern
2. **Sites**: where to look — which functions match?
3. **Expect**: what property to check at each site?
4. **Threshold**: minimum adherence ratio and site count

Available site selectors:
- `(functions-matching pred ...)` — functions matching structural predicates
- `(callers-of "func")` — all callers of a named function
- `(methods-of "Type")` — all methods on a receiver type

Available predicates (for `functions-matching`):
- `(has-params "type" ...)` — signature contains param types
- `(has-receiver "type")` — method receiver matches
- `(name-matches "pattern")` — function name substring
- `(contains-call "func" ...)` — body calls any of these
- `(stores-to-fields "Struct" "field" ...)` — SSA stores to fields
- `(all-of pred ...)` / `(any-of pred ...)` / `(none-of pred ...)`

Available property checkers:
- `(contains-call "func" ...)` — call present/absent
- `(paired-with "A" "B")` — A paired with B (e.g., Lock/Unlock)
- `(ordered "A" "B")` — dominance ordering between calls
- `(co-mutated "field" ...)` — fields always stored together
- `(checked-before-use "val")` — value guarded before use
- `(custom (lambda (site ctx) ...))` — user-defined check

Write the belief to `.goast-beliefs/<name>.scm`. The file should import the
belief DSL but NOT call `run-beliefs` — the runner supplies the target.

### Interpreting results

Belief output reports:
- **Adherence**: what percentage of sites follow the pattern
- **Deviations**: which sites break the pattern (with locations)
- **Classification**: the majority behavior vs. minority deviations

Deviations are potential bugs — they break a pattern that most code follows.
But not all deviations are bugs; some are intentional exceptions. Present both
interpretations.

## Rules

- Belief files should NOT call `run-beliefs` — the runner supplies the target
- Each `.goast-beliefs/*.scm` file should `(import (wile goast belief))`
- Threshold: use 0.90 for strong patterns, 0.66 for weaker ones; minimum 3 sites
