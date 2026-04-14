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

package transcall

// Config holds connection settings.
type Config struct {
	Host string
	Port int
}

// Logger holds logging settings.
type Logger struct {
	Level string
	Path  string
}

// SetupConfig writes only Config fields.
func SetupConfig(c *Config) {
	c.Host = "localhost"
	c.Port = 8080
}

// SetupLogger writes only Logger fields.
func SetupLogger(l *Logger) {
	l.Level = "info"
	l.Path = "/var/log/app.log"
}

// Initialize has no direct field writes but transitively writes
// to both Config and Logger via its callees.
func Initialize(c *Config, l *Logger) {
	SetupConfig(c)
	SetupLogger(l)
}

// ResetConfig only writes Config fields, even transitively.
func ResetConfig(c *Config) {
	c.Host = ""
	c.Port = 0
}
