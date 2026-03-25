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
