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

package pncounter

type Dot struct {
	ID  string
	Seq uint64
}

type CounterValue struct {
	N int64
}

type Counter struct {
	id    string
	store map[Dot]CounterValue
}

func (p *Counter) Increment(n int64) *Counter {
	var oldDot Dot
	var oldValue int64
	hasOld := false

	for d, v := range p.store {
		if d.ID == p.id {
			oldDot = d
			oldValue = v.N
			hasOld = true
			break
		}
	}

	newVal := CounterValue{N: oldValue + n}
	if hasOld {
		delete(p.store, oldDot)
	}
	p.store[Dot{ID: p.id}] = newVal

	delta := make(map[Dot]CounterValue)
	delta[Dot{ID: p.id}] = newVal

	return &Counter{store: delta}
}
