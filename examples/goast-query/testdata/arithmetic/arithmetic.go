package arithmetic

func Add(x, y int) int { return x + y }

func Negate(x int) int { return -x }

func Branch(x int) int {
	if x > 0 {
		return x + 1
	}
	return x - 1
}

func LoopSum(n int) int {
	sum := 0
	for i := 0; i < n; i++ {
		sum += i
	}
	return sum
}
