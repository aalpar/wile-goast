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

// go-ssa-narrow primitive: flow-sensitive SSA narrowing. Given an
// (ssa-func ...) alist and a value name, backward-walks the def-use
// chain and returns the set of concrete producing types plus confidence
// and reasons. See plans/2026-04-19-axis-b-analyzer-impl-design.md §6.

package goastssa

import (
	"github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

var errSSANarrow = werr.NewStaticError("ssa narrow error")

// PrimGoSSANarrow implements (go-ssa-narrow ssa-func value-name).
// Returns a narrow-result alist: (narrow-result (types (string ...))
// (confidence narrow|widened|no-paths) (reasons (symbol ...))).
func PrimGoSSANarrow(mc machine.CallContext) error {
	funcArg := mc.Arg(0)
	refField, ok := getRefField(funcArg)
	if !ok {
		return werr.WrapForeignErrorf(errSSANarrow,
			"go-ssa-narrow: first arg is not an ssa-func alist (no 'ref' field)")
	}
	fn, ok := UnwrapSSAFunctionRef(refField)
	if !ok {
		return werr.WrapForeignErrorf(errSSANarrow,
			"go-ssa-narrow: ref field is not an ssa-function-ref")
	}

	nameArg, ok := mc.Arg(1).(*values.String)
	if !ok {
		return werr.WrapForeignErrorf(errSSANarrow,
			"go-ssa-narrow: second arg must be a string, got %T", mc.Arg(1))
	}

	v, ok := findValueByName(fn, nameArg.Value)
	if !ok {
		// Argument error, not an analysis verdict. Distinguishes from
		// the algorithm's legitimate 'no-paths' result — a caller who
		// mistypes a value name gets told, rather than seeing a
		// plausible 'this value has no producing paths' claim.
		return werr.WrapForeignErrorf(errSSANarrow,
			"go-ssa-narrow: no value named %q in function %s",
			nameArg.Value, fn.Name())
	}

	result := narrow(fn, v)
	mc.SetValue(buildNarrowResult(result.Types, result.Confidence, result.Reasons))
	return nil
}

// getRefField returns the 'ref' field from an (ssa-func ...) tagged alist.
// The node must be a Pair whose car is the tag and cdr is the alist of fields.
func getRefField(node values.Value) (values.Value, bool) {
	p, ok := node.(*values.Pair)
	if !ok {
		return nil, false
	}
	return goast.GetField(p.Cdr(), "ref")
}

// buildNarrowResult constructs the Scheme-visible narrow-result alist.
func buildNarrowResult(typeNames []string, confidence string, reasons []string) values.Value {
	types := make([]values.Value, len(typeNames))
	for i, t := range typeNames {
		types[i] = goast.Str(t)
	}
	reasonVals := make([]values.Value, len(reasons))
	for i, r := range reasons {
		reasonVals[i] = goast.Sym(r)
	}
	return goast.Node("narrow-result",
		goast.Field("types", goast.ValueList(types)),
		goast.Field("confidence", goast.Sym(confidence)),
		goast.Field("reasons", goast.ValueList(reasonVals)),
	)
}
