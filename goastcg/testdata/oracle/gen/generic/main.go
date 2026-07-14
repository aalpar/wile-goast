package main

type Ia interface{ M() }
type TaA struct{}; func (TaA) M() {}
type TaB struct{}; func (TaB) M() {}

type Box[T any] struct{ xs []T }
func (b *Box[T]) Add(v T)          { b.xs = append(b.xs, v) }
func (b *Box[T]) All() []T         { return b.xs }

var registry Box[Ia]
var spare    Box[Ia]

func main() {
	registry.Add(TaA{})
	spare.Add(TaB{})
	for _, v := range registry.All() { v.M() }
}
