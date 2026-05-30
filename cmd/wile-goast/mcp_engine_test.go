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
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
)

// A per-session engine manager must return the same engine for repeated
// lookups of one session key — that is what preserves cross-call state
// (go-load sessions, defined beliefs) within a single client.
func TestEngineManager_SameKeyReturnsSameEngine(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ms := &mcpServer{}
	defer ms.closeAll()

	e1, err := ms.engineForKey(ctx, "session-a")
	c.Assert(err, qt.IsNil)
	c.Assert(e1, qt.IsNotNil)

	e2, err := ms.engineForKey(ctx, "session-a")
	c.Assert(err, qt.IsNil)
	c.Assert(e2, qt.Equals, e1)
}

// Distinct session keys must get distinct engines — that is the isolation
// guarantee that keeps one HTTP client's state invisible to another.
func TestEngineManager_DistinctKeysGetDistinctEngines(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ms := &mcpServer{}
	defer ms.closeAll()

	a, err := ms.engineForKey(ctx, "session-a")
	c.Assert(err, qt.IsNil)
	b, err := ms.engineForKey(ctx, "session-b")
	c.Assert(err, qt.IsNil)

	c.Assert(a, qt.Not(qt.Equals), b)
}

// Evicting a session (the OnUnregisterSession path) must drop its engine, so a
// later lookup of the same key builds a fresh one rather than reusing the
// closed engine.
func TestEngineManager_EvictRebuildsFresh(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ms := &mcpServer{}
	defer ms.closeAll()

	first, err := ms.engineForKey(ctx, "session-a")
	c.Assert(err, qt.IsNil)

	ms.evict("session-a")

	second, err := ms.engineForKey(ctx, "session-a")
	c.Assert(err, qt.IsNil)
	c.Assert(second, qt.Not(qt.Equals), first)
}
