// Package recvasym is calibration data for the receiver-parameter-asymmetry
// belief checker. Each method exercises one classification category.
package recvasym

import "fmt"

type Server struct {
	name string
	host string
}

// candidate: reads s.name exactly once, writes no field, has one parameter.
func (s *Server) formatError(e error) string {
	return fmt.Sprintf("%s: %v", s.name, e)
}

// accessor: zero non-receiver parameters.
func (s *Server) Name() string {
	return s.name
}

// mutation: writes a receiver field — the receiver is state-bearing.
func (s *Server) SetHost(h string) {
	s.host = h
}

// multi-read: reads two distinct receiver fields.
func (s *Server) describe(x int) string {
	return fmt.Sprintf("%s/%s/%d", s.name, s.host, x)
}

// Stringer is a single-method interface; its members are excluded from
// candidacy (the method form is forced by the interface, not a smell).
type Stringer interface {
	Render(prefix string) string
}

type Tag struct {
	label string
}

// interface-method: Render satisfies Stringer. Without the exclusion it would
// read as a candidate (one field read, one param, joint use); the
// interface-member filter reclassifies it.
func (t *Tag) Render(prefix string) string {
	return fmt.Sprintf("%s<%s>", prefix, t.label)
}

type inner struct{ data map[string]string }

func (i *inner) Get(k string) string { return i.data[k] }

type Cache struct {
	store *inner
}

// forwarder: c.store is read once and is itself the subject of the call;
// the parameter k is its argument, not used jointly with the receiver read
// as data. The receiver read is the call subject, not context.
func (c *Cache) Get(k string) string {
	return c.store.Get(k)
}
