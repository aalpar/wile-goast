// Package nodups has functions that share no external package references, so
// the FCA reference clustering produces no duplicate-candidate concepts and
// find_duplicates returns zero candidates. Used by the empty-success test.
package nodups

func Add(a, b int) int { return a + b }

func Greet() string { return "hi" }
