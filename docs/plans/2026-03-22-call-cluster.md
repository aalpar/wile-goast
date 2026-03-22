# Call-Set Clustering — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** A Scheme script that clusters methods on a receiver type by call-set similarity, revealing subgroups within types that lack conventions.

**Architecture:** Single-file Scheme script. Reuses call extraction from the mining script. Computes pairwise Jaccard similarity, then greedy agglomerative clustering. Reports top pairs, clusters with core call sets, and unclustered methods.

**Tech Stack:** wile-goast (Scheme + Go AST), `(wile goast utils)`, `go-typecheck-package`

**Design:** `docs/plans/2026-03-22-call-cluster-design.md`

---

### Task 1: Script Skeleton with Call Extraction

**Files:**
- Create: `examples/goast-query/call-cluster.scm`

**Step 1: Write the script with configuration, imports, and call extraction**

The call extraction logic is identical to the mining script. Reuse the same
functions: `receiver-type-name`, `callee-name`, `extract-callees`.

```scheme
;;; call-cluster.scm — Method clustering by call-set similarity
;;;
;;; Clusters methods on a receiver type by Jaccard similarity of their
;;; call sets. Reveals subgroups within types that lack conventions.
;;;
;;; Usage: wile-goast -f examples/goast-query/call-cluster.scm

(import (wile goast utils))

;; ── Configuration ────────────────────────────────────────
(define target "github.com/aalpar/wile/machine")
(define target-type "CompileTimeContinuation")
(define min-cluster-similarity 0.30)

;; ── Call Extraction (same as call-convention-mine.scm) ───

(define (receiver-type-name func)
  (let ((recv (nf func 'recv)))
    (and recv (pair? recv)
         (let* ((recv-field (car recv))
                (recv-type (nf recv-field 'type))
                (base-type (if (tag? recv-type 'star-expr)
                             (nf recv-type 'x)
                             recv-type)))
           (cond
             ((tag? base-type 'ident) (nf base-type 'name))
             ((tag? base-type 'index-expr)
              (let ((x (nf base-type 'x)))
                (and (tag? x 'ident) (nf x 'name))))
             ((tag? base-type 'index-list-expr)
              (let ((x (nf base-type 'x)))
                (and (tag? x 'ident) (nf x 'name))))
             (else #f))))))

(define (callee-name call-node)
  (let ((fun (nf call-node 'fun)))
    (cond
      ((tag? fun 'ident) (nf fun 'name))
      ((tag? fun 'selector-expr) (nf fun 'sel))
      (else #f))))

(define (extract-callees func)
  (let ((body (nf func 'body)))
    (if body
      (unique
        (walk body
          (lambda (node)
            (and (tag? node 'call-expr)
                 (callee-name node)))))
      '())))
```

**Step 2: Add package parsing and method extraction for the target type**

```scheme
;; ── Parse and Extract ────────────────────────────────────

(display "Loading ") (display target) (display " ...") (newline)
(define pkgs (go-typecheck-package target))

;; Extract all func-decls with bodies for the target type
(define target-methods
  (filter-map
    (lambda (func)
      (and (equal? (receiver-type-name func) target-type)
           (nf func 'body)
           func))
    (flat-map
      (lambda (pkg)
        (flat-map
          (lambda (file)
            (filter-map
              (lambda (decl)
                (and (tag? decl 'func-decl) decl))
              (nf file 'decls)))
          (nf pkg 'files)))
      pkgs)))

;; Build call sets, excluding methods with no calls
(define call-sets
  (filter-map
    (lambda (func)
      (let* ((name (nf func 'name))
             (callees (extract-callees func)))
        (and (pair? callees)
             (list name callees))))
    target-methods))

(newline)
(display "══ ") (display target-type)
(display ": Method Clusters ══") (newline)
(display "  ") (display (length call-sets))
(display " methods with calls (of ")
(display (length target-methods)) (display " total)")
(newline) (newline)
```

**Step 3: Verify the skeleton runs**

Run: `./dist/darwin/arm64/wile-goast -f examples/goast-query/call-cluster.scm`
Expected: Shows method count for CompileTimeContinuation (expect ~95 methods with calls).

**Step 4: Commit**

```bash
git add examples/goast-query/call-cluster.scm
git commit -m "feat: add call-cluster script skeleton with call extraction"
```

---

### Task 2: Jaccard Similarity and Pairwise Matrix

**Files:**
- Modify: `examples/goast-query/call-cluster.scm`

**Step 1: Write set operations and Jaccard computation**

```scheme
;; ── Jaccard Similarity ───────────────────────────────────

;; Count elements in A that are also in B
(define (set-intersect-count a b)
  (let loop ((xs a) (count 0))
    (if (null? xs) count
      (loop (cdr xs)
            (if (member? (car xs) b) (+ count 1) count)))))

;; |A ∪ B| = |A| + |B| - |A ∩ B|
(define (jaccard a b)
  (let* ((inter (set-intersect-count a b))
         (union (- (+ (length a) (length b)) inter)))
    (if (= union 0) 0.0
      (exact->inexact (/ inter union)))))
```

**Step 2: Compute all pairwise similarities and sort**

```scheme
;; ── Pairwise Matrix ──────────────────────────────────────

;; Insertion sort a triple into a descending-by-similarity list
(define (insert-by-sim entry sorted)
  (cond
    ((null? sorted) (list entry))
    ((>= (caddr entry) (caddr (car sorted)))
     (cons entry sorted))
    (else (cons (car sorted) (insert-by-sim entry (cdr sorted))))))

;; Compute all pairs, sorted by similarity descending
(define (all-pairs call-sets)
  (let ((v (list->vector call-sets))
        (n (length call-sets)))
    (let outer ((i 0) (sorted '()))
      (if (>= i n) sorted
        (let inner ((j (+ i 1)) (acc sorted))
          (if (>= j n)
            (outer (+ i 1) acc)
            (let* ((a (vector-ref v i))
                   (b (vector-ref v j))
                   (sim (jaccard (cadr a) (cadr b))))
              (if (> sim 0.0)
                (inner (+ j 1)
                       (insert-by-sim (list (car a) (car b) sim) acc))
                (inner (+ j 1) acc)))))))))

(define sim-pairs (all-pairs call-sets))

(display "  ") (display (length sim-pairs))
(display " pairs with non-zero similarity") (newline) (newline)
```

**Step 3: Display top 10 pairs**

```scheme
;; ── Top Similarity Pairs ─────────────────────────────────
(display "── Top 10 Similarity Pairs ──") (newline)
(let loop ((ps sim-pairs) (i 0))
  (if (or (null? ps) (>= i 10)) 'done
    (let ((p (car ps)))
      (display "  ") (display (car p))
      (display " <-> ") (display (cadr p))
      (display "  ") (display (caddr p))
      (newline)
      (loop (cdr ps) (+ i 1)))))
(newline)
```

**Step 4: Run and verify**

Run: `./dist/darwin/arm64/wile-goast -f examples/goast-query/call-cluster.scm`
Expected: Top 10 similarity pairs with scores between 0 and 1.
The highest-similarity pair should be two methods you'd expect to be
similar (e.g., two compile* methods for related forms).

**Step 5: Commit**

```bash
git add examples/goast-query/call-cluster.scm
git commit -m "feat(cluster): pairwise Jaccard similarity and top pairs"
```

---

### Task 3: Greedy Agglomerative Clustering

**Files:**
- Modify: `examples/goast-query/call-cluster.scm`

**Step 1: Write the clustering algorithm**

Each cluster is stored as `(cluster-id (member ...))` in a flat list.
A separate alist maps method names to cluster IDs (or #f if unclustered).

```scheme
;; ── Greedy Clustering ────────────────────────────────────

;; Find which cluster a method belongs to, or #f
(define (find-cluster method-name clusters)
  (let loop ((cs clusters))
    (cond
      ((null? cs) #f)
      ((member? method-name (cadar cs)) (car cs))
      (else (loop (cdr cs))))))

;; Average similarity of a method to all members of a cluster
(define (avg-sim-to-cluster method-name cluster-members call-set-index)
  (let* ((method-callees (let ((e (assoc method-name call-set-index)))
                           (if e (cadr e) '())))
         (sims (filter-map
                 (lambda (member-name)
                   (let ((member-callees
                           (let ((e (assoc member-name call-set-index)))
                             (if e (cadr e) '()))))
                     (jaccard method-callees member-callees)))
                 cluster-members))
         (n (length sims)))
    (if (= n 0) 0.0
      (/ (apply + sims) n))))

;; Main clustering loop
(define (cluster-methods sim-pairs call-sets min-sim)
  (let ((index call-sets))  ;; call-sets is already ((name (callees)) ...)
    (let loop ((pairs sim-pairs) (clusters '()) (next-id 0))
      (if (null? pairs)
        clusters
        (let* ((pair (car pairs))
               (a (car pair))
               (b (cadr pair))
               (sim (caddr pair))
               (ca (find-cluster a clusters))
               (cb (find-cluster b clusters)))
          (cond
            ;; Both unclustered: create new cluster
            ((and (not ca) (not cb))
             (loop (cdr pairs)
                   (cons (list next-id (list a b)) clusters)
                   (+ next-id 1)))

            ;; A in cluster, B unclustered: try adding B
            ((and ca (not cb))
             (let ((avg (avg-sim-to-cluster b (cadr ca) index)))
               (if (>= avg min-sim)
                 (begin
                   (set-cdr! ca (list (cons b (cadr ca))))
                   (loop (cdr pairs) clusters next-id))
                 (loop (cdr pairs) clusters next-id))))

            ;; B in cluster, A unclustered: try adding A
            ((and (not ca) cb)
             (let ((avg (avg-sim-to-cluster a (cadr cb) index)))
               (if (>= avg min-sim)
                 (begin
                   (set-cdr! cb (list (cons a (cadr cb))))
                   (loop (cdr pairs) clusters next-id))
                 (loop (cdr pairs) clusters next-id))))

            ;; Both in clusters (same or different): skip
            (else
             (loop (cdr pairs) clusters next-id))))))))

(define clusters (cluster-methods sim-pairs call-sets min-cluster-similarity))
```

Note: uses `set-cdr!` to mutate cluster member lists in place (confirmed
working in wile). The `find-cluster` returns the actual pair, so `set-cdr!`
modifies it in the `clusters` list.

**Step 2: Verify clustering produces results**

Add temporary output:

```scheme
(display "── Clustering ──") (newline)
(display "  ") (display (length clusters)) (display " clusters formed")
(newline)
(for-each
  (lambda (c)
    (display "  Cluster ") (display (car c))
    (display ": ") (display (length (cadr c)))
    (display " members") (newline))
  clusters)
(newline)
```

Run: `./dist/darwin/arm64/wile-goast -f examples/goast-query/call-cluster.scm`
Expected: At least 2 clusters. If 0 clusters, the threshold may be too high
— try lowering `min-cluster-similarity` to 0.20.

**Step 3: Commit**

```bash
git add examples/goast-query/call-cluster.scm
git commit -m "feat(cluster): greedy agglomerative clustering"
```

---

### Task 4: Cluster Characterization and Final Report

**Files:**
- Modify: `examples/goast-query/call-cluster.scm`

**Step 1: Write core call set computation**

The core call set is the intersection of all members' call sets — callees
that every member calls.

```scheme
;; ── Cluster Characterization ─────────────────────────────

;; Intersection of all call sets for cluster members
(define (core-calls cluster-members call-set-index)
  (let* ((member-callees
           (filter-map
             (lambda (name)
               (let ((e (assoc name call-set-index)))
                 (if e (cadr e) #f)))
             cluster-members)))
    (if (null? member-callees) '()
      (let loop ((candidates (car member-callees))
                 (rest (cdr member-callees)))
        (if (null? rest) candidates
          (loop (filter-map
                  (lambda (c)
                    (and (member? c (car rest)) c))
                  candidates)
                (cdr rest)))))))
```

**Step 2: Write the full report replacing temporary output**

Replace the temporary clustering output with:

```scheme
;; ── Report ───────────────────────────────────────────────

;; Sort clusters by size descending
(define sorted-clusters
  (let loop ((cs clusters) (sorted '()))
    (if (null? cs) sorted
      (let insert ((c (car cs)) (acc '()) (rest sorted))
        (cond
          ((null? rest)
           (loop (cdr cs) (reverse (cons c acc))))
          ((>= (length (cadr c)) (length (cadr (car rest))))
           (loop (cdr cs) (append (reverse (cons c acc)) rest)))
          (else
           (insert c (cons (car rest) acc) (cdr rest))))))))

;; Count clustered methods
(define clustered-count
  (apply + (map (lambda (c) (length (cadr c))) sorted-clusters)))
(define unclustered-count (- (length call-sets) clustered-count))

(display "  Clusters: ") (display (length sorted-clusters))
(display " (") (display clustered-count) (display " methods)")
(display ", Unclustered: ") (display unclustered-count)
(newline) (newline)

;; Print each cluster
(let loop ((cs sorted-clusters) (i 1))
  (if (pair? cs)
    (let* ((c (car cs))
           (members (cadr c))
           (core (core-calls members call-sets)))
      (display "── Cluster ") (display i)
      (display " (") (display (length members))
      (display " methods) ──") (newline)
      (display "  Core calls: ")
      (if (pair? core)
        (let show ((xs core) (first #t))
          (if (pair? xs)
            (begin
              (if (not first) (display ", "))
              (display (car xs))
              (show (cdr xs) #f))))
        (display "(none)"))
      (newline)
      (display "  Members:") (newline)
      (let ((wrapped (wrap-names members 60)))
        (for-each
          (lambda (line)
            (display "    ") (display line) (newline))
          wrapped))
      (newline)
      (loop (cdr cs) (+ i 1)))))

;; Unclustered methods
(define unclustered
  (filter-map
    (lambda (cs)
      (and (not (find-cluster (car cs) clusters))
           (car cs)))
    call-sets))

(if (pair? unclustered)
  (begin
    (display "── Unclustered (") (display (length unclustered))
    (display " methods) ──") (newline)
    (let ((wrapped (wrap-names unclustered 60)))
      (for-each
        (lambda (line)
          (display "    ") (display line) (newline))
        wrapped))
    (newline)))
```

Note: `wrap-names` needs to be defined in this script (same logic as
in the mining script):

```scheme
(define (wrap-names names width)
  (let loop ((ns names) (line "") (lines '()))
    (cond
      ((null? ns)
       (reverse (if (> (string-length line) 0)
                  (cons line lines)
                  lines)))
      (else
       (let* ((name (car ns))
              (sep (if (> (string-length line) 0) ", " ""))
              (candidate (string-append line sep name)))
         (if (and (> (string-length line) 0)
                  (> (string-length candidate) width))
           (loop ns "" (cons line lines))
           (loop (cdr ns) candidate lines)))))))
```

**Step 3: Run the complete script**

Run: `./dist/darwin/arm64/wile-goast -f examples/goast-query/call-cluster.scm`
Expected: Full report with top pairs, named clusters with core call sets,
and unclustered methods. Review: do clusters make semantic sense?

**Step 4: Commit**

```bash
git add examples/goast-query/call-cluster.scm
git commit -m "feat(cluster): characterization and full report"
```

---

### Task 5: Validate and Tune

**Files:**
- Modify: `examples/goast-query/call-cluster.scm` (threshold only)

**Step 1: Assess output quality**

Run: `./dist/darwin/arm64/wile-goast -f examples/goast-query/call-cluster.scm`

Evaluate:
- Do clusters correspond to recognizable compiler subsystems?
- Are core call sets non-trivial (≥2 callees)?
- Are unclustered methods genuinely dissimilar?
- If clusters are too large (>30 members): raise `min-cluster-similarity`
- If too many singletons (>50%): lower `min-cluster-similarity`

**Step 2: Adjust threshold and rerun**

**Step 3: Commit final threshold**

```bash
git add examples/goast-query/call-cluster.scm
git commit -m "chore(cluster): tune threshold from CTC validation"
```
