package main

import "fmt"

type Stringer interface{ String() string }
type Impl struct{ v int }

func (i *Impl) String() string { return fmt.Sprint(i.v) }

type L1 struct{ p Stringer }
type L2 struct{ c *L1 }
type L3 struct{ c *L2 }

func main() {
	target := &Impl{42}

	// hop 1: target stored into L1
	a := &L1{}
	a.p = target
	// hop 2: L1 stored into L2
	x := &L2{}
	x.c = a
	// hop 3: L2 stored into L3
	y := &L3{}
	y.c = x

	// read back out through all three containers, then dispatch
	got := y.c.c.p // L3 -> L2 -> L1 -> Stringer
	fmt.Println(got.String())
}
