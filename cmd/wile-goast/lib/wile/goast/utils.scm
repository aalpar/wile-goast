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

;;; utils.scm — shared s-expression utilities for goast tagged alists
;;;
;;; Goast AST nodes are tagged alists: (tag (key . val) ...)
;;; These utilities provide field access, traversal, and common
;;; list operations used across analysis scripts.

;; Node field accessor: (nf node key) → value or #f
(define (nf node key)
  "Access a field in a tagged-alist AST node by key.\nReturns the associated value, or #f if not found.\n\nParameters:\n  node : list\n  key : symbol\nReturns: any\nCategory: goast-utils\n\nExamples:\n  (nf func-decl 'name)      ; => \"MyFunc\"\n  (nf func-decl 'missing)   ; => #f\n\nSee also: `tag?', `walk'."
  (let ((e (assoc key (cdr node))))
    (if e (cdr e) #f)))

;; Tag predicate: (tag? node t) → #t if node is a pair tagged with t
(define (tag? node t)
  "Test whether NODE is a tagged-alist with tag T.\nReturns #t if node is a pair whose car is T.\n\nParameters:\n  node : list\n  t : symbol\nReturns: boolean\nCategory: goast-utils\n\nExamples:\n  (tag? node 'func-decl)    ; => #t or #f\n\nSee also: `nf', `walk'."
  (and (pair? node) (eq? (car node) t)))

;; Keep elements where predicate returns true. Correct for #f elements
;; (unlike (filter-map (lambda (x) (and (pred x) x)) lst), which drops #f).
(define (filter pred lst)
  "Return the elements of LST for which PRED returns a true value.\nPreserves #f elements when PRED accepts them (unlike filter-map wrappers).\n\nParameters:\n  pred : procedure\n  lst : list\nReturns: list\nCategory: goast-utils\n\nExamples:\n  (filter odd? '(1 2 3 4))  ; => (1 3)\n\nSee also: `filter-map'."
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (loop (cdr xs)
            (if (pred (car xs)) (cons (car xs) acc) acc)))))

;; Map keeping only non-#f results
(define (filter-map f lst)
  "Apply F to each element of LST, keeping only non-#f results.\n\nParameters:\n  f : procedure\n  lst : list\nReturns: list\nCategory: goast-utils\n\nExamples:\n  (filter-map (lambda (x) (and (> x 2) x)) '(1 2 3 4))  ; => (3 4)\n\nSee also: `flat-map', `filter'."
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (let ((v (f (car xs))))
        (loop (cdr xs) (if v (cons v acc) acc))))))

;; Map + concatenate
(define (flat-map f lst)
  "Apply F to each element of LST and concatenate the resulting lists.\n\nParameters:\n  f : procedure\n  lst : list\nReturns: list\nCategory: goast-utils\n\nExamples:\n  (flat-map (lambda (x) (list x x)) '(1 2))  ; => (1 1 2 2)\n\nSee also: `filter-map'."
  (apply append (map f lst)))

;; Depth-first walk over goast s-expressions.
;; Calls visitor on each tagged-alist node; collects non-#f results.
(define (walk val visitor)
  "Depth-first walk over goast s-expressions.\nCalls (visitor node) on every tagged-alist node in the tree.\nVisitor is called for side effects; return value is ignored.\n\nParameters:\n  val : any\n  visitor : procedure\nReturns: any\nCategory: goast-utils\n\nSee also: `nf', `tag?', `ast-transform'."
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
  "Test whether X is a member of LST using equal?.\n\nParameters:\n  x : any\n  lst : list\nReturns: boolean\nCategory: goast-utils\n\nSee also: `unique'."
  (cond ((null? lst) #f)
        ((equal? x (car lst)) #t)
        (else (member? x (cdr lst)))))

;; Remove duplicates preserving order
(define (unique lst)
  "Remove duplicates from LST, preserving first-occurrence order.\n\nParameters:\n  lst : list\nReturns: list\nCategory: goast-utils\n\nSee also: `member?'."
  (let loop ((xs lst) (seen '()))
    (cond ((null? xs) (reverse seen))
          ((member? (car xs) seen) (loop (cdr xs) seen))
          (else (loop (cdr xs) (cons (car xs) seen))))))

;; String contains character?
(define (has-char? s c)
  "Test whether string S contains character C.\n\nParameters:\n  s : string\n  c : char\nReturns: boolean\nCategory: goast-utils"
  (let loop ((i 0))
    (cond ((>= i (string-length s)) #f)
          ((char=? (string-ref s i) c) #t)
          (else (loop (+ i 1))))))

;; String contains substring?
(define (string-contains s sub)
  "Test whether string S contains substring SUB.\nReturns #t if SUB appears anywhere in S (including as a prefix or suffix).\nAn empty SUB matches any S. O(|S| * |SUB|) naive scan.\n\nParameters:\n  s : string\n  sub : string\nReturns: boolean\nCategory: goast-utils\n\nSee also: `has-char?'."
  (let ((slen (string-length s))
        (sublen (string-length sub)))
    (let loop ((i 0))
      (cond ((> (+ i sublen) slen) #f)
            ((string=? (substring s i (+ i sublen)) sub) #t)
            (else (loop (+ i 1)))))))

;; All unordered pairs from a list (each pair once, in order)
(define (ordered-pairs lst)
  "Return all ordered pairs from LST. Each pair appears once.\nUseful for pairwise comparison of functions.\n\nParameters:\n  lst : list\nReturns: list\nCategory: goast-utils\n\nExamples:\n  (ordered-pairs '(a b c))  ; => ((a b) (a c) (b c))"
  (if (null? lst) '()
    (append
      (map (lambda (b) (list (car lst) b)) (cdr lst))
      (ordered-pairs (cdr lst)))))

;; First n elements of a list
(define (take lst n)
  "Return the first N elements of LST.\n\nParameters:\n  lst : list\n  n : integer\nReturns: list\nCategory: goast-utils\n\nSee also: `drop'."
  (if (or (<= n 0) (null? lst)) '()
    (cons (car lst) (take (cdr lst) (- n 1)))))

;; Drop first n elements of a list
(define (drop lst n)
  "Drop the first N elements of LST and return the rest.\n\nParameters:\n  lst : list\n  n : integer\nReturns: list\nCategory: goast-utils\n\nSee also: `take'."
  (if (or (<= n 0) (null? lst)) lst
    (drop (cdr lst) (- n 1))))

;; Depth-first pre-order tree rewriter over goast s-expressions.
;; f returns a replacement node (no recursion into it) or #f (keep, recurse).
(define (ast-transform node f)
  "Depth-first pre-order tree rewriter over goast s-expressions.\nF receives each tagged-alist node and returns a replacement node.\nIf F returns #f, the original node is kept and children are visited.\nIf F returns a node, that node replaces the original (children not revisited).\n\nParameters:\n  node : list\n  f : procedure\nReturns: list\nCategory: goast-utils\n\nSee also: `ast-splice', `walk'."
  (let ((replacement (f node)))
    (if replacement replacement
      (cond
        ;; Tagged alist: recurse into field values
        ((and (pair? node) (symbol? (car node)))
         (cons (car node)
               (map (lambda (field)
                      (if (pair? field)
                        (cons (car field)
                              (ast-transform (cdr field) f))
                        field))
                    (cdr node))))
        ;; List of child nodes
        ((and (pair? node) (pair? (car node)))
         (map (lambda (n) (ast-transform n f)) node))
        ;; Atom
        (else node)))))

;; Keyword option lookup — (opt-ref '(k1 v1 k2 v2) 'k1 default) => v1
(define (opt-ref opts key default)
  "Look up a keyword option in an alternating key/value list.\nReturns the associated value, or DEFAULT if KEY is not present.\n\nParameters:\n  opts : list\n  key : symbol\n  default : any\nReturns: any\nCategory: goast-utils\n\nExamples:\n  (opt-ref '(fuel 10 mode 'fast) 'fuel 5)  ; => 10\n  (opt-ref '() 'fuel 5)                    ; => 5\n\nSee also: `take', `drop'."
  (let loop ((os opts))
    (cond ((null? os) default)
          ((and (not (null? (cdr os)))
                (eq? (car os) key))
           (cadr os))
          (else (loop (cdr os))))))

;; Flat-mapping rewriter for lists (e.g. statement lists).
;; f returns a list (splice in place) or #f (keep original element).
(define (ast-splice lst f)
  "Flat-mapping rewriter for statement/declaration lists.\nF receives each element and returns a list of replacements.\nUseful for inserting or removing statements in a block.\n\nParameters:\n  lst : list\n  f : procedure\nReturns: list\nCategory: goast-utils\n\nSee also: `ast-transform'."
  (let loop ((xs lst) (acc '()))
    (if (null? xs) (reverse acc)
      (let ((result (f (car xs))))
        (if result
          (loop (cdr xs) (append (reverse result) acc))
          (loop (cdr xs) (cons (car xs) acc)))))))
