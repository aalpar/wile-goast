# The Andersen oracle

A vendored copy of `golang.org/x/tools/go/pointer`, kept as the **ground truth** against
which `(wile goast andersen)` is validated on the interface-dispatch corpora.

**It cannot analyze generic Go. Read the limits section before you trust a result.**

## Why it is vendored

`go/pointer` is x/tools' Andersen-style, inclusion-based, field-sensitive points-to
analysis. It was **removed from x/tools after v0.9.0** (mid-2023) and is absent from
every release since. The maintained Go analysis toolkit now tops out at VTA, which is
field-*based*: it abstracts container elements by type, so two distinct `[]T` globals
share one abstract node. That is the exact imprecision `(wile goast andersen)` exists to
fix, so the only available exact reference had to be recovered from a deleted package.

Sources, unmodified, from x/tools v0.9.0 (Apache-2.0, upstream copyright retained):

- `pointer/` the analysis itself
- `typeparams/` an internal dependency, also since deleted

The surrounding module pins x/tools v0.46 for `go/ssa` and `go/packages`, so the
vendored analysis runs against a current SSA builder.

It is third-party, and it predates these corpora by years, so it cannot have been tuned
to them.

## Limits. This is the important section

### It cannot analyze generic Go. There is no workaround

Run against any program containing generic types, `pointer.Analyze` **panics**:

    internal error in pointer analysis: ForEachElement called on type
    containing *types.TypeParam (please report this bug)

`pointer/gen.go:1335` iterates `prog.RuntimeTypes()`, and any type still carrying a
`*types.TypeParam` panics `ForEachElement`. **`ssa.InstantiateGenerics` does not help.**
It creates the instantiated copies but does not remove the generic *templates* from the
program, so their type-parameter-bearing types reach `RuntimeTypes()` anyway. The panic
is inside x/tools, not in code vendored here, and not in code that can be patched from
outside.

Reproduce, against the wile interpreter (36 generic function declarations):

    GOWORK=off GODEBUG=gotypesalias=0 go run ./genprobe ../../../../wile instantiate
    GOWORK=off GODEBUG=gotypesalias=0 go run ./genprobe ../../../../wile

Both panic.

**A narrow class of generic code does survive**, and this is a trap rather than a
feature. `gen/hard/` uses a generic container, a generic function, and a nested generic
instantiation, and the oracle resolves it exactly. It survives only because it never
converts a *generic* type into an interface, so no type-parameter-bearing type is ever
registered as a runtime type. That condition is invisible from the call site, cannot be
checked in advance, and real Go crosses it immediately. **Do not read a passing result on
generic code as evidence the oracle works on generic code.** That mistake has been made
twice on this exact analysis; see E27 (retracted) and E28.

### Materialized type aliases

    cannot flatten unsupported type *types.Alias

`types.Alias` was materialized in Go 1.22+, years after this code was frozen. This one
*is* fixable: set `GODEBUG=gotypesalias=0`. It clears the alias error and immediately
exposes the generics panic above, so it is necessary and not sufficient.

### Therefore

The oracle is usable **only on generics-free code**. That covers the interface-dispatch
corpora, which is what it is kept for. It does not cover real Go, and **there is no
oracle for real Go.** Establishing `(wile goast andersen)`'s correctness on real code
needs a different strategy, and that gap is open. See
`plans/2026-07-14-andersen-pointsto-design.md` section 9.

## Running it

The directory is its own Go module and is **not** part of the workspace, so `go.work`
must be disabled:

    GOWORK=off go run ./genprobe ./gen/plain

`main.go` is the scale/throughput probe; `genprobe/` prints every interface-dispatch call
site and the callees Andersen resolves for it. `gen/` holds the small subjects from E27
and E28: `plain` (no generics), `generic`, and `hard`.

## What it is exact on

126/126 across the two interface-dispatch corpora (E24), both of which are generics-free:

| corpus | n | Andersen | VTA (what we ship) |
|--------|---|----------|--------------------|
| A (decoy) | 63 | exact | exact |
| B (bound) | 63 | exact | over-approximates, by exactly the decoy |

The corpora themselves are **regenerable** rather than vendored, from
`~/ClaudeProjects/LLMAccuracy/code-restructuring/generate_interface_dispatch.py`:

    --depths 15 20 25 --impls 2 4 6 --count 7 --seed 31337 --dead 22

## What we are deliberately NOT copying

`pointer/hvn.go` (hash-value numbering) and `pointer/opt.go` (cycle elimination) are
solver optimizations that rewrite the constraint graph. `(wile goast andersen)` is barred
from those by design: its solve must remain `lfp(F)` with `F` composed from the algebra's
operators, so that a result speaks to the algebraic bet rather than to hand-written
solver engineering. Schedules may be optimized; the operator may not. See
`plans/2026-07-14-andersen-pointsto-design.md` section 8.

They remain here as a reference for what the fast path would look like.

`pointer/reflect.go` is a different matter: it is a genuine *model* of reflection, not an
optimization, and it is worth reading when the defeater story is implemented.
