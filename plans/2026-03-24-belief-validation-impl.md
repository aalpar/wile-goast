# Belief Categories 1-4 Validation — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Validate belief DSL categories 1-4 (pairing, check, handling, ordering) against synthetic testdata and etcd raft, fixing DSL bugs discovered during validation.

**Architecture:** Four synthetic Go testdata packages exercise each checker with planted deviations. Go integration tests call `evaluate-belief` directly to verify structured results. Three checker bugs are fixed: `ordered` (wrong IR layer), `callers-of` (wrong site format), `checked-before-use` (missing data-flow step). etcd raft scripts validate real-world signal.

**Tech Stack:** Go testdata packages, R7RS Scheme belief scripts, Go integration tests using `newBeliefEngine(t)` + the `eval` test helper from `prim_goast_test.go`.

---

### Task 1: Create testdata packages

Create four Go packages under `examples/goast-query/testdata/`, one per belief category. Each has 4-5 functions with a clear majority pattern and 1 intentional deviation.

**Files:**
- Create: `examples/goast-query/testdata/pairing/pairing.go`
- Create: `examples/goast-query/testdata/ordering/ordering.go`
- Create: `examples/goast-query/testdata/handling/handling.go`
- Create: `examples/goast-query/testdata/checking/checking.go`

**Step 1: Create `pairing/pairing.go`**

Category 1 — Lock/Unlock pairing. 5 functions calling `Lock()`. 4 use `defer Unlock()` (paired-defer), 1 omits Unlock (unpaired).

```go
package pairing

import "sync"

type Service struct {
	mu   sync.Mutex
	data map[string]int
}

func (s *Service) ReadSafe() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data["key"]
}

func (s *Service) WriteSafe(k string, v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[k] = v
}

func (s *Service) UpdateSafe(k string, v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[k]; ok {
		s.data[k] = v
	}
}

func (s *Service) DeleteSafe(k string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, k)
}

// ReadUnsafe is an intentional deviation: Lock without Unlock.
func (s *Service) ReadUnsafe() int {
	s.mu.Lock()
	return s.data["key"]
}
```

**Step 2: Create `ordering/ordering.go`**

Category 4 — Validate before Process. 5 functions calling both. 4 have Validate in an early-return guard (Validate's block dominates Process's block), 1 calls Process before the guard.

The code uses early-return guards to ensure Validate and Process end up in different SSA blocks (same-block would not test dominance).

```go
package ordering

import "errors"

var ErrInvalid = errors.New("invalid")

func Validate(data string) error {
	if len(data) == 0 {
		return ErrInvalid
	}
	return nil
}

func Process(data string) string {
	return data + " processed"
}

func PipelineA(data string) (string, error) {
	if err := Validate(data); err != nil {
		return "", err
	}
	return Process(data), nil
}

func PipelineB(data string) (string, error) {
	if err := Validate(data); err != nil {
		return "", err
	}
	return Process(data), nil
}

func PipelineC(data string) (string, error) {
	if err := Validate(data); err != nil {
		return "", err
	}
	return Process(data), nil
}

func PipelineD(data string) (string, error) {
	if err := Validate(data); err != nil {
		return "", err
	}
	return Process(data), nil
}

// PipelineReversed calls Process before Validate — intentional deviation.
func PipelineReversed(data string) (string, error) {
	result := Process(data)
	if err := Validate(data); err != nil {
		return "", err
	}
	return result, nil
}
```

**Step 3: Create `handling/handling.go`**

Category 3 — callers of `DoWork` should wrap errors. 5 callers: 4 call `fmt.Errorf` to wrap, 1 ignores the error.

```go
package handling

import "fmt"

func DoWork(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("empty input")
	}
	return input + " done", nil
}

func CallerA(input string) (string, error) {
	result, err := DoWork(input)
	if err != nil {
		return "", fmt.Errorf("callerA: %w", err)
	}
	return result, nil
}

func CallerB(input string) (string, error) {
	result, err := DoWork(input)
	if err != nil {
		return "", fmt.Errorf("callerB: %w", err)
	}
	return result, nil
}

func CallerC(input string) (string, error) {
	result, err := DoWork(input)
	if err != nil {
		return "", fmt.Errorf("callerC: %w", err)
	}
	return result, nil
}

func CallerD(input string) (string, error) {
	result, err := DoWork(input)
	if err != nil {
		return "", fmt.Errorf("callerD: %w", err)
	}
	return result, nil
}

// CallerBad ignores the error from DoWork — intentional deviation.
func CallerBad(input string) string {
	result, _ := DoWork(input)
	return result
}
```

**Step 4: Create `checking/checking.go`**

Category 2 — functions receiving an `error` parameter should check it before use. 5 functions with `err error` parameter: 4 guard with `if err != nil`, 1 uses err without checking.

Using a parameter (not a local variable) because SSA preserves parameter names in operands. Local variables get register names like `t0` which the checker can't match by source name.

```go
package checking

import "log"

func HandleSafeA(err error) string {
	if err != nil {
		log.Println("error:", err)
		return "error"
	}
	return "ok"
}

func HandleSafeB(err error) string {
	if err != nil {
		return err.Error()
	}
	return "ok"
}

func HandleSafeC(err error) string {
	if err != nil {
		log.Println(err)
		return "failed"
	}
	return "ok"
}

func HandleSafeD(err error) string {
	if err != nil {
		return "err: " + err.Error()
	}
	return "ok"
}

// HandleUnsafe uses err without checking — intentional deviation.
func HandleUnsafe(err error) string {
	log.Println("processing:", err)
	return "done"
}
```

**Step 5: Verify all packages compile**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build ./examples/goast-query/testdata/...`
Expected: Success, no errors.

**Step 6: Commit**

```
test: synthetic testdata packages for belief categories 1-4

Four Go packages with planted deviations: pairing (Lock/Unlock),
ordering (Validate/Process), handling (error wrapping), checking
(nil guard). Each has 4 correct + 1 deviant function.
```

---

### Task 2: Category 1 — Pairing validation

The `paired-with` checker is the most straightforward. Validate it works correctly.

**Files:**
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the test**

Add to `goast/belief_integration_test.go`. Uses `newBeliefEngine(t)` from the same file and the `eval` helper from `prim_goast_test.go`.

```go
func TestBeliefCategory1_Pairing(t *testing.T) {
	engine := newBeliefEngine(t)

	// evaluate-belief returns:
	//   (name majority-cat ratio total adherence-sites deviation-sites)
	result := eval(t, engine, `
		(import (wile goast belief))

		(define-belief "test-lock-unlock"
		  (sites (functions-matching (contains-call "Lock")))
		  (expect (paired-with "Lock" "Unlock"))
		  (threshold 0.66 3))

		(let* ((ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing"))
		       (belief (car *beliefs*))
		       (result (evaluate-belief belief ctx)))
		  result)
	`)
	c := qt.New(t)

	// Result should be a list starting with the belief name.
	c.Assert(result.SchemeString(), qt.Matches, `.*test-lock-unlock.*`)

	t.Run("majority is paired-defer", func(t *testing.T) {
		maj := eval(t, engine, `(list-ref result 1)`)
		qt.New(t).Assert(maj.SchemeString(), qt.Equals, "paired-defer")
	})

	t.Run("5 sites found", func(t *testing.T) {
		total := eval(t, engine, `(list-ref result 3)`)
		qt.New(t).Assert(total.SchemeString(), qt.Equals, "5")
	})

	t.Run("1 deviation", func(t *testing.T) {
		devs := eval(t, engine, `(length (list-ref result 5))`)
		qt.New(t).Assert(devs.SchemeString(), qt.Equals, "1")
	})

	t.Run("deviation is ReadUnsafe", func(t *testing.T) {
		devName := eval(t, engine, `
			(let ((dev (car (list-ref result 5))))
			  (nf (car dev) 'name))
		`)
		qt.New(t).Assert(devName.SchemeString(), qt.Equals, `"ReadUnsafe"`)
	})
}
```

Note: `result` is bound in the engine's Scheme environment by the first call. Subsequent calls in subtests reuse the same engine, so `result` is accessible.

**Step 2: Run the test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefCategory1_Pairing -v -timeout 120s`
Expected: PASS — `paired-with` is well-implemented and should work correctly.

**Step 3: Commit**

```
test(belief): category 1 pairing validation — paired-with checker

Validates Lock/Unlock pairing against synthetic testdata.
Majority: paired-defer (4/5), deviation: ReadUnsafe (unpaired).
```

---

### Task 3: Fix `ordered` checker

The `ordered` checker is broken — it uses `go-cfg` which returns blocks without statement content, and passes cfg directly to `go-cfg-dominates?` which expects a dom-tree. The fix: use SSA representation (blocks have `instrs` and `idom` fields) instead of CFG.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm`
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the failing test**

Add to `goast/belief_integration_test.go`:

```go
func TestBeliefCategory4_Ordering(t *testing.T) {
	engine := newBeliefEngine(t)

	result := eval(t, engine, `
		(import (wile goast belief))

		(define-belief "test-validate-before-process"
		  (sites (functions-matching
		           (all-of (contains-call "Validate")
		                   (contains-call "Process"))))
		  (expect (ordered "Validate" "Process"))
		  (threshold 0.66 3))

		(let* ((ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/ordering"))
		       (belief (car *beliefs*))
		       (result (evaluate-belief belief ctx)))
		  result)
	`)
	c := qt.New(t)
	c.Assert(result.SchemeString(), qt.Matches, `.*test-validate-before-process.*`)

	t.Run("majority is a-dominates-b", func(t *testing.T) {
		maj := eval(t, engine, `(list-ref result 1)`)
		qt.New(t).Assert(maj.SchemeString(), qt.Equals, "a-dominates-b")
	})

	t.Run("5 sites found", func(t *testing.T) {
		total := eval(t, engine, `(list-ref result 3)`)
		qt.New(t).Assert(total.SchemeString(), qt.Equals, "5")
	})

	t.Run("1 deviation", func(t *testing.T) {
		devs := eval(t, engine, `(length (list-ref result 5))`)
		qt.New(t).Assert(devs.SchemeString(), qt.Equals, "1")
	})
}
```

**Step 2: Run test to confirm it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefCategory4_Ordering -v -timeout 120s`
Expected: FAIL — `ordered` returns `'missing` for all sites because CFG blocks have no `stmts` field and `go-cfg-dominates?` receives wrong input type.

**Step 3: Fix the `ordered` checker and helpers in `belief.scm`**

Replace `find-call-blocks` (lines 567-581) with `find-ssa-call-blocks`:

```scheme
;; Find SSA block indices containing a call to the named function.
;; Walks SSA instructions (not AST statements). Checks both static
;; calls (func field) and method calls (method field).
(define (find-ssa-call-blocks blocks func-name)
  (filter-map
    (lambda (block)
      (let ((idx (nf block 'index))
            (instrs (or (nf block 'instrs) '())))
        (and (pair? (walk instrs
               (lambda (node)
                 (and (or (tag? node 'ssa-call) (tag? node 'ssa-go)
                          (tag? node 'ssa-defer))
                      (or (equal? (nf node 'func) func-name)
                          (equal? (nf node 'method) func-name))))))
             idx)))
    (if (pair? blocks) blocks '())))
```

Add `ssa-dominates?` helper (after `find-ssa-call-blocks`):

```scheme
;; Check whether block a-idx dominates block b-idx using the idom chain.
;; Walks b's immediate dominator chain upward. If a is encountered, a dominates b.
(define (ssa-dominates? blocks a-idx b-idx)
  (let ((block-map (map (lambda (b) (cons (nf b 'index) b))
                        (if (pair? blocks) blocks '()))))
    (let loop ((current b-idx))
      (cond
        ((= current a-idx) #t)
        (else
          (let ((entry (assoc current block-map)))
            (if (not entry) #f
              (let ((idom (nf (cdr entry) 'idom)))
                (if (or (not idom) (eq? idom #f) (= idom current))
                  #f
                  (loop idom))))))))))
```

Replace the `ordered` checker (lines 550-564):

```scheme
;; (ordered op-a op-b) — checks whether op-a's SSA block dominates op-b's block.
;; Uses SSA representation (blocks have instrs + idom). Does not require go-cfg.
;; Returns: 'a-dominates-b, 'b-dominates-a, 'same-block, 'unordered, or 'missing
(define (ordered op-a op-b)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (ssa-fn (and pkg-path (ctx-find-ssa-func ctx pkg-path fname))))
      (if (not ssa-fn) 'missing
        (let* ((blocks (nf ssa-fn 'blocks))
               (a-blocks (find-ssa-call-blocks blocks op-a))
               (b-blocks (find-ssa-call-blocks blocks op-b)))
          (cond
            ((or (null? a-blocks) (null? b-blocks)) 'missing)
            ((= (car a-blocks) (car b-blocks)) 'same-block)
            ((ssa-dominates? blocks (car a-blocks) (car b-blocks)) 'a-dominates-b)
            ((ssa-dominates? blocks (car b-blocks) (car a-blocks)) 'b-dominates-a)
            (else 'unordered)))))))
```

Delete old `find-call-blocks` (lines 567-581) — it operated on CFG blocks which lack instructions. Nothing else calls it.

**Step 4: Run the test to confirm it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefCategory4_Ordering -v -timeout 120s`
Expected: PASS

**Step 5: Run category 1 test to confirm no regression**

Run: `go test ./goast/ -run "TestBeliefCategory[14]" -v -timeout 120s`
Expected: Both pass.

**Step 6: Commit**

```
fix(belief): ordered checker — use SSA blocks instead of CFG

The ordered checker was broken: go-cfg blocks lack statement content,
and go-cfg-dominates? requires a dom-tree (not cfg). Fixed by using
the SSA representation directly — blocks have instrs (for call
detection) and idom (for dominance checking).

Adds find-ssa-call-blocks and ssa-dominates? helpers.
Removes find-call-blocks (operated on CFG blocks without content).
```

---

### Task 4: Fix `callers-of` selector

The `callers-of` selector returns `(caller-name edge)` pairs, but all checkers expect func-decl AST nodes with a `body` field. Fix: look up the caller's AST func-decl from the loaded packages.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm`
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the failing test**

Add to `goast/belief_integration_test.go`:

```go
func TestBeliefCategory3_Handling(t *testing.T) {
	engine := newBeliefEngine(t)

	result := eval(t, engine, `
		(import (wile goast belief))

		(define-belief "test-dowork-error-wrapping"
		  (sites (callers-of "DoWork"))
		  (expect (contains-call "Errorf"))
		  (threshold 0.66 3))

		(let* ((ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/handling"))
		       (belief (car *beliefs*))
		       (result (evaluate-belief belief ctx)))
		  result)
	`)
	c := qt.New(t)
	c.Assert(result.SchemeString(), qt.Matches, `.*test-dowork-error-wrapping.*`)

	t.Run("majority is present", func(t *testing.T) {
		maj := eval(t, engine, `(list-ref result 1)`)
		qt.New(t).Assert(maj.SchemeString(), qt.Equals, "present")
	})

	t.Run("5 sites found", func(t *testing.T) {
		total := eval(t, engine, `(list-ref result 3)`)
		qt.New(t).Assert(total.SchemeString(), qt.Equals, "5")
	})

	t.Run("1 deviation", func(t *testing.T) {
		devs := eval(t, engine, `(length (list-ref result 5))`)
		qt.New(t).Assert(devs.SchemeString(), qt.Equals, "1")
	})
}
```

**Step 2: Run test to confirm it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefCategory3_Handling -v -timeout 120s`
Expected: FAIL — `callers-of` returns `(name edge)` pairs; `contains-call` calls `(nf site 'body)` which returns `#f`, so all sites are classified as `'absent`.

**Step 3: Fix `callers-of` to return func-decls**

Replace `callers-of` (lines 247-253) in `belief.scm`:

```scheme
;; (callers-of func-name) -> (lambda (ctx) -> list-of-func-decls)
;; Finds all callers of func-name via the call graph, then looks up
;; each caller's AST func-decl from the loaded packages. Callers
;; without an AST func-decl (e.g., generated code) are skipped.
(define (callers-of func-name)
  (lambda (ctx)
    (let* ((cg (ctx-callgraph ctx))
           (edges (go-callgraph-callers cg func-name))
           (funcs (all-func-decls (ctx-pkgs ctx))))
      (if (pair? edges)
        (filter-map
          (lambda (e)
            (let ((caller (nf e 'caller)))
              (and caller
                   (let ((short (ssa-short-name caller)))
                     (let loop ((fs funcs))
                       (cond ((null? fs) #f)
                             ((equal? (nf (car fs) 'name) short) (car fs))
                             (else (loop (cdr fs)))))))))
          edges)
        '()))))
```

This returns func-decl nodes (with `body`, `name`, `pkg-path` fields), making them compatible with all checkers.

Also clean up `site-display-name` — remove the dead `(caller-name edge)` branch (lines 699-703). Replace the whole function (lines 689-704):

```scheme
(define (site-display-name site)
  (cond
    ((and (pair? site) (tag? site 'func-decl))
     (let ((name (or (nf site 'name) "<anonymous>"))
           (impl-type (nf site 'impl-type))
           (pkg-path (nf site 'pkg-path)))
       (cond
         (impl-type (string-append impl-type "." name))
         (pkg-path (string-append (package-short-name pkg-path) "." name))
         (else name))))
    (else
     (display-to-string site))))
```

**Step 4: Run the test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefCategory3_Handling -v -timeout 120s`
Expected: PASS

**Step 5: Run all belief tests**

Run: `go test ./goast/ -run "TestBelief" -v -timeout 120s`
Expected: All pass. No regressions — the old `callers-of` format was never tested.

**Step 6: Commit**

```
fix(belief): callers-of returns func-decls instead of edge pairs

The callers-of selector returned (name edge) pairs incompatible with
all checkers that expect func-decl nodes. Now looks up each caller's
AST func-decl from loaded packages via ssa-short-name matching.
Removes dead (caller-name edge) branch from site-display-name.
```

---

### Task 5: Fix `checked-before-use` checker

The checker looks for `value-pattern` directly in `ssa-if` operands, but `if err != nil` compiles to `BinOp(err, nil) -> If(t0)`. The `ssa-if` contains `"t0"`, not `"err"`. Fix: follow one level of data flow — if the value appears in a comparison instruction whose result feeds an `ssa-if`, that's a guard.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/belief.scm`
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the failing test**

Add to `goast/belief_integration_test.go`:

```go
func TestBeliefCategory2_Check(t *testing.T) {
	engine := newBeliefEngine(t)

	result := eval(t, engine, `
		(import (wile goast belief))

		(define-belief "test-err-checked"
		  (sites (functions-matching (has-params "error")))
		  (expect (checked-before-use "err"))
		  (threshold 0.66 3))

		(let* ((ctx (make-context
		              "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking"))
		       (belief (car *beliefs*))
		       (result (evaluate-belief belief ctx)))
		  result)
	`)
	c := qt.New(t)
	c.Assert(result.SchemeString(), qt.Matches, `.*test-err-checked.*`)

	t.Run("majority is guarded", func(t *testing.T) {
		maj := eval(t, engine, `(list-ref result 1)`)
		qt.New(t).Assert(maj.SchemeString(), qt.Equals, "guarded")
	})

	t.Run("5 sites found", func(t *testing.T) {
		total := eval(t, engine, `(list-ref result 3)`)
		qt.New(t).Assert(total.SchemeString(), qt.Equals, "5")
	})

	t.Run("1 deviation", func(t *testing.T) {
		devs := eval(t, engine, `(length (list-ref result 5))`)
		qt.New(t).Assert(devs.SchemeString(), qt.Equals, "1")
	})

	t.Run("deviation is HandleUnsafe", func(t *testing.T) {
		devName := eval(t, engine, `
			(let ((dev (car (list-ref result 5))))
			  (nf (car dev) 'name))
		`)
		qt.New(t).Assert(devName.SchemeString(), qt.Equals, `"HandleUnsafe"`)
	})
}
```

**Step 2: Run test to confirm it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefCategory2_Check -v -timeout 120s`
Expected: FAIL — all functions classified as `'unguarded` because `ssa-if` operands contain the comparison result (`"t0"`), not the original value (`"err"`).

**Step 3: Fix `checked-before-use` in `belief.scm`**

Replace the checker (lines 600-623) with a two-level data-flow check:

```scheme
;; (checked-before-use value-pattern) — checks whether a value is
;; tested before use. Two-level check:
;;   1. Direct: value appears in ssa-if operands
;;   2. Indirect: value appears in a comparison (ssa-binop) whose
;;      result feeds an ssa-if (covers `if err != nil` pattern)
;; Returns: 'guarded or 'unguarded
(define (checked-before-use value-pattern)
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
                             '()))
               ;; Find all instructions that use value-pattern as an operand.
               (uses (filter-map
                       (lambda (instr)
                         (let ((ops (nf instr 'operands)))
                           (and (pair? ops) (member? value-pattern ops)
                                instr)))
                       all-instrs))
               ;; Direct guard: value appears in an ssa-if.
               (has-direct-guard
                 (let loop ((us uses))
                   (cond ((null? us) #f)
                         ((tag? (car us) 'ssa-if) #t)
                         (else (loop (cdr us))))))
               ;; Indirect guard: value appears in a comparison whose
               ;; result name feeds into an ssa-if.
               (comparison-names
                 (filter-map
                   (lambda (u)
                     (and (tag? u 'ssa-binop)
                          (nf u 'name)))
                   uses))
               (has-indirect-guard
                 (and (pair? comparison-names)
                      (let loop ((is all-instrs))
                        (cond
                          ((null? is) #f)
                          ((and (tag? (car is) 'ssa-if)
                                (let ((ops (nf (car is) 'operands)))
                                  (and (pair? ops)
                                       (let check ((ns comparison-names))
                                         (cond ((null? ns) #f)
                                               ((member? (car ns) ops) #t)
                                               (else (check (cdr ns))))))))
                           #t)
                          (else (loop (cdr is))))))))
          (if (or has-direct-guard has-indirect-guard)
            'guarded
            'unguarded))))))
```

**Step 4: Run the test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestBeliefCategory2_Check -v -timeout 120s`
Expected: PASS

**Step 5: Run all category tests**

Run: `go test ./goast/ -run "TestBeliefCategory" -v -timeout 120s`
Expected: All 4 pass.

**Step 6: Commit**

```
fix(belief): checked-before-use follows comparison data flow

The checker only looked for value-pattern directly in ssa-if
operands. But `if err != nil` compiles to BinOp(err, nil) -> If(t0),
so the ssa-if has "t0" not "err". Now also checks: value in
comparison -> comparison result in ssa-if (indirect guard).
```

---

### Task 6: Combined validation script

Write a standalone Scheme script that runs all four categories and prints results.

**Files:**
- Create: `examples/goast-query/belief-validate-categories.scm`

**Step 1: Write the script**

```scheme
;;; belief-validate-categories.scm — Validate belief categories 1-4
;;;
;;; Runs beliefs against synthetic testdata packages with known
;;; deviations. Each category should find exactly 1 deviation.
;;;
;;; Usage: wile-goast -f examples/goast-query/belief-validate-categories.scm

(import (wile goast belief))

;; ── Category 1: Pairing ──────────────────────────

(define-belief "cat1-lock-unlock"
  (sites (functions-matching (contains-call "Lock")))
  (expect (paired-with "Lock" "Unlock"))
  (threshold 0.66 3))

(run-beliefs
  "github.com/aalpar/wile-goast/examples/goast-query/testdata/pairing")

;; Reset registry for next category.

;; ── Category 2: Check ────────────────────────────

(set! *beliefs* '())

(define-belief "cat2-err-checked"
  (sites (functions-matching (has-params "error")))
  (expect (checked-before-use "err"))
  (threshold 0.66 3))

(run-beliefs
  "github.com/aalpar/wile-goast/examples/goast-query/testdata/checking")

;; ── Category 3: Handling ─────────────────────────

(set! *beliefs* '())

(define-belief "cat3-dowork-wrap"
  (sites (callers-of "DoWork"))
  (expect (contains-call "Errorf"))
  (threshold 0.66 3))

(run-beliefs
  "github.com/aalpar/wile-goast/examples/goast-query/testdata/handling")

;; ── Category 4: Ordering ─────────────────────────

(set! *beliefs* '())

(define-belief "cat4-validate-process"
  (sites (functions-matching
           (all-of (contains-call "Validate")
                   (contains-call "Process"))))
  (expect (ordered "Validate" "Process"))
  (threshold 0.66 3))

(run-beliefs
  "github.com/aalpar/wile-goast/examples/goast-query/testdata/ordering")
```

**Step 2: Run the script**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go run ./cmd/wile-goast -f examples/goast-query/belief-validate-categories.scm`

Expected: 4 belief reports, each showing 1 deviation:
- cat1: ReadUnsafe -> unpaired
- cat2: HandleUnsafe -> unguarded
- cat3: CallerBad -> absent
- cat4: PipelineReversed -> deviation category depends on SSA block layout

Record the exact output.

**Step 3: Commit**

```
feat(belief): validation script for categories 1-4

Runs all four belief categories against synthetic testdata.
Each finds the planted deviation, confirming checker correctness.
```

---

### Task 7: etcd raft validation scripts

Write new belief scripts for categories 2 and 3 against etcd raft.

**Files:**
- Create: `examples/etcd/raft-check-beliefs.scm`
- Create: `examples/etcd/raft-error-handling.scm`
- Existing (no changes): `examples/etcd/lock-beliefs.scm` (categories 1 & 4)
- Existing (no changes): `examples/etcd/raft-storage-consistency.scm` (category 3 variant)

**Step 1: Write `raft-check-beliefs.scm` (category 2)**

```scheme
;;; raft-check-beliefs.scm — Do raft functions guard values before use?
;;;
;;; Usage: cd etcd && wile-goast -f /path/to/raft-check-beliefs.scm

(import (wile goast belief))

;; Functions receiving a raftpb.Message should check msg.Type before processing.
(define-belief "raft-msg-type-guard"
  (sites (functions-matching (has-params "raftpb.Message")))
  (expect (checked-before-use "m"))
  (threshold 0.50 3))

(run-beliefs "go.etcd.io/raft/v3")
```

**Step 2: Write `raft-error-handling.scm` (category 3)**

```scheme
;;; raft-error-handling.scm — Do callers of raft.Step handle errors consistently?
;;;
;;; Usage: cd etcd && wile-goast -f /path/to/raft-error-handling.scm

(import (wile goast belief))

;; All callers of Step should check the returned error.
(define-belief "step-error-handling"
  (sites (callers-of "Step"))
  (expect (contains-call "Errorf" "Error" "Fatalf" "Warn"))
  (threshold 0.50 3))

(run-beliefs "go.etcd.io/raft/v3/...")
```

**Step 3: Run etcd validation (manual)**

These require an etcd checkout. Run each script from the etcd directory and record output.

Run (from etcd checkout):
```
wile-goast -f /path/to/lock-beliefs.scm             # cats 1 & 4
wile-goast -f /path/to/raft-storage-consistency.scm  # cat 3 variant
wile-goast -f /path/to/raft-check-beliefs.scm        # cat 2
wile-goast -f /path/to/raft-error-handling.scm       # cat 3
```

Record results — deviations found, adherence ratios, any checker limitations.

**Step 4: Commit**

```
feat(belief): etcd raft validation scripts for categories 2-3

New beliefs: raft message type guarding (checked-before-use),
Step error handling (callers-of + contains-call).
```

---

### Task 8: Document results and update plans

**Files:**
- Modify: `plans/CONSISTENCY-DEVIATION.md`
- Modify: `docs/PRIMITIVES.md` (if checker signatures changed)
- Modify: `CLAUDE.md` (update checker descriptions)

**Step 1: Update `CONSISTENCY-DEVIATION.md`**

Add a "Validation Results" section after the "Unvalidated Belief Categories" section with the table of results from synthetic testdata and etcd, plus the bugs-fixed section documenting the three fixes.

**Step 2: Update `docs/PRIMITIVES.md`**

Update the `ordered`, `callers-of`, and `checked-before-use` descriptions to reflect the fixed behavior.

**Step 3: Update `CLAUDE.md`**

Update the Belief DSL section to note that categories 1-4 are now validated. Mention `find-ssa-call-blocks` and `ssa-dominates?` as key helpers.

**Step 4: Commit**

```
docs: belief categories 1-4 validation results and bug fixes
```

---

### Task 9: Full test suite verification

**Step 1: Run all tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: All pass, coverage >= 80%.

**Step 2: If coverage is below threshold, add targeted tests**

The new integration tests should contribute to coverage. If needed, add tests for edge cases in the fixed checkers (e.g., function with no SSA match, empty blocks).

**Step 3: Final commit if needed**

```
test: coverage for belief validation
```
