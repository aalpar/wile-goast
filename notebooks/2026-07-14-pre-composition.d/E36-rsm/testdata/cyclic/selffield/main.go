package main

import "fmt"

type Node struct {
	next *Node
	tag  Stringer
}

type Stringer interface{ String() string }
type S struct{ v int }

func (s *S) String() string { return fmt.Sprint(s.v) }

func main() {
	n := &Node{tag: &S{1}}
	n.next = n // self-referential field: x = x.next forever
	for i := 0; i < 3; i++ {
		n = n.next
	}
	fmt.Println(n.tag.String()) // dispatch on the cyclically-loaded tag
}
