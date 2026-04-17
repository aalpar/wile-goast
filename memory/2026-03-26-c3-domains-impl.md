# C3 — Pre-built Abstract Domains — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add five pre-built abstract domains (reaching definitions, liveness, constant propagation, sign, interval) as a single `(wile goast domains)` library that plugs into C2's `run-analysis`.

**Architecture:** Each domain is a factory function `make-<name>-analysis` that constructs a lattice and transfer function, calls `run-analysis`, and returns the result alist. Shared helpers (`go-concrete-eval`, `parse-ssa-const`) live alongside the domains. All pure Scheme — no Go changes.

**Tech Stack:** Scheme (R7RS), `(wile algebra)` lattice constructors, `(wile goast dataflow)` worklist analysis, SSA blocks from `go-ssa-build`

**Design doc:** `plans/2026-03-26-c3-domains-design.md`

---

### Task 1: Testdata and library scaffold

**Files:**
- Create: `examples/goast-query/testdata/arithmetic/arithmetic.go`
- Create: `cmd/wile-goast/lib/wile/goast/domains.sld`
- Create: `cmd/wile-goast/lib/wile/goast/domains.scm`
- Modify: `goast/belief_integration_test.go`

**Step 1: Create testdata**

Create `examples/goast-query/testdata/arithmetic/arithmetic.go`:

```go
package arithmetic

func Add(x, y int) int { return x + y }

func Negate(x int) int { return -x }

func Branch(x int) int {
	if x > 0 {
		return x + 1
	}
	return x - 1
}

func LoopSum(n int) int {
	sum := 0
	for i := 0; i < n; i++ {
		sum += i
	}
	return sum
}
```

**Step 2: Create library definition**

Create `cmd/wile-goast/lib/wile/goast/domains.sld`:

```scheme
(define-library (wile goast domains)
  (export
    go-concrete-eval
    make-reaching-definitions
    make-liveness
    make-constant-propagation
    sign-lattice
    make-sign-analysis
    interval-lattice
    make-interval-analysis)
  (import (wile algebra)
          (wile goast dataflow)
          (wile goast utils))
  (include "domains.scm"))
```

**Step 3: Create empty implementation**

Create `cmd/wile-goast/lib/wile/goast/domains.scm`:

```scheme
;;; (wile goast domains) — Pre-built abstract domains for C2 dataflow analysis
;;;
;;; Five domains: reaching definitions, liveness, constant propagation,
;;; sign analysis, interval analysis. All built on (wile goast dataflow)
;;; run-analysis and (wile algebra) lattice constructors.

;; Placeholder — implementations added in subsequent tasks.
(define (go-concrete-eval opcode a b) 'unknown)
(define (make-reaching-definitions ssa-fn) '())
(define (make-liveness ssa-fn) '())
(define (make-constant-propagation ssa-fn) '())
(define (sign-lattice) #f)
(define (make-sign-analysis ssa-fn) '())
(define (interval-lattice) #f)
(define (make-interval-analysis ssa-fn) '())
```

**Step 4: Write import test**

Add to `goast/belief_integration_test.go`:

```go
func TestDomainsImport(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `(import (wile goast domains))`)
}
```

**Step 5: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDomainsImport -v`
Expected: PASS

**Step 6: Commit**

```bash
git add examples/goast-query/testdata/arithmetic/arithmetic.go \
  cmd/wile-goast/lib/wile/goast/domains.sld \
  cmd/wile-goast/lib/wile/goast/domains.scm \
  goast/belief_integration_test.go
git commit -m "feat(domains): scaffold (wile goast domains) library with testdata"
```

---

### Task 2: Shared helpers and `go-concrete-eval`

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/domains.scm`
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the failing test**

Add to `goast/belief_integration_test.go`:

```go
func TestDomainsConcreteEval(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `(import (wile goast domains))`)

	t.Run("add", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'add 2 3)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "5")
	})

	t.Run("sub", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'sub 10 4)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "6")
	})

	t.Run("mul", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'mul 3 7)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "21")
	})

	t.Run("div", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'div 10 3)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "3")
	})

	t.Run("div-by-zero", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'div 10 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "unknown")
	})

	t.Run("rem", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'rem 10 3)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("unknown-opcode", func(t *testing.T) {
		result := eval(t, engine, `(go-concrete-eval 'shl 1 2)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "unknown")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDomainsConcreteEval -v`
Expected: FAIL — placeholder returns `'unknown` for everything

**Step 3: Implement helpers and `go-concrete-eval`**

Replace the placeholder content in `cmd/wile-goast/lib/wile/goast/domains.scm` with:

```scheme
;;; (wile goast domains) — Pre-built abstract domains for C2 dataflow analysis
;;;
;;; Five domains: reaching definitions, liveness, constant propagation,
;;; sign analysis, interval analysis. All built on (wile goast dataflow)
;;; run-analysis and (wile algebra) lattice constructors.

;; --- String helpers --------------------------

(define (find-char str ch)
  (let loop ((i 0))
    (cond ((>= i (string-length str)) #f)
          ((char=? (string-ref str i) ch) i)
          (else (loop (+ i 1))))))

;; --- SSA constant parsing --------------------

(define *integer-types*
  '("int" "int8" "int16" "int32" "int64"
    "uint" "uint8" "uint16" "uint32" "uint64"
    "untyped int"))

(define (parse-ssa-const name)
  ;; "3:int" -> 3, "-5:int64" -> -5, anything else -> #f
  (let ((colon (find-char name #\:)))
    (and colon
         (let ((type-str (substring name (+ colon 1) (string-length name))))
           (and (member type-str *integer-types*)
                (string->number (substring name 0 colon)))))))

;; --- Go token to opcode mapping --------------

(define (go-token->opcode sym)
  (case sym
    ((+) 'add) ((-) 'sub) ((*) 'mul)
    ((/) 'div) ((%) 'rem)
    ((&) 'and) ((|) 'or) ((^) 'xor)
    (else #f)))

;; --- Concrete evaluator ----------------------
;; Evaluates Go SSA opcodes on Scheme integers.
;; Returns integer result or 'unknown.
;; Minimal set: arithmetic + remainder. Bitwise deferred (no Wile support).

(define (go-concrete-eval opcode a b)
  (case opcode
    ((add) (+ a b))
    ((sub) (- a b))
    ((mul) (* a b))
    ((div) (if (zero? b) 'unknown (quotient a b)))
    ((rem) (if (zero? b) 'unknown (remainder a b)))
    (else  'unknown)))

;; --- State helpers for map-lattice analyses --

(define (state-update state key val)
  (map (lambda (entry)
         (if (equal? (car entry) key)
             (cons key val)
             entry))
       state))

(define (state-lookup state name)
  (let ((pair (assoc name state)))
    (if pair (cdr pair) #f)))

;; --- Resolve an SSA operand to a lattice value
;; For map-lattice analyses: look up in state, or parse as constant.
;; Returns: concrete value, 'flat-bottom (unknown), or 'flat-top (non-constant).

(define (resolve-value state name)
  (let ((in-state (state-lookup state name)))
    (cond (in-state in-state)
          ((parse-ssa-const name) =>
           (lambda (v) v))
          (else 'flat-top))))

;; --- Placeholder factories (replaced in subsequent tasks) --

(define (make-reaching-definitions ssa-fn) '())
(define (make-liveness ssa-fn) '())
(define (make-constant-propagation ssa-fn) '())
(define (sign-lattice) #f)
(define (make-sign-analysis ssa-fn) '())
(define (interval-lattice) #f)
(define (make-interval-analysis ssa-fn) '())
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDomainsConcreteEval -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/domains.scm goast/belief_integration_test.go
git commit -m "feat(domains): go-concrete-eval and SSA constant parsing helpers"
```

---

### Task 3: Reaching definitions (forward, powerset)

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/domains.scm`
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the failing test**

Add to `goast/belief_integration_test.go`:

```go
func TestDomainsReachingDefinitions(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile goast domains))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "./examples/goast-query/testdata/arithmetic"))
		(define fn-branch (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "Branch") (car fs))
		        (else (loop (cdr fs))))))

		(define rd-result (make-reaching-definitions fn-branch))
	`)

	t.Run("returns non-empty result", func(t *testing.T) {
		result := eval(t, engine, `(> (length rd-result) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("entry block out has definitions from block 0", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((out0 (analysis-out rd-result 0)))
			  (and (pair? out0) (> (length out0) 0)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("successor inherits predecessor definitions", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((out0 (analysis-out rd-result 0))
			      (in1  (analysis-in rd-result 1)))
			  ;; Every name in out0 should appear in in1
			  (let check ((ns out0))
			    (cond ((null? ns) #t)
			          ((member (car ns) in1) (check (cdr ns)))
			          (else #f))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDomainsReachingDefinitions -v`
Expected: FAIL -- placeholder returns `'()`

**Step 3: Implement**

In `cmd/wile-goast/lib/wile/goast/domains.scm`, replace the `make-reaching-definitions` placeholder with:

```scheme
;; --- Domain 1: Reaching Definitions (forward, powerset) --

(define (make-reaching-definitions ssa-fn)
  (let* ((universe (ssa-instruction-names ssa-fn))
         (lat (powerset-lattice universe))
         (transfer (lambda (block state)
                     (let ((names (filter-map
                                    (lambda (i) (nf i 'name))
                                    (block-instrs block))))
                       (lattice-join lat state names)))))
    (run-analysis 'forward lat transfer ssa-fn)))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDomainsReachingDefinitions -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/domains.scm goast/belief_integration_test.go
git commit -m "feat(domains): make-reaching-definitions forward powerset analysis"
```

---

### Task 4: Liveness (backward, powerset)

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/domains.scm`
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the failing test**

Add to `goast/belief_integration_test.go`:

```go
func TestDomainsLiveness(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile goast domains))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "./examples/goast-query/testdata/arithmetic"))
		(define fn-branch (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "Branch") (car fs))
		        (else (loop (cdr fs))))))

		(define live-result (make-liveness fn-branch))
	`)

	t.Run("returns non-empty result", func(t *testing.T) {
		result := eval(t, engine, `(> (length live-result) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("exit blocks have empty out-state", func(t *testing.T) {
		;; Exit blocks (no successors) should have out = bottom = ()
		result := eval(t, engine, `
			(let* ((blocks (nf fn-branch 'blocks))
			       (exits (filter
			                (lambda (b)
			                  (let ((s (nf b 'succs)))
			                    (or (not s) (null? s))))
			                blocks)))
			  (let check ((es exits))
			    (cond ((null? es) #t)
			          ((null? (analysis-out live-result (nf (car es) 'index)))
			           (check (cdr es)))
			          (else #f))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("entry block out reflects uses in successors", func(t *testing.T) {
		;; Block 0 should have non-empty out-state (names used downstream)
		result := eval(t, engine, `(> (length (analysis-out live-result 0)) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDomainsLiveness -v`
Expected: FAIL -- placeholder returns `'()`

**Step 3: Implement**

In `cmd/wile-goast/lib/wile/goast/domains.scm`, replace the `make-liveness` placeholder with:

```scheme
;; --- Domain 2: Liveness (backward, powerset) --

(define (make-liveness ssa-fn)
  (let* ((universe (ssa-instruction-names ssa-fn))
         (lat (powerset-lattice universe))
         (transfer
           (lambda (block state)
             (let loop ((instrs (reverse (block-instrs block)))
                        (st state))
               (if (null? instrs) st
                   (let* ((instr (car instrs))
                          (nm (nf instr 'name))
                          ;; Kill: remove defined name
                          (st1 (if nm
                                   (filter (lambda (x) (not (equal? x nm))) st)
                                   st))
                          ;; Gen: add used operands that are in universe
                          (ops (or (nf instr 'operands) '()))
                          (used (filter (lambda (o) (member o universe)) ops))
                          (st2 (lattice-join lat st1 used)))
                     (loop (cdr instrs) st2)))))))
    (run-analysis 'backward lat transfer ssa-fn)))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDomainsLiveness -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/domains.scm goast/belief_integration_test.go
git commit -m "feat(domains): make-liveness backward powerset analysis"
```

---

### Task 5: Constant propagation (forward, flat + map-lattice)

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/domains.scm`
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the failing test**

Add to `goast/belief_integration_test.go`:

```go
func TestDomainsConstantPropagation(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast domains))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "./examples/goast-query/testdata/arithmetic"))
		(define fn-add (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "Add") (car fs))
		        (else (loop (cdr fs))))))

		(define cp-result (make-constant-propagation fn-add))
	`)

	t.Run("returns non-empty result", func(t *testing.T) {
		result := eval(t, engine, `(> (length cp-result) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("parameter-derived binop is non-constant", func(t *testing.T) {
		;; Add(x, y) has t0 = x + y. Since x,y are params, t0 should be top.
		result := eval(t, engine, `
			(let* ((out0 (analysis-out cp-result 0))
			       (t0-val (assoc "t0" out0)))
			  (and t0-val (eq? (cdr t0-val) 'flat-top)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestDomainsConstantPropagationBranch(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast domains))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "./examples/goast-query/testdata/arithmetic"))
		(define fn-branch (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "Branch") (car fs))
		        (else (loop (cdr fs))))))

		(define cp-branch (make-constant-propagation fn-branch))
	`)

	t.Run("analysis completes on branching function", func(t *testing.T) {
		result := eval(t, engine, `(> (length cp-branch) 1)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("all states have alist structure", func(t *testing.T) {
		result := eval(t, engine, `
			(let check ((states (analysis-states cp-branch)))
			  (cond ((null? states) #t)
			        ((not (pair? (cadr (car states)))) #f)
			        (else (check (cdr states)))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestDomainsConstantPropagation" -v`
Expected: FAIL -- placeholder returns `'()`

**Step 3: Implement**

In `cmd/wile-goast/lib/wile/goast/domains.scm`, replace the `make-constant-propagation` placeholder with:

```scheme
;; --- Domain 3: Constant Propagation (forward, flat + map-lattice) --

(define (make-constant-propagation ssa-fn)
  (let* ((names (ssa-instruction-names ssa-fn))
         (val-lat (flat-lattice '() equal?))
         (lat (map-lattice names val-lat))
         (transfer
           (lambda (block state)
             (let loop ((instrs (block-instrs block)) (st state))
               (if (null? instrs) st
                   (let ((instr (car instrs)))
                     (loop (cdr instrs)
                       (cond
                         ;; BinOp: fold if both operands are concrete
                         ((tag? instr 'ssa-binop)
                          (let* ((nm (nf instr 'name))
                                 (op-sym (nf instr 'op))
                                 (opcode (go-token->opcode op-sym))
                                 (v1 (resolve-value st (nf instr 'x)))
                                 (v2 (resolve-value st (nf instr 'y)))
                                 (result
                                   (cond
                                     ((or (eq? v1 'flat-bottom) (eq? v2 'flat-bottom))
                                      'flat-bottom)
                                     ((or (eq? v1 'flat-top) (eq? v2 'flat-top))
                                      'flat-top)
                                     ((and opcode (integer? v1) (integer? v2))
                                      (let ((r (go-concrete-eval opcode v1 v2)))
                                        (if (eq? r 'unknown) 'flat-top r)))
                                     (else 'flat-top))))
                            (if nm (state-update st nm result) st)))

                         ;; Phi: join all operand values
                         ((tag? instr 'ssa-phi)
                          (let* ((nm (nf instr 'name))
                                 (ops (or (nf instr 'operands) '()))
                                 (result
                                   (let join-ops ((os ops) (acc 'flat-bottom))
                                     (if (null? os) acc
                                         (join-ops (cdr os)
                                           (lattice-join val-lat acc
                                             (resolve-value st (car os))))))))
                            (if nm (state-update st nm result) st)))

                         ;; Anything else with a name: conservative top
                         ((nf instr 'name)
                          (state-update st (nf instr 'name) 'flat-top))

                         ;; No name (store, jump, etc.): pass through
                         (else st)))))))))
    (run-analysis 'forward lat transfer ssa-fn)))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestDomainsConstantPropagation" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/domains.scm goast/belief_integration_test.go
git commit -m "feat(domains): make-constant-propagation with flat map-lattice"
```

---

### Task 6: Sign lattice and analysis (forward, 5-element)

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/domains.scm`
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the failing test**

Add to `goast/belief_integration_test.go`:

```go
func TestDomainsSignLattice(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast domains))
	`)

	t.Run("lattice validates", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sl (sign-lattice)))
			  (validate-lattice sl '(neg zero pos)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("join of incomparable is top", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sl (sign-lattice)))
			  (lattice-join sl 'neg 'pos))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "flat-top")
	})

	t.Run("meet of incomparable is bottom", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sl (sign-lattice)))
			  (lattice-meet sl 'neg 'pos))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "flat-bottom")
	})
}

func TestDomainsSignAnalysis(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast domains))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "./examples/goast-query/testdata/arithmetic"))
		(define fn-add (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "Add") (car fs))
		        (else (loop (cdr fs))))))

		(define sign-result (make-sign-analysis fn-add))
	`)

	t.Run("analysis completes", func(t *testing.T) {
		result := eval(t, engine, `(> (length sign-result) 0)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("parameter-derived binop is top", func(t *testing.T) {
		;; Add(x, y): t0 = x + y. Both params are top, so result is top.
		result := eval(t, engine, `
			(let* ((out0 (analysis-out sign-result 0))
			       (t0-val (assoc "t0" out0)))
			  (and t0-val (eq? (cdr t0-val) 'flat-top)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestDomainsSign" -v`
Expected: FAIL -- placeholder returns `#f`

**Step 3: Implement sign lattice and transfer tables**

In `cmd/wile-goast/lib/wile/goast/domains.scm`, replace the `sign-lattice` and `make-sign-analysis` placeholders with:

```scheme
;; --- Domain 4: Sign Analysis (forward, 5-element) --

;; Sign lattice: {flat-bottom, neg, zero, pos, flat-top}
;; Reuses flat-lattice since the three middle elements are incomparable.
(define (sign-lattice)
  (flat-lattice '(neg zero pos) eq?))

;; Abstract a concrete integer to its sign.
(define (abstract-sign n)
  (cond ((< n 0) 'neg)
        ((= n 0) 'zero)
        (else    'pos)))

;; Sign transfer tables: (op sign-a sign-b) -> sign-result
;; Only covers add, sub, mul. Others -> flat-top.
(define (sign-binop op a b)
  (let ((bot 'flat-bottom) (top 'flat-top))
    (cond
      ((or (eq? a bot) (eq? b bot)) bot)
      ((and (eq? op 'mul) (or (eq? a 'zero) (eq? b 'zero))) 'zero)
      ((or (eq? a top) (eq? b top)) top)
      (else
        (case op
          ((add)
           (case a
             ((neg)  (case b ((neg) 'neg)  ((zero) 'neg) ((pos) top)))
             ((zero) b)
             ((pos)  (case b ((neg) top)   ((zero) 'pos) ((pos) 'pos)))))
          ((sub)
           (case a
             ((neg)  (case b ((neg) top)   ((zero) 'neg) ((pos) 'neg)))
             ((zero) (case b ((neg) 'pos)  ((zero) 'zero) ((pos) 'neg)))
             ((pos)  (case b ((neg) 'pos)  ((zero) 'pos) ((pos) top)))))
          ((mul)
           (case a
             ((neg)  (case b ((neg) 'pos)  ((zero) 'zero) ((pos) 'neg)))
             ((zero) 'zero)
             ((pos)  (case b ((neg) 'neg)  ((zero) 'zero) ((pos) 'pos)))))
          (else top))))))

;; Resolve an operand to a sign value.
(define (resolve-sign state name)
  (let ((in-state (state-lookup state name)))
    (cond (in-state in-state)
          ((parse-ssa-const name) => abstract-sign)
          (else 'flat-top))))

(define (make-sign-analysis ssa-fn)
  (let* ((names (ssa-instruction-names ssa-fn))
         (s-lat (sign-lattice))
         (lat (map-lattice names s-lat))
         (transfer
           (lambda (block state)
             (let loop ((instrs (block-instrs block)) (st state))
               (if (null? instrs) st
                   (let ((instr (car instrs)))
                     (loop (cdr instrs)
                       (cond
                         ((tag? instr 'ssa-binop)
                          (let* ((nm (nf instr 'name))
                                 (opcode (go-token->opcode (nf instr 'op)))
                                 (v1 (resolve-sign st (nf instr 'x)))
                                 (v2 (resolve-sign st (nf instr 'y)))
                                 (result (if opcode
                                             (sign-binop opcode v1 v2)
                                             'flat-top)))
                            (if nm (state-update st nm result) st)))

                         ((tag? instr 'ssa-phi)
                          (let* ((nm (nf instr 'name))
                                 (ops (or (nf instr 'operands) '()))
                                 (result
                                   (let join-ops ((os ops) (acc 'flat-bottom))
                                     (if (null? os) acc
                                         (join-ops (cdr os)
                                           (lattice-join s-lat acc
                                             (resolve-sign st (car os))))))))
                            (if nm (state-update st nm result) st)))

                         ((nf instr 'name)
                          (state-update st (nf instr 'name) 'flat-top))

                         (else st)))))))))
    (run-analysis 'forward lat transfer ssa-fn)))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestDomainsSign" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/domains.scm goast/belief_integration_test.go
git commit -m "feat(domains): sign-lattice and make-sign-analysis with transfer tables"
```

---

### Task 7: Interval lattice (forward, custom lattice)

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/domains.scm`
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the failing test**

Add to `goast/belief_integration_test.go`:

```go
func TestDomainsIntervalLattice(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast domains))

		(define il (interval-lattice))
	`)

	t.Run("lattice validates on sample intervals", func(t *testing.T) {
		result := eval(t, engine, `
			(validate-lattice il
			  (list '(1 . 5) '(3 . 10) '(-2 . 2) '(0 . 0)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("join widens to encompass both", func(t *testing.T) {
		result := eval(t, engine, `
			(lattice-join il '(1 . 5) '(3 . 10))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(1 . 10)")
	})

	t.Run("meet narrows to intersection", func(t *testing.T) {
		result := eval(t, engine, `
			(lattice-meet il '(1 . 5) '(3 . 10))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(3 . 5)")
	})

	t.Run("empty meet is bottom", func(t *testing.T) {
		result := eval(t, engine, `
			(lattice-meet il '(1 . 3) '(5 . 10))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "interval-bot")
	})

	t.Run("bottom is join identity", func(t *testing.T) {
		result := eval(t, engine, `
			(lattice-join il (lattice-bottom il) '(2 . 5))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(2 . 5)")
	})

	t.Run("leq checks containment", func(t *testing.T) {
		result := eval(t, engine, `
			(and (lattice-leq? il '(2 . 5) '(1 . 10))
			     (not (lattice-leq? il '(1 . 10) '(2 . 5))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDomainsIntervalLattice -v`
Expected: FAIL -- placeholder returns `#f`

**Step 3: Implement interval lattice and arithmetic helpers**

In `cmd/wile-goast/lib/wile/goast/domains.scm`, replace the `interval-lattice` placeholder with:

```scheme
;; --- Domain 5: Interval Analysis (forward, widening) --

;; Infinity-aware comparison and arithmetic.

(define (inf<= a b)
  (cond ((eq? a '-inf) #t)
        ((eq? b '+inf) #t)
        ((eq? b '-inf) #f)
        ((eq? a '+inf) #f)
        (else (<= a b))))

(define (inf-min a b) (if (inf<= a b) a b))
(define (inf-max a b) (if (inf<= a b) b a))

(define (inf+ a b)
  (cond ((or (and (eq? a '+inf) (eq? b '-inf))
             (and (eq? a '-inf) (eq? b '+inf)))
         '+inf)
        ((or (eq? a '+inf) (eq? b '+inf)) '+inf)
        ((or (eq? a '-inf) (eq? b '-inf)) '-inf)
        (else (+ a b))))

(define (inf- a b)
  (cond ((or (and (eq? a '+inf) (eq? b '+inf))
             (and (eq? a '-inf) (eq? b '-inf)))
         '+inf)
        ((eq? a '+inf) '+inf)
        ((eq? a '-inf) '-inf)
        ((eq? b '+inf) '-inf)
        ((eq? b '-inf) '+inf)
        (else (- a b))))

(define (inf* a b)
  (cond ((or (and (eqv? a 0) (or (eq? b '+inf) (eq? b '-inf)))
             (and (eqv? b 0) (or (eq? a '+inf) (eq? a '-inf))))
         0)
        ((and (eq? a '+inf) (eq? b '+inf)) '+inf)
        ((and (eq? a '-inf) (eq? b '-inf)) '+inf)
        ((or (and (eq? a '+inf) (eq? b '-inf))
             (and (eq? a '-inf) (eq? b '+inf)))
         '-inf)
        ((eq? a '+inf) (if (< b 0) '-inf '+inf))
        ((eq? a '-inf) (if (< b 0) '+inf '-inf))
        ((eq? b '+inf) (if (< a 0) '-inf '+inf))
        ((eq? b '-inf) (if (< a 0) '+inf '-inf))
        (else (* a b))))

(define (interval-lattice)
  (make-lattice
    ;; join: widen to encompass both
    (lambda (a b)
      (cond ((eq? a 'interval-bot) b)
            ((eq? b 'interval-bot) a)
            (else (cons (inf-min (car a) (car b))
                        (inf-max (cdr a) (cdr b))))))
    ;; meet: narrow to intersection
    (lambda (a b)
      (cond ((eq? a 'interval-bot) 'interval-bot)
            ((eq? b 'interval-bot) 'interval-bot)
            (else (let ((lo (inf-max (car a) (car b)))
                        (hi (inf-min (cdr a) (cdr b))))
                    (if (inf<= lo hi)
                        (cons lo hi)
                        'interval-bot)))))
    'interval-bot
    '(-inf . +inf)
    ;; leq: a contained in b
    (lambda (a b)
      (cond ((eq? a 'interval-bot) #t)
            ((eq? b 'interval-bot) #f)
            (else (and (inf<= (car b) (car a))
                       (inf<= (cdr a) (cdr b))))))))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDomainsIntervalLattice -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/domains.scm goast/belief_integration_test.go
git commit -m "feat(domains): interval-lattice with infinity arithmetic"
```

---

### Task 8: Interval analysis with widening

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/domains.scm`
- Modify: `goast/belief_integration_test.go`

**Step 1: Write the failing test**

Add to `goast/belief_integration_test.go`:

```go
func TestDomainsIntervalAnalysis(t *testing.T) {
	engine := newBeliefEngine(t)

	// Note: "eval" here is the existing Go test helper that runs Scheme
	// expressions via the Wile engine -- NOT JavaScript/Python eval().
	eval(t, engine, `
		(import (wile algebra))
		(import (wile goast domains))
		(import (wile goast dataflow))
		(import (wile goast ssa))
		(import (wile goast utils))

		(define ssa (go-ssa-build "./examples/goast-query/testdata/arithmetic"))
		(define fn-add (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "Add") (car fs))
		        (else (loop (cdr fs))))))
		(define fn-loop (let loop ((fs ssa))
		  (cond ((null? fs) #f)
		        ((equal? (nf (car fs) 'name) "LoopSum") (car fs))
		        (else (loop (cdr fs))))))
	`)

	t.Run("analysis completes on Add", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((r (make-interval-analysis fn-add)))
			  (> (length r) 0))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("analysis terminates on LoopSum with widening", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((r (make-interval-analysis fn-loop)))
			  (> (length r) 0))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("custom widening threshold", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((r (make-interval-analysis fn-loop 2)))
			  (> (length r) 0))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDomainsIntervalAnalysis -v`
Expected: FAIL -- placeholder returns `'()`

**Step 3: Implement interval analysis with widening**

In `cmd/wile-goast/lib/wile/goast/domains.scm`, replace the `make-interval-analysis` placeholder with:

```scheme
;; Interval arithmetic on (lo . hi) pairs.
(define (interval-add a b)
  (cons (inf+ (car a) (car b)) (inf+ (cdr a) (cdr b))))

(define (interval-sub a b)
  (cons (inf- (car a) (cdr b)) (inf- (cdr a) (car b))))

(define (interval-mul a b)
  (let* ((corners (list (inf* (car a) (car b))
                        (inf* (car a) (cdr b))
                        (inf* (cdr a) (car b))
                        (inf* (cdr a) (cdr b))))
         (lo (let loop ((cs (cdr corners)) (m (car corners)))
               (if (null? cs) m (loop (cdr cs) (inf-min m (car cs))))))
         (hi (let loop ((cs (cdr corners)) (m (car corners)))
               (if (null? cs) m (loop (cdr cs) (inf-max m (car cs)))))))
    (cons lo hi)))

;; Abstract a concrete integer to a point interval.
(define (abstract-interval n) (cons n n))

;; Resolve an operand to an interval value.
(define (resolve-interval state name)
  (let ((in-state (state-lookup state name)))
    (cond (in-state in-state)
          ((parse-ssa-const name) => abstract-interval)
          (else '(-inf . +inf)))))

(define (make-interval-analysis ssa-fn . args)
  (let* ((threshold (if (pair? args) (car args) 3))
         (names (ssa-instruction-names ssa-fn))
         (i-lat (interval-lattice))
         (lat (map-lattice names i-lat))
         ;; Per-block visit counter for widening.
         (visit-counts '())
         (get-visits (lambda (idx)
                       (let ((e (assv idx visit-counts)))
                         (if e (cdr e) 0))))
         (inc-visits! (lambda (idx)
                        (let ((e (assv idx visit-counts)))
                          (if e
                              (set-cdr! e (+ (cdr e) 1))
                              (set! visit-counts
                                (cons (cons idx 1) visit-counts))))))
         ;; Widen a single interval: push growing bounds to infinity.
         (widen-interval
           (lambda (old new)
             (cond ((eq? old 'interval-bot) new)
                   ((eq? new 'interval-bot) old)
                   (else
                     (cons (if (inf<= (car new) (car old)) (car new) '-inf)
                           (if (inf<= (cdr old) (cdr new)) (cdr new) '+inf))))))
         ;; Widen an entire map-lattice state: pointwise on each key.
         (widen-state
           (lambda (old-st new-st)
             (map (lambda (old-entry new-entry)
                    (cons (car new-entry)
                          (widen-interval (cdr old-entry) (cdr new-entry))))
                  old-st new-st)))
         (transfer
           (lambda (block state)
             (let* ((idx (nf block 'index))
                    (visits (get-visits idx))
                    (raw-result
                      (let loop ((instrs (block-instrs block)) (st state))
                        (if (null? instrs) st
                            (let ((instr (car instrs)))
                              (loop (cdr instrs)
                                (cond
                                  ((tag? instr 'ssa-binop)
                                   (let* ((nm (nf instr 'name))
                                          (opcode (go-token->opcode
                                                    (nf instr 'op)))
                                          (v1 (resolve-interval
                                                st (nf instr 'x)))
                                          (v2 (resolve-interval
                                                st (nf instr 'y)))
                                          (result
                                            (cond
                                              ((or (eq? v1 'interval-bot)
                                                   (eq? v2 'interval-bot))
                                               'interval-bot)
                                              (else
                                                (case opcode
                                                  ((add) (interval-add v1 v2))
                                                  ((sub) (interval-sub v1 v2))
                                                  ((mul) (interval-mul v1 v2))
                                                  (else '(-inf . +inf)))))))
                                     (if nm
                                         (state-update st nm result)
                                         st)))

                                  ((tag? instr 'ssa-phi)
                                   (let* ((nm (nf instr 'name))
                                          (ops (or (nf instr 'operands) '()))
                                          (result
                                            (let join-ops
                                              ((os ops)
                                               (acc 'interval-bot))
                                              (if (null? os) acc
                                                  (join-ops (cdr os)
                                                    (lattice-join i-lat acc
                                                      (resolve-interval
                                                        st (car os))))))))
                                     (if nm
                                         (state-update st nm result)
                                         st)))

                                  ((nf instr 'name)
                                   (state-update st (nf instr 'name)
                                     '(-inf . +inf)))

                                  (else st))))))))
               (inc-visits! idx)
               (if (> visits threshold)
                   (widen-state state raw-result)
                   raw-result)))))
    (run-analysis 'forward lat transfer ssa-fn)))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestDomainsIntervalAnalysis -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/wile-goast/lib/wile/goast/domains.scm goast/belief_integration_test.go
git commit -m "feat(domains): make-interval-analysis with per-block widening"
```

---

### Task 9: Full test suite and CI

**Files:** none modified

**Step 1: Run all tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./... -count=1`
Expected: ALL PASS -- existing tests unchanged, new domain tests pass

**Step 2: Run CI check**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: PASS (lint, build, test, 80% coverage threshold)

**Step 3: Fix any issues, commit if needed**

---

### Task 10: Update TODO.md and CLAUDE.md

**Files:**
- Modify: `TODO.md`
- Modify: `CLAUDE.md`

**Step 1: Mark C3 items as done in TODO.md**

Update the C3 section:

```markdown
### C3. Pre-built abstract domains -- DONE

Completed 2026-03-26. Five domains in `(wile goast domains)` library:
reaching definitions, liveness, constant propagation, sign, interval.
Shared `go-concrete-eval` for integer opcodes. Interval analysis uses
per-block widening in transfer closure.
See `plans/2026-03-26-c3-domains-design.md`.

- [x] Powerset lattice -- liveness, reaching definitions
- [x] Flat lattice (bottom < concrete values < top) -- constant propagation
- [x] Sign lattice ({bottom, neg, zero, pos, top})
- [x] Interval lattice -- range analysis with widening
```

Add to the "Other" section:

```markdown
- [ ] Extend `go-concrete-eval` to cover full SSA opcode set (shifts, comparisons, negation, bitwise)
  - Blocked on Wile bitwise support (no `bitwise-and`/`bitwise-ior`/`bitwise-xor`)
  - Needed for C5 auto-lifting exhaustive verification
```

**Step 2: Add domains library to CLAUDE.md key files and library tables**

Add `(wile goast domains)` to the Scheme libraries table and key files section.

**Step 3: Commit**

```bash
git add TODO.md CLAUDE.md
git commit -m "docs: mark C3 pre-built abstract domains complete"
```
