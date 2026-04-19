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

// SSAFunctionRef is an opaque wrapper for *ssa.Function, embedded inside
// (ssa-func ...) tagged alists by the mapper so Scheme code can hand an
// SSA function back to Go-side primitives (go-ssa-narrow) without
// name-lookup overhead or ambiguity.

package goastssa

import (
	"golang.org/x/tools/go/ssa"

	"github.com/aalpar/wile/values"
)

const ssaFunctionRefTag = "ssa-function-ref"

// WrapSSAFunctionRef wraps an *ssa.Function as an OpaqueValue for Scheme.
func WrapSSAFunctionRef(fn *ssa.Function) *values.OpaqueValue {
	return values.NewOpaqueValue(ssaFunctionRefTag, fn)
}

// UnwrapSSAFunctionRef extracts an *ssa.Function from a values.Value.
// Returns nil, false if v is not an ssa-function-ref OpaqueValue.
func UnwrapSSAFunctionRef(v values.Value) (*ssa.Function, bool) {
	o, ok := v.(*values.OpaqueValue)
	if !ok || o.OpaqueTag() != ssaFunctionRefTag {
		return nil, false
	}
	fn, ok := o.Unwrap().(*ssa.Function)
	return fn, ok
}
