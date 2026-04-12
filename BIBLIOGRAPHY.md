# Bibliography

References for the static analysis techniques underlying wile-goast's five
extension layers: AST parsing and type-checking, SSA construction and data-flow
analysis, call graph construction, control flow graphs with dominance, and
pluggable lint passes via the go/analysis framework. Scheme and Lisp language
references are covered by the parent Wile project and are not repeated here.

---

## 1. Foundational Static Analysis

Flemming Nielson, Hanne Riis Nielson, and Chris Hankin.
*Principles of Program Analysis*.
Springer, 1999. Corrected edition 2015.
ISBN 978-3-642-08474-4.
https://doi.org/10.1007/978-3-662-03811-6

Alfred V. Aho, Monica S. Lam, Ravi Sethi, and Jeffrey D. Ullman.
*Compilers: Principles, Techniques, and Tools* (2nd edition).
Addison-Wesley, 2006.
ISBN 978-0-321-48681-3.

Steven S. Muchnick.
*Advanced Compiler Design and Implementation*.
Morgan Kaufmann, 1997.
ISBN 978-1-558-60320-2.

## 2. SSA Form

Ron Cytron, Jeanne Ferrante, Barry K. Rosen, Mark N. Wegman, and F. Kenneth Zadeck.
"Efficiently Computing Static Single Assignment Form and the Control Dependence Graph."
*ACM Transactions on Programming Languages and Systems*, 13(4):451--490, October 1991.
https://doi.org/10.1145/115372.115320

Marc M. Brandis and Hanspeter Moessenboeck.
"Single-Pass Generation of Static Single-Assignment Form for Structured Languages."
*ACM Transactions on Programming Languages and Systems*, 16(6):1684--1698, November 1994.
https://doi.org/10.1145/197320.197331

## 3. Control Flow and Dominators

Thomas Lengauer and Robert Endre Tarjan.
"A Fast Algorithm for Finding Dominators in a Flowgraph."
*ACM Transactions on Programming Languages and Systems*, 1(1):121--141, July 1979.
https://doi.org/10.1145/357062.357071

Keith D. Cooper, Timothy J. Harvey, and Ken Kennedy.
"A Simple, Fast Dominance Algorithm."
Software Practice and Experience, 4:1--10, 2001.
https://www.cs.rice.edu/~keith/EMBED/dom.pdf

## 4. Call Graph Construction

Jeffrey Dean, David Grove, and Craig Chambers.
"Optimization of Object-Oriented Programs Using Static Class Hierarchy Analysis."
In *Proceedings of the 9th European Conference on Object-Oriented Programming (ECOOP '95)*,
Lecture Notes in Computer Science, vol. 952, pp. 77--101. Springer, 1995.
https://doi.org/10.1007/3-540-49538-X_5

David F. Bacon and Peter F. Sweeney.
"Fast Static Analysis of C++ Virtual Function Calls."
In *Proceedings of the 11th ACM SIGPLAN Conference on Object-Oriented Programming, Systems, Languages, and Applications (OOPSLA '96)*, pp. 324--341. ACM, 1996.
https://doi.org/10.1145/236337.236371

Vijay Sundaresan, Laurie Hendren, Chrislain Razafimahefa, Raja Vallee-Rai, Patrick Lam, Etienne Gagnon, and Charles Godin.
"Practical Virtual Method Call Resolution for Java."
In *Proceedings of the 15th ACM SIGPLAN Conference on Object-Oriented Programming, Systems, Languages, and Applications (OOPSLA '00)*, pp. 264--280. ACM, 2000.
https://doi.org/10.1145/353171.353189

## 5. Go-Specific Tooling

Alan Donovan and others.
`golang.org/x/tools/go/ssa` -- SSA construction for Go programs.
https://pkg.go.dev/golang.org/x/tools/go/ssa

Alan Donovan and others.
`golang.org/x/tools/go/callgraph` -- Call graph construction algorithms (static, CHA, RTA, VTA).
https://pkg.go.dev/golang.org/x/tools/go/callgraph

`golang.org/x/tools/go/cfg` -- Control flow graph construction for Go functions.
https://pkg.go.dev/golang.org/x/tools/go/cfg

`golang.org/x/tools/go/analysis` -- Modular static analysis framework for Go.
https://pkg.go.dev/golang.org/x/tools/go/analysis

Alan Donovan.
"Using go/analysis to write a custom linter."
GopherCon 2019.
https://www.youtube.com/watch?v=eGKDqSbRqhI

## 6. Consistency Deviation Detection

Dawson Engler, David Yu Chen, Seth Hallem, Andy Chou, and Benjamin Chelf.
"Bugs as Deviant Behavior: A General Approach to Inferring Errors in Systems Code."
In *Proceedings of the 18th ACM Symposium on Operating Systems Principles (SOSP '01)*, pp. 57--72. ACM, 2001.
https://doi.org/10.1145/502034.502041

The foundational paper for wile-goast's belief DSL. Key insight: if a pattern
holds in 95% of cases, the 5% are likely bugs. No explicit specification needed —
statistical deviation *is* the specification.

## 7. Pattern Detection and Code Similarity

Ira D. Baxter, Andrew Yahin, Leonardo Moura, Marcelo Sant'Anna, and Lorraine Bier.
"Clone Detection Using Abstract Syntax Trees."
In *Proceedings of the International Conference on Software Maintenance (ICSM '98)*, pp. 368--377. IEEE, 1998.
https://doi.org/10.1109/ICSM.1998.738528

Lingxiao Jiang, Ghassan Misherghi, Zhendong Su, and Stephane Glondu.
"DECKARD: Scalable and Accurate Tree-Based Detection of Code Clones."
In *Proceedings of the 29th International Conference on Software Engineering (ICSE '07)*, pp. 96--105. IEEE, 2007.
https://doi.org/10.1109/ICSE.2007.30

Raghavan Komondoor and Susan Horwitz.
"Using Slicing to Identify Duplication in Source Code."
In *Proceedings of the 8th International Symposium on Static Analysis (SAS '01)*,
Lecture Notes in Computer Science, vol. 2126, pp. 40--56. Springer, 2001.
https://doi.org/10.1007/3-540-47764-0_3

## 8. Formal Concept Analysis and Boundary Discovery

Bernhard Ganter and Rudolf Wille.
*Formal Concept Analysis: Mathematical Foundations*.
Springer, 1999.
ISBN 978-3-540-62771-5.
https://doi.org/10.1007/978-3-642-59830-2

The foundational text for FCA. Defines formal contexts, concept lattices,
derivation operators (Galois connections), and the NextClosure algorithm for
efficient lattice enumeration. Applied in wile-goast to discover natural
field groupings from function access patterns.

Bernhard Ganter.
"Two Basic Algorithms in Concept Analysis."
In *Formal Concept Analysis: 8th International Conference (ICFCA 2010)*,
Lecture Notes in Computer Science, vol. 5986, pp. 312--340. Springer, 2010.
https://doi.org/10.1007/978-3-642-11928-6_22

Describes NextClosure and Close-by-One algorithms. NextClosure generates
concepts in lexicographic order without redundancy — O(|G|·|M|·|L|).

Gail C. Murphy, David Notkin, and Kevin J. Sullivan.
"Software Reflexion Models: Bridging the Gap between Design and Implementation."
*IEEE Transactions on Software Engineering*, 27(4):364--380, April 2001.
https://doi.org/10.1109/32.917525

Reflexion models compare intended architecture against actual dependencies.
Divergences (actual coupling crossing intended boundaries) correspond to
false boundaries in wile-goast's sense.

David L. Parnas.
"On the Criteria To Be Used in Decomposing Systems into Modules."
*Communications of the ACM*, 15(12):1053--1058, December 1972.
https://doi.org/10.1145/361598.361623

The original argument for information hiding. A module boundary is justified
iff it hides a design decision that could change. A false boundary hides
nothing — the "secret" is routinely exposed to the other side.

## 9. Software Modularization and Package Decomposition

Spyros Mancoridis, Brian S. Mitchell, Christine Rorres, Yih-Farn Chen, and Emden R. Gansner.
"Using Automatic Clustering to Produce High-Level System Organizations of Source Code."
In *Proceedings of the 6th International Workshop on Program Comprehension (IWPC '98)*, pp. 45--52. IEEE, 1998.
https://doi.org/10.1109/WPC.1998.693283

Introduced the Bunch tool: treat source files as graph nodes, dependencies as
edges, then partition to maximize a quality metric (MQ) that rewards intra-cluster
cohesion and penalizes inter-cluster coupling. Hill-climbing search over the
partition space. The import-signature clustering in wile-goast's refactoring
analysis follows this principle — functions with identical external dependency
sets belong together.

Brian S. Mitchell and Spyros Mancoridis.
"On the Automatic Modularization of Software Systems Using the Bunch Tool."
*IEEE Transactions on Software Engineering*, 32(3):193--208, March 2006.
https://doi.org/10.1109/TSE.2006.31

The journal version of Bunch. Adds genetic algorithm search, hierarchical
clustering, and evaluation against authoritative decompositions. Key finding:
automatic clustering recovers developer-intended module structure with 60-80%
accuracy, and divergences often reveal real structural problems.

Carliss Y. Baldwin and Kim B. Clark.
*Design Rules: The Power of Modularity*.
MIT Press, 2000.
ISBN 978-0-262-02466-2.

Formalizes the Dependency Structure Matrix (DSM) approach to modular design.
A DSM is an N×N matrix where entry (i,j) marks a dependency from module i to
module j. Reordering rows/columns to produce block-diagonal form reveals
independent clusters; off-diagonal entries are the inter-module coupling that
a package split must bridge. Baldwin and Clark's contribution is showing that
modular structure has economic value — it enables independent parallel work
and option-like flexibility.

Robert C. Martin.
*Agile Software Development: Principles, Patterns, and Practices*.
Prentice Hall, 2003.
ISBN 978-0-135-97444-5.

Chapter 20 introduces package cohesion and coupling metrics:
Afferent Coupling (Ca, inbound dependencies), Efferent Coupling (Ce, outbound),
Instability I = Ce/(Ca+Ce), and Abstractness A. The Main Sequence plots
(A, I) to identify packages in the "zone of pain" (stable + concrete) or
"zone of uselessness" (unstable + abstract). These metrics formalize the
intuition behind import-signature analysis: a function's efferent coupling
set *is* its import signature.

Christian Lindig and Gregor Snelting.
"Assessing Modular Structure of Legacy Code Based on Mathematical Concept Analysis."
In *Proceedings of the 19th International Conference on Software Engineering (ICSE '97)*, pp. 349--359. ACM, 1997.
https://doi.org/10.1145/253228.253354

Applies Formal Concept Analysis to software modularization. Functions are
objects, imported modules are attributes; the concept lattice reveals which
function groups share the same dependency set. This is the direct theoretical
basis for import-signature clustering: each concept in the lattice is a maximal
set of functions sharing a maximal set of imports. The lattice structure shows
the refinement hierarchy — which clusters are subsets of which.
