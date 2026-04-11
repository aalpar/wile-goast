# Function Boundary Recommendations — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `(wile goast fca-recommend)` — a library that analyzes FCA concept lattices to produce ranked split/merge/extract recommendations for function boundaries, filtered by SSA data flow analysis.

**Architecture:** Scheme library importing `(wile goast fca)` for lattice operations and `(wile goast dataflow)` for SSA cross-flow filtering. One small Go change adds `struct` metadata to SSA field-addr mapper nodes. Pareto dominance ranking with separate frontiers per recommendation type.

**Tech Stack:** Scheme (R7RS), existing `(wile goast fca)`, `(wile goast dataflow)`, `(wile goast utils)`. Go change to `goastssa/mapper.go`.

**Design doc:** `plans/2026-04-10-function-boundary-recommendations-design.md`

---

### Task 1: Testdata — funcboundary package

Create a synthetic Go package with clear split, merge, and extract patterns.

**Files:**
- Create: `examples/goast-query/testdata/funcboundary/funcboundary.go`

**Step 1: Create the testdata package**

`examples/goast-query/testdata/funcboundary/funcboundary.go`:
```go
package funcboundary

type Config struct {
	Timeout    int
	MaxRetries int
}

type Metrics struct {
	RequestCount int
	ErrorCount   int
}

type Session struct {
	Token  string
	Expiry int
}

type Auth struct {
	User  string
	Level int
}

type Response struct {
	Body   string
	Status int
}

// ── Split candidate: two independent state clusters, no cross-flow ──

// ProcessRequest writes Config and Metrics independently.
// No data flows from Config fields to Metrics fields.
func ProcessRequest(c *Config, m *Metrics, timeout int, count int) {
	c.Timeout = timeout
	c.MaxRetries = 3
	m.RequestCount = count
	m.ErrorCount = 0
}

// ── Split candidate in lattice, filtered by cross-flow ──

// ProcessAndRecord writes Config then uses Config values to compute Metrics.
// Data flows from Config fields to Metrics store — intentional coordination.
func ProcessAndRecord(c *Config, m *Metrics) {
	c.Timeout = 30
	c.MaxRetries = 3
	m.RequestCount = c.Timeout + c.MaxRetries
	m.ErrorCount = 0
}

// ── Merge candidates: overlapping session writes ──

// ResetSession clears session fields.
func ResetSession(s *Session) {
	s.Token = ""
	s.Expiry = 0
}

// ExpireSession also writes session fields.
func ExpireSession(s *Session) {
	s.Token = ""
	s.Expiry = -1
}

// ── Extract candidates: shared sub-operation (read-write mode) ──

// ValidateSession reads Session fields only (the sub-operation).
func ValidateSession(s *Session) bool {
	return s.Token != "" && s.Expiry > 0
}

// HandleAuth reads Session, writes Auth.
func HandleAuth(s *Session, a *Auth) {
	a.User = s.Token
	a.Level = s.Expiry
}

// HandleResponse reads Session, writes Response.
func HandleResponse(s *Session, r *Response) {
	r.Body = s.Token
	r.Status = s.Expiry
}

// ── Single-cluster controls (no recommendations expected) ──

func ConfigOnly(c *Config) {
	c.Timeout = 30
}

func MetricsOnly(m *Metrics) {
	m.RequestCount = 0
}
```

**Step 2: Verify it compiles**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build ./examples/goast-query/testdata/funcboundary/`
Expected: Success (no output)

**Step 3: Commit**

```
testdata: add funcboundary package for function boundary recommendations
```

---

### Task 2: Add `struct` field to SSA mapper

Add struct type name to `ssa-field-addr` and `ssa-field` nodes so the
Scheme-side cross-flow filter can classify field accesses by struct.

**Files:**
- Modify: `goastssa/mapper.go:281-305`

**Step 1: Write the failing test**

Add to `goastssa/mapper_test.go`:
```go
func TestMapFieldAddr_HasStructField(t *testing.T) {
	c := qt.New(t)

	fn := compileToSSA(t, `
package main

type Point struct{ X, Y int }

func SetX(p *Point, v int) { p.X = v }
`, "SetX")

	mapper := &ssaMapper{fset: token.NewFileSet()}
	result := mapper.mapFunction(fn)

	found := findNodeByTag(result, "ssa-field-addr")
	c.Assert(found, qt.IsNotNil, qt.Commentf("expected ssa-field-addr in SSA of SetX"))

	structField, ok := goast.GetField(found.(*values.Pair).Cdr(), "struct")
	c.Assert(ok, qt.IsTrue, qt.Commentf("ssa-field-addr should have struct field"))
	c.Assert(structField.(*values.String).Value, qt.Equals, "Point")
}

func TestMapField_HasStructField(t *testing.T) {
	c := qt.New(t)

	fn := compileToSSA(t, `
package main

type Point struct{ X, Y int }

func GetX(p Point) int { return p.X }
`, "GetX")

	mapper := &ssaMapper{fset: token.NewFileSet()}
	result := mapper.mapFunction(fn)

	found := findNodeByTag(result, "ssa-field")
	c.Assert(found, qt.IsNotNil, qt.Commentf("expected ssa-field in SSA of GetX"))

	structField, ok := goast.GetField(found.(*values.Pair).Cdr(), "struct")
	c.Assert(ok, qt.IsTrue, qt.Commentf("ssa-field should have struct field"))
	c.Assert(structField.(*values.String).Value, qt.Equals, "Point")
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goastssa/ -run "TestMapFieldAddr_HasStructField|TestMapField_HasStructField" -v -count=1`
Expected: FAIL — `struct` field not found.

**Step 3: Implement mapper enhancement**

In `goastssa/mapper.go`, modify `mapFieldAddr`:
```go
func (p *ssaMapper) mapFieldAddr(v *ssa.FieldAddr) values.Value {
	structType := typesDeref(v.X.Type())
	fieldName := fieldNameAt(structType, v.Field)
	structName, _ := structTypeName(structType)
	return goast.Node("ssa-field-addr",
		goast.Field("name", goast.Str(v.Name())),
		goast.Field("x", valName(v.X)),
		goast.Field("struct", goast.Str(structName)),
		goast.Field("field", goast.Str(fieldName)),
		goast.Field("field-index", values.NewInteger(int64(v.Field))),
		goast.Field("type", goast.Str(types.TypeString(v.Type(), nil))),
		goast.Field("operands", goast.ValueList([]values.Value{valName(v.X)})),
	)
}
```

Modify `mapField`:
```go
func (p *ssaMapper) mapField(v *ssa.Field) values.Value {
	structType := v.X.Type()
	fieldName := fieldNameAt(structType, v.Field)
	structName, _ := structTypeName(structType)
	return goast.Node("ssa-field",
		goast.Field("name", goast.Str(v.Name())),
		goast.Field("x", valName(v.X)),
		goast.Field("struct", goast.Str(structName)),
		goast.Field("field", goast.Str(fieldName)),
		goast.Field("field-index", values.NewInteger(int64(v.Field))),
		goast.Field("type", goast.Str(types.TypeString(v.Type(), nil))),
		goast.Field("operands", goast.ValueList([]values.Value{valName(v.X)})),
	)
}
```

Note: `structTypeName` already exists in `prim_ssa.go`. It returns `(name, pkg)`.
For `mapFieldAddr`, `typesDeref` is needed (already used on line 282); for
`mapField`, the type is already a struct type (not pointer).

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goastssa/ -run "TestMapFieldAddr_HasStructField|TestMapField_HasStructField" -v -count=1`
Expected: PASS

**Step 5: Run full test suite**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./... -count=1`
Expected: All PASS — existing tests don't assert on exact node structure.

**Step 6: Commit**

```
feat(ssa): add struct field to ssa-field-addr and ssa-field mapper nodes
```

---

### Task 3: Library scaffold

**Files:**
- Create: `cmd/wile-goast/lib/wile/goast/fca-recommend.sld`
- Create: `cmd/wile-goast/lib/wile/goast/fca-recommend.scm`

**Step 1: Create the library definition**

`cmd/wile-goast/lib/wile/goast/fca-recommend.sld`:
```scheme
(define-library (wile goast fca-recommend)
  (export
    ;; Pareto dominance
    dominates?
    pareto-frontier

    ;; Lattice analysis
    concept-signature
    incomparable-pairs

    ;; Candidate detection
    split-candidates
    merge-candidates
    extract-candidates

    ;; Top-level
    boundary-recommendations)
  (import (wile goast utils)
          (wile goast fca)
          (wile goast dataflow))
  (include "fca-recommend.scm"))
```

**Step 2: Create empty implementation**

`cmd/wile-goast/lib/wile/goast/fca-recommend.scm`:
```scheme
;;; fca-recommend.scm — Function boundary recommendations via FCA + SSA
;;;
;;; Analyzes concept lattice structure to produce ranked split/merge/extract
;;; recommendations for function boundaries. SSA data flow filtering
;;; distinguishes intentional coordination from accidental aggregation.
;;; Pareto dominance ranking with separate frontiers per type.
```

**Step 3: Commit**

```
scaffold: add (wile goast fca-recommend) library definition
```

---

### Task 4: Pareto dominance machinery

Generic Pareto dominance over factor vectors. No FCA dependency.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca-recommend.scm`
- Create: `goast/fca_recommend_test.go`

**Step 1: Write the failing test**

`goast/fca_recommend_test.go` — the test helper is named `eval` (see `prim_goast_test.go`).
The engine constructor is `newBeliefEngine(t)` (see `belief_integration_test.go`).

```go
package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestPareto_Dominates(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast fca-recommend))`)

	t.Run("strictly better on all factors", func(t *testing.T) {
		result := eval(t, engine, `
			(dominates?
			  '((a . 3) (b . 2) (c . #t))
			  '((a . 1) (b . 1) (c . #f)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("equal on all factors is not domination", func(t *testing.T) {
		result := eval(t, engine, `
			(dominates?
			  '((a . 3) (b . 2))
			  '((a . 3) (b . 2)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#f")
	})

	t.Run("better on one worse on another is not domination", func(t *testing.T) {
		result := eval(t, engine, `
			(dominates?
			  '((a . 3) (b . 1))
			  '((a . 1) (b . 3)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#f")
	})

	t.Run("boolean factor ordering", func(t *testing.T) {
		result := eval(t, engine, `
			(dominates?
			  '((a . 3) (b . #t))
			  '((a . 3) (b . #f)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestPareto_Frontier(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast fca-recommend))`)

	t.Run("single item frontier", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((result (pareto-frontier
			                (list (list 'r1 '((a . 3) (b . 2))))
			                '(a b))))
			  (length (cdr (assoc 'frontier result))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("incomparable items both on frontier", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((result (pareto-frontier
			                (list (list 'r1 '((a . 3) (b . 1)))
			                      (list 'r2 '((a . 1) (b . 3))))
			                '(a b))))
			  (length (cdr (assoc 'frontier result))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "2")
	})

	t.Run("dominated item not on frontier", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((result (pareto-frontier
			                (list (list 'r1 '((a . 3) (b . 3)))
			                      (list 'r2 '((a . 1) (b . 1))))
			                '(a b))))
			  (list (length (cdr (assoc 'frontier result)))
			        (length (cdr (assoc 'dominated result)))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "(1 1)")
	})

	t.Run("dominated grouped under dominator", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((result (pareto-frontier
			                (list (list 'r1 '((a . 3) (b . 3)))
			                      (list 'r2 '((a . 1) (b . 1))))
			                '(a b))))
			  (let ((dom-groups (cdr (assoc 'dominated result))))
			    (car (car dom-groups))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "r1")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestPareto" -v -count=1`
Expected: FAIL — procedures not defined.

**Step 3: Implement Pareto dominance**

Add to `fca-recommend.scm`:
```scheme
;;; ── Factor comparison ───────────────────────────────────

;; Compare two factor values. Booleans: #f < #t. Numbers: standard <=.
(define (factor-leq? a b)
  (cond ((boolean? a) (or (not a) b))
        ((boolean? b) (and a b))
        (else (<= a b))))

(define (factor-less? a b)
  (and (factor-leq? a b) (not (equal? a b))))

;;; ── Pareto dominance ────────────────────────────────────

;; X dominates Y iff X >= Y on every factor and X > Y on at least one.
;; factors-x, factors-y: alists ((name . value) ...)
(define (dominates? factors-x factors-y)
  (let loop ((fx factors-x) (any-strict #f))
    (if (null? fx)
      any-strict
      (let* ((key (car (car fx)))
             (vx (cdr (car fx)))
             (vy (cdr (assoc key factors-y))))
        (if (factor-leq? vy vx)
          (loop (cdr fx) (or any-strict (factor-less? vy vx)))
          #f)))))

;; Compute Pareto frontier and dominated groups.
;; candidates: list of (id factors-alist) pairs.
;; factor-names: list of factor name symbols (documentation only).
;; Returns: ((frontier id ...) (dominated (dominator-id dominated-id ...) ...))
(define (pareto-frontier candidates factor-names)
  (let* ((ids (map car candidates))
         (factors-of (lambda (id)
                       (cadr (let loop ((cs candidates))
                               (cond ((null? cs) #f)
                                     ((equal? (car (car cs)) id) (car cs))
                                     (else (loop (cdr cs))))))))
         (frontier-ids
           (filter-map
             (lambda (c)
               (let ((c-id (car c))
                     (c-factors (cadr c)))
                 (let dominated? ((rest candidates))
                   (cond ((null? rest) c-id)
                         ((equal? (car (car rest)) c-id) (dominated? (cdr rest)))
                         ((dominates? (cadr (car rest)) c-factors) #f)
                         (else (dominated? (cdr rest)))))))
             candidates))
         (dominated-ids (filter (lambda (id) (not (member? id frontier-ids))) ids))
         (dom-groups
           (filter-map
             (lambda (fid)
               (let ((f-factors (factors-of fid))
                     (doms (filter
                             (lambda (did)
                               (dominates? f-factors (factors-of did)))
                             dominated-ids)))
                 (if (null? doms) #f
                   (cons fid doms))))
             frontier-ids)))
    (list (cons 'frontier frontier-ids)
          (cons 'dominated dom-groups))))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestPareto" -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(fca-recommend): Pareto dominance machinery
```

---

### Task 5: Concept signature and incomparable pairs

Map each function to its concept set in the lattice, then find incomparable pairs.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca-recommend.scm`
- Modify: `goast/fca_recommend_test.go`

**Step 1: Write the failing test**

Append to `goast/fca_recommend_test.go`:
```go
func TestConceptSignature(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("F1" "A.x" "B.y")
		    ("F2" "A.x")
		    ("F3" "B.y"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("F1 in multiple concepts", func(t *testing.T) {
		result := eval(t, engine, `(length (concept-signature lat "F1"))`)
		qt.New(t).Assert(result.SchemeString(), qt.Not(qt.Equals), "0")
	})

	t.Run("F2 in at least one concept", func(t *testing.T) {
		result := eval(t, engine, `(>= (length (concept-signature lat "F2")) 1)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestIncomparablePairs(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("F1" "A.x" "B.y")
		    ("F2" "A.x")
		    ("F3" "B.y"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("F1 has incomparable pairs", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sig (concept-signature lat "F1")))
			  (length (incomparable-pairs sig)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("F2 has no incomparable pairs", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sig (concept-signature lat "F2")))
			  (length (incomparable-pairs sig)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "0")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestConceptSignature|TestIncomparablePairs" -v -count=1`
Expected: FAIL

**Step 3: Implement concept signature and incomparable pairs**

Add to `fca-recommend.scm`:
```scheme
;;; ── Lattice analysis utilities ──────────────────────────

;; The concept signature of a function: all concepts whose extent contains it.
(define (concept-signature lattice func-name)
  (filter
    (lambda (concept)
      (member? func-name (concept-extent concept)))
    lattice))

;; #t if every element of sorted list i1 is in sorted list i2.
(define (intent-subset? i1 i2)
  (cond ((null? i1) #t)
        ((null? i2) #f)
        ((string<? (car i1) (car i2)) #f)
        ((string=? (car i1) (car i2)) (intent-subset? (cdr i1) (cdr i2)))
        (else (intent-subset? i1 (cdr i2)))))

;; Two concepts are incomparable when neither intent is a subset of the other.
(define (concepts-incomparable? c1 c2)
  (let ((i1 (concept-intent c1))
        (i2 (concept-intent c2)))
    (and (not (intent-subset? i1 i2))
         (not (intent-subset? i2 i1)))))

;; All incomparable pairs in a concept set.
;; Returns list of (c1 . c2) pairs.
(define (incomparable-pairs concepts)
  (let loop ((cs concepts) (acc '()))
    (if (null? cs) acc
      (let inner ((rest (cdr cs)) (acc acc))
        (if (null? rest)
          (loop (cdr cs) acc)
          (inner (cdr rest)
                 (if (concepts-incomparable? (car cs) (car rest))
                   (cons (cons (car cs) (car rest)) acc)
                   acc)))))))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestConceptSignature|TestIncomparablePairs" -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(fca-recommend): concept signature and incomparable pair detection
```

---

### Task 6: Split candidate detection (lattice factors)

Detect split candidates from incomparable concept pairs. Compute lattice-only
factors. SSA cross-flow filter added in Task 7.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca-recommend.scm`
- Modify: `goast/fca_recommend_test.go`

**Step 1: Write the failing test**

Append to `goast/fca_recommend_test.go`:
```go
func TestSplitCandidates_Lattice(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("ProcessRequest" "A.x" "A.y" "B.z" "B.w")
		    ("ConfigOnly" "A.x")
		    ("MetricsOnly" "B.z"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("ProcessRequest is a split candidate", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((splits (split-candidates lat #f)))
			  (and (pair? splits)
			       (let ((first (car splits)))
			         (string=? (cdr (assoc 'function first))
			                   "ProcessRequest"))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("ConfigOnly is not a split candidate", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((splits (split-candidates lat #f)))
			  (let loop ((ss splits))
			    (cond ((null? ss) #t)
			          ((string=? (cdr (assoc 'function (car ss)))
			                     "ConfigOnly") #f)
			          (else (loop (cdr ss))))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("split has intent-disjointness factor", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((splits (split-candidates lat #f))
			       (first (car splits))
			       (factors (cdr (assoc 'factors first))))
			  (assoc 'intent-disjointness factors))`)
		qt.New(t).Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestSplitCandidates_Lattice" -v -count=1`
Expected: FAIL

**Step 3: Implement split candidate detection**

Add to `fca-recommend.scm`:
```scheme
;;; ── Set operations for intents ──────────────────────────

(define (intent-intersection i1 i2)
  (cond ((null? i1) '())
        ((null? i2) '())
        ((string<? (car i1) (car i2)) (intent-intersection (cdr i1) i2))
        ((string<? (car i2) (car i1)) (intent-intersection i1 (cdr i2)))
        (else (cons (car i1) (intent-intersection (cdr i1) (cdr i2))))))

(define (intent-union i1 i2)
  (cond ((null? i1) i2)
        ((null? i2) i1)
        ((string<? (car i1) (car i2))
         (cons (car i1) (intent-union (cdr i1) i2)))
        ((string<? (car i2) (car i1))
         (cons (car i2) (intent-union i1 (cdr i2))))
        (else (cons (car i1) (intent-union (cdr i1) (cdr i2))))))

;;; ── Helper: string suffix check ─────────────────────────

(define (string-suffix? suffix s)
  (let ((slen (string-length s))
        (sufflen (string-length suffix)))
    (and (>= slen sufflen)
         (string=? (substring s (- slen sufflen) slen) suffix))))

;;; ── SSA function lookup ─────────────────────────────────

(define (find-ssa-func ssa-funcs func-name)
  (let loop ((fs ssa-funcs))
    (cond ((null? fs) #f)
          ((and (pair? (car fs))
                (string=? (or (nf (car fs) 'name) "") func-name))
           (car fs))
          (else (loop (cdr fs))))))

(define (ssa-stmt-count ssa-funcs func-name)
  (let ((fn (find-ssa-func ssa-funcs func-name)))
    (if fn (length (ssa-all-instrs fn)) 0)))

;;; ── Cross-flow placeholder (Task 7) ────────────────────

(define (cross-flow-between? ssa-funcs func-name cluster1-fields cluster2-fields)
  #f)

;;; ── Split candidate detection ───────────────────────────

;; Compute split candidates from the lattice.
;; ssa-funcs: list of SSA function nodes from go-ssa-build, or #f to skip SSA.
;; Returns list of recommendation alists.
(define (split-candidates lattice ssa-funcs)
  (let ((all-funcs (unique (flat-map concept-extent lattice))))
    (filter-map
      (lambda (func-name)
        (let* ((sig (concept-signature lattice func-name))
               (sig-nontop (filter (lambda (c) (pair? (concept-intent c))) sig))
               (pairs (incomparable-pairs sig-nontop)))
          (if (null? pairs) #f
            (let* ((pair1 (car pairs))
                   (c1 (car pair1))
                   (c2 (cdr pair1))
                   (i1 (concept-intent c1))
                   (i2 (concept-intent c2))
                   (e1 (concept-extent c1))
                   (e2 (concept-extent c2))
                   (isect (intent-intersection i1 i2))
                   (iunion (intent-union i1 i2))
                   (disjointness
                     (if (null? iunion) 0
                       (exact->inexact
                         (- 1 (/ (length isect) (length iunion))))))
                   (balance
                     (let ((min-e (min (length e1) (length e2)))
                           (max-e (max (length e1) (length e2))))
                       (if (= max-e 0) 0
                         (exact->inexact (/ min-e max-e)))))
                   (no-cross-flow
                     (if ssa-funcs
                       (not (cross-flow-between? ssa-funcs func-name i1 i2))
                       #t))
                   (stmt-ct (if ssa-funcs
                              (ssa-stmt-count ssa-funcs func-name) 0))
                   (factors
                     (list (cons 'incomparable-count (length pairs))
                           (cons 'intent-disjointness disjointness)
                           (cons 'no-cross-flow no-cross-flow)
                           (cons 'pattern-balance balance)
                           (cons 'stmt-count stmt-ct))))
              (list (cons 'type 'split)
                    (cons 'function func-name)
                    (cons 'factors factors)
                    (cons 'clusters
                      (list (list (cons 'intent i1) (cons 'extent e1))
                            (list (cons 'intent i2) (cons 'extent e2)))))))))
      all-funcs)))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestSplitCandidates_Lattice" -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(fca-recommend): split candidate detection with lattice factors
```

---

### Task 7: Cross-flow filter (SSA integration)

Implement `cross-flow-between?` using `defuse-reachable?` on SSA instructions.
Replace the placeholder from Task 6.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca-recommend.scm`
- Modify: `goast/fca_recommend_test.go`

**Step 1: Write the failing test**

Append to `goast/fca_recommend_test.go`:
```go
func TestCrossFlowFilter(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))
		(import (wile goast dataflow))

		(define s (go-load
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/funcboundary"))
		(define ssa-funcs (go-ssa-build s))
		(define idx (go-ssa-field-index s))
		(define ctx (field-index->context idx 'write-only))
		(define lat (concept-lattice ctx))
		(define splits (split-candidates lat ssa-funcs))
	`)

	t.Run("ProcessRequest has no cross-flow", func(t *testing.T) {
		result := eval(t, engine, `
			(let loop ((ss splits))
			  (cond ((null? ss) 'not-found)
			        ((string-suffix? ".ProcessRequest"
			           (cdr (assoc 'function (car ss))))
			         (cdr (assoc 'no-cross-flow
			                (cdr (assoc 'factors (car ss))))))
			        (else (loop (cdr ss)))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("ProcessAndRecord has cross-flow", func(t *testing.T) {
		result := eval(t, engine, `
			(let loop ((ss splits))
			  (cond ((null? ss) 'not-found)
			        ((string-suffix? ".ProcessAndRecord"
			           (cdr (assoc 'function (car ss))))
			         (cdr (assoc 'no-cross-flow
			                (cdr (assoc 'factors (car ss))))))
			        (else (loop (cdr ss)))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#f")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestCrossFlowFilter" -v -count=1`
Expected: FAIL — cross-flow always returns #f (placeholder)

**Step 3: Implement cross-flow detection**

Replace the `cross-flow-between?` placeholder in `fca-recommend.scm`:
```scheme
;;; ── SSA cross-flow detection ────────────────────────────

;; Extract struct.field key from an ssa-field-addr instruction.
;; Requires the struct field added in Task 2.
(define (field-addr-key instr)
  (let ((struct-name (nf instr 'struct))
        (field-name (nf instr 'field)))
    (and struct-name field-name
         (string-append struct-name "." field-name))))

;; Classify field-addr instructions by cluster membership.
;; Returns register names for field-addrs whose struct.field is in intent-fields.
(define (cluster-field-addrs instrs intent-fields)
  (filter-map
    (lambda (instr)
      (and (tag? instr 'ssa-field-addr)
           (let ((key (field-addr-key instr)))
             (and key (member? key intent-fields)
                  (nf instr 'name)))))
    instrs))

;; Check cross-cluster data flow within a function.
;; Returns #t if any value from cluster1 fields reaches a store
;; targeting cluster2 fields via def-use chains.
(define (cross-flow-between? ssa-funcs func-name cluster1-fields cluster2-fields)
  (let ((fn (find-ssa-func ssa-funcs func-name)))
    (if (not fn) #f
      (let* ((instrs (ssa-all-instrs fn))
             (c1-names (cluster-field-addrs instrs cluster1-fields))
             (c2-names (cluster-field-addrs instrs cluster2-fields)))
        (if (or (null? c1-names) (null? c2-names)) #f
          (defuse-reachable? fn c1-names
            (lambda (instr)
              (and (tag? instr 'ssa-store)
                   (let ((addr (nf instr 'addr)))
                     (and addr (member? addr c2-names)))))
            10))))))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestCrossFlowFilter" -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(fca-recommend): SSA cross-flow filter for split candidates
```

---

### Task 8: Merge candidate detection

Find concepts where multiple functions share the same state access pattern.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca-recommend.scm`
- Modify: `goast/fca_recommend_test.go`

**Step 1: Write the failing test**

Append to `goast/fca_recommend_test.go`:
```go
func TestMergeCandidates(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("ResetSession" "Session.Token" "Session.Expiry")
		    ("ExpireSession" "Session.Token" "Session.Expiry")
		    ("Other" "X.a"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("merge candidate found", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((merges (merge-candidates lat)))
			  (pair? merges))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("merge has intent-overlap factor", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((merges (merge-candidates lat))
			       (first (car merges))
			       (factors (cdr (assoc 'factors first))))
			  (cdr (assoc 'intent-overlap factors)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("merge functions are ResetSession and ExpireSession", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((merges (merge-candidates lat))
			       (first (car merges))
			       (fns (cdr (assoc 'functions first))))
			  (and (member? "ResetSession" fns)
			       (member? "ExpireSession" fns)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestMergeCandidates" -v -count=1`
Expected: FAIL

**Step 3: Implement merge candidate detection**

Add to `fca-recommend.scm`:
```scheme
;;; ── Merge candidate detection ───────────────────────────

;; Merge candidates: concepts with |E| >= 2 and non-empty intent.
;; Multiple functions sharing the same field access pattern indicates
;; separate maintenance of shared state — a coordination risk.
(define (merge-candidates lattice)
  (filter-map
    (lambda (concept)
      (let ((ext (concept-extent concept))
            (int (concept-intent concept)))
        (if (or (< (length ext) 2) (null? int)) #f
          (let* (;; Compute overlap: concept intent vs union of per-function intents
                 ;; For functions with identical access patterns, overlap = 1.0
                 (func-intents
                   (map (lambda (f)
                          ;; Find the most specific concept for this function
                          (let loop ((cs lattice) (best int))
                            (cond ((null? cs) best)
                                  ((and (member? f (concept-extent (car cs)))
                                        (intent-subset? best (concept-intent (car cs))))
                                   (loop (cdr cs) (concept-intent (car cs))))
                                  (else (loop (cdr cs) best)))))
                        ext))
                 (all-union (let loop ((fis func-intents) (acc '()))
                              (if (null? fis) acc
                                (loop (cdr fis) (intent-union acc (car fis))))))
                 (overlap (if (null? all-union) 0
                            (exact->inexact (/ (length int) (length all-union)))))
                 ;; Write overlap: fields without ":r" suffix
                 (write-fields (filter
                                 (lambda (a) (not (string-suffix? ":r" a))) int))
                 (all-write (filter
                              (lambda (a) (not (string-suffix? ":r" a))) all-union))
                 (write-ovl (if (null? all-write) 0
                              (exact->inexact (/ (length write-fields)
                                                 (length all-write)))))
                 (factors
                   (list (cons 'intent-overlap overlap)
                         (cons 'write-overlap write-ovl)
                         (cons 'extent-count (length ext)))))
            (list (cons 'type 'merge)
                  (cons 'functions ext)
                  (cons 'factors factors)
                  (cons 'shared-intent int))))))
    lattice))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestMergeCandidates" -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(fca-recommend): merge candidate detection
```

---

### Task 9: Extract candidate detection

Find concept pairs where a sub-operation serves more callers than the full operation.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca-recommend.scm`
- Modify: `goast/fca_recommend_test.go`

**Step 1: Write the failing test**

Append to `goast/fca_recommend_test.go`:
```go
func TestExtractCandidates(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("Validate" "S.Token:r" "S.Expiry:r")
		    ("HandleAuth" "S.Token:r" "S.Expiry:r" "Auth.User:w" "Auth.Level:w")
		    ("HandleResp" "S.Token:r" "S.Expiry:r" "Resp.Body:w" "Resp.Status:w"))))
		(define lat (concept-lattice ctx))
	`)

	t.Run("extract candidate found", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((extracts (extract-candidates lat)))
			  (pair? extracts))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("extent-ratio > 1", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((extracts (extract-candidates lat))
			       (first (car extracts))
			       (factors (cdr (assoc 'factors first))))
			  (> (cdr (assoc 'extent-ratio factors)) 1))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("sub-operation includes session reads", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((extracts (extract-candidates lat))
			       (first (car extracts))
			       (sub-op (cdr (assoc 'sub-operation first))))
			  (and (member? "S.Token:r" sub-op)
			       (member? "S.Expiry:r" sub-op)))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestExtractCandidates" -v -count=1`
Expected: FAIL

**Step 3: Implement extract candidate detection**

Add to `fca-recommend.scm`:
```scheme
;;; ── Extract candidate detection ─────────────────────────

;; Compute lattice depth of a concept (distance from top).
(define (concept-depth lattice concept)
  (let ((target-int (concept-intent concept)))
    (let loop ((cs lattice) (depth 0) (prev-size 0))
      (cond ((null? cs) depth)
            ((equal? (concept-intent (car cs)) target-int) depth)
            ((and (intent-subset? (concept-intent (car cs)) target-int)
                  (> (length (concept-intent (car cs))) prev-size))
             (loop (cdr cs) (+ depth 1) (length (concept-intent (car cs)))))
            (else (loop (cdr cs) depth prev-size))))))

;; Extract candidates: concept pairs (C_broad, C_narrow) where
;; C_broad has broader extent and smaller intent. The broad concept's
;; intent is the shared sub-operation.
(define (extract-candidates lattice)
  (let ((multi-extent
          (filter (lambda (c)
                    (and (>= (length (concept-extent c)) 2)
                         (pair? (concept-intent c))))
                  lattice)))
    (filter-map
      (lambda (c-broad)
        (let* ((e-broad (concept-extent c-broad))
               (i-broad (concept-intent c-broad))
               ;; Find narrower concepts: smaller extent, larger intent
               (narrower
                 (filter
                   (lambda (c)
                     (and (< (length (concept-extent c)) (length e-broad))
                          (intent-subset? i-broad (concept-intent c))
                          (not (equal? (concept-intent c) i-broad))))
                   lattice))
               ;; Take the narrower concept with the most functions
               (best-narrow
                 (if (null? narrower) #f
                   (let loop ((ns (cdr narrower)) (best (car narrower)))
                     (if (null? ns) best
                       (loop (cdr ns)
                             (if (> (length (concept-extent (car ns)))
                                    (length (concept-extent best)))
                               (car ns) best)))))))
          (if (not best-narrow) #f
            (let* ((e-narrow (concept-extent best-narrow))
                   (ratio (exact->inexact
                            (/ (length e-broad) (length e-narrow))))
                   (depth (concept-depth lattice c-broad))
                   (factors
                     (list (cons 'extent-ratio ratio)
                           (cons 'intent-size (length i-broad))
                           (cons 'sub-concept-depth depth))))
              (list (cons 'type 'extract)
                    (cons 'sub-operation i-broad)
                    (cons 'factors factors)
                    (cons 'broad-extent e-broad)
                    (cons 'narrow-extent e-narrow))))))
      multi-extent)))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestExtractCandidates" -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(fca-recommend): extract candidate detection
```

---

### Task 10: Top-level `boundary-recommendations`

Compose all three detectors with Pareto frontiers.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca-recommend.scm`
- Modify: `goast/fca_recommend_test.go`

**Step 1: Write the failing test**

Append to `goast/fca_recommend_test.go`:
```go
func TestBoundaryRecommendations(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))

		(define ctx (context-from-alist
		  '(("Split1" "A.x" "B.y")
		    ("A-only" "A.x")
		    ("B-only" "B.y")
		    ("Merge1" "C.a" "C.b")
		    ("Merge2" "C.a" "C.b"))))
		(define lat (concept-lattice ctx))
		(define recs (boundary-recommendations lat #f))
	`)

	t.Run("returns three frontiers", func(t *testing.T) {
		result := eval(t, engine, `
			(and (assoc 'splits recs)
			     (assoc 'merges recs)
			     (assoc 'extracts recs)
			     #t)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("split frontier non-empty", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((sf (cdr (assoc 'splits recs))))
			  (pair? (cdr (assoc 'frontier sf))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("merge frontier non-empty", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((mf (cdr (assoc 'merges recs))))
			  (pair? (cdr (assoc 'frontier mf))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestBoundaryRecommendations" -v -count=1`
Expected: FAIL

**Step 3: Implement boundary-recommendations**

Add to `fca-recommend.scm`:
```scheme
;;; ── Top-level recommendation pipeline ───────────────────

;; Produce ranked recommendations: three Pareto frontiers.
;; lattice: from (concept-lattice ctx)
;; ssa-funcs: from (go-ssa-build session), or #f to skip SSA filtering
(define (boundary-recommendations lattice ssa-funcs)
  (let* ((splits (split-candidates lattice ssa-funcs))
         (merges (merge-candidates lattice))
         (extracts (extract-candidates lattice))
         ;; Build Pareto input: (id factors) pairs
         (split-input
           (map (lambda (s)
                  (list (cdr (assoc 'function s))
                        (cdr (assoc 'factors s))))
                splits))
         (merge-input
           (map (lambda (m)
                  (list (cdr (assoc 'functions m))
                        (cdr (assoc 'factors m))))
                merges))
         (extract-input
           (map (lambda (e)
                  (list (cdr (assoc 'sub-operation e))
                        (cdr (assoc 'factors e))))
                extracts))
         (split-frontier
           (if (null? split-input)
             '((frontier) (dominated))
             (pareto-frontier split-input
               '(incomparable-count intent-disjointness no-cross-flow
                 pattern-balance stmt-count))))
         (merge-frontier
           (if (null? merge-input)
             '((frontier) (dominated))
             (pareto-frontier merge-input
               '(intent-overlap write-overlap extent-count))))
         (extract-frontier
           (if (null? extract-input)
             '((frontier) (dominated))
             (pareto-frontier extract-input
               '(extent-ratio intent-size sub-concept-depth)))))
    (list (cons 'splits split-frontier)
          (cons 'merges merge-frontier)
          (cons 'extracts extract-frontier))))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestBoundaryRecommendations" -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(fca-recommend): top-level boundary-recommendations with Pareto frontiers
```

---

### Task 11: Integration test with funcboundary testdata

Full pipeline on real Go code: load -> SSA -> field index -> lattice -> recommendations.

**Files:**
- Modify: `goast/fca_recommend_test.go`

**Step 1: Write the integration test**

Append to `goast/fca_recommend_test.go`:
```go
func TestIntegration_FuncBoundary(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast fca))
		(import (wile goast fca-recommend))
		(import (wile goast dataflow))

		(define s (go-load
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/funcboundary"))
		(define ssa-funcs (go-ssa-build s))
		(define idx (go-ssa-field-index s))

		;; Write-only mode for splits and merges
		(define ctx-w (field-index->context idx 'write-only))
		(define lat-w (concept-lattice ctx-w))

		;; Read-write mode for extracts
		(define ctx-rw (field-index->context idx 'read-write))
		(define lat-rw (concept-lattice ctx-rw))

		(define recs-w (boundary-recommendations lat-w ssa-funcs))
		(define recs-rw (boundary-recommendations lat-rw ssa-funcs))
	`)

	t.Run("ProcessRequest on split frontier with no-cross-flow", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((sf (cdr (assoc 'splits recs-w)))
			       (frontier-ids (cdr (assoc 'frontier sf))))
			  (let loop ((ids frontier-ids))
			    (cond ((null? ids) #f)
			          ((string-suffix? ".ProcessRequest" (car ids)) #t)
			          (else (loop (cdr ids))))))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("merge candidates include session writers", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((mf (cdr (assoc 'merges recs-w)))
			       (frontier-ids (cdr (assoc 'frontier mf))))
			  (pair? frontier-ids))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("extract candidates found in read-write mode", func(t *testing.T) {
		result := eval(t, engine, `
			(let* ((ef (cdr (assoc 'extracts recs-rw)))
			       (frontier-ids (cdr (assoc 'frontier ef))))
			  (pair? frontier-ids))`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run all tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestPareto|TestConcept|TestIncomparable|TestSplit|TestCrossFlow|TestMerge|TestExtract|TestBoundary|TestIntegration_FuncBound" -v -count=1`
Expected: All PASS

**Step 3: Run full test suite**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make test`
Expected: All PASS, no regressions

**Step 4: Commit**

```
test(fca-recommend): integration test on funcboundary testdata
```

---

### Task 12: CI, documentation, and cleanup

**Step 1: Run CI**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: PASS (lint + build + test + coverage)

**Step 2: Update CLAUDE.md primitives table**

Add under a new section:
```markdown
### fca-recommend — `(wile goast fca-recommend)`
| Primitive | Description |
|-----------|-------------|
| `dominates?` | Pareto dominance: X >= Y on all factors, > on at least one |
| `pareto-frontier` | Compute Pareto frontier and dominated groups |
| `concept-signature` | Map function name to its concepts in the lattice |
| `incomparable-pairs` | Find incomparable concept pairs in a signature |
| `split-candidates` | Functions serving incomparable state clusters |
| `merge-candidates` | Functions maintaining shared state separately |
| `extract-candidates` | Sub-operations shared by more callers than the full op |
| `boundary-recommendations` | Top-level: three Pareto frontiers (split/merge/extract) |
```

**Step 3: Update plans/CLAUDE.md**

Add entries to the plan files table.

**Step 4: Commit**

```
docs: add (wile goast fca-recommend) to CLAUDE.md and plans index
```

---

## Summary

| Task | What | Files | ~Lines |
|------|------|-------|--------|
| 1 | Testdata | `funcboundary.go` | 80 Go |
| 2 | Mapper: add struct field | `mapper.go`, `mapper_test.go` | 10 Go |
| 3 | Library scaffold | `fca-recommend.sld`, `fca-recommend.scm` | 15 Scheme |
| 4 | Pareto dominance | `fca-recommend.scm`, `fca_recommend_test.go` | 60 Scheme, 80 Go |
| 5 | Concept signature + incomparable pairs | `fca-recommend.scm`, `fca_recommend_test.go` | 40 Scheme, 50 Go |
| 6 | Split candidates (lattice) | `fca-recommend.scm`, `fca_recommend_test.go` | 80 Scheme, 40 Go |
| 7 | Cross-flow filter (SSA) | `fca-recommend.scm`, `fca_recommend_test.go` | 50 Scheme, 50 Go |
| 8 | Merge candidates | `fca-recommend.scm`, `fca_recommend_test.go` | 50 Scheme, 40 Go |
| 9 | Extract candidates | `fca-recommend.scm`, `fca_recommend_test.go` | 60 Scheme, 40 Go |
| 10 | boundary-recommendations | `fca-recommend.scm`, `fca_recommend_test.go` | 40 Scheme, 30 Go |
| 11 | Integration test | `fca_recommend_test.go` | 60 Go |
| 12 | CI + docs | `CLAUDE.md`, `plans/CLAUDE.md` | 20 prose |

Total: ~395 lines Scheme, ~470 lines Go test, ~80 lines Go testdata, ~10 lines Go mapper. 12 commits.

## Known risks

1. **Lattice size on large codebases.** The existing `concept-lattice` has a
   known performance issue with large contexts (669x803 hit it). The recommendation
   pipeline inherits this. Mitigation: use `'cross-type-only` filter on
   `field-index->context` for large packages.

2. **SSA register naming.** `defuse-reachable?` tracks string names through
   operand lists. If the SSA builder reuses register names across blocks (it
   shouldn't, but verify), the cross-flow filter could produce false positives.

3. **Field name collisions.** Two structs with identically named fields
   (e.g., both have `Name`) are now distinguished by the `struct` mapper
   field. But `cluster-field-addrs` matches on `"Struct.Field"` strings,
   which is unambiguous.

4. **Exact/inexact arithmetic.** Ratio computations use `exact->inexact` to
   produce floating-point values for Pareto comparison. If Wile's `<=`
   handles mixed exact/inexact differently than expected, factor comparison
   may need adjustment.
