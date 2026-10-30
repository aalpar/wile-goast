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

// Package methodset declares one method of each receiver kind, a promoted
// method, generic code, and two init funcs, for the SSA method enumeration
// and go-func-refs qname / func-uses tests.
package methodset

import "strings"

type V struct{ s string }

// Upper has a value receiver: SSA reaches it only through FuncValue, since
// the *V method set holds a synthetic wrapper for it.
func (v V) Upper() string { return strings.ToUpper(v.s) }

type P struct{ n int }

func (p *P) Inc() { p.n++ }

// E embeds P; Inc is promoted, so it is P's method, not E's.
type E struct{ P }

type G[T any] struct{ v T }

func (g *G[T]) Get() T { return g.v }

func Map[T any](xs []T, f func(T) T) []T {
	for i := range xs {
		xs[i] = f(xs[i])
	}
	return xs
}

func Uses(e *E, v V, g *G[int]) string {
	e.Inc()
	_ = Map([]int{g.Get()}, func(i int) int { return i })
	return v.Upper()
}

var order []string

func init() { order = append(order, strings.TrimSpace(" a ")) }

func init() { order = append(order, strings.TrimSpace(" b ")) }
