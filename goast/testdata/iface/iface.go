package iface

import "errors"

var ErrNotFound = errors.New("not found")

// Store is a key-value store interface for testing go-interface-implementors.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

// MemoryStore implements Store with pointer receivers.
type MemoryStore struct {
	data map[string]string
}

func (m *MemoryStore) Get(key string) (string, error) {
	v, ok := m.data[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (m *MemoryStore) Set(key, value string) error {
	m.data[key] = value
	return nil
}

func (m *MemoryStore) Delete(key string) error {
	delete(m.data, key)
	return nil
}

// SimpleStore implements Store with value receivers.
type SimpleStore struct {
	data map[string]string
}

func (s SimpleStore) Get(key string) (string, error) {
	return s.data[key], nil
}

func (s SimpleStore) Set(key, value string) error {
	s.data[key] = value
	return nil
}

func (s SimpleStore) Delete(key string) error {
	delete(s.data, key)
	return nil
}

// NotAStore does not implement Store.
type NotAStore struct{}

func (n *NotAStore) Unrelated() {}
