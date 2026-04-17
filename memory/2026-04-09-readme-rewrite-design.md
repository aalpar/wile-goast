# README Rewrite Design

## Problem

The current README is a reference manual — primitive tables, syntax listings,
individual layer examples. It serves someone who already decided to use the tool,
not someone evaluating whether to.

Reddit post (r/golang, 36K views, 17 upvotes) confirmed: visitors bounce because
the README doesn't answer "why should I care?" before presenting "here's how it works."

## Target Audience

People interested in structured code querying — using code structure to drive
analysis rather than pattern matching. PL/tooling crowd, static analysis
enthusiasts, developers building custom analysis. Not the general "I need a Go
linter" audience.

## Approach: Question-led (Approach A)

Lead with the class of questions the tool answers. Frame as capability, not
finding. The reader self-selects: if these questions are interesting, they stay.

## Structure

### 1. Opening — questions as hook

One-line pitch: what questions can you ask about Go code that existing tools can't?

4-5 questions demonstrating range across layers:
- Lock/unlock pairing on all control flow paths (CFG)
- Convention adherence across 30 functions (belief DSL, cross-function)
- Structural identity modulo types (unification)
- Struct boundaries contradicted by field access patterns (FCA)
- Nil check before dereference (SSA + dataflow)

Then the one-sentence explanation: wile-goast exposes Go's compiler internals as
composable Scheme primitives.

### 2. Why Scheme

Preempt the obvious objection. 3-4 sentences:
- Go AST is a tree, s-expressions are trees. No marshaling, no schema.
- Show one Go expression as s-expression. Queryable with car/cdr/assoc/case.
- Same tagged-alist format across all layers. Learn one representation.
- Bidirectional: parse to s-expressions, modify, format back to Go.

### 3. Example 1 — paired-with with call depth

Lead example. Lock/unlock pairing analyzed across call boundaries. Demonstrates
what lint tools can't do: cross-function analysis via call graph + CFG.

The "lint does this already" objection is preempted by the call depth aspect —
lint checks one function body; this checks across call boundaries.

Show belief DSL code, show output.

### 4. Example 2 — FCA false boundary detection

FCA (Ganter & Wille, 1999) applied to field access patterns. No novelty claim —
established technique, value is availability as a composable primitive in the
same toolkit.

Show the pipeline: go-load -> go-ssa-field-index -> field-index->context ->
concept-lattice -> cross-boundary-concepts -> boundary-report.

### 5-11. Retained sections (lower half)

- Installation (go install)
- MCP Server
- Shared Sessions
- As a Go Library
- Build & Test
- Dependencies
- Documentation links

### Cut from README, moved to docs

- Seven-layer table (replaced by opening questions)
- Five primitive tables (already in docs/PRIMITIVES.md)
- Five individual layer examples (already in docs/EXAMPLES.md)
- Belief DSL syntax reference (selectors, predicates, checkers)
- "Zero external consumers" line — remove or reframe

### Updated

- Version: v0.5.6 (was v0.5.5)

## Design Decisions

**Why question-led over finding-led:** Reddit showed that leading with findings
invites dismissal ("just use grep", "that's not a real bug"). Leading with
questions frames the tool as a capability class.

**Why paired-with as lead example:** Familiar problem (lock/unlock), but the call
depth angle preempts the "lint does this" objection. The reader can see that
cross-function CFG analysis is doing work that per-function lint can't.

**Why FCA as second example:** Demonstrates breadth (a different analysis
paradigm). Framed as established technique (credited to Ganter & Wille), not
novelty claim. Value is composability within the toolkit.

**Why Scheme section exists:** Every Go developer's first reaction. Not
addressing it means the reader processes the objection silently while reading
examples, instead of engaging with the content.
