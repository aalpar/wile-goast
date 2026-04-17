# C4: Path Algebra on Call Graphs

Semiring-parameterized path computation over call graphs, using `(wile algebra semiring)`.

**Status:** Approved

**Depends on:** `go-callgraph` primitives (existing), `(wile algebra semiring)` (wile v1.13+)

**References:** Tarjan (1981) — algebraic path problem. Design section in `plans/2026-03-25-b3-c2-c6-design.md:214-265`.

---

## Library

`(wile goast path-algebra)` — new Scheme library.

**Files:** `cmd/wile-goast/lib/wile/goast/path-algebra.{sld,scm}`

**Imports:** `(wile algebra semiring)`, `(wile goast utils)`

## API

```scheme
(make-path-analysis semiring call-graph edge-weight)
;; semiring:    from (wile algebra semiring)
;; call-graph:  list of cg-node alists from go-callgraph
;; edge-weight: (cg-edge -> semiring-value), or #f for default (semiring-one)
;; Returns: path-analysis record

(path-query analysis source target)
;; Returns semiring value between source and target.
;; semiring-zero means unreachable.
;; Lazy: computes single-source on first query, caches by source.

(path-query-all analysis source)
;; Returns alist ((name . value) ...) for all nodes reachable from source.
;; Same lazy computation + caching as path-query.

(path-analysis? x)
;; Type predicate.
```

## Data Model

```scheme
(define-record-type <path-analysis>
  (make-path-analysis* semiring adjacency weight-fn cache)
  path-analysis?
  (semiring   pa-semiring)
  (adjacency  pa-adjacency)    ;; alist: name -> ((callee-name . edge) ...)
  (weight-fn  pa-weight-fn)    ;; (edge -> semiring-value)
  (cache      pa-cache))       ;; mutable alist: name -> ((target . value) ...)
```

`make-path-analysis` builds the adjacency alist from the call graph s-expressions once, using `nf` to extract node names and edge fields.

## Algorithm

Single-source generalized Bellman-Ford, parameterized by the semiring:

1. Initialize: `dist[source] = semiring-one`, all others implicit `semiring-zero`
2. Worklist seeded with source
3. For each node popped from worklist, for each outgoing edge to callee:
   - `candidate = semiring-times(dist[node], weight(edge))`
   - `merged = semiring-plus(dist[callee], candidate)`
   - If `merged != dist[callee]`: update dist, add callee to worklist
4. Converges when worklist empties

**Convergence assumption:** The semiring must have no infinite ascending chains under
`plus` (i.e., repeated `plus` on a finite graph stabilizes). Boolean, tropical, and
counting semirings all satisfy this on finite graphs. The library documents this
assumption but does not validate it — user-supplied semirings must satisfy it.

**Complexity:** O(V * E) worst case per source (standard Bellman-Ford). For boolean
semiring, reduces to O(V + E) BFS. Acceptable for call graph sizes in practice.

## Instantiations

### Boolean semiring — reachability

```scheme
(define reach (make-path-analysis (boolean-semiring) cg #f))
(path-query reach "main" "panic")        ;; => #t or #f
(path-query-all reach "main")            ;; => (("main" . #t) ("helper" . #t) ...)
```

Subsumes `go-callgraph-reachable`. The Go primitive stays as a fast path;
the Scheme version is the generic one.

### Tropical semiring — shortest call chains

```scheme
(define depth (make-path-analysis (tropical-semiring) cg (lambda (_) 1)))
(path-query depth "Handler" "DB.Query")  ;; => 3
```

### Counting semiring — path count

```scheme
(define paths (make-path-analysis (counting-semiring) cg (lambda (_) 1)))
(path-query paths "main" "log")          ;; => 7  (seven distinct call paths)
```

## Relationship to Existing Code

`go-callgraph-reachable` (`goastcg/prim_callgraph.go:378-399`) is the Go-native
boolean-reachability special case. It stays as-is — the Go BFS on `map[string]Value`
is faster than Scheme list traversal, and existing callers (belief DSL `callers-of`)
depend on it. The Scheme path algebra is the general-purpose version.

## CFL-Reachability

Deferred. Context-sensitive analysis via matched call/return pairs is not
semiring-parameterizable — it requires summary edges (Reps 1998) and a different
algorithm. Better as a separate primitive.

## Testing

1. Boolean semiring vs `go-callgraph-reachable` on testdata — identical results
2. Tropical on linear chain A->B->C: distances 0, 1, 2
3. Tropical on diamond A->B->C, A->C: min(2, 1) = 1
4. Counting on diamond: path count = 2
5. Unreachable node returns `semiring-zero`
6. Cache hit: second query from same source reuses result
7. Custom edge-weight function: weighted tropical
