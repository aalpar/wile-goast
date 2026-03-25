# Belief Checker Improvements — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix two belief checkers that produced poor results on etcd: `ordered` (same-block vote splitting) and `checked-before-use` (shallow data-flow tracking).

**Architecture:** Category 4 (`ordered`): add intra-block instruction position comparison so `same-block` resolves to `a-dominates-b` or `b-dominates-a`. Category 2 (`checked-before-use`): replace 1-hop binop check with bounded transitive reachability on the SSA def-use graph (4-hop depth limit) to follow chains like `m -> store -> field-addr -> load -> call -> if`.

**Tech Stack:** R7RS Scheme (`cmd/wile-goast/lib/wile/goast/belief.scm`), Go integration tests (`goast/belief_integration_test.go`), Go test helpers (`newBeliefEngine(t)`, the test `eval` helper from `prim_goast_test.go`).

---

### Task 1: Fix `ordered` — intra-block instruction ordering

When both calls are in the same SSA block, walk the instruction list to determine which call comes first. Return `'a-dominates-b` or `'b-dominates-a` instead of `'same-block`.

**Files:**
- Create: `examples/goast-query/testdata/sameblock/sameblock.go`
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm`
- Modify: `goast/belief_integration_test.go`

**Step 1: Create the testdata package**

Create `examples/goast-query/testdata/sameblock/sameblock.go`:

```go
package sameblock

func Foo() int { return 1 }
func Bar() int { return 2 }

func FooFirst() int {
	a := Foo()
	b := Bar()
	return a + b
}

func BarFirst() int {
	b := Bar()
	a := Foo()
	return a + b
}
```

No branching — both calls in one block. SSA instruction order matches source order.

**Step 2: Write the failing test**

Add to `goast/belief_integration_test.go`:

```go
func TestBeliefCategory4_SameBlockOrdering(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/sameblock"))

		(define checker (ordered "Foo" "Bar"))
		(define sites ((functions-matching
		                 (all-of (contains-call "Foo") (contains-call "Bar")))
		               ctx))
		(define classified
		  (map (lambda (site) (cons (nf site 'name) (checker site ctx)))
		       sites))
	`)

	t.Run("FooFirst is a-dominates-b", func(t *testing.T) {
		result := eval(t, engine, `
			(cdr (car (filter-map
			  (lambda (p) (and (equal? (car p) "FooFirst") p))
			  classified)))
		`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "a-dominates-b")
	})

	t.Run("BarFirst is b-dominates-a", func(t *testing.T) {
		result := eval(t, engine, `
			(cdr (car (filter-map
			  (lambda (p) (and (equal? (car p) "BarFirst") p))
			  classified)))
		`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "b-dominates-a")
	})
}
```

**Step 3: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefCategory4_SameBlockOrdering -v -timeout 120s`
Expected: FAIL — both return `'same-block`.

**Step 4: Add `find-call-position` helper to belief.scm**

Add after `find-ssa-call-blocks` (around line 618):

```scheme
;; Find the instruction index of the first call to func-name in a block.
;; Returns the 0-based position in the block's instrs list, or #f.
(define (find-call-position block func-name)
  (let ((instrs (or (nf block 'instrs) '())))
    (let loop ((is instrs) (pos 0))
      (cond
        ((null? is) #f)
        ((and (or (tag? (car is) 'ssa-call) (tag? (car is) 'ssa-go)
                  (tag? (car is) 'ssa-defer))
              (or (equal? (nf (car is) 'func) func-name)
                  (equal? (nf (car is) 'method) func-name)))
         pos)
        (else (loop (cdr is) (+ pos 1)))))))
```

**Step 5: Update `ordered` checker same-block branch**

In belief.scm, replace the line (around line 598):

```scheme
            ((= (car a-blocks) (car b-blocks)) 'same-block)
```

With:

```scheme
            ((= (car a-blocks) (car b-blocks))
             (let* ((blk-idx (car a-blocks))
                    (block (let find ((bs (if (pair? blocks) blocks '())))
                             (cond ((null? bs) #f)
                                   ((= (nf (car bs) 'index) blk-idx) (car bs))
                                   (else (find (cdr bs))))))
                    (pos-a (and block (find-call-position block op-a)))
                    (pos-b (and block (find-call-position block op-b))))
               (cond
                 ((or (not pos-a) (not pos-b)) 'unordered)
                 ((< pos-a pos-b) 'a-dominates-b)
                 (else 'b-dominates-a))))
```

**Step 6: Run the new test**

Run: `go test ./goast/ -run TestBeliefCategory4_SameBlockOrdering -v -timeout 120s`
Expected: PASS

**Step 7: Run all category tests**

Run: `go test ./goast/ -run "TestBeliefCategory" -v -timeout 120s`
Expected: All pass. The existing `TestBeliefCategory4_Ordering` may need an assertion update if PipelineReversed now returns `b-dominates-a` instead of `same-block`. Check and update if needed.

**Step 8: Commit**

```
fix(belief): ordered resolves same-block via instruction position

When both calls are in the same SSA block, compares instruction
positions instead of returning 'same-block. Eliminates vote
splitting between same-block and a-dominates-b in etcd results.
```

---

### Task 2: Fix `checked-before-use` — transitive def-use reachability

Replace the current 1-hop check (value -> binop -> if) with bounded transitive reachability on the SSA def-use graph, up to 4 hops.

**Files:**
- Create: `examples/goast-query/testdata/fieldguard/fieldguard.go`
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm`
- Modify: `goast/belief_integration_test.go`

**Step 1: Create the testdata package**

Create `examples/goast-query/testdata/fieldguard/fieldguard.go`:

```go
package fieldguard

import "log"

type Request struct {
	Valid bool
	Data  string
}

func HandleSafeA(r Request) string {
	if !r.Valid {
		return "invalid"
	}
	return r.Data
}

func HandleSafeB(r Request) string {
	if !r.Valid {
		log.Println("invalid request")
		return ""
	}
	return r.Data
}

func HandleSafeC(r Request) string {
	if !r.Valid {
		return "bad"
	}
	return r.Data
}

func HandleSafeD(r Request) string {
	if !r.Valid {
		return "err"
	}
	return r.Data
}

// HandleUnsafe uses r without checking Valid — intentional deviation.
func HandleUnsafe(r Request) string {
	log.Println("data:", r.Data)
	return r.Data
}
```

**Step 2: Write the failing test**

Add to `goast/belief_integration_test.go`:

```go
func TestBeliefCategory2_FieldGuard(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast belief))

		(define ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/fieldguard"))

		(define checker (checked-before-use "r"))
		(define sites ((functions-matching (has-params "Request")) ctx))
		(define classified
		  (map (lambda (site) (cons (nf site 'name) (checker site ctx)))
		       sites))
	`)

	t.Run("5 sites found", func(t *testing.T) {
		total := eval(t, engine, `(length sites)`)
		qt.New(t).Assert(total.SchemeString(), qt.Equals, "5")
	})

	t.Run("4 guarded", func(t *testing.T) {
		count := eval(t, engine, `
			(length (filter-map (lambda (p) (and (eq? (cdr p) 'guarded) p)) classified))
		`)
		qt.New(t).Assert(count.SchemeString(), qt.Equals, "4")
	})

	t.Run("1 unguarded", func(t *testing.T) {
		count := eval(t, engine, `
			(length (filter-map (lambda (p) (and (eq? (cdr p) 'unguarded) p)) classified))
		`)
		qt.New(t).Assert(count.SchemeString(), qt.Equals, "1")
	})
}
```

**Step 3: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefCategory2_FieldGuard -v -timeout 120s`
Expected: FAIL — all 5 sites return `'unguarded` because the 1-hop check can't follow `r -> field-addr -> load -> unop -> if`.

**Step 4: Replace `checked-before-use` with bounded reachability implementation**

Replace the entire `checked-before-use` function (around lines 650-706 of belief.scm) with:

```scheme
;; (checked-before-use value-pattern) — checks whether a value is
;; tested before use via bounded transitive reachability on the
;; SSA def-use graph. Each iteration expands the tracked name set
;; hops. If any ssa-if is reached, the value is guarded.
;; Covers: direct comparison (if err != nil), field access
;; (if m.Type == x), and any chain up to 4 hops.
;; Returns: 'guarded or 'unguarded
(define (checked-before-use value-pattern)
  (define max-depth 4)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if (not ssa-fn) 'unguarded
        (let* ((blocks (nf ssa-fn 'blocks))
               (all-instrs (if (pair? blocks)
                             (flat-map
                               (lambda (b) (let ((is (nf b 'instrs)))
                                             (if (pair? is) is '())))
                               blocks)
                             '())))
          ;; Bounded Kleene iteration: expand the tracked name set through
          ;; the def-use chain. Each round finds instructions whose operands
          ;; intersect the tracked set, adds their output names, checks for ssa-if.
          (let chase ((tracked (list value-pattern)) (depth 0))
            (if (> depth max-depth) 'unguarded
              ;; Find all instructions using any tracked name as operand.
              (let* ((reached (filter-map
                                (lambda (instr)
                                  (let ((ops (nf instr 'operands)))
                                    (and (pair? ops)
                                         (let check ((ts tracked))
                                           (cond ((null? ts) #f)
                                                 ((member? (car ts) ops) instr)
                                                 (else (check (cdr ts))))))))
                                all-instrs))
                     ;; Check if any reached instruction is an ssa-if.
                     (found-guard (let loop ((rs reached))
                                   (cond ((null? rs) #f)
                                         ((tag? (car rs) 'ssa-if) #t)
                                         (else (loop (cdr rs))))))
                     ;; Collect output names for the next iteration.
                     (new-names (filter-map
                                  (lambda (instr)
                                    (let ((nm (nf instr 'name)))
                                      (and nm (not (member? nm tracked)) nm)))
                                  reached)))
                (cond
                  (found-guard 'guarded)
                  ((null? new-names) 'unguarded)
                  (else (chase (append tracked new-names) (+ depth 1))))))))))))
```

**Step 5: Run the new test**

Run: `go test ./goast/ -run TestBeliefCategory2_FieldGuard -v -timeout 120s`
Expected: PASS

**Step 6: Run existing category 2 test**

Run: `go test ./goast/ -run TestBeliefCategory2_Check -v -timeout 120s`
Expected: PASS — the bounded reachability subsumes the old 1-hop check.

**Step 7: Run all category tests**

Run: `go test ./goast/ -run "TestBeliefCategory" -v -timeout 120s`
Expected: All pass.

**Step 8: Commit**

```
fix(belief): checked-before-use uses transitive def-use reachability

Replaces 1-hop (value -> binop -> if) with bounded transitive
reachability on the SSA def-use graph (up to 4 hops). Handles
struct field guard patterns
like `if m.Type == x` where the chain is:
  m -> store -> field-addr -> load -> call/binop -> if
```

---

### Task 3: Re-run etcd validation and document results

**Step 1: Re-run etcd lock-beliefs.scm**

Run from `~/projects/etcd`:
```
/path/to/dist/wile-goast -f examples/etcd/lock-beliefs.scm
```

Expected: `lock-before-unlock` belief should now be strong — sites that were `same-block` should now be `a-dominates-b`.

**Step 2: Re-run raft-check-beliefs.scm**

Run from `~/projects/etcd/raft`:
```
/path/to/dist/wile-goast -f examples/etcd/raft-check-beliefs.scm
```

Expected: Some sites should now be `guarded` instead of all `unguarded`. The reachability traversal should follow `m -> store -> field-addr -> load -> call/binop -> if` chains.

**Step 3: Update `plans/CONSISTENCY-DEVIATION.md`**

Update the etcd raft section with the new results. Note which beliefs improved and by how much.

**Step 4: Commit**

```
docs: updated etcd validation results after checker improvements
```

---

### Task 4: Full test suite verification

**Step 1: Run all tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: All pass, coverage >= 80%.

**Step 2: Commit if needed**

```
test: coverage for checker improvements
```
