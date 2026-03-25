package gcounter

type Dot struct {
	ID  string
	Seq uint64
}

type GValue struct {
	N uint64
}

type Counter struct {
	id    string
	store map[Dot]GValue
}

func (g *Counter) Increment(n uint64) *Counter {
	var oldDot Dot
	var oldValue uint64
	hasOld := false

	for d, v := range g.store {
		if d.ID == g.id {
			oldDot = d
			oldValue = v.N
			hasOld = true
			break
		}
	}

	newVal := GValue{N: oldValue + n}
	if hasOld {
		delete(g.store, oldDot)
	}
	g.store[Dot{ID: g.id}] = newVal

	delta := make(map[Dot]GValue)
	delta[Dot{ID: g.id}] = newVal

	return &Counter{store: delta}
}
