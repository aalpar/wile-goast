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

package gcounter

type Dot struct {
	ID  string
	Seq uint64
}

type GValue struct {
	N uint64
}

type Counter struct {
	id    string
	store map[Dot]GValue
}

func (g *Counter) Increment(n uint64) *Counter {
	var oldDot Dot
	var oldValue uint64
	hasOld := false

	for d, v := range g.store {
		if d.ID == g.id {
			oldDot = d
			oldValue = v.N
			hasOld = true
			break
		}
	}

	newVal := GValue{N: oldValue + n}
	if hasOld {
		delete(g.store, oldDot)
	}
	g.store[Dot{ID: g.id}] = newVal

	delta := make(map[Dot]GValue)
	delta[Dot{ID: g.id}] = newVal

	return &Counter{store: delta}
}
