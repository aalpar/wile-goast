# Introduce parameter object

**Name:** introduce parameter object (`before → after`). The inverse is **inline
parameter object** — flattening a rarely-used bundle back into discrete params.

**Precondition.** A function takes a **clump of parameters that travel together**
(coordinates, an RGB triple), and the same clump recurs across signatures.

**Transform (`before → after`), steps:**

1. **Group the co-travelling parameters** into a struct.
2. **Replace them** in the signature with one value of that type.
3. **Reuse the type** wherever the same clump appears.

**What it optimizes / sacrifices.** Shorter signatures, a named concept, and one
place to add a related field. Sacrifices directness for one-off calls, and can
hide that a function really needs only one field of the object — see
**preserve-whole-object**, the related move of passing an existing object rather
than extracting and threading its fields.

**Detector status:** could-build — signature analysis over `go-func-refs`:
parameter subsets that recur together across functions.

---

Before — eight positional parameters, two implicit clumps:

```go
func drawLine(x1, y1, x2, y2 int, r, g, b uint8) {}
```

After — the clumps become parameter objects:

```go
type point struct{ x, y int }
type rgb struct{ r, g, b uint8 }

func drawLine(from, to point, color rgb) {}
```
