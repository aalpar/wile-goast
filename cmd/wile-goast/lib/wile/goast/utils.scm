;;; utils.scm — shared s-expression utilities for goast tagged alists
;;;
;;; Goast AST nodes are tagged alists: (tag (key . val) ...)
;;; These utilities provide field access, traversal, and common
;;; list operations used across analysis scripts.

;; Node field accessor: (nf node key) → value or #f
(define (nf node key)
  (let ((e (assoc key (cdr node))))
    (if e (cdr e) #f)))

;; Tag predicate: (tag? node t) → #t if node is a pair tagged with t
(define (tag? node t)
  (and (pair? node) (eq? (car node) t)))

;; Map keeping only non-#f results
(define (filter-map f lst)
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (let ((v (f (car xs))))
        (loop (cdr xs) (if v (cons v acc) acc))))))

;; Map + concatenate
(define (flat-map f lst)
  (apply append (map f lst)))

;; Depth-first walk over goast s-expressions.
;; Calls visitor on each tagged-alist node; collects non-#f results.
(define (walk val visitor)
  (cond
    ((not (pair? val)) '())
    ((symbol? (car val))
     (let ((here (visitor val))
           (children (flat-map
                       (lambda (kv)
                         (if (pair? kv) (walk (cdr kv) visitor) '()))
                       (cdr val))))
       (if here (cons here children) children)))
    ((pair? (car val))
     (flat-map (lambda (child) (walk child visitor)) val))
    (else '())))

;; List membership with equal?
(define (member? x lst)
  (cond ((null? lst) #f)
        ((equal? x (car lst)) #t)
        (else (member? x (cdr lst)))))

;; Remove duplicates preserving order
(define (unique lst)
  (let loop ((xs lst) (seen '()))
    (cond ((null? xs) (reverse seen))
          ((member? (car xs) seen) (loop (cdr xs) seen))
          (else (loop (cdr xs) (cons (car xs) seen))))))

;; String contains character?
(define (has-char? s c)
  (let loop ((i 0))
    (cond ((>= i (string-length s)) #f)
          ((char=? (string-ref s i) c) #t)
          (else (loop (+ i 1))))))

;; All unordered pairs from a list (each pair once, in order)
(define (ordered-pairs lst)
  (if (null? lst) '()
    (append
      (map (lambda (b) (list (car lst) b)) (cdr lst))
      (ordered-pairs (cdr lst)))))
