# State-variable collapse

**Name:** state-variable collapse (`before → after`). The inverse is **state
splitting** — exploding one enum into per-state booleans, almost always a
regression. Thematically identical to `pull.md`: distributed state reduced to a
single scalar decision.

**Precondition.** Several boolean (or otherwise small) fields encode **mutually
exclusive states** of one entity. The booleans co-vary: only one is "true" at a
time, and the illegal combinations (two true at once) are unrepresentable in
intent but representable in the type.

**Transform (`before → after`), steps:**

1. **Enumerate the legal states** the booleans actually express.
2. **Replace the fields with one enum** whose cases are those states.
3. **Rewrite reads** (`if c.connecting`) as comparisons (`case connecting`).

**What it optimizes / sacrifices.** Makes illegal states unrepresentable, shrinks
the struct, and turns scattered boolean tests into one `switch`. Sacrifices the
ability to be in "two states at once" — which was a latent bug, not a feature.

**Prior art in this repo:** the `state-trace` skill targets exactly this —
bounded state split across multiple variables where distributed comparisons
collapse to a single scalar operation.

---

Before — three booleans encoding one mutually-exclusive state:

```go
type conn struct {
	connecting bool
	connected  bool
	closed     bool
}

func (c *conn) status() string {
	if c.connecting {
		return "connecting"
	}
	if c.connected {
		return "connected"
	}
	if c.closed {
		return "closed"
	}
	return "idle"
}
```

After — one enum; illegal combinations unrepresentable:

```go
type state int

const (
	idle state = iota
	connecting
	connected
	closed
)

type conn struct {
	state state
}

func (c *conn) status() string {
	switch c.state {
	case connecting:
		return "connecting"
	case connected:
		return "connected"
	case closed:
		return "closed"
	default:
		return "idle"
	}
}
```
