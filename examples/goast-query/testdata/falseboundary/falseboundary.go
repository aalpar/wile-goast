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

package falseboundary

// Cache holds cached entries with a time-to-live.
type Cache struct {
	Entries []string
	TTL     int
}

// Index holds lookup keys with a version counter.
type Index struct {
	Keys    []string
	Version int
}

// UpdateBoth writes to both Cache and Index fields.
func UpdateBoth(c *Cache, idx *Index, entry string, key string) {
	c.Entries = append(c.Entries, entry)
	c.TTL = 300
	idx.Keys = append(idx.Keys, key)
	idx.Version++
}

// Invalidate clears both Cache and Index.
func Invalidate(c *Cache, idx *Index) {
	c.Entries = nil
	c.TTL = 0
	idx.Keys = nil
	idx.Version = 0
}

// Rebuild replaces both Cache and Index contents.
func Rebuild(c *Cache, idx *Index, entries []string, keys []string) {
	c.Entries = entries
	c.TTL = 600
	idx.Keys = keys
	idx.Version++
}

// CacheOnly touches only Cache fields — not cross-coupled.
func CacheOnly(c *Cache) {
	c.TTL = 0
}

// IndexOnly touches only Index fields — not cross-coupled.
func IndexOnly(idx *Index) {
	idx.Version++
}
