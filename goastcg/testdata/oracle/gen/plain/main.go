package main

type Ia interface{ M() }
type TaA struct{}; func (TaA) M() {}
type TaB struct{}; func (TaB) M() {}

type BoxIa struct{ xs []Ia }
func (b *BoxIa) Add(v Ia) { b.xs = append(b.xs, v) }
func (b *BoxIa) Each()    { for _, v := range b.xs { v.M() } }

var registry BoxIa
var spare    BoxIa

func main() {
	registry.Add(TaA{})
	spare.Add(TaB{})
	registry.Each()
}
