# Replace manual init with sync.Once

**Name:** replace manual init with `sync.Once` (`before → after`). No useful
inverse — hand-rolling a lazy-init guard where `sync.Once` fits is the pattern
this removes.

**Precondition.** A package- or struct-level value is lazily initialized behind a
hand-written "have I done this yet?" flag: a boolean (or nil) check that, on first
call, runs the initializer and sets the flag. The pattern is recognizable in the
AST — a guarded assignment to a shared variable plus a companion flag. It is also
usually a latent data race: the flag check and set are not atomic, so concurrent
first-callers can both initialize. Here `Get` checks `ready` before building `cfg`.

**Transform (`before → after`), steps:**

1. **Recognize the guard.** A read of a flag/nil, a conditional initializer, and a
   set of the flag — the AST shape of double-checked (or unchecked) lazy init.
2. **Replace the flag with a `sync.Once`** field/var.
3. **Move the initializer into `once.Do(func(){ … })`** and delete the flag and the
   manual guard. The read of the initialized value follows the `Do`.

**What it optimizes / sacrifices.** It makes the initialization exactly-once and
race-safe with no explicit locking, and states the intent ("init once") in the
type. It sacrifices nothing in the common case; the only caveat is that
`once.Do` blocks concurrent first-callers until init completes (correct, but a
slow initializer serializes them), and `Once` cannot be reset — if the value must
be re-initializable, this is the wrong tool.

**Detector status:** could-build (Tier A) — wire a query over the AST: a
double-checked (or flag-guarded) lazy initializer is a `sync.Once` candidate. The
AST shape is fully available; the un-built piece is the recognizer for the
guard/assign/set triple.

---

Before — a hand-rolled, non-race-safe "init once" flag:

```go
type Config struct{ v int }

var (
	cfg   *Config
	ready bool
)

func Get() *Config {
	if !ready {
		cfg = &Config{v: 42}
		ready = true
	}
	return cfg
}
```

After — `sync.Once` makes the lazy init exactly-once and race-safe:

```go
import "sync"

var (
	cfg  *Config
	once sync.Once
)

func Get() *Config {
	once.Do(func() {
		cfg = &Config{v: 42}
	})
	return cfg
}
```
