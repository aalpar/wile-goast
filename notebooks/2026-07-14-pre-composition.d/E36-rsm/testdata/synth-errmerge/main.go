// synth-errmerge — a purpose-built fixture for the E36 Phase 2 Task 0
// flood-taxonomy gate (E36-rsm/main.go's TAXONOMY dump).
//
// ONE errors.New call site (inside mkErr) is reached from 24 distinct static
// call sites (c00..c23), each its own calling context. All 24 returned errors
// flow into a SHARED interface dispatch (the invoke of error.Error() in
// dispatch). Context-insensitively, every context's error merges into ONE
// *errors.errorString object -- exactly the multi-allocation merge E33/E34
// identified on wile and Phase 2's per-context heap cloning is designed to
// split (unlike soundfix's #1 god object, runtime.mheap_, a GLOBAL with no
// allocation context to clone).
//
// No fmt import: fmt.Println's formatting machinery drags in enough
// runtime-global surface (mheap/workType/scavenger/...) to outrank the
// errorString merge in soundfix's own taxonomy dump (errors.errorString
// ranks #67 there, holders 3172, behind a dozen runtime globals/allocs). This
// fixture keeps the merge itself the dominant, or only, contender for #1.
package main

import "errors"

// Sink keeps results live without importing fmt.
var Sink int

func mkErr(tag string) error { return errors.New(tag) } // the ONE allocation site

func c00() error { return mkErr("00") }
func c01() error { return mkErr("01") }
func c02() error { return mkErr("02") }
func c03() error { return mkErr("03") }
func c04() error { return mkErr("04") }
func c05() error { return mkErr("05") }
func c06() error { return mkErr("06") }
func c07() error { return mkErr("07") }
func c08() error { return mkErr("08") }
func c09() error { return mkErr("09") }
func c10() error { return mkErr("10") }
func c11() error { return mkErr("11") }
func c12() error { return mkErr("12") }
func c13() error { return mkErr("13") }
func c14() error { return mkErr("14") }
func c15() error { return mkErr("15") }
func c16() error { return mkErr("16") }
func c17() error { return mkErr("17") }
func c18() error { return mkErr("18") }
func c19() error { return mkErr("19") }
func c20() error { return mkErr("20") }
func c21() error { return mkErr("21") }
func c22() error { return mkErr("22") }
func c23() error { return mkErr("23") }

// dispatch is the shared interface invoke site every caller's error flows
// through -- the demand-query receiver whose slice would, context-
// insensitively, pull in the whole 24-way merged union.
func dispatch(e error) int {
	if e == nil {
		return 0
	}
	return len(e.Error())
}

func main() {
	callers := []func() error{
		c00, c01, c02, c03, c04, c05, c06, c07, c08, c09, c10, c11,
		c12, c13, c14, c15, c16, c17, c18, c19, c20, c21, c22, c23,
	}
	for _, f := range callers {
		Sink += dispatch(f())
	}
}
