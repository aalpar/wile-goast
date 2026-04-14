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

package comutation

// Config holds server configuration. Methods that update Config
// should write Host, Port, and Timeout together (co-mutation belief).
type Config struct {
	Host    string
	Port    int
	Timeout int
}

// Reset writes all three fields — co-mutated.
func (c *Config) Reset(host string, port int, timeout int) {
	c.Host = host
	c.Port = port
	c.Timeout = timeout
}

// Init writes all three fields — co-mutated.
func (c *Config) Init(host string, port int, timeout int) {
	c.Host = host
	c.Port = port
	c.Timeout = timeout
}

// Update writes all three fields — co-mutated.
func (c *Config) Update(host string, port int, timeout int) {
	c.Host = host
	c.Port = port
	c.Timeout = timeout
}

// Restore writes all three fields — co-mutated.
func (c *Config) Restore(host string, port int, timeout int) {
	c.Host = host
	c.Port = port
	c.Timeout = timeout
}

// SetServer writes only Host and Port — intentional deviation.
func (c *Config) SetServer(host string, port int) {
	c.Host = host
	c.Port = port
}
