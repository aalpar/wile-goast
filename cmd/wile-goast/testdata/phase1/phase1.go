// Package phase1 is a tiny Go package used only by MCP Phase 1
// integration tests. It contains two simple structs with a handful of
// methods exercising the belief, FCA, and split pipelines. Kept minimal
// and deterministic so per-tool assertions stay stable.
package phase1

import "sync"

// Counter guards a single int with a mutex; both methods pair Lock with
// a deferred Unlock — the canonical lock-pairing belief target.
type Counter struct {
	mu    sync.Mutex
	value int
}

func (c *Counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value++
}

func (c *Counter) Read() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

// Cache is an unsynchronized map wrapper — distinct field set from
// Counter, so the two share no struct boundary (no cross-boundary
// concept is expected between them).
type Cache struct {
	data map[string]int
}

func (c *Cache) Get(k string) (int, bool) {
	v, ok := c.data[k]
	return v, ok
}

func (c *Cache) Put(k string, v int) {
	c.data[k] = v
}
