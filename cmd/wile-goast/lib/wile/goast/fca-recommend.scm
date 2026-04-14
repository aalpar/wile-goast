;;; fca-recommend.scm — Function boundary recommendations via FCA + SSA
;;;
;;; Analyzes concept lattice structure to produce ranked split/merge/extract
;;; recommendations for function boundaries. SSA data flow filtering
;;; distinguishes intentional coordination from accidental aggregation.
;;; Pareto dominance ranking with separate frontiers per type.
