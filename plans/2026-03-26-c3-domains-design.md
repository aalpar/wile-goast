# C3 — Pre-built Abstract Domains

Four abstract domains that plug into C2's `run-analysis` as the lattice + transfer
function. Single library `(wile goast domains)` with shared `go-concrete-eval`.

**Status:** Design approved. Not yet implemented.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| File layout | Single `(wile goast domains)` — `domains.scm` + `domains.sld` | Domains are small (~30-50 lines each), share `go-concrete-eval`, imported together |
| `go-concrete-eval` scope | Minimal: `add sub mul div rem and or xor` | Enough for constant propagation; extend later (tracked in TODO) |
| Interval widening | Inside transfer function closure | Keeps C2 `run-analysis` API unchanged |
| Sign transfer tables | Hand-written per binop | 25 entries each; validated retroactively by C5 |
| Per-variable state | `map-lattice` from `(wile algebra)` | Keys = SSA instruction names |
| Testdata | New `examples/goast-query/testdata/arithmetic/arithmetic.go` | Existing testdata lacks integer operations |

## Exports

```scheme
(define-library (wile goast domains)
  (export
    ;; Concrete evaluator (shared, reused by C5)
    go-concrete-eval

    ;; Powerset analyses
    make-reaching-definitions
    make-liveness

    ;; Constant propagation
    make-constant-propagation

    ;; Sign domain
    sign-lattice
    make-sign-analysis

    ;; Interval domain
    interval-lattice
    make-interval-analysis)
  (import (wile algebra)
          (wile goast dataflow)
          (wile goast utils))
  (include "domains.scm"))
```

## Dependencies

- `(wile algebra)` — `powerset-lattice`, `flat-lattice`, `map-lattice`, `make-lattice`, lattice ops
- `(wile goast dataflow)` — `run-analysis`, `block-instrs`, `ssa-instruction-names`, `ssa-all-instrs`
- `(wile goast utils)` — `nf`, `tag?`, `flat-map`, `filter-map`
- SSA block structure from `go-ssa-build`

No new Go-side dependencies. Pure Scheme.

## Shared Infrastructure

### `go-concrete-eval`

```scheme
(define (go-concrete-eval opcode a b)
  ;; opcode: symbol ('add, 'sub, 'mul, 'div, 'rem, 'and, 'or, 'xor)
  ;; a, b: Scheme integers
  ;; returns: integer result or 'unknown
  (case opcode
    ((add) (+ a b))
    ((sub) (- a b))
    ((mul) (* a b))
    ((div) (if (zero? b) 'unknown (quotient a b)))
    ((rem) (if (zero? b) 'unknown (remainder a b)))
    ((and) (bitwise-and a b))
    ((or)  (bitwise-ior a b))
    ((xor) (bitwise-xor a b))
    (else  'unknown)))
```

Division by zero returns `'unknown` rather than erroring.

## Domain 1: Reaching Definitions (forward, powerset)

**Lattice:** `(powerset-lattice universe)` where universe = `(ssa-instruction-names fn)`.

**Transfer:**
```
transfer(block, state):
  for each instr in block-instrs(block):
    if instr has name:
      state = state union {name}
  return state
```

SSA names are unique definitions — no kill set needed.

**Factory:** `(make-reaching-definitions ssa-fn)` returns analysis result alist.

## Domain 2: Liveness (backward, powerset)

**Lattice:** `(powerset-lattice universe)`.

**Transfer (backward — state flows from successors toward entry):**
```
transfer(block, state):
  for each instr in block-instrs(block) (reverse order):
    if instr has name: state = state \ {name}    ;; kill: defined here
    for each op in operands(instr):
      if op in universe: state = state union {op} ;; gen: used here
  return state
```

**Factory:** `(make-liveness ssa-fn)` returns analysis result alist.

## Domain 3: Constant Propagation (forward, flat)

**Lattice:** `(map-lattice names value-lat)` where `value-lat = (flat-lattice '() equal?)`.

The flat lattice uses `'()` as the elements list — any concrete integer becomes
a lattice element. `flat-bottom` = undefined, `flat-top` = non-constant.

**Transfer:**
```
transfer(block, state):
  for each instr in block-instrs(block):
    if ssa-binop:
      v1 = lookup(state, operand1)
      v2 = lookup(state, operand2)
      if both concrete integers:
        result = go-concrete-eval(opcode, v1, v2)
      else if either is top: result = top
      else: result = bottom
      state[name] = result
    if ssa-phi:
      result = join of operand values in state
      state[name] = result
    else if has name:
      state[name] = top    ;; conservative for calls, loads, etc.
  return state
```

**Opcode extraction:** SSA binop nodes have an `op` field with the Go token
(`+`, `-`, `*`, `/`, `%`, `&`, `|`, `^`). Map to `go-concrete-eval` symbols.

**Factory:** `(make-constant-propagation ssa-fn)` returns analysis result alist.

## Domain 4: Sign Analysis (forward, 5-element)

**Lattice:** `{bot, neg, zero, pos, top}` — new lattice, not from `(wile algebra)`.

```
      top
    / | \
  neg zero pos
    \ | /
      bot
```

Hasse diagram: bot <= {neg, zero, pos} <= top. The three middle elements are
incomparable.

**State:** `(map-lattice names sign-lattice)`.

**Transfer tables (non-bot/top cases):**

add:

|     | neg | zero | pos |
|-----|-----|------|-----|
| neg | neg | neg  | top |
| zero| neg | zero | pos |
| pos | top | pos  | pos |

sub:

|     | neg | zero | pos |
|-----|-----|------|-----|
| neg | top | neg  | neg |
| zero| pos | zero | neg |
| pos | pos | pos  | top |

mul:

|     | neg | zero | pos |
|-----|-----|------|-----|
| neg | pos | zero | neg |
| zero| zero| zero | zero|
| pos | neg | zero | pos |

Rules for bot/top inputs:
- Any input is bot -> bot
- Any input is top -> top (exception: `mul` with `zero` -> `zero`)

**Abstracting constants:** Integer constants mapped to `neg`/`zero`/`pos` based on sign.

**Factory:** `(make-sign-analysis ssa-fn)` returns analysis result alist.

## Domain 5: Interval Analysis (forward)

**Lattice elements:** bot, `(lo . hi)` where `lo <= hi`, `top = ('-inf . '+inf)`.

Infinities represented as symbols `'+inf` and `'-inf`.

**Join:** `(min(a.lo, b.lo) . max(a.hi, b.hi))`. With bot as identity.

**Meet:** `(max(a.lo, b.lo) . min(a.hi, b.hi))`. Empty result -> bot.

**Leq:** `a.lo >= b.lo and a.hi <= b.hi` (b contains a).

**Transfer — interval arithmetic:**
- `add: (a.lo + b.lo . a.hi + b.hi)`
- `sub: (a.lo - b.hi . a.hi - b.lo)`
- `mul: (min of 4 products . max of 4 products)`
- `div`: conservative — if `0 in b` then top, else interval division

Infinity arithmetic: `x + +inf = +inf`, etc.

**Widening (inside transfer closure):**

```scheme
(define (make-interval-analysis ssa-fn . args)
  (let ((threshold (if (pair? args) (car args) 3)))
    ;; Track per-block iteration count in a mutable alist
    ;; After threshold iterations where a bound grows, push to +/-inf
    ...))
```

After `threshold` visits to a block where either bound changed, replace
the growing bound with `'-inf` or `'+inf`. Ensures termination for loops.

**Factory:** `(make-interval-analysis ssa-fn)` or `(make-interval-analysis ssa-fn threshold)`.

## Testdata

New file: `examples/goast-query/testdata/arithmetic/arithmetic.go`

```go
package arithmetic

func ConstAdd() int    { return 2 + 3 }
func ConstMul() int    { return 4 * 5 }
func NonConst(x int) int { return x + 1 }

func SignPos(x int) int {
    if x > 0 {
        return x * x  // pos * pos = pos
    }
    return 0
}

func IntervalLoop(n int) int {
    sum := 0
    for i := 0; i < n; i++ {
        sum += i
    }
    return sum
}
```

## Relationship to Other Tracks

- **C2:** Direct consumer. All factories call `run-analysis`.
- **C4:** Independent. Path algebra on call graphs, not CFG dataflow.
- **C5:** Validates C3 transfer tables. Reuses `go-concrete-eval`.
  Will extend `go-concrete-eval` to cover full opcode set.
- **C6:** Uses C3 lattices when graduating beliefs into dataflow assertions.
