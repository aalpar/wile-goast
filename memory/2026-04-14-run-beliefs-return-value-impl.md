# run-beliefs Return Value — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make `run-beliefs` return a flat list of self-describing alists instead of printing and returning void, so MCP agents get structured data.

**Architecture:** Single-file change to `belief.scm`. Reshape the existing `evaluate-belief` output into alists in `run-beliefs`. Change `evaluate-aggregate-beliefs` to return alists instead of printing. Remove dead print functions. Update four existing tests that assert on printed output.

**Tech Stack:** Scheme (R7RS), Go test harness via `newBeliefEngine(t)` + test helpers

---

### Task 1: Write failing test for per-site belief return shape

**Files:**
- Modify: `goast/belief_integration_test.go` (replace `TestBeliefRunBeliefsOutput` around line 719)

**Step 1: Write the failing test**

Replace `TestBeliefRunBeliefsOutput` with a test that asserts on return value structure instead of printed output.

The test should:
1. Register a single belief ("ordering-test") with `ordered "Validate" "Process"` checker
2. Call `run-beliefs` and bind the return value to `results`
3. Assert: `(pair? results)` is `#t`
4. Assert: first result has `(name . "ordering-test")`, `(type . per-site)`, `(status . strong)`, `(pattern . a-dominates-b)`, `(ratio . 4/5)`, `(total . 5)`
5. Assert: adherence list has length 4 (display name strings)
6. Assert: deviations list has 1 entry, it's a `(string . symbol)` pair, and the string matches `PipelineReversed`

Use `assoc` to extract fields from the result alist.

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefRunBeliefsOutput -v -count=1`

Expected: FAIL — `run-beliefs` currently returns void, so `results` is void and `(pair? results)` returns `#f`.

---

### Task 2: Write failing test for weak/no-sites/error entries

**Files:**
- Modify: `goast/belief_integration_test.go` (add new test after `TestBeliefRunBeliefsOutput`)

**Step 1: Write the failing test**

Add `TestBeliefRunBeliefsStatuses`:
1. Register three beliefs against the ordering testdata:
   - "strong-one": threshold 0.60 / 3 (will be strong — 4/5 adherence)
   - "weak-one": threshold 0.99 / 3 (will be weak — same 4/5 can't reach 0.99)
   - "empty-one": matches `ZZZZZ_NO_MATCH` (will be no-sites)
2. Call `run-beliefs`, bind to `results`
3. Assert: 3 results returned
4. Assert: first has status `strong`, second has status `weak`, third has status `no-sites`
5. Assert: weak entry still has `pattern` and `ratio` keys

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefRunBeliefsStatuses -v -count=1`

Expected: FAIL — `run-beliefs` returns void.

---

### Task 3: Write failing test for aggregate belief return shape

**Files:**
- Modify: `goast/belief_integration_test.go` (replace `TestAggregateBeliefEvaluation` around line 1575)

**Step 1: Write the failing test**

Replace `TestAggregateBeliefEvaluation`:
1. Register one aggregate belief with `aggregate-custom` that returns `((verdict . TEST-OK) (functions . N))`
2. Call `run-beliefs`, bind to `results`
3. Assert: 1 result returned
4. Assert: result has `(type . aggregate)`, `(status . ok)`, `(name . "test-cohesion")`
5. Assert: analyzer key `verdict` is `TEST-OK` (pass-through)

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestAggregateBeliefEvaluation -v -count=1`

Expected: FAIL.

---

### Task 4: Implement run-beliefs return value

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm` (lines ~822-995)

This is the core implementation task. Three sub-steps:

**Step 1: Rewrite `evaluate-aggregate-beliefs` (line 858)**

Replace the current implementation that prints and returns `(values count errors)` with one that returns a list of alists. Each entry has `(name . X) (type . aggregate) (status . ok|error)` plus pass-through keys from the analyzer result. Error entries include `(message . X)`.

Key changes:
- Remove all `display`/`newline` calls
- Accumulate results in a `results` list parameter to the loop
- Return `(reverse results)` at end
- Guard clause builds error alist instead of printing

**Step 2: Rewrite `run-beliefs` (line 926)**

Replace the current implementation that prints report and returns void. The new version:
- Removes `(print-header)` call
- Loops over `*beliefs*`, building one alist per belief
- Error: `(status . error) (message . X)`
- No sites: `(status . no-sites)`
- Has results: `(status . strong|weak)` plus `pattern`, `ratio`, `total`, `adherence` (display names), `deviations` (name . category pairs)
- Still calls `ctx-store-result!` for `sites-from` bootstrapping
- At end: `(append (reverse results) (evaluate-aggregate-beliefs ctx))`
- Update the docstring: `Returns: list` (was `Returns: any`)

**Step 3: Remove dead print functions**

Delete these functions (no longer called after the rewrite):
- `print-header` (~lines 822-828)
- `print-belief-result` (~lines 831-854)
- `print-aggregate-result` (~lines 881-922)

Keep `site-display-name` and `display-to-string` — they're used in the alist construction.

**Step 4: Run the three new tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestBeliefRunBeliefsOutput|TestBeliefRunBeliefsStatuses|TestAggregateBeliefEvaluation" -v -count=1`

Expected: All PASS.

---

### Task 5: Update sites-from bootstrapping test

**Files:**
- Modify: `goast/belief_integration_test.go` (`TestBeliefSitesFrom` around line 679)

**Step 1: Rewrite the test**

The current test captures stdout via `parameterize` and checks printed output. Replace with:
1. Call `run-beliefs` and bind to `results`
2. Assert: 2 results returned (pairing + followup)
3. Assert: first result (pairing) has status `strong`
4. Assert: second result (followup) has total = 1 (bootstrapped from pairing's deviation)
5. Assert: second result has status `strong`

**Step 2: Run the test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefSitesFrom -v -count=1`

Expected: PASS.

---

### Task 6: Update define-and-run test

**Files:**
- Modify: `goast/belief_integration_test.go` (`TestBeliefDefineAndRun` around line 125)

**Step 1: Rewrite the test**

Current test calls `run-beliefs` and ignores the void return. Update to verify a result list is returned:
1. Add `(reset-beliefs!)` before `define-belief`
2. Bind `run-beliefs` return to a variable
3. Assert: result is a pair and first element has a `name` key

**Step 2: Run the test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefDefineAndRun -v -count=1`

Expected: PASS.

---

### Task 7: Run full test suite and verify coverage

**Step 1: Run all belief tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "Belief|Aggregate" -v -count=1`

Expected: All PASS.

**Step 2: Run full CI**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`

Expected: All PASS, coverage >= 80%.

**Step 3: Verify exports are unchanged**

Read `cmd/wile-goast/lib/wile/goast/belief.sld` — no changes needed. `run-beliefs` is already exported.

**Step 4: Commit**

```
feat(belief): return structured alists from run-beliefs

run-beliefs now returns a flat list of self-describing alists
instead of printing a report and returning void. Each entry
has name, type (per-site | aggregate), status (strong | weak |
no-sites | error | ok), and type-specific fields (pattern,
ratio, deviations for per-site; analyzer pass-through for
aggregate). This makes beliefs usable programmatically via MCP.
```
