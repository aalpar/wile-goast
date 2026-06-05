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
