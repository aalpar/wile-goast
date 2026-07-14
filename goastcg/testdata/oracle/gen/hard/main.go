package main

type Ia interface{ M() }
type TaA struct{}; func (TaA) M() {}
type TaB struct{}; func (TaB) M() {}
type TaC struct{}; func (TaC) M() {}

// generic FUNCTION over a constrained type param
type Runner interface{ ~struct{}; M() }

// generic function taking a slice of interface
func Each[T Ia](xs []T) { for _, v := range xs { v.M() } }

// generic type with a method that dispatches
type Box[T Ia] struct{ xs []T }
func (b *Box[T]) Add(v T) { b.xs = append(b.xs, v) }
func (b *Box[T]) Run()    { for _, v := range b.xs { v.M() } }

// nested generic instantiation
type Pair[A any, B any] struct{ a A; b B }
func MkPair[A any, B any](a A, b B) Pair[A, B] { return Pair[A, B]{a, b} }

var registry Box[Ia]
var spare    Box[Ia]

func main() {
	registry.Add(TaA{})
	spare.Add(TaB{})       // decoy: never dispatched
	registry.Run()

	Each([]Ia{TaC{}})      // generic func dispatch

	p := MkPair[Ia, int](TaA{}, 3)
	_ = p
}
