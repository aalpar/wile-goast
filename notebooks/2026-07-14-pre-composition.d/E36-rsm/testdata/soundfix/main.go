// Soundness fixture for the E34 offline-HVN diagnostic. Small enough that the
// difference-propagation solve CONVERGES, so the per-PE-class points-to
// uniformity check is authoritative. Exercises every constraint form that the
// god-set flood flows through: interfaces (invoke), errors (a flood in
// miniature), type assertions (untag + tfilter), closures (icall), struct
// field store/load (store/load/gep), and phi copy chains.
package main

import (
	"errors"
	"fmt"
)

type Animal interface{ Speak() string }

type Dog struct{ name string }

func (d *Dog) Speak() string { return "woof " + d.name }

type Cat struct{ name string }

func (c *Cat) Speak() string { return "meow " + c.name }

type box struct {
	err error
	a   Animal
}

// flood: one allocation site whose error flows to many callers (errors.New
// pattern). Every caller's err should be pointer-equivalent to this result if
// it is a pure copy.
func mkErr(s string) error { return errors.New(s) }

func chain(n int, cond bool) error {
	var e error
	e = mkErr("a")
	f := mkErr("b")
	g := mkErr("c")
	// phi: e merges across a branch (copy chain)
	if cond {
		e = f
	} else {
		e = g
	}
	// store into a struct field, then load back (store/gep/load)
	b := &box{err: e}
	back := b.err
	_ = back
	return e
}

func dispatch(a Animal) string { return a.Speak() } // invoke

func indirect(fn func(string) error, s string) error { return fn(s) } // icall

func assertErr(x interface{}) (*Dog, error) {
	if d, ok := x.(*Dog); ok { // untag (concrete)
		return d, nil
	}
	if a, ok := x.(Animal); ok { // tfilter (interface)
		_ = a.Speak()
	}
	return nil, mkErr("no dog")
}

func main() {
	var animals []Animal
	animals = append(animals, &Dog{"rex"}, &Cat{"tom"})
	for _, a := range animals {
		fmt.Println(dispatch(a))
	}
	fmt.Println(chain(3, true))
	fmt.Println(indirect(mkErr, "z"))
	d, e := assertErr(&Dog{"spot"})
	fmt.Println(d, e)
	// spread the flood: many error-typed values, all copies of mkErr results
	errs := make([]error, 0, 8)
	for i := 0; i < 8; i++ {
		errs = append(errs, mkErr(fmt.Sprint(i)))
	}
	for _, er := range errs {
		if er != nil {
			fmt.Println(er.Error())
		}
	}
}
