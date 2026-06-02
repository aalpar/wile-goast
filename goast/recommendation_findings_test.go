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

func TestRecommendSplitFindings(t *testing.T) {
	c := qt.New(t)
	engine := newBeliefEngine(t)
	pkg := "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster"
	out := eval(t, engine, `
		(import (wile goast split))
		(import (wile goast provenance))
		(define refs (go-func-refs "`+pkg+`"))
		(define report (recommend-split refs))
		(define f (recommend-split-findings report refs))
		(define all (append (cdr (assoc 'group-a f)) (cdr (assoc 'group-b f))))
		(list (cons 'n (length all)) (cons 'render (render-category "split" all)))
	`).SchemeString()

	t.Run("group functions are located at dupcluster.go", func(t *testing.T) {
		c.Assert(strings.Contains(out, "dupcluster.go"), qt.IsTrue, qt.Commentf("%s", out))
		c.Assert(strings.Contains(out, "split-group"), qt.IsTrue, qt.Commentf("%s", out))
	})
}
