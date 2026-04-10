;;; fca.scm — Formal Concept Analysis for false boundary detection
;;;
;;; Discovers natural field groupings from function access patterns
;;; via concept lattice construction (NextClosure, Ganter 1984).
;;; Compares discovered groupings against actual struct boundaries.

;;; ── Sorted string sets (internal) ─────────────────────────

;; Insertion sort with dedup. Returns a sorted list of unique strings.
(define (sort-strings lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) acc
      (loop (cdr xs) (set-add (car xs) acc)))))

;; Add element to a sorted string list, maintaining sort and uniqueness.
(define (set-add elem sorted)
  (cond ((null? sorted) (list elem))
        ((string<? elem (car sorted)) (cons elem sorted))
        ((string=? elem (car sorted)) sorted)
        (else (cons (car sorted) (set-add elem (cdr sorted))))))

;; Intersection of two sorted string lists.
(define (set-intersect a b)
  (cond ((null? a) '())
        ((null? b) '())
        ((string<? (car a) (car b)) (set-intersect (cdr a) b))
        ((string<? (car b) (car a)) (set-intersect a (cdr b)))
        (else (cons (car a) (set-intersect (cdr a) (cdr b))))))

;; Membership test with early exit on sorted list.
(define (set-member? elem sorted)
  (cond ((null? sorted) #f)
        ((string<? elem (car sorted)) #f)
        ((string=? elem (car sorted)) #t)
        (else (set-member? elem (cdr sorted)))))

;; Elements strictly before a given element in sorted order.
(define (set-before elem sorted)
  (cond ((null? sorted) '())
        ((string<? (car sorted) elem)
         (cons (car sorted) (set-before elem (cdr sorted))))
        (else '())))

;;; ── Context construction ──────────────────────────────────

;; Build an FCA context from objects, attributes, and an incidence function.
;; incidence: (lambda (obj attr) -> boolean)
;; Lookup tables are hash tables for O(1) access in intent/extent.
(define (make-context objects attributes incidence)
  (let* ((objs (sort-strings objects))
         (attrs (sort-strings attributes))
         (obj->attrs (make-hashtable))
         (attr->objs (make-hashtable)))
    (for-each
      (lambda (o)
        (hashtable-set! obj->attrs o
          (let loop ((as attrs))
            (cond ((null? as) '())
                  ((incidence o (car as))
                   (cons (car as) (loop (cdr as))))
                  (else (loop (cdr as)))))))
      objs)
    (for-each
      (lambda (a)
        (hashtable-set! attr->objs a
          (let loop ((os objs))
            (cond ((null? os) '())
                  ((incidence (car os) a)
                   (cons (car os) (loop (cdr os))))
                  (else (loop (cdr os)))))))
      attrs)
    (list 'fca-context
          (cons 'objects objs)
          (cons 'attributes attrs)
          (cons 'obj->attrs obj->attrs)
          (cons 'attr->objs attr->objs))))

;; Accessors
(define (context-objects ctx) (nf ctx 'objects))
(define (context-attributes ctx) (nf ctx 'attributes))

;; Convenience: build context from an association list.
;; Each entry is (object attr1 attr2 ...).
(define (context-from-alist entries)
  (let* ((objs (map car entries))
         (attrs (sort-strings
                  (let loop ((es entries) (acc '()))
                    (if (null? es) acc
                      (loop (cdr es) (append (cdr (car es)) acc))))))
         (incidence
           (lambda (o a)
             (let ((entry (assoc o entries)))
               (and entry (member? a (cdr entry)))))))
    (make-context objs attrs incidence)))

;;; ── Derivation operators (Galois connection) ──────────────

;; Attributes shared by ALL objects in object-set.
;; Empty object-set → all attributes (vacuous truth).
(define (intent ctx object-set)
  (if (null? object-set)
    (context-attributes ctx)
    (let ((ht (nf ctx 'obj->attrs)))
      (let loop ((rest (cdr object-set))
                 (acc (hashtable-ref ht (car object-set) '())))
        (if (null? rest) acc
          (loop (cdr rest)
                (set-intersect acc (hashtable-ref ht (car rest) '()))))))))

;; Objects having ALL attributes in attribute-set.
;; Empty attribute-set → all objects (vacuous truth).
(define (extent ctx attribute-set)
  (if (null? attribute-set)
    (context-objects ctx)
    (let ((ht (nf ctx 'attr->objs)))
      (let loop ((rest (cdr attribute-set))
                 (acc (hashtable-ref ht (car attribute-set) '())))
        (if (null? rest) acc
          (loop (cdr rest)
                (set-intersect acc (hashtable-ref ht (car rest) '()))))))))

;;; ── Concept lattice (NextClosure, Ganter 1984) ───────────

;; Concept accessors: a concept is (extent . intent).
(define (concept-extent c) (car c))
(define (concept-intent c) (cdr c))

;; Closure operator: attribute set → closed attribute set.
(define (fca-close ctx attrs)
  (intent ctx (extent ctx attrs)))

;; Next closure in lectic order.
;; Returns the next closed set after current, or #f if done.
(define (next-closure current attrs close)
  (let ((attr-vec (list->vector attrs))
        (n (length attrs)))
    (let loop ((i (- n 1)))
      (if (< i 0) #f
        (let ((ai (vector-ref attr-vec i)))
        (if (set-member? ai current)
          (loop (- i 1))
          (let* ((prefix (set-before ai current))
                 (b-prime (set-add ai prefix))
                 (c (close b-prime)))
            (if (equal? (set-before ai c) prefix)
              c
              (loop (- i 1))))))))))

;; Build the full concept lattice.
;; Returns a list of concepts (extent . intent) in lectic order.
(define (concept-lattice ctx)
  (let* ((attrs (context-attributes ctx))
         (close (lambda (b) (fca-close ctx b)))
         (first (close '())))
    (let loop ((current first) (acc '()))
      (let ((concept (cons (extent ctx current) current)))
        (let ((next (next-closure current attrs close)))
          (if next
            (loop next (cons concept acc))
            (reverse (cons concept acc))))))))

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
