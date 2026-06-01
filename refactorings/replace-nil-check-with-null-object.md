# Replace nil-check with null object

**Name:** replace nil-check with null object (`before → after`). The inverse is
**reintroduce nil** — sometimes wanted when "absent" is genuinely distinct from
"present but inert" and callers must tell them apart.

**Precondition.** An optional dependency is passed as a **nilable pointer**, so
every use site is guarded by `if x != nil`. The nil checks are scattered and easy
to forget — a forgotten one is a nil dereference.

**Transform (`before → after`), steps:**

1. **Define a no-op implementation** of the dependency's interface (the null
   object).
2. **Make the parameter an interface**, never nil — callers pass the no-op
   instead of `nil`.
3. **Delete the nil checks**; calls become unconditional.

**What it optimizes / sacrifices.** Use sites lose their guards and can't forget
one — a nil deref becomes impossible. Sacrifices the ability to distinguish
"no logger" from "a logger that drops everything," and adds a type; if callers
must branch on absence, the null object hides information they need.

**Detector status:** could-build — SSA nil-flow: a pointer parameter whose every
dereference is dominated by a `!= nil` guard is a null-object candidate.

---

Before — every use guarded against nil:

```go
type Logger struct{}

func (Logger) Log(msg string) {}

func run(log *Logger) {
	if log != nil {
		log.Log("start")
	}
	if log != nil {
		log.Log("end")
	}
}
```

After — a no-op logger means no guards:

```go
type Logger interface{ Log(msg string) }

type nopLogger struct{}

func (nopLogger) Log(string) {}

func run(log Logger) {
	log.Log("start")
	log.Log("end")
}
```
