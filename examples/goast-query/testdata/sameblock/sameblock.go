package sameblock

func Foo() int { return 1 }
func Bar() int { return 2 }

func FooFirst() int {
	a := Foo()
	b := Bar()
	return a + b
}

func BarFirst() int {
	b := Bar()
	a := Foo()
	return a + b
}
