# Receiver-Parameter Asymmetry Detection — Design

> **Status:** Design draft (2026-04-20). Implementation plan pending.
> **Spec source:** Conversation 2026-04-20 during the wile G.1 mid-parse-EOF
> fix (`wile/internal/parser/parser.go` `wrapMidParseEOF`). User critique
> exposed a recognizable anti-pattern: a method whose logical subject is a
> parameter while one receiver field is pulled in as implicit context.
> **Scope:** Go method declarations. New belief in `(wile goast belief)`
> or new linter pass in `(wile goast lint)`. No new base primitives
> needed — `go-typecheck-package` + `go-ssa-build` + existing receiver
> field-access analysis are sufficient.
> **Discovery case (2026-04-20):** in wile, the single receiver read was
> `p.cur` used as the `tok` argument to `NewParserErrorWithWrapf`. The
> operation's real subject was `err` (parameter); the method form hid
> the sync contract between `err` and `p.cur` ("the token must be the
> one at which the error occurred"). Rewritten as a free function, both
> become explicit.

## Goal

Detect methods in Go source where the method form obscures the logical subject
of the operation: the receiver is being used as a namespace/context holder
rather than as the entity the operation acts on. Surface these as
convert-to-function candidates.

## Non-Goals

- **Not replacing `gocritic`'s receiver checks.** Those look at naming, value
  vs. pointer, consistency — surface-level style. This is a semantic check:
  *what is the method actually about*.
- **No automatic refactoring.** Detection only; conversion touches call sites
  and belongs to a separate refactor step.
- **Not a general-purpose "bad method" detector.** Scope is specifically the
  receiver-as-namespace anti-pattern where the receiver contributes a single
  contextual read while a parameter drives the logic.
- **Not flagging accessors.** Methods with zero non-receiver parameters are
  out of scope — they serve the "expose field through interface shape" role.

## Motivation

### Concrete trigger (2026-04-20)

Fixing wile's R7RS §6.13.2 mid-parse-EOF deviation required introducing an
error-wrapping helper. First draft:

```go
func (p *Parser) wrapMidParseEOF(err error, form string) error {
    if errors.Is(err, io.EOF) {
        return NewParserErrorWithWrapf(io.ErrUnexpectedEOF, p.cur,
            "unterminated %s: unexpected end of input", form)
    }
    return err
}
```

The receiver `p` contributes exactly one read: `p.cur`. Every other input
arrives as a parameter. The method signature `p.wrapMidParseEOF(err, form)`
makes the sync contract between `err` and `p.cur` (the token must be the
one at which `err` occurred) invisible at call sites. If a refactor shifts
when `p.cur` advances relative to error production, every call site breaks
silently — no signature change forces re-examination.

Rewriting as a free function:

```go
func wrapMidParseEOF(err error, tok tokenizer.Token, form string) error {
    if errors.Is(err, io.EOF) {
        return NewParserErrorWithWrapf(io.ErrUnexpectedEOF, tok, "...")
    }
    return err
}
```

makes every input explicit. Call sites become `wrapMidParseEOF(p.err, p.cur,
"list")`. The sync contract is now visible in every call. If future changes
desynchronize `p.err` and `p.cur`, the call sites must be audited.

### Why this matters as a detection target

Any method where:

- The receiver read set is a *singleton*
- The receiver write set is empty
- There is at least one non-receiver parameter
- The receiver field read is *used in combination with* a parameter

is a candidate for the same refactor. In a codebase the size of wile
(~60 kLOC Go), this pattern almost certainly recurs — nobody writes it
deliberately as a class; it emerges when a helper is first written inside
a type's file and ends up with receiver syntax by proximity.

A belief or lint pass that finds these surfaces them as a batch. The fix
is mechanical once detected, and the resulting code has the hidden-contract
property eliminated.

## Terminology

This anti-pattern has names across multiple traditions:

| Tradition | Name | Source | What it captures |
|---|---|---|---|
| Refactoring | **Feature Envy** | Fowler, *Refactoring* (1999) | Method uses more from other entities than from its own receiver state. Classically framed as envying another class; here the "other class" is the parameter set itself. |
| OO design | **Connascence of Meaning** | Page-Jones, *Fundamentals of Object-Oriented Design in UML* (1999) | Two inputs must agree on a shared semantic (here: `p.cur` and `err` must correspond — the token must be the one at which the error occurred). Method form hides this; function form surfaces it. |
| Go community | **Receiver as namespace** | folklore | Receiver is a grouping mechanism, not state-bearing for the operation. In Go where free functions are first-class, this is a standard smell. |
| Clean Code | **Inappropriate intimacy** (variant) | Martin, *Clean Code* (2008) | Method too close to another class's internals — here, the receiver field is being peeked at when it's really a parameter in spirit. |

Page-Jones's **connascence** taxonomy is the most precise framing. The two
inputs (the received field and the parameter) have
Connascence-of-Meaning; making them both explicit parameters moves the
relationship from hidden (requires reading code to know they must match)
to visible (every call site must supply both). The "level" of connascence
doesn't change — it's still CoM — but its *locality* improves: the
requirement appears at every call, not just in the function body.

## Pattern Characterization

For a Go method `m` declared on type `T` with signature

```
func (r T_or_*T) m(args...) R
```

define:

- **Receiver read set** `RR(m)` — set of field accesses `r.f` occurring in `m`'s
  body, including transitive accesses via embedded fields. Measured over all
  paths (union, not path-sensitive).
- **Receiver write set** `RW(m)` — set of field assignments `r.f = ...` in
  `m`'s body, including calls to mutating methods on fields.
- **Parameter set** `Params(m)` — names of non-receiver parameters.
- **Interface-membership predicate** `IM(m)` — `true` iff `m` is in the method
  set of some interface that more than one concrete type implements. For
  single-implementor interfaces, `m` is morally free — `IM(m) = false`.
- **Method-value usage** `MV(m)` — `true` iff `m` appears as a method value
  or method expression outside call position (e.g., `f := obj.m` or
  `slice.SortFunc(xs, cmp.Compare)` where `Compare` is a method).
- **Use correlation** `UC(m)` — semantic predicate: does `m`'s body use the
  singleton receiver read in combination with a parameter to produce the
  result? Hardest to mechanize; see §Detection below.

### Mechanical detection rule (Level 1)

```
flag(m) := |RR(m)| = 1
        ∧ |RW(m)| = 0
        ∧ |Params(m)| ≥ 1
        ∧ ¬IM(m)
        ∧ ¬MV(m)
```

This is fully mechanizable over SSA. False positives expected on:

- Accessors with side-argument formatting (e.g., `func (e Err) Error() string`
  using `e.msg` — `|Params| = 0` should exclude, but stringer-style
  methods with args may slip through).
- Tiny methods that read one field for dispatch (e.g., `func (p *P) Kind()
  Kind { return p.kind }` — `|Params| = 0`, safe).
- Convenience forwarders (`func (p *P) Add(x int) int { return p.base + x }`) —
  these *are* technically the pattern, but refactoring buys little.

### Refined detection rule (Level 2)

Add a signal that the receiver read is used *jointly with* a parameter:

```
UC_mechanical(m) := ∃ expression E in body(m) such that
                    E uses both RR(m) and some p ∈ Params(m)
                    within a single call, binary op, or struct literal.
```

Operationalize via SSA: the single receiver-read value and at least one
parameter value must flow into the *same* instruction. Empty write set
stays mandatory.

```
flag_L2(m) := flag_L1(m) ∧ UC_mechanical(m)
```

This separates "convenience forwarders" (receiver field used alone, result
combined with params downstream) from "subject inversion" (receiver field
used *inside* an operation whose other inputs are parameters).

### Severity heuristics

| Signal | Severity contribution |
|---|---|
| Receiver field is passed *as an argument* to a call using a parameter | High — this is the G.1 pattern exactly |
| Receiver field is used in a condition that gates parameter use | Medium |
| Receiver field appears in the error/log message formatting a parameter | Medium |
| Receiver field is combined arithmetically/structurally with a parameter | Low-Medium |
| Receiver field appears only in a message that doesn't reference any parameter | Low (receiver as pure context) |

A reported finding includes the severity signal for prioritization.

## Implementation Sketch

### Option A: Belief in `(wile goast belief)`

```scheme
(define-belief method-reads-singleton-with-param
  (sites (functions-matching
           (lambda (fn)
             (and (has-receiver? fn)
                  (positive-parameter-count? fn)
                  (not (interface-method? fn))
                  (not (referenced-as-method-value? fn))))))
  (expect (lambda (fn ctx)
            (let ((rr (receiver-reads fn))
                  (rw (receiver-writes fn)))
              (cond
                ((positive? (size rw))           'mutation)
                ((not (= 1 (size rr)))           'other)
                ((joint-use? rr (parameters fn)) 'candidate)
                (else                            'forwarder)))))
  (threshold 0 1))
```

Meaning: in the file (or package) context, find all methods meeting the
filter; classify each as one of mutation, other, candidate, or forwarder.
A finding is emitted for each `candidate`, with supporting data (receiver
field name, parameters involved, source location).

### Option B: Linter pass in `(wile goast lint)`

Wrap as a standalone Go `go/analysis.Analyzer` that emits diagnostics:

```
method-receiver-asymmetry: method wrapMidParseEOF reads only p.cur;
  consider converting to free function taking tokenizer.Token explicitly.
  Receiver-read fields: {cur}
  Parameters combined with receiver read: {err}
  Suggested signature: wrapMidParseEOF(err error, tok tokenizer.Token, form string) error
```

### Required primitives (all existing)

- `go-typecheck-package` — to get `*types.Func` and `*ssa.Function` for each method
- `go-ssa-build` — for receiver access analysis
- Field-access walk over SSA (composable from `(wile goast ssa)` primitives;
  may warrant a helper `ssa-receiver-reads` / `ssa-receiver-writes`)
- `go-func-refs` — adjacent, not directly needed
- `types.MethodSet` lookup for interface membership — exposed via type info
  alists from `go-typecheck-package`

### Missing primitives (may need)

- `go-method-is-interface-member?` — determine whether a method satisfies any
  interface. Currently approximable by scanning all named interfaces in the
  package; a first-class primitive avoids the O(types × interfaces) walk.
- `ssa-receiver-field-reads(fn)` — returns list of field names read via the
  receiver parameter. Can be built on `ssa.Function.Params[0]` walk.

## Evaluation

### Dogfood on wile itself

After the G.1 fix, `wile/internal/parser/parser.go` is known clean for this
pattern. Run the detector on the remaining ~120 Go files in wile and
sibling repos:

- `wile/registry/**/*.go` — most primitives use `CallContext` (an interface),
  so interface-membership filter should drop most. Remaining methods are
  candidates.
- `wile/machine/**/*.go` — larger receivers (`MachineContext`, `NativeTemplate`)
  with many real state reads. Expect low flag rate.
- `wile-goast/goast*/**/*.go` — smaller helper types. Expect moderate rate.

Expected distribution: 5–20 L1 findings across wile + wile-goast; 1–5 after
L2 filter (joint use). Each reviewable in ~30 seconds.

### Calibration corpus

Use a small set of known-good and known-bad cases to calibrate the
`joint-use?` predicate:

**Positive (should flag):**
- The pre-fix `wrapMidParseEOF` (baseline)
- Hypothetical: `func (s *Server) formatError(e error) string { return fmt.Sprintf("%s: %v", s.name, e) }` — `s.name` read, combined with parameter `e`

**Negative (should not flag):**
- Standard accessors: `func (p *Point) X() float64 { return p.x }` (zero params)
- Stringers: `func (p Point) String() string { return fmt.Sprintf("(%d, %d)", p.x, p.y) }` (multiple field reads, zero params)
- Forwarders: `func (c *Cache) Get(k K) V { return c.inner.Get(k) }` (receiver field is itself the subject being delegated to, not context)
- Interface-satisfying methods: `func (p *P) Write(b []byte) (int, error) { ... }` (IM = true)

The forwarder case is subtle. By the L1 rule it would flag; by L2 (joint
use) it would not if `c.inner.Get(k)` is viewed as "`c.inner` is the
subject, `k` is the subject's argument." The discriminator is whether the
receiver read is *the subject* (forwarder, keep as method) or *context*
(convert to function).

The calibration corpus should include at least 20 positives and 20 negatives
before shipping.

### Belief output format

Per the Belief DSL `--emit` mode (once suppression ships), each finding
emits:

```scheme
;; Method: github.com/aalpar/wile/internal/parser.(*Parser).wrapMidParseEOF
;; File: internal/parser/parser.go:486
;; Receiver reads: {cur}
;; Parameters combined with receiver read: {err}
;; Severity: high (field passed as argument alongside parameter)
;; Suggested signature: wrapMidParseEOF(err error, tok tokenizer.Token, form string) error

(define-belief receiver-parameter-asymmetry-Parser.wrapMidParseEOF
  (sites (method "github.com/aalpar/wile/internal/parser"
                 "(*Parser).wrapMidParseEOF"))
  (expect (converted-to-free-function))
  (threshold 1.0 1))
```

The emitted belief then becomes a CI gate: if the method reappears in
method form, the belief fails.

## Relation to Prior Art

- **`goimports` / `gofmt`** — whitespace and import ordering only.
- **`gocritic` — `methodExprCall`, `rangeExprCopy`, etc.** — receiver-related
  style rules at the syntactic level. Does not analyze receiver *usage*
  inside the body.
- **`revive` — `receiver-naming`** — enforces consistent receiver names. Pure
  naming, not behavior.
- **Staticcheck `SA4006` (unused write), `SA5007` (infinite recursion via
  method-expression)** — unrelated defects.
- **IntelliJ/GoLand "Convert method to function"** — an IDE action, not a
  detection. Runs only when invoked.
- **`unparam`** — detects unused function parameters. Adjacent (also checks
  parameter ergonomics) but different pattern.

To my knowledge no existing Go linter targets this specific asymmetry. The
closest non-Go prior art is Fowler's *Move Method* refactoring; the
receiver-usage detection needed to drive it has been manual in most tools.
Converting `joint-use?` into a mechanical predicate is the novel piece.

## Scheme-side applicability (future)

The analogous pattern exists in Scheme for procedures closed over one field
of a struct:

```scheme
(define (make-logger ctx)
  (lambda (msg)
    (format (ctx-prefix ctx) msg)))
```

If `ctx` is used only for `ctx-prefix`, a free procedure taking `prefix` as
a parameter is often cleaner. But Scheme's closure idiom makes this a
different pattern — closures *are* the natural namespace. Out of scope for
Go-targeted detection; noted for possible future cross-language
generalization.

## Deliverables

| Phase | Artifact | Effort |
|---|---|---|
| 1 | Helper primitives `ssa-receiver-reads` / `ssa-receiver-writes` in `(wile goast ssa)` if not derivable from existing primitives | 2–4 hours |
| 2 | Belief `receiver-parameter-asymmetry` with L1 rule only | 4–6 hours |
| 3 | Calibration corpus (20 positives + 20 negatives across real codebases) | 2–4 hours |
| 4 | L2 refinement (joint-use predicate) | 4–6 hours |
| 5 | Run on wile + wile-goast, produce findings report, triage | 2 hours |
| 6 | (optional) Wrap as `go/analysis.Analyzer` in `(wile goast lint)` for IDE integration | 4–6 hours |

Total: 18–28 hours before IDE integration. Phases 1–5 form a minimum viable
shipping target.

## Open Questions

1. **Embedded field reads**: if `m` reads `r.Embedded.f` (transitive access
   through an embedded struct), does `|RR|` count as 1 or 2? Current
   proposal: count at the "outermost receiver field" level, so `r.Embedded`
   is one read. Revisit if corpus shows edge cases.

2. **Multi-path asymmetry**: what if `m` reads `r.cur` on one branch and
   `r.err` on another? `|RR| = 2`. L1 excludes. Correctly — it's not the
   same anti-pattern.

3. **Mutation of a receiver field followed by read**: a method that first
   writes `r.cur = x` and then reads it. `|RW| > 0` excludes by L1. Correct
   — mutation means the receiver *is* state-bearing.

4. **Closures returning methods**: `func (p *P) makeHandler() func(x int) int
   { return func(x int) int { return p.base + x } }` — the returned
   closure's body reads `p.base`. Detection should look at the method's
   own body, not returned closures. Straightforward in SSA.

5. **Severity threshold for emission**: emit only `high` findings in CI
   mode; include `medium`/`low` in `--discover` mode. Tunable.

## Why this is Scheme-appropriate

The detection is a pure structural query over compiled SSA — no runtime
semantics needed. It fits `(wile goast belief)`'s model: predicate over a
set of functions, classify each into a category, report per-category
results. The belief form is declarative, the site-selector filters, and
the checker returns a category symbol — all three parts map cleanly to
existing DSL primitives. No new machinery needed beyond the two optional
SSA helpers.

---

## References

- Fowler, M. *Refactoring: Improving the Design of Existing Code*, 1st ed.,
  Addison-Wesley, 1999. §3 (Code Smells), §7 (Move Method).
- Page-Jones, M. *Fundamentals of Object-Oriented Design in UML*,
  Addison-Wesley, 2000. Ch. 6 (Connascence).
- Go source: `cmd/compile/internal/types2` (method set computation),
  `golang.org/x/tools/go/ssa` (SSA form for field access analysis).
- Sibling plan: `2026-04-17-fca-duplicate-detection-design.md` (another
  belief-based detector using the same DSL).
- Discovery context: `wile/plans/2026-04-19-audit-findings-phase4-exceptions.md`
  (G.1 finding) and wile commit `c1000595` (the fix that surfaced the
  pattern).
