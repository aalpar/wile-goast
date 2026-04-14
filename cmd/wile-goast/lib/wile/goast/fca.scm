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

;;; fca.scm — Formal Concept Analysis for false boundary detection
;;;
;;; Core FCA operations (sorted sets, context, Galois connection, concept
;;; lattice, algebra bridge) are provided by (wile algebra fca).
;;; This module adds Go-specific bridges: SSA field index → FCA context,
;;; call graph propagation, and cross-boundary detection.

;;; ── SSA bridge ───────────────────────────────────────────

;; Find the index of char c in string s, or #f if not found.
(define (string-index-of s c)
  (let ((len (string-length s)))
    (let loop ((i 0))
      (cond ((>= i len) #f)
            ((char=? (string-ref s i) c) i)
            (else (loop (+ i 1)))))))

;; Build an attribute string from a field-access node based on mode.
;; Returns a string or #f if the access doesn't match the mode filter.
(define (field-access-attr access mode)
  (let* ((struct-name (nf access 'struct))
         (field-name (nf access 'field))
         (access-mode (nf access 'mode))
         (base (string-append struct-name "." field-name)))
    (case mode
      ((write-only)
       (and (eq? access-mode 'write) base))
      ((read-write)
       (string-append base (if (eq? access-mode 'write) ":w" ":r")))
      ((type-only)
       struct-name)
      (else (error "field-index->context: unknown mode" mode)))))

;; Count distinct struct types in an attribute list.
(define (attr-type-count attrs)
  (length (unique (map attr-struct-name attrs))))

;; Convert go-ssa-field-index output to an FCA context.
;; mode: 'write-only, 'read-write, or 'type-only.
;; Optional: pass 'cross-type-only after mode to keep only functions
;; that access fields from 2+ struct types. This pre-filter dramatically
;; reduces context size for large codebases.
(define (field-index->context index mode . opts)
  "Convert go-ssa-field-index output to an FCA context.\nMODE controls attribute encoding: 'write-only, 'read-write, or 'type-only.\nOptional 'cross-type-only pre-filters to functions accessing 2+ struct types.\n\nParameters:\n  index : list\n  mode : symbol\nReturns: list\nCategory: goast-fca\n\nExamples:\n  (field-index->context (go-ssa-field-index \"./...\") 'write-only)\n  (field-index->context idx 'write-only 'cross-type-only)\n\nSee also: `go-ssa-field-index', `concept-lattice'."
  (let* ((cross-only (and (pair? opts) (eq? (car opts) 'cross-type-only)))
         (entries
           (filter-map
             (lambda (summary)
               (let* ((qualified (nf summary 'func))
                      (fields (nf summary 'fields))
                      (attrs (unique
                               (filter-map
                                 (lambda (acc) (field-access-attr acc mode))
                                 (if (pair? fields) fields '())))))
                 (if (null? attrs) #f
                   (if (and cross-only (< (attr-type-count attrs) 2))
                     #f
                     (cons qualified attrs)))))
             index)))
    (context-from-alist entries)))

;;; ── Transitive field write propagation ───────────────────

;; Tail-recursive map via for-each + reverse. Safe for large lists.
(define (map* f lst)
  (let ((acc '()))
    (for-each (lambda (x) (set! acc (cons (f x) acc))) lst)
    (reverse acc)))

;; Tail-recursive filter-map via for-each + reverse. Safe for large lists.
(define (filter-map* f lst)
  (let ((acc '()))
    (for-each (lambda (x) (let ((v (f x))) (if v (set! acc (cons v acc))))) lst)
    (reverse acc)))

;; Build adjacency map from call graph: func-name → list of callee names.
(define (cg-adjacency callgraph)
  (let ((adj (make-hashtable)))
    (for-each
      (lambda (node)
        (let* ((name (nf node 'name))
               (edges (nf node 'edges-out))
               (callees (if (pair? edges)
                          (map* (lambda (e) (nf e 'callee)) edges)
                          '())))
          (hashtable-set! adj name callees)))
      callgraph)
    adj))

;; Iterative DFS topological sort with back-edge detection.
;; Returns (processing-order . back-edges) where:
;;   processing-order: list of func names, callees before callers
;;   back-edges: hashtable of "caller\0callee" → #t
;;
;; Uses an explicit stack to avoid call-depth limits on large graphs.
;; Stack frames are (node . remaining-callees). When remaining-callees
;; is exhausted, the node finishes (black) and is added to topo order.
(define (cg-topo-sort adj all-nodes)
  (let ((color (make-hashtable))
        (back-edges (make-hashtable))
        (topo '())
        (stack '()))
    ;; Push a node onto the DFS stack if unvisited.
    (define (push-if-white node)
      (let ((c (hashtable-ref color node #f)))
        (if (not c)
          (begin
            (hashtable-set! color node 'gray)
            (set! stack (cons (cons node (hashtable-ref adj node '()))
                              stack))))))
    ;; Process the explicit DFS stack until empty.
    (define (run)
      (let loop ()
        (if (null? stack) #f
          (let* ((frame (car stack))
                 (node (car frame))
                 (remaining (cdr frame)))
            (if (null? remaining)
              ;; All callees processed — finish this node.
              (begin
                (hashtable-set! color node 'black)
                (set! topo (cons node topo))
                (set! stack (cdr stack))
                (loop))
              ;; Process next callee.
              (let ((callee (car remaining)))
                (set-cdr! frame (cdr remaining))
                (let ((cc (hashtable-ref color callee #f)))
                  (cond
                    ((eq? cc 'gray)
                     (hashtable-set! back-edges
                       (string-append node "\x00;" callee) #t))
                    ((not cc)
                     (hashtable-set! color callee 'gray)
                     (set! stack (cons (cons callee (hashtable-ref adj callee '()))
                                       stack)))))
                (loop)))))))
    (for-each (lambda (n) (push-if-white n) (run)) all-nodes)
    ;; topo is reverse-postorder (sources first).
    ;; Reverse it: callees before callers.
    (cons (reverse topo) back-edges)))

;; Extract write-mode field accesses per function.
(define (direct-write-map field-index)
  (let ((m (make-hashtable)))
    (for-each
      (lambda (summary)
        (let* ((fname (nf summary 'func))
               (fields (nf summary 'fields))
               (writes (filter-map
                         (lambda (acc)
                           (and (eq? (nf acc 'mode) 'write) acc))
                         (if (pair? fields) fields '()))))
          (hashtable-set! m fname writes)))
      field-index)
    m))

;; Test whether caller→callee is a back-edge.
(define (back-edge? back-edges caller callee)
  (hashtable-ref back-edges (string-append caller "\x00;" callee) #f))

;; Accumulate transitive writes along DAG edges.
;; processing-order: callees before callers.
;; adj: func → callee names.
;; direct: func → list of write field-accesses.
;; back-edges: hashtable of back-edge keys.
;; Returns: hashtable of func → accumulated field-access list.
(define (accumulate-writes processing-order adj direct back-edges)
  (let ((acc (make-hashtable)))
    (for-each
      (lambda (func)
        (let loop ((callees (hashtable-ref adj func '()))
                   (ws (hashtable-ref direct func '())))
          (if (null? callees)
            (hashtable-set! acc func ws)
            (loop (cdr callees)
                  (if (back-edge? back-edges func (car callees))
                    ws
                    (append (hashtable-ref acc (car callees) '()) ws))))))
      processing-order)
    acc))

;; Rebuild field-index summaries with accumulated field lists.
;; Preserves pkg from originals; includes functions from call graph
;; that had no direct writes but gained transitive writes.
(define (rebuild-summaries field-index all-funcs acc)
  (let ((pkg-map (make-hashtable)))
    (for-each
      (lambda (s) (hashtable-set! pkg-map (nf s 'func) (nf s 'pkg)))
      field-index)
    (filter-map*
      (lambda (fname)
        (let ((fields (hashtable-ref acc fname '())))
          (if (null? fields) #f
            (list 'ssa-field-summary
                  (cons 'func fname)
                  (cons 'pkg (hashtable-ref pkg-map fname ""))
                  (cons 'fields fields)))))
      all-funcs)))

;; Propagate write-mode field accesses transitively along call graph edges.
;; Recursive edges (DFS back-edges) are skipped — recursive functions
;; contribute only their direct writes.
;;
;; field-index: output from go-ssa-field-index
;; callgraph: output from go-callgraph (use 'static)
;; Returns: new field-index with expanded write sets.
(define (propagate-field-writes field-index callgraph)
  (let* ((adj (cg-adjacency callgraph))
         (all-nodes (map* (lambda (n) (nf n 'name)) callgraph))
         (direct (direct-write-map field-index))
         (sorted (cg-topo-sort adj all-nodes))
         (processing-order (car sorted))
         (back-edges (cdr sorted))
         (acc (accumulate-writes processing-order adj direct back-edges)))
    (rebuild-summaries field-index processing-order acc)))

;;; ── Cross-boundary detection ─────────────────────────────

;; Extract struct name from "Struct.Field" or "Struct.Field:r" attribute.
(define (attr-struct-name attr)
  (let ((dot (string-index-of attr #\.)))
    (if dot (substring attr 0 dot) attr)))

;; Look up a key in a symbol-keyed plist, returning default if absent.
;; Plist form: (key1 val1 key2 val2 ...).
(define (plist-ref plist key default)
  (cond ((null? plist) default)
        ((null? (cdr plist)) default)
        ((eq? (car plist) key) (cadr plist))
        (else (plist-ref (cddr plist) key default))))

;; Group attribute strings by struct name.
;; Returns ((struct-name attr ...) ...).
(define (group-fields-by-struct attrs)
  (let loop ((as attrs) (groups '()))
    (if (null? as)
      (map (lambda (g) (cons (car g) (reverse (cdr g))))
           (reverse groups))
      (let* ((attr (car as))
             (sname (attr-struct-name attr))
             (existing (assoc sname groups)))
        (if existing
          (begin (set-cdr! existing (cons attr (cdr existing)))
                 (loop (cdr as) groups))
          (loop (cdr as) (cons (list sname attr) groups)))))))

;; Filter concepts whose intent spans multiple distinct struct types.
;; Optional plist args: 'min-extent N, 'min-intent N, 'min-types N.
(define (cross-boundary-concepts lattice . opts)
  "Filter concepts whose intent spans multiple distinct struct types.\nOptional keyword args: 'min-extent, 'min-intent, 'min-types (all default 2).\n\nParameters:\n  lattice : list\nReturns: list\nCategory: goast-fca\n\nSee also: `concept-lattice', `boundary-report'."
  (let ((min-ext (plist-ref opts 'min-extent 2))
        (min-int (plist-ref opts 'min-intent 2))
        (min-typ (plist-ref opts 'min-types 2)))
    (filter-map
      (lambda (concept)
        (let* ((ext (concept-extent concept))
               (int (concept-intent concept))
               (types (unique (map attr-struct-name int))))
          (and (>= (length ext) min-ext)
               (>= (length int) min-int)
               (>= (length types) min-typ)
               concept)))
      lattice)))

;; Build a structured report for a list of cross-boundary concepts.
;; Returns a list of alists, one per concept.
(define (boundary-report concepts)
  "Build a structured report for cross-boundary concepts.\nEach entry is an alist with: types, fields, functions, extent-size.\n\nParameters:\n  concepts : list\nReturns: list\nCategory: goast-fca\n\nSee also: `cross-boundary-concepts'."
  (map (lambda (concept)
         (let* ((ext (concept-extent concept))
                (int (concept-intent concept))
                (types (unique (map attr-struct-name int)))
                (grouped (group-fields-by-struct int)))
           (list (cons 'types types)
                 (cons 'fields grouped)
                 (cons 'functions ext)
                 (cons 'extent-size (length ext)))))
       concepts))
