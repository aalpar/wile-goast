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

package goast

// Set is a membership-only collection: the map value carries no data, so the
// element type is a marker, not a payload. This is the canonical Go set idiom
// (the same shape the standard library uses in its own examples) given a name
// and methods so call sites read intent rather than map mechanics.
type Set[T comparable] map[T]struct{}

// NewSet returns a Set seeded with items. Construction routes through Add so a
// static literal and a runtime-built set share one code path.
func NewSet[T comparable](items ...T) Set[T] {
	s := make(Set[T], len(items))
	for _, it := range items {
		s.Add(it)
	}
	return s
}

// Add inserts item, a no-op if already present.
func (s Set[T]) Add(item T) { s[item] = struct{}{} }

// Contains reports whether item is a member.
func (s Set[T]) Contains(item T) bool {
	_, ok := s[item]
	return ok
}
