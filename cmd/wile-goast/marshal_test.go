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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"
)

// TestMarshalToJSON exercises every row of the Phase 1 mapping table by
// evaluating a Scheme literal, marshalling the resulting Wile value, and
// re-marshalling to JSON. json.Marshal sorts map keys, so object output
// is deterministic.
func TestMarshalToJSON(t *testing.T) {
	ctx := context.Background()
	ms := &mcpServer{}
	defer ms.closeAll()
	engine, err := ms.engineForKey(ctx, "marshal-test")
	qt.Assert(t, err, qt.IsNil)

	cases := []struct {
		name   string
		scheme string // Scheme source producing the value
		expect string // expected JSON after re-marshalling
	}{
		{"integer", "42", `42`},
		{"negative integer", "-7", `-7`},
		{"float", "3.14", `3.14`},
		{"rational", "9/10", `"9/10"`},
		{"bigint", "100000000000000000000", `100000000000000000000`},
		{"symbol", "'strong", `"strong"`},
		{"string", `"hello"`, `"hello"`},
		{"true", "#t", `true`},
		{"false", "#f", `false`},
		{"null", "'()", `[]`},
		{"proper list", "'(1 2 3)", `[1,2,3]`},
		{"alist", `'((a . 1) (b . 2))`, `{"a":1,"b":2}`},
		{"alist kebab to snake", `'((sites-expr . "x"))`, `{"sites_expr":"x"}`},
		{"nested alist", `'((outer . ((inner . 1))))`, `{"outer":{"inner":1}}`},
		{"dotted pair", `'(1 . 2)`, `{"car":1,"cdr":2}`},
		{"list of alists", `'(((name . "a")) ((name . "b")))`, `[{"name":"a"},{"name":"b"}]`},
		{"vector", "#(1 2 3)", `[1,2,3]`},
		{"void", "(if #f #f)", `null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			val, err := engine.EvalMultiple(ctx, tc.scheme)
			qt.Assert(t, err, qt.IsNil)
			got, err := marshalToJSON(val)
			qt.Assert(t, err, qt.IsNil)
			b, err := json.Marshal(got)
			qt.Assert(t, err, qt.IsNil)
			qt.Assert(t, string(b), qt.Equals, tc.expect)
		})
	}
}

// A value type with no mapping-table row (here, a closure) must surface
// errMarshalUnsupported rather than silently producing garbage JSON.
func TestMarshalToJSON_UnsupportedType(t *testing.T) {
	ctx := context.Background()
	ms := &mcpServer{}
	defer ms.closeAll()
	engine, err := ms.engineForKey(ctx, "marshal-unsupported")
	qt.Assert(t, err, qt.IsNil)

	val, err := engine.EvalMultiple(ctx, "(lambda (x) x)")
	qt.Assert(t, err, qt.IsNil)

	_, err = marshalToJSON(val)
	qt.Assert(t, errors.Is(err, errMarshalUnsupported), qt.IsTrue)
}
