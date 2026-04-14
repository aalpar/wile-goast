# Package Splitting — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `go-func-refs` (Go primitive) and `(wile goast split)` (Scheme library) to analyze Go package functions by their external dependency profiles and recommend package splits.

**Architecture:** New Go primitive `go-func-refs` walks type-checked ASTs to extract per-function external references via `types.Info.Uses`. Scheme library builds on `(wile goast fca)` for concept lattice computation, adds IDF weighting for noise filtering, and produces split recommendations with min-cut analysis and cycle verification.

**Tech Stack:** Go (`go/ast`, `go/types`, `golang.org/x/tools/go/packages`), Scheme R7RS (`(wile goast fca)`, `(wile goast utils)`).

**Design doc:** `plans/2026-04-12-package-splitting-design.md`

---

### Task 1: `go-func-refs` — failing test

Write a test that exercises the primitive on a package with known dependency structure.

**Files:**
- Modify: `goast/prim_goast_test.go`

**Step 1: Write the failing test**

Add to `goast/prim_goast_test.go`:
```go
func TestGoFuncRefs(t *testing.T) {
	engine := newEngine(t)

	// Use goast's own testdata — it has a known import structure.
	result := eval(t, engine, `
		(import (wile goast utils))
		(define refs (go-func-refs
		  "github.com/aalpar/wile-goast/goast/testdata/iface"))
		refs
	`)

	c := qt.New(t)
	// Should return a list of func-ref alists, one per function/method.
	c.Assert(result.SchemeString(), qt.Not(qt.Equals), "()")

	t.Run("each entry is a func-ref", func(t *testing.T) {
		result := eval(t, engine, `
			(define first-ref (car refs))
			(car first-ref)`)
		qt.New(t).Assert(result.SchemeString(), qt.Equals, "func-ref")
	})

	t.Run("entry has name field", func(t *testing.T) {
		result := eval(t, engine, `(nf (car refs) 'name)`)
		qt.New(t).Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})

	t.Run("entry has pkg field", func(t *testing.T) {
		result := eval(t, engine, `(nf (car refs) 'pkg)`)
		qt.New(t).Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})

	t.Run("entry has refs field", func(t *testing.T) {
		result := eval(t, engine, `(nf (car refs) 'refs)`)
		qt.New(t).Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})
}

func TestGoFuncRefs_WithSession(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(import (wile goast utils))
		(define s (go-load
		  "github.com/aalpar/wile-goast/goast/testdata/iface"))
		(define refs (go-func-refs s))
	`)

	t.Run("returns same data from session", func(t *testing.T) {
		result := eval(t, engine, `(length refs)`)
		qt.New(t).Assert(result.SchemeString(), qt.Not(qt.Equals), "0")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestGoFuncRefs -v`
Expected: FAIL — `go-func-refs` is not a known primitive.

**Step 3: Commit**

```
test(go-func-refs): failing tests for external reference extraction
```

---

### Task 2: `go-func-refs` — implementation

Register and implement the primitive. For each function/method in the loaded packages, walk the body with `ast.Inspect`, look up each `*ast.Ident` in `types.Info.Uses`, and collect external `(package-path, object-name)` pairs.

**Files:**
- Create: `goast/prim_funcrefs.go`
- Modify: `goast/register.go`

**Step 1: Implement the primitive**

Create `goast/prim_funcrefs.go`:
```go
package goast

import (
	"go/ast"
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"

	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

// PrimGoFuncRefs implements (go-func-refs target).
// target is a package pattern string or GoSession.
// For each function/method, returns the set of external
// (package-path, object-name) pairs it references.
func PrimGoFuncRefs(mc machine.CallContext) error {
	arg := mc.Arg(0)
	switch v := arg.(type) {
	case *GoSession:
		return funcRefsFromSession(mc, v)
	case *values.String:
		return funcRefsFromPattern(mc, v)
	default:
		return werr.WrapForeignErrorf(werr.ErrNotAString,
			"go-func-refs: expected string or go-session, got %T", arg)
	}
}

func funcRefsFromSession(mc machine.CallContext, session *GoSession) error {
	var result []values.Value
	for _, pkg := range session.Packages() {
		result = append(result, collectFuncRefs(pkg)...)
	}
	mc.SetValue(ValueList(result))
	return nil
}

func funcRefsFromPattern(mc machine.CallContext, pattern *values.String) error {
	pkgs, err := LoadPackagesChecked(mc,
		packages.NeedName|packages.NeedFiles|packages.NeedSyntax|
			packages.NeedTypes|packages.NeedTypesInfo,
		nil, errGoPackageLoadError, "go-func-refs",
		pattern.Value)
	if err != nil {
		return err
	}
	var result []values.Value
	for _, pkg := range pkgs {
		result = append(result, collectFuncRefs(pkg)...)
	}
	mc.SetValue(ValueList(result))
	return nil
}

// collectFuncRefs extracts external references for each function in pkg.
func collectFuncRefs(pkg *packages.Package) []values.Value {
	var result []values.Value
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			name := funcDeclName(fn)
			refs := extractRefs(fn.Body, pkg.TypesInfo, pkg.PkgPath)
			result = append(result, Node("func-ref",
				Field("name", Str(name)),
				Field("pkg", Str(pkg.PkgPath)),
				Field("refs", mapRefGroups(refs)),
			))
		}
	}
	return result
}

// funcDeclName returns "RecvType.Method" or just "Func".
func funcDeclName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	return recvTypeName(recv) + "." + fn.Name.Name
}

// recvTypeName extracts the type name from a receiver expression,
// handling *T and T forms.
func recvTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return recvTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return recvTypeName(t.X)
	case *ast.IndexListExpr:
		return recvTypeName(t.X)
	default:
		return "?"
	}
}

// refGroup groups object names by package path.
type refGroup struct {
	pkg     string
	objects []string
}

// extractRefs walks the function body and collects external references.
func extractRefs(body *ast.BlockStmt, info *types.Info, selfPkg string) []refGroup {
	seen := make(map[string]map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj, exists := info.Uses[id]
		if !exists {
			return true
		}
		objPkg := obj.Pkg()
		if objPkg == nil || objPkg.Path() == selfPkg {
			return true
		}
		pkgPath := objPkg.Path()
		if seen[pkgPath] == nil {
			seen[pkgPath] = make(map[string]bool)
		}
		seen[pkgPath][obj.Name()] = true
		return true
	})

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	groups := make([]refGroup, len(paths))
	for i, p := range paths {
		names := make([]string, 0, len(seen[p]))
		for n := range seen[p] {
			names = append(names, n)
		}
		sort.Strings(names)
		groups[i] = refGroup{pkg: p, objects: names}
	}
	return groups
}

// mapRefGroups converts refGroups to s-expression alist nodes.
func mapRefGroups(groups []refGroup) values.Value {
	result := make([]values.Value, len(groups))
	for i, g := range groups {
		objs := make([]values.Value, len(g.objects))
		for j, o := range g.objects {
			objs[j] = Str(o)
		}
		result[i] = Node("ref",
			Field("pkg", Str(g.pkg)),
			Field("objects", ValueList(objs)),
		)
	}
	return ValueList(result)
}
```

**Step 2: Register the primitive**

Add to `goast/register.go` in the `Extension` list, after the `go-list-deps` entry:
```go
{Name: "go-func-refs", ParamCount: 1, Impl: PrimGoFuncRefs,
	Doc: "Returns per-function external reference profiles for a Go package.\n" +
		"For each function/method, lists the external (package, object-names)\n" +
		"pairs it references via types.Info.Uses.\n" +
		"Input: package pattern string or GoSession.\n" +
		"Output: list of (func-ref (name . N) (pkg . P) (refs . ((ref ...)))).\n\n" +
		"Examples:\n" +
		"  (import (wile goast utils))\n" +
		"  (define refs (go-func-refs \"my/pkg\"))\n" +
		"  (nf (car refs) 'name)    ; => \"MyFunc\"\n" +
		"  (nf (car refs) 'refs)    ; => ((ref (pkg . \"io\") (objects . (\"Reader\"))))\n\n" +
		"See also: `go-typecheck-package', `go-load', `go-list-deps'.",
	ParamNames: []string{"target"}, Category: "goast",
	ReturnType: values.TypeList},
```

**Step 3: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestGoFuncRefs -v`
Expected: PASS

**Step 4: Commit**

```
feat(go-func-refs): extract per-function external reference profiles
```

---

### Task 3: `go-func-refs` — verify on goast/ itself

Add a test that analyzes the `goast/` package itself and validates the output structure against known facts (goast imports `go/ast`, `go/types`, etc.).

**Files:**
- Modify: `goast/prim_goast_test.go`

**Step 1: Write the self-analysis test**

```go
func TestGoFuncRefs_SelfAnalysis(t *testing.T) {
	engine := newEngine(t)

	eval(t, engine, `
		(import (wile goast utils))
		(define refs (go-func-refs
		  "github.com/aalpar/wile-goast/goast"))
	`)

	c := qt.New(t)

	t.Run("many functions found", func(t *testing.T) {
		result := eval(t, engine, `(> (length refs) 20)`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("some function references go/ast", func(t *testing.T) {
		result := eval(t, engine, `
			(let loop ((rs refs))
			  (cond ((null? rs) #f)
			        ((let ((r (nf (car rs) 'refs)))
			           (and r (let check ((gs r))
			             (cond ((null? gs) #f)
			                   ((equal? (nf (car gs) 'pkg) "go/ast") #t)
			                   (else (check (cdr gs)))))))
			         #t)
			        (else (loop (cdr rs)))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestGoFuncRefs_SelfAnalysis -v`
Expected: PASS

**Step 3: Commit**

```
test(go-func-refs): self-analysis on goast/ package
```

---

### Task 4: `(wile goast split)` — library scaffold and `import-signatures`

Create the library with the first function: `import-signatures` extracts per-function package-level dependency sets from `go-func-refs` output.

**Files:**
- Create: `cmd/wile-goast/lib/wile/goast/split.sld`
- Create: `cmd/wile-goast/lib/wile/goast/split.scm`
- Verify: `cmd/wile-goast/embed.go` glob covers new files

**Step 1: Create the library definition**

`cmd/wile-goast/lib/wile/goast/split.sld`:
```scheme
;; Copyright 2026 Aaron Alpar
;;
;; Licensed under the Apache License, Version 2.0 (the "License");
;; you may not use this file except in compliance with the License.
;; You may obtain a copy of the License at
;;
;;     http://www.apache.org/licenses/LICENSE-2.0
;;
;; Unless required by applicable law or agreed to in writing, software
;; distributed under the License is distributed on an "AS IS" BASIS,
;; WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
;; See the License for the specific language governing permissions and
;; limitations under the License.

(define-library (wile goast split)
  (import (scheme base))
  (import (wile goast utils))
  (import (wile goast fca))
  (export
    import-signatures)
  (include "split.scm"))
```

`cmd/wile-goast/lib/wile/goast/split.scm`:
```scheme
;; Copyright 2026 Aaron Alpar
;;
;; Licensed under the Apache License, Version 2.0 (the "License");
;; you may not use this file except in compliance with the License.
;; You may obtain a copy of the License at
;;
;;     http://www.apache.org/licenses/LICENSE-2.0
;;
;; Unless required by applicable law or agreed to in writing, software
;; distributed under the License is distributed on an "AS IS" BASIS,
;; WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
;; See the License for the specific language governing permissions and
;; limitations under the License.

;;; (wile goast split) — Package splitting via import signature analysis
;;;
;;; Analyzes a Go package's functions by their external dependency profiles
;;; to discover natural package boundaries.

(define (import-signatures func-refs)
  "Extract per-function import signatures from go-func-refs output.
Each function maps to the set of external package paths it references.

Parameters:
  func-refs : list — output from (go-func-refs ...)
Returns: list — alist mapping function name to list of package paths
Category: goast-split

Examples:
  (import-signatures (go-func-refs \"my/pkg\"))
  ;; => ((\"MyFunc\" \"io\" \"fmt\") (\"Helper\" \"strings\"))

See also: `compute-idf', `filter-noise'."
  (map (lambda (fr)
         (cons (nf fr 'name)
               (map (lambda (r) (nf r 'pkg))
                    (let ((refs (nf fr 'refs)))
                      (if refs refs '())))))
       func-refs))
```

**Step 2: Verify embed glob covers new files**

Check that `cmd/wile-goast/embed.go` embeds `lib/wile/goast/*.scm` and `lib/wile/goast/*.sld` — the existing wildcard should cover `split.scm` and `split.sld` automatically.

**Step 3: Write the test**

Create `goast/split_test.go`:
```go
package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestSplit_ImportSignatures(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(import (wile goast utils))

		(define refs (go-func-refs
		  "github.com/aalpar/wile-goast/goast/testdata/iface"))
		(define sigs (import-signatures refs))
	`)

	c := qt.New(t)

	t.Run("returns alist", func(t *testing.T) {
		result := eval(t, engine, `(pair? (car sigs))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("car is function name", func(t *testing.T) {
		result := eval(t, engine, `(string? (caar sigs))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("cdr is list of package paths", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((first-sig (car sigs)))
			  (or (null? (cdr first-sig))
			      (string? (cadr first-sig))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 4: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestSplit_ImportSignatures -v`
Expected: PASS

**Step 5: Commit**

```
feat(split): library scaffold and import-signatures
```

---

### Task 5: `compute-idf` and `filter-noise`

IDF weighting identifies noise dependencies. `filter-noise` removes low-IDF packages.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/split.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/split.sld` (add exports)
- Modify: `goast/split_test.go`

**Step 1: Add implementations**

In `split.scm`:
```scheme
(define (compute-idf signatures)
  "Compute IDF weights for each external package.
IDF(pkg) = log(N / df(pkg)), where N = total functions, df = functions referencing pkg.
High IDF = rare dependency (informative). Low IDF = ubiquitous (noise).

Parameters:
  signatures : list — output from (import-signatures ...)
Returns: list — alist mapping package path to IDF score (inexact)
Category: goast-split

Examples:
  (compute-idf '((\"F1\" \"io\" \"fmt\") (\"F2\" \"io\")))
  ;; => ((\"io\" . 0.0) (\"fmt\" . 0.693...))

See also: `import-signatures', `filter-noise'."
  (let* ((n (length signatures))
         (df (make-df-table signatures))
         (all-pkgs (map car df)))
    (map (lambda (entry)
           (cons (car entry) (log (/ n (cdr entry)))))
         df)))

(define (make-df-table signatures)
  "Build document-frequency table: pkg -> count of functions referencing it."
  (let ((table '()))
    (for-each
      (lambda (sig)
        (for-each
          (lambda (pkg)
            (let ((entry (assoc pkg table)))
              (if entry
                (set-cdr! entry (+ (cdr entry) 1))
                (set! table (cons (cons pkg 1) table)))))
          (cdr sig)))
      signatures)
    table))

(define (filter-noise signatures idf-weights . opts)
  "Remove low-IDF (noise) packages from import signatures.
Default threshold: 0.36 (excludes packages in >70% of functions).

Parameters:
  signatures  : list — output from (import-signatures ...)
  idf-weights : list — output from (compute-idf ...)
  opts        : optional number — IDF threshold (default 0.36)
Returns: list — filtered signatures (same shape, fewer packages per entry)
Category: goast-split

Examples:
  (filter-noise sigs idf 0.5)

See also: `compute-idf', `build-package-context'."
  (let ((threshold (if (null? opts) 0.36 (car opts))))
    (map (lambda (sig)
           (cons (car sig)
                 (filter (lambda (pkg)
                           (let ((w (assoc pkg idf-weights)))
                             (and w (>= (cdr w) threshold))))
                         (cdr sig))))
         signatures)))
```

Update `split.sld` exports:
```scheme
  (export
    import-signatures
    compute-idf
    filter-noise)
```

**Step 2: Write tests**

Add to `goast/split_test.go`:
```go
func TestSplit_ComputeIDF(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))

		;; 3 functions: io appears in all 3 (IDF=0), fmt in 1 (IDF=log(3))
		(define sigs '(("F1" "io" "fmt")
		               ("F2" "io" "strings")
		               ("F3" "io")))
		(define idf (compute-idf sigs))
	`)

	c := qt.New(t)

	t.Run("io has IDF 0", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc "io" idf))`)
		c.Assert(result.SchemeString(), qt.Equals, "0.0")
	})

	t.Run("fmt has positive IDF", func(t *testing.T) {
		result := eval(t, engine, `(> (cdr (assoc "fmt" idf)) 0)`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}

func TestSplit_FilterNoise(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))

		(define sigs '(("F1" "io" "fmt")
		               ("F2" "io" "strings")
		               ("F3" "io")))
		(define idf (compute-idf sigs))
		(define filtered (filter-noise sigs idf))
	`)

	c := qt.New(t)

	t.Run("io removed (IDF=0 < threshold)", func(t *testing.T) {
		result := eval(t, engine, `
			(member "io" (cdr (assoc "F1" filtered)))`)
		c.Assert(result.SchemeString(), qt.Equals, "#f")
	})

	t.Run("fmt preserved (IDF > threshold)", func(t *testing.T) {
		result := eval(t, engine, `
			(not (equal? #f (member "fmt" (cdr (assoc "F1" filtered)))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 3: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestSplit_Compute|TestSplit_Filter" -v`
Expected: PASS

**Step 4: Commit**

```
feat(split): IDF weighting and noise filtering
```

---

### Task 6: `build-package-context` and `refine-by-api-surface`

Build FCA contexts at two granularities: package-level and (package, object) level.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/split.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/split.sld`
- Modify: `goast/split_test.go`

**Step 1: Add implementations**

In `split.scm`:
```scheme
(define (build-package-context filtered-signatures)
  "Build FCA formal context from filtered import signatures.
Objects = function names. Attributes = high-IDF package paths.

Parameters:
  filtered-signatures : list — output from (filter-noise ...)
Returns: fca-context — formal context for (concept-lattice)
Category: goast-split

Examples:
  (define ctx (build-package-context filtered))
  (concept-lattice ctx)

See also: `filter-noise', `refine-by-api-surface', `context-from-alist'."
  (context-from-alist
    (filter (lambda (sig) (not (null? (cdr sig))))
            filtered-signatures)))

(define (refine-by-api-surface func-refs filtered-signatures)
  "Refine FCA context to (package, object-name) granularity.
Replaces package-level attributes with pkg:object pairs for finer
sub-clustering when two functions import the same package but use
different API surfaces.

Parameters:
  func-refs             : list — raw output from (go-func-refs ...)
  filtered-signatures   : list — output from (filter-noise ...)
Returns: fca-context — refined formal context
Category: goast-split

Examples:
  (define ctx (refine-by-api-surface raw-refs filtered))
  (concept-lattice ctx)

See also: `build-package-context', `import-signatures'."
  (let* ((high-idf-pkgs
           (let loop ((sigs filtered-signatures) (acc '()))
             (if (null? sigs) acc
               (loop (cdr sigs)
                     (append (cdr (car sigs)) acc)))))
         (high-set (delete-duplicates high-idf-pkgs)))
    (context-from-alist
      (filter-map
        (lambda (fr)
          (let* ((name (nf fr 'name))
                 (refs (let ((r (nf fr 'refs))) (if r r '())))
                 (attrs
                   (let loop ((rs refs) (acc '()))
                     (if (null? rs) acc
                       (let* ((r (car rs))
                              (pkg (nf r 'pkg)))
                         (if (member pkg high-set)
                           (let ((objs (nf r 'objects)))
                             (loop (cdr rs)
                                   (append
                                     (map (lambda (o)
                                            (string-append pkg ":" o))
                                          (if objs objs '()))
                                     acc)))
                           (loop (cdr rs) acc)))))))
            (if (null? attrs) #f
              (cons name attrs))))
        func-refs))))

(define (delete-duplicates lst)
  "Remove duplicate strings from a list."
  (let loop ((xs lst) (seen '()) (acc '()))
    (cond ((null? xs) (reverse acc))
          ((member (car xs) seen) (loop (cdr xs) seen acc))
          (else (loop (cdr xs) (cons (car xs) seen) (cons (car xs) acc))))))

(define (filter-map f lst)
  "Map f over lst, removing #f results."
  (let loop ((xs lst) (acc '()))
    (cond ((null? xs) (reverse acc))
          ((f (car xs)) => (lambda (v) (loop (cdr xs) (cons v acc))))
          (else (loop (cdr xs) acc)))))
```

Update `split.sld` exports:
```scheme
  (export
    import-signatures
    compute-idf
    filter-noise
    build-package-context
    refine-by-api-surface)
```

**Step 2: Write tests**

Add to `goast/split_test.go`:
```go
func TestSplit_BuildPackageContext(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(import (wile goast fca))

		(define filtered '(("F1" "fmt" "strings")
		                    ("F2" "fmt")
		                    ("F3" "strings")))
		(define ctx (build-package-context filtered))
	`)

	c := qt.New(t)

	t.Run("3 objects", func(t *testing.T) {
		result := eval(t, engine, `(length (context-objects ctx))`)
		c.Assert(result.SchemeString(), qt.Equals, "3")
	})

	t.Run("2 attributes", func(t *testing.T) {
		result := eval(t, engine, `(length (context-attributes ctx))`)
		c.Assert(result.SchemeString(), qt.Equals, "2")
	})

	t.Run("functions with no deps excluded", func(t *testing.T) {
		result := eval(t, engine, `
			(define filtered2 '(("F1" "fmt") ("F2")))
			(define ctx2 (build-package-context filtered2))
			(length (context-objects ctx2))`)
		c.Assert(result.SchemeString(), qt.Equals, "1")
	})
}

func TestSplit_RefineByAPISurface(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(import (wile goast fca))

		;; Simulate go-func-refs output
		(define raw-refs
		  '((func-ref (name . "F1")
		              (pkg . "test/pkg")
		              (refs . ((ref (pkg . "io") (objects . ("Reader" "Writer")))
		                       (ref (pkg . "fmt") (objects . ("Println"))))))
		    (func-ref (name . "F2")
		              (pkg . "test/pkg")
		              (refs . ((ref (pkg . "io") (objects . ("Closer")))
		                       (ref (pkg . "fmt") (objects . ("Sprintf"))))))))
		;; Only io is high-IDF
		(define filtered '(("F1" "io") ("F2" "io")))
		(define ctx (refine-by-api-surface raw-refs filtered))
	`)

	c := qt.New(t)

	t.Run("attributes are pkg:object pairs", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((attrs (context-attributes ctx)))
			  (and (member "io:Reader" attrs)
			       (member "io:Closer" attrs)))`)
		c.Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})

	t.Run("F1 and F2 have different attributes", func(t *testing.T) {
		result := eval(t, engine, `
			(equal? (intent ctx '("F1"))
			        (intent ctx '("F2")))`)
		c.Assert(result.SchemeString(), qt.Equals, "#f")
	})
}
```

**Step 3: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run "TestSplit_Build|TestSplit_Refine" -v`
Expected: PASS

**Step 4: Commit**

```
feat(split): FCA context construction at package and API-surface granularity
```

---

### Task 7: `find-split` — min-cut on concept lattice

Find the two-way partition that minimizes cross-group references using the FCA lattice.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/split.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/split.sld`
- Modify: `goast/split_test.go`

**Step 1: Add implementation**

In `split.scm`:
```scheme
(define (find-split context lattice)
  "Find a two-way partition of functions minimizing cross-group coupling.
Uses the concept lattice to identify two large incomparable concepts,
then classifies each function by which concept's attributes it shares more.

Parameters:
  context : fca-context
  lattice : list — output from (concept-lattice context)
Returns: alist with keys: group-a, group-b, cut, cut-ratio
Category: goast-split

Examples:
  (define result (find-split ctx lat))
  (assoc 'group-a result)
  (assoc 'cut-ratio result)

See also: `build-package-context', `verify-acyclic'."
  (let* ((concepts (filter (lambda (c)
                             (> (length (concept-extent c)) 1))
                           lattice))
         (pairs (incomparable-concept-pairs concepts context))
         (best (best-split-pair pairs context)))
    (if (not best)
      '((group-a) (group-b) (cut) (cut-ratio . 1.0))
      (let* ((c1 (car best))
             (c2 (cdr best))
             (e1 (concept-extent c1))
             (e2 (concept-extent c2))
             (all-objs (context-objects context))
             (only-a (set-difference e1 e2))
             (only-b (set-difference e2 e1))
             (both (set-intersect e1 e2))
             (neither (set-difference
                        (set-difference all-objs e1) e2))
             (assigned (assign-remainder neither c1 c2 context))
             (group-a (append only-a (car assigned)))
             (group-b (append only-b (cadr assigned)))
             (cut-items (append both (caddr assigned)))
             (total (length all-objs))
             (ratio (if (zero? total) 1.0
                      (/ (length cut-items) total))))
        (list (cons 'group-a (sort-strings group-a))
              (cons 'group-b (sort-strings group-b))
              (cons 'cut (sort-strings cut-items))
              (cons 'cut-ratio (exact->inexact ratio)))))))

(define (incomparable-concept-pairs concepts context)
  "Find all pairs of concepts that are lattice-incomparable."
  (let loop ((cs concepts) (acc '()))
    (if (null? cs) acc
      (let inner ((rest (cdr cs)) (acc acc))
        (if (null? rest)
          (loop (cdr cs) acc)
          (let* ((c1 (car cs))
                 (c2 (car rest))
                 (e1 (concept-extent c1))
                 (e2 (concept-extent c2)))
            (if (and (not (set-subset? e1 e2))
                     (not (set-subset? e2 e1)))
              (inner (cdr rest) (cons (cons c1 c2) acc))
              (inner (cdr rest) acc))))))))

(define (best-split-pair pairs context)
  "Select the pair with the largest combined extent and smallest overlap."
  (if (null? pairs) #f
    (let loop ((ps pairs)
               (best #f) (best-score -1))
      (if (null? ps) best
        (let* ((pair (car ps))
               (e1 (concept-extent (car pair)))
               (e2 (concept-extent (cdr pair)))
               (coverage (length (set-union e1 e2)))
               (overlap (length (set-intersect e1 e2)))
               (score (- coverage (* 2 overlap))))
          (if (> score best-score)
            (loop (cdr ps) pair score)
            (loop (cdr ps) best best-score)))))))

(define (assign-remainder objs c1 c2 context)
  "Assign functions not in either concept by attribute affinity.
Returns (a-additions b-additions ambiguous)."
  (let ((i1 (concept-intent c1))
        (i2 (concept-intent c2)))
    (let loop ((os objs) (a '()) (b '()) (amb '()))
      (if (null? os) (list a b amb)
        (let* ((o (car os))
               (attrs (intent context (list o)))
               (a-overlap (length (set-intersect attrs i1)))
               (b-overlap (length (set-intersect attrs i2))))
          (cond ((> a-overlap b-overlap) (loop (cdr os) (cons o a) b amb))
                ((> b-overlap a-overlap) (loop (cdr os) a (cons o b) amb))
                (else (loop (cdr os) a b (cons o amb)))))))))

(define (set-difference a b)
  "Return elements in sorted list a not in sorted list b."
  (let loop ((xs a) (ys b) (acc '()))
    (cond ((null? xs) (reverse acc))
          ((null? ys) (append (reverse acc) xs))
          ((string<? (car xs) (car ys))
           (loop (cdr xs) ys (cons (car xs) acc)))
          ((string=? (car xs) (car ys))
           (loop (cdr xs) (cdr ys) acc))
          (else (loop xs (cdr ys) acc)))))

(define (sort-strings lst)
  "Sort a list of strings lexicographically."
  (sort lst string<?))
```

Update `split.sld` exports:
```scheme
  (export
    import-signatures
    compute-idf
    filter-noise
    build-package-context
    refine-by-api-surface
    find-split)
```

**Step 2: Write tests**

Add to `goast/split_test.go`:
```go
func TestSplit_FindSplit(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(import (wile goast fca))

		;; Two clear groups: F1,F2 use "A"; F3,F4 use "B"; F5 bridges both.
		(define ctx (context-from-alist
		  '(("F1" "A" "C")
		    ("F2" "A" "C")
		    ("F3" "B" "D")
		    ("F4" "B" "D")
		    ("F5" "A" "B"))))
		(define lat (concept-lattice ctx))
		(define result (find-split ctx lat))
	`)

	c := qt.New(t)

	t.Run("two non-empty groups", func(t *testing.T) {
		result := eval(t, engine, `
			(and (not (null? (cdr (assoc 'group-a result))))
			     (not (null? (cdr (assoc 'group-b result)))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("F5 in cut (bridges both)", func(t *testing.T) {
		result := eval(t, engine, `
			(not (equal? #f (member "F5" (cdr (assoc 'cut result)))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("cut-ratio is 0.2 (1 of 5)", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'cut-ratio result))`)
		c.Assert(result.SchemeString(), qt.Equals, "0.2")
	})

	t.Run("no split when all share same deps", func(t *testing.T) {
		result := eval(t, engine, `
			(define ctx-uniform (context-from-alist
			  '(("F1" "A") ("F2" "A") ("F3" "A"))))
			(define lat-u (concept-lattice ctx-uniform))
			(define result-u (find-split ctx-uniform lat-u))
			(cdr (assoc 'cut-ratio result-u))`)
		c.Assert(result.SchemeString(), qt.Equals, "1.0")
	})
}
```

**Step 3: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestSplit_FindSplit -v`
Expected: PASS

**Step 4: Commit**

```
feat(split): min-cut partition via concept lattice
```

---

### Task 8: `verify-acyclic` — cycle detection

Verify a proposed split doesn't create Go import cycles.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/split.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/split.sld`
- Modify: `goast/split_test.go`

**Step 1: Add implementation**

In `split.scm`:
```scheme
(define (verify-acyclic group-a group-b func-refs)
  "Check that a proposed split doesn't create Go import cycles.
Group A functions may depend on group B's package or vice versa, but not both.
Uses go-func-refs data to detect cross-references within the original package.

Parameters:
  group-a    : list — function names in group A
  group-b    : list — function names in group B
  func-refs  : list — raw output from (go-func-refs ...)
Returns: alist — (acyclic . #t/#f) (a-refs-b . count) (b-refs-a . count)
Category: goast-split

Examples:
  (verify-acyclic '(\"F1\" \"F2\") '(\"F3\" \"F4\") refs)
  ;; => ((acyclic . #t) (a-refs-b . 2) (b-refs-a . 0))

See also: `find-split', `recommend-split'."
  (let* ((pkg (if (null? func-refs) ""
               (nf (car func-refs) 'pkg)))
         (a-refs-b (count-internal-refs group-a group-b func-refs pkg))
         (b-refs-a (count-internal-refs group-b group-a func-refs pkg)))
    (list (cons 'acyclic (or (zero? a-refs-b) (zero? b-refs-a)))
          (cons 'a-refs-b a-refs-b)
          (cons 'b-refs-a b-refs-a))))

(define (count-internal-refs from-group to-group func-refs pkg)
  "Count how many functions in from-group call functions in to-group.
Uses identifier references within the same package."
  (let ((to-names to-group))
    (let loop ((frs func-refs) (count 0))
      (if (null? frs) count
        (let* ((fr (car frs))
               (name (nf fr 'name))
               (refs (let ((r (nf fr 'refs))) (if r r '()))))
          (if (not (member name from-group))
            (loop (cdr frs) count)
            (let ((has-internal
                    (let check ((rs refs))
                      (cond ((null? rs) #f)
                            ((equal? (nf (car rs) 'pkg) pkg)
                             (let ((objs (nf (car rs) 'objects)))
                               (if (and objs (any-member? objs to-names))
                                 #t
                                 (check (cdr rs)))))
                            (else (check (cdr rs)))))))
              (loop (cdr frs) (if has-internal (+ count 1) count)))))))))

(define (any-member? items lst)
  "True if any element of items is a member of lst."
  (let loop ((is items))
    (cond ((null? is) #f)
          ((member (car is) lst) #t)
          (else (loop (cdr is))))))
```

Update `split.sld` exports:
```scheme
  (export
    import-signatures
    compute-idf
    filter-noise
    build-package-context
    refine-by-api-surface
    find-split
    verify-acyclic)
```

**Step 2: Write tests**

Add to `goast/split_test.go`:
```go
func TestSplit_VerifyAcyclic(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(import (wile goast utils))

		;; Simulate: F1 calls F3 (a->b), but F3 doesn't call F1 (no b->a).
		(define refs
		  '((func-ref (name . "F1") (pkg . "my/pkg")
		              (refs . ((ref (pkg . "my/pkg")
		                            (objects . ("F3"))))))
		    (func-ref (name . "F2") (pkg . "my/pkg")
		              (refs . ()))
		    (func-ref (name . "F3") (pkg . "my/pkg")
		              (refs . ()))
		    (func-ref (name . "F4") (pkg . "my/pkg")
		              (refs . ()))))
	`)

	c := qt.New(t)

	t.Run("one-way dependency is acyclic", func(t *testing.T) {
		result := eval(t, engine, `
			(define v (verify-acyclic '("F1" "F2") '("F3" "F4") refs))
			(cdr (assoc 'acyclic v))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("a-refs-b count is 1", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'a-refs-b v))`)
		c.Assert(result.SchemeString(), qt.Equals, "1")
	})

	t.Run("bidirectional dependency is cyclic", func(t *testing.T) {
		result := eval(t, engine, `
			;; F3 also calls F1 -> cycle
			(define refs-cycle
			  '((func-ref (name . "F1") (pkg . "p")
			              (refs . ((ref (pkg . "p") (objects . ("F3"))))))
			    (func-ref (name . "F3") (pkg . "p")
			              (refs . ((ref (pkg . "p") (objects . ("F1"))))))))
			(define vc (verify-acyclic '("F1") '("F3") refs-cycle))
			(cdr (assoc 'acyclic vc))`)
		c.Assert(result.SchemeString(), qt.Equals, "#f")
	})
}
```

**Step 3: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestSplit_VerifyAcyclic -v`
Expected: PASS

**Step 4: Commit**

```
feat(split): import cycle verification for proposed splits
```

---

### Task 9: `recommend-split` — top-level entry point

Wire everything together: compute IDF, filter, build context, find split, verify acyclic, produce report.

**Files:**
- Modify: `cmd/wile-goast/lib/wile/goast/split.scm`
- Modify: `cmd/wile-goast/lib/wile/goast/split.sld`
- Modify: `goast/split_test.go`

**Step 1: Add implementation**

In `split.scm`:
```scheme
(define (recommend-split func-refs . opts)
  "Analyze a package's functions and recommend a two-way split.
Computes IDF-weighted import signatures, builds FCA concept lattice,
finds min-cut partition, and verifies acyclicity.

Parameters:
  func-refs : list — output from (go-func-refs ...)
  opts      : optional — keyword options:
              'idf-threshold N (default 0.36)
              'refine         (use API-surface refinement)
Returns: alist with keys:
  functions  — total function count
  high-idf   — high-IDF packages with scores
  groups     — (find-split) result
  acyclic    — (verify-acyclic) result
  confidence — HIGH / MEDIUM / LOW / NONE
Category: goast-split

Examples:
  (define report (recommend-split (go-func-refs \"my/pkg\")))
  (assoc 'confidence report)

See also: `import-signatures', `find-split', `verify-acyclic'."
  (let* ((threshold (opt-ref opts 'idf-threshold 0.36))
         (refine? (memq 'refine opts))
         (sigs (import-signatures func-refs))
         (idf (compute-idf sigs))
         (filtered (filter-noise sigs idf threshold))
         (context (if refine?
                    (refine-by-api-surface func-refs filtered)
                    (build-package-context filtered)))
         (lattice (concept-lattice context))
         (groups (find-split context lattice))
         (group-a (cdr (assoc 'group-a groups)))
         (group-b (cdr (assoc 'group-b groups)))
         (acyclic-info (verify-acyclic group-a group-b func-refs))
         (high-idf-pkgs (filter (lambda (e) (>= (cdr e) threshold))
                                idf))
         (confidence (compute-confidence groups acyclic-info)))
    (list (cons 'functions (length func-refs))
          (cons 'high-idf high-idf-pkgs)
          (cons 'groups groups)
          (cons 'acyclic acyclic-info)
          (cons 'confidence confidence))))

(define (opt-ref opts key default)
  "Look up a keyword option: (opt-ref '(key1 val1 key2 val2) 'key1 #f) => val1."
  (let loop ((os opts))
    (cond ((null? os) default)
          ((and (not (null? (cdr os)))
                (eq? (car os) key))
           (cadr os))
          (else (loop (cdr os))))))

(define (compute-confidence groups acyclic-info)
  "Compute confidence level from split quality metrics."
  (let* ((cut-ratio (cdr (assoc 'cut-ratio groups)))
         (group-a (cdr (assoc 'group-a groups)))
         (group-b (cdr (assoc 'group-b groups)))
         (acyclic? (cdr (assoc 'acyclic acyclic-info)))
         (has-groups? (and (not (null? group-a))
                           (not (null? group-b)))))
    (cond ((not has-groups?) 'NONE)
          ((and acyclic? (<= cut-ratio 0.15)) 'HIGH)
          ((and acyclic? (<= cut-ratio 0.30)) 'MEDIUM)
          (else 'LOW))))
```

Update `split.sld` exports:
```scheme
  (export
    import-signatures
    compute-idf
    filter-noise
    build-package-context
    refine-by-api-surface
    find-split
    verify-acyclic
    recommend-split)
```

**Step 2: Write unit test with synthetic data**

Add to `goast/split_test.go`:
```go
func TestSplit_RecommendSplit_Synthetic(t *testing.T) {
	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))

		;; Two clusters with distinct deps, one bridge function.
		(define refs
		  '((func-ref (name . "Read")  (pkg . "p")
		              (refs . ((ref (pkg . "io") (objects . ("Reader"))))))
		    (func-ref (name . "Write") (pkg . "p")
		              (refs . ((ref (pkg . "io") (objects . ("Writer"))))))
		    (func-ref (name . "Parse") (pkg . "p")
		              (refs . ((ref (pkg . "go/ast") (objects . ("File"))))))
		    (func-ref (name . "Check") (pkg . "p")
		              (refs . ((ref (pkg . "go/ast") (objects . ("Inspect"))))))
		    (func-ref (name . "Bridge") (pkg . "p")
		              (refs . ((ref (pkg . "io") (objects . ("Reader")))
		                       (ref (pkg . "go/ast") (objects . ("File"))))))))
		(define report (recommend-split refs))
	`)

	c := qt.New(t)

	t.Run("has confidence", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'confidence report))`)
		c.Assert(result.SchemeString(), qt.Not(qt.Equals), "NONE")
	})

	t.Run("function count is 5", func(t *testing.T) {
		result := eval(t, engine, `(cdr (assoc 'functions report))`)
		c.Assert(result.SchemeString(), qt.Equals, "5")
	})

	t.Run("two non-empty groups", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((gs (cdr (assoc 'groups report))))
			  (and (not (null? (cdr (assoc 'group-a gs))))
			       (not (null? (cdr (assoc 'group-b gs))))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 3: Run tests**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestSplit_RecommendSplit -v`
Expected: PASS

**Step 4: Commit**

```
feat(split): top-level recommend-split entry point
```

---

### Task 10: Integration test — self-analysis on `goast/`

Run the full pipeline on the `goast/` package itself. Verify the output is structurally valid and produces real dependency data.

**Files:**
- Modify: `goast/split_test.go`

**Step 1: Write integration test**

```go
func TestSplit_Integration_Goast(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	engine := newBeliefEngine(t)

	eval(t, engine, `
		(import (wile goast split))
		(import (wile goast utils))

		(define refs (go-func-refs
		  "github.com/aalpar/wile-goast/goast"))
		(define report (recommend-split refs))
	`)

	c := qt.New(t)

	t.Run("many functions", func(t *testing.T) {
		result := eval(t, engine, `(> (cdr (assoc 'functions report)) 20)`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("has high-IDF packages", func(t *testing.T) {
		result := eval(t, engine, `
			(not (null? (cdr (assoc 'high-idf report))))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("confidence is a known level", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((c (cdr (assoc 'confidence report))))
			  (or (eq? c 'HIGH) (eq? c 'MEDIUM)
			      (eq? c 'LOW) (eq? c 'NONE)))`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})

	t.Run("acyclic field present", func(t *testing.T) {
		result := eval(t, engine, `
			(let ((a (cdr (assoc 'acyclic report))))
			  (assoc 'acyclic a))`)
		c.Assert(result.SchemeString(), qt.Not(qt.Equals), "#f")
	})

	t.Run("refined split also works", func(t *testing.T) {
		result := eval(t, engine, `
			(define report-r (recommend-split refs 'refine))
			(> (cdr (assoc 'functions report-r)) 20)`)
		c.Assert(result.SchemeString(), qt.Equals, "#t")
	})
}
```

**Step 2: Run test**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && go test ./goast/ -run TestSplit_Integration_Goast -v -timeout 120s`
Expected: PASS

**Step 3: Commit**

```
test(split): integration test — self-analysis on goast/ package
```

---

### Task 11: Update docs and CLAUDE.md

Add the new primitive and library to documentation.

**Files:**
- Modify: `CLAUDE.md` — add `go-func-refs` to goast primitives table, add `(wile goast split)` section
- Modify: `docs/PRIMITIVES.md` — add `go-func-refs` reference
- Modify: `plans/CLAUDE.md` — mark package-splitting design as "Phase 1-2 complete"

**Step 1: Update CLAUDE.md**

Add `go-func-refs` to the goast primitives table:
```
| `go-func-refs` | Per-function external reference profiles |
```

Add new section:
```markdown
## Package Splitting — `(wile goast split)`

Import signature analysis for Go package decomposition. Discovers natural
package boundaries using IDF-weighted FCA on per-function dependency profiles.

| Export | Description |
|--------|-------------|
| `import-signatures` | Extract per-function package dependency sets |
| `compute-idf` | IDF weights for dependency informativeness |
| `filter-noise` | Remove ubiquitous (low-IDF) dependencies |
| `build-package-context` | FCA context at package granularity |
| `refine-by-api-surface` | FCA context at (package, object) granularity |
| `find-split` | Min-cut two-way partition via concept lattice |
| `verify-acyclic` | Check proposed split for Go import cycles |
| `recommend-split` | Top-level: IDF + FCA + min-cut + cycle check |
```

Add to Key Files table:
```
| `goast/prim_funcrefs.go` | Per-function external reference extraction (`go-func-refs`) |
| `cmd/wile-goast/lib/wile/goast/split.scm` | Package splitting analysis library (embedded in binary) |
```

**Step 2: Update docs/PRIMITIVES.md**

Add `go-func-refs` entry with signature, parameters, return format, and examples.

**Step 3: Update plans/CLAUDE.md**

Change status of `2026-04-12-package-splitting-design.md` from "Proposed" to "Phase 1-2 complete".
Add `2026-04-13-package-splitting-impl.md` entry.

**Step 4: Run full CI**

Run: `cd /Users/aalpar/projects/wile-workspace/wile-goast && make ci`
Expected: All tests pass, coverage >= 80%.

**Step 5: Commit**

```
docs: add go-func-refs and (wile goast split) to documentation
```

---

### Task 12: Version bump

**Files:**
- Modify: `VERSION`

**Step 1: Bump version**

The current version is v0.5.68. Bump to v0.5.69 (or whatever the current version is at time of execution + 1).

**Step 2: Commit**

```
chore: bump version to v0.5.69
```
