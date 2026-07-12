// Copyright 2026 Aaron Alpar
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package goast_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/aalpar/wile/pkg/values"
)

const dispatchPkg = `"github.com/aalpar/wile-goast/goast/testdata/dispatch"`
const dispatchReflectPkg = `"github.com/aalpar/wile-goast/goast/testdata/dispatch_reflect"`
const dispatchSyntheticPkg = `"github.com/aalpar/wile-goast/goast/testdata/dispatch_synthetic"`
const dispatchStructuralPkg = `"github.com/aalpar/wile-goast/goast/testdata/dispatch_structural"`

// TestDispatch_MustSite: one implementor flows to the site, so VTA's SOUND set is
// a singleton — the true callee set is a subset of {(OnlyImpl).S}. That is a
// genuine `must`, and it needs no analysis beyond counting.
//
// CAVEAT: this soundness argument is VTA's, and VTA's is conditional. x/tools
// go/callgraph/vta/vta.go:74-75 states the call graph is sound "MODULO USE OF
// REFLECTION AND UNSAFE" — a concrete type injected into an interface only
// through reflection (reflect.New(t).Elem().Interface().(I), the reflective-
// registry idiom used by encoding/json, database/sql, protobuf, and
// apimachinery's runtime.Scheme) never appears in a MakeInterface, so VTA
// cannot see it flow in and `must` CAN BE WRONG in a scope that uses
// reflect/unsafe. See TestDispatch_ReflectionInScope, which reproduces exactly
// this: a `must` finding with the WRONG candidate, disclosed via the
// `reflection-in-scope` defeater field rather than silently trusted.
func TestDispatch_MustSite(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast dispatch) (wile goast utils))`)
	eval(t, engine, `(define ds (dispatch-sites `+dispatchPkg+`))`)
	// The iface field is a FULL type string, e.g.
	// "github.com/aalpar/wile-goast/goast/testdata/dispatch.Single" — match on the
	// suffix, never on a guessed fully-qualified name.
	eval(t, engine, `
		(define must-site
		  (let loop ((l ds))
		    (cond ((null? l) #f)
		          ((string-contains? (or (dispatch-iface (car l)) "") "Single") (car l))
		          (else (loop (cdr l))))))`)

	c.Assert(eval(t, engine, `(if must-site #t #f)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(eq? (dispatch-class must-site) 'must)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(= (dispatch-n must-site) 1)`).Internal(), qt.Equals, values.TrueValue)
}

// TestDispatch_MaySite_PrunesTheDecoy: THE property the library exists for. Decoy
// implements Multi and is allocated, but never converted to Multi. CHA folds it in;
// VTA prunes it. n must be 3, and `Decoy` must not appear among the candidates.
// If this fails, the library is reporting a bound formatted as a fact.
func TestDispatch_MaySite_PrunesTheDecoy(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast dispatch) (wile goast utils))`)
	eval(t, engine, `(define ds (dispatch-sites `+dispatchPkg+`))`)
	eval(t, engine, `
		(define may-site
		  (let loop ((l ds))
		    (cond ((null? l) #f)
		          ((and (eq? (dispatch-class (car l)) 'may)
		                (= (dispatch-n (car l)) 3)) (car l))
		          (else (loop (cdr l))))))`)

	c.Assert(eval(t, engine, `(if may-site #t #f)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(= (dispatch-n may-site) 3)`).Internal(), qt.Equals, values.TrueValue)

	// narrowed-from records CHA's count at the same site: it must exceed n,
	// which is the evidence that VTA actually pruned something.
	c.Assert(eval(t, engine, `(> (dispatch-narrowed-from may-site) 3)`).Internal(),
		qt.Equals, values.TrueValue)

	// The decoy must be absent from the candidate set.
	c.Assert(eval(t, engine, `
		(let loop ((cs (dispatch-candidates may-site)))
		  (cond ((null? cs) #f)
		        ((string-contains? (nf (car cs) 'callee) "Decoy") #t)
		        (else (loop (cdr cs)))))`).Internal(), qt.Equals, values.FalseValue)
}

// TestDispatch_IsAFinding: a dispatch site IS a (wile goast provenance) finding, so
// render-finding and every other finding consumer work on it unchanged. `score` is
// #f: no natural confidence exists here and inventing one would be a fabrication.
func TestDispatch_IsAFinding(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast dispatch) (wile goast provenance))`)
	eval(t, engine, `(define d (car (dispatch-sites `+dispatchPkg+`)))`)

	c.Assert(eval(t, engine, `(symbol? (finding-value d))`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(eq? (finding-score d) #f)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(eq? (car (finding-why d)) 'dispatch)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(string? (render-finding d))`).Internal(), qt.Equals, values.TrueValue)
}

// TestDispatch_WitnessLocatesTheConversion: a candidate says WHERE its concrete type
// entered the interface. `func` is always present; `pos` may be #f — MakeInterface
// carries a position only for an EXPLICIT conversion, and implicit conversion is the
// common form in real Go.
func TestDispatch_WitnessLocatesTheConversion(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast dispatch) (wile goast utils))`)
	eval(t, engine, `(define ds (dispatch-sites `+dispatchPkg+`))`)
	eval(t, engine, `
		(define must-site
		  (let loop ((l ds))
		    (cond ((null? l) #f)
		          ((and (eq? (dispatch-class (car l)) 'must)
		                (string-contains? (or (dispatch-iface (car l)) "") "Single")) (car l))
		          (else (loop (cdr l))))))`)
	eval(t, engine, `(define cand (car (dispatch-candidates must-site)))`)

	// Every witness names the function in which the conversion happens.
	c.Assert(eval(t, engine, `
		(let loop ((ws (nf cand 'witness)))
		  (cond ((null? ws) #t)
		        ((string? (nf (car ws) 'func)) (loop (cdr ws)))
		        (else #f)))`).Internal(), qt.Equals, values.TrueValue)
}

// TestDispatch_WitnessPosIsAbsentNotFabricated: an implicit conversion has no
// MakeInterface position. The witness must report #f — never a nearby line, never a
// guess. A WRONG witness is worse than a MISSING one, because the consumer cannot
// detect it.
func TestDispatch_WitnessPosIsAbsentNotFabricated(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast dispatch) (wile goast utils))`)
	eval(t, engine, `(define ds (dispatch-sites `+dispatchPkg+`))`)

	// Every witness pos is either a string or #f. Nothing else is legal.
	c.Assert(eval(t, engine, `
		(let sloop ((sites ds) (ok #t))
		  (if (or (not ok) (null? sites))
		      ok
		      (let ((cs (dispatch-candidates (car sites))))
		        (if (not cs)
		            (sloop (cdr sites) ok)
		            (let cloop ((cs cs) (o ok))
		              (if (or (not o) (null? cs))
		                  (sloop (cdr sites) o)
		                  (let wloop ((ws (nf (car cs) 'witness)) (w #t))
		                    (if (or (not w) (null? ws))
		                        (cloop (cdr cs) w)
		                        (let ((p (nf (car ws) 'pos)))
		                          (wloop (cdr ws)
		                                 (or (string? p) (eq? p #f)))))))))))) `).Internal(),
		qt.Equals, values.TrueValue)
}

// TestDispatch_WitnessNamesTheInterfaceItEntered: witness-index keys on concrete
// type alone, so a witness list MAY contain conversions of this type into a
// DIFFERENT interface than the site dispatches on (see the doc comment on
// witness-index). Every witness is therefore LABELLED with the interface it
// actually entered — `iface`, ssa-make-interface's own `type` field — so the
// consumer can tell which witness matches this site and which does not.
func TestDispatch_WitnessNamesTheInterfaceItEntered(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast dispatch) (wile goast utils))`)
	eval(t, engine, `(define ds (dispatch-sites `+dispatchPkg+`))`)

	// Every witness on every candidate of every site carries a STRING iface.
	c.Assert(eval(t, engine, `
		(let sloop ((sites ds) (ok #t))
		  (if (or (not ok) (null? sites))
		      ok
		      (let ((cs (dispatch-candidates (car sites))))
		        (if (not cs)
		            (sloop (cdr sites) ok)
		            (let cloop ((cs cs) (o ok))
		              (if (or (not o) (null? cs))
		                  (sloop (cdr sites) o)
		                  (let wloop ((ws (nf (car cs) 'witness)) (w #t))
		                    (if (or (not w) (null? ws))
		                        (cloop (cdr cs) w)
		                        (wloop (cdr ws) (string? (nf (car ws) 'iface))))))))))) `).Internal(),
		qt.Equals, values.TrueValue)
}

// TestDispatch_KInvariant is THE property. The knob may remove DETAIL, never TRUTH.
//
// For ANY k, at every site:
//   - the set of sites is identical (k never hides a site)
//   - `n` is identical (k never makes a site look smaller than it is)
//   - only `detail` and the PRESENCE of `candidates` may differ
//   - `candidates` is ABSENT (#f) when elided — never '() — so a 27-way site can
//     never read as "no candidates" to a careless consumer
//
// If this fails, the knob has become the silent false negative that the entire
// finding shape was designed to make impossible.
func TestDispatch_KInvariant(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast dispatch) (wile goast utils))`)
	eval(t, engine, `(define k1    (dispatch-sites `+dispatchPkg+` 1))`)
	eval(t, engine, `(define k8    (dispatch-sites `+dispatchPkg+` 8))`)
	eval(t, engine, `(define kbig  (dispatch-sites `+dispatchPkg+` 1000))`)

	// Same number of sites at every k.
	c.Assert(eval(t, engine, `
		(and (= (length k1) (length k8)) (= (length k8) (length kbig)))`).Internal(),
		qt.Equals, values.TrueValue)

	// n is identical, site by site, at every k.
	c.Assert(eval(t, engine, `
		(let loop ((a k1) (b k8) (c kbig))
		  (cond ((null? a) #t)
		        ((and (= (dispatch-n (car a)) (dispatch-n (car b)))
		              (= (dispatch-n (car b)) (dispatch-n (car c))))
		         (loop (cdr a) (cdr b) (cdr c)))
		        (else #f)))`).Internal(), qt.Equals, values.TrueValue)

	// candidates is #f when elided — never '(). At k=1, every site with n>1 is
	// elided, and every one of them must report #f.
	c.Assert(eval(t, engine, `
		(let loop ((l k1))
		  (cond ((null? l) #t)
		        ((> (dispatch-n (car l)) 1)
		         (if (eq? (dispatch-candidates (car l)) #f)
		             (loop (cdr l))
		             #f))
		        (else (loop (cdr l)))))`).Internal(), qt.Equals, values.TrueValue)

	// At a large k nothing is elided: every site enumerates, and the enumeration
	// length equals n.
	c.Assert(eval(t, engine, `
		(let loop ((l kbig))
		  (cond ((null? l) #t)
		        ((eq? (dispatch-detail (car l)) 'elided) #f)
		        ((and (> (dispatch-n (car l)) 0)
		              (not (= (length (dispatch-candidates (car l)))
		                      (dispatch-n (car l))))) #f)
		        (else (loop (cdr l)))))`).Internal(), qt.Equals, values.TrueValue)
}

// TestDispatch_ReflectionInScope: THE CRITICAL reproduction. dispatch_reflect's
// `Make` returns a Greeter via reflection (reflect.New(t).Elem().Interface()
// .(Greeter)) on every path except the literal "alpha" branch, which is the
// ONLY explicit MakeInterface in the package. VTA's flow set for the interface
// is therefore {Alpha} alone, so a call built as `Make("beta")` — whose GROUND
// TRUTH callee is (Beta).Greet — is reported `must`, n=1, candidate=Alpha: A
// FALSE `must`, exactly as x/tools go/callgraph/vta/vta.go:74-75 predicts
// ("sound... MODULO USE OF REFLECTION AND UNSAFE"). The class itself does NOT
// change — VTA genuinely cannot see Beta — but the finding must carry its own
// defeater: `reflection-in-scope` is #t here, and #f on a scope that never
// touches reflect/unsafe (dispatchPkg, the plain fixture).
func TestDispatch_ReflectionInScope(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast dispatch) (wile goast utils))`)
	eval(t, engine, `(define dr (dispatch-sites `+dispatchReflectPkg+`))`)

	// Exactly one dispatch site: the reflective-registry reproduction.
	c.Assert(eval(t, engine, `(= (length dr) 1)`).Internal(), qt.Equals, values.TrueValue)

	// The FALSE must, reproduced: class=must, n=1, candidate=Alpha -- while the
	// ground truth (running the program) is Beta. This is not a bug to fix; the
	// class is exactly what VTA can support given what it can see. What must be
	// true is that the finding DISCLOSES the mechanism that makes it fallible.
	c.Assert(eval(t, engine, `(eq? (dispatch-class (car dr)) 'must)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(= (dispatch-n (car dr)) 1)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `
		(string-contains? (nf (car (dispatch-candidates (car dr))) 'callee) "Alpha")`).Internal(),
		qt.Equals, values.TrueValue)

	// THE DEFEATER: reflection-in-scope is #t on this finding.
	c.Assert(eval(t, engine, `(eq? (dispatch-reflection-in-scope (car dr)) #t)`).Internal(),
		qt.Equals, values.TrueValue)

	// A scope that never touches reflect/unsafe carries #f on every finding --
	// the defeater is not fabricated where the mechanism is absent.
	eval(t, engine, `(define ds (dispatch-sites `+dispatchPkg+`))`)
	c.Assert(eval(t, engine, `
		(let loop ((l ds))
		  (cond ((null? l) #t)
		        ((eq? (dispatch-reflection-in-scope (car l)) #f) (loop (cdr l)))
		        (else #f)))`).Internal(), qt.Equals, values.TrueValue)
}

// TestDispatch_SyntheticCallerMarksPhantomSites: dispatch_synthetic's BoundSite
// takes a METHOD VALUE (`f := x.M`) rather than calling immediately. The SSA
// builder synthesizes a compiler-generated forwarding function
// ((Ifc).M$bound, ssa.Function.Synthetic != "") whose single invoke has no
// source position — it is not a call site that exists in source at all. This
// is the mechanism behind client-go's 61-of-62 phantom pos-less `must`
// findings ($bound/$thunk closures and embedded-interface promotion stubs).
// `synthetic-caller` marks it so a consumer can filter phantom sites out.
func TestDispatch_SyntheticCallerMarksPhantomSites(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast dispatch) (wile goast provenance) (wile goast utils))`)
	eval(t, engine, `(define dsy (dispatch-sites `+dispatchSyntheticPkg+`))`)

	c.Assert(eval(t, engine, `(= (length dsy) 1)`).Internal(), qt.Equals, values.TrueValue)

	// The phantom site: no source position, exactly the shape client-go's
	// pos-less findings have.
	c.Assert(eval(t, engine, `(eq? (finding-where (car dsy)) #f)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(eq? (dispatch-synthetic-caller (car dsy)) #t)`).Internal(),
		qt.Equals, values.TrueValue)

	// A real, source-level call site is NOT marked synthetic.
	eval(t, engine, `(define ds (dispatch-sites `+dispatchPkg+`))`)
	c.Assert(eval(t, engine, `
		(let loop ((l ds))
		  (cond ((null? l) #t)
		        ((eq? (dispatch-synthetic-caller (car l)) #f) (loop (cdr l)))
		        (else #f)))`).Internal(), qt.Equals, values.TrueValue)
}

// TestDispatch_StructuralInterfaceIsUnnamed: dispatch_structural's UseAnon
// dispatches on `interface{Close() error}`, a TYPE LITERAL with no package to
// be "exported" FROM. The original type-exported? assumed every `iface` was a
// qualified type name and read whatever capital letter it found scanning from
// the end — #f here (FALSE REASSURANCE: any package anywhere can structurally
// satisfy this interface, so `must` is MORE scope-limited than an exported
// named interface, not less). `dispatch-iface-exported` must report 'unnamed,
// never fabricate a boolean, on a structural interface.
func TestDispatch_StructuralInterfaceIsUnnamed(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)

	eval(t, engine, `(import (wile goast dispatch) (wile goast utils))`)
	eval(t, engine, `(define dst (dispatch-sites `+dispatchStructuralPkg+`))`)

	c.Assert(eval(t, engine, `(= (length dst) 1)`).Internal(), qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `
		(string-contains? (dispatch-iface (car dst)) "interface{")`).Internal(),
		qt.Equals, values.TrueValue)
	c.Assert(eval(t, engine, `(eq? (dispatch-iface-exported (car dst)) 'unnamed)`).Internal(),
		qt.Equals, values.TrueValue)

	// A NAMED exported interface (the plain fixture's Single/Multi) is
	// unaffected by the fix -- still a plain boolean.
	eval(t, engine, `(define ds (dispatch-sites `+dispatchPkg+`))`)
	c.Assert(eval(t, engine, `
		(let loop ((l ds))
		  (cond ((null? l) #f)
		        ((eq? (dispatch-iface-exported (car l)) #t) #t)
		        (else (loop (cdr l)))))`).Internal(), qt.Equals, values.TrueValue)
}
