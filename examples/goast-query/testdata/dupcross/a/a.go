// Copyright 2026 Aaron Alpar
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package a and its sibling b hold same-named functions with unrelated
// bodies. find_duplicates must keep a.Parse and b.Parse distinct, and must not
// score X.Encode against Y.Encode as identical because they share a method name.
package a

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"

	"github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcross/b"
)

// Parse sums comma-separated ints.
func Parse(s string) int {
	_ = unicode.IsDigit
	t := 0
	for _, f := range strings.Split(s, ",") {
		n, _ := strconv.Atoi(strings.TrimSpace(f))
		t += n
	}
	return t
}

type X struct{}

// Encode joins prefixed ints.
func (x *X) Encode(v []int) string {
	_ = bytes.MinRead
	parts := make([]string, 0, len(v))
	for _, n := range v {
		parts = append(parts, "x"+strconv.Itoa(n))
	}
	return strings.Join(parts, ";")
}

type Y struct{}

// Encode shares only its name with X.Encode.
func (y *Y) Encode(v []int) string {
	_ = bytes.MinRead
	if len(v) == 0 {
		return strings.Repeat("-", 3)
	}
	return strconv.Quote(strings.ToUpper("y"))
}

// CallsB calls b.Parse and nothing named Parse in this package.
func CallsB() int { return b.Parse("#") }
