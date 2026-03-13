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

## 6. Pattern Detection and Code Similarity

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
