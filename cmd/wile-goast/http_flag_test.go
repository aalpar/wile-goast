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

package main

import (
	"testing"
	"time"

	goflags "github.com/jessevdk/go-flags"

	qt "github.com/frankban/quicktest"
)

// The --http flag uses an optional value: absent ⇒ "", bare ⇒ loopback default,
// =ADDR ⇒ the given address. The dispatch in main() treats opts.HTTP != "" as
// "HTTP mode requested", so this contract must hold.
func TestHTTPFlagOptionalValue(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"absent", []string{}, ""},
		{"bare defaults to loopback", []string{"--http"}, "127.0.0.1:8080"},
		{"explicit address", []string{"--http=:9000"}, ":9000"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var o Options
			p := goflags.NewParser(&o, goflags.Default)
			_, err := p.ParseArgs(tc.args)
			qt.Assert(t, err, qt.IsNil)
			qt.Assert(t, o.HTTP, qt.Equals, tc.want)
		})
	}
}

// --http-idle-ttl configures the per-session idle reap interval. Absent ⇒ the
// 30m default; an explicit value parses as a duration; 0 passes through to
// disable the sweeper (mcp-go treats zero/negative as "never reap"), so the
// default must be distinguishable from an explicit zero.
func TestHTTPIdleTTLFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want time.Duration
	}{
		{"absent defaults to 30m", []string{}, 30 * time.Minute},
		{"explicit duration", []string{"--http-idle-ttl=45m"}, 45 * time.Minute},
		{"explicit zero disables", []string{"--http-idle-ttl=0"}, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var o Options
			p := goflags.NewParser(&o, goflags.Default)
			_, err := p.ParseArgs(tc.args)
			qt.Assert(t, err, qt.IsNil)
			qt.Assert(t, o.HTTPIdleTTL, qt.Equals, tc.want)
		})
	}
}

// --http-idle-ttl only has meaning for the HTTP server, so an explicit value
// without --http is rejected. The guard keys on whether the user *typed* the
// flag, not its value: the 30m default is applied unconditionally, so it must
// not trip the guard, while an explicit 30m (equal to the default) must.
func TestHTTPIdleTTLRequiresHTTP(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"idle-ttl without http is rejected", []string{"--http-idle-ttl=45m"}, true},
		{"idle-ttl with http is allowed", []string{"--http", "--http-idle-ttl=45m"}, false},
		{"http alone is allowed", []string{"--http"}, false},
		{"neither flag is allowed", []string{"-e", "(+ 1 2)"}, false},
		// The proving case: an explicit value equal to the default. It must be
		// rejected even though its value matches the default the "neither flag"
		// row accepts — guarding on intent (was the flag supplied?), not value.
		{"explicit default value without http is rejected", []string{"--http-idle-ttl=30m"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var o Options
			p := goflags.NewParser(&o, goflags.Default)
			_, err := p.ParseArgs(tc.args)
			qt.Assert(t, err, qt.IsNil)

			err = validateFlagCombinations(p, &o)
			if tc.wantErr {
				qt.Assert(t, err, qt.IsNotNil)
			} else {
				qt.Assert(t, err, qt.IsNil)
			}
		})
	}
}
