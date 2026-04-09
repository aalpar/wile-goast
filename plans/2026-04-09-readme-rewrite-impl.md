# README Rewrite Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rewrite README.md to convert the "structured code querying" audience by leading with questions, explaining why Scheme, and showing cross-layer examples instead of primitive tables.

**Architecture:** Single-file rewrite of README.md. Material cut from README already has a home in docs/PRIMITIVES.md and docs/EXAMPLES.md. No new code, no new files beyond the README itself.

**Tech Stack:** Markdown. Scheme code blocks (must be correct — verify against belief.scm and fca.scm).

---

### Task 1: Write the opening section

**Files:**
- Modify: `README.md:1-48` (replace title through seven-layer table)

**Step 1: Write the new opening**

Replace everything from the title through the seven-layer table with:

- `# wile-goast` (unchanged)
- One-line pitch framing the tool as a class of questions
- 4-5 questions demonstrating range:
  - Lock/unlock pairing on all control flow paths, across call boundaries
  - Convention adherence across N functions (statistical, threshold-based)
  - Structural identity modulo types (unification)
  - Struct boundaries contradicted by field access patterns (FCA)
  - Nil check before dereference (SSA + dataflow)
- One-sentence explanation: exposes Go compiler internals as composable Scheme primitives
- Link to Wile

**Step 2: Verify no broken links**

Run: `grep -c 'docs/PRIMITIVES.md\|docs/EXAMPLES.md\|docs/AST-NODES.md\|docs/GO-STATIC-ANALYSIS.md' README.md`

Ensure documentation links still appear (they'll be in the lower sections).

**Step 3: Commit**

```
git add README.md
git commit -m "docs(readme): question-led opening replaces layer table"
```

---

### Task 2: Write the "Why Scheme" section

**Files:**
- Modify: `README.md` (insert after opening, before examples)

**Step 1: Write the section**

3-4 concise paragraphs:
- Go AST is a tree, s-expressions are trees. No marshaling, no schema, no custom query grammar.
- Show `x + 1` as s-expression: `(binary-expr (op . +) (x . (ident (name . "x"))) (y . (lit (kind . int) (value . "1"))))`
  - Verify this is accurate by checking goast mapper output or existing examples.
- Same tagged-alist format across all layers. One representation.
- Bidirectional: parse, modify, format back to Go source.

**Step 2: Verify the s-expression example is correct**

Run: `wile-goast '(go-parse-expr "x + 1")'`

Use the actual output in the README. Do not guess.

**Step 3: Commit**

```
git add README.md
git commit -m "docs(readme): add why-scheme section"
```

---

### Task 3: Write Example 1 — paired-with with call depth

**Files:**
- Modify: `README.md` (insert after Why Scheme)

**Step 1: Draft the belief chain example**

Two chained beliefs demonstrating cross-function analysis:

```scheme
;; Belief 1: functions that call Lock — do they pair with Unlock?
(define-belief "lock-unlock-direct"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.90 5))

;; Belief 2: deviations from belief 1 — do their callers handle Unlock?
(define-belief "lock-unlock-callers"
  (sites (sites-from "lock-unlock-direct" 'deviation))
  (expect (contains-call "Unlock"))
  (threshold 0.75 3))

(run-beliefs "my/package/...")
```

**Step 2: Verify the chained belief is syntactically valid**

The `sites-from` selector returns func-decl nodes from a prior belief's
deviation set. The second belief checks whether those deviating functions
(which call Lock but don't Unlock) have callers that handle Unlock.

Test this against an actual package if possible, or at minimum verify
`sites-from` accepts `'deviation` as shown in belief.scm:367-377.

**Step 3: Write the explanatory text**

Brief text before and after the code block:
- Before: frame the problem ("lint checks lock/unlock within one function — what about across call boundaries?")
- After: explain what the chained belief does, and why lint can't
- Mention threshold model: "90% of functions that call Lock also call Unlock — here are the 10% that don't, and here's what their callers do"

**Step 4: Commit**

```
git add README.md
git commit -m "docs(readme): add paired-with call depth example"
```

---

### Task 4: Write Example 2 — FCA false boundary detection

**Files:**
- Modify: `README.md` (insert after Example 1)

**Step 1: Write the FCA example**

Use the pipeline from the current README's FCA section (which is accurate):

```scheme
(import (wile goast fca))

(let* ((s   (go-load "my/pkg/..."))
       (idx (go-ssa-field-index s))
       (ctx (field-index->context idx 'write-only 'cross-type-only))
       (lat (concept-lattice ctx))
       (xb  (cross-boundary-concepts lat 'min-extent 3)))
  (boundary-report xb))
```

**Step 2: Write the explanatory text**

- Credit FCA: "Formal Concept Analysis (Ganter & Wille, 1999) applied to field access patterns."
- Explain what it does: builds a concept lattice from SSA data, compares natural field groupings against actual struct boundaries, reports mismatches.
- No novelty claim. Value is availability as a composable primitive.

**Step 3: Commit**

```
git add README.md
git commit -m "docs(readme): add FCA false boundary example"
```

---

### Task 5: Restructure the lower half

**Files:**
- Modify: `README.md` (everything after the examples)

**Step 1: Cut primitive tables and individual examples**

Remove from README:
- The five primitive tables (AST, SSA, Call Graph, CFG, Lint) — already in docs/PRIMITIVES.md
- The five individual layer examples (parse-and-query, call graph, dominance, lint, unify-detect-pkg) — already in docs/EXAMPLES.md
- The Belief DSL syntax listing (selectors, predicates, checkers) — already in docs/PRIMITIVES.md

**Step 2: Keep and reorder retained sections**

Order:
1. Installation (`go install ...`)
2. MCP Server (unchanged)
3. Shared Sessions (unchanged)
4. As a Go Library (unchanged)
5. Build & Test (unchanged)
6. Dependencies table (unchanged)
7. Documentation links table (unchanged)
8. Version line: update to v0.5.6, remove "Zero external consumers" or reframe

**Step 3: Add a "Full Reference" link**

After the examples section, before Installation, add a one-line pointer:
"See [docs/PRIMITIVES.md](docs/PRIMITIVES.md) for the complete primitive reference across all layers."

**Step 4: Verify all internal links**

Check that every `[text](path)` link in the README points to an existing file.

Run: `grep -oP '\[.*?\]\(((?!http)[^)]+)\)' README.md` and verify each path exists.

**Step 5: Commit**

```
git add README.md
git commit -m "docs(readme): cut primitive tables, restructure lower half"
```

---

### Task 6: Final review

**Step 1: Read the complete README end-to-end**

Check for:
- Consistent voice and tone
- No orphaned references to removed sections
- Version updated to v0.5.6
- No "Zero external consumers" language

**Step 2: Verify build is clean**

Run: `make ci`

README changes shouldn't affect build, but verify nothing was accidentally touched.

**Step 3: Final commit if any cleanup needed**

```
git add README.md
git commit -m "docs(readme): final cleanup"
```
