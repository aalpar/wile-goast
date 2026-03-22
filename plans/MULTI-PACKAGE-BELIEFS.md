# Multi-Package Belief Context

**Status**: Planned
**Foundation**: [BELIEF-DSL.md](BELIEF-DSL.md) — single-package belief evaluation
**Scope**: Scheme-only changes to `belief.scm`

## Problem

The belief DSL evaluates beliefs within a single package's scope. Each `run-beliefs` call creates a context bound to one package pattern, and the SSA index keys functions by short name only. This prevents SSA-based beliefs (`stores-to-fields`, `co-mutated`, `checked-before-use`) from working across packages that share types.

In codebases like the Kubernetes kubelet, types are factored across subpackages:

```
container/     defines ContainerStatus
kuberuntime/   has methods that store to ContainerStatus fields
status/        has methods that store to ContainerStatus fields
```

A co-mutation belief on `ContainerStatus` fields needs to see functions from all three packages simultaneously. The current per-package scoping makes this impossible — `struct-field-names` can't find the struct (defined elsewhere), and the SSA index can't find functions across packages.

### Root Cause

The Go primitives (`go-typecheck-package`, `go-ssa-build`, `go-callgraph`) already support multi-package loading via `...` glob patterns — `packages.Load` handles them natively. The limitation is entirely in the Scheme-side belief DSL:

1. `all-func-decls` extracts func-decl nodes but drops which package each came from
2. `ctx-ssa-index` keys by short name only — collides across packages (e.g., two packages both have `Init`)
3. `ctx-find-ssa-func` looks up by short name only, gets the wrong function on collision

Both layers already carry the needed metadata:
- AST package nodes have a `path` field (full import path)
- SSA function nodes have a `pkg` field (full package path)

### Pre-existing Bug

The `ordered` checker calls `(go-cfg fname)` with one argument, but `go-cfg` requires two (pattern and function name). This has never been exercised — no existing belief uses `ordered`. The fix requires the same site annotation.

## Design

### Approach: Annotate Sites with Package Path

Inject a `pkg-path` field into each func-decl site during extraction. Use it for SSA/CFG lookup. No Go changes.

### Change 1: Site Annotation

`all-func-decls` propagates the package `path` onto each func-decl:

```scheme
(define (all-func-decls pkgs)
  (flat-map
    (lambda (pkg)
      (let ((pkg-path (nf pkg 'path)))
        (flat-map
          (lambda (file)
            (filter-map
              (lambda (decl)
                (and (tag? decl 'func-decl)
                     (cons (car decl)
                           (cons (cons 'pkg-path pkg-path)
                                 (cdr decl)))))
              (let ((decls (nf file 'decls)))
                (if (pair? decls) decls '()))))
          (let ((files (nf pkg 'files)))
            (if (pair? files) files '())))))
    pkgs))
```

Backward-compatible: existing predicates (`nf fn 'name`, `nf fn 'body`, etc.) still work — they `assoc` for their field, ignoring `pkg-path`.

### Change 2: Two-Level SSA Index

The SSA index changes from a flat alist to a two-level structure keyed by `(pkg-path, short-name)`:

```scheme
;; Before: ((short-name . ssa-func) ...)
;; After:  ((pkg-path . ((short-name . ssa-func) ...)) ...)
```

`ctx-ssa-index` groups SSA functions by their `pkg` field:

```scheme
(define (build-ssa-index ssa-funcs)
  (let loop ((fns (if (pair? ssa-funcs) ssa-funcs '()))
             (index '()))
    (if (null? fns) index
      (let* ((fn (car fns))
             (name (nf fn 'name))
             (pkg (nf fn 'pkg))
             (short (and name (ssa-short-name name))))
        (if (and short pkg)
          (let ((pkg-entry (assoc pkg index)))
            (if pkg-entry
              (begin
                (set-cdr! pkg-entry
                  (cons (cons short fn) (cdr pkg-entry)))
                (loop (cdr fns) index))
              (loop (cdr fns)
                    (cons (list pkg (cons short fn)) index))))
          (loop (cdr fns) index))))))
```

### Change 3: Package-Qualified SSA Lookup

`ctx-find-ssa-func` takes both package path and name:

```scheme
(define (ctx-find-ssa-func ctx pkg-path name)
  (let ((pkg-entry (assoc pkg-path (ctx-ssa-index ctx))))
    (and pkg-entry
         (let ((entry (assoc name (cdr pkg-entry))))
           (and entry (cdr entry))))))
```

All callers extract `pkg-path` from the annotated site:

```scheme
;; stores-to-fields, co-mutated, checked-before-use all follow this pattern:
(let* ((fname (nf fn 'name))
       (pkg-path (nf fn 'pkg-path))
       (ssa-fn (ctx-find-ssa-func ctx pkg-path fname)))
  ...)
```

### Change 4: Fix `ordered` Checker

Pass the site's package path as the pattern argument to `go-cfg`:

```scheme
(define (ordered op-a op-b)
  (lambda (site ctx)
    (let* ((fname (nf site 'name))
           (pkg-path (nf site 'pkg-path))
           (cfg (go-cfg pkg-path fname)))
      ...)))
```

### Change 5: Package-Qualified Display Names

`site-display-name` includes the short package name for locatability:

```scheme
;; Before: "startContainer"
;; After:  "kuberuntime.startContainer"
```

A helper extracts the last path segment:

```scheme
(define (package-short-name path)
  (let ((len (string-length path)))
    (let loop ((i (- len 1)))
      (cond ((<= i 0) path)
            ((char=? (string-ref path i) #\/)
             (substring path (+ i 1) len))
            (else (loop (- i 1)))))))
```

### What Doesn't Change

- **`run-beliefs` signature** — still `(run-beliefs target-string)`. Pass `"k8s.io/.../kubelet/..."` for multi-package.
- **Go primitives** — no changes. `go-typecheck-package`, `go-ssa-build`, `go-callgraph` already handle `...` patterns.
- **AST-only predicates** — `contains-call`, `has-params`, `has-receiver`, `name-matches` work unchanged.
- **`callers-of`** — returns call-graph edge sites, not func-decl sites. Call graph is already cross-package.
- **`struct-field-names`** — searches all packages in `ctx-pkgs`. With `...` patterns, finds structs across all loaded packages.
- **`define-belief` macro** — unchanged.
- **Threshold and statistical comparison** — unchanged. Sites from all packages pool together.

## Usage After

Single-package beliefs work exactly as before:

```scheme
(run-beliefs "my/package")
```

Multi-package beliefs use `...` patterns:

```scheme
(define-belief "container-status-comutation"
  (sites (functions-matching
           (stores-to-fields "ContainerStatus" "State" "Reason" "Message")))
  (expect (co-mutated "State" "Reason" "Message"))
  (threshold 0.66 3))

(run-beliefs "k8s.io/kubernetes/pkg/kubelet/...")
```

Output includes package-qualified names:

```
-- Belief: container-status-comutation --
  Pattern: co-mutated (12/14 sites)
    DEVIATION: kuberuntime.syncPodStatus -> partial
    DEVIATION: status.updateCachedStatus -> partial
```

## Non-Goals

- Per-package output grouping (presentation concern; add later if wanted)
- Caching SSA across `run-beliefs` calls (context is per-call)
- Lazy per-package SSA loading within a single context
- Changes to Go primitives or s-expression format

## Testing

1. **Existing single-package beliefs** — run the wile `belief-comutation.scm` example, verify identical output.
2. **Multi-package k8s beliefs** — convert `k8s-kubelet-beliefs.scm` from per-package loop to single `(run-beliefs "k8s.io/kubernetes/pkg/kubelet/...")` call, verify results match.
3. **SSA-based k8s belief** — add a `stores-to-fields`/`co-mutated` belief targeting a type shared across kubelet subpackages, verify it finds sites across packages.
4. **Name collision** — verify that two packages with same-named functions resolve to the correct SSA functions.
