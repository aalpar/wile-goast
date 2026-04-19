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

// Target parameter state for wile-goast.
//
// current-go-target is an R7RS parameter holding the default Go package
// pattern used by pattern-accepting primitives (go-ssa-build,
// go-typecheck-package, go-load) when called with no explicit pattern.
//
// Initialized from the WILE_GOAST_TARGET env var at first access, with
// a fallback default of "./...". Scheme code reads via (current-go-target)
// and overrides via parameterize.
//
// See plans/2026-04-19-axis-b-analyzer-impl-design.md §5 and
// plans/2026-04-19-pr-1-target-setting-impl.md.

package goast

import (
	"os"
	"sync"

	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

const (
	// targetEnvVar is the environment variable consulted at initialization
	// to set the parameter's default value.
	targetEnvVar = "WILE_GOAST_TARGET"

	// targetDefaultPattern is the fallback when the env var is unset or empty.
	// "./..." matches wile-goast's current hardcoded target in existing
	// scripts — this preserves current behavior for new scripts that don't
	// set the parameter explicitly. This specific default is the sanctioned
	// exception to the project's "never default on nil/zero" rule: top-level
	// session-root parameters are the analog of primordial-thread state and
	// are allowed to default on zero.
	targetDefaultPattern = "./..."
)

// errExtractTargetError is the sentinel for ExtractTargetAndRest failures.
var errExtractTargetError = werr.NewStaticError("target extraction error")

var (
	targetOnce  sync.Once
	targetParam *machine.Parameter
)

// ExtractTargetAndRest unpacks the rest-list arg of a pattern-accepting
// primitive. If the rest list is non-empty, returns the first element
// (which the caller dispatches as string or GoSession) plus the remaining
// rest list. If the rest list is empty, reads the current-go-target
// parameter and returns its value plus an empty rest list.
//
// Used by PrimGoSSABuild (goastssa), PrimGoTypecheckPackage (goast), and
// PrimGoLoad (goast).
func ExtractTargetAndRest(mc *machine.MachineContext, restArg values.Value) (values.Value, values.Value, error) {
	tuple, ok := restArg.(values.Tuple)
	if !ok {
		return nil, nil, werr.WrapForeignErrorf(errExtractTargetError,
			"ExtractTargetAndRest: rest arg is %T, not a values.Tuple", restArg)
	}
	if values.IsEmptyList(tuple) {
		param := GetCurrentGoTargetParam()
		return mc.ResolveParameterValue(param), tuple, nil
	}
	pair, ok := tuple.(*values.Pair)
	if !ok {
		return nil, nil, werr.WrapForeignErrorf(errExtractTargetError,
			"ExtractTargetAndRest: non-empty rest is %T, not a *values.Pair", tuple)
	}
	return pair.Car(), pair.Cdr(), nil
}

// InitTargetState lazily initializes the current-go-target parameter.
// Idempotent — safe to call multiple times.
func InitTargetState() {
	targetOnce.Do(func() {
		initial := os.Getenv(targetEnvVar)
		if initial == "" {
			initial = targetDefaultPattern
		}
		targetParam = machine.NewParameter(values.NewString(initial), nil)
	})
}

// GetCurrentGoTargetParam returns the *machine.Parameter backing the
// current-go-target Scheme parameter. Calls InitTargetState first.
func GetCurrentGoTargetParam() *machine.Parameter {
	InitTargetState()
	return targetParam
}

// ResetTargetState resets the parameter for test isolation. Must not be
// called from production code — only from tests that need a clean slate.
func ResetTargetState() {
	targetOnce = sync.Once{}
	targetParam = nil
}
