# C4: Path Algebra Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `(wile goast path-algebra)` — semiring-parameterized path computation over call graphs with lazy single-source caching.

**Architecture:** New Scheme library at `cmd/wile-goast/lib/wile/goast/path-algebra.{sld,scm}`. Tests in `goast/path_algebra_test.go` using `newBeliefEngine` (provides `StdLibFS` for `(wile algebra semiring)` + all goast extensions). Synthetic CG construction in Scheme for topology-specific tests; real CG comparison for boolean vs `go-callgraph-reachable`.

**Tech Stack:** R7RS Scheme, `(wile algebra semiring)`, `(wile goast utils)` for `nf`

---

### Task 1: Library skeleton + type predicate

**Files:**
- Create: `cmd/wile-goast/lib/wile/goast/path-algebra.sld`
- Create: `cmd/wile-goast/lib/wile/goast/path-algebra.scm`
- Create: `goast/path_algebra_test.go`

**Step 1: Write the failing test**

In `goast/path_algebra_test.go`:

```go
package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/aalpar/wile/values"
)

func TestPathAlgebra_TypePredicate(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	evalScheme(t, engine, `(import (wile goast path-algebra))`)

	// Construct a trivial CG: single node, no edges.
	evalScheme(t, engine, `
		(define cg (list (list 'cg-node
			(cons 'name "A") (cons 'id 0)
			(cons 'edges-in '()) (cons 'edges-out '()))))`)

	evalScheme(t, engine, `
		(import (wile algebra semiring))
		(define pa (make-path-analysis (boolean-semiring) cg #f))`)

	result := evalScheme(t, engine, `(path-analysis? pa)`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	result = evalScheme(t, engine, `(path-analysis? 42)`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)
}
```

Note: Check whether existing test helpers in `goast/` use `eval` or a
different name. The belief integration tests use a local `eval` function
(`goast/prim_goast_test.go`). Name the helper to match the existing
convention — if `eval` is already taken, use the same name they use.

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestPathAlgebra_TypePredicate -v -count=1`
Expected: FAIL — `(wile goast path-algebra)` library not found

**Step 3: Write the library skeleton**

`cmd/wile-goast/lib/wile/goast/path-algebra.sld`:

```scheme
(define-library (wile goast path-algebra)
  (export
    make-path-analysis
    path-analysis?
    path-query
    path-query-all)
  (import (wile algebra semiring)
          (wile goast utils))
  (include "path-algebra.scm"))
```

`cmd/wile-goast/lib/wile/goast/path-algebra.scm`:

```scheme
;;; (wile goast path-algebra) — Semiring path computation over call graphs
;;;
;;; Lazy single-source Bellman-Ford parameterized by semiring.
;;; Boolean semiring = reachability, tropical = shortest path,
;;; counting = path count.

;; --- Record type ---

(define-record-type <path-analysis>
  (make-path-analysis* semiring adjacency weight-fn cache)
  path-analysis?
  (semiring   pa-semiring)
  (adjacency  pa-adjacency)
  (weight-fn  pa-weight-fn)
  (cache      pa-cache set-pa-cache!))

;; --- Adjacency construction ---

;; Build adjacency alist from CG node list: ((name . ((callee . edge) ...)) ...)
(define (build-adjacency cg)
  (let loop ((nodes cg) (adj '()))
    (if (null? nodes) adj
        (let* ((node (car nodes))
               (name (nf node 'name))
               (edges-out (nf node 'edges-out))
               (targets (map (lambda (e) (cons (nf e 'callee) e)) edges-out)))
          (loop (cdr nodes) (cons (cons name targets) adj))))))

;; --- Constructor ---

(define (make-path-analysis semiring cg edge-weight)
  (let ((adj (build-adjacency cg))
        (wfn (or edge-weight (lambda (_) (semiring-one semiring)))))
    (make-path-analysis* semiring adj wfn '())))

;; --- Stub query (Task 2 implements) ---

(define (path-query pa source target)
  (error "path-query: not yet implemented"))

(define (path-query-all pa source)
  (error "path-query-all: not yet implemented"))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestPathAlgebra_TypePredicate -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(path-algebra): library skeleton with record type and adjacency builder
```

---

### Task 2: Single-source Bellman-Ford + path-query

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/path-algebra.scm` — replace stubs
- Modify: `goast/path_algebra_test.go` — add tests

**Step 1: Write the failing test**

Append to `goast/path_algebra_test.go`:

```go
func TestPathAlgebra_BooleanLinearChain(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B -> C (linear chain)
	evalScheme(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))))
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static"))))
				(cons 'edges-out '()))))
		(define pa (make-path-analysis (boolean-semiring) cg #f))`)

	// A reaches C
	result := evalScheme(t, engine, `(path-query pa "A" "C")`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// A reaches B
	result = evalScheme(t, engine, `(path-query pa "A" "B")`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// A reaches A (self)
	result = evalScheme(t, engine, `(path-query pa "A" "A")`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// C does not reach A
	result = evalScheme(t, engine, `(path-query pa "C" "A")`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestPathAlgebra_BooleanLinearChain -v -count=1`
Expected: FAIL — "path-query: not yet implemented"

**Step 3: Implement single-source + path-query**

Replace the stub section in `path-algebra.scm` with:

```scheme
;; --- Single-source computation ---

;; Compute distances from source using worklist Bellman-Ford.
;; Returns alist ((name . value) ...) for all reachable nodes.
(define (compute-single-source pa source)
  (let ((S   (pa-semiring pa))
        (adj (pa-adjacency pa))
        (wfn (pa-weight-fn pa)))
    (let loop ((worklist (list source))
               (dist (list (cons source (semiring-one S)))))
      (if (null? worklist) dist
          (let* ((node (car worklist))
                 (rest (cdr worklist))
                 (node-dist (cdr (assoc node dist))))
            ;; Get outgoing edges for this node
            (let ((entry (assoc node adj)))
              (if (not entry)
                  (loop rest dist)
                  (let edge-loop ((edges (cdr entry))
                                  (wl rest)
                                  (d dist))
                    (if (null? edges)
                        (loop wl d)
                        (let* ((callee-name (caar edges))
                               (edge (cdar edges))
                               (w (wfn edge))
                               (candidate (semiring-times S node-dist w))
                               (old-entry (assoc callee-name d))
                               (old-val (if old-entry (cdr old-entry) (semiring-zero S)))
                               (merged (semiring-plus S old-val candidate)))
                          (if (equal? merged old-val)
                              (edge-loop (cdr edges) wl d)
                              (let ((new-d (cons (cons callee-name merged)
                                                 (if old-entry
                                                     (remove (lambda (p) (equal? (car p) callee-name)) d)
                                                     d))))
                                (edge-loop (cdr edges)
                                           (if (member callee-name wl) wl (cons callee-name wl))
                                           new-d)))))))))))))

;; --- Cache layer ---

(define (get-or-compute pa source)
  (let ((cached (assoc source (pa-cache pa))))
    (if cached (cdr cached)
        (let ((result (compute-single-source pa source)))
          (set-pa-cache! pa (cons (cons source result) (pa-cache pa)))
          result))))

;; --- Public API ---

(define (path-query pa source target)
  (let* ((dist (get-or-compute pa source))
         (entry (assoc target dist)))
    (if entry (cdr entry) (semiring-zero (pa-semiring pa)))))

(define (path-query-all pa source)
  (get-or-compute pa source))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestPathAlgebra_BooleanLinearChain -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(path-algebra): single-source Bellman-Ford with lazy caching
```

---

### Task 3: Tropical and counting semiring tests

**Files:**
- Modify: `goast/path_algebra_test.go` — add tropical + counting tests

**Step 1: Write the tests**

```go
func TestPathAlgebra_TropicalLinearChain(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B -> C (linear chain, unit weight)
	evalScheme(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))))
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static"))))
				(cons 'edges-out '()))))
		(define pa (make-path-analysis (tropical-semiring) cg (lambda (_) 1)))`)

	// A to A = 0 (source identity)
	result := evalScheme(t, engine, `(path-query pa "A" "A")`)
	c.Assert(result.Internal().(*values.Integer).GoInt(), qt.Equals, int64(0))

	// A to B = 1
	result = evalScheme(t, engine, `(path-query pa "A" "B")`)
	c.Assert(result.Internal().(*values.Integer).GoInt(), qt.Equals, int64(1))

	// A to C = 2
	result = evalScheme(t, engine, `(path-query pa "A" "C")`)
	c.Assert(result.Internal().(*values.Integer).GoInt(), qt.Equals, int64(2))

	// C to A = tropical-inf (unreachable)
	result = evalScheme(t, engine, `(eq? (path-query pa "C" "A") tropical-inf)`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}

func TestPathAlgebra_TropicalDiamond(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B -> C, A -> C (diamond: two paths, lengths 2 and 1)
	evalScheme(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list
					(list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))
					(list 'cg-edge (cons 'caller "A") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))))
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in (list
					(list 'cg-edge (cons 'caller "A") (cons 'callee "C") (cons 'description "static"))
					(list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static"))))
				(cons 'edges-out '()))))
		(define pa (make-path-analysis (tropical-semiring) cg (lambda (_) 1)))`)

	// Shortest path A to C = 1 (direct), not 2 (via B)
	result := evalScheme(t, engine, `(path-query pa "A" "C")`)
	c.Assert(result.Internal().(*values.Integer).GoInt(), qt.Equals, int64(1))
}

func TestPathAlgebra_CountingDiamond(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// Same diamond: A -> B -> C, A -> C — two distinct paths to C
	evalScheme(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list
					(list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))
					(list 'cg-edge (cons 'caller "A") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))))
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in (list
					(list 'cg-edge (cons 'caller "A") (cons 'callee "C") (cons 'description "static"))
					(list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static"))))
				(cons 'edges-out '()))))
		(define pa (make-path-analysis (counting-semiring) cg (lambda (_) 1)))`)

	// Two paths from A to C: direct + via B
	result := evalScheme(t, engine, `(path-query pa "A" "C")`)
	c.Assert(result.Internal().(*values.Integer).GoInt(), qt.Equals, int64(2))

	// One path from A to B
	result = evalScheme(t, engine, `(path-query pa "A" "B")`)
	c.Assert(result.Internal().(*values.Integer).GoInt(), qt.Equals, int64(1))
}
```

**Step 2: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestPathAlgebra_Tropical|TestPathAlgebra_Counting" -v -count=1`
Expected: PASS (implementation from Task 2 handles all semirings)

If tests fail, common issues:
- `tropical-inf` comparison: use `eq?` not `equal?` (it's a symbol)
- Counting semiring `times` is `*`: verify `semiring-one` (= 1) is initial weight

**Step 3: Commit**

```
test(path-algebra): tropical and counting semiring on linear chain and diamond
```

---

### Task 4: path-query-all + unreachable node tests

**Files:**
- Modify: `goast/path_algebra_test.go`

**Step 1: Write the tests**

```go
func TestPathAlgebra_QueryAll(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B -> C
	evalScheme(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))))
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "static"))))
				(cons 'edges-out '()))))
		(define pa (make-path-analysis (boolean-semiring) cg #f))`)

	// path-query-all returns alist of all reachable from A
	result := evalScheme(t, engine, `(length (path-query-all pa "A"))`)
	c.Assert(result.Internal().(*values.Integer).GoInt(), qt.Equals, int64(3)) // A, B, C

	// path-query-all from C returns only C itself
	result = evalScheme(t, engine, `(length (path-query-all pa "C"))`)
	c.Assert(result.Internal().(*values.Integer).GoInt(), qt.Equals, int64(1))
}

func TestPathAlgebra_Unreachable(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B, C isolated
	evalScheme(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "static"))))
				(cons 'edges-out '()))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in '())
				(cons 'edges-out '()))))
		(define pa (make-path-analysis (boolean-semiring) cg #f))`)

	// A does not reach C
	result := evalScheme(t, engine, `(path-query pa "A" "C")`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)

	// Nonexistent node returns semiring-zero
	result = evalScheme(t, engine, `(path-query pa "A" "Z")`)
	c.Assert(result.Internal(), qt.Equals, values.FalseValue)
}
```

**Step 2: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestPathAlgebra_(QueryAll|Unreachable)" -v -count=1`
Expected: PASS

**Step 3: Commit**

```
test(path-algebra): path-query-all and unreachable node coverage
```

---

### Task 5: Boolean reachability vs go-callgraph-reachable comparison

**Files:**
- Modify: `goast/path_algebra_test.go`

**Step 1: Write the test**

```go
func TestPathAlgebra_BooleanVsReachable(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// Build real CG on the goast package and compare boolean path-algebra
	// with the Go-native go-callgraph-reachable.
	evalScheme(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(import (wile goast utils))
		(define cg (go-callgraph "github.com/aalpar/wile-goast/goast" 'static))
		(define pa (make-path-analysis (boolean-semiring) cg #f))`)

	// Pick a known function as root.
	root := goastTestFunc

	evalScheme(t, engine, `
		(define root "`+root+`")
		(define go-reach (go-callgraph-reachable cg root))
		(define pa-reach (map car (path-query-all pa root)))`)

	// Every node in go-callgraph-reachable should be reachable via path-algebra.
	result := evalScheme(t, engine, `
		(let loop ((names go-reach))
			(cond
				((null? names) #t)
				((not (path-query pa root (car names)))
				 (car names))  ;; return the name that failed
				(else (loop (cdr names)))))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)

	// Both should have the same count.
	result = evalScheme(t, engine, `(= (length go-reach) (length pa-reach))`)
	c.Assert(result.Internal(), qt.Equals, values.TrueValue)
}
```

Note: `goastTestFunc` is defined in `goast/belief_integration_test.go` or
`goastcg/prim_callgraph_test.go`. Check which is visible from
`goast/path_algebra_test.go` (same package `goast_test`). If not visible,
define it locally.

**Step 2: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestPathAlgebra_BooleanVsReachable -v -count=1 -timeout 120s`
Expected: PASS

**Step 3: Commit**

```
test(path-algebra): boolean reachability matches go-callgraph-reachable
```

---

### Task 6: Custom edge-weight test

**Files:**
- Modify: `goast/path_algebra_test.go`

**Step 1: Write the test**

```go
func TestPathAlgebra_CustomWeight(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	// A -> B (weight 3) -> C (weight 5)
	evalScheme(t, engine, `
		(import (wile goast path-algebra))
		(import (wile algebra semiring))
		(import (wile goast utils))
		(define cg (list
			(list 'cg-node (cons 'name "A") (cons 'id 0)
				(cons 'edges-in '())
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "w3")))))
			(list 'cg-node (cons 'name "B") (cons 'id 1)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "A") (cons 'callee "B") (cons 'description "w3"))))
				(cons 'edges-out (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "w5")))))
			(list 'cg-node (cons 'name "C") (cons 'id 2)
				(cons 'edges-in (list (list 'cg-edge (cons 'caller "B") (cons 'callee "C") (cons 'description "w5"))))
				(cons 'edges-out '()))))

		;; Weight function uses description to assign edge weights.
		(define (edge-weight e)
			(let ((desc (nf e 'description)))
				(cond ((equal? desc "w3") 3)
				      ((equal? desc "w5") 5)
				      (else 1))))

		(define pa (make-path-analysis (tropical-semiring) cg edge-weight))`)

	// A to B = 3, A to C = 3+5 = 8
	result := evalScheme(t, engine, `(path-query pa "A" "B")`)
	c.Assert(result.Internal().(*values.Integer).GoInt(), qt.Equals, int64(3))

	result = evalScheme(t, engine, `(path-query pa "A" "C")`)
	c.Assert(result.Internal().(*values.Integer).GoInt(), qt.Equals, int64(8))
}
```

**Step 2: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestPathAlgebra_CustomWeight -v -count=1`
Expected: PASS

**Step 3: Commit**

```
test(path-algebra): custom edge-weight function with tropical semiring
```

---

### Task 7: Update TODO.md and CLAUDE.md

**Files:**
- Modify: `TODO.md` — check off C4 boolean and tropical items
- Modify: `CLAUDE.md` — add path-algebra to library table and primitives

**Step 1: Update TODO.md**

Change:
```
- [ ] Boolean semiring — reachability (generalize `go-callgraph-reachable`)
- [ ] Tropical semiring — shortest/longest call chains
```
To:
```
- [x] Boolean semiring — reachability (generalize `go-callgraph-reachable`)
- [x] Tropical semiring — shortest/longest call chains
```

CFL-reachability stays unchecked (deferred per design).

**Step 2: Update CLAUDE.md**

Add to the Scheme libraries / Key Files tables:
- `(wile goast path-algebra)` entry with exports
- File entries for `path-algebra.{sld,scm}`

**Step 3: Run full test suite**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./... -count=1 -timeout 300s`
Expected: All PASS

**Step 4: Commit**

```
docs: update TODO.md and CLAUDE.md for C4 path algebra
```
