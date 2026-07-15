package main

import "fmt"

type Stringer interface{ String() string }
type S struct{ v int }

func (s *S) String() string { return fmt.Sprint(s.v) }

type A struct {
	b   *B
	tag Stringer
}
type B struct {
	a   *A
	tag Stringer
}

func main() {
	a := &A{tag: &S{1}}
	b := &B{tag: &S{2}}
	a.b = b
	b.a = a // A contains B contains A
	fmt.Println(a.b.a.b.a.tag.String()) // deep containment chain, dispatch at the end
}
