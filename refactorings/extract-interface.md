# Extract interface

**Name:** extract interface (`before → after`). The inverse is **inline
interface** — depending on the concrete type again when only one implementation
will ever exist and the abstraction earns nothing.

**Precondition.** A function depends on a **concrete type but uses only a small
slice of its methods**, and you want to vary the implementation (tests, alternate
backends) or break a dependency.

**Transform (`before → after`), steps:**

1. **Name an interface** containing only the methods the consumer actually calls.
2. **Change the consumer's parameter** to that interface.
3. The concrete type satisfies it structurally — no `implements` declaration is
   needed in Go.

**What it optimizes / sacrifices.** The consumer depends on a capability, not a
type (dependency inversion); tests can pass a fake. Sacrifices nothing when the
interface stays small; a fat interface mirroring the whole concrete type is just
indirection.

**Detector status:** existing — `go-interface-implementors` finds the types
satisfying a given interface (the check that an extracted interface is actually
implemented); method-use profiling via `go-func-refs` suggests the minimal method
set.

---

Before — `backup` depends on the concrete `FileStore`:

```go
type FileStore struct{}

func (FileStore) Save(data []byte) error { return nil }

func backup(s FileStore, data []byte) error {
	return s.Save(data)
}
```

After — `backup` depends on the `Saver` capability:

```go
type FileStore struct{}

func (FileStore) Save(data []byte) error { return nil }

type Saver interface {
	Save(data []byte) error
}

func backup(s Saver, data []byte) error {
	return s.Save(data)
}
```
