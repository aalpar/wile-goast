package main

import (
	"errors"
	"fmt"
)

// Domain interface + method: the query target.
type Widget interface{ Area() int }

type Box struct{ w, h int }

func (b *Box) Area() int { return b.w * b.h }

// An address-taken closure whose param the domain receiver's slice crosses.
func apply(f func(Widget) int, wg Widget) int { return f(wg) }

// Error-typed indirect call: the flood that must NOT be pulled into the domain query.
func onErr(cb func(error), e error) { cb(e) }

func main() {
	var wg Widget = &Box{3, 4}
	measure := func(w Widget) int { return w.Area() } // Area() dispatch: the domain receiver
	fmt.Println(apply(measure, wg))

	handler := func(e error) { fmt.Println(e.Error()) } // Error() dispatch on errorString
	onErr(handler, errors.New("boom"))
	onErr(handler, fmt.Errorf("wrapped: %w", errors.New("inner")))
}
