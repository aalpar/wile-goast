package main

import "fmt"

type Stringer interface{ String() string }
type S struct{ v int }

func (s *S) String() string { return fmt.Sprint(s.v) }

type B struct {
	next *B
	tag  Stringer
}

func main() {
	b1 := &B{tag: &S{1}}
	b2 := &B{tag: &S{2}}
	b1.next = b2
	b2.next = b1 // two-node cycle
	p := b1
	for i := 0; i < 5; i++ { // traversal phi merges b1/b2 into p
		p = p.next
	}
	fmt.Println(p.tag.String())
}
