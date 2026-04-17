# C2 Dataflow Analysis Framework — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a worklist-based dataflow analysis framework to `(wile goast dataflow)` that runs forward or backward analyses over SSA block graphs.

**Architecture:** `run-analysis` takes a direction, lattice, per-block transfer function, and SSA function, then runs a priority-ordered worklist until convergence. Results are queried via `analysis-in`/`analysis-out`. All new code goes in the existing `dataflow.scm`/`dataflow.sld` files.

**Tech Stack:** Scheme (R7RS), `(wile algebra)` lattice operations, SSA blocks from `go-ssa-build`

**Design doc:** `plans/2026-03-26-c2-dataflow-design.md`

---

### Task 1: `block-instrs` accessor

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/dataflow.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/dataflow.sld`
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the failing test**

Add to `goast/belief_integration_test.go`:

```go
func TestDataflowBlockInstrs(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that evaluates Scheme
	// expressions via the Wile engine — NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "./examples/goast-query/testdata/checking"))
		(define fn (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "HandleSafeA") (car fs))
		        (else (loop (cdr fs))))))
		(define b0 (car (nf fn 'blocks)))
	`)

	t.Run("returns instruction list", func(t *testing.T) {
		result := eval(t, engine, `(length (block-instrs b0))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "2")
	})

	t.Run("returns empty list for no-instr block", func(t *testing.T) {
		result := eval(t, engine, `(block-instrs '(ssa-block (index . 99)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "()")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDataflowBlockInstrs -v`
Expected: FAIL — `block-instrs` not defined

**Step 3: Implement**

Add to `dataflow.scm` (after the existing helpers):

```scheme
;; ─── Block accessors ──────────────────────────

(define (block-instrs block)
  (or (nf block 'instrs) '()))
```

Add `block-instrs` to the export list in `dataflow.sld`.

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDataflowBlockInstrs -v`
Expected: PASS

**Step 5: Commit**

```bash
git add goast/belief_integration_test.go cmd/wile-goast/lib/wile/goast/dataflow.scm cmd/wile-goast/lib/wile/goast/dataflow.sld
git commit -m "feat(dataflow): add block-instrs accessor"
```

---

### Task 2: Reverse postorder computation

Internal helper — not exported, but tested indirectly through `run-analysis`.
Implement now so Task 3 can use it.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/dataflow.scm`

**Step 1: Implement reverse-postorder**

Add to `dataflow.scm`:

```scheme
;; ─── Block ordering ───────────────────────────

(define (reverse-postorder blocks)
  ;; DFS from entry block (first in list). Returns block indices in RPO order.
  ;; Cons on backtrack builds RPO directly (prepend = reverse of append-postorder).
  (let ((block-map (map (lambda (b) (cons (nf b 'index) b)) blocks)))
    (define (succs-of idx)
      (let ((entry (assv idx block-map)))
        (if entry (or (nf (cdr entry) 'succs) '()) '())))
    (let ((visited '()) (result '()))
      (define (dfs idx)
        (unless (memv idx visited)
          (set! visited (cons idx visited))
          (for-each dfs (succs-of idx))
          (set! result (cons idx result))))
      (dfs (nf (car blocks) 'index))
      result)))
```

**Step 2: No separate test — verified by Task 3's integration tests**

**Step 3: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/dataflow.scm
git commit -m "feat(dataflow): add reverse-postorder block ordering"
```

---

### Task 3: Forward analysis — single block (trivial case)

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/dataflow.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/dataflow.sld`
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the failing test**

Test a "reaching names" analysis on `HandleUnsafe` (single block, no branches):

```go
func TestDataflowRunAnalysisForwardSingleBlock(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "./examples/goast-query/testdata/checking"))
		(define fn (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "HandleUnsafe") (car fs))
		        (else (loop (cdr fs))))))

		;; Reaching names: which SSA names are defined on any path reaching this point
		(define universe (ssa-instruction-names fn))
		(define lat (powerset-lattice universe))
		(define (transfer block state)
		  (let ((names (filter-map (lambda (i) (nf i 'name)) (block-instrs block))))
		    (lattice-join lat state names)))

		(define result (run-analysis 'forward lat transfer fn))
	`)

	t.Run("entry in-state is bottom", func(t *testing.T) {
		result := eval(t, engine, `(analysis-in result 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "()")
	})

	t.Run("entry out-state has defined names", func(t *testing.T) {
		result := eval(t, engine, `(> (length (analysis-out result 0)) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("analysis-states returns alist", func(t *testing.T) {
		result := eval(t, engine, `(length (analysis-states result))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDataflowRunAnalysisForwardSingleBlock -v`
Expected: FAIL — `run-analysis` not defined

**Step 3: Implement `run-analysis` and result accessors**

Add to `dataflow.scm`:

```scheme
;; ─── Result accessors ─────────────────────────

(define (analysis-in result block-idx)
  (let ((entry (assv block-idx result)))
    (and entry (cadr entry))))

(define (analysis-out result block-idx)
  (let ((entry (assv block-idx result)))
    (and entry (caddr entry))))

(define (analysis-states result)
  result)

;; ─── Worklist analysis ────────────────────────

(define (run-analysis direction lattice transfer ssa-fn . args)
  ;; Parse optional args: [initial-state] ['check-monotone]
  (let* ((initial-state (if (and (pair? args) (not (symbol? (car args))))
                            (car args)
                            (lattice-bottom lattice)))
         (flags (if (and (pair? args) (not (symbol? (car args))))
                    (cdr args)
                    args))
         (check-mono (and (memq 'check-monotone flags) #t))
         (blocks (nf ssa-fn 'blocks))
         (forward? (eq? direction 'forward))
         ;; Block lookup: idx → ssa-block
         (block-map (map (lambda (b) (cons (nf b 'index) b)) blocks))
         (block-ref (lambda (idx) (cdr (assv idx block-map))))
         ;; Compute ordering
         (rpo (reverse-postorder blocks))
         (order (if forward? rpo (reverse rpo)))
         (rank-map (let loop ((os order) (r 0) (m '()))
                     (if (null? os) m
                         (loop (cdr os) (+ r 1)
                               (cons (cons (car os) r) m)))))
         (rank-of (lambda (idx)
                    (let ((e (assv idx rank-map)))
                      (if e (cdr e) 999))))
         ;; Direction-aware edge accessors
         (flow-preds (lambda (b)
                       (or (if forward? (nf b 'preds) (nf b 'succs)) '())))
         (flow-succs (lambda (b)
                       (or (if forward? (nf b 'succs) (nf b 'preds)) '())))
         ;; Seed blocks: entry for forward, exits for backward
         (entry-idx (nf (car blocks) 'index))
         (exit-idxs (filter-map
                      (lambda (b)
                        (let ((s (nf b 'succs)))
                          (and (or (not s) (null? s)) (nf b 'index))))
                      blocks))
         (seed-idxs (if forward? (list entry-idx) exit-idxs))
         ;; State storage: mutable alist (idx in out)
         (bot (lattice-bottom lattice))
         (states (map (lambda (b)
                        (let ((idx (nf b 'index)))
                          (if (memv idx seed-idxs)
                              (if forward?
                                  (list idx initial-state bot)
                                  (list idx bot initial-state))
                              (list idx bot bot))))
                      blocks)))
    ;; State access helpers
    (define (get-in idx) (cadr (assv idx states)))
    (define (get-out idx) (caddr (assv idx states)))
    (define (set-state! idx in-val out-val)
      (set! states
        (map (lambda (entry)
               (if (= (car entry) idx)
                   (list idx in-val out-val)
                   entry))
             states)))
    ;; Worklist: sorted list of block indices by rank
    (define (worklist-insert wl idx)
      (if (memv idx wl) wl
          (let insert ((rest wl))
            (cond ((null? rest) (list idx))
                  ((<= (rank-of idx) (rank-of (car rest)))
                   (cons idx rest))
                  (else (cons (car rest) (insert (cdr rest))))))))
    (define (worklist-insert-all wl idxs)
      (let loop ((is idxs) (w wl))
        (if (null? is) w
            (loop (cdr is) (worklist-insert w (car is))))))
    ;; Main loop
    (let loop ((wl (worklist-insert-all '() seed-idxs)))
      (if (null? wl)
          ;; Done — return result alist
          states
          (let* ((idx (car wl))
                 (wl (cdr wl))
                 (blk (block-ref idx))
                 ;; Compute in-state from flow-predecessors
                 (pred-idxs (flow-preds blk))
                 (in-val (if (null? pred-idxs)
                             (if (memv idx seed-idxs)
                                 (if forward? initial-state bot)
                                 bot)
                             (let join-preds ((ps pred-idxs)
                                              (acc (if (and (memv idx seed-idxs) forward?)
                                                      initial-state
                                                      bot)))
                               (if (null? ps) acc
                                   (join-preds (cdr ps)
                                     (lattice-join lattice acc
                                       (get-out (car ps))))))))
                 ;; Apply transfer
                 (out-val (transfer blk in-val))
                 (old-out (get-out idx)))
            ;; Monotonicity check
            (when (and check-mono
                       (not (lattice-leq? lattice old-out out-val)))
              (error (string-append "monotonicity violation at block "
                                    (number->string idx))))
            ;; Update and propagate
            (set-state! idx in-val out-val)
            (if (lattice-leq? lattice out-val old-out)
                (loop wl)  ; no change
                (loop (worklist-insert-all wl
                        (flow-succs blk)))))))))
```

Add to `dataflow.sld` exports: `run-analysis`, `analysis-in`, `analysis-out`, `analysis-states`.

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDataflowRunAnalysisForwardSingleBlock -v`
Expected: PASS

**Step 5: Commit**

```bash
git add goast/belief_integration_test.go cmd/wile-goast/lib/wile/goast/dataflow.scm cmd/wile-goast/lib/wile/goast/dataflow.sld
git commit -m "feat(dataflow): run-analysis forward worklist with result accessors"
```

---

### Task 4: Forward analysis — branching and join

**Files:**
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the test**

Test reaching-names on `HandleSafeA` (3 blocks, branching, no reconvergence):

```go
func TestDataflowRunAnalysisForwardBranching(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "./examples/goast-query/testdata/checking"))
		(define fn (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "HandleSafeA") (car fs))
		        (else (loop (cdr fs))))))

		(define universe (ssa-instruction-names fn))
		(define lat (powerset-lattice universe))
		(define (transfer block state)
		  (let ((names (filter-map (lambda (i) (nf i 'name)) (block-instrs block))))
		    (lattice-join lat state names)))

		(define result (run-analysis 'forward lat transfer fn))
	`)

	t.Run("block 0 out has t0", func(t *testing.T) {
		result := eval(t, engine, `(member "t0" (analysis-out result 0))`)
		qt.New(t).Assert(result.IsTrue(), qt.IsTrue)
	})

	t.Run("block 1 in has t0 from predecessor", func(t *testing.T) {
		result := eval(t, engine, `(member "t0" (analysis-in result 1))`)
		qt.New(t).Assert(result.IsTrue(), qt.IsTrue)
	})

	t.Run("block 1 out has names from both blocks", func(t *testing.T) {
		result := eval(t, engine, `
			(and (member "t0" (analysis-out result 1))
			     (member "t1" (analysis-out result 1)))`)
		qt.New(t).Assert(result.IsTrue(), qt.IsTrue)
	})

	t.Run("block 2 in has t0 only", func(t *testing.T) {
		result := eval(t, engine, `
			(and (member "t0" (analysis-in result 2))
			     (= (length (analysis-in result 2)) 1))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDataflowRunAnalysisForwardBranching -v`
Expected: PASS (implementation already done in Task 3)

**Step 3: Write join test with reconvergence**

Test on `UpdateSafe` where block 3 has predecessors {0, 2}:

```go
func TestDataflowRunAnalysisForwardJoin(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "./examples/goast-query/testdata/pairing"))
		(define fn (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "UpdateSafe") (car fs))
		        (else (loop (cdr fs))))))

		(define universe (ssa-instruction-names fn))
		(define lat (powerset-lattice universe))
		(define (transfer block state)
		  (let ((names (filter-map (lambda (i) (nf i 'name)) (block-instrs block))))
		    (lattice-join lat state names)))

		(define result (run-analysis 'forward lat transfer fn))
	`)

	t.Run("join block in includes names from both predecessors", func(t *testing.T) {
		// Block 3 has preds {0, 2}. Its in-state should be union of
		// out(0) and out(2), which means it includes names defined
		// in block 2 even though the direct path 0->3 skips block 2.
		result := eval(t, engine, `
			(let ((b2-names (filter-map (lambda (i) (nf i 'name))
			                  (block-instrs (caddr (nf fn 'blocks)))))
			      (b3-in (analysis-in result 3)))
			  ;; Every name from block 2 should appear in block 3's in-state
			  (let check ((ns b2-names))
			    (cond ((null? ns) #t)
			          ((member (car ns) b3-in) (check (cdr ns)))
			          (else #f))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 4: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestDataflowRunAnalysisForward(Branching|Join)" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add goast/belief_integration_test.go
git commit -m "test(dataflow): forward analysis branching and join convergence"
```

---

### Task 5: Initial state parameter

**Files:**
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the test**

```go
func TestDataflowRunAnalysisInitialState(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "./examples/goast-query/testdata/checking"))
		(define fn (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "HandleUnsafe") (car fs))
		        (else (loop (cdr fs))))))

		;; Run with a non-bottom initial state: pretend "SEED" is already defined
		(define seeded-universe (cons "SEED" (ssa-instruction-names fn)))
		(define seeded-lat (powerset-lattice seeded-universe))
		(define (seeded-transfer block state)
		  (let ((names (filter-map (lambda (i) (nf i 'name)) (block-instrs block))))
		    (lattice-join seeded-lat state names)))

		(define result-seeded (run-analysis 'forward seeded-lat seeded-transfer fn
		                        (list "SEED")))
	`)

	t.Run("custom initial state propagates to in", func(t *testing.T) {
		result := eval(t, engine, `(member "SEED" (analysis-in result-seeded 0))`)
		qt.New(t).Assert(result.IsTrue(), qt.IsTrue)
	})

	t.Run("custom initial state reaches output", func(t *testing.T) {
		result := eval(t, engine, `(member "SEED" (analysis-out result-seeded 0))`)
		qt.New(t).Assert(result.IsTrue(), qt.IsTrue)
	})
}
```

**Step 2: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDataflowRunAnalysisInitialState -v`
Expected: PASS (initial-state parsing already in Task 3 implementation)

**Step 3: Commit**

```bash
git add goast/belief_integration_test.go
git commit -m "test(dataflow): initial state parameter propagation"
```

---

### Task 6: Backward analysis

**Files:**
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the test**

Backward "used names" analysis on `HandleSafeA`: which names are referenced
by instructions downstream of each block.

```go
func TestDataflowRunAnalysisBackward(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "./examples/goast-query/testdata/checking"))
		(define fn (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "HandleSafeA") (car fs))
		        (else (loop (cdr fs))))))

		;; Backward: collect operands referenced in each block
		(define universe (ssa-instruction-names fn))
		(define lat (powerset-lattice universe))
		(define (transfer block state)
		  (let ((ops (flat-map
		               (lambda (i) (or (nf i 'operands) '()))
		               (block-instrs block))))
		    ;; Keep only names that are in our universe (filter out literals)
		    (let ((relevant (filter (lambda (o) (member o universe)) ops)))
		      (lattice-join lat state relevant))))

		(define result (run-analysis 'backward lat transfer fn))
	`)

	t.Run("exit blocks out-state is bottom", func(t *testing.T) {
		;; Blocks 1 and 2 are exits. In backward analysis,
		;; their out-state (direction input) is initial-state = bottom.
		result := eval(t, engine, `(analysis-out result 1)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "()")
	})

	t.Run("entry block accumulates from successors", func(t *testing.T) {
		;; Block 0 has succs {1, 2}. In backward analysis, block 0's
		;; out-state is the join of block 1 and block 2's in-states.
		result := eval(t, engine, `(> (length (analysis-out result 0)) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("backward propagates usage toward entry", func(t *testing.T) {
		;; Block 1 references t0 (used in if-branch). After backward
		;; propagation, block 0's out-state should include names used
		;; in successor blocks.
		result := eval(t, engine, `
			(let ((b0-out (analysis-out result 0))
			      (b1-in  (analysis-in result 1))
			      (b2-in  (analysis-in result 2)))
			  ;; b0-out = join(b1-in, b2-in) — names used downstream
			  (>= (length b0-out)
			      (max (length b1-in) (length b2-in))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDataflowRunAnalysisBackward -v`
Expected: PASS (backward logic already in Task 3 implementation)

**Step 3: Commit**

```bash
git add goast/belief_integration_test.go
git commit -m "test(dataflow): backward analysis direction"
```

---

### Task 7: Monotonicity assertion

**Files:**
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the test**

A deliberately non-monotone transfer function should trigger an error:

```go
func TestDataflowMonotonicityViolation(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "./examples/goast-query/testdata/checking"))
		(define fn (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "HandleSafeA") (car fs))
		        (else (loop (cdr fs))))))

		(define universe (ssa-instruction-names fn))
		(define lat (powerset-lattice universe))

		;; Buggy transfer: returns empty set (shrinks state = non-monotone)
		(define call-count 0)
		(define (bad-transfer block state)
		  (set! call-count (+ call-count 1))
		  (if (> call-count 1) '() state))
	`)

	t.Run("no flag means no check", func(t *testing.T) {
		// Without 'check-monotone, the buggy transfer completes without error
		eval(t, engine, `
			(set! call-count 0)
			(run-analysis 'forward lat bad-transfer fn)`)
	})

	t.Run("check-monotone catches violation", func(t *testing.T) {
		// Verify evalExpectError exists in test helpers first.
		// If not, wrap in a guard-based approach.
		evalExpectError(t, engine, `
			(set! call-count 0)
			(run-analysis 'forward lat bad-transfer fn
			  (lattice-bottom lat) 'check-monotone)`)
	})
}
```

Note: `evalExpectError` is used in existing tests (e.g., `goastssa/prim_canonicalize_test.go`).
Verify it exists in the `goast` package test helpers. If it doesn't, the test should use
a different assertion pattern — check for error substring in the eval result.

**Step 2: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDataflowMonotonicityViolation -v`
Expected: PASS

**Step 3: Commit**

```bash
git add goast/belief_integration_test.go
git commit -m "test(dataflow): monotonicity assertion catches buggy transfer"
```

---

### Task 8: Full test suite + existing tests still pass

**Files:** none modified

**Step 1: Run all tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./... -count=1`
Expected: ALL PASS — existing `TestDataflowBooleanLattice`, `TestDataflowSSANames`, `TestDataflowDefuseReachable` unchanged

**Step 2: Run CI check**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: PASS (lint, build, test, 80% coverage threshold)

**Step 3: Fix any issues, commit if needed**

---

### Task 9: Update TODO.md

**Files:**
- Modify: `TODO.md`

**Step 1: Mark C2 items as done**

Update the C2 section in `TODO.md`:

```markdown
### C2. Dataflow analysis framework — DONE

Completed 2026-03-26. Worklist-based forward/backward analysis over SSA blocks.
`run-analysis` with per-block transfer, `analysis-in`/`analysis-out` queries,
`'check-monotone` flag. See `plans/2026-03-26-c2-dataflow-design.md`.

- [x] Define transfer function interface (per-block)
- [x] Forward/backward analysis combinator over SSA blocks (reverse postorder)
- [x] Worklist algorithm integrated with block ordering
- [x] Per-variable analysis via map lattice (vars -> lattice values) — uses existing (wile algebra)
- [x] Product lattice for combining analysis dimensions — uses existing (wile algebra)
- [x] Monotonicity assertion (debug mode) — detect buggy transfer functions
```

**Step 2: Commit**

```bash
git add TODO.md
git commit -m "docs: mark C2 dataflow analysis framework complete"
```
