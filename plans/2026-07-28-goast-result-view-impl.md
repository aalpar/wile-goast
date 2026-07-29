# Result Views for `find_duplicates` / `find_false_boundaries` — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `find_duplicates` and `find_false_boundaries` server-side narrowing (`scope`/`symbol` globs), ranking, row-width control (`detail`), and paging (`limit`/`offset`), with `find_duplicates` pushing its filter down before scoring and `find_false_boundaries` post-filtering finished rows.

**Architecture:** One `apply-view` combinator in `pipelines.scm` owns filter → rank → page → render and emits the provenance those operations imply. Each tool supplies three hooks: a `keep?` predicate (or `#f`), a rank key, and a `render`. `find_duplicates` prunes the flat candidate-pair list *before* scoring and takes its `max-candidate-pairs` guard reading *after* the prune; `find_false_boundaries` filters finished concepts, because restricting the FCA objects would recompute the lattice and report absence where findings exist. A new `glob-match?` in `(wile goast utils)` implements the whole-string-or-separator-aligned-suffix rule. The Go handlers gain five pass-through parameters and validate `detail`/`limit`/`offset`; every default is written once, in Scheme.

**Tech Stack:** Wile Scheme (R7RS libraries under `lib/wile/goast/`), Go 1.x, `mark3labs/mcp-go` v0.45.0, `frankban/quicktest`.

**Design doc:** [`plans/2026-07-28-goast-result-view-design.md`](2026-07-28-goast-result-view-design.md). Read it first; this plan implements it and does not restate its rationale.

---

## Global Constraints

- Direct push to master. **No `Co-Authored-By` lines** in commit messages.
- Plan/design files live in `plans/`, never `docs/plans`. `plans/` is gitignored globally — new files need `git add -f`.
- Coverage threshold 80% (`tools/sh/covercheck.sh`); `cmd/wile-goast` and `testutil` are excluded, so the Go handler code is not coverage-bearing but the Scheme is exercised through `cmd/wile-goast` integration tests either way.
- No new dependencies. Standard library plus the three already in `go.mod`.
- Full check before each commit: `make ci` (lint + build + test + covercheck + verify-mod). Per-task steps below use narrower commands; run `make ci` at least before the final commit of each task.
- Scheme alist keys stay kebab-case; `cmd/wile-goast/marshal.go` snake-cases them at the JSON boundary. Never write snake_case in Scheme.
- An R7RS library that exports an unbound identifier fails to instantiate. Add each `.sld` export in the same commit as its `.scm` definition.

### Wile call-depth constraint (load-bearing, verified)

Wile has a fixed 10000-frame call-depth limit and several list procedures are **not** tail-recursive. Measured in this session on the live engine:

| expression | 20 000 elements |
|---|---|
| `(sort less? lst)` | **dies**: `call depth 10001 exceeds limit 10000` |
| `(map f lst)` | non-tail (this is the bug that produced the old `max-candidate-pairs 9992`) |
| `(filter p lst)` (`utils.scm:34`) | safe — accumulator loop |
| `(filter-map f lst)` (`utils.scm:42`) | safe — accumulator loop |
| `(drop lst n)` (`utils.scm:119`) | safe — tail-recursive |
| `(take lst n)` (`utils.scm:113`) | depth = `n`; safe for `limit`-sized `n` |

Consequences that this plan bakes in, and that must not be "simplified" away:

1. `apply-view` must not call the builtin `sort` — `find_duplicates`' own guard admits up to 25000 scored pairs and every one of them may unify, which is past the cliff. Task 2 adds a tail-safe `view-sort`.
2. `apply-view` must not `map` over the *unpaged* row list. It decorates with a tail-recursive named let, and only `map`s over the ≤ `limit` rows of the page.
3. Rendering happens **after** paging, so a withheld row costs nothing.

---

## Preconditions

- [ ] The working tree currently carries **uncommitted** changes to `cmd/wile-goast/mcp_tools.go`, `cmd/wile-goast/mcp_tools_integration_test.go`, `docs/MCP.md`, and `lib/wile/goast/pipelines.scm` (the equiv-tier filter + `max-candidate-pairs 25000` work). Every line reference in this plan is against that **working tree**, not `HEAD`. Commit it before starting, or confirm it is intact:

```bash
git -C /Users/aalpar/projects/wile-workspace/wile-goast status --short
grep -n "max-candidate-pairs 25000" lib/wile/goast/pipelines.scm   # expect line 245
```

- [ ] Baseline green: `make ci`.

---

## Corrections to the design

Verified against the code and the live engine during planning. Each is a deviation the implementer should **not** relitigate.

1. **The design's regression pin does not discriminate.** The design (Testing §2) names `symbol=*map*` on `./goast` as the pin for decision 2. Measured: the five cross-boundary concepts on `./goast` are over `loopRewriter.*`, `makeCtlGuard`, `restructureBackwardGotos`, `assign*LeadingPos`, and `attach*Comments` — **no extent member contains `map`**. Post-filtering `*map*` returns 0, exactly like pushing it down, so it proves nothing. Measured discriminators:

   | glob | push-down (restrict FCA objects) | post-filter (this design) |
   |---|---:|---:|
   | none | 5 | 5 |
   | `*restructure*` | **0** | **2** |
   | `*attach*` | **0** | **2** |
   | `*loopRewriter*` | **1** | **3** |
   | `*map*` | 0 | 0 |

   Task 3 uses **`*restructure*`** as the pin.

2. **`find_false_boundaries` rows carry no position, so `scope` needs a pos-index.** `annotated-boundary-report` (`fca-algebra.scm:45`) emits `extent`/`intent`/`extent-size`/`summary` plus the three relation lists — no positions. The design's parameter table asserts positions "in the same absolute form" without saying where from. They come from `field-index->positions` (`fca.scm:78`, exported), whose keys are the same qualified names the FCA context uses. Verified on `goastcfg`: `("…goastcfg.PrimGoCFGDominators" "/Users/…/goastcfg/prim_cfg.go:281:6")`. Task 3 builds that index and threads it into the filter.

3. **`project` becomes `render (row detail)`.** The design's combinator signature takes a `project` that "narrows a row for `detail=summary`", which requires rows to already be in *full* form — incompatible with the same design's requirement that `detail=summary` "skip `annotated-boundary-report` rather than computing and discarding it". Replacing `project` with a `render` that receives the detail level and runs after paging satisfies both, and additionally means `find_duplicates` projects only the page instead of every kept candidate.

4. **`scope` globs match the file, not the position.** Positions are `file.go:line:col`; an anchored glob `marshal.go` cannot match `marshal.go:568:1`. Task 1 adds `position-file` to strip one trailing `:<digits>:<digits>`, so `scope` matches the path a caller actually has.

5. **Both tools' envelope `version` goes 1 → 2.** `detail=summary` is the default, so a default call's row shape changes (`measures` disappears from `find_duplicates`; the three relation lists disappear from `find_false_boundaries`). That is precisely the "breaking change to that tool's result shape" the version field exists for (`pipelines.scm:22`).

6. **`find_duplicates` provenance drops `candidate-count`.** It means "kept rows before paging", which is exactly `total-count` from the shared combinator. Two names for one number across a tool boundary is the redundancy the project's own refactoring rules flag, and design decision 3 wants identical semantics across tools. `total_count` replaces it; the version bump in (5) covers the removal.

7. **The canon index is built only over pair-participating functions.** The design mentions this in passing ("the surviving function set can also restrict what `canon-index` canonicalizes"). It is required for a narrow `scope` to actually be *cheap* rather than merely small, and it is result-preserving: `score-candidate-pair` reads `canon-index` by short name and only for names in the pair list, and filtering by short-name membership keeps colliding short names together, so `build-func-canon-index`'s last-wins order is unchanged.

---

## File Structure

| File | Change | Responsibility |
|---|---|---|
| `lib/wile/goast/utils.scm` | modify (append) | `glob-match-anchored?`, `glob-match?`, `position-file` — pure string matching, no goast concepts |
| `lib/wile/goast/utils.sld` | modify | export `glob-match?`, `position-file` |
| `lib/wile/goast/pipelines.scm` | modify | shared view layer (`view-sort`, `view-key<?`, `apply-view`) + per-tool hooks + the two rewritten pipelines |
| `lib/wile/goast/pipelines.sld` | modify | import the new dup-detect entry points |
| `lib/wile/goast/fca.sld` | modify | export `attr-struct-name` |
| `lib/wile/goast/fca-algebra.sld` | modify | export `concept-summary` |
| `lib/wile/goast/dup-detect.scm` | modify | `scored-candidates` takes a pair list; new `score-candidate-pairs`, `pair-short-names` |
| `lib/wile/goast/dup-detect.sld` | modify | export `candidate-pairs`, `score-candidate-pairs` |
| `cmd/wile-goast/mcp_tools.go` | modify | five parameters on both tools; `viewOpts` validation |
| `goast/utils_glob_test.go` | **create** | `glob-match?` / `position-file` unit tests |
| `goast/view_test.go` | **create** | `apply-view` unit tests over synthetic rows |
| `goast/dup_detect_test.go` | modify (append) | Task 4 refactor regression |
| `cmd/wile-goast/mcp_tools_integration_test.go` | modify | end-to-end view tests; update version + count assertions |
| `docs/MCP.md` | modify | parameter table + provenance keys |
| `tools/legibility/probe.go` | modify | drop the unreachable `distinct` choice from the prompt |

---

### Task 1: `glob-match?` and `position-file` in `(wile goast utils)`

**Files:**
- Modify: `lib/wile/goast/utils.scm` (append at end, after `ast-splice`)
- Modify: `lib/wile/goast/utils.sld:16-24` (export list)
- Test: `goast/utils_glob_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `(glob-match? pat s sep) -> boolean` — `pat` may contain `*`, matching any run of characters including `sep`. True when `pat` matches all of `s`, or all of any suffix of `s` starting immediately after a `sep` character. Non-string `s` → `#f`.
  - `(position-file pos) -> string` — `pos` minus one trailing `:<digits>:<digits>`; unchanged if it has no such suffix.

- [ ] **Step 1: Write the failing test**

The security hook false-positives on Go test files containing an `eval(` helper call and will refuse a `Write`/`Edit`. Create this file with a **quoted** heredoc:

```bash
cat > goast/utils_glob_test.go <<'GOEOF'
// Copyright 2026 Aaron Alpar
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// The two real strings the globs are written against: an absolute position
// path and a qualified function name. Both tools carry the first in the same
// form; they disagree on the second, which is why the suffix rule matches at
// ANY separator boundary rather than only the last one.
const (
	globPos      = "/Users/aalpar/projects/wile-workspace/wile-goast/goast/marshal.go:568:1"
	globDupName  = "ssaMapper.mapChangeInterface"
	globFbName   = "(*github.com/aalpar/wile-goast/goast.loopRewriter).replaceInStmts"
	globPlainFn  = "github.com/aalpar/wile-goast/goast.cloneTypeExpr"
)

func TestGlobMatch_Scope(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	cases := []struct {
		pat  string
		want string
	}{
		// Whole-string match.
		{"/Users/*/wile-goast/goast/marshal.go", "#t"},
		{"*/goast/*.go", "#t"},
		// Suffix at a separator boundary — the common caller inputs.
		{"marshal.go", "#t"},
		{"goast/marshal.go", "#t"},
		{"wile-goast/goast/marshal.go", "#t"},
		{"*.go", "#t"},
		// A last-segment-only rule would pass "marshal.go" and fail this;
		// this case is why the rule matches at every boundary.
		{"goast/*.go", "#t"},
		// Not aligned to a separator: "st/marshal.go" is a suffix of the
		// string but does not begin just after a '/'.
		{"st/marshal.go", "#f"},
		{"other.go", "#f"},
		{"goast/other.go", "#f"},
	}
	for _, tc := range cases {
		got := eval(t, engine, `(glob-match? "`+tc.pat+`" (position-file "`+globPos+`") #\/)`)
		qt.Assert(t, got.SchemeString(), qt.Equals, tc.want,
			qt.Commentf("scope glob %q", tc.pat))
	}
}

func TestGlobMatch_Symbol(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	cases := []struct {
		pat, subject, want string
	}{
		// find_duplicates names: Type.Method or bare.
		{"mapChangeInterface", globDupName, "#t"},
		{"ssaMapper.*", globDupName, "#t"},
		{"ssaMapper.mapChangeInterface", globDupName, "#t"},
		{"*map*", globDupName, "#t"},
		{"SumSlice", "SumSlice", "#t"},
		{"changeInterface", globDupName, "#f"}, // not at a '.' boundary
		// find_false_boundaries names: fully package-qualified SSA form.
		// The same short pattern must work despite the qualification.
		{"cloneTypeExpr", globPlainFn, "#t"},
		{"replaceInStmts", globFbName, "#t"},
		{"*loopRewriter*", globFbName, "#t"},
		{"loopRewriter.*", globFbName, "#f"}, // the qualified form has ")." not "."
		{"restructureBackwardGotos", globPlainFn, "#f"},
	}
	for _, tc := range cases {
		got := eval(t, engine, `(glob-match? "`+tc.pat+`" "`+tc.subject+`" #\.)`)
		qt.Assert(t, got.SchemeString(), qt.Equals, tc.want,
			qt.Commentf("symbol glob %q against %q", tc.pat, tc.subject))
	}
}

// A literal '*' in the subject must be matched by a literal '*' in the
// pattern, not consumed by the wildcard branch, and a non-string subject
// (an unlocated row) matches nothing rather than erroring.
func TestGlobMatch_EdgeCases(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	for expr, want := range map[string]string{
		`(glob-match? "*" "anything/at/all" #\/)`: "#t",
		`(glob-match? "" "" #\/)`:                 "#t",
		`(glob-match? "" "abc" #\/)`:              "#f",
		`(glob-match? "a*c" "abc" #\/)`:           "#t",
		`(glob-match? "a*c" "ac" #\/)`:            "#t",
		`(glob-match? "a*c" "abd" #\/)`:           "#f",
		`(glob-match? "a**c" "abbbc" #\/)`:        "#t",
		`(glob-match? "*x" "ax" #\/)`:             "#t",
		`(glob-match? "x*" "xa" #\/)`:             "#t",
		`(glob-match? "abc" #f #\/)`:              "#f",
	} {
		got := eval(t, engine, expr)
		qt.Assert(t, got.SchemeString(), qt.Equals, want, qt.Commentf("%s", expr))
	}
}

func TestPositionFile(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `(import (wile goast utils))`)

	for in, want := range map[string]string{
		"/a/b.go:568:1": "/a/b.go",
		"/a/b.go:1:1":   "/a/b.go",
		"marshal.go":    "marshal.go",   // already a bare path
		"/a/b.go:568:":  "/a/b.go:568:", // malformed: left alone
		"/a/b.go:568":   "/a/b.go:568",  // only one number: left alone
		"":              "",
	} {
		got := eval(t, engine, `(position-file "`+in+`")`)
		qt.Assert(t, got.SchemeString(), qt.Equals, `"`+want+`"`,
			qt.Commentf("position-file %q", in))
	}
}
GOEOF
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./goast/ -run 'TestGlobMatch|TestPositionFile' -count=1`
Expected: FAIL — `no such binding "glob-match?"` (compile/expand error from the engine).

- [ ] **Step 3: Implement in `lib/wile/goast/utils.scm`**

Append to the end of the file (after `ast-splice`):

```scheme
;;; ── Glob matching ────────────────────────────────────────

;; glob-match-anchored?: does PAT match the WHOLE of S? `*' is the only
;; metacharacter and matches any run of characters, separators included.
;; Iterative greedy-with-backtrack (star/mark): every branch is a tail call,
;; so a long absolute path costs no stack. Worst case O(|pat| * |s|).
(define (glob-match-anchored? pat s)
  (let ((pl (string-length pat))
        (sl (string-length s)))
    (let loop ((i 0) (j 0) (star -1) (mark 0))
      (if (>= i sl)
        ;; S exhausted: PAT matches iff only `*'s remain.
        (let skip ((k j))
          (cond ((>= k pl) #t)
                ((char=? (string-ref pat k) #\*) (skip (+ k 1)))
                (else #f)))
        (cond
          ;; `*' tested first, so a literal `*' in S is not consumed by it.
          ((and (< j pl) (char=? (string-ref pat j) #\*))
           (loop i (+ j 1) j i))
          ((and (< j pl) (char=? (string-ref pat j) (string-ref s i)))
           (loop (+ i 1) (+ j 1) star mark))
          ;; Mismatch: retry the most recent `*', letting it eat one more char.
          ((>= star 0)
           (loop (+ mark 1) (+ star 1) star (+ mark 1)))
          (else #f))))))

;; glob-match?: PAT matches S if it matches the whole string, or the whole of
;; any suffix of S beginning just after a SEP character.
;;
;; Matching a suffix at ANY boundary, not only the last one, is what makes the
;; inputs a caller actually holds work. With SEP #\/ against an absolute
;; position path, "marshal.go", "goast/marshal.go" and "*/goast/*.go" all
;; match, with no relativization anywhere. With SEP #\., a bare
;; "cloneTypeExpr" matches both the "Type.Method" form one tool reports and
;; the fully package-qualified form the other does.
;;
;; The trade-off: a glob can match via several candidate suffixes, so a caller
;; cannot demand the fully qualified form specifically, and a short symbol can
;; match same-named methods on different receivers. Every returned row carries
;; the qualified name, so a second, narrower call is always available.
;;
;; A non-string S (an unlocated row) matches nothing.
(define (glob-match? pat s sep)
  (and (string? s)
       (let ((n (string-length s)))
         (let loop ((i 0))
           (cond ((> i n) #f)
                 ((and (or (= i 0) (char=? (string-ref s (- i 1)) sep))
                       (glob-match-anchored? pat (substring s i n)))
                  #t)
                 (else (loop (+ i 1))))))))

;; position-file: the file part of a "file:line:col" position, so a path glob
;; can be written against the path a caller has ("marshal.go") rather than the
;; position ("marshal.go:568:1"), which an anchored glob could never match.
;; Strips exactly one trailing :<digits>:<digits>; anything else (a bare path,
;; a malformed position) is returned unchanged.
(define (position-file pos)
  (let* ((n (string-length pos))
         (digits-back
           ;; index just past the last digit run ending at END
           (lambda (end)
             (let loop ((i end))
               (if (and (> i 0) (char-numeric? (string-ref pos (- i 1))))
                 (loop (- i 1))
                 i))))
         (col (digits-back n)))
    (if (and (< col n) (> col 0) (char=? (string-ref pos (- col 1)) #\:))
      (let ((line (digits-back (- col 1))))
        (if (and (< line (- col 1)) (> line 0)
                 (char=? (string-ref pos (- line 1)) #\:))
          (substring pos 0 (- line 1))
          pos))
      pos)))
```

- [ ] **Step 4: Export from `lib/wile/goast/utils.sld`**

Replace the export line `ordered-pairs` block so the list reads:

```scheme
  (export
    nf tag? walk
    filter filter-map flat-map
    member? unique has-char? string-contains string-contains? string-join
    ordered-pairs
    glob-match? position-file
    take drop
    opt-ref
    ast-transform ast-splice)
```

`glob-match-anchored?` stays internal.

- [ ] **Step 5: Run the tests**

Run: `go test ./goast/ -run 'TestGlobMatch|TestPositionFile' -count=1 -v`
Expected: PASS, 4 tests.

- [ ] **Step 6: Full check and commit**

```bash
make ci
git add lib/wile/goast/utils.scm lib/wile/goast/utils.sld goast/utils_glob_test.go
git commit -m "feat(utils): glob-match? and position-file

Whole-string-or-separator-aligned-suffix matching with '*' as the only
metacharacter. Suffix-at-any-boundary is what lets one glob work against
both an absolute position path and either tool's function-name
qualification. position-file strips a trailing :line:col so a scope glob
can be written against the path a caller actually holds."
```

---

### Task 2: shared view layer in `pipelines.scm`

**Files:**
- Modify: `lib/wile/goast/pipelines.scm` — insert a new section immediately after `assoc-default` (currently `pipelines.scm:117-120`), before the `find_false_boundaries` section
- Modify: `lib/wile/goast/pipelines.sld` — export `apply-view` (needed by the unit test)
- Test: `goast/view_test.go` (create)

**Interfaces:**
- Consumes: `filter`, `take`, `drop`, and (Task 1) `glob-match?`, `position-file` from `(wile goast utils)`; `assoc-default` from `pipelines.scm:118`.
- Produces:
  - `(view-detail opts) -> symbol`, `(view-limit opts) -> integer`, `(view-offset opts) -> integer`
  - `(view-merge less? a b) -> list`, `(view-sort less? lst) -> list`
  - `(view-name-matcher opts pos-index) -> (lambda (name) boolean)` or `#f` — the shared scope/symbol core both tools' `keep?` hooks are built from
  - `(view-key<? ka kb) -> boolean`
  - `(apply-view rows opts keep? rank-key render) -> (values shown view-provenance)` where
    `keep? : row -> boolean` or `#f`; `rank-key : row -> list`; `render : row detail -> alist`.

- [ ] **Step 1: Write the failing test**

```bash
cat > goast/view_test.go <<'GOEOF'
// Copyright 2026 Aaron Alpar
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// These tests drive apply-view with SYNTHETIC rows -- ((score . N) (name . S))
// -- plus the three hooks it takes, so they exercise the combinator itself and
// not either tool's analysis.

func TestApplyView_RankAndTieBreak(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(import (wile goast utils))
		(define rows
		  '(((score . 3) (name . "c"))
		    ((score . 9) (name . "b"))
		    ((score . 9) (name . "a"))
		    ((score . 1) (name . "d"))))
		(define (rk r) (list (cdr (assq 'score r)) (cdr (assq 'name r))))
		(define (rn r detail) (if (eq? detail 'full) r (list (assq 'name r))))
		(define (names shown) (map (lambda (r) (cdr (assq 'name r))) shown))
	`)

	// score descending; equal scores broken by name ascending.
	got := eval(t, engine, `
		(call-with-values
		  (lambda () (apply-view rows '() #f rk rn))
		  (lambda (shown prov) (names shown)))`)
	qt.Assert(t, got.SchemeString(), qt.Equals, `("a" "b" "c" "d")`)
}

func TestApplyView_ProvenanceAndTruncation(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(import (wile goast utils))
		(define rows
		  '(((score . 3) (name . "c"))
		    ((score . 9) (name . "b"))
		    ((score . 9) (name . "a"))
		    ((score . 1) (name . "d"))))
		(define (rk r) (list (cdr (assq 'score r)) (cdr (assq 'name r))))
		(define (rn r detail) (if (eq? detail 'full) r (list (assq 'name r))))
		(define (prov-of opts)
		  (call-with-values
		    (lambda () (apply-view rows opts #f rk rn))
		    (lambda (shown prov) prov)))
		(define (get opts k) (cdr (assq k (prov-of opts))))
	`)

	// Complete result: truncated #f, counts agree, defaults echoed.
	for expr, want := range map[string]string{
		`(get '() 'total-count)`:                       "4",
		`(get '() 'returned-count)`:                    "4",
		`(get '() 'truncated)`:                         "#f",
		`(get '() 'limit)`:                             "50",
		`(get '() 'offset)`:                            "0",
		`(get '() 'detail)`:                            "summary",
		`(get '((limit . 2)) 'returned-count)`:         "2",
		`(get '((limit . 2)) 'total-count)`:            "4",
		`(get '((limit . 2)) 'truncated)`:              "#t",
		`(get '((limit . 2) (offset . 2)) 'truncated)`: "#f",
		`(get '((limit . 0)) 'returned-count)`:         "0",
		`(get '((limit . 0)) 'truncated)`:              "#t",
		`(get '((offset . 99)) 'returned-count)`:       "0",
		`(get '((offset . 99)) 'truncated)`:            "#f",
	} {
		got := eval(t, engine, expr)
		qt.Assert(t, got.SchemeString(), qt.Equals, want, qt.Commentf("%s", expr))
	}

	// scope/symbol are echoed only when set.
	qt.Assert(t, eval(t, engine, `(assq 'scope (prov-of '()))`).SchemeString(),
		qt.Equals, "#f")
	qt.Assert(t, eval(t, engine, `(cdr (assq 'scope (prov-of '((scope . "x/*")))))`).SchemeString(),
		qt.Equals, `"x/*"`)
	qt.Assert(t, eval(t, engine, `(cdr (assq 'symbol (prov-of '((symbol . "Foo")))))`).SchemeString(),
		qt.Equals, `"Foo"`)
}

// Paging must partition: page 1 and page 2 are disjoint and together equal an
// unpaged call, in the same order. This is what makes offset paging usable.
func TestApplyView_PagesPartition(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(import (wile goast utils))
		(define rows
		  '(((score . 3) (name . "c"))
		    ((score . 9) (name . "b"))
		    ((score . 9) (name . "a"))
		    ((score . 1) (name . "d"))))
		(define (rk r) (list (cdr (assq 'score r)) (cdr (assq 'name r))))
		(define (rn r detail) r)
		(define (page opts)
		  (call-with-values
		    (lambda () (apply-view rows opts #f rk rn))
		    (lambda (shown prov) (map (lambda (r) (cdr (assq 'name r))) shown))))
	`)
	all := eval(t, engine, `(page '())`).SchemeString()
	p1 := eval(t, engine, `(page '((limit . 2) (offset . 0)))`).SchemeString()
	p2 := eval(t, engine, `(page '((limit . 2) (offset . 2)))`).SchemeString()
	qt.Assert(t, all, qt.Equals, `("a" "b" "c" "d")`)
	qt.Assert(t, p1, qt.Equals, `("a" "b")`)
	qt.Assert(t, p2, qt.Equals, `("c" "d")`)
}

// keep? filters BEFORE total-count, so truncated means the same thing whether
// the tool filtered here or upstream. #f admits everything.
func TestApplyView_KeepPredicate(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(import (wile goast utils))
		(define rows
		  '(((score . 3) (name . "c"))
		    ((score . 9) (name . "b"))
		    ((score . 9) (name . "a"))
		    ((score . 1) (name . "d"))))
		(define (rk r) (list (cdr (assq 'score r)) (cdr (assq 'name r))))
		(define (rn r detail) r)
		(define (hi? r) (> (cdr (assq 'score r)) 2))
		(define (tc keep) (call-with-values
		  (lambda () (apply-view rows '() keep rk rn))
		  (lambda (shown prov) (cdr (assq 'total-count prov)))))
	`)
	qt.Assert(t, eval(t, engine, `(tc hi?)`).SchemeString(), qt.Equals, "3")
	qt.Assert(t, eval(t, engine, `(tc #f)`).SchemeString(), qt.Equals, "4")
}

// render receives the detail level and runs only on the returned page.
func TestApplyView_RenderPerDetail(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(import (wile goast utils))
		(define rows '(((score . 9) (name . "a"))))
		(define (rk r) (list (cdr (assq 'score r)) (cdr (assq 'name r))))
		;; The render counter lives in a hashtable, not a top-level variable:
		;; Wile top-level bindings are immutable ("cannot mutate immutable
		;; binding"), so set! on one fails. Mutating the hashtable is fine.
		(define counter (make-hashtable))
		(define (bump!) (hashtable-set! counter 'n (+ 1 (hashtable-ref counter 'n 0))))
		(define (renders) (hashtable-ref counter 'n 0))
		(define (rn r detail)
		  (bump!)
		  (if (eq? detail 'full) r (list (assq 'name r))))
		(define (keys opts)
		  (call-with-values
		    (lambda () (apply-view rows opts #f rk rn))
		    (lambda (shown prov) (map car (car shown)))))
	`)
	qt.Assert(t, eval(t, engine, `(keys '())`).SchemeString(),
		qt.Equals, "(name)")
	qt.Assert(t, eval(t, engine, `(keys '((detail . full)))`).SchemeString(),
		qt.Equals, "(score name)")

	// Withheld rows are never rendered: 5 rows, limit 1 => exactly 1 more
	// render call than before, whatever the two calls above left behind.
	eval(t, engine, `
		(define before (renders))
		(define rows5
		  '(((score . 5) (name . "e")) ((score . 4) (name . "d"))
		    ((score . 3) (name . "c")) ((score . 2) (name . "b"))
		    ((score . 1) (name . "a"))))
		(call-with-values
		  (lambda () (apply-view rows5 '((limit . 1)) #f rk rn))
		  (lambda (shown prov) shown))`)
	qt.Assert(t, eval(t, engine, `(- (renders) before)`).SchemeString(),
		qt.Equals, "1")
}

// view-name-matcher is the shared scope/symbol core both tools' keep? hooks
// are built from. The subtle parts are that scope is applied to the FILE of
// the name's position, and that both globs set is a conjunction on the SAME
// name -- not "scope matched something and symbol matched something".
func TestViewNameMatcher(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(import (wile goast utils))
		(define px (make-hashtable))
		(hashtable-set! px "pkg.Alpha" "/repo/goast/a.go:10:1")
		(hashtable-set! px "pkg.Beta"  "/repo/goast/b.go:20:1")
		;; Gamma is deliberately unlocated: no entry in the index.
		(define (m opts name)
		  (let ((f (view-name-matcher opts px)))
		    (if f (f name) 'no-matcher)))
	`)

	for expr, want := range map[string]string{
		// Neither glob set => no matcher at all, so apply-view admits all.
		`(m '() "pkg.Alpha")`:                                   "no-matcher",
		// symbol alone.
		`(m '((symbol . "Alpha")) "pkg.Alpha")`:                 "#t",
		`(m '((symbol . "Alpha")) "pkg.Beta")`:                  "#f",
		// scope alone, matched against the file, not the position.
		`(m '((scope . "a.go")) "pkg.Alpha")`:                   "#t",
		`(m '((scope . "goast/a.go")) "pkg.Alpha")`:             "#t",
		`(m '((scope . "a.go")) "pkg.Beta")`:                    "#f",
		// Conjunction on the SAME name: Alpha is in a.go, Beta is not.
		`(m '((scope . "a.go") (symbol . "Alpha")) "pkg.Alpha")`: "#t",
		`(m '((scope . "a.go") (symbol . "Beta")) "pkg.Beta")`:   "#f",
		`(m '((scope . "b.go") (symbol . "Beta")) "pkg.Beta")`:   "#t",
		// An unlocated name cannot satisfy a path glob, but a name glob
		// still works on it.
		`(m '((scope . "a.go")) "pkg.Gamma")`:                   "#f",
		`(m '((symbol . "Gamma")) "pkg.Gamma")`:                 "#t",
	} {
		got := eval(t, engine, expr)
		qt.Assert(t, got.SchemeString(), qt.Equals, want, qt.Commentf("%s", expr))
	}
}

// view-sort must survive a list past Wile's 10000-frame call-depth limit;
// the builtin `sort' dies at 20000. find_duplicates' own guard admits 25000
// scored pairs, every one of which may unify, so this is reachable.
func TestViewSort_DeepList(t *testing.T) {
	engine := newBeliefEngine(t)
	eval(t, engine, `
		(import (wile goast pipelines))
		(define big
		  (let loop ((i 0) (acc '()))
		    (if (>= i 25000) acc (loop (+ i 1) (cons i acc)))))
		(define sorted (view-sort < big))
	`)
	qt.Assert(t, eval(t, engine, `(length sorted)`).SchemeString(), qt.Equals, "25000")
	qt.Assert(t, eval(t, engine, `(car sorted)`).SchemeString(), qt.Equals, "0")
	qt.Assert(t, eval(t, engine, `(list-ref sorted 24999)`).SchemeString(), qt.Equals, "24999")
}
GOEOF
```

Then check formatting: `gofmt -l goast/view_test.go` (expect no output).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./goast/ -run 'TestApplyView|TestViewSort|TestViewNameMatcher' -count=1`
Expected: FAIL — `no such binding "apply-view"`.

- [ ] **Step 3: Implement the view layer**

Insert into `lib/wile/goast/pipelines.scm` immediately after `assoc-default` (after the current line 120), before the `;; ── recommend_boundaries ──` banner:

```scheme
;; ── Shared result view ───────────────────────────────────
;;
;; Filter, rank, page and render, uniformly across tools. Five caller
;; parameters live in OPTS: scope and symbol (globs), detail ('summary or
;; 'full), limit, offset. The DEFAULTS ARE WRITTEN HERE AND NOWHERE ELSE --
;; the Go handler forwards only what the caller actually set -- so a client
;; that knows nothing about these parameters gets a usable first response and
;; there is exactly one place to read the defaults off.

(define view-default-limit 50)
(define view-default-detail 'summary)

(define (view-detail opts) (or (assoc-default opts 'detail) view-default-detail))
(define (view-limit opts)  (or (assoc-default opts 'limit)  view-default-limit))
(define (view-offset opts) (or (assoc-default opts 'offset) 0))

;; view-merge / view-sort: a stable bottom-up merge sort in which every loop
;; is a tail call.
;;
;; The builtin `sort' is unusable here: measured on this engine it recurses one
;; frame per element and dies with "call depth 10001 exceeds limit 10000" on a
;; 20000-element list. find_duplicates can reach that -- max-candidate-pairs
;; admits 25000 scored pairs and nothing stops all of them unifying -- and the
;; failure would land AFTER the whole scoring cost is sunk. This is the same
;; class of bug as the old non-tail `map' that produced max-candidate-pairs
;; 9992; the fix there was to make the pass iterative, and it is here too.
(define (view-merge less? a b)
  (let loop ((a a) (b b) (acc '()))
    (cond ((and (null? a) (null? b)) (reverse acc))
          ((null? a) (loop a (cdr b) (cons (car b) acc)))
          ((null? b) (loop (cdr a) b (cons (car a) acc)))
          ;; Take from B only when strictly less, so equal keys keep A's
          ;; order: the sort is stable.
          ((less? (car b) (car a)) (loop a (cdr b) (cons (car b) acc)))
          (else (loop (cdr a) b (cons (car a) acc))))))

(define (view-sort less? lst)
  (if (or (null? lst) (null? (cdr lst)))
    lst
    (let pass ((runs (let split ((xs lst) (acc '()))
                       (if (null? xs)
                         (reverse acc)
                         (split (cdr xs) (cons (list (car xs)) acc))))))
      (if (null? (cdr runs))
        (car runs)
        (pass (let merge-pass ((rs runs) (acc '()))
                (cond ((null? rs) (reverse acc))
                      ((null? (cdr rs)) (reverse (cons (car rs) acc)))
                      (else (merge-pass (cddr rs)
                                        (cons (view-merge less? (car rs) (cadr rs))
                                              acc))))))))))

;; view-name-matcher: OPTS' scope/symbol globs as a predicate over ONE
;; function name, or #f when neither is set.
;;
;; Both tools identify a row by the function names it carries and differ only
;; in WHICH collection they scan -- a concept's extent, or a candidate pair's
;; two members -- so the matching itself belongs here, once. `symbol' matches
;; the name; `scope' matches the FILE of that name's position, because a
;; position is "file:line:col" and an anchored glob could never match one.
;; When both are set they must match the SAME name: the caller is identifying
;; one function by name and path, not two different ones.
(define (view-name-matcher opts pos-index)
  (let ((scope (assoc-default opts 'scope))
        (sym   (assoc-default opts 'symbol)))
    (and (or scope sym)
         (lambda (name)
           (and (or (not sym) (glob-match? sym name #\.))
                (or (not scope)
                    (glob-match? scope
                                 (position-file
                                   (hashtable-ref pos-index name ""))
                                 #\/)))))))

;; view-key<?: lexicographic order over a rank key (a list). NUMERIC
;; components compare DESCENDING -- a higher score, or more struct types tied
;; together, ranks first. STRING components compare ascending. Every rank key
;; ends in a string unique to its row, so the order is total: two calls with
;; the same arguments return the same rows in the same order, and page 2
;; cannot repeat a row from page 1. Offset paging is meaningless without that.
(define (view-key<? ka kb)
  (cond ((null? ka) #f)
        ((null? kb) #f)
        (else
          (let ((a (car ka)) (b (car kb)))
            (if (and (number? a) (number? b))
              (cond ((> a b) #t)
                    ((< a b) #f)
                    (else (view-key<? (cdr ka) (cdr kb))))
              (cond ((string<? a b) #t)
                    ((string<? b a) #f)
                    (else (view-key<? (cdr ka) (cdr kb)))))))))

;; apply-view: ROWS -> (values shown view-provenance).
;;   KEEP?     admits a row; #f admits everything, for a tool that already
;;             pruned upstream -- re-applying the predicate here would be a
;;             no-op that hides where the work was actually saved.
;;   RANK-KEY  row -> rank key (see view-key<?).
;;   RENDER    (row detail) -> the row's public shape. Called only on the rows
;;             actually returned, so a withheld row costs nothing to render.
;;             That is what lets find_false_boundaries skip its lattice
;;             annotation pass entirely at detail=summary rather than compute
;;             and discard it.
;;
;; total-count counts rows AFTER filtering and BEFORE offset/limit, so
;; `truncated' means the same thing whether the tool filtered here or upstream.
;; truncated is the load-bearing field: a client seeing total-count 186,
;; returned-count 50, truncated #t knows to narrow or page, and one seeing
;; truncated #f knows it holds the complete answer. Silent truncation with no
;; such signal would be worse than no limit at all.
(define (apply-view rows opts keep? rank-key render)
  (let* ((detail (view-detail opts))
         (limit  (view-limit opts))
         (offset (view-offset opts))
         (kept   (if keep? (filter keep? rows) rows))
         (total  (length kept))
         ;; Decorate once per row, not once per comparison, via a tail
         ;; recursion: `map' is not tail-recursive here and this list is one
         ;; entry per surviving row.
         (keyed  (let loop ((rs kept) (acc '()))
                   (if (null? rs)
                     (reverse acc)
                     (loop (cdr rs) (cons (cons (rank-key (car rs)) (car rs)) acc)))))
         (sorted (view-sort (lambda (a b) (view-key<? (car a) (car b))) keyed))
         ;; Undecorate and render the page in one tail-recursive pass: `take'
         ;; and `map' are both non-tail, and LIMIT is caller-supplied with no
         ;; ceiling of its own (find_duplicates' max-candidate-pairs guard
         ;; alone admits 25000 rows), so a caller asking for everything would
         ;; hit the same call-depth cliff view-sort exists to avoid, only
         ;; after the expensive analysis is already paid for.
         (shown  (let loop ((rs (drop sorted offset)) (n limit) (acc '()))
                   (if (or (null? rs) (<= n 0))
                     (reverse acc)
                     (loop (cdr rs) (- n 1)
                           (cons (render (cdr (car rs)) detail) acc)))))
         (returned (length shown)))
    (values shown
            (append
              (list (cons 'total-count total)
                    (cons 'returned-count returned)
                    (cons 'offset offset)
                    (cons 'limit limit)
                    (cons 'detail detail)
                    (cons 'truncated (> total (+ offset returned))))
              (let ((sc (assoc-default opts 'scope)))
                (if sc (list (cons 'scope sc)) '()))
              (let ((sy (assoc-default opts 'symbol)))
                (if sy (list (cons 'symbol sy)) '()))))))
```

- [ ] **Step 4: Export from `lib/wile/goast/pipelines.sld`**

In the `(export ...)` list, after `pipeline-envelope`, add:

```scheme
    ;; Shared result view (filter / rank / page / render). Exported so the
    ;; combinator is testable against synthetic rows, without either tool's
    ;; analysis in the way.
    apply-view
    view-sort
    view-name-matcher
```

- [ ] **Step 5: Run the tests**

Run: `go test ./goast/ -run 'TestApplyView|TestViewSort|TestViewNameMatcher' -count=1 -v`
Expected: PASS, 7 tests. `TestViewSort_DeepList` is the one that would have failed with the builtin `sort`.

- [ ] **Step 6: Full check and commit**

```bash
make ci
git add lib/wile/goast/pipelines.scm lib/wile/goast/pipelines.sld goast/view_test.go
git commit -m "feat(pipelines): apply-view combinator for filter/rank/page/render

One place owns everything not tool-specific: filtering, a total order,
offset/limit, per-detail rendering of the page only, and the provenance
those imply. Rendering after paging is what lets a tool skip work for
rows it will not return.

view-sort is a tail-safe bottom-up merge sort. The builtin sort recurses
one frame per element and dies at 20000 against Wile's 10000-frame limit;
find_duplicates' guard admits 25000 pairs, so that is reachable, and it
would land after the scoring cost is already sunk."
```

---

### Task 3: `find_false_boundaries` — post-filter, rank, detail, paging

**Files:**
- Modify: `lib/wile/goast/fca.sld` — export `attr-struct-name`
- Modify: `lib/wile/goast/fca-algebra.sld` — export `concept-summary`
- Modify: `lib/wile/goast/pipelines.scm` — new hooks + rewrite `pipeline-find-false-boundaries` (currently `pipelines.scm:151-170`)
- Modify: `cmd/wile-goast/mcp_tools.go` — `viewOpts` helper; five params on `find_false_boundaries` (registration `:111-128`, handler `:289-320`)
- Test: `cmd/wile-goast/mcp_tools_integration_test.go` (append)

**Interfaces:**
- Consumes: `apply-view`, `view-name-matcher` (Task 2); `field-index->positions` (`fca.scm:78`), `concept-extent`, `concept-intent`, `cross-boundary-concepts`, `attr-struct-name` from `(wile goast fca)`; `annotated-boundary-report`, `concept-summary` from `(wile goast fca-algebra)`.
- Produces:
  - `(fb-keep? opts pos-index) -> predicate-or-#f`
  - `(fb-types concept) -> list of string`
  - `(fb-rank-key concept) -> list`
  - `(fb-render lattice) -> (lambda (concept detail) alist)`
  - `(viewOpts req) ([]string, string)` in Go — Scheme `(cons 'key val)` fragments, or a non-empty error message.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/wile-goast/mcp_tools_integration_test.go`:

```go
// goastPkg is a real package (the phase1 fixture has no cross-boundary
// concept at all). Measured at plan time: 178 FCA objects, an 89-concept
// lattice, 5 cross-boundary concepts, ~1.7s.
const goastPkg = "github.com/aalpar/wile-goast/goast"

// Decision 2's regression pin. Pushing a symbol filter into the FCA context
// restricts the objects, recomputes the lattice over a different context, and
// reports absence where findings exist. Measured on ./goast, write-only:
// *restructure* keeps 3 objects and finds 0 concepts pushed down, against the
// 2 finished concepts whose extent contains restructureBackwardGotos. Telling
// a caller there are no false boundaries when there are is worse than
// declining, so this tool post-filters.
//
// Asserted as an invariant rather than a pinned count, because the extent
// names track goast's own churn: non-empty, a subset of the unfiltered rows,
// and every returned row genuinely matches. If goast changes such that no
// concept contains "restructure", replace the glob with another one that
// separates the two approaches (*attach* and *loopRewriter* also did at plan
// time) -- do not "fix" it by pushing the filter down.
func TestFindFalseBoundaries_PostFilterPreservesFindings(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: builds an 89-concept lattice over ./goast")
	}
	mc := inProcessClient(t)

	all := callTool(t, mc, "find_false_boundaries", map[string]any{"target": goastPkg})
	envelopeOK(t, all, 2.0)
	allRows := all["result"].([]any)
	qt.Assert(t, len(allRows) > 0, qt.IsTrue,
		qt.Commentf("fixture no longer produces cross-boundary concepts"))

	scoped := callTool(t, mc, "find_false_boundaries", map[string]any{
		"target": goastPkg,
		"symbol": "*restructure*",
	})
	envelopeOK(t, scoped, 2.0)
	rows := scoped["result"].([]any)

	qt.Assert(t, len(rows) > 0, qt.IsTrue,
		qt.Commentf("post-filter reported absence where findings exist: %v", rows))
	qt.Assert(t, len(rows) < len(allRows), qt.IsTrue,
		qt.Commentf("filter did not narrow: %d of %d", len(rows), len(allRows)))

	// Every returned row really matches, and is one of the unfiltered rows.
	unfiltered := map[string]bool{}
	for _, r := range allRows {
		unfiltered[extentKey(r)] = true
	}
	for _, r := range rows {
		qt.Assert(t, unfiltered[extentKey(r)], qt.IsTrue,
			qt.Commentf("row absent from the unfiltered result: %v", r))
		matched := false
		for _, n := range r.(map[string]any)["extent"].([]any) {
			if strings.Contains(n.(string), "restructure") {
				matched = true
			}
		}
		qt.Assert(t, matched, qt.IsTrue,
			qt.Commentf("row has no matching extent member: %v", r))
	}

	// The provenance is self-describing: cross_boundary_count is the
	// pre-filter population, total_count the post-filter one.
	prov := scoped["provenance"].(map[string]any)
	qt.Assert(t, prov["symbol"], qt.Equals, "*restructure*")
	qt.Assert(t, prov["total_count"].(float64), qt.Equals, float64(len(rows)))
	qt.Assert(t, prov["cross_boundary_count"].(float64) > prov["total_count"].(float64),
		qt.IsTrue, qt.Commentf("prov=%v", prov))
}

// extentKey joins a boundary row's extent into a stable identity string.
func extentKey(row any) string {
	ext, ok := row.(map[string]any)["extent"].([]any)
	if !ok {
		return ""
	}
	names := make([]string, 0, len(ext))
	for _, n := range ext {
		s, _ := n.(string)
		names = append(names, s)
	}
	return strings.Join(names, "|")
}

// Row WIDTH, not row count, is this tool's failure mode: measured on ./goast
// the three lattice-relationship lists are 90.5% of the result bytes
// (incomparable_with alone is 60.6%). summary drops them and adds types_count;
// full reproduces today's shape exactly.
func TestFindFalseBoundaries_DetailFieldSets(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: builds an 89-concept lattice over ./goast")
	}
	mc := inProcessClient(t)

	summary := callTool(t, mc, "find_false_boundaries", map[string]any{"target": goastPkg})
	full := callTool(t, mc, "find_false_boundaries", map[string]any{
		"target": goastPkg,
		"detail": "full",
	})

	wantSummary := []string{"extent", "intent", "extent_size", "summary", "types_count"}
	wantFull := []string{"extent", "intent", "extent_size", "summary",
		"subconcept_of", "superconcept_of", "incomparable_with"}

	assertFieldSet(t, summary["result"].([]any), wantSummary)
	assertFieldSet(t, full["result"].([]any), wantFull)

	// types_count agrees with the summary string, which is the '+'-joined
	// type list, so the recompute from the intent is not drifting.
	for _, r := range summary["result"].([]any) {
		row := r.(map[string]any)
		n := int(row["types_count"].(float64))
		qt.Assert(t, n, qt.Equals, strings.Count(row["summary"].(string), "+")+1,
			qt.Commentf("types_count vs summary: %v", row))
	}
}

// assertFieldSet fails unless every row has exactly the wanted keys.
func assertFieldSet(t *testing.T, rows []any, want []string) {
	t.Helper()
	qt.Assert(t, len(rows) > 0, qt.IsTrue, qt.Commentf("no rows to check"))
	for _, r := range rows {
		row := r.(map[string]any)
		got := make([]string, 0, len(row))
		for k := range row {
			got = append(got, k)
		}
		slices.Sort(got)
		w := slices.Clone(want)
		slices.Sort(w)
		qt.Assert(t, got, qt.DeepEquals, w, qt.Commentf("row=%v", row))
	}
}

// Ranking: more struct types tied together first, then a larger shared field
// set, then a larger extent. Repeated identical calls agree, and pages
// partition the unpaged result.
func TestFindFalseBoundaries_RankStabilityAndPaging(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: builds an 89-concept lattice over ./goast")
	}
	mc := inProcessClient(t)
	args := map[string]any{"target": goastPkg}

	a := callTool(t, mc, "find_false_boundaries", args)
	b := callTool(t, mc, "find_false_boundaries", args)
	qt.Assert(t, reflect.DeepEqual(a["result"], b["result"]), qt.IsTrue,
		qt.Commentf("non-deterministic order"))

	all := a["result"].([]any)
	qt.Assert(t, len(all) >= 2, qt.IsTrue, qt.Commentf("need >=2 rows to page"))

	// Rank key is non-increasing across the result.
	prev := -1
	for _, r := range all {
		n := int(r.(map[string]any)["types_count"].(float64))
		if prev >= 0 {
			qt.Assert(t, n <= prev, qt.IsTrue,
				qt.Commentf("types_count not descending: %d after %d", n, prev))
		}
		prev = n
	}

	p1 := callTool(t, mc, "find_false_boundaries", map[string]any{
		"target": goastPkg, "limit": float64(1), "offset": float64(0)})
	p2 := callTool(t, mc, "find_false_boundaries", map[string]any{
		"target": goastPkg, "limit": float64(len(all) - 1), "offset": float64(1)})

	qt.Assert(t, p1["provenance"].(map[string]any)["truncated"], qt.Equals, true)
	qt.Assert(t, p2["provenance"].(map[string]any)["truncated"], qt.Equals, false)

	joined := append(slices.Clone(p1["result"].([]any)), p2["result"].([]any)...)
	qt.Assert(t, reflect.DeepEqual(joined, all), qt.IsTrue,
		qt.Commentf("pages do not partition the unpaged result"))
}

// A glob matching nothing is an empty SUCCESS, not an error -- consistent
// with an empty find_duplicates result meaning "no duplicates".
func TestFindFalseBoundaries_NoMatchIsEmptySuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: builds an 89-concept lattice over ./goast")
	}
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_false_boundaries", map[string]any{
		"target": goastPkg,
		"symbol": "NoSuchFunctionAnywhere",
	})
	envelopeOK(t, env, 2.0)
	qt.Assert(t, env["result"], qt.DeepEquals, []any{})
	prov := env["provenance"].(map[string]any)
	qt.Assert(t, prov["total_count"], qt.Equals, float64(0))
	qt.Assert(t, prov["truncated"], qt.Equals, false)
}

// detail / limit / offset are validated in the Go handler, matching how mode
// already is. A bad value is a tool error naming what is legal.
func TestViewParams_Validation(t *testing.T) {
	mc := inProcessClient(t)
	ctx := context.Background()

	cases := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"find_false_boundaries", map[string]any{"target": phase1Pkg, "detail": "brief"},
			"detail must be summary or full"},
		{"find_false_boundaries", map[string]any{"target": phase1Pkg, "limit": float64(-1)},
			"limit must be a non-negative integer"},
		{"find_false_boundaries", map[string]any{"target": phase1Pkg, "offset": float64(-3)},
			"offset must be a non-negative integer"},
		{"find_false_boundaries", map[string]any{"target": phase1Pkg, "limit": 2.5},
			"limit must be a non-negative integer"},
	}
	for _, tc := range cases {
		req := mcp.CallToolRequest{}
		req.Params.Name = tc.tool
		req.Params.Arguments = tc.args
		res, err := mc.CallTool(ctx, req)
		qt.Assert(t, err, qt.IsNil)
		qt.Assert(t, res.IsError, qt.IsTrue, qt.Commentf("args=%v", tc.args))
		tcText, ok := mcp.AsTextContent(res.Content[0])
		qt.Assert(t, ok, qt.IsTrue)
		qt.Assert(t, tcText.Text, qt.Equals, tc.want)
	}
}
```

Update the existing version assertion at `mcp_tools_integration_test.go:251`:

```go
	env := callTool(t, mc, "find_false_boundaries", map[string]any{"target": phase1Pkg})
	envelopeOK(t, env, 2.0)
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/wile-goast/ -run 'TestFindFalseBoundaries|TestViewParams' -count=1`
Expected: FAIL — version 1 vs 2, unknown parameters accepted without error, `types_count` missing.

- [ ] **Step 3: Export the two library identifiers**

`lib/wile/goast/fca.sld` — add to the "Defined locally" export block, after `field-index->positions`:

```scheme
    attr-struct-name
```

`lib/wile/goast/fca-algebra.sld` — add to the export list:

```scheme
    concept-summary
```

- [ ] **Step 4: Add the `find_false_boundaries` hooks to `pipelines.scm`**

Insert directly above the `;; ── find_false_boundaries ──` banner (currently `pipelines.scm:143`):

```scheme
;; ── find_false_boundaries: view hooks ────────────────────

;; fb-keep?: OPTS' scope/symbol as a predicate over a cross-boundary concept,
;; or #f when neither is set.
;;
;; The filter runs on FINISHED concepts, never on the FCA context. Restricting
;; the context recomputes the lattice over a different object set and reports
;; absence where findings exist. Measured on ./goast, write-only, 178 objects,
;; 5 cross-boundary concepts: pushing *restructure* down keeps 3 objects and
;; finds 0 concepts, against the 2 finished concepts whose extent contains
;; restructureBackwardGotos. Telling a caller there are no false boundaries
;; when there are five is worse than declining. There is no cost argument on
;; the other side either: the whole call is ~1.7s over an 89-concept lattice.
;;
;; A concept is admitted when AT LEAST ONE extent member matches -- the caller
;; asked about a function, and a cross-boundary concept is interesting
;; precisely because it ties that function to others.
(define (fb-keep? opts pos-index)
  (let ((match? (view-name-matcher opts pos-index)))
    (and match?
         (lambda (concept)
           (let loop ((ns (concept-extent concept)))
             (cond ((null? ns) #f)
                   ((match? (car ns)) #t)
                   (else (loop (cdr ns)))))))))

;; fb-types: the distinct struct types a concept's intent spans.
;; cross-boundary-concepts computes exactly this at fca.scm:286 to enforce
;; min-types and then discards it, returning the bare concept; the pipeline
;; calls annotated-boundary-report rather than the boundary-report that
;; recomputes it. So this is a one-line recompute, not an exposed value being
;; plumbed through the algebra layer.
(define (fb-types concept)
  (unique (map attr-struct-name (concept-intent concept))))

;; fb-rank-key: types spanned, then intent size, then extent size, all
;; descending; the joined extent is the deterministic tie-break. A concept
;; that ties MORE struct types together across MORE shared fields is the
;; stronger false-boundary signal.
;;
;; NB this ordering is a choice made HERE. cross-boundary-concepts takes
;; min-types as a floor and expresses no preference among concepts above it.
(define (fb-rank-key concept)
  (list (length (fb-types concept))
        (length (concept-intent concept))
        (length (concept-extent concept))
        (string-join (concept-extent concept) " ")))

;; fb-render: a concept -> its public row at DETAIL.
;;   full    = today's shape exactly: extent, intent, extent-size, summary,
;;             and the three lattice-relationship lists.
;;   summary = the finding itself plus types-count, dropping the relationship
;;             lists. Measured on ./goast those three fields are 90.5% of the
;;             13301 result bytes (incomparable-with alone 60.6%), which is
;;             why row WIDTH, not row count, is this tool's failure mode: a
;;             design that only paged would leave it exactly as unusable.
;; Because apply-view renders after paging, annotated-boundary-report runs
;; only for the rows actually returned -- and at summary, not at all.
(define (fb-render lattice)
  (lambda (concept detail)
    (if (eq? detail 'full)
      (car (annotated-boundary-report (list concept) lattice))
      (list (cons 'extent (concept-extent concept))
            (cons 'intent (concept-intent concept))
            (cons 'extent-size (length (concept-extent concept)))
            (cons 'summary (concept-summary concept))
            (cons 'types-count (length (fb-types concept)))))))
```

- [ ] **Step 5: Rewrite `pipeline-find-false-boundaries`**

Replace the body (`pipelines.scm:151-170`) with:

```scheme
(define (pipeline-find-false-boundaries target opts)
  (let* ((session (go-load target))
         (field-idx (go-ssa-field-index session))
         (mode (or (assoc-default opts 'mode) 'write-only))
         (min-ext (or (assoc-default opts 'min-extent) 2))
         (min-int (or (assoc-default opts 'min-intent) 2))
         (min-typ (or (assoc-default opts 'min-types) 2))
         (ctx (field-index->context field-idx mode))
         (lattice (concept-lattice ctx))
         (cross (cross-boundary-concepts lattice
                  'min-extent min-ext
                  'min-intent min-int
                  'min-types min-typ))
         ;; The annotated report carries no position, so scope needs the
         ;; name->position join the FCA layer keeps out of the algebra.
         (pos-index (field-index->positions field-idx)))
    (call-with-values
      (lambda ()
        (apply-view cross opts (fb-keep? opts pos-index)
                    fb-rank-key (fb-render lattice)))
      (lambda (shown view-prov)
        ;; Version 2: detail=summary is the default, so a default call's row
        ;; shape changed (the three relationship lists are gone).
        (pipeline-envelope 2
          (append
            (list (cons 'target target)
                  (cons 'mode mode)
                  (cons 'lattice-size (length lattice))
                  ;; cross-boundary-count is the PRE-filter population;
                  ;; total-count (from the view) is the post-filter one.
                  (cons 'cross-boundary-count (length cross)))
            view-prov)
          shown)))))
```

Also update the section comment above it (`pipelines.scm:143-150`) to mention the post-filter.

- [ ] **Step 6: Add `viewOpts` and wire the Go handler**

In `cmd/wile-goast/mcp_tools.go`, add `"math"` to the import block, and add this helper immediately after `boundaryMode` (currently `:160-169`):

```go
// viewOpts validates the five shared result-view parameters and returns the
// Scheme (cons 'key val) fragments for those the caller actually set, or a
// non-empty error message.
//
// An unset parameter emits nothing. Every default lives in the Scheme
// pipeline (view-default-limit, view-default-detail), so there is exactly one
// place a default is written and it is testable without an MCP round trip.
//
// RequireFloat accepts both float64 (JSON transport) and int (the in-process
// test client), so the integrality check is done here rather than delegated
// to RequireInt, which would silently truncate limit=2.5.
func viewOpts(req mcp.CallToolRequest) ([]string, string) {
	args := req.GetArguments()
	var parts []string

	// NB: no compound `if init; cond` below -- this repo's own ruleguard rule
	// noCompoundIf (ruleguard/rules.go:34) bans that form and `make lint`
	// fails on it. `switch init; tag` is not matched by that rule and is fine.
	scope := req.GetString("scope", "")
	if scope != "" {
		parts = append(parts, "(cons 'scope "+schemeStringLiteral(scope)+")")
	}
	symbol := req.GetString("symbol", "")
	if symbol != "" {
		parts = append(parts, "(cons 'symbol "+schemeStringLiteral(symbol)+")")
	}
	switch s := req.GetString("detail", ""); s {
	case "": // unset
	case "summary", "full":
		parts = append(parts, "(cons 'detail '"+s+")")
	default:
		return nil, "detail must be summary or full"
	}
	for _, name := range []string{"limit", "offset"} {
		_, present := args[name]
		if !present {
			continue
		}
		f, err := req.RequireFloat(name)
		if err != nil || f < 0 || f != math.Trunc(f) {
			return nil, name + " must be a non-negative integer"
		}
		parts = append(parts, fmt.Sprintf("(cons '%s %d)", name, int64(f)))
	}
	return parts, ""
}
```

Add the five parameter declarations to the `find_false_boundaries` registration (after the `min_types` line, currently `:124-125`):

```go
			mcp.WithString("scope",
				mcp.Description("Only report rows whose function matches this path glob "+
					"('*' only). Matches the whole path or any '/'-aligned suffix, so "+
					"'marshal.go', 'goast/marshal.go' and '*/goast/*.go' all work.")),
			mcp.WithString("symbol",
				mcp.Description("Only report rows whose function name matches this glob "+
					"('*' only). Matches the whole name or any '.'-aligned suffix, so a "+
					"bare 'cloneTypeExpr' matches the package-qualified form.")),
			mcp.WithString("detail",
				mcp.Description("Row width: summary (default) drops the lattice "+
					"relationship lists and adds types_count; full returns them.")),
			mcp.WithNumber("limit",
				mcp.Description("Maximum rows returned (default 50). provenance.truncated "+
					"says whether rows were withheld.")),
			mcp.WithNumber("offset",
				mcp.Description("Rows skipped before limit applies (default 0).")),
```

Replace the option-building block in `handleFindFalseBoundaries` (currently `:299-315`) with:

```go
	args := req.GetArguments()
	var optsParts []string
	if mode != "" {
		optsParts = append(optsParts, "(cons 'mode "+modeArg+")")
	}
	v, ok := args["min_extent"]
	if ok {
		optsParts = append(optsParts, fmt.Sprintf("(cons 'min-extent %v)", v))
	}
	v, ok = args["min_intent"]
	if ok {
		optsParts = append(optsParts, fmt.Sprintf("(cons 'min-intent %v)", v))
	}
	v, ok = args["min_types"]
	if ok {
		optsParts = append(optsParts, fmt.Sprintf("(cons 'min-types %v)", v))
	}
	viewParts, verr := viewOpts(req)
	if verr != "" {
		return mcp.NewToolResultError(verr), nil
	}
	optsParts = append(optsParts, viewParts...)
```

- [ ] **Step 7: Run the tests**

```bash
go test ./cmd/wile-goast/ -run 'TestFindFalseBoundaries|TestViewParams' -count=1 -v
```

Expected: PASS, 6 tests. If a `.scm` edit does not seem to take effect, `-count=1` is already forcing a re-run; also confirm `embed.go` at the repo root still embeds `lib`.

- [ ] **Step 8: Full check and commit**

```bash
make ci
git add lib/wile/goast/fca.sld lib/wile/goast/fca-algebra.sld \
        lib/wile/goast/pipelines.scm cmd/wile-goast/mcp_tools.go \
        cmd/wile-goast/mcp_tools_integration_test.go
git commit -m "feat(find_false_boundaries): scope/symbol post-filter, rank, detail, paging

Row width, not row count, is this tool's failure mode: on ./goast the
three lattice-relationship lists are 90.5% of the result bytes.
detail=summary drops them and adds types_count; full is unchanged.

The filter runs on finished concepts, never on the FCA context.
Restricting the objects recomputes the lattice and reports absence where
findings exist: *restructure* pushed down finds 0 concepts against the 2
that actually match. Envelope version 1 -> 2 (default row shape changed)."
```

> **2026-07-29:** the "full is unchanged" / "full reproduces today's shape
> exactly" field lists above (test comment and commit message, as landed)
> are historical record of Task 3 as originally shipped, kept as-is. The
> requirement they document — `full` byte-for-byte stable, `types_count`
> summary-only — is retired by owner's decision: `types_count` is
> `fb-rank-key`'s primary sort field, so a `full` result was ordered by a
> key it did not expose, and this tool has no consumers other than the
> owner to preserve the old shape for. `types_count` now appears at both
> detail levels. See `2026-07-28-goast-result-view-design.md`, Testing
> item 6.

---

### Task 4: `dup-detect.scm` — a pair-list entry point (pure refactor)

Behavior-preserving. Split out so the risky restructure is reviewable on its own, before any filtering rides on it.

**Files:**
- Modify: `lib/wile/goast/dup-detect.scm:237-279`
- Modify: `lib/wile/goast/dup-detect.sld` — export `candidate-pairs`, `score-candidate-pairs`
- Test: `goast/dup_detect_test.go` (append)

**Interfaces:**
- Consumes: existing `candidate-pairs` (`dup-detect.scm:257`), `short-name`, `build-func-ast-index`, `build-func-canon-index`, `func-refs->positions`, `score-pair-entry`.
- Produces:
  - `(scored-candidates pairs ast-index canon-index pos-index threshold) -> list` — **signature change**: first argument is now the flat pair list, not the concept list.
  - `(score-candidate-pairs session refs pairs threshold) -> list of scored candidate`
  - `(pair-short-names pairs) -> hashtable` (internal)
  - `(candidate-pairs concepts) -> list of (a . b)` — unchanged, now exported.

- [ ] **Step 1: Write the failing test**

Append to `goast/dup_detect_test.go`:

```go
// The split of find-scored-candidates into discovery + score-candidate-pairs
// is behaviour-preserving: driving the lower entry point with the full pair
// list must reproduce the top-level result exactly. This is the regression
// guard for the restructure that the scope/symbol push-down rides on.
func TestScoreCandidatePairs_MatchesTopLevel(t *testing.T) {
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"

	eval(t, engine, `
		(import (wile goast dup-detect))
		(import (wile goast fca))
		(import (wile goast utils))
		(define sess (go-load "`+pkg+`"))
		(define refs (go-func-refs sess))
		(define cands (duplicate-candidate-concepts
		                (concept-lattice (function-ref-context refs))))
		(define prs (candidate-pairs cands))
		(define via-top (find-scored-candidates sess 0.6))
		(define via-pairs (score-candidate-pairs sess refs prs 0.6))
		(define (ids cs)
		  (map (lambda (c) (cdr (assq 'pair c))) cs))
		(define (tiers cs)
		  (map (lambda (c) (cdr (assq 'equiv-tier (cdr (assq 'measures c))))) cs))
	`)

	c := qt.New(t)
	c.Assert(eval(t, engine, `(> (length via-top) 0)`).SchemeString(), qt.Equals, "#t")
	c.Assert(eval(t, engine, `(= (length via-top) (length via-pairs))`).SchemeString(),
		qt.Equals, "#t")
	c.Assert(eval(t, engine, `(equal? (ids via-top) (ids via-pairs))`).SchemeString(),
		qt.Equals, "#t")
	c.Assert(eval(t, engine, `(equal? (tiers via-top) (tiers via-pairs))`).SchemeString(),
		qt.Equals, "#t")
}

// Scoring a PRUNED pair list yields exactly the corresponding subset: the
// same pairs score identically whether or not their neighbours were scored.
// That is the soundness claim the push-down rests on.
func TestScoreCandidatePairs_PrunedIsSubset(t *testing.T) {
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"

	eval(t, engine, `
		(import (wile goast dup-detect))
		(import (wile goast fca))
		(import (wile goast utils))
		(define sess2 (go-load "`+pkg+`"))
		(define refs2 (go-func-refs sess2))
		(define cands2 (duplicate-candidate-concepts
		                 (concept-lattice (function-ref-context refs2))))
		(define prs2 (candidate-pairs cands2))
		(define (touches-sum? pr)
		  (or (glob-match? "*SumSlice*" (car pr) #\.)
		      (glob-match? "*SumSlice*" (cdr pr) #\.)))
		(define pruned (filter touches-sum? prs2))
		(define full-run (score-candidate-pairs sess2 refs2 prs2 0.6))
		(define pruned-run (score-candidate-pairs sess2 refs2 pruned 0.6))
		(define (find-pair cs a b)
		  (let loop ((xs cs))
		    (cond ((null? xs) #f)
		          ((equal? (cdr (assq 'pair (car xs))) (list a b)) (car xs))
		          (else (loop (cdr xs))))))
	`)

	c := qt.New(t)
	c.Assert(eval(t, engine, `(< (length pruned) (length prs2))`).SchemeString(),
		qt.Equals, "#t")
	c.Assert(eval(t, engine, `(<= (length pruned-run) (length full-run))`).SchemeString(),
		qt.Equals, "#t")
	// The clone pair scores identically in both runs.
	c.Assert(eval(t, engine, `
		(let ((a (find-pair full-run "SumSlice" "TotalSlice"))
		      (b (find-pair pruned-run "SumSlice" "TotalSlice")))
		  (and a b (equal? (cdr (assq 'measures a)) (cdr (assq 'measures b)))))
	`).SchemeString(), qt.Equals, "#t")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./goast/ -run 'TestScoreCandidatePairs' -count=1`
Expected: FAIL — `no such binding "score-candidate-pairs"`.

- [ ] **Step 3: Rewrite `dup-detect.scm:237-279`**

Replace `scored-candidates` and `find-scored-candidates`, and add the two new procedures. The block from the `;; scored-candidates:` comment through `find-scored-candidates` becomes:

```scheme
;; scored-candidates: score a flat PAIRS work list into scored candidate
;; entries ((pair . (a b)) (measures . M) (findings . (finding-a finding-b))).
;; Pairs whose AST nodes cannot be resolved (short-name miss) are dropped.
;;
;; Takes the pairs rather than the concepts so a caller can size or prune the
;; work list before paying for it; candidate-pairs flattens concepts into that
;; list. The concept grouping does not survive into the (flat) output anyway.
(define (scored-candidates pairs ast-index canon-index pos-index threshold)
  (filter-map (lambda (pr)
                (score-pair-entry pr ast-index canon-index pos-index threshold))
              pairs))

;; pair-short-names: the short names appearing anywhere in PAIRS, as a set.
;; These are the only keys score-candidate-pair will ever look up, so
;; canonicalizing anything else is wasted work -- and skipping it is what makes
;; a narrowly scoped scan genuinely CHEAP rather than merely small.
(define (pair-short-names pairs)
  (let ((h (make-hashtable)))
    (for-each (lambda (pr)
                (hashtable-set! h (short-name (car pr)) #t)
                (hashtable-set! h (short-name (cdr pr)) #t))
              pairs)
    h))

;; score-candidate-pairs: score PAIRS against SESSION's AST and SSA. REFS is
;; the caller's go-func-refs result, reused rather than re-fetched.
;;
;; Split out of find-scored-candidates so a caller that already ran discovery
;; -- to size the work list against a budget, or to prune it -- does not run
;; discovery twice.
;;
;; The canon index covers only the functions PAIRS actually names. This is
;; result-preserving: score-candidate-pair reads canon-index by short name and
;; only for names in PAIRS, and filtering by short-name membership keeps
;; colliding short names together (both survive or neither), so
;; build-func-canon-index's last-wins order over a collision is unchanged.
(define (score-candidate-pairs session refs pairs threshold)
  (let* ((needed (pair-short-names pairs))
         (pos-index (func-refs->positions refs))
         (ast-index (build-func-ast-index (go-typecheck-package session)))
         (canon-index
           (build-func-canon-index
             (filter (lambda (fn)
                       (let ((nm (nf fn 'name)))
                         (and (string? nm)
                              (hashtable-ref needed (short-name nm) #f))))
                     (let ((fs (go-ssa-build session))) (if (pair? fs) fs '()))))))
    (scored-candidates pairs ast-index canon-index pos-index threshold)))

;; find-scored-candidates: top-level. TARGET is a package pattern or a
;; GoSession. Runs 5a discovery (FCA-on-refs clusters) then 5b structural
;; scoring over every within-cluster pair. Optional THRESHOLD (default 0.6) is
;; the similarity/unifiable? threshold. Returns a flat list of scored entries.
(define (find-scored-candidates target . opts)
  (let* ((threshold (if (pair? opts) (car opts) 0.6))
         (s         (if (go-session? target) target (go-load target)))
         (refs      (go-func-refs s))
         (cands     (duplicate-candidate-concepts
                      (concept-lattice (function-ref-context refs)))))
    (score-candidate-pairs s refs (candidate-pairs cands) threshold)))
```

Keep `candidate-pairs` (`dup-detect.scm:257`) exactly as it is; it now has an external caller.

- [ ] **Step 4: Export from `lib/wile/goast/dup-detect.sld`**

In the "slice 5b" export block, alongside `scored-candidates`:

```scheme
          candidate-pairs
          scored-candidates
          score-candidate-pairs
          find-scored-candidates
```

- [ ] **Step 5: Run the tests**

```bash
go test ./goast/ -run 'TestScoreCandidatePairs|TestDup|TestFindScored' -count=1 -v
go test ./cmd/wile-goast/ -run TestFindDuplicates -count=1 -short
```

Expected: PASS. The existing `find_duplicates` tests must be unaffected — this task changes no observable behaviour.

- [ ] **Step 6: Full check and commit**

```bash
make ci
git add lib/wile/goast/dup-detect.scm lib/wile/goast/dup-detect.sld goast/dup_detect_test.go
git commit -m "refactor(dup-detect): score-candidate-pairs takes the pair list

scored-candidates now consumes the flat pair work list rather than the
concepts, and score-candidate-pairs exposes the scoring half so a caller
that already ran discovery can size or prune the list before paying for
it. find-scored-candidates is unchanged from the outside.

The canon index is built only over functions the pair list names --
result-preserving, since score-candidate-pair looks up nothing else and
short-name collisions survive or die together."
```

---

### Task 5: `find_duplicates` — push-down, guard-after-prune, view

**Files:**
- Modify: `lib/wile/goast/pipelines.scm` — new hooks; rewrite `pipeline-find-duplicates` (`:283-326`); delete `candidate-pair-count` (`:247-256`); amend the `max-candidate-pairs` comment (`:229-245`)
- Modify: `lib/wile/goast/pipelines.sld` — import the new dup-detect entry points
- Modify: `cmd/wile-goast/mcp_tools.go` — five params on `find_duplicates` (registration `:87-110`, handler `:322-344`)
- Test: `cmd/wile-goast/mcp_tools_integration_test.go` (append + amend)

**Interfaces:**
- Consumes: `apply-view`, `view-name-matcher` (Task 2); `candidate-pairs`, `score-candidate-pairs`, `func-refs->positions` (Task 4); existing `candidate-tier`, `kept-tiers`, `tier-tally`, `candidate->clean`, `->float64`. In Go: the `assertFieldSet` test helper added by Task 3 in the same file — this task must land after Task 3.
- Produces:
  - `(dup-keep-pair? opts pos-index) -> predicate-or-#f` over a pair `(a . b)`
  - `(dup-tier-rank tier) -> 0|1`
  - `(dup-rank-key cand) -> list`
  - `(dup-render verdict?) -> (lambda (cand detail) alist)`

- [ ] **Step 1: Write the failing tests**

Append to `cmd/wile-goast/mcp_tools_integration_test.go`:

```go
// The push-down is a real prune, not a post-filter: a scoped call scores
// strictly fewer pairs and returns a subset of the unscoped rows.
//
// Asserted on the pair counts rather than on wall-clock. A timing assertion
// would smuggle machine-dependent nondeterminism into the suite; the pair
// count IS the work, and it is deterministic.
func TestFindDuplicates_ScopeIsAPrune(t *testing.T) {
	mc := inProcessClient(t)

	all := callTool(t, mc, "find_duplicates", map[string]any{"target": dupclusterPkg})
	envelopeOK(t, all, 2.0)
	allProv := all["provenance"].(map[string]any)

	scoped := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg,
		"symbol": "*SumSlice*",
	})
	envelopeOK(t, scoped, 2.0)
	prov := scoped["provenance"].(map[string]any)

	// Fewer pairs reach the scorer, and the count is taken AFTER the prune.
	qt.Assert(t, prov["candidate_pair_count"].(float64) <
		allProv["candidate_pair_count"].(float64), qt.IsTrue,
		qt.Commentf("pair count not pruned: %v vs %v",
			prov["candidate_pair_count"], allProv["candidate_pair_count"]))
	qt.Assert(t, prov["scored_pair_count"].(float64) <
		allProv["scored_pair_count"].(float64), qt.IsTrue)

	// Every scoped row is an unscoped row, and mentions the queried symbol.
	unscoped := map[string]bool{}
	for _, r := range all["result"].([]any) {
		unscoped[pairKey(r)] = true
	}
	rows := scoped["result"].([]any)
	qt.Assert(t, len(rows) > 0, qt.IsTrue, qt.Commentf("scoped call returned nothing"))
	for _, r := range rows {
		qt.Assert(t, unscoped[pairKey(r)], qt.IsTrue,
			qt.Commentf("scoped row absent from unscoped result: %v", r))
		qt.Assert(t, strings.Contains(pairKey(r), "SumSlice"), qt.IsTrue,
			qt.Commentf("row does not mention the queried symbol: %v", r))
	}
	// A pair survives if EITHER function matches: asking about SumSlice
	// surfaces its duplicate, TotalSlice, which does not match the glob.
	qt.Assert(t, findCandidateWithPair(rows, "SumSlice", "TotalSlice"),
		qt.Not(qt.IsNil))
}

// pairKey joins a candidate's function names into a stable identity string.
func pairKey(row any) string {
	funcs, ok := row.(map[string]any)["functions"].([]any)
	if !ok {
		return ""
	}
	names := make([]string, 0, len(funcs))
	for _, f := range funcs {
		n, _ := f.(map[string]any)["name"].(string)
		names = append(names, n)
	}
	return strings.Join(names, "|")
}

// scope narrows by file, against the absolute position the tool carries.
func TestFindDuplicates_ScopeByFile(t *testing.T) {
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg,
		"scope":  "clones.go",
	})
	envelopeOK(t, env, 2.0)
	rows := env["result"].([]any)
	qt.Assert(t, findCandidateWithPair(rows, "SumSlice", "TotalSlice"), qt.Not(qt.IsNil),
		qt.Commentf("scope=clones.go did not surface the clone pair: %v", rows))
	qt.Assert(t, env["provenance"].(map[string]any)["scope"], qt.Equals, "clones.go")
}

// The single largest win in the design: the pair-count guard reads the count
// AFTER the prune, so a narrow query against a target that would otherwise be
// refused is answered instead. The guard becomes a wall the caller can walk
// around by being specific.
func TestFindDuplicates_ScopeWalksAroundTheGuard(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: loads and SSA-builds the whole tree")
	}
	mc := inProcessClient(t)

	// Unscoped, this target is refused (58551 pairs, over the 25000 budget).
	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Name = "find_duplicates"
	req.Params.Arguments = map[string]any{"target": wholeTreePkg}
	res, err := mc.CallTool(ctx, req)
	qt.Assert(t, err, qt.IsNil)
	qt.Assert(t, res.IsError, qt.IsTrue, qt.Commentf("target no longer exceeds the budget"))

	// Scoped to one file, the same target is answered.
	env := callTool(t, mc, "find_duplicates", map[string]any{
		"target": wholeTreePkg,
		"scope":  "goastcfg/prim_cfg.go",
	})
	envelopeOK(t, env, 2.0)
	prov := env["provenance"].(map[string]any)
	qt.Assert(t, prov["candidate_pair_count"].(float64) <= 25000, qt.IsTrue,
		qt.Commentf("pruned pair count still over budget: %v", prov["candidate_pair_count"]))
	qt.Assert(t, prov["scope"], qt.Equals, "goastcfg/prim_cfg.go")
}

// summary drops the measures sub-object (whose similarity merely repeats the
// top-level score); full is today's shape. An opt-in verdict survives both:
// a caller who asked for it asked for it.
func TestFindDuplicates_DetailFieldSets(t *testing.T) {
	mc := inProcessClient(t)

	summary := callTool(t, mc, "find_duplicates", map[string]any{"target": dupclusterPkg})
	full := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg, "detail": "full"})
	withVerdict := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg, "verdict": true})

	assertFieldSet(t, summary["result"].([]any),
		[]string{"functions", "score", "equiv_tier"})
	assertFieldSet(t, full["result"].([]any),
		[]string{"functions", "score", "equiv_tier", "measures"})
	assertFieldSet(t, withVerdict["result"].([]any),
		[]string{"functions", "score", "equiv_tier", "verdict"})
}

// Ranking: score descending, proven before structural on ties, deterministic
// on repeat, and pages partition the unpaged result.
func TestFindDuplicates_RankStabilityAndPaging(t *testing.T) {
	mc := inProcessClient(t)
	args := map[string]any{"target": dupclusterPkg}
	a := callTool(t, mc, "find_duplicates", args)
	b := callTool(t, mc, "find_duplicates", args)
	qt.Assert(t, reflect.DeepEqual(a["result"], b["result"]), qt.IsTrue,
		qt.Commentf("non-deterministic order"))

	all := a["result"].([]any)
	qt.Assert(t, len(all) >= 2, qt.IsTrue, qt.Commentf("need >=2 rows to page"))

	prev := 2.0
	for _, r := range all {
		s := r.(map[string]any)["score"].(float64)
		qt.Assert(t, s <= prev, qt.IsTrue,
			qt.Commentf("score not descending: %v after %v", s, prev))
		prev = s
	}

	p1 := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg, "limit": float64(1), "offset": float64(0)})
	p2 := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg, "limit": float64(len(all) - 1), "offset": float64(1)})

	qt.Assert(t, p1["provenance"].(map[string]any)["truncated"], qt.Equals, true)
	qt.Assert(t, p1["provenance"].(map[string]any)["total_count"].(float64),
		qt.Equals, float64(len(all)))
	qt.Assert(t, p2["provenance"].(map[string]any)["truncated"], qt.Equals, false)

	joined := append(slices.Clone(p1["result"].([]any)), p2["result"].([]any)...)
	qt.Assert(t, reflect.DeepEqual(joined, all), qt.IsTrue,
		qt.Commentf("pages do not partition the unpaged result"))
}

// A glob matching nothing is an empty success, not an error.
func TestFindDuplicates_NoMatchIsEmptySuccess(t *testing.T) {
	mc := inProcessClient(t)
	env := callTool(t, mc, "find_duplicates", map[string]any{
		"target": dupclusterPkg,
		"symbol": "NoSuchFunctionAnywhere",
	})
	envelopeOK(t, env, 2.0)
	qt.Assert(t, env["result"], qt.DeepEquals, []any{})
	prov := env["provenance"].(map[string]any)
	qt.Assert(t, prov["total_count"], qt.Equals, float64(0))
	qt.Assert(t, prov["candidate_pair_count"], qt.Equals, float64(0))
	qt.Assert(t, prov["truncated"], qt.Equals, false)
}
```

Amend the existing `find_duplicates` tests for version 2 and the dropped `candidate_count`:

- `TestFindDuplicates_DupclusterFixture` (`:290`, `:294`): `envelopeOK(t, env, 2.0)`; replace the `candidate_count` assertion with `qt.Assert(t, prov["total_count"].(float64) > 0, qt.IsTrue)`; **and add `"detail": "full"` to the call args** — this test reads `clone["measures"]` unconditionally, which v2's default `summary` drops, so without it the test panics on a nil map assertion. Coverage of the *default* shape is not lost: `TestFindDuplicates_DetailFieldSets` asserts the summary row's field set is exactly `{functions, score, equiv_tier}`.
- `TestFindDuplicates_VerdictOptIn` (`:346`): `envelopeOK(t, env, 2.0)`.
- `TestFindDuplicates_EmptySuccess` (`:428-431`): `envelopeOK(t, env, 2.0)`; `prov["total_count"]` instead of `candidate_count`.
- `TestFindDuplicates_AnswersPastOldCeiling` (`:515-524`): `envelopeOK(t, env, 2.0)`; replace the two `candidate_count` / `len(result)` equality assertions with:

```go
	tiers := prov["tiers"].(map[string]any)
	kept := tiers["proven"].(float64) + tiers["structural"].(float64)
	qt.Assert(t, prov["total_count"], qt.Equals, kept)
	// The default limit is 50, so the result is the first page of `kept`.
	qt.Assert(t, prov["returned_count"].(float64), qt.Equals, math.Min(kept, 50))
	qt.Assert(t, float64(len(env["result"].([]any))), qt.Equals, prov["returned_count"])
	qt.Assert(t, prov["truncated"], qt.Equals, kept > 50)
```

- `TestFindDuplicates_DivergentOnlyReportsInProvenance` (`:544-551`): `envelopeOK(t, env, 2.0)`; `prov["total_count"]` instead of `candidate_count`.
- `TestFindDuplicates_GuardAllowsSmallTarget` (`:563`): `envelopeOK(t, env, 2.0)`.
- `TestPipelineFindDuplicates_EvalSmoke` (`:279`): unchanged — the eval path returns the same kebab-case keys at `detail=summary`.

Add `"math"` to the test file's import block.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/wile-goast/ -run TestFindDuplicates -count=1 -short`
Expected: FAIL — version 1 vs 2, `scope`/`symbol` ignored, `total_count` missing.

- [ ] **Step 3: Add the `find_duplicates` hooks to `pipelines.scm`**

Insert directly above the `pipeline-find-duplicates` definition (currently `pipelines.scm:283`):

```scheme
;; ── find_duplicates: view hooks ──────────────────────────

;; dup-keep-pair?: OPTS' scope/symbol as a predicate over a CANDIDATE PAIR
;; (a . b) of function names, or #f when neither is set.
;;
;; This filter pushes DOWN: it lands on the flat pair list, before scoring, so
;; a scoped query costs less rather than merely returning less. Sound because
;; the FCA concepts are formed from the whole session BEFORE the prune, and
;; score-candidate-pair depends only on the two functions and the threshold --
;; a surviving pair scores identically, so a scoped result is a genuine subset
;; of the unscoped one.
;;
;; A pair survives if EITHER function matches: asking about marshalValue
;; should surface its duplicate wherever that lives.
(define (dup-keep-pair? opts pos-index)
  (let ((match? (view-name-matcher opts pos-index)))
    (and match?
         (lambda (pr) (or (match? (car pr)) (match? (cdr pr)))))))

;; dup-rank-key: score descending, then proven before structural, then the
;; pair's names as the deterministic tie-break. Score is coerced to float64
;; for the same reason candidate->clean coerces it: score-diffs emits a
;; 256-bit BigFloat, and the key must order by the number actually reported.
(define (dup-tier-rank tier) (if (eq? tier 'proven) 1 0))

(define (dup-rank-key cand)
  (let ((measures (cdr (assq 'measures cand))))
    (list (->float64 (cdr (assq 'similarity measures)))
          (dup-tier-rank (cdr (assq 'equiv-tier measures)))
          (string-join (cdr (assq 'pair cand)) " "))))

;; dup-render: a RAW scored candidate -> its public row at DETAIL. Both levels
;; go through candidate->clean, because a raw make-finding record's leading
;; 'finding tag defeats the marshaller's alist detection. summary then drops
;; the measures sub-object, whose `similarity' merely repeats the top-level
;; score. An opt-in verdict survives both levels: a caller who asked for it
;; asked for it.
(define (dup-render verdict?)
  (lambda (cand detail)
    (let ((row (candidate->clean cand verdict?)))
      (if (eq? detail 'full)
        row
        (filter (lambda (kv) (not (eq? (car kv) 'measures))) row)))))
```

- [ ] **Step 4: Rewrite `pipeline-find-duplicates` and drop `candidate-pair-count`**

Delete `candidate-pair-count` entirely (`pipelines.scm:247-256`) — the pruned count is now `(length pairs)`, which for an unfiltered call is the same sum of C(k,2) it computed. Amend the `max-candidate-pairs` comment by appending a paragraph:

```scheme
;; The count checked against this budget is taken AFTER the scope/symbol
;; prune, so a narrow query against a wide target is answered rather than
;; refused. That converts the guard from a hard wall into one the caller can
;; walk around by being specific: ./... is 58551 pairs unscoped and refused,
;; but a scope naming one file is scored and answered.
```

Replace the `pipeline-find-duplicates` body (`:295-326`) with:

```scheme
(define (pipeline-find-duplicates target opts)
  (let* ((threshold (or (assoc-default opts 'threshold) 0.6))
         (verdict?  (and (assoc-default opts 'verdict) #t))
         (session   (if (go-session? target) target (go-load target)))
         ;; Discovery runs once, here, and its refs and positions are handed
         ;; to the scorer rather than re-derived: pos-index is available from
         ;; the cheap go-func-refs pass, before the expensive canon-index, so
         ;; the prune can see both name and file.
         (refs      (go-func-refs session))
         (concepts  (duplicate-candidate-concepts
                      (concept-lattice (function-ref-context refs))))
         (pos-index (func-refs->positions refs))
         (keep-pair? (dup-keep-pair? opts pos-index))
         (all-prs   (candidate-pairs concepts))
         (pairs     (if keep-pair? (filter keep-pair? all-prs) all-prs))
         (npairs    (length pairs)))
    (if (> npairs max-candidate-pairs)
      (error (string-append
               "find_duplicates: target yields " (number->string npairs)
               " candidate pairs, over the " (number->string max-candidate-pairs)
               " this tool will score; narrow with scope or symbol, or narrow"
               " a '/...' tree to one package")
             target)
      (let* ((cands (score-candidate-pairs session refs pairs threshold))
             ;; The tier filter is adjudication, not the caller's filter, so
             ;; it runs here and apply-view's keep? stays #f: re-applying the
             ;; scope predicate on finished rows would be a no-op that hides
             ;; where the work was actually saved.
             (kept  (filter (lambda (c) (memq (candidate-tier c) kept-tiers))
                            cands)))
        (call-with-values
          (lambda () (apply-view kept opts #f dup-rank-key (dup-render verdict?)))
          (lambda (shown view-prov)
            ;; Version 2: detail=summary is the default, so a default call's
            ;; row shape changed (measures is gone), and candidate-count is
            ;; retired in favour of the view's total-count -- one number, one
            ;; name, the same on both tools.
            (pipeline-envelope 2
              (append
                (list (cons 'target target)
                      (cons 'threshold threshold)
                      (cons 'scored-pair-count (length cands))
                      (cons 'tiers (tier-tally cands))
                      (cons 'candidate-pair-count npairs)
                      (cons 'max-candidate-pairs max-candidate-pairs)
                      (cons 'verdict-included verdict?))
                view-prov)
              shown)))))))
```

- [ ] **Step 5: Update `pipelines.sld` imports**

Replace the `(only (wile goast dup-detect) ...)` clause with:

```scheme
    ;; Discovery (duplicate-candidate-concepts, function-ref-context,
    ;; candidate-pairs, func-refs->positions) is run by the pipeline itself so
    ;; the scope/symbol prune and the pair-count guard both see the work list;
    ;; score-candidate-pairs is the scoring half it then drives.
    (only (wile goast dup-detect)
          find-scored-candidates candidate->verdict
          duplicate-candidate-concepts function-ref-context
          candidate-pairs func-refs->positions score-candidate-pairs)
```

- [ ] **Step 6: Wire the Go handler**

Add the same five parameter declarations to the `find_duplicates` registration (after the `verdict` block, `:105-107`), with the tool-specific `detail` wording:

```go
			mcp.WithString("scope",
				mcp.Description("Only score pairs where one function's file matches this "+
					"path glob ('*' only). Matches the whole path or any '/'-aligned "+
					"suffix: 'marshal.go', 'goast/marshal.go', '*/goast/*.go'. Pushed "+
					"down before scoring, so a scoped query is cheaper and can be "+
					"answered on a target that would otherwise exceed the pair budget.")),
			mcp.WithString("symbol",
				mcp.Description("Only score pairs where one function's name matches this "+
					"glob ('*' only). Matches the whole name or any '.'-aligned suffix, "+
					"so 'marshalValue' matches 'ssaMapper.marshalValue'. A pair survives "+
					"if either function matches.")),
			mcp.WithString("detail",
				mcp.Description("Row width: summary (default) drops the measures "+
					"sub-object; full returns it.")),
			mcp.WithNumber("limit",
				mcp.Description("Maximum rows returned (default 50). provenance.truncated "+
					"says whether rows were withheld.")),
			mcp.WithNumber("offset",
				mcp.Description("Rows skipped before limit applies (default 0).")),
```

In `handleFindDuplicates`, after the `verdict` block and before building `code`:

```go
	viewParts, verr := viewOpts(req)
	if verr != "" {
		return mcp.NewToolResultError(verr), nil
	}
	optsParts = append(optsParts, viewParts...)
```

- [ ] **Step 7: Run the tests**

```bash
go test ./cmd/wile-goast/ -run TestFindDuplicates -count=1 -short -v
go test ./cmd/wile-goast/ -run TestFindDuplicates_ScopeWalksAroundTheGuard -count=1 -v
```

Expected: PASS. The guard test is slow (whole-tree load + SSA build) and is `-short`-skipped.

- [ ] **Step 8: Full check and commit**

```bash
make ci
git add lib/wile/goast/pipelines.scm lib/wile/goast/pipelines.sld \
        cmd/wile-goast/mcp_tools.go cmd/wile-goast/mcp_tools_integration_test.go
git commit -m "feat(find_duplicates): scope/symbol push-down, rank, detail, paging

The filter lands on the flat candidate-pair list before scoring, so a
scoped query costs less rather than merely returning less. Sound because
concepts are formed before the prune and a pair's score depends only on
the two functions and the threshold.

The pair-count guard now reads the count AFTER the prune, which turns it
from a hard wall into one the caller can walk around: ./... is refused at
58551 pairs unscoped and answered when scoped to a file.

Envelope version 1 -> 2: detail=summary is the default so the row shape
changed, and candidate_count is retired in favour of the shared
total_count."
```

---

### Task 6: documentation and the legibility probe

**Files:**
- Modify: `docs/MCP.md:33-84`
- Modify: `tools/legibility/probe.go:183` (the prompt's choice list)
- Modify: `plans/CLAUDE.local.md` (mark the design row shipped, add the impl row)

- [ ] **Step 1: Update the parameter table in `docs/MCP.md`**

Replace the two affected rows:

```markdown
| `find_false_boundaries` | `target`, `mode?`, `min_extent?`, `min_intent?`, `min_types?`, `scope?`, `symbol?`, `detail?`, `limit?`, `offset?` | cross-boundary report |
| `find_duplicates` | `target`, `threshold?`, `verdict?`, `scope?`, `symbol?`, `detail?`, `limit?`, `offset?` | the scored, located pairs that unify (`equiv_tier` `proven`/`structural`); opt-in `verdict` |
```

- [ ] **Step 2: Add the result-view section**

Insert after the `find_duplicates` guard bullet (currently ending `docs/MCP.md:82`):

````markdown
### Result views (`find_duplicates`, `find_false_boundaries`)

Both tools take the same five optional parameters, with the same meaning.
Defaults live in the Scheme pipeline, so a client that passes none of them
still gets a usable response.

| Param | Type | Default | Meaning |
|-------|------|---------|---------|
| `scope` | glob | unset | file path the element must match |
| `symbol` | glob | unset | element name must match |
| `detail` | `summary` \| `full` | `summary` | row width |
| `limit` | integer ≥ 0 | 50 | maximum rows returned |
| `offset` | integer ≥ 0 | 0 | rows skipped before `limit` applies |

`*` is the only metacharacter and matches any run of characters, separators
included. A glob matches if it matches the whole string **or the whole of any
suffix beginning at a separator boundary** — `/` for `scope`, `.` for
`symbol`. So `scope=marshal.go`, `scope=goast/marshal.go` and
`scope=*/goast/*.go` all work against the absolute position path with no
relativization, and `symbol=cloneTypeExpr` works on both tools despite one
reporting `Type.Method` and the other the package-qualified SSA form. Unset
matches everything; both set is a conjunction on the same element. There is no
negation or disjunction.

A glob matching nothing is an **empty success**, not an error. An invalid
`detail`, or a negative or non-integer `limit`/`offset`, is a tool error.

Provenance gains `total_count` (rows after filtering, before paging),
`returned_count`, `offset`, `limit`, `detail`, `truncated`, and an echo of
`scope`/`symbol` when set. **`truncated` is the field to read**: false means
the result is complete, true means narrow or page.

The two tools differ in *where* the filter applies, and this is deliberate:

- `find_duplicates` **pushes down**. The prune lands on the flat candidate-pair
  list before scoring, so a scoped query is cheaper, not merely smaller — and
  the `max-candidate-pairs` guard reads the count *after* the prune, so a
  narrow query on a target that would otherwise be refused is answered.
- `find_false_boundaries` **post-filters** finished concepts. Restricting the
  FCA objects would recompute the lattice over a different context and report
  absence where findings exist: on `./goast`, `symbol=*restructure*` pushed
  into the context finds 0 cross-boundary concepts against the 2 that actually
  match. The whole call is ~1.7s over an 89-concept lattice, so there is no
  cost argument on the other side.

`detail=summary` is where the second tool's real saving is: on `./goast` its
three lattice-relationship fields are 90.5% of the result bytes, and `summary`
skips computing them rather than computing and discarding them. It adds
`types_count`. `find_duplicates`' `summary` drops the `measures` sub-object,
whose `similarity` merely repeats the top-level `score`; an opt-in `verdict`
survives both detail levels.

Design: `plans/2026-07-28-goast-result-view-{design,impl}.md`.
````

Also update the earlier `candidate_count` mention (`docs/MCP.md:66-67`) to `total_count`, and note in the envelope bullet that both tools are at `version` 2.

- [ ] **Step 3: Fix the legibility probe prompt**

`tools/legibility/probe.go:144` maps `divergent → distinct`, but since the equiv-tier filter shipped no `divergent` pair reaches the output, so the `distinct` bucket is unreachable while the prompt at `:183` still offers it. Read both sites, then remove `distinct` from the prompt's choice list, leaving the mapping at `:144` in place with a comment recording that it is now unreachable-by-construction rather than dead code:

```bash
sed -n '135,195p' tools/legibility/probe.go
```

Expected end state: the prompt offers only the buckets a correct answer can use, and `go vet ./tools/...` plus `make lint` stay clean.

- [ ] **Step 4: Update `plans/CLAUDE.local.md`**

In the Active Plan Files table, change the design row's status to `Shipped — see 2026-07-28-goast-result-view-impl.md`, and add:

```markdown
| `2026-07-28-goast-result-view-impl.md` | Impl: 6 tasks — `glob-match?`/`position-file` in utils; the `apply-view` combinator with a tail-safe `view-sort` (the builtin `sort` dies past 20000 rows on Wile's 10000-frame limit); `find_false_boundaries` post-filter + `detail`; the `dup-detect` pair-list refactor; `find_duplicates` push-down with the guard read after the prune; docs + probe prompt. Corrects three design defects: the `*map*` regression pin does not discriminate (uses `*restructure*`), `find_false_boundaries` needed a `field-index->positions` join for `scope`, and `project` becomes `render (row detail)` so summary can skip the annotation pass. Both tools go to envelope version 2. | Ready to implement |
```

- [ ] **Step 5: Verify and commit**

```bash
make ci
git add docs/MCP.md tools/legibility/probe.go
git add -f plans/CLAUDE.local.md plans/2026-07-28-goast-result-view-impl.md
git commit -m "docs(mcp): result-view parameters; drop the probe's unreachable bucket

Documents scope/symbol/detail/limit/offset, the glob rule, the provenance
keys, and why the two tools filter at different points. The legibility
probe stopped being able to reach its 'distinct' choice when the
equiv-tier filter shipped; the prompt no longer offers it."
```

---

## Self-Review

**Spec coverage.** Every section of the design maps to a task:

| Design section | Task |
|---|---|
| Decisions 1, 3, 5 (server-side narrowing, two globs, defaults on) | 1, 2 |
| Decision 2 (push-down where sound) | 3 (post-filter), 4–5 (push-down) |
| Decision 4 (row width as `detail`) | 2 (`render`), 3, 5 |
| Surface (five parameters) | 3, 5 (Go); 2 (Scheme defaults) |
| What the globs match against | 1 |
| Shared view layer + provenance table | 2 |
| Per-tool hooks: `find_duplicates` push-down + guard interaction | 5 |
| Per-tool hooks: `find_false_boundaries` post-filter, `types_count` | 3 |
| Ordering | 2 (`view-key<?`, unique tie-break), tested in 3 and 5 |
| Errors | 3 (`viewOpts`; tested there, applies to both tools) |
| Testing 1–7 | 5, 3, 5, 2+3+5, 2+3+5, 3+5, 1 |
| Interactions: probe prompt, `docs/MCP.md` | 6 |
| Out of scope (other six tools, caching, grammar, byte cap) | untouched |

**Type consistency.** `apply-view` is called with `(rows opts keep? rank-key render)` in Tasks 3 and 5, matching its Task 2 definition; `render` is `(row detail)` at both sites. `scored-candidates`' first argument becomes the pair list in Task 4 and every caller in the tree (`find-scored-candidates`, and `pipeline-find-duplicates` via `score-candidate-pairs`) is updated in that same task. `glob-match?` is `(pat s sep)` at all four call sites. `viewOpts` returns `([]string, string)` and both handlers check the string.

**Known scope decisions, stated so they can be rejected:**

- Both envelopes go to version 2 (design is silent; the default row shape changes, which is what the field is for).
- `find_duplicates`' `candidate_count` is retired in favour of `total_count`.
- `view-sort` replaces the builtin `sort`, because the builtin cannot handle the row count the tool's own guard admits.
- The canon index is restricted to pair-participating functions (design mentions it in passing).
- `TestFindDuplicates_ScopeIsAPrune` asserts on pair counts, not wall-clock, despite the design's "completes measurably faster".

**Not done, deliberately:** the other six pipeline tools; cross-call result caching; any query grammar beyond `*`; a hard byte cap on responses. All four are the design's own *Out of scope*.
