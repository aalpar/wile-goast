# False Boundary Detection — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `(wile goast fca)` — a Formal Concept Analysis library that discovers natural field groupings from SSA access patterns and compares them against actual struct boundaries.

**Architecture:** Pure Scheme library with no new Go primitives. Builds a formal context from `go-ssa-field-index` output, computes the concept lattice via NextClosure, then filters for cross-boundary concepts. Structured output compatible with MCP tool.

**Tech Stack:** Scheme (R7RS), `(wile algebra lattice)` for set theory, `go-ssa-field-index` for field access data.

**Design doc:** `plans/2026-04-08-false-boundary-detection-design.md`

---

### Task 1: Library scaffold

Create the `.sld` and empty `.scm` files so imports resolve.

**Files:**
- Create: `cmd/wile-goast/lib/wile/goast/fca.sld`
- Create: `cmd/wile-goast/lib/wile/goast/fca.scm`

**Step 1: Create the library definition**

`cmd/wile-goast/lib/wile/goast/fca.sld`:
```scheme
(define-library (wile goast fca)
  (export
    ;; Context construction
    make-context
    context-from-alist
    context-objects
    context-attributes
    field-index->context

    ;; Derivation operators
    intent
    extent

    ;; Concept lattice
    concept-lattice
    concept-extent
    concept-intent

    ;; Boundary discovery
    cross-boundary-concepts
    boundary-report)
  (import (wile goast utils))
  (include "fca.scm"))
```

**Step 2: Create empty implementation**

`cmd/wile-goast/lib/wile/goast/fca.scm`:
```scheme
;;; fca.scm — Formal Concept Analysis for false boundary detection
;;;
;;; Discovers natural field groupings from function access patterns
;;; via concept lattice construction (NextClosure, Ganter 1984).
;;; Compares discovered groupings against actual struct boundaries.
```

**Step 3: Commit**

```
scaffold: add (wile goast fca) library definition
```

---

### Task 2: Context construction

Build formal contexts from raw data. Internal sorted-set operations.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca.scm`
- Create: `goast/fca_test.go`

**Step 1: Write the failing test**

`goast/fca_test.go`:
```go
package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestFCA_ContextFromAlist(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	run(t, engine, `(import (wile goast fca))`)

	// Build context: 3 functions accessing fields from 2 structs
	result := run(t, engine, `
		(let ((ctx (context-from-alist
		             '(("pkg.F1" "A.x" "A.y" "B.z")
		               ("pkg.F2" "A.x" "A.y" "B.z")
		               ("pkg.F3" "A.x" "A.y")))))
		  (list (length (context-objects ctx))
		        (length (context-attributes ctx))))`)
	c.Assert(result.SchemeString(), qt.Equals, "(3 3)")
}

func TestFCA_ContextAttributesSorted(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	run(t, engine, `(import (wile goast fca))`)

	result := run(t, engine, `
		(let ((ctx (context-from-alist
		             '(("f1" "C.z" "A.x" "B.y")))))
		  (context-attributes ctx))`)
	// Attributes should be sorted lexicographically
	c.Assert(result.SchemeString(), qt.Equals, `("A.x" "B.y" "C.z")`)
}
```

Note: uses the existing `run` helper (same as `eval` in `prim_goast_test.go` — check which name the test file uses and match it).

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestFCA -v -count=1`
Expected: FAIL — functions not defined.

**Step 3: Implement context construction**

Add to `fca.scm`:
```scheme
;; ══════════════════════════════════════════════════════════
;; Sorted string set operations (internal)
;; ══════════════════════════════════════════════════════════

(define (sort-strings lst)
  (define (insert s sorted)
    (cond ((null? sorted) (list s))
          ((string<? s (car sorted)) (cons s sorted))
          ((string=? s (car sorted)) sorted)
          (else (cons (car sorted) (insert s (cdr sorted))))))
  (let loop ((xs lst) (acc '()))
    (if (null? xs) acc
      (loop (cdr xs) (insert (car xs) acc)))))

(define (set-intersect a b)
  (cond ((or (null? a) (null? b)) '())
        ((string=? (car a) (car b))
         (cons (car a) (set-intersect (cdr a) (cdr b))))
        ((string<? (car a) (car b))
         (set-intersect (cdr a) b))
        (else
         (set-intersect a (cdr b)))))

(define (set-member? elem s)
  (cond ((null? s) #f)
        ((string=? elem (car s)) #t)
        ((string<? elem (car s)) #f)
        (else (set-member? elem (cdr s)))))

(define (set-add s elem)
  (cond ((null? s) (list elem))
        ((string=? elem (car s)) s)
        ((string<? elem (car s)) (cons elem s))
        (else (cons (car s) (set-add (cdr s) elem)))))

(define (set-before s elem)
  (cond ((null? s) '())
        ((string<? (car s) elem)
         (cons (car s) (set-before (cdr s) elem)))
        (else '())))

;; ══════════════════════════════════════════════════════════
;; Context construction
;; ══════════════════════════════════════════════════════════

(define (make-context objects attributes incidence)
  (let* ((sorted-objs (sort-strings objects))
         (sorted-attrs (sort-strings attributes))
         (obj->attrs
           (map (lambda (o)
                  (cons o (filter (lambda (a) (incidence o a))
                                 sorted-attrs)))
                sorted-objs))
         (attr->objs
           (map (lambda (a)
                  (cons a (filter (lambda (o) (incidence o a))
                                 sorted-objs)))
                sorted-attrs)))
    (list 'fca-context
          (cons 'objects sorted-objs)
          (cons 'attributes sorted-attrs)
          (cons 'obj->attrs obj->attrs)
          (cons 'attr->objs attr->objs))))

(define (context-objects ctx) (cdr (assoc 'objects (cdr ctx))))
(define (context-attributes ctx) (cdr (assoc 'attributes (cdr ctx))))
(define (context-obj->attrs ctx) (cdr (assoc 'obj->attrs (cdr ctx))))
(define (context-attr->objs ctx) (cdr (assoc 'attr->objs (cdr ctx))))

(define (context-from-alist entries)
  (let* ((objects (map car entries))
         (all-attrs (apply append (map cdr entries)))
         (sorted-attrs (sort-strings all-attrs)))
    (make-context objects sorted-attrs
      (lambda (o a)
        (let ((entry (assoc o entries)))
          (and entry (member a (cdr entry)) #t))))))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestFCA -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(fca): context construction with sorted-set internals
```

---

### Task 3: Derivation operators

The Galois connection: `intent` (objects → shared attributes) and `extent` (attributes → objects that have all).

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca.scm`
- Modify: `goast/fca_test.go`

**Step 1: Write the failing test**

Append to `goast/fca_test.go`:
```go
func TestFCA_Intent(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	run(t, engine, `(import (wile goast fca))`)

	// Context: F1={A.x, A.y, B.z}, F2={A.x, A.y, B.z}, F3={A.x, A.y}
	run(t, engine, `
		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))`)

	// Intent of all 3 functions = attributes they ALL share
	result := run(t, engine, `(intent ctx '("F1" "F2" "F3"))`)
	c.Assert(result.SchemeString(), qt.Equals, `("A.x" "A.y")`)

	// Intent of F1 and F2 = all 3 attributes
	result = run(t, engine, `(intent ctx '("F1" "F2"))`)
	c.Assert(result.SchemeString(), qt.Equals, `("A.x" "A.y" "B.z")`)

	// Intent of empty set = all attributes (vacuous)
	result = run(t, engine, `(intent ctx '())`)
	c.Assert(result.SchemeString(), qt.Equals, `("A.x" "A.y" "B.z")`)
}

func TestFCA_Extent(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	run(t, engine, `(import (wile goast fca))`)

	run(t, engine, `
		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))`)

	// Extent of {A.x, A.y} = all functions that have BOTH
	result := run(t, engine, `(extent ctx '("A.x" "A.y"))`)
	c.Assert(result.SchemeString(), qt.Equals, `("F1" "F2" "F3")`)

	// Extent of {A.x, A.y, B.z} = only F1 and F2
	result = run(t, engine, `(extent ctx '("A.x" "A.y" "B.z"))`)
	c.Assert(result.SchemeString(), qt.Equals, `("F1" "F2")`)

	// Extent of empty set = all objects
	result = run(t, engine, `(extent ctx '())`)
	c.Assert(result.SchemeString(), qt.Equals, `("F1" "F2" "F3")`)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestFCA_Intent|TestFCA_Extent" -v -count=1`
Expected: FAIL

**Step 3: Implement derivation operators**

Add to `fca.scm`:
```scheme
;; ══════════════════════════════════════════════════════════
;; Derivation operators (Galois connection)
;; ══════════════════════════════════════════════════════════

(define (intent ctx object-set)
  (let ((oa (context-obj->attrs ctx)))
    (if (null? object-set)
      (context-attributes ctx)
      (let loop ((objs (cdr object-set))
                 (acc (cdr (assoc (car object-set) oa))))
        (if (null? objs) acc
          (loop (cdr objs)
                (set-intersect acc (cdr (assoc (car objs) oa)))))))))

(define (extent ctx attribute-set)
  (let ((ao (context-attr->objs ctx)))
    (if (null? attribute-set)
      (context-objects ctx)
      (let loop ((attrs (cdr attribute-set))
                 (acc (cdr (assoc (car attribute-set) ao))))
        (if (null? attrs) acc
          (loop (cdr attrs)
                (set-intersect acc (cdr (assoc (car attrs) ao)))))))))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestFCA_Intent|TestFCA_Extent" -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(fca): derivation operators (intent/extent Galois connection)
```

---

### Task 4: Concept lattice via NextClosure

The core algorithm. Generates all concepts in lexicographic order.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca.scm`
- Modify: `goast/fca_test.go`

**Step 1: Write the failing test**

Append to `goast/fca_test.go`:
```go
func TestFCA_ConceptLattice(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	run(t, engine, `(import (wile goast fca))`)

	// Context: F1={A.x, A.y, B.z}, F2={A.x, A.y, B.z}, F3={A.x, A.y}
	// Expected concepts (by hand calculation):
	//   ({F1,F2,F3}, {A.x, A.y})      — top: all functions share A.x and A.y
	//   ({F1,F2},    {A.x, A.y, B.z}) — F1+F2 share all three
	run(t, engine, `
		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))`)

	result := run(t, engine, `
		(let ((lat (concept-lattice ctx)))
		  (length lat))`)
	c.Assert(result.SchemeString(), qt.Equals, "2")

	// Verify the cross-boundary concept exists: extent={F1,F2}, intent includes B.z
	result = run(t, engine, `
		(let* ((lat (concept-lattice ctx))
		       (intents (map concept-intent lat)))
		  (and (member '("A.x" "A.y" "B.z") intents) #t))`)
	c.Assert(result.SchemeString(), qt.Equals, "#t")
}

func TestFCA_ConceptLattice_NoCrossing(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	run(t, engine, `(import (wile goast fca))`)

	// Functions touch disjoint struct fields — no single concept spans both
	run(t, engine, `
		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y")
		    ("F2" "A.x" "A.y")
		    ("F3" "B.z" "B.w"))))`)

	result := run(t, engine, `
		(let ((lat (concept-lattice ctx)))
		  (length lat))`)
	// 3 concepts: {F1,F2,F3}x{}, {F1,F2}x{A.x,A.y}, {F3}x{B.z,B.w}
	c.Assert(result.SchemeString(), qt.Equals, "3")
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestFCA_ConceptLattice" -v -count=1`
Expected: FAIL

**Step 3: Implement NextClosure and concept-lattice**

Add to `fca.scm`:
```scheme
;; ══════════════════════════════════════════════════════════
;; Concept lattice — NextClosure (Ganter 1984)
;;
;; Generates all concepts in lectic (lexicographic-from-right)
;; order. For each position i from last to first: if attribute
;; a_i is not in the current set, try adding it (after trimming
;; everything at or after position i), close, and check that
;; the closure didn't add anything before position i.
;; ══════════════════════════════════════════════════════════

(define (concept-extent c) (car c))
(define (concept-intent c) (cdr c))

(define (next-closure current attrs close)
  (let ((attr-vec (list->vector attrs))
        (n (length attrs)))
    (let loop ((i (- n 1)))
      (if (< i 0) #f
        (let ((ai (vector-ref attr-vec i)))
          (if (set-member? ai current)
            (loop (- i 1))
            (let* ((prefix (set-before current ai))
                   (b-prime (set-add prefix ai))
                   (c (close b-prime))
                   (c-prefix (set-before c ai)))
              (if (equal? c-prefix prefix)
                c
                (loop (- i 1))))))))))

(define (concept-lattice ctx)
  (let* ((attrs (context-attributes ctx))
         (close (lambda (b) (intent ctx (extent ctx b))))
         (first-intent (close '())))
    (let loop ((current first-intent) (concepts '()))
      (let* ((ext (extent ctx current))
             (concept (cons ext current))
             (next (next-closure current attrs close)))
        (if next
          (loop next (cons concept concepts))
          (reverse (cons concept concepts)))))))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestFCA_ConceptLattice" -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(fca): NextClosure algorithm for concept lattice construction
```

---

### Task 5: Synthetic testdata

Create a Go package with two structs whose fields are always co-modified.

**Files:**
- Create: `examples/goast-query/testdata/falseboundary/falseboundary.go`

**Step 1: Create the testdata package**

`examples/goast-query/testdata/falseboundary/falseboundary.go`:
```go
package falseboundary

// Cache holds cached entries with a time-to-live.
type Cache struct {
	Entries []string
	TTL     int
}

// Index holds lookup keys with a version counter.
type Index struct {
	Keys    []string
	Version int
}

// UpdateBoth writes to both Cache and Index fields.
func UpdateBoth(c *Cache, idx *Index, entry string, key string) {
	c.Entries = append(c.Entries, entry)
	c.TTL = 300
	idx.Keys = append(idx.Keys, key)
	idx.Version++
}

// Invalidate clears both Cache and Index.
func Invalidate(c *Cache, idx *Index) {
	c.Entries = nil
	c.TTL = 0
	idx.Keys = nil
	idx.Version = 0
}

// Rebuild replaces both Cache and Index contents.
func Rebuild(c *Cache, idx *Index, entries []string, keys []string) {
	c.Entries = entries
	c.TTL = 600
	idx.Keys = keys
	idx.Version++
}

// CacheOnly touches only Cache fields — not cross-coupled.
func CacheOnly(c *Cache) {
	c.TTL = 0
}

// IndexOnly touches only Index fields — not cross-coupled.
func IndexOnly(idx *Index) {
	idx.Version++
}
```

**Step 2: Verify it compiles**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go build ./examples/goast-query/testdata/falseboundary/`
Expected: Success (no output)

**Step 3: Commit**

```
testdata: add falseboundary package for FCA validation
```

---

### Task 6: field-index->context bridge

Transform `go-ssa-field-index` output into a formal context.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca.scm`
- Modify: `goast/fca_test.go`

**Step 1: Write the failing test**

Append to `goast/fca_test.go`:
```go
func TestFCA_FieldIndexToContext(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	run(t, engine, `(import (wile goast fca))`)

	// Load the falseboundary testdata and build context from field index
	run(t, engine, `
		(define s (go-load
		  "github.com/aalpar/wile-goast/examples/goast-query/testdata/falseboundary"))
		(define idx (go-ssa-field-index s))
		(define ctx (field-index->context idx 'write-only))`)

	// Should have 5 functions as objects
	result := run(t, engine, `(length (context-objects ctx))`)
	c.Assert(result.SchemeString(), qt.Equals, "5")

	// Attributes should be qualified as "Struct.Field"
	result = run(t, engine, `
		(let ((attrs (context-attributes ctx)))
		  (and (member "Cache.Entries" attrs)
		       (member "Index.Keys" attrs)
		       #t))`)
	c.Assert(result.SchemeString(), qt.Equals, "#t")
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestFCA_FieldIndex -v -count=1`
Expected: FAIL

**Step 3: Implement field-index->context**

Add to `fca.scm`:
```scheme
;; ══════════════════════════════════════════════════════════
;; Bridge: go-ssa-field-index -> formal context
;; ══════════════════════════════════════════════════════════

(define (string-index-of str ch)
  (let ((n (string-length str)))
    (let loop ((i 0))
      (cond ((= i n) #f)
            ((char=? (string-ref str i) ch) i)
            (else (loop (+ i 1)))))))

(define (field-index->context field-index mode)
  (let ((entries
          (filter-map
            (lambda (summary)
              (let* ((func-name (nf summary 'func))
                     (pkg-name (nf summary 'pkg))
                     (qualified (string-append pkg-name "." func-name))
                     (fields (nf summary 'fields))
                     (attrs
                       (filter-map
                         (lambda (fa)
                           (let ((struct-name (nf fa 'struct))
                                 (field-name (nf fa 'field))
                                 (access-mode (nf fa 'mode)))
                             (case mode
                               ((write-only)
                                (and (eq? access-mode 'write)
                                     (string-append struct-name "."
                                                    field-name)))
                               ((read-write)
                                (string-append struct-name "." field-name
                                  (if (eq? access-mode 'write) ":w" ":r")))
                               ((type-only)
                                struct-name)
                               (else
                                (string-append struct-name "."
                                               field-name)))))
                         fields)))
                (and (pair? attrs) (cons qualified (unique attrs)))))
            field-index)))
    (context-from-alist entries)))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestFCA_FieldIndex -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(fca): field-index->context bridge for SSA field access data
```

---

### Task 7: Cross-boundary detection and reporting

Filter concepts that span multiple struct types. Format as structured output.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/fca.scm`
- Modify: `goast/fca_test.go`

**Step 1: Write the failing test**

Append to `goast/fca_test.go`:
```go
func TestFCA_CrossBoundaryConcepts(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	run(t, engine, `(import (wile goast fca))`)

	// Same 3-function context from earlier tests
	run(t, engine, `
		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))
		(define lat (concept-lattice ctx))`)

	// One concept should cross the A/B boundary
	result := run(t, engine, `
		(let ((xb (cross-boundary-concepts lat)))
		  (length xb))`)
	c.Assert(result.SchemeString(), qt.Equals, "1")

	// Its extent should be F1 and F2
	result = run(t, engine, `
		(let ((xb (cross-boundary-concepts lat)))
		  (concept-extent (car xb)))`)
	c.Assert(result.SchemeString(), qt.Equals, `("F1" "F2")`)
}

func TestFCA_CrossBoundaryMinExtent(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	run(t, engine, `(import (wile goast fca))`)

	run(t, engine, `
		(define ctx (context-from-alist
		  '(("F1" "A.x" "A.y" "B.z")
		    ("F2" "A.x" "A.y" "B.z")
		    ("F3" "A.x" "A.y"))))
		(define lat (concept-lattice ctx))`)

	// min-extent 3 should filter out the cross-boundary concept (only 2 functions)
	result := run(t, engine, `
		(let ((xb (cross-boundary-concepts lat 'min-extent 3)))
		  (length xb))`)
	c.Assert(result.SchemeString(), qt.Equals, "0")
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestFCA_CrossBoundary" -v -count=1`
Expected: FAIL

**Step 3: Implement cross-boundary detection**

Add to `fca.scm`:
```scheme
;; ══════════════════════════════════════════════════════════
;; Boundary discovery
;; ══════════════════════════════════════════════════════════

(define (attr-struct-name attr)
  (let ((dot (string-index-of attr #\.)))
    (if dot (substring attr 0 dot) attr)))

(define (plist-ref opts key default)
  (let loop ((xs opts))
    (cond ((null? xs) default)
          ((null? (cdr xs)) default)
          ((eq? (car xs) key) (cadr xs))
          (else (loop (cdr xs))))))

(define (cross-boundary-concepts lattice . opts)
  (let ((min-extent (plist-ref opts 'min-extent 2))
        (min-intent (plist-ref opts 'min-intent 2))
        (min-types  (plist-ref opts 'min-types 2)))
    (filter
      (lambda (concept)
        (let* ((ext (concept-extent concept))
               (int (concept-intent concept))
               (types (unique (map attr-struct-name int))))
          (and (>= (length ext) min-extent)
               (>= (length int) min-intent)
               (>= (length types) min-types))))
      lattice)))

(define (group-fields-by-struct attrs)
  (let loop ((as attrs) (groups '()))
    (if (null? as)
      (map (lambda (g) (cons (car g) (reverse (cdr g))))
           (reverse groups))
      (let* ((attr (car as))
             (sn (attr-struct-name attr))
             (existing (assoc sn groups)))
        (if existing
          (loop (cdr as)
                (map (lambda (g)
                       (if (string=? (car g) sn)
                         (cons sn (cons attr (cdr g)))
                         g))
                     groups))
          (loop (cdr as)
                (cons (list sn attr) groups)))))))

(define (boundary-report concepts)
  (map (lambda (concept)
         (let* ((ext (concept-extent concept))
                (int (concept-intent concept))
                (grouped (group-fields-by-struct int))
                (types (map car grouped)))
           `((types ,types)
             (fields ,grouped)
             (functions ,ext)
             (extent-size ,(length ext)))))
       concepts))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestFCA_CrossBoundary" -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat(fca): cross-boundary concept detection and reporting
```

---

### Task 8: Integration test — full pipeline

End-to-end: load Go package -> field index -> context -> lattice -> boundary report.

**Files:**
- Modify: `goast/fca_test.go`

**Step 1: Write the integration test**

Append to `goast/fca_test.go`:
```go
func TestFCA_Integration_FalseBoundary(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	run(t, engine, `(import (wile goast fca))`)

	// Full pipeline on the falseboundary testdata
	result := run(t, engine, `
		(let* ((s (go-load
		          "github.com/aalpar/wile-goast/examples/goast-query/testdata/falseboundary"))
		       (idx (go-ssa-field-index s))
		       (ctx (field-index->context idx 'write-only))
		       (lat (concept-lattice ctx))
		       (xb  (cross-boundary-concepts lat 'min-extent 2))
		       (rpt (boundary-report xb)))
		  ;; At least one cross-boundary concept should exist
		  ;; spanning Cache and Index types
		  (and (pair? rpt)
		       (let ((first (car rpt)))
		         (let ((types (cdr (assoc 'types first))))
		           (and (member "Cache" types)
		                (member "Index" types)
		                #t)))))`)
	c.Assert(result.SchemeString(), qt.Equals, "#t")

	// The cross-boundary concept should be backed by 3 functions
	// (UpdateBoth, Invalidate, Rebuild)
	result = run(t, engine, `
		(let* ((s (go-load
		          "github.com/aalpar/wile-goast/examples/goast-query/testdata/falseboundary"))
		       (idx (go-ssa-field-index s))
		       (ctx (field-index->context idx 'write-only))
		       (lat (concept-lattice ctx))
		       (xb  (cross-boundary-concepts lat 'min-extent 2))
		       (rpt (boundary-report xb))
		       (first (car rpt)))
		  (cdr (assoc 'extent-size first)))`)
	c.Assert(result.SchemeString(), qt.Equals, "3")
}
```

**Step 2: Run all FCA tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestFCA -v -count=1`
Expected: All PASS

**Step 3: Run full test suite**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make test`
Expected: All PASS, no regressions

**Step 4: Commit**

```
test(fca): integration test — full pipeline on falseboundary testdata
```

---

### Task 9: Final verification and documentation

**Step 1: Run CI**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: PASS (lint + build + test + coverage)

**Step 2: Update CLAUDE.md**

Add `(wile goast fca)` to the Key Scheme Libraries table and the primitives
table in the project's `CLAUDE.md`. Add to MEMORY.md if needed.

**Step 3: Commit**

```
docs: add (wile goast fca) to CLAUDE.md
```

---

## Summary

| Task | What | Files |
|------|------|-------|
| 1 | Library scaffold | `fca.sld`, `fca.scm` |
| 2 | Context construction | `fca.scm`, `fca_test.go` |
| 3 | Derivation operators | `fca.scm`, `fca_test.go` |
| 4 | NextClosure lattice | `fca.scm`, `fca_test.go` |
| 5 | Synthetic testdata | `falseboundary.go` |
| 6 | field-index->context | `fca.scm`, `fca_test.go` |
| 7 | Boundary detection | `fca.scm`, `fca_test.go` |
| 8 | Integration test | `fca_test.go` |
| 9 | CI + docs | `CLAUDE.md` |

Total: ~200 lines of Scheme, ~150 lines of Go test, 1 testdata file, 9 commits.

## Implementation note: test helper name

The test file uses either `eval` or `run` as the Scheme execution helper.
Check `goast/prim_goast_test.go` for the actual name before writing tests.
The plan uses `run` as a placeholder — substitute the real name.
