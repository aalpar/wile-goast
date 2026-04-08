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
(define (make-context objects attributes incidence)
  (let* ((objs (sort-strings objects))
         (attrs (sort-strings attributes))
         (obj->attrs
           (map (lambda (o)
                  (cons o (let loop ((as attrs))
                            (cond ((null? as) '())
                                  ((incidence o (car as))
                                   (cons (car as) (loop (cdr as))))
                                  (else (loop (cdr as)))))))
                objs))
         (attr->objs
           (map (lambda (a)
                  (cons a (let loop ((os objs))
                            (cond ((null? os) '())
                                  ((incidence (car os) a)
                                   (cons (car os) (loop (cdr os))))
                                  (else (loop (cdr os)))))))
                attrs)))
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
    (let* ((lookup (nf ctx 'obj->attrs))
           (first-entry (assoc (car object-set) lookup))
           (first-attrs (if first-entry (cdr first-entry) '())))
      (let loop ((rest (cdr object-set)) (acc first-attrs))
        (if (null? rest) acc
          (let* ((entry (assoc (car rest) lookup))
                 (attrs (if entry (cdr entry) '())))
            (loop (cdr rest) (set-intersect acc attrs))))))))

;; Objects having ALL attributes in attribute-set.
;; Empty attribute-set → all objects (vacuous truth).
(define (extent ctx attribute-set)
  (if (null? attribute-set)
    (context-objects ctx)
    (let* ((lookup (nf ctx 'attr->objs))
           (first-entry (assoc (car attribute-set) lookup))
           (first-objs (if first-entry (cdr first-entry) '())))
      (let loop ((rest (cdr attribute-set)) (acc first-objs))
        (if (null? rest) acc
          (let* ((entry (assoc (car rest) lookup))
                 (objs (if entry (cdr entry) '())))
            (loop (cdr rest) (set-intersect acc objs))))))))

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
  (let loop ((i (- (length attrs) 1)))
    (if (< i 0) #f
      (let ((ai (list-ref attrs i)))
        (if (set-member? ai current)
          (loop (- i 1))
          (let* ((prefix (set-before ai current))
                 (b-prime (set-add ai prefix))
                 (c (close b-prime)))
            (if (equal? (set-before ai c) prefix)
              c
              (loop (- i 1)))))))))

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

;;; ── Stubs for future tasks ────────────────────────────────

(define (field-index->context . args)
  (error "field-index->context not yet implemented"))

(define (cross-boundary-concepts . args)
  (error "cross-boundary-concepts not yet implemented"))

(define (boundary-report . args)
  (error "boundary-report not yet implemented"))
