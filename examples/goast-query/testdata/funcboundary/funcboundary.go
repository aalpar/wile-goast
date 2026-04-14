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

package funcboundary

type Config struct {
	Timeout    int
	MaxRetries int
}

type Metrics struct {
	RequestCount int
	ErrorCount   int
}

type Session struct {
	Token  string
	Expiry int
}

type Auth struct {
	User  string
	Level int
}

type Response struct {
	Body   string
	Status int
}

// ── Split candidate: two independent state clusters, no cross-flow ──

// ProcessRequest writes Config and Metrics independently.
// No data flows from Config fields to Metrics fields.
func ProcessRequest(c *Config, m *Metrics, timeout int, count int) {
	c.Timeout = timeout
	c.MaxRetries = 3
	m.RequestCount = count
	m.ErrorCount = 0
}

// ── Split candidate in lattice, filtered by cross-flow ──

// ProcessAndRecord writes Config then uses Config values to compute Metrics.
// Data flows from Config fields to Metrics store — intentional coordination.
func ProcessAndRecord(c *Config, m *Metrics) {
	c.Timeout = 30
	c.MaxRetries = 3
	m.RequestCount = c.Timeout + c.MaxRetries
	m.ErrorCount = 0
}

// ── Merge candidates: overlapping session writes ──

// ResetSession clears session fields.
func ResetSession(s *Session) {
	s.Token = ""
	s.Expiry = 0
}

// ExpireSession also writes session fields.
func ExpireSession(s *Session) {
	s.Token = ""
	s.Expiry = -1
}

// ── Extract candidates: shared sub-operation (read-write mode) ──

// ValidateSession reads Session fields only (the sub-operation).
func ValidateSession(s *Session) bool {
	return s.Token != "" && s.Expiry > 0
}

// HandleAuth reads Session, writes Auth.
func HandleAuth(s *Session, a *Auth) {
	a.User = s.Token
	a.Level = s.Expiry
}

// HandleResponse reads Session, writes Response.
func HandleResponse(s *Session, r *Response) {
	r.Body = s.Token
	r.Status = s.Expiry
}

// ── Single-cluster controls (no recommendations expected) ──

func ConfigOnly(c *Config) {
	c.Timeout = 30
}

func MetricsOnly(m *Metrics) {
	m.RequestCount = 0
}
