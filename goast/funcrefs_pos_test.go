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

package goast_test

import (
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestGoFuncRefs_Position(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	// Every func-ref entry for a real declaration must carry a source position.
	out := eval(t, engine, `
		(define refs (go-func-refs
		  "github.com/aalpar/wile-goast/goast/testdata/iface"))
		(let loop ((rs refs))
		  (if (null? rs) #f
		    (let ((p (assoc 'pos (cdr (car rs)))))
		      (if (and p (string? (cdr p))) (cdr p) (loop (cdr rs))))))
	`).SchemeString()
	c.Assert(out, qt.Not(qt.Equals), "#f")
	c.Assert(strings.Contains(out, "iface.go:"), qt.IsTrue, qt.Commentf("pos = %s", out))
}
