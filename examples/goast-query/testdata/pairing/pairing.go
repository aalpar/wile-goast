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

package pairing

import "sync"

type Service struct {
	mu   sync.Mutex
	data map[string]int
}

func (s *Service) ReadSafe() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data["key"]
}

func (s *Service) WriteSafe(k string, v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[k] = v
}

func (s *Service) UpdateSafe(k string, v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[k]; ok {
		s.data[k] = v
	}
}

func (s *Service) DeleteSafe(k string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, k)
}

// ReadUnsafe is an intentional deviation: Lock without Unlock.
func (s *Service) ReadUnsafe() int {
	s.mu.Lock()
	return s.data["key"]
}
