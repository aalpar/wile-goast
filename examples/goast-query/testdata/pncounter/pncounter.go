package pncounter

type Dot struct {
	ID  string
	Seq uint64
}

type CounterValue struct {
	N int64
}

type Counter struct {
	id    string
	store map[Dot]CounterValue
}

func (p *Counter) Increment(n int64) *Counter {
	var oldDot Dot
	var oldValue int64
	hasOld := false

	for d, v := range p.store {
		if d.ID == p.id {
			oldDot = d
			oldValue = v.N
			hasOld = true
			break
		}
	}

	newVal := CounterValue{N: oldValue + n}
	if hasOld {
		delete(p.store, oldDot)
	}
	p.store[Dot{ID: p.id}] = newVal

	delta := make(map[Dot]CounterValue)
	delta[Dot{ID: p.id}] = newVal

	return &Counter{store: delta}
}
