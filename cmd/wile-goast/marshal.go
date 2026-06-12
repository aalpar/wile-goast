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
	"encoding/json"
	"strings"

	"github.com/aalpar/wile/pkg/wile"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

var errMarshalUnsupported = werr.NewStaticError("marshal: unsupported wile value type")

// marshalToJSON converts the value returned by Engine.EvalMultiple into
// a Go any suitable for json.Marshal. EvalMultiple returns a wile.Value
// wrapper; this unwraps it to the underlying values.Value (via Internal)
// and delegates to marshalValue. Callers pass the EvalMultiple result
// directly without knowing about the wrapper.
func marshalToJSON(wv wile.Value) (any, error) {
	if wv == nil {
		return nil, nil
	}
	return marshalValue(wv.Internal())
}

// marshalValue converts a Wile value into a Go any suitable for
// json.Marshal, following the Phase 1 mapping table:
//
//	*values.Boolean    -> bool
//	*values.String     -> string
//	*values.Symbol     -> string (loses symbol/string distinction)
//	*values.Integer    -> int64 (number)
//	*values.BigInteger -> json.Number (exact, unquoted number)
//	*values.Float      -> float64 (number)
//	*values.Rational   -> string "9/10" (exact value preserved)
//	*values.Vector     -> array
//	*values.Pair       -> object (symbol-keyed alist), array (proper
//	                      list), or {"car","cdr"} (dotted pair)
//	empty list         -> []
//	void / nil         -> nil (caller/json omits)
//
// Alist keys are converted kebab-case -> snake_case at the JSON
// boundary. Returns errMarshalUnsupported for an uncovered value type.
func marshalValue(v values.Value) (any, error) {
	if v == nil || v.IsVoid() {
		return nil, nil
	}
	if values.IsEmptyList(v) {
		return []any{}, nil
	}
	switch x := v.(type) {
	case *values.Boolean:
		return x.Value, nil
	case *values.String:
		return x.Value, nil
	case *values.Symbol:
		return x.Key, nil
	case *values.Integer:
		return x.Value, nil
	case *values.BigInteger:
		// Private *big.Int; SchemeString gives the exact decimal. Emit
		// as an unquoted JSON number via json.Number (marshalled verbatim).
		return json.Number(x.SchemeString()), nil
	case *values.Float:
		return x.Value, nil
	case *values.Rational:
		// Exact value preserved as string per locked mapping ("9/10").
		return x.SchemeString(), nil
	case *values.Vector:
		return marshalVector(x)
	case *values.Pair:
		return marshalPair(x)
	}
	return nil, werr.WrapForeignErrorf(errMarshalUnsupported, "type=%T", v)
}

// marshalPair emits an alist as an object, a proper list as an array,
// and any other (dotted) pair as a {"car","cdr"} fallback.
func marshalPair(p *values.Pair) (any, error) {
	if isAlist(p) {
		obj := map[string]any{}
		var cur values.Value = p
		for !values.IsEmptyList(cur) {
			pair := cur.(*values.Pair) // spine guaranteed by isAlist
			entry := pair.Car().(*values.Pair)
			key := kebabToSnake(entry.Car().(*values.Symbol).Key)
			val, err := marshalValue(entry.Cdr())
			if err != nil {
				return nil, err
			}
			obj[key] = val
			cur = pair.Cdr()
		}
		return obj, nil
	}
	if isProperList(p) {
		arr := []any{}
		var cur values.Value = p
		for !values.IsEmptyList(cur) {
			pair := cur.(*values.Pair)
			elem, err := marshalValue(pair.Car())
			if err != nil {
				return nil, err
			}
			arr = append(arr, elem)
			cur = pair.Cdr()
		}
		return arr, nil
	}
	// Dotted pair fallback.
	car, err := marshalValue(p.Car())
	if err != nil {
		return nil, err
	}
	cdr, err := marshalValue(p.Cdr())
	if err != nil {
		return nil, err
	}
	return map[string]any{"car": car, "cdr": cdr}, nil
}

// isProperList reports whether v is a spine of *values.Pair cells
// terminating in the empty list.
func isProperList(v values.Value) bool {
	for !values.IsEmptyList(v) {
		pair, ok := v.(*values.Pair)
		if !ok {
			return false
		}
		v = pair.Cdr()
	}
	return true
}

// isAlist reports whether v is a non-empty proper list whose every
// element is a pair with a symbol car. The empty list is handled by the
// caller (emits []), so it returns false here by construction.
func isAlist(v values.Value) bool {
	saw := false
	for !values.IsEmptyList(v) {
		pair, ok := v.(*values.Pair)
		if !ok {
			return false
		}
		entry, ok := pair.Car().(*values.Pair)
		if !ok {
			return false
		}
		_, ok = entry.Car().(*values.Symbol)
		if !ok {
			return false
		}
		saw = true
		v = pair.Cdr()
	}
	return saw
}

func marshalVector(vec *values.Vector) (any, error) {
	n := vec.Length()
	arr := make([]any, 0, n)
	for i := range n {
		elem, err := marshalValue(vec.Get(i))
		if err != nil {
			return nil, err
		}
		arr = append(arr, elem)
	}
	return arr, nil
}

func kebabToSnake(s string) string {
	return strings.ReplaceAll(s, "-", "_")
}
