package identity

// AddZero computes x + 0 (identity operation that SSA normalization should simplify).
func AddZero(x int) int {
	return x + 0
}

// JustReturn returns x directly (the normalized form of AddZero).
func JustReturn(x int) int {
	return x
}
